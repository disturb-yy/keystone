// Package codex 将受限 ExecutionInput 适配到 Codex CLI。
package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/disturb-yy/keystone/internal/execution"
)

const (
	defaultTimeout  = 30 * time.Minute
	processStopTime = 2 * time.Second
)

// CommandFactory 是 Codex 进程构造 seam；参数保持数组边界，不经过 shell。
type CommandFactory func(context.Context, string, ...string) *exec.Cmd

// Adapter 是 V1 的 Codex RuntimeAdapter。
type Adapter struct {
	Binary      string
	Guard       *execution.Guard
	Now         func() time.Time
	Environment func() []string
	Command     CommandFactory
}

// New 创建 Codex Adapter；binary 为空时按 PATH 发现 codex。
func New(binary string) *Adapter {
	return &Adapter{
		Binary:      binary,
		Guard:       execution.NewGuard(),
		Now:         time.Now,
		Environment: os.Environ,
		Command:     exec.CommandContext,
	}
}

// Name 返回 Runtime capability 名称。
func (a *Adapter) Name() string { return execution.RuntimeCodex }

// Run 执行一次不可自动重试的 Codex CLI，并收集独立证据。
func (a *Adapter) Run(ctx context.Context, input execution.ExecutionInput) (execution.RuntimeResult, error) {
	result := execution.RuntimeResult{}
	if ctx == nil {
		return result, errors.New("run codex: nil context")
	}
	if input.Runtime == "" {
		input.Runtime = execution.RuntimeCodex
	}
	if err := input.Validate(); err != nil {
		return result, err
	}
	if input.Runtime != execution.RuntimeCodex {
		return result, fmt.Errorf("run codex: unsupported runtime %q", input.Runtime)
	}
	if a == nil || a.Guard == nil {
		return result, errors.New("run codex: guard is required")
	}
	workspace, err := a.Guard.ValidateWorkspace(ctx, input.Workspace)
	if err != nil {
		return result, err
	}
	before, err := a.Guard.Head(ctx, workspace)
	if err != nil {
		return result, err
	}
	if input.BeforeRevision != "" {
		before = input.BeforeRevision
	}
	return a.runValidated(ctx, input, workspace, before)
}

func (a *Adapter) runValidated(ctx context.Context, input execution.ExecutionInput, workspace, before string) (execution.RuntimeResult, error) {
	limits := input.Limits
	if limits == (execution.Limits{}) {
		limits = execution.DefaultLimits()
	}
	timeout := input.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	environment := input.Environment
	if environment == nil {
		environmentProvider := a.Environment
		if environmentProvider == nil {
			environmentProvider = os.Environ
		}
		environment = environmentProvider()
	}
	tempBase, err := prepareRuntimeTempBase(workspace, input.ResultMode)
	if err != nil {
		return execution.RuntimeResult{}, err
	}
	env, cleanup, err := a.Guard.PrepareEnvironmentAt(runCtx, environment, tempBase)
	if err != nil {
		return execution.RuntimeResult{}, err
	}
	defer cleanup()
	started := a.now()
	candidatePath, cleanupCandidate, err := prepareCandidateOutput(input.ResultMode, tempBase)
	if err != nil {
		return execution.RuntimeResult{}, err
	}
	defer cleanupCandidate()
	observation, runErr := a.runProcess(runCtx, workspace, input.Instruction, env, limits, candidatePath)
	result := observation.result(started, a.now(), before)
	if candidatePath != "" {
		candidate, candidateErr := readCandidateOutput(candidatePath, limits.CandidateBytes)
		result.Candidate = candidate
		if candidateErr != nil {
			result.CaptureFailures = append(result.CaptureFailures, execution.CaptureFailure{Kind: execution.ArtifactCandidate, Stage: "candidate-output", Error: "candidate output unavailable"})
		}
	}
	if runErr != nil && !isProcessExit(runErr) && !errors.Is(runErr, context.DeadlineExceeded) {
		result.FailureReason = runErr.Error()
		return result, fmt.Errorf("run codex process: %w", runErr)
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.FailureReason = "runtime timeout"
	}
	evidence, evidenceErr := a.Guard.Evidence(ctx, workspace, limits)
	if evidenceErr != nil {
		result.CaptureFailures = append(result.CaptureFailures,
			execution.CaptureFailure{Kind: execution.ArtifactDiff, Stage: "git-evidence", Error: evidenceErr.Error()},
			execution.CaptureFailure{Kind: execution.ArtifactChangedFiles, Stage: "git-evidence", Error: evidenceErr.Error()},
		)
		return result, fmt.Errorf("collect codex evidence: %w", evidenceErr)
	}
	result.AfterRevision = evidence.Revision
	diff, diffTruncated := execution.LimitBytes(evidence.Diff, limits.DiffBytes)
	result.Diff = artifact(execution.ArtifactDiff, diff, diffTruncated)
	result.ChangedFileList = evidence.ChangedFiles
	result.ChangedFiles = execution.ChangedFilesArtifact(evidence.ChangedFiles, limits.ChangedFilesBytes)
	result.GuardFindings = a.Guard.CheckRevision(before, evidence.Revision)
	execution.EnforceTotalLimit(&result, limits)
	return result, nil
}

func (a *Adapter) runProcess(ctx context.Context, workspace, instruction string, environment []string, limits execution.Limits, candidatePath string) (processObservation, error) {
	binary := a.Binary
	if binary == "" {
		binary = "codex"
	}
	sandboxMode := "workspace-write"
	if candidatePath != "" {
		sandboxMode = "read-only"
	}
	args := []string{"exec", "--json", "--ephemeral", "--sandbox", sandboxMode, "--ask-for-approval", "never"}
	if candidatePath != "" {
		args = append(args, "--output-last-message", candidatePath)
	}
	args = append(args, "-")
	command := a.Command
	if command == nil {
		command = exec.CommandContext
	}
	// CommandContext 的默认取消只终止父进程；ProcessTree 才是 Runtime 及其
	// tool 子进程的唯一生命周期 owner。
	cmd := command(context.WithoutCancel(ctx), binary, args...)
	cmd.Dir = workspace
	cmd.Env = environment
	stdout, stderr := newLimitedWriter(limits.StdoutBytes), newLimitedWriter(limits.StderrBytes)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return processObservation{stdout: stdout, stderr: stderr, runErr: err}, fmt.Errorf("prepare codex input pipe: %w", err)
	}
	tree, err := execution.StartProcessTree(cmd)
	if err != nil {
		_ = stdin.Close()
		return processObservation{stdout: stdout, stderr: stderr, exitCode: processExitCode(cmd, err), runErr: err}, err
	}
	inputCh := make(chan error, 1)
	go func() {
		_, writeErr := io.WriteString(stdin, instruction)
		closeErr := stdin.Close()
		inputCh <- errors.Join(writeErr, closeErr)
	}()
	select {
	case <-tree.Done():
		err = tree.Wait()
		if inputErr := <-inputCh; inputErr != nil && err == nil {
			err = fmt.Errorf("write codex instruction: %w", inputErr)
		}
	case <-ctx.Done():
		_ = stdin.Close()
		err = errors.Join(ctx.Err(), execution.TerminateProcessTree(tree, processStopTime))
	}
	return processObservation{stdout: stdout, stderr: stderr, exitCode: processExitCode(cmd, err), runErr: err}, err
}

func prepareCandidateOutput(resultMode, tempBase string) (string, func(), error) {
	if resultMode != execution.ResultModePlanningCandidate {
		return "", func() {}, nil
	}
	file, err := os.CreateTemp(tempBase, "keystone-planning-candidate-*")
	if err != nil {
		return "", nil, fmt.Errorf("prepare codex candidate output: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("prepare codex candidate output: %w", err)
	}
	return path, func() { _ = os.Remove(path) }, nil
}

func prepareRuntimeTempBase(workspace, resultMode string) (string, error) {
	workspace, err := filepath.EvalSymlinks(filepath.Clean(workspace))
	if err != nil || !filepath.IsAbs(workspace) {
		return "", errors.New("prepare codex runtime temporary base: workspace is unavailable")
	}
	if resultMode == execution.ResultModePlanningCandidate {
		parent := filepath.Dir(workspace)
		if filepath.Base(workspace) != "source" || !strings.HasPrefix(filepath.Base(parent), "keystone-planning-snapshot-") {
			return "", errors.New("prepare codex runtime temporary base: planning snapshot shape is invalid")
		}
		base := filepath.Join(parent, "runtime")
		if err := os.Mkdir(base, 0o700); err != nil && !os.IsExist(err) {
			return "", errors.New("prepare codex runtime temporary base: create failed")
		}
		info, err := os.Lstat(base)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("prepare codex runtime temporary base: verification failed")
		}
		return base, nil
	}
	candidates := []string{os.TempDir()}
	if runtime.GOOS != "windows" && filepath.Clean(os.TempDir()) != string(os.PathSeparator)+"tmp" {
		candidates = append(candidates, string(os.PathSeparator)+"tmp")
	}
	for _, candidate := range candidates {
		resolved, resolveErr := filepath.EvalSymlinks(filepath.Clean(candidate))
		if resolveErr != nil || !filepath.IsAbs(resolved) || pathWithin(workspace, resolved) {
			continue
		}
		info, statErr := os.Lstat(resolved)
		if statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return resolved, nil
		}
	}
	return "", errors.New("prepare codex runtime temporary base: no safe directory")
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func readCandidateOutput(path string, limit int64) (execution.Artifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return execution.NewArtifact(execution.ArtifactCandidate, nil, false, err), err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return execution.NewArtifact(execution.ArtifactCandidate, nil, false, err), err
	}
	truncated := int64(len(content)) > limit
	if truncated {
		content = content[:limit]
	}
	return execution.NewArtifact(execution.ArtifactCandidate, content, truncated, nil), nil
}

type processObservation struct {
	stdout   *limitedWriter
	stderr   *limitedWriter
	exitCode *int
	runErr   error
}

func (o processObservation) result(started, completed time.Time, before string) execution.RuntimeResult {
	return execution.RuntimeResult{
		StartedAt:      started,
		CompletedAt:    completed,
		ExitCode:       o.exitCode,
		BeforeRevision: before,
		Stdout:         artifact(execution.ArtifactStdout, o.stdout.Bytes(), o.stdout.truncated),
		Stderr:         artifact(execution.ArtifactStderr, o.stderr.Bytes(), o.stderr.truncated),
	}
}

func (a *Adapter) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func artifact(kind execution.ArtifactKind, content []byte, truncated bool) execution.Artifact {
	return execution.NewArtifact(kind, content, truncated, nil)
}

func processExitCode(command *exec.Cmd, err error) *int {
	if err == nil {
		value := 0
		return &value
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		value := exitErr.ExitCode()
		return &value
	}
	return nil
}

func isProcessExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

type limitedWriter struct {
	data      bytes.Buffer
	limit     int64
	truncated bool
}

func newLimitedWriter(limit int64) *limitedWriter { return &limitedWriter{limit: limit} }

func (w *limitedWriter) Write(value []byte) (int, error) {
	remaining := w.limit - int64(w.data.Len())
	if remaining <= 0 {
		w.truncated = len(value) > 0
		return len(value), nil
	}
	if int64(len(value)) > remaining {
		_, _ = w.data.Write(value[:remaining])
		w.truncated = true
		return len(value), nil
	}
	_, _ = w.data.Write(value)
	return len(value), nil
}

func (w *limitedWriter) Bytes() []byte { return append([]byte(nil), w.data.Bytes()...) }

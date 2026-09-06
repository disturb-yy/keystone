// Package codex 将受限 ExecutionInput 适配到 Codex CLI。
package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/disturb-yy/keystone/internal/execution"
)

const defaultTimeout = 30 * time.Minute

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
	env, cleanup, err := a.Guard.PrepareEnvironment(runCtx, environment)
	if err != nil {
		return execution.RuntimeResult{}, err
	}
	defer cleanup()
	started := a.now()
	observation, runErr := a.runProcess(runCtx, workspace, input.Instruction, env, limits)
	result := observation.result(started, a.now(), before)
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
		result.CaptureFailures = append(result.CaptureFailures, execution.CaptureFailure{Stage: "git-evidence", Error: evidenceErr.Error()})
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

func (a *Adapter) runProcess(ctx context.Context, workspace, instruction string, environment []string, limits execution.Limits) (processObservation, error) {
	binary := a.Binary
	if binary == "" {
		binary = "codex"
	}
	args := []string{"exec", "--json", "--ephemeral", "--sandbox", "workspace-write", "--ask-for-approval", "never", "-"}
	command := a.Command
	if command == nil {
		command = exec.CommandContext
	}
	cmd := command(ctx, binary, args...)
	cmd.Dir = workspace
	cmd.Env = environment
	cmd.Stdin = strings.NewReader(instruction)
	stdout, stderr := newLimitedWriter(limits.StdoutBytes), newLimitedWriter(limits.StderrBytes)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	return processObservation{stdout: stdout, stderr: stderr, exitCode: processExitCode(cmd, err), runErr: err}, err
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

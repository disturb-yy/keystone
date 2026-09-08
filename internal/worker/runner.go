package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/execution"
)

const (
	defaultPollInterval     = time.Second
	defaultHeartbeatSeconds = 5 * time.Second
	defaultRuntimeTimeout   = 30 * time.Minute
	defaultRuntimeStopTime  = 2 * time.Second
)

var errWorkerLeaseNotRenewed = errors.New("worker lease was not renewed")

// RunnerConfig 描述一个独立 Worker 的主动循环和可替换 seam。
type RunnerConfig struct {
	Client             *Client
	WorkerID           string
	Capabilities       []string
	Runtimes           map[string]execution.RuntimeAdapter
	PollInterval       time.Duration
	HeartbeatInterval  time.Duration
	RuntimeTimeout     time.Duration
	RuntimeStopTimeout time.Duration
	Environment        func() []string
	Now                func() time.Time
	Sleep              func(context.Context, time.Duration) error
	// Verifier 是 kind=verify Assignment 的固定执行器；nil 使用只读 CommandVerifier。
	Verifier VerificationExecutor
}

// Runner 执行 Register → Heartbeat/Pull → Runtime → Report 顺序。
type Runner struct {
	config RunnerConfig
}

type runtimeStopResult struct {
	value execution.RuntimeResult
	err   error
}

type verificationStopResult struct {
	value workercontract.VerificationReport
	err   error
}

// NewRunner 创建一个不拥有外部资源的 Worker Runner。
func NewRunner(config RunnerConfig) (*Runner, error) {
	if config.Client == nil || strings.TrimSpace(config.WorkerID) == "" {
		return nil, errors.New("create worker runner: client and worker id are required")
	}
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = defaultHeartbeatSeconds
	}
	if config.RuntimeTimeout <= 0 {
		config.RuntimeTimeout = defaultRuntimeTimeout
	}
	if config.RuntimeStopTimeout <= 0 {
		config.RuntimeStopTimeout = defaultRuntimeStopTime
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Sleep == nil {
		config.Sleep = sleepContext
	}
	if config.Verifier == nil {
		config.Verifier = CommandVerifier{}
	}
	return &Runner{config: config}, nil
}

// Run 等待上下文取消；注册失败时不会 Pull 或调用 Runtime。
func (r *Runner) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("run worker: nil context")
	}
	capabilities := append([]string(nil), r.config.Capabilities...)
	if _, err := r.config.Client.Register(ctx, workercontract.Register{WorkerID: r.config.WorkerID, ProtocolVersion: workercontract.ProtocolVersionV1, Capabilities: capabilities}); err != nil {
		return fmt.Errorf("register worker: %w", err)
	}
	for {
		if err := r.waitHeartbeat(ctx, "", ""); err != nil {
			return err
		}
		response, err := r.config.Client.Pull(ctx, workercontract.PullRequest{WorkerID: r.config.WorkerID})
		if err != nil {
			if protocolErr, ok := err.(*ProtocolError); ok && protocolErr.Retryable() {
				if err := r.config.Sleep(ctx, r.config.PollInterval); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("pull worker assignment: %w", err)
		}
		if response.Assignment == nil {
			if err := r.config.Sleep(ctx, r.config.PollInterval); err != nil {
				return err
			}
			continue
		}
		if err := r.executeAssignment(ctx, *response.Assignment); err != nil {
			return err
		}
	}
}

func (r *Runner) executeAssignment(ctx context.Context, assignment workercontract.Assignment) error {
	if assignment.Kind == workercontract.AssignmentKindVerify {
		return r.executeVerificationAssignment(ctx, assignment)
	}
	if assignment.ExecutionMode == "edit" {
		claimID, err := newRuntimeClaimID()
		if err != nil {
			return err
		}
		claim, claimErr := r.config.Client.Claim(ctx, workercontract.ClaimRequest{AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, RuntimeClaimID: claimID})
		if claimErr != nil {
			return fmt.Errorf("claim worker assignment: %w", claimErr)
		}
		if claim.Disposition != "claimed" && claim.Disposition != "duplicate" {
			return fmt.Errorf("claim worker assignment: unexpected disposition %q", claim.Disposition)
		}
	}
	adapter := r.config.Runtimes[assignment.Runtime]
	if adapter == nil {
		report := workercontract.Report{AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, Attempt: assignment.Attempt, Outcome: workercontract.Outcome("failed"), FailureReason: "runtime_unavailable", StartedAt: r.now().Format(time.RFC3339Nano), CompletedAt: r.now().Format(time.RFC3339Nano)}
		return r.reportWithRetry(ctx, report)
	}
	timeout := r.config.RuntimeTimeout
	if assignment.TimeoutSeconds > 0 {
		timeout = time.Duration(assignment.TimeoutSeconds) * time.Second
	}
	input := execution.ExecutionInput{Workspace: assignment.WorkspacePath, Instruction: assignment.Instruction, Runtime: assignment.Runtime, BeforeRevision: assignment.BeforeRevision, Timeout: timeout, ResultMode: assignment.ResultMode}
	if r.config.Environment != nil {
		input.Environment = execution.SanitizeEnvironment(r.config.Environment())
	}
	var result execution.RuntimeResult
	var runErr error
	result, runErr = r.runWithHeartbeat(ctx, assignment, func(runContext context.Context) (execution.RuntimeResult, error) {
		return adapter.Run(runContext, input)
	})
	if assignment.ResultMode == workercontract.ResultModePlanningCandidate {
		result = enforcePlanningSnapshotUnchanged(result)
		result = redactPlanningSnapshotPath(result, assignment.WorkspacePath)
	}
	report := ReportFromRuntime(assignment, result, runErr, r.now())
	return r.reportWithRetry(ctx, report)
}

func (r *Runner) executeVerificationAssignment(ctx context.Context, assignment workercontract.Assignment) error {
	if assignment.Verification == nil {
		return fmt.Errorf("verification assignment is missing its fixed input")
	}
	var report workercontract.VerificationReport
	var runErr error
	report, runErr = r.verifyWithHeartbeat(ctx, assignment, func(runContext context.Context) (workercontract.VerificationReport, error) {
		return r.config.Verifier.Verify(runContext, *assignment.Verification, assignment.WorkspacePath, r.config.EnvironmentValue())
	})
	if runErr != nil {
		return r.reportWithRetry(ctx, workercontract.Report{AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, Attempt: assignment.Attempt, Outcome: workercontract.Outcome("failed"), FailureReason: "verification_executor_failed", Verification: &report, StartedAt: r.now().Format(time.RFC3339Nano), CompletedAt: r.now().Format(time.RFC3339Nano)})
	}
	outcome := workercontract.Outcome("succeeded")
	if report.Outcome == workercontract.Outcome("failed") {
		outcome = workercontract.Outcome("failed")
	}
	return r.reportWithRetry(ctx, workercontract.Report{AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, Attempt: assignment.Attempt, Outcome: outcome, Verification: &report, StartedAt: r.now().Format(time.RFC3339Nano), CompletedAt: r.now().Format(time.RFC3339Nano)})
}

func (c RunnerConfig) EnvironmentValue() []string {
	if c.Environment == nil {
		return nil
	}
	return c.Environment()
}

func newRuntimeClaimID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create runtime claim id: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func enforcePlanningSnapshotUnchanged(result execution.RuntimeResult) execution.RuntimeResult {
	modified := len(result.ChangedFileList) != 0 || len(result.Diff.Content) != 0 || len(result.ChangedFiles.Content) != 0
	if modified && !containsString(result.GuardFindings, "planning_snapshot_modified") {
		result.GuardFindings = append(result.GuardFindings, "planning_snapshot_modified")
	}
	return result
}

func redactPlanningSnapshotPath(result execution.RuntimeResult, workspace string) execution.RuntimeResult {
	variants := snapshotPathVariants(workspace)
	if len(variants) == 0 {
		return result
	}
	disclosed := false
	for _, artifact := range []*execution.Artifact{&result.Stdout, &result.Stderr, &result.Diff, &result.ChangedFiles, &result.Candidate} {
		content, found := redactSnapshotPath(artifact.Content, variants)
		if !found {
			continue
		}
		disclosed = true
		captureErr := error(nil)
		if artifact.CaptureError != "" {
			captureErr = errors.New("capture failed")
		}
		*artifact = execution.NewArtifact(artifact.Kind, content, artifact.Truncated, captureErr)
	}
	for index, finding := range result.GuardFindings {
		redacted, found := redactSnapshotPath([]byte(finding), variants)
		if found {
			disclosed = true
			result.GuardFindings[index] = string(redacted)
		}
	}
	if disclosed && !containsString(result.GuardFindings, "snapshot_path_disclosure") {
		result.GuardFindings = append(result.GuardFindings, "snapshot_path_disclosure")
	}
	return result
}

func snapshotPathVariants(workspace string) [][]byte {
	clean := filepath.Clean(workspace)
	if clean == "." || !filepath.IsAbs(clean) || len(clean) < 2 {
		return nil
	}
	paths := []string{clean}
	parent := filepath.Dir(clean)
	if filepath.Base(clean) == "source" && strings.HasPrefix(filepath.Base(parent), "keystone-planning-snapshot-") {
		paths = append(paths, parent, filepath.Dir(parent))
	}
	values := make([]string, 0, len(paths)*4)
	for _, snapshotPath := range paths {
		pathValues := []string{snapshotPath, filepath.ToSlash(snapshotPath), strings.ReplaceAll(snapshotPath, "/", `\`)}
		values = append(values, pathValues...)
		for _, value := range pathValues {
			encoded, err := json.Marshal(value)
			if err == nil && len(encoded) >= 2 {
				values = append(values, string(encoded[1:len(encoded)-1]))
			}
		}
	}
	seen := make(map[string]struct{}, len(values))
	variants := make([][]byte, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		variants = append(variants, []byte(value))
	}
	sort.Slice(variants, func(left, right int) bool { return len(variants[left]) > len(variants[right]) })
	return variants
}

func redactSnapshotPath(content []byte, variants [][]byte) ([]byte, bool) {
	redacted := append([]byte(nil), content...)
	found := false
	for _, variant := range variants {
		if !bytes.Contains(redacted, variant) {
			continue
		}
		found = true
		redacted = bytes.ReplaceAll(redacted, variant, []byte("<snapshot>"))
	}
	return redacted, found
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (r *Runner) runWithHeartbeat(ctx context.Context, assignment workercontract.Assignment, run func(context.Context) (execution.RuntimeResult, error)) (execution.RuntimeResult, error) {
	resultCh := make(chan runtimeStopResult, 1)
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		value, err := run(runContext)
		resultCh <- runtimeStopResult{value: value, err: err}
	}()
	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case value := <-resultCh:
			return value.value, value.err
		case <-ticker.C:
			heartbeatErr := r.waitHeartbeat(ctx, assignment.AgentRunID, leaseTokenDigest(assignment.LeaseToken))
			if heartbeatErr == nil {
				continue
			}
			cancel()
			return r.waitRuntimeStop(resultCh, heartbeatErr)
		case <-ctx.Done():
			cancel()
			return r.waitRuntimeStop(resultCh, ctx.Err())
		}
	}
}

func (r *Runner) verifyWithHeartbeat(ctx context.Context, assignment workercontract.Assignment, verify func(context.Context) (workercontract.VerificationReport, error)) (workercontract.VerificationReport, error) {
	resultCh := make(chan verificationStopResult, 1)
	verifyContext, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		value, err := verify(verifyContext)
		resultCh <- verificationStopResult{value: value, err: err}
	}()
	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case value := <-resultCh:
			return value.value, value.err
		case <-ticker.C:
			heartbeatErr := r.waitHeartbeat(ctx, assignment.AgentRunID, leaseTokenDigest(assignment.LeaseToken))
			if heartbeatErr == nil {
				continue
			}
			cancel()
			return r.waitVerificationStop(resultCh, heartbeatErr)
		case <-ctx.Done():
			cancel()
			return r.waitVerificationStop(resultCh, ctx.Err())
		}
	}
}

func (r *Runner) waitVerificationStop(resultCh <-chan verificationStopResult, cause error) (workercontract.VerificationReport, error) {
	timer := time.NewTimer(r.config.RuntimeStopTimeout)
	defer timer.Stop()
	select {
	case value := <-resultCh:
		return value.value, errors.Join(cause, value.err)
	case <-timer.C:
		return workercontract.VerificationReport{}, errors.Join(cause, errors.New("verification shutdown timed out"))
	}
}

func (r *Runner) waitRuntimeStop(resultCh <-chan runtimeStopResult, cause error) (execution.RuntimeResult, error) {
	timer := time.NewTimer(r.config.RuntimeStopTimeout)
	defer timer.Stop()
	select {
	case value := <-resultCh:
		return value.value, errors.Join(cause, value.err)
	case <-timer.C:
		return execution.RuntimeResult{}, errors.Join(cause, errors.New("worker runtime shutdown timed out"))
	}
}

func (r *Runner) waitHeartbeat(ctx context.Context, runID, tokenDigest string) error {
	request := workercontract.Heartbeat{WorkerID: r.config.WorkerID, AgentRunID: runID, LeaseTokenSHA256: tokenDigest}
	response, err := r.config.Client.Heartbeat(ctx, request)
	if err != nil {
		if protocolErr, ok := err.(*ProtocolError); ok && protocolErr.Retryable() {
			return nil
		}
		return fmt.Errorf("heartbeat worker: %w", err)
	}
	if runID != "" && !response.LeaseRenewed {
		return errWorkerLeaseNotRenewed
	}
	return nil
}

func (r *Runner) reportWithRetry(ctx context.Context, report workercontract.Report) error {
	for {
		response, err := r.config.Client.Report(ctx, report)
		if err == nil {
			switch response.Disposition {
			case "accepted", "accepted_fenced", "candidate_received", "duplicate", "late":
				return nil
			case "terminal_conflict":
				return fmt.Errorf("report worker: terminal conflict")
			default:
				return fmt.Errorf("report worker: unexpected disposition %q", response.Disposition)
			}
		}
		protocolErr, ok := err.(*ProtocolError)
		if !ok || !protocolErr.Retryable() {
			return fmt.Errorf("report worker: %w", err)
		}
		if err := r.config.Sleep(ctx, r.config.PollInterval); err != nil {
			return err
		}
	}
}

// ReportFromRuntime 将 Runtime 观察值转换成不含生命周期命令的 Worker Report。
func ReportFromRuntime(assignment workercontract.Assignment, result execution.RuntimeResult, runErr error, now time.Time) workercontract.Report {
	outcome := workercontract.Outcome("failed")
	if runErr == nil && result.ExitCode != nil && *result.ExitCode == 0 && len(result.GuardFindings) == 0 && len(result.CaptureFailures) == 0 {
		outcome = workercontract.Outcome("succeeded")
	}
	failureReason := ""
	switch {
	case result.TimedOut:
		failureReason = "runtime_timeout"
	case len(result.CaptureFailures) > 0:
		failureReason = "capture_failed"
	case len(result.GuardFindings) > 0:
		failureReason = "guard_violation"
	case runErr != nil || outcome != workercontract.Outcome("succeeded"):
		failureReason = "runtime_failed"
	}
	report := workercontract.Report{
		AgentRunID:      assignment.AgentRunID,
		LeaseToken:      assignment.LeaseToken,
		Attempt:         assignment.Attempt,
		Outcome:         outcome,
		ExitCode:        result.ExitCode,
		StartedAt:       result.StartedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt:     result.CompletedAt.UTC().Format(time.RFC3339Nano),
		AfterRevision:   result.AfterRevision,
		FailureReason:   failureReason,
		GuardFindings:   append([]string(nil), result.GuardFindings...),
		CaptureFailures: make([]workercontract.CaptureFailure, 0, len(result.CaptureFailures)),
	}
	if report.StartedAt == "0001-01-01T00:00:00Z" || report.StartedAt == "0001-01-01T00:00:00.000000000Z" {
		report.StartedAt = now.UTC().Format(time.RFC3339Nano)
	}
	if report.CompletedAt == "0001-01-01T00:00:00Z" || report.CompletedAt == "0001-01-01T00:00:00.000000000Z" {
		report.CompletedAt = now.UTC().Format(time.RFC3339Nano)
	}
	for _, failure := range result.CaptureFailures {
		report.CaptureFailures = append(report.CaptureFailures, workercontract.CaptureFailure{Kind: string(failure.Kind), Stage: failure.Stage, Error: "capture_failed"})
	}
	for _, item := range []execution.Artifact{result.Stdout, result.Stderr, result.Diff, result.ChangedFiles, result.Candidate} {
		if item.SHA256 == "" && len(item.Content) == 0 && item.SizeBytes == 0 && item.CaptureError == "" {
			continue
		}
		captureError := ""
		if item.CaptureError != "" {
			captureError = "capture_failed"
		}
		report.Artifacts = append(report.Artifacts, workercontract.Artifact{Kind: string(item.Kind), ContentBase64: base64.StdEncoding.EncodeToString(item.Content), SHA256: item.SHA256, SizeBytes: item.SizeBytes, Truncated: item.Truncated, CaptureError: captureError})
	}
	return report
}

func leaseTokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (r *Runner) now() time.Time {
	if r.config.Now == nil {
		return time.Now().UTC()
	}
	return r.config.Now().UTC()
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

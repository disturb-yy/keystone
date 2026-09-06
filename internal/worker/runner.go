package worker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/execution"
)

const (
	defaultPollInterval     = time.Second
	defaultHeartbeatSeconds = 5 * time.Second
	defaultRuntimeTimeout   = 30 * time.Minute
)

// RunnerConfig 描述一个独立 Worker 的主动循环和可替换 seam。
type RunnerConfig struct {
	Client            *Client
	WorkerID          string
	Capabilities      []string
	Runtimes          map[string]execution.RuntimeAdapter
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	RuntimeTimeout    time.Duration
	Environment       func() []string
	Now               func() time.Time
	Sleep             func(context.Context, time.Duration) error
}

// Runner 执行 Register → Heartbeat/Pull → Runtime → Report 顺序。
type Runner struct {
	config RunnerConfig
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
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Sleep == nil {
		config.Sleep = sleepContext
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
	adapter := r.config.Runtimes[assignment.Runtime]
	if adapter == nil {
		report := workercontract.Report{AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, Attempt: assignment.Attempt, Outcome: workercontract.Outcome("failed"), FailureReason: "runtime_unavailable", StartedAt: r.now().Format(time.RFC3339Nano), CompletedAt: r.now().Format(time.RFC3339Nano)}
		return r.reportWithRetry(ctx, report)
	}
	input := execution.ExecutionInput{Workspace: assignment.WorkspacePath, Instruction: assignment.Instruction, Runtime: assignment.Runtime, BeforeRevision: assignment.BeforeRevision, Timeout: r.config.RuntimeTimeout}
	if r.config.Environment != nil {
		input.Environment = execution.SanitizeEnvironment(r.config.Environment())
	}
	var result execution.RuntimeResult
	var runErr error
	result, runErr = r.runWithHeartbeat(ctx, assignment, func(runContext context.Context) (execution.RuntimeResult, error) {
		return adapter.Run(runContext, input)
	})
	report := ReportFromRuntime(assignment, result, runErr, r.now())
	return r.reportWithRetry(ctx, report)
}

func (r *Runner) runWithHeartbeat(ctx context.Context, assignment workercontract.Assignment, run func(context.Context) (execution.RuntimeResult, error)) (execution.RuntimeResult, error) {
	type result struct {
		value execution.RuntimeResult
		err   error
	}
	resultCh := make(chan result, 1)
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		value, err := run(runContext)
		resultCh <- result{value: value, err: err}
	}()
	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case value := <-resultCh:
			return value.value, value.err
		case <-ticker.C:
			_ = r.waitHeartbeat(ctx, assignment.AgentRunID, leaseTokenDigest(assignment.LeaseToken))
		case <-ctx.Done():
			return execution.RuntimeResult{}, ctx.Err()
		}
	}
}

func (r *Runner) waitHeartbeat(ctx context.Context, runID, tokenDigest string) error {
	request := workercontract.Heartbeat{WorkerID: r.config.WorkerID, AgentRunID: runID, LeaseTokenSHA256: tokenDigest}
	if _, err := r.config.Client.Heartbeat(ctx, request); err != nil {
		if protocolErr, ok := err.(*ProtocolError); ok && protocolErr.Retryable() {
			return nil
		}
		return fmt.Errorf("heartbeat worker: %w", err)
	}
	return nil
}

func (r *Runner) reportWithRetry(ctx context.Context, report workercontract.Report) error {
	for {
		response, err := r.config.Client.Report(ctx, report)
		if err == nil {
			switch response.Disposition {
			case "accepted", "accepted_fenced", "duplicate", "late":
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
	for _, item := range []execution.Artifact{result.Stdout, result.Stderr, result.Diff, result.ChangedFiles} {
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

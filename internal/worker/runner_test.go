package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/execution"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type fakeRuntime struct {
	calls          int
	lastResultMode string
}

func (f *fakeRuntime) Name() string { return execution.RuntimeCodex }

func (f *fakeRuntime) Run(_ context.Context, input execution.ExecutionInput) (execution.RuntimeResult, error) {
	f.calls++
	f.lastResultMode = input.ResultMode
	if input.Workspace == "" || input.Instruction == "" {
		return execution.RuntimeResult{}, context.Canceled
	}
	started := time.Unix(10, 0).UTC()
	completed := started.Add(time.Second)
	exitCode := 0
	result := execution.RuntimeResult{StartedAt: started, CompletedAt: completed, ExitCode: &exitCode, AfterRevision: "after", Stdout: execution.NewArtifact(execution.ArtifactStdout, []byte("stdout"), false, nil), Stderr: execution.NewArtifact(execution.ArtifactStderr, []byte("stderr"), false, nil), Diff: execution.NewArtifact(execution.ArtifactDiff, []byte("diff"), false, nil), ChangedFiles: execution.NewArtifact(execution.ArtifactChangedFiles, []byte("fixture.go"), false, nil)}
	if input.ResultMode == execution.ResultModePlanningCandidate {
		result.Candidate = execution.NewArtifact(execution.ArtifactCandidate, []byte(`{"summary":"candidate"}`), false, nil)
		result.Diff = execution.NewArtifact(execution.ArtifactDiff, nil, false, nil)
		result.ChangedFiles = execution.NewArtifact(execution.ArtifactChangedFiles, nil, false, nil)
	}
	return result, nil
}

func TestRunnerRegistersExecutesAndRetriesSameReport(t *testing.T) {
	assignment := workercontract.Assignment{AgentRunID: "run-1", LeaseToken: "lease-1", WorkspacePath: "/tmp/workspace", Runtime: execution.RuntimeCodex, ResultMode: workercontract.ResultModePlanningCandidate, Instruction: "edit fixture", Attempt: 1}
	var routes []string
	var firstReport workercontract.Report
	reportCalls := 0
	pullCalls := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		routes = append(routes, request.URL.Path)
		body, _ := io.ReadAll(request.Body)
		var payload any
		_ = json.Unmarshal(body, &payload)
		status := http.StatusOK
		var response any
		switch request.URL.Path {
		case "/worker/v1/register":
			response = workercontract.RegisterResponse{WorkerID: "worker-1", ProtocolVersion: workercontract.ProtocolVersionV1, HeartbeatIntervalSecs: 5, LeaseTTLSeconds: 30, Capabilities: []string{"runtime:codex"}}
		case "/worker/v1/heartbeat":
			response = workercontract.HeartbeatResponse{WorkerID: "worker-1", WorkerAvailable: true}
		case "/worker/v1/pull":
			pullCalls++
			if pullCalls == 1 {
				response = workercontract.PullResponse{Assignment: &assignment}
			} else {
				response = workercontract.PullResponse{Assignment: nil}
			}
		case "/worker/v1/report":
			var report workercontract.Report
			if err := json.Unmarshal(body, &report); err != nil {
				t.Fatalf("decode report: %v", err)
			}
			if reportCalls == 0 {
				firstReport = report
				status = http.StatusServiceUnavailable
				response = workercontract.ErrorResponse{Code: "unavailable", Message: "retry"}
			} else {
				if !reflect.DeepEqual(firstReport, report) {
					t.Fatalf("retried report changed: first=%+v retry=%+v", firstReport, report)
				}
				response = workercontract.ReportResponse{Disposition: "accepted", AgentRunID: assignment.AgentRunID, LeaseState: "consumed"}
			}
			reportCalls++
		default:
			t.Fatalf("unexpected route %s", request.URL.Path)
		}
		encoded, _ := json.Marshal(response)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(encoded))}, nil
	})
	client := NewClient("http://daemon.invalid", "worker-secret", &http.Client{Transport: transport})
	runtime := &fakeRuntime{}
	sleepCalls := 0
	runner, err := NewRunner(RunnerConfig{
		Client: client, WorkerID: "worker-1", Capabilities: []string{"runtime:codex"},
		Runtimes:          map[string]execution.RuntimeAdapter{execution.RuntimeCodex: runtime},
		HeartbeatInterval: time.Hour, PollInterval: time.Millisecond,
		Sleep: func(_ context.Context, _ time.Duration) error {
			sleepCalls++
			if sleepCalls == 1 {
				return nil
			}
			return context.Canceled
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = runner.Run(context.Background())
	if err != context.Canceled {
		t.Fatalf("runner error = %v, want context.Canceled", err)
	}
	if runtime.calls != 1 || reportCalls != 2 {
		t.Fatalf("runtime calls = %d, report calls = %d, want 1 and 2", runtime.calls, reportCalls)
	}
	if len(routes) < 5 || routes[0] != "/worker/v1/register" || routes[1] != "/worker/v1/heartbeat" || routes[2] != "/worker/v1/pull" || routes[3] != "/worker/v1/report" || routes[4] != "/worker/v1/report" {
		t.Fatalf("worker route sequence = %v", routes)
	}
	if firstReport.Outcome != workercontract.Outcome("succeeded") || len(firstReport.Artifacts) != 5 || runtime.lastResultMode != execution.ResultModePlanningCandidate {
		t.Fatalf("first report = %+v", firstReport)
	}
}

func TestPlanningResultRejectsSnapshotChanges(t *testing.T) {
	result := execution.RuntimeResult{
		Diff:            execution.NewArtifact(execution.ArtifactDiff, []byte("diff --git a/file b/file"), false, nil),
		ChangedFiles:    execution.NewArtifact(execution.ArtifactChangedFiles, []byte("file"), false, nil),
		ChangedFileList: []string{"file"},
	}
	guarded := enforcePlanningSnapshotUnchanged(result)
	if !containsString(guarded.GuardFindings, "planning_snapshot_modified") {
		t.Fatalf("guard findings = %v, want planning snapshot modification", guarded.GuardFindings)
	}
	guarded = enforcePlanningSnapshotUnchanged(guarded)
	if got := len(guarded.GuardFindings); got != 1 {
		t.Fatalf("duplicate guard findings = %v", guarded.GuardFindings)
	}
}

func TestPlanningResultRedactsSnapshotPathAndFailsClosed(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "keystone-planning-snapshot-fixture", "source")
	snapshotBase := filepath.Dir(filepath.Dir(workspace))
	candidate, err := json.Marshal(map[string]string{"summary": workspace})
	if err != nil {
		t.Fatal(err)
	}
	result := execution.RuntimeResult{
		Stdout:    execution.NewArtifact(execution.ArtifactStdout, []byte("cwd="+workspace+" parent="+filepath.Dir(workspace)+" base="+snapshotBase), false, nil),
		Stderr:    execution.NewArtifact(execution.ArtifactStderr, []byte(""), false, nil),
		Candidate: execution.NewArtifact(execution.ArtifactCandidate, candidate, false, nil),
	}
	zero := 0
	result.ExitCode = &zero
	redacted := redactPlanningSnapshotPath(result, workspace)
	for _, artifact := range []execution.Artifact{redacted.Stdout, redacted.Stderr, redacted.Candidate} {
		for _, variant := range snapshotPathVariants(workspace) {
			if bytes.Contains(artifact.Content, variant) {
				t.Fatalf("artifact %s retained snapshot path variant %q", artifact.Kind, variant)
			}
		}
		if artifact.Kind != "" && artifact.Validate() != nil {
			t.Fatalf("artifact %s has stale identity after redaction", artifact.Kind)
		}
	}
	if !containsString(redacted.GuardFindings, "snapshot_path_disclosure") {
		t.Fatalf("guard findings = %v, want snapshot disclosure", redacted.GuardFindings)
	}
	report := ReportFromRuntime(workercontract.Assignment{AgentRunID: "run", LeaseToken: "lease", Attempt: 1}, redacted, nil, time.Now())
	if report.Outcome != workercontract.Outcome("failed") || report.FailureReason != "guard_violation" {
		t.Fatalf("report = %+v, want failed guard violation", report)
	}
}

func TestRunWithHeartbeatCancelsRuntimeWhenLeaseIsNotRenewed(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/worker/v1/heartbeat" {
			t.Fatalf("unexpected route %s", request.URL.Path)
		}
		encoded, _ := json.Marshal(workercontract.HeartbeatResponse{WorkerID: "worker-1", LeaseRenewed: false, WorkerAvailable: false})
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(encoded))}, nil
	})
	client := NewClient("http://daemon.invalid", "worker-secret", &http.Client{Transport: transport})
	runner, err := NewRunner(RunnerConfig{Client: client, WorkerID: "worker-1", HeartbeatInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	cancelled := make(chan struct{})
	_, err = runner.runWithHeartbeat(context.Background(), workercontract.Assignment{AgentRunID: "run-1", LeaseToken: "lease-1"}, func(ctx context.Context) (execution.RuntimeResult, error) {
		<-ctx.Done()
		close(cancelled)
		return execution.RuntimeResult{}, ctx.Err()
	})
	if !errors.Is(err, errWorkerLeaseNotRenewed) {
		t.Fatalf("runWithHeartbeat() error = %v, want lease-not-renewed", err)
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("runtime context was not cancelled")
	}
}

func TestClientAddsRequestDeadlineForUnboundedHTTPClient(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		if !ok {
			t.Fatal("worker request has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > defaultWorkerRequestTimeout+time.Second {
			t.Fatalf("worker request deadline remaining = %s", remaining)
		}
		body, _ := json.Marshal(workercontract.PullResponse{})
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	client := NewClient("http://daemon.invalid", "worker-secret", &http.Client{Transport: transport})
	if _, err := client.Pull(context.Background(), workercontract.PullRequest{WorkerID: "worker-1"}); err != nil {
		t.Fatal(err)
	}
}

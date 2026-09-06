package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/execution"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type fakeRuntime struct{ calls int }

func (f *fakeRuntime) Name() string { return execution.RuntimeCodex }

func (f *fakeRuntime) Run(_ context.Context, input execution.ExecutionInput) (execution.RuntimeResult, error) {
	f.calls++
	if input.Workspace == "" || input.Instruction == "" {
		return execution.RuntimeResult{}, context.Canceled
	}
	started := time.Unix(10, 0).UTC()
	completed := started.Add(time.Second)
	exitCode := 0
	return execution.RuntimeResult{StartedAt: started, CompletedAt: completed, ExitCode: &exitCode, AfterRevision: "after", Stdout: execution.NewArtifact(execution.ArtifactStdout, []byte("stdout"), false, nil), Stderr: execution.NewArtifact(execution.ArtifactStderr, []byte("stderr"), false, nil), Diff: execution.NewArtifact(execution.ArtifactDiff, []byte("diff"), false, nil), ChangedFiles: execution.NewArtifact(execution.ArtifactChangedFiles, []byte("fixture.go"), false, nil)}, nil
}

func TestRunnerRegistersExecutesAndRetriesSameReport(t *testing.T) {
	assignment := workercontract.Assignment{AgentRunID: "run-1", LeaseToken: "lease-1", WorkspacePath: "/tmp/workspace", Runtime: execution.RuntimeCodex, Instruction: "edit fixture", Attempt: 1}
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
	if firstReport.Outcome != workercontract.Outcome("succeeded") || len(firstReport.Artifacts) != 4 {
		t.Fatalf("first report = %+v", firstReport)
	}
}

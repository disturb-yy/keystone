package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
)

func TestCommandTreeAndDataDirResolvedOnce(t *testing.T) {
	paths := testPaths(t)
	server := httptest.NewServer(readyDaemonHandler("daemon-tree", paths.DatabasePath))
	t.Cleanup(server.Close)
	publishMetadata(t, paths, endpointForServer(t, server), "daemon-tree")

	var resolveCalls int
	var resolvedInput string
	deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
		t.Fatal("status must not start a daemon")
		return nil, nil
	})
	deps.ResolveDataDir = func(dataDir string) (localstate.Paths, error) {
		resolveCalls++
		resolvedInput = dataDir
		return paths, nil
	}
	command := NewRootCommand(deps)
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"daemon", "--data-dir", "relative/instance", "status"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resolveCalls != 1 {
		t.Fatalf("ResolveDataDir calls = %d, want 1", resolveCalls)
	}
	if resolvedInput != "relative/instance" {
		t.Fatalf("ResolveDataDir input = %q, want original CLI value", resolvedInput)
	}
	var status controlplane.DaemonStatusResponse
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatalf("decode status output: %v", err)
	}
	if status.DaemonInstanceID != "daemon-tree" {
		t.Fatalf("status instance ID = %q, want daemon-tree", status.DaemonInstanceID)
	}
}

func TestStatusAndStopNeverStartDaemon(t *testing.T) {
	paths := testPaths(t)
	server := httptest.NewServer(readyDaemonHandler("daemon-no-start", paths.DatabasePath))
	t.Cleanup(server.Close)
	publishMetadata(t, paths, endpointForServer(t, server), "daemon-no-start")

	var runnerCalls int
	runner := func(context.Context, string, ...string) (DaemonProcess, error) {
		runnerCalls++
		return nil, errors.New("unexpected daemon launch")
	}
	deps := testDependencies(paths, runner)
	if _, err := executeCLI(t, deps, "daemon", "status", "--data-dir", paths.Root); err != nil {
		t.Fatalf("status error = %v", err)
	}
	if _, err := executeCLI(t, deps, "daemon", "stop", "--data-dir", paths.Root); err != nil {
		t.Fatalf("stop error = %v", err)
	}
	if runnerCalls != 0 {
		t.Fatalf("daemon runner calls = %d, want 0", runnerCalls)
	}
}

func TestStatusMetadataAndEndpointFailures(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*testing.T, localstate.Paths) (cleanup func())
		wantCategory ErrorCategory
	}{
		{
			name: "metadata missing",
			setup: func(*testing.T, localstate.Paths) func() {
				return func() {}
			},
			wantCategory: ErrorMetadataMissing,
		},
		{
			name: "metadata corrupt",
			setup: func(t *testing.T, paths localstate.Paths) func() {
				if err := os.WriteFile(paths.MetadataPath, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
				return func() {}
			},
			wantCategory: ErrorMetadataInvalid,
		},
		{
			name: "endpoint unreachable",
			setup: func(t *testing.T, paths localstate.Paths) func() {
				server := httptest.NewServer(http.NotFoundHandler())
				endpoint := endpointForServer(t, server)
				server.Close()
				publishMetadata(t, paths, endpoint, "daemon-unreachable")
				return func() {}
			},
			wantCategory: ErrorEndpointUnreachable,
		},
		{
			name: "health not ready",
			setup: func(t *testing.T, paths localstate.Paths) func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/healthz" {
						t.Fatalf("unexpected request path %q", r.URL.Path)
					}
					writeContractJSON(w, http.StatusServiceUnavailable, controlplane.HealthResponse{Ready: false})
				}))
				publishMetadata(t, paths, endpointForServer(t, server), "daemon-not-ready")
				return server.Close
			},
			wantCategory: ErrorHealthNotReady,
		},
		{
			name: "status unavailable",
			setup: func(t *testing.T, paths localstate.Paths) func() {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/healthz":
						writeContractJSON(w, http.StatusOK, controlplane.HealthResponse{Ready: true})
					case "/v1/daemon/status":
						writeContractJSON(w, http.StatusServiceUnavailable, controlplane.ErrorEnvelope{Code: "unavailable", Message: "status unavailable"})
					default:
						http.NotFound(w, r)
					}
				}))
				publishMetadata(t, paths, endpointForServer(t, server), "daemon-status-error")
				return server.Close
			},
			wantCategory: ErrorStatusUnavailable,
		},
		{
			name: "instance mismatch",
			setup: func(t *testing.T, paths localstate.Paths) func() {
				server := httptest.NewServer(readyDaemonHandler("other-instance", paths.DatabasePath))
				publishMetadata(t, paths, endpointForServer(t, server), "metadata-instance")
				return server.Close
			},
			wantCategory: ErrorInstanceMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			cleanup := tt.setup(t, paths)
			t.Cleanup(cleanup)
			deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
				t.Fatal("status must not start a daemon")
				return nil, nil
			})
			_, err := executeCLI(t, deps, "daemon", "--data-dir", paths.Root, "status")
			assertCLIErrorCategory(t, err, tt.wantCategory)
		})
	}
}

func TestStartReusesHealthyInstance(t *testing.T) {
	paths := testPaths(t)
	server := httptest.NewServer(readyDaemonHandler("daemon-reused", paths.DatabasePath))
	t.Cleanup(server.Close)
	publishMetadata(t, paths, endpointForServer(t, server), "daemon-reused")

	var runnerCalls int
	deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
		runnerCalls++
		return nil, errors.New("healthy instance should be reused")
	})
	output, err := executeCLI(t, deps, "daemon", "--data-dir", paths.Root, "start")
	if err != nil {
		t.Fatalf("start reuse error = %v", err)
	}
	if runnerCalls != 0 {
		t.Fatalf("daemon runner calls = %d, want 0", runnerCalls)
	}
	var status controlplane.DaemonStatusResponse
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("decode start output: %v", err)
	}
	if status.DaemonInstanceID != "daemon-reused" || !status.DaemonReadiness {
		t.Fatalf("start output = %+v, want reused ready status", status)
	}
}

func TestStartWaitsForHealthAndMatchingStatus(t *testing.T) {
	paths := testPaths(t)
	const instanceID = "daemon-started"
	var healthCalls atomic.Int32
	var statusCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			ready := healthCalls.Add(1) >= 2
			if ready {
				writeContractJSON(w, http.StatusOK, controlplane.HealthResponse{Ready: true})
				return
			}
			writeContractJSON(w, http.StatusServiceUnavailable, controlplane.HealthResponse{Ready: false})
		case "/v1/daemon/status":
			id := "different-instance"
			if statusCalls.Add(1) >= 2 {
				id = instanceID
			}
			writeContractJSON(w, http.StatusOK, controlplane.DaemonStatusResponse{
				DatabasePath:           paths.DatabasePath,
				SchemaMigrationVersion: 1,
				DaemonReadiness:        true,
				DaemonInstanceID:       id,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	process := newFakeProcess()
	t.Cleanup(func() { process.finish(nil) })
	deps := testDependencies(paths, func(ctx context.Context, executable string, args ...string) (DaemonProcess, error) {
		if executable != "injected-daemon" {
			t.Fatalf("executable = %q, want injected-daemon", executable)
		}
		if len(args) != 2 || args[0] != "--data-dir" || args[1] != paths.Root {
			t.Fatalf("daemon args = %v, want --data-dir %q", args, paths.Root)
		}
		if err := publishTestMetadata(paths, endpointForServer(t, server), instanceID); err != nil {
			t.Fatalf("publish startup metadata: %v", err)
		}
		if ctx == nil {
			t.Fatal("runner context is nil")
		}
		return process, nil
	})
	deps.StartTimeout = time.Second
	deps.PollInterval = 5 * time.Millisecond
	output, err := executeCLI(t, deps, "daemon", "--data-dir", paths.Root, "start")
	if err != nil {
		t.Fatalf("start readiness error = %v", err)
	}
	if healthCalls.Load() < 3 || statusCalls.Load() < 2 {
		t.Fatalf("health calls = %d, status calls = %d, want repeated health and matching status checks", healthCalls.Load(), statusCalls.Load())
	}
	var status controlplane.DaemonStatusResponse
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("decode start readiness output: %v", err)
	}
	if status.DaemonInstanceID != instanceID || !status.DaemonReadiness {
		t.Fatalf("start readiness output = %+v, want same ready instance", status)
	}
}

func TestStartTimeoutAndSubprocessFailure(t *testing.T) {
	t.Run("readiness timeout", func(t *testing.T) {
		paths := testPaths(t)
		process := newFakeProcess()
		t.Cleanup(func() { process.finish(nil) })
		deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
			return process, nil
		})
		deps.StartTimeout = 30 * time.Millisecond
		deps.PollInterval = 5 * time.Millisecond
		_, err := executeCLI(t, deps, "daemon", "--data-dir", paths.Root, "start")
		assertCLIErrorCategory(t, err, ErrorDaemonStartTimeout)
	})

	t.Run("subprocess failure", func(t *testing.T) {
		paths := testPaths(t)
		deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
			return nil, errors.New("exit status 1")
		})
		_, err := executeCLI(t, deps, "daemon", "start", "--data-dir", paths.Root)
		assertCLIErrorCategory(t, err, ErrorDaemonStartFailed)
	})
}

func TestStartPreservesUnverifiableMetadata(t *testing.T) {
	paths := testPaths(t)
	server := httptest.NewServer(http.NotFoundHandler())
	endpoint := endpointForServer(t, server)
	server.Close()
	publishMetadata(t, paths, endpoint, "stale-instance")
	metadataBefore, err := os.ReadFile(paths.MetadataPath)
	if err != nil {
		t.Fatal(err)
	}
	deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
		return nil, errors.New("same-root daemon refused lock")
	})
	_, err = executeCLI(t, deps, "daemon", "--data-dir", paths.Root, "start")
	assertCLIErrorCategory(t, err, ErrorDaemonStartFailed)
	metadataAfter, err := os.ReadFile(paths.MetadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(metadataBefore, metadataAfter) {
		t.Fatalf("start changed unverifiable metadata: before=%s after=%s", metadataBefore, metadataAfter)
	}
}

func TestStopRequestResponseAndErrorEnvelope(t *testing.T) {
	paths := testPaths(t)
	const instanceID = "daemon-stop"
	var request controlplane.DaemonStopRequest
	var requestContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/daemon/stop" || r.Method != http.MethodPost {
			t.Fatalf("stop request = %s %s", r.Method, r.URL.Path)
		}
		requestContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode stop request: %v", err)
		}
		writeContractJSON(w, http.StatusOK, controlplane.DaemonStopResponse{Accepted: true, DaemonInstanceID: instanceID})
	}))
	t.Cleanup(server.Close)
	publishMetadata(t, paths, endpointForServer(t, server), instanceID)
	deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
		t.Fatal("stop must not start a daemon")
		return nil, nil
	})
	output, err := executeCLI(t, deps, "daemon", "stop", "--data-dir", paths.Root)
	if err != nil {
		t.Fatalf("stop success error = %v", err)
	}
	if request.DaemonInstanceID != instanceID || requestContentType != "application/json" {
		t.Fatalf("stop request = %+v Content-Type=%q", request, requestContentType)
	}
	var response controlplane.DaemonStopResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode stop output: %v", err)
	}
	if !response.Accepted || response.DaemonInstanceID != instanceID {
		t.Fatalf("stop output = %+v, want accepted response", response)
	}

	t.Run("error envelope instance mismatch", func(t *testing.T) {
		errorPaths := testPaths(t)
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeContractJSON(w, http.StatusConflict, controlplane.ErrorEnvelope{Code: "instance_mismatch", Message: "daemon instance id does not match"})
		}))
		t.Cleanup(errorServer.Close)
		publishMetadata(t, errorPaths, endpointForServer(t, errorServer), "old-instance")
		deps := testDependencies(errorPaths, nil)
		_, err := executeCLI(t, deps, "daemon", "--data-dir", errorPaths.Root, "stop")
		assertCLIErrorCategory(t, err, ErrorInstanceMismatch)
	})

	t.Run("accepted response instance mismatch", func(t *testing.T) {
		mismatchPaths := testPaths(t)
		mismatchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeContractJSON(w, http.StatusOK, controlplane.DaemonStopResponse{Accepted: true, DaemonInstanceID: "new-instance"})
		}))
		t.Cleanup(mismatchServer.Close)
		publishMetadata(t, mismatchPaths, endpointForServer(t, mismatchServer), "old-instance")
		deps := testDependencies(mismatchPaths, nil)
		_, err := executeCLI(t, deps, "daemon", "stop", "--data-dir", mismatchPaths.Root)
		assertCLIErrorCategory(t, err, ErrorInstanceMismatch)
	})
}

func TestDifferentRootsAreIsolated(t *testing.T) {
	firstPaths := testPaths(t)
	secondPaths := testPaths(t)
	firstServer := httptest.NewServer(readyDaemonHandler("first-root", firstPaths.DatabasePath))
	secondServer := httptest.NewServer(readyDaemonHandler("second-root", secondPaths.DatabasePath))
	t.Cleanup(firstServer.Close)
	t.Cleanup(secondServer.Close)
	publishMetadata(t, firstPaths, endpointForServer(t, firstServer), "first-root")
	publishMetadata(t, secondPaths, endpointForServer(t, secondServer), "second-root")

	firstOutput, err := executeCLI(t, testDependencies(firstPaths, nil), "daemon", "--data-dir", firstPaths.Root, "status")
	if err != nil {
		t.Fatalf("first root status error = %v", err)
	}
	secondOutput, err := executeCLI(t, testDependencies(secondPaths, nil), "daemon", "--data-dir", secondPaths.Root, "status")
	if err != nil {
		t.Fatalf("second root status error = %v", err)
	}
	var firstStatus, secondStatus controlplane.DaemonStatusResponse
	if err := json.Unmarshal([]byte(firstOutput), &firstStatus); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(secondOutput), &secondStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus.DaemonInstanceID != "first-root" || secondStatus.DaemonInstanceID != "second-root" {
		t.Fatalf("root statuses = (%+v, %+v), want isolated IDs", firstStatus, secondStatus)
	}
}

func TestDiscoverDaemonExecutable(t *testing.T) {
	currentExecutable := filepath.Join(t.TempDir(), "bin", "keystone")
	expectedDaemonExecutable := filepath.Join(filepath.Dir(currentExecutable), "keystone-daemon")
	deps := DefaultDependencies()
	deps.CurrentExecutable = func() (string, error) { return currentExecutable, nil }
	var lookedUp []string
	deps.LookPath = func(name string) (string, error) {
		lookedUp = append(lookedUp, name)
		if name == expectedDaemonExecutable {
			return name, nil
		}
		return "", errors.New("not found")
	}
	path, err := discoverDaemonExecutable(deps)
	if err != nil || path != expectedDaemonExecutable {
		t.Fatalf("same-directory executable = (%q, %v)", path, err)
	}
	if len(lookedUp) != 1 || lookedUp[0] != expectedDaemonExecutable {
		t.Fatalf("LookPath calls = %v, want same-directory candidate only", lookedUp)
	}

	deps.CurrentExecutable = func() (string, error) { return "", errors.New("executable unknown") }
	deps.LookPath = func(name string) (string, error) {
		if name == "keystone-daemon" {
			return "/path/keystone-daemon", nil
		}
		return "", errors.New("not found")
	}
	path, err = discoverDaemonExecutable(deps)
	if err != nil || path != "/path/keystone-daemon" {
		t.Fatalf("PATH executable = (%q, %v)", path, err)
	}

	deps.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	_, err = discoverDaemonExecutable(deps)
	assertCLIErrorCategory(t, err, ErrorDaemonExecutable)
}

func TestRealCLIDaemonLifecycle(t *testing.T) {
	daemonExecutable := buildDaemonForTest(t)
	root := t.TempDir()
	paths, err := localstate.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	deps := DefaultDependencies()
	deps.DaemonExecutablePath = daemonExecutable
	deps.StartTimeout = 5 * time.Second
	deps.PollInterval = 10 * time.Millisecond

	startOutput, err := executeCLI(t, deps, "daemon", "--data-dir", root, "start")
	if err != nil {
		t.Fatalf("real start error = %v", err)
	}
	var startedStatus controlplane.DaemonStatusResponse
	if err := json.Unmarshal([]byte(startOutput), &startedStatus); err != nil {
		t.Fatalf("decode real start output: %v", err)
	}
	if !startedStatus.DaemonReadiness || startedStatus.SchemaMigrationVersion != 2 {
		t.Fatalf("real start status = %+v, want ready migration version 2", startedStatus)
	}
	if _, err := os.Stat(startedStatus.DatabasePath); err != nil {
		t.Fatalf("real database stat error = %v", err)
	}

	statusOutput, err := executeCLI(t, deps, "daemon", "status", "--data-dir", root)
	if err != nil {
		t.Fatalf("real status error = %v", err)
	}
	var status controlplane.DaemonStatusResponse
	if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
		t.Fatalf("decode real status output: %v", err)
	}
	if status.DaemonInstanceID != startedStatus.DaemonInstanceID || status.DatabasePath != paths.DatabasePath {
		t.Fatalf("real status = %+v, want same instance and database path %q", status, paths.DatabasePath)
	}

	stopOutput, err := executeCLI(t, deps, "daemon", "stop", "--data-dir", root)
	if err != nil {
		t.Fatalf("real stop error = %v", err)
	}
	var stopResponse controlplane.DaemonStopResponse
	if err := json.Unmarshal([]byte(stopOutput), &stopResponse); err != nil {
		t.Fatalf("decode real stop output: %v", err)
	}
	if !stopResponse.Accepted || stopResponse.DaemonInstanceID != startedStatus.DaemonInstanceID {
		t.Fatalf("real stop response = %+v, want accepted same instance", stopResponse)
	}
	waitForMetadataRemoval(t, paths.MetadataPath)

	_, err = executeCLI(t, deps, "daemon", "status", "--data-dir", root)
	assertCLIErrorCategory(t, err, ErrorMetadataMissing)
}

type fakeProcess struct {
	done chan error
	once sync.Once
}

func newFakeProcess() *fakeProcess {
	return &fakeProcess{done: make(chan error, 1)}
}

func (p *fakeProcess) Wait() error {
	err, ok := <-p.done
	if !ok {
		return nil
	}
	return err
}

func (p *fakeProcess) finish(err error) {
	p.once.Do(func() {
		p.done <- err
		close(p.done)
	})
}

func testPaths(t *testing.T) localstate.Paths {
	t.Helper()
	paths, err := localstate.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Initialize(); err != nil {
		t.Fatal(err)
	}
	return paths
}

func publishMetadata(t *testing.T, paths localstate.Paths, endpoint, instanceID string) {
	t.Helper()
	if err := publishTestMetadata(paths, endpoint, instanceID); err != nil {
		t.Fatal(err)
	}
}

func publishTestMetadata(paths localstate.Paths, endpoint, instanceID string) error {
	return localstate.PublishMetadata(paths, localstate.Metadata{
		PID:        12345,
		Endpoint:   endpoint,
		InstanceID: instanceID,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func endpointForServer(t *testing.T, server *httptest.Server) string {
	t.Helper()
	endpoint := strings.TrimPrefix(server.URL, "http://")
	if err := validateDaemonEndpoint(endpoint); err != nil {
		t.Fatalf("test server endpoint %q is not valid: %v", endpoint, err)
	}
	return endpoint
}

func readyDaemonHandler(instanceID, databasePath string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeContractJSON(w, http.StatusOK, controlplane.HealthResponse{Ready: true})
	})
	mux.HandleFunc("/v1/daemon/status", func(w http.ResponseWriter, _ *http.Request) {
		writeContractJSON(w, http.StatusOK, controlplane.DaemonStatusResponse{
			DatabasePath:           databasePath,
			SchemaMigrationVersion: 1,
			DaemonReadiness:        true,
			DaemonInstanceID:       instanceID,
		})
	})
	mux.HandleFunc("/v1/daemon/stop", func(w http.ResponseWriter, _ *http.Request) {
		writeContractJSON(w, http.StatusOK, controlplane.DaemonStopResponse{Accepted: true, DaemonInstanceID: instanceID})
	})
	return mux
}

func writeContractJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func testDependencies(paths localstate.Paths, runner CommandRunner) Dependencies {
	if runner == nil {
		runner = func(context.Context, string, ...string) (DaemonProcess, error) {
			return nil, errors.New("unexpected daemon launch")
		}
	}
	return Dependencies{
		ResolveDataDir: func(string) (localstate.Paths, error) {
			return paths, nil
		},
		CommandRunner:        runner,
		DaemonExecutablePath: "injected-daemon",
		HTTPTimeout:          200 * time.Millisecond,
		StartTimeout:         500 * time.Millisecond,
		PollInterval:         5 * time.Millisecond,
		Clock:                time.Now,
	}
}

func executeCLI(t *testing.T, deps Dependencies, args ...string) (string, error) {
	t.Helper()
	command := NewRootCommand(deps)
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(new(bytes.Buffer))
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func assertCLIErrorCategory(t *testing.T, err error, want ErrorCategory) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want category %s", want)
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error = %v, want *CLIError", err)
	}
	if cliErr.Category != want {
		t.Fatalf("error category = %s, want %s; error=%v", cliErr.Category, want, err)
	}
}

func buildDaemonForTest(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	executableName := "keystone-daemon"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	executable := filepath.Join(t.TempDir(), executableName)
	build := exec.Command("go", "build", "-o", executable, "./cmd/keystone-daemon")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOCACHE="+t.TempDir())
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build keystone-daemon: %v\n%s", err, output)
	}
	return executable
}

func waitForMetadataRemoval(t *testing.T, metadataPath string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("metadata %q was not removed", metadataPath)
}

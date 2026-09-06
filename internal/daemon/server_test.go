package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
)

func TestServerBootingHealthAndReadyMetadata(t *testing.T) {
	root := t.TempDir()
	booting := make(chan struct{}, 1)
	release := make(chan struct{})
	server := New(root, Options{
		OnBooting: func(*Server) { booting <- struct{}{} },
		BootingGate: func(ctx context.Context) error {
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	runErr := runServer(server, ctx)

	select {
	case <-booting:
	case err := <-runErr:
		t.Fatalf("daemon exited before booting: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not enter booting")
	}

	paths, err := localstate.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	status, health := getHealth(t, server)
	if status != http.StatusServiceUnavailable || health.Ready {
		t.Fatalf("booting health = (%d, %+v), want (503, false)", status, health)
	}
	if _, err := os.Stat(paths.MetadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata during booting stat error = %v, want not exist", err)
	}

	close(release)
	waitForReady(t, server, runErr)
	if _, err := os.Stat(paths.MetadataPath); err != nil {
		t.Fatalf("ready metadata stat error = %v", err)
	}

	cancel()
	waitRun(t, runErr)
	if _, err := os.Stat(paths.MetadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata after shutdown stat error = %v, want not exist", err)
	}
	lock, err := localstate.Acquire(paths)
	if err != nil {
		t.Fatalf("Acquire() after shutdown error = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() after shutdown error = %v", err)
	}
}

func TestServerStatusIncludesProjectBootstrapSchema(t *testing.T) {
	root := t.TempDir()
	server, runErr, cancel := startReadyServer(t, root)
	defer stopServer(t, server, runErr, cancel)

	response, status := getStatus(t, server)
	if status != http.StatusOK {
		t.Fatalf("status code = %d, want 200", status)
	}
	paths, err := localstate.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if response.DatabasePath != paths.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", response.DatabasePath, paths.DatabasePath)
	}
	if response.SchemaMigrationVersion != 3 || !response.DaemonReadiness {
		t.Fatalf("status = %+v, want migration version 3 and ready", response)
	}
	if response.DaemonInstanceID != server.InstanceID() || response.DaemonInstanceID == "" {
		t.Fatalf("DaemonInstanceID = %q, want current non-empty ID %q", response.DaemonInstanceID, server.InstanceID())
	}

	db, err := sql.Open("sqlite", paths.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close inspection database: %v", err)
		}
	}()
	rows, err := db.Query(`
SELECT name
FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantTables := []string{"t_agent_run_artifacts", "t_agent_runs", "t_artifact_refs", "t_artifacts", "t_change_command_receipts", "t_changes", "t_event_artifacts", "t_human_decisions", "t_project_events", "t_project_initialization_intents", "t_project_initialization_receipts", "t_projects", "t_schema_migrations"}
	if len(tables) != len(wantTables) {
		t.Fatalf("SQLite tables = %v, want %v", tables, wantTables)
	}
	for index, table := range tables {
		if table != wantTables[index] {
			t.Fatalf("SQLite tables = %v, want %v", tables, wantTables)
		}
	}
}

func TestServerStatusBootingIsUnavailable(t *testing.T) {
	booting := make(chan struct{}, 1)
	release := make(chan struct{})
	server := New(t.TempDir(), Options{
		OnBooting: func(*Server) { booting <- struct{}{} },
		BootingGate: func(ctx context.Context) error {
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	runErr := runServer(server, ctx)
	defer func() {
		cancel()
		waitRun(t, runErr)
	}()
	select {
	case <-booting:
	case err := <-runErr:
		t.Fatalf("daemon exited before booting: %v", err)
	}

	_, status := getStatus(t, server)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("booting status code = %d, want 503", status)
	}
}

func TestServerStopValidationMismatchAndRepeat(t *testing.T) {
	server, runErr, cancel := startReadyServer(t, t.TempDir())

	response, status, envelope := postStop(t, server, "not-json")
	if status != http.StatusBadRequest || response.Accepted || envelope.Code != "invalid_request" {
		t.Fatalf("invalid stop = (%+v, %d, %+v), want invalid_request 400", response, status, envelope)
	}
	response, status, envelope = postStop(t, server, `{}`)
	if status != http.StatusBadRequest || response.Accepted || envelope.Code != "invalid_request" {
		t.Fatalf("empty stop = (%+v, %d, %+v), want invalid_request 400", response, status, envelope)
	}
	response, status, envelope = postStop(t, server, `{"daemon_instance_id":"other"}`)
	if status != http.StatusConflict || response.Accepted || envelope.Code != "instance_mismatch" {
		t.Fatalf("mismatch stop = (%+v, %d, %+v), want instance_mismatch 409", response, status, envelope)
	}

	payload, err := json.Marshal(controlplane.DaemonStopRequest{DaemonInstanceID: server.InstanceID()})
	if err != nil {
		t.Fatal(err)
	}
	first, firstStatus, firstEnvelope := postStop(t, server, string(payload))
	if firstStatus != http.StatusOK || !first.Accepted || firstEnvelope.Code != "" {
		t.Fatalf("first stop = (%+v, %d, %+v), want accepted 200", first, firstStatus, firstEnvelope)
	}
	second, secondStatus, secondEnvelope := postStop(t, server, string(payload))
	if secondStatus != http.StatusOK || !second.Accepted || second.DaemonInstanceID != first.DaemonInstanceID || secondEnvelope.Code != "" {
		t.Fatalf("repeat stop = (%+v, %d, %+v), want accepted idempotent response", second, secondStatus, secondEnvelope)
	}
	waitRun(t, runErr)
	cancel()
}

func TestServerDatabaseDegradationKeepsStopAvailable(t *testing.T) {
	root := t.TempDir()
	server := New(root, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	runErr := runServer(server, ctx)
	waitForReady(t, server, runErr)

	server.mu.RLock()
	db := server.db
	server.mu.RUnlock()
	if db == nil {
		t.Fatal("server database is nil")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database for degradation: %v", err)
	}

	status, health := getHealth(t, server)
	if status != http.StatusServiceUnavailable || health.Ready {
		t.Fatalf("degraded health = (%d, %+v), want (503, false)", status, health)
	}
	_, status = getStatus(t, server)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("degraded status code = %d, want 503", status)
	}
	_, status, envelope := postStop(t, server, `{"daemon_instance_id":"`+server.InstanceID()+`"}`)
	if status != http.StatusOK || envelope.Code != "" {
		t.Fatalf("stop after DB degradation = (%d, %+v), want accepted 200", status, envelope)
	}
	waitRun(t, runErr)
	cancel()
}

func TestServerStartupFailureReleasesLock(t *testing.T) {
	root := t.TempDir()
	paths, err := localstate.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(paths.DatabasePath, 0700); err != nil {
		t.Fatal(err)
	}

	err = Run(context.Background(), root, Options{})
	if err == nil {
		t.Fatal("Run() error = nil, want database startup failure")
	}
	lock, err := localstate.Acquire(paths)
	if err != nil {
		t.Fatalf("Acquire() after startup failure error = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() after startup failure error = %v", err)
	}
	if _, err := os.Stat(paths.MetadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata after startup failure stat error = %v, want not exist", err)
	}
}

func TestDifferentRootsRunIndependently(t *testing.T) {
	first, firstErr, firstCancel := startReadyServer(t, t.TempDir())
	second, secondErr, secondCancel := startReadyServer(t, t.TempDir())
	defer stopServer(t, first, firstErr, firstCancel)
	defer stopServer(t, second, secondErr, secondCancel)

	if first.Addr() == second.Addr() || first.InstanceID() == second.InstanceID() {
		t.Fatalf("independent servers share endpoint or ID: first=(%q,%q), second=(%q,%q)", first.Addr(), first.InstanceID(), second.Addr(), second.InstanceID())
	}
}

func startReadyServer(t *testing.T, root string) (*Server, <-chan error, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	server := New(root, Options{})
	runErr := runServer(server, ctx)
	waitForReady(t, server, runErr)
	return server, runErr, cancel
}

func runServer(server *Server, ctx context.Context) <-chan error {
	runErr := make(chan error, 1)
	go func() {
		runErr <- server.Run(ctx)
	}()
	return runErr
}

func waitForReady(t *testing.T, server *Server, runErr <-chan error) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-runErr:
			t.Fatalf("daemon exited before ready: %v", err)
		default:
		}
		if server.Addr() != "" {
			status, health := getHealth(t, server)
			if status == http.StatusOK && health.Ready {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("daemon did not become ready")
}

func getHealth(t *testing.T, server *Server) (int, controlplane.HealthResponse) {
	t.Helper()
	client := http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get("http://" + server.Addr() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz error = %v", err)
	}
	defer response.Body.Close()
	var health controlplane.HealthResponse
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	return response.StatusCode, health
}

func getStatus(t *testing.T, server *Server) (controlplane.DaemonStatusResponse, int) {
	t.Helper()
	client := http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get("http://" + server.Addr() + "/v1/daemon/status")
	if err != nil {
		t.Fatalf("GET /v1/daemon/status error = %v", err)
	}
	defer response.Body.Close()
	var status controlplane.DaemonStatusResponse
	if response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
			t.Fatalf("decode status response: %v", err)
		}
	}
	return status, response.StatusCode
}

func postStop(t *testing.T, server *Server, body string) (controlplane.DaemonStopResponse, int, controlplane.ErrorEnvelope) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://"+server.Addr()+"/v1/daemon/stop", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 500 * time.Millisecond}).Do(request)
	if err != nil {
		t.Fatalf("POST /v1/daemon/stop error = %v", err)
	}
	defer response.Body.Close()
	var stopResponse controlplane.DaemonStopResponse
	var envelope controlplane.ErrorEnvelope
	if response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&stopResponse); err != nil {
			t.Fatalf("decode stop response: %v", err)
		}
	} else if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode stop error response: %v", err)
	}
	return stopResponse, response.StatusCode, envelope
}

func stopServer(t *testing.T, server *Server, runErr <-chan error, cancel context.CancelFunc) {
	t.Helper()
	cancel()
	waitRun(t, runErr)
}

func waitRun(t *testing.T, runErr <-chan error) {
	t.Helper()
	select {
	case err := <-runErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("daemon Run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

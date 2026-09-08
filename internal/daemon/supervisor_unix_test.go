//go:build !windows

package daemon

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestWorkerSupervisorWatchdogStopsProcessTreeBeforeReconcile(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "term.marker")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := `
trap 'printf stopped > "$1"; exit 0' TERM
sh -c 'trap "" TERM; while :; do sleep 1; done' &
printf '%s' "$!" > "$2"
while :; do sleep 1; done
`
	authority := &watchdogSupervisorAuthority{}
	supervisor := NewWorkerSupervisor(WorkerSupervisorOptions{
		Store: authority, Endpoint: "127.0.0.1:1", ShutdownTimeout: 300 * time.Millisecond,
		WatchInterval: 10 * time.Millisecond,
		Command: func(_ context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.Command("sh", "-c", script, "worker-fixture", marker, pidFile)
		},
	})
	err := supervisor.runProcess(context.Background(), "ignored", "worker-1", "secret")
	if err == nil || !errors.Is(err, errWorkerHeartbeatExpired) {
		t.Fatalf("runProcess() error = %v, want heartbeat expiry", err)
	}
	if _, err := waitFile(marker); err != nil {
		t.Fatal(err)
	}
	childPID, err := readPID(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	waitGone(t, childPID)
}

type watchdogSupervisorAuthority struct{}

func (*watchdogSupervisorAuthority) PrepareWorker(context.Context, string, string) error { return nil }
func (*watchdogSupervisorAuthority) WorkerProcessLost(context.Context, string) (bool, error) {
	return true, nil
}
func (*watchdogSupervisorAuthority) ReconcileWorkerLost(context.Context, string) error { return nil }

func waitFile(path string) ([]byte, error) {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if content, err := os.ReadFile(path); err == nil {
			return content, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, errors.New("timed out waiting for process marker")
}

func readPID(path string) (int, error) {
	content, err := waitFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(content))
	if err != nil || pid <= 0 {
		return 0, errors.New("invalid child pid")
	}
	return pid, nil
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH || err == syscall.ECHILD {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("process %d is still alive", pid)
}

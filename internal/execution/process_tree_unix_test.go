//go:build !windows

package execution

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestProcessTreeNaturalExitCleansDescendants(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	command := exec.Command("sh", "-c", `
sh -c 'trap "" TERM; while :; do sleep 1; done' &
printf '%s' "$!" > "$1"
exit 0
`, "process-tree-fixture", pidFile)
	tree, err := StartProcessTree(command)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Wait(); err != nil {
		t.Fatal(err)
	}
	childPID := readProcessTreePID(t, pidFile)
	waitProcessGone(t, childPID)
}

func TestTerminateProcessTreeKillsDescendantsAfterGracePeriod(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	command := exec.Command("sh", "-c", `
sh -c 'trap "" TERM; while :; do sleep 1; done' &
printf '%s' "$!" > "$1"
trap 'exit 0' TERM
while :; do sleep 1; done
`, "process-tree-fixture", pidFile)
	tree, err := StartProcessTree(command)
	if err != nil {
		t.Fatal(err)
	}
	childPID := waitProcessTreePID(t, pidFile)
	if err := TerminateProcessTree(tree, 300*time.Millisecond); err != nil {
		t.Fatalf("terminate process tree: %v", err)
	}
	waitProcessGone(t, childPID)
}

func readProcessTreePID(t *testing.T, path string) int {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(value))
	if err != nil || pid <= 0 {
		t.Fatalf("child pid = %q, err=%v", value, err)
	}
	return pid
}

func waitProcessTreePID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return readProcessTreePID(t, path)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("child pid file %q was not created", path)
	return 0
}

func waitProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if err != nil && (err == syscall.ESRCH || err == syscall.ECHILD) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("process %d is still alive", pid)
}

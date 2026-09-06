package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/internal/execution"
)

func TestAdapterRunsFakeCodexWithFixedCommandAndEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX executable")
	}
	workspace := initGitWorkspace(t)
	binary := filepath.Join(t.TempDir(), "fake-codex")
	script := "#!/bin/sh\ncat >/dev/null\nprintf 'stdout-fact'\nprintf 'stderr-fact' >&2\nprintf 'changed' > changed.txt\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter := New(binary)
	adapter.Guard.GitBinary = mustLookPath(t, "git")
	result, err := adapter.Run(context.Background(), execution.ExecutionInput{
		Workspace:   workspace,
		Instruction: "change one file",
		Runtime:     execution.RuntimeCodex,
		Timeout:     time.Second,
		Environment: []string{"PATH=" + os.Getenv("PATH"), "WORKER_SECRET=must-not-pass"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("exit code = %#v, want zero", result.ExitCode)
	}
	if string(result.Stdout.Content) != "stdout-fact" || string(result.Stderr.Content) != "stderr-fact" {
		t.Fatalf("captured output = %q/%q", result.Stdout.Content, result.Stderr.Content)
	}
	if !strings.Contains(string(result.ChangedFiles.Content), "changed.txt") {
		t.Fatalf("changed files = %q", result.ChangedFiles.Content)
	}
	if result.AfterRevision != result.BeforeRevision {
		t.Fatalf("revision changed unexpectedly: %q -> %q", result.BeforeRevision, result.AfterRevision)
	}
}

func TestAdapterReturnsTimeoutFactWithoutSecondRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX executable")
	}
	workspace := initGitWorkspace(t)
	binary := filepath.Join(t.TempDir(), "slow-codex")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nsleep 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := New(binary).Run(context.Background(), execution.ExecutionInput{Workspace: workspace, Instruction: "wait", Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.TimedOut || result.ExitCode == nil {
		t.Fatalf("result = %#v, want timeout and observed exit code", result)
	}
}

func initGitWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	run := func(args ...string) {
		if output, err := runCommand(workspace, args...); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, output)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-m", "base")
	return workspace
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("look up %s: %v", name, err)
	}
	return path
}

func runCommand(directory string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = directory
	return command.CombinedOutput()
}

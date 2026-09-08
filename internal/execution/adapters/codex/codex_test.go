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

func TestAdapterCapturesPlanningCandidateSeparately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX executable")
	}
	workspace := initPlanningGitWorkspace(t)
	binary := filepath.Join(t.TempDir(), "fake-codex-candidate")
	script := "#!/bin/sh\noutput=''\nsandbox=''\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = '--output-last-message' ]; then shift; output=\"$1\"; fi\n  if [ \"$1\" = '--sandbox' ]; then shift; sandbox=\"$1\"; fi\n  shift\ndone\ncat >/dev/null\n[ \"$sandbox\" = 'read-only' ] || exit 9\nprintf '{\"summary\":\"candidate\"}' > \"$output\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := New(binary).Run(context.Background(), execution.ExecutionInput{
		Workspace: workspace, Instruction: "produce candidate", Runtime: execution.RuntimeCodex,
		ResultMode: execution.ResultModePlanningCandidate, Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Candidate.Kind != execution.ArtifactCandidate || string(result.Candidate.Content) != `{"summary":"candidate"}` || result.Candidate.Truncated {
		t.Fatalf("candidate = %#v", result.Candidate)
	}
	if status, err := runCommand(workspace, "status", "--porcelain=v1", "--untracked-files=all"); err != nil || strings.TrimSpace(string(status)) != "" {
		t.Fatalf("planning runtime temporary files changed snapshot: %q, err=%v", status, err)
	}
}

func TestAdapterClassifiesGitEvidenceCaptureFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX executable")
	}
	workspace := initGitWorkspace(t)
	binary := filepath.Join(t.TempDir(), "fake-codex-evidence-failure")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncat >/dev/null\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter := New(binary)
	defaultRunner := adapter.Guard.RunGit
	adapter.Guard.RunGit = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[2] == "diff" {
			return nil, os.ErrPermission
		}
		return defaultRunner(ctx, args...)
	}
	result, err := adapter.Run(context.Background(), execution.ExecutionInput{
		Workspace: workspace, Instruction: "observe evidence", Runtime: execution.RuntimeCodex,
		Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("Run() unexpectedly accepted missing Git evidence")
	}
	want := []execution.ArtifactKind{execution.ArtifactDiff, execution.ArtifactChangedFiles}
	if len(result.CaptureFailures) != len(want) {
		t.Fatalf("capture failures = %#v", result.CaptureFailures)
	}
	for index, kind := range want {
		failure := result.CaptureFailures[index]
		if failure.Kind != kind || failure.Stage != "git-evidence" {
			t.Fatalf("capture failure %d = %#v, want kind %s", index, failure, kind)
		}
	}
}

func TestAdapterDoesNotUseWorkspaceTMPDIR(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fallback and shell fixture are platform specific")
	}
	workspace := initGitWorkspace(t)
	temporaryBase := filepath.Join(workspace, "tmp")
	if err := os.Mkdir(temporaryBase, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporaryBase, "keep"), []byte("tracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := runCommand(workspace, "add", "tmp/keep"); err != nil {
		t.Fatalf("git add: %v (%s)", err, output)
	}
	if output, err := runCommand(workspace, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-m", "track temp sentinel"); err != nil {
		t.Fatalf("git commit: %v (%s)", err, output)
	}
	binary := filepath.Join(t.TempDir(), "fake-codex-safe-temp")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' \"$TMPDIR\"\ntouch \"$TMPDIR/codex-child-temp\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", temporaryBase)
	result, err := New(binary).Run(context.Background(), execution.ExecutionInput{Workspace: workspace, Instruction: "observe", Runtime: execution.RuntimeCodex, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	runtimeTemp := string(result.Stdout.Content)
	if runtimeTemp == "" || runtimeTemp == temporaryBase || filepath.Dir(runtimeTemp) == workspace {
		t.Fatalf("runtime TMPDIR = %q, want controlled directory outside workspace", runtimeTemp)
	}
	if _, err := os.Stat(runtimeTemp); !os.IsNotExist(err) {
		t.Fatalf("controlled runtime TMPDIR survived cleanup: %v", err)
	}
	entries, err := os.ReadDir(temporaryBase)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "keep" {
		t.Fatalf("workspace TMPDIR was touched: %v", entries)
	}
}

func initPlanningGitWorkspace(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "keystone-planning-snapshot-fixture")
	workspace := filepath.Join(parent, "source")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	return initGitWorkspaceAt(t, workspace)
}

func initGitWorkspace(t *testing.T) string {
	t.Helper()
	return initGitWorkspaceAt(t, t.TempDir())
}

func initGitWorkspaceAt(t *testing.T, workspace string) string {
	t.Helper()
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

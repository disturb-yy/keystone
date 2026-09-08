package sourcecontrol

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdapterProvisionAndObserveRealRepository(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-qm", "base")
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	workspace := filepath.Join(t.TempDir(), "workspace")
	adapter := Adapter{}
	result, err := adapter.Provision(context.Background(), ProvisionRequest{RepositoryRoot: root, WorkspacePath: workspace, Branch: "keystone/change/test", BaseRevision: base})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if result.Branch != "keystone/change/test" || result.HeadRevision != base {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := os.WriteFile(filepath.Join(workspace, "changed.txt"), []byte("change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := adapter.Observe(context.Background(), workspace, base)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if !snapshot.HasUntracked || len(snapshot.ChangedFiles) != 1 || snapshot.ChangedFiles[0] != "changed.txt" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	replayed, err := adapter.Provision(context.Background(), ProvisionRequest{RepositoryRoot: root, WorkspacePath: workspace, Branch: "keystone/change/test", BaseRevision: base})
	if err != nil || replayed.PhysicalPath != result.PhysicalPath {
		t.Fatalf("expected matching worktree replay, result=%+v err=%v", replayed, err)
	}
}

func TestAdapterRejectsDirtySource(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-qm", "base")
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Adapter{}).Provision(context.Background(), ProvisionRequest{RepositoryRoot: root, WorkspacePath: filepath.Join(t.TempDir(), "workspace"), Branch: "keystone/change/dirty", BaseRevision: base})
	if !errors.Is(err, ErrRepositoryDirty) {
		t.Fatalf("expected dirty source, got %v", err)
	}
}

func TestParseStatusIncludesRenameSourceAndTarget(t *testing.T) {
	files, untracked, err := parseStatus([]byte("R  old.txt\x00new.txt\x00?? untracked.txt\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if !untracked || len(files) != 3 || files[0] != "new.txt" || files[1] != "old.txt" || files[2] != "untracked.txt" {
		t.Fatalf("files=%#v untracked=%v", files, untracked)
	}
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
	return string(output)
}

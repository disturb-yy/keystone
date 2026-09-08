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

func TestAdapterCandidateAndCommitUseExpectedTreeAndParent(t *testing.T) {
	root := t.TempDir()
	initTestRepository(t, root)
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	workspace := filepath.Join(t.TempDir(), "workspace")
	if _, err := (Adapter{}).Provision(context.Background(), ProvisionRequest{RepositoryRoot: root, WorkspacePath: workspace, Branch: "keystone/change/commit", BaseRevision: base}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "new.txt"), []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidate, err := (Adapter{}).Candidate(context.Background(), workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.TreeIdentity == "" || !candidate.HasUntracked {
		t.Fatalf("candidate identity = %+v", candidate)
	}
	runGit(t, workspace, "add", "-N", "--", "new.txt")
	result, err := (Adapter{}).Commit(context.Background(), CommitRequest{WorkspacePath: workspace, ExpectedParent: base, ExpectedTree: candidate.TreeIdentity, Message: "controlled commit\n\nKeystone-Change-ID: change\nKeystone-Ticket-ID: ticket\nKeystone-Commit-ID: commit"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ParentRevision != base || result.AfterRevision == base || result.TreeIdentity != candidate.TreeIdentity {
		t.Fatalf("commit result = %+v", result)
	}
	if status := runGit(t, workspace, "status", "--porcelain"); status != "" {
		t.Fatalf("workspace is dirty after commit: %q", status)
	}
}

func TestAdapterReadsManifestFromBaseRevision(t *testing.T) {
	root := t.TempDir()
	initTestRepository(t, root)
	if err := os.MkdirAll(filepath.Join(root, ".keystone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".keystone", "project.yaml"), []byte("version: 1\nbase: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".keystone/project.yaml")
	runGit(t, root, "commit", "-qm", "manifest")
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	workspace := filepath.Join(t.TempDir(), "workspace")
	if _, err := (Adapter{}).Provision(context.Background(), ProvisionRequest{RepositoryRoot: root, WorkspacePath: workspace, Branch: "keystone/change/manifest", BaseRevision: base}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".keystone", "project.yaml"), []byte("version: 2\nproject_id: changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := (Adapter{}).ReadManifestAtRevision(context.Background(), workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "version: 1\nbase: true\n" {
		t.Fatalf("base manifest = %q", content)
	}
}

func initTestRepository(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-qm", "base")
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

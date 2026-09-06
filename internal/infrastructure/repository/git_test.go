package repository

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestDiscoverNormalRepositoryAndSubdirectory(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	subdir := filepath.Join(root, "nested")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	binding, err := (Git{}).Discover(context.Background(), subdir)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Root != root {
		t.Fatalf("root = %q, want %q", binding.Root, root)
	}
}

func TestDiscoverBareRepositoryIsUnsupported(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bare.git")
	runGit(t, filepath.Dir(root), "init", "--bare", root)
	if _, err := (Git{}).Discover(context.Background(), root); !errors.Is(err, domain.ErrRepositoryUnsupported) {
		t.Fatalf("Discover() error = %v, want ErrRepositoryUnsupported", err)
	}
}

func TestRootExistsRejectsExistingFileAsPreviousRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-repository")
	if err := os.WriteFile(path, []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	exists, err := (Git{}).RootExists(context.Background(), path)
	if exists {
		t.Fatal("RootExists() reported a file as an existing repository root")
	}
	if !errors.Is(err, domain.ErrProjectIdentityConflict) {
		t.Fatalf("RootExists() error = %v, want ErrProjectIdentityConflict", err)
	}
}

func TestSnapshotAcceptsCleanIgnoredAndDetachedRepositories(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("cache"), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := (Git{}).Snapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.BaseRevision) != 40 || snapshot.RepositoryRoot != root {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	runGit(t, root, "checkout", "--detach", "HEAD")
	if _, err := (Git{}).Snapshot(context.Background(), root); err != nil {
		t.Fatalf("detached snapshot error = %v", err)
	}
}

func TestSnapshotRejectsAllTrackedStateChanges(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, string)
	}{
		{name: "unstaged", change: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("changed"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "staged", change: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("changed"), 0644); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", "tracked.txt")
		}},
		{name: "untracked", change: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := committedRepository(t)
			test.change(t, root)
			_, err := (Git{}).Snapshot(context.Background(), root)
			if !errors.Is(err, domain.ErrRepositoryDirty) {
				t.Fatalf("Snapshot() error = %v, want ErrRepositoryDirty", err)
			}
		})
	}
}

func TestSnapshotRejectsUnbornHead(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	_, err := (Git{}).Snapshot(context.Background(), root)
	if !errors.Is(err, domain.ErrBaseRevisionUnavailable) {
		t.Fatalf("Snapshot() error = %v, want ErrBaseRevisionUnavailable", err)
	}
}

func committedRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "tracked.txt")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

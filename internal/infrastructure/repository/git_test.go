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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

package work_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/disturb-yy/keystone/internal/infrastructure/manifest"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/infrastructure/repository"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestInitializeIsIdempotentAndEventsAreUnique(t *testing.T) {
	service := newService(t)
	root := newGitRepository(t)
	first, err := service.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: root, IdempotencyKey: "key-1"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: filepath.Join(root, "nested"), IdempotencyKey: "key-1"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Identity.ProjectID != first.Identity.ProjectID {
		t.Fatalf("replay project = %q, want %q", replay.Identity.ProjectID, first.Identity.ProjectID)
	}
	second, err := service.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: root, IdempotencyKey: "key-2"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Identity.ProjectID != first.Identity.ProjectID {
		t.Fatalf("second key project = %q, want %q", second.Identity.ProjectID, first.Identity.ProjectID)
	}
	events, err := service.ListEvents(context.Background(), first.Identity.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
}

func TestInitializeSupportsMovedRepositoryButRejectsActiveClone(t *testing.T) {
	service := newService(t)
	root := newGitRepository(t)
	first, err := service.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: root, IdempotencyKey: "move-1"})
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	rebound, err := service.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: moved, IdempotencyKey: "move-2"})
	if err != nil {
		t.Fatal(err)
	}
	if rebound.Binding.Root != moved || rebound.Identity.ProjectID != first.Identity.ProjectID {
		t.Fatalf("rebound = %+v, want root %q and project %q", rebound, moved, first.Identity.ProjectID)
	}
	runGit(t, moved, "add", ".keystone/project.yaml")
	runGit(t, moved, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "manifest")
	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, filepath.Dir(clone), "clone", moved, clone)
	_, err = service.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: clone, IdempotencyKey: "clone-1"})
	if !errors.Is(err, domain.ErrProjectIdentityConflict) {
		data, readErr := os.ReadFile(filepath.Join(clone, ".keystone", "project.yaml"))
		t.Fatalf("clone init error = %v, manifest=%q, read_error=%v, want ErrProjectIdentityConflict", err, data, readErr)
	}
}

func TestInitializePreservesInvalidManifest(t *testing.T) {
	service := newService(t)
	root := newGitRepository(t)
	manifestPath := filepath.Join(root, ".keystone", "project.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		t.Fatal(err)
	}
	original := []byte("version: 1\nunknown: value\n")
	if err := os.WriteFile(manifestPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := service.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: root, IdempotencyKey: "invalid-1"})
	if !errors.Is(err, domain.ErrManifestInvalid) {
		t.Fatalf("init error = %v, want ErrManifestInvalid", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("manifest changed to %q", data)
	}
}

func newService(t *testing.T) *work.Service {
	t.Helper()
	db, err := sql.Open("sqlite", "file:service-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migration.NewRunner(append(migration.DefaultMigrations(), workstore.Migrations()...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	state, err := workstore.New(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := work.NewService(repository.Git{}, manifest.Store{}, state)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func newGitRepository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
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

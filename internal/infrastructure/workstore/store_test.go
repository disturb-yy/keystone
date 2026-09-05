package workstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:workstore-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrations := append(migration.DefaultMigrations(), Migrations()...)
	if err := migration.NewRunner(migrations).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestReserveFinalizeAndReplayKeepsOneEvent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	root := "/tmp/workstore-repo"
	binding := domain.RepositoryBinding{Root: root, ManifestPath: root + "/.keystone/project.yaml"}
	first, err := store.Reserve(ctx, root, "key-1", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	manifest := domain.ProjectManifest{Version: 1, ProjectID: first.Intent.ProjectID}
	project, err := store.Finalize(ctx, "key-1", first.Intent, manifest, binding, false)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.Reserve(ctx, root, "key-1", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if replay.Project == nil || replay.Project.Identity.ProjectID != project.Identity.ProjectID {
		t.Fatalf("replay = %+v, want project %q", replay.Project, project.Identity.ProjectID)
	}
	second, err := store.Reserve(ctx, root, "key-2", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if second.Intent.ProjectID != project.Identity.ProjectID {
		t.Fatalf("second intent project = %q, want %q", second.Intent.ProjectID, project.Identity.ProjectID)
	}
	if _, err := store.Finalize(ctx, "key-2", second.Intent, manifest, binding, false); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(ctx, project.Identity.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestFinalizeRejectsMissingEventWithoutRepair(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	root := "/tmp/workstore-integrity"
	first, err := store.Reserve(ctx, root, "key", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	binding := domain.RepositoryBinding{Root: root, ManifestPath: root + "/.keystone/project.yaml"}
	if _, err := store.Finalize(ctx, "key", first.Intent, domain.ProjectManifest{Version: 1, ProjectID: first.Intent.ProjectID}, binding, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DELETE FROM t_project_events WHERE project_id = ?`, first.Intent.ProjectID); err != nil {
		t.Fatal(err)
	}
	reservation, err := store.Reserve(ctx, root, "other-key", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Finalize(ctx, "other-key", reservation.Intent, domain.ProjectManifest{Version: 1, ProjectID: first.Intent.ProjectID}, binding, false)
	if !errors.Is(err, domain.ErrInternal) {
		t.Fatalf("Finalize() error = %v, want ErrInternal", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM t_project_events WHERE project_id = ?`, first.Intent.ProjectID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("event count = %d, want 0", count)
	}
}

func TestReserveSameKeyDifferentRootConflicts(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Reserve(context.Background(), "/tmp/root-a", "same-key", domain.NewProjectID()); err != nil {
		t.Fatal(err)
	}
	_, err := store.Reserve(context.Background(), "/tmp/root-b", "same-key", domain.NewProjectID())
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("Reserve() error = %v, want ErrIdempotencyConflict", err)
	}
}

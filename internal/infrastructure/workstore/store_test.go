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
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
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
	project, err := store.Finalize(ctx, "key-1", first.Intent, manifest, binding, "")
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
	if _, err := store.Finalize(ctx, "key-2", second.Intent, manifest, binding, ""); err != nil {
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

func TestSharedPendingIntentWritesReceiptsForBothKeys(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	root := "/tmp/workstore-shared-pending"
	binding := domain.RepositoryBinding{Root: root, ManifestPath: root + "/.keystone/project.yaml"}
	first, err := store.Reserve(ctx, root, "key-1", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Reserve(ctx, root, "key-2", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if second.Intent.ID != first.Intent.ID || second.Intent.IdempotencyKey != "key-1" {
		t.Fatalf("shared intent = %+v, want original intent %q", second.Intent, first.Intent.ID)
	}
	manifest := domain.ProjectManifest{Version: 1, ProjectID: first.Intent.ProjectID}
	project, err := store.Finalize(ctx, "key-2", second.Intent, manifest, binding, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"key-1", "key-2"} {
		replay, err := store.Reserve(ctx, root, key, domain.NewProjectID())
		if err != nil {
			t.Fatalf("Reserve(%q) error = %v", key, err)
		}
		if replay.Project == nil || replay.Project.Identity.ProjectID != project.Identity.ProjectID {
			t.Fatalf("Reserve(%q) project = %+v, want %q", key, replay.Project, project.Identity.ProjectID)
		}
	}
	events, err := store.ListEvents(ctx, project.Identity.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestFailIntentWritesStableFailureForSharedKeysAndReleasesRoot(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	root := "/tmp/workstore-failed-shared"
	first, err := store.Reserve(ctx, root, "key-1", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Reserve(ctx, root, "key-2", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailIntent(ctx, second.Intent, "key-2", "manifest_invalid"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"key-1", "key-2"} {
		if _, err := store.Reserve(ctx, root, key, domain.NewProjectID()); !errors.Is(err, domain.ErrManifestInvalid) {
			t.Fatalf("Reserve(%q) error = %v, want ErrManifestInvalid", key, err)
		}
	}
	retry, err := store.Reserve(ctx, root, "key-3", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if retry.Intent.ID == first.Intent.ID || retry.Intent.Status != domain.IntentPending {
		t.Fatalf("retry intent = %+v, want a new pending intent", retry.Intent)
	}
}

func TestConcurrentRebindsAllowOnlyOneStaleTransition(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	oldRoot := "/tmp/workstore-rebind-old"
	oldBinding := domain.RepositoryBinding{Root: oldRoot, ManifestPath: oldRoot + "/.keystone/project.yaml"}
	initial, err := store.Reserve(ctx, oldRoot, "initial", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	manifest := domain.ProjectManifest{Version: 1, ProjectID: initial.Intent.ProjectID}
	if _, err := store.Finalize(ctx, "initial", initial.Intent, manifest, oldBinding, ""); err != nil {
		t.Fatal(err)
	}
	rootA := "/tmp/workstore-rebind-a"
	rootB := "/tmp/workstore-rebind-b"
	reservationA, err := store.Reserve(ctx, rootA, "rebind-a", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	reservationB, err := store.Reserve(ctx, rootB, "rebind-b", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type result struct{ err error }
	results := make(chan result, 2)
	go func() {
		<-start
		_, err := store.Finalize(ctx, "rebind-a", reservationA.Intent, manifest, domain.RepositoryBinding{Root: rootA, ManifestPath: rootA + "/.keystone/project.yaml"}, oldRoot)
		results <- result{err: err}
	}()
	go func() {
		<-start
		_, err := store.Finalize(ctx, "rebind-b", reservationB.Intent, manifest, domain.RepositoryBinding{Root: rootB, ManifestPath: rootB + "/.keystone/project.yaml"}, oldRoot)
		results <- result{err: err}
	}()
	close(start)
	var successes int
	for range 2 {
		result := <-results
		if result.err == nil {
			successes++
			continue
		}
		if !errors.Is(result.err, domain.ErrProjectIdentityConflict) {
			t.Fatalf("concurrent rebind error = %v, want ErrProjectIdentityConflict", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent rebind successes = %d, want 1", successes)
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
	if _, err := store.Finalize(ctx, "key", first.Intent, domain.ProjectManifest{Version: 1, ProjectID: first.Intent.ProjectID}, binding, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER tr_change_events_no_delete`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DELETE FROM t_project_events WHERE project_id = ?`, first.Intent.ProjectID); err != nil {
		t.Fatal(err)
	}
	reservation, err := store.Reserve(ctx, root, "other-key", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Finalize(ctx, "other-key", reservation.Intent, domain.ProjectManifest{Version: 1, ProjectID: first.Intent.ProjectID}, binding, "")
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

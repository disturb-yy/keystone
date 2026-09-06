package workstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestCreateChangeCommandsAndTrace(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	root := "/tmp/change-store-repository"
	reservation, err := store.Reserve(ctx, root, "project-key", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.Finalize(ctx, "project-key", reservation.Intent, domain.ProjectManifest{Version: 1, ProjectID: reservation.Intent.ProjectID}, domain.RepositoryBinding{Root: root, ManifestPath: root + "/.keystone/project.yaml"}, "")
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domain.NewChangeIntent("  add lifecycle \n support ")
	if err != nil {
		t.Fatal(err)
	}
	change, err := store.CreateChange(ctx, work.ChangeCreateRecord{Project: project, Snapshot: domain.ChangeSourceSnapshot{RepositoryRoot: root, BaseRevision: "0123456789012345678901234567890123456789"}, Intent: intent, IntentIdentity: domain.NewArtifactIdentity([]byte(intent.Original)), IdempotencyKey: "change-key", Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if change.Status != domain.ChangeStatusActive || change.Stage != domain.LifecycleStageIntent || change.Version != 1 {
		t.Fatalf("created change = %+v", change)
	}
	replay, err := store.CreateChange(ctx, work.ChangeCreateRecord{Project: project, Snapshot: domain.ChangeSourceSnapshot{RepositoryRoot: root, BaseRevision: change.BaseRevision}, Intent: intent, IntentIdentity: domain.NewArtifactIdentity([]byte(intent.Original)), IdempotencyKey: "change-key", Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != change.ID {
		t.Fatalf("replay id = %q, want %q", replay.ID, change.ID)
	}
	if _, err := store.ApplyCommand(ctx, change.ID, "pause", 1, "pause-key", "test"); err != nil {
		t.Fatal(err)
	}
	pausedReplay, err := store.ApplyCommand(ctx, change.ID, "pause", 1, "pause-key", "test")
	if err != nil {
		t.Fatal(err)
	}
	if pausedReplay.Status != domain.ChangeStatusPaused || pausedReplay.Version != 2 {
		t.Fatalf("paused replay = %+v, want original response snapshot", pausedReplay)
	}
	if _, err := store.ApplyCommand(ctx, change.ID, "pause", 1, "stale-key", "test"); !errors.Is(err, domain.ErrChangeVersionConflict) {
		t.Fatalf("stale pause error = %v, want version conflict", err)
	}
	if _, err := store.ApplyCommand(ctx, change.ID, "resume", 2, "resume-key", "test"); err != nil {
		t.Fatal(err)
	}
	run, err := store.StartAgentRunWithArtifacts(ctx, change.ID, "test", []domain.AgentRunArtifact{{ArtifactRefID: change.Intent.ID, Role: domain.ArtifactRoleInput, Ordinal: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRunStarted(run); err != nil {
		t.Fatal(err)
	}
	failureRef := insertTestArtifactRef(t, store, project, change, domain.ArtifactRoleFailure, "failure evidence")
	if _, err := store.CompleteAgentRunWithArtifacts(ctx, run.ID, domain.AgentRunOutcomeFailed, "test", []domain.AgentRunArtifact{{ArtifactRefID: failureRef.ID, Role: domain.ArtifactRoleFailure, Ordinal: 0}}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.ChangeStatusHumanRequired || updated.Version != 4 {
		t.Fatalf("failed run change = %+v, want human_required version 4", updated)
	}
	retried, err := store.ApplyDecision(ctx, change.ID, domain.HumanDecisionRetry, updated.Version, "retry-key", "test", "retry once")
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != domain.ChangeStatusActive || retried.Version != 5 {
		t.Fatalf("retried change = %+v", retried)
	}
	events, err := store.ListChangeEvents(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 8 || events[0].Type != domain.ChangeCreatedType || events[1].Type != domain.ChangePausedType || events[2].Type != domain.ChangeResumedType || events[3].Type != domain.AgentRunStartedType || events[4].Type != domain.AgentRunCompletedType || events[5].Type != domain.ChangeHumanRequiredType || events[6].Type != domain.HumanDecisionRecordedType || events[7].Type != domain.AgentRunStartedType {
		t.Fatalf("events = %+v", events)
	}
	if len(events[3].ArtifactRefIDs) != 1 || events[3].ArtifactRefIDs[0] != change.Intent.ID || len(events[4].ArtifactRefIDs) != 1 || events[4].ArtifactRefIDs[0] != failureRef.ID || len(events[5].ArtifactRefIDs) != 1 || events[5].ArtifactRefIDs[0] != failureRef.ID {
		t.Fatalf("agent run event artifacts = %+v", events[3:6])
	}
	runs, err := store.ListAgentRuns(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Status != domain.AgentRunStatusCompleted || runs[1].Attempt != 2 {
		t.Fatalf("runs = %+v", runs)
	}
	if len(runs[0].Artifacts) != 2 || runs[0].Artifacts[0].Role != domain.ArtifactRoleInput || runs[0].Artifacts[1].Role != domain.ArtifactRoleFailure {
		t.Fatalf("run artifacts = %+v", runs[0].Artifacts)
	}
	decisions, err := store.ListHumanDecisions(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Reason != "retry once" {
		t.Fatalf("decisions = %+v", decisions)
	}
}

func checkRunStarted(run domain.AgentRun) error {
	if run.Status != domain.AgentRunStatusRunning || run.Attempt != 1 || run.StartedAt.IsZero() {
		return errors.New("agent run was not started")
	}
	return nil
}

func TestCreateChangeDifferentKeysAllowIndependentChanges(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	root := "/tmp/change-store-independent"
	reservation, err := store.Reserve(ctx, root, "project-key-independent", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.Finalize(ctx, "project-key-independent", reservation.Intent, domain.ProjectManifest{Version: 1, ProjectID: reservation.Intent.ProjectID}, domain.RepositoryBinding{Root: root, ManifestPath: root + "/.keystone/project.yaml"}, "")
	if err != nil {
		t.Fatal(err)
	}
	intent, _ := domain.NewChangeIntent("same intent")
	record := work.ChangeCreateRecord{Project: project, Snapshot: domain.ChangeSourceSnapshot{RepositoryRoot: root, BaseRevision: "abcdefabcdefabcdefabcdefabcdefabcdefabcd"}, Intent: intent, IntentIdentity: domain.NewArtifactIdentity([]byte(intent.Original)), Actor: "test"}
	record.IdempotencyKey = "one"
	first, err := store.CreateChange(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	record.IdempotencyKey = "two"
	second, err := store.CreateChange(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("different idempotency keys reused a Change")
	}
	changes, err := store.ListChanges(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %d, want 2", len(changes))
	}
}

func TestHumanDecisionCancelAppendsCancellationEvent(t *testing.T) {
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/change-store-decision-cancel", "decision-cancel-project", "decision-cancel-change")
	run, err := store.StartAgentRun(context.Background(), change.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteAgentRun(context.Background(), run.ID, domain.AgentRunOutcomeFailed, "test"); err != nil {
		t.Fatal(err)
	}
	humanRequired, err := store.FindChange(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyDecision(context.Background(), change.ID, domain.HumanDecisionCancel, humanRequired.Version, "decision-cancel-key", "test", "stop"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListChangeEvents(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 || events[4].Type != domain.HumanDecisionRecordedType || events[5].Type != domain.ChangeCancelledType {
		t.Fatalf("cancel decision events = %+v", events)
	}
}

func TestOlderAgentRunOnlyAppendsTrace(t *testing.T) {
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/change-store-late-run", "late-run-project", "late-run-change")
	first, err := store.StartAgentRun(context.Background(), change.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.StartAgentRun(context.Background(), change.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteAgentRun(context.Background(), first.ID, domain.AgentRunOutcomeSucceeded, "test"); err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.FindChange(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Stage != domain.LifecycleStageIntent || unchanged.Version != 1 {
		t.Fatalf("older run changed current stage = %+v", unchanged)
	}
	if _, err := store.CompleteAgentRun(context.Background(), second.ID, domain.AgentRunOutcomeSucceeded, "test"); err != nil {
		t.Fatal(err)
	}
	advanced, err := store.FindChange(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Stage != domain.LifecycleStageUnderstand || advanced.Version != 2 {
		t.Fatalf("current run did not advance stage = %+v", advanced)
	}
}

func TestChangeSchemaEnforcesOwnershipAndAppendOnlyHistory(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	firstProject, firstChange := createTestChange(t, store, "/tmp/change-store-owner-one", "owner-project-one", "owner-change-one")
	_, secondChange := createTestChange(t, store, "/tmp/change-store-owner-two", "owner-project-two", "owner-change-two")

	firstRun, err := store.StartAgentRunWithArtifacts(ctx, firstChange.ID, "test", []domain.AgentRunArtifact{{ArtifactRefID: firstChange.Intent.ID, Role: domain.ArtifactRoleInput, Ordinal: 0}})
	if err != nil {
		t.Fatal(err)
	}
	secondRun, err := store.StartAgentRun(ctx, secondChange.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO t_agent_run_artifacts (agent_run_id, artifact_ref_id, role, ordinal) VALUES (?, ?, 'input', 0)`, secondRun.ID, firstChange.Intent.ID); err == nil {
		t.Fatal("cross-change AgentRun artifact association was accepted")
	}
	if _, err := store.db.Exec(`UPDATE t_agent_runs SET status = 'running' WHERE agent_run_id = ?`, firstRun.ID); err == nil {
		t.Fatal("invalid AgentRun update was accepted")
	}
	if _, err := store.db.Exec(`DELETE FROM t_agent_runs WHERE agent_run_id = ?`, firstRun.ID); err == nil {
		t.Fatal("AgentRun delete was accepted")
	}
	if _, err := store.db.Exec(`UPDATE t_artifacts SET media_type = 'application/json' WHERE artifact_id = ?`, firstChange.Intent.ArtifactID); err == nil {
		t.Fatal("Artifact update was accepted")
	}
	if _, err := store.db.Exec(`DELETE FROM t_artifact_refs WHERE artifact_ref_id = ?`, firstChange.Intent.ID); err == nil {
		t.Fatal("ArtifactRef delete was accepted")
	}
	if _, err := store.db.Exec(`DELETE FROM t_project_events WHERE change_id = ?`, firstChange.ID); err == nil {
		t.Fatal("Change Event delete was accepted")
	}
	if _, err := store.db.Exec(`INSERT INTO t_project_events (event_id, project_id, type, occurred_at, event_sequence, change_id, agent_run_id, actor) VALUES ('cross-event', ?, 'StageAdvanced', ?, 4, ?, ?, '')`, firstProject.Identity.ProjectID, time.Now().UTC().Format(time.RFC3339Nano), firstChange.ID, firstRun.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO t_event_artifacts (event_id, artifact_ref_id, ordinal) VALUES ('cross-event', ?, 0)`, secondChange.Intent.ID); err == nil {
		t.Fatal("cross-change Event artifact association was accepted")
	}
	var foreignKeys int
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestChangeMigrationEvolvesExistingProjectLedger(t *testing.T) {
	dsn := "file:change-migration-" + filepath.Base(t.TempDir()) + "?mode=memory&cache=shared"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	legacyMigrations := append(migration.DefaultMigrations(), Migrations()[0])
	if err := migration.NewRunner(legacyMigrations).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	root := "/tmp/change-store-legacy"
	projectID := domain.NewProjectID()
	created := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO t_projects (project_id, repository_root, manifest_path, created_at) VALUES (?, ?, ?, ?)`, projectID, root, root+"/.keystone/project.yaml", created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t_project_events (event_id, project_id, type, occurred_at) VALUES (?, ?, ?, ?)`, "legacy-event", projectID, domain.ProjectInitializedType, created); err != nil {
		t.Fatal(err)
	}
	if err := migration.NewRunner(append(migration.DefaultMigrations(), Migrations()...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ProjectID != projectID {
		t.Fatalf("migrated project events = %+v", events)
	}
}

func createTestChange(t *testing.T, store *Store, root, projectKey, changeKey string) (domain.Project, domain.Change) {
	t.Helper()
	reservation, err := store.Reserve(context.Background(), root, projectKey, domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.Finalize(context.Background(), projectKey, reservation.Intent, domain.ProjectManifest{Version: 1, ProjectID: reservation.Intent.ProjectID}, domain.RepositoryBinding{Root: root, ManifestPath: filepath.Join(root, ".keystone", "project.yaml")}, "")
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domain.NewChangeIntent(fmt.Sprintf("intent for %s", changeKey))
	if err != nil {
		t.Fatal(err)
	}
	change, err := store.CreateChange(context.Background(), work.ChangeCreateRecord{Project: project, Snapshot: domain.ChangeSourceSnapshot{RepositoryRoot: root, BaseRevision: "0123456789012345678901234567890123456789"}, Intent: intent, IntentIdentity: domain.NewArtifactIdentity([]byte(intent.Original)), IdempotencyKey: changeKey, Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return project, change
}

func insertTestArtifactRef(t *testing.T, store *Store, project domain.Project, change domain.Change, role, content string) domain.ArtifactRef {
	t.Helper()
	identity := domain.NewArtifactIdentity([]byte(content))
	artifactID := domain.ArtifactID(domain.NewProjectID())
	ref := domain.ArtifactRef{ID: domain.ArtifactRefID(domain.NewProjectID()), ChangeID: change.ID, ArtifactID: artifactID, Role: role, Ordinal: 0}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`INSERT INTO t_artifacts (artifact_id, sha256, byte_length, media_type, created_at) VALUES (?, ?, ?, 'text/plain; charset=utf-8', ?)`, artifactID, identity.SHA256, identity.ByteLength, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO t_artifact_refs (artifact_ref_id, project_id, change_id, artifact_id, role, ordinal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, ref.ID, project.Identity.ProjectID, change.ID, artifactID, role, ref.Ordinal, now); err != nil {
		t.Fatal(err)
	}
	return ref
}

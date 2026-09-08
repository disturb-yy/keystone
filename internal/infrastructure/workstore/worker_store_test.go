package workstore

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/artifact"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestWorkerAssignmentReportDuplicateAndLateTrace(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "worker-project", "worker-change")
	workerID := "worker-instance-1"
	secret, err := NewWorkerSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareWorker(ctx, workerID, secret); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthenticateWorker(ctx, workerID, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RegisterWorker(ctx, workercontract.Register{WorkerID: workerID, ProtocolVersion: workercontract.ProtocolVersionV1, Capabilities: []string{"runtime:codex"}}); err != nil {
		t.Fatal(err)
	}
	run, err := store.StartAgentRun(ctx, change.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	assignment, err := store.IssueAssignment(ctx, run.ID, workerID, workspace, "codex", "edit fixture", change.BaseRevision, "fixture-workspace", nil)
	if err != nil {
		t.Fatal(err)
	}
	pulled, err := store.PullAssignment(ctx, workerID)
	if err != nil {
		t.Fatal(err)
	}
	if pulled == nil || pulled.LeaseToken != assignment.LeaseToken || pulled.AgentRunID != string(run.ID) {
		t.Fatalf("pulled assignment = %+v, want %+v", pulled, assignment)
	}
	artifactStore, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("worker output")
	identity := domain.NewArtifactIdentity(content)
	exitCode := 0
	report := workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
		Outcome: workercontract.Outcome("succeeded"), ExitCode: &exitCode,
		Artifacts: []workercontract.Artifact{{Kind: "stdout", ContentBase64: base64.StdEncoding.EncodeToString(content), SHA256: identity.SHA256, SizeBytes: identity.ByteLength}},
	}
	response, err := store.ReportWorkerFor(ctx, workerID, report, artifactStore)
	if err != nil {
		t.Fatal(err)
	}
	if response.Disposition != "accepted" || response.LeaseState != "consumed" {
		t.Fatalf("first report = %+v, want accepted/consumed", response)
	}
	assertNoLeaseTokens(t, store)
	replay, err := store.ReportWorkerFor(ctx, workerID, report, artifactStore)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Disposition != "duplicate" {
		t.Fatalf("replay = %+v, want duplicate", replay)
	}
	events, err := store.ListChangeEvents(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[2].Type != domain.AgentRunCompletedType || events[3].Type != domain.StageAdvancedType {
		t.Fatalf("events after duplicate = %+v", events)
	}
	if _, err := store.HeartbeatWorker(ctx, workercontract.Heartbeat{WorkerID: workerID}); err != nil {
		t.Fatal(err)
	}

	lateRun, err := store.StartAgentRun(ctx, change.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	lateAssignment, err := store.IssueAssignment(ctx, lateRun.ID, workerID, workspace, "codex", "late fixture", change.BaseRevision, "fixture-workspace-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(workerLeaseTTL + time.Second)
	oldNow := store.now
	store.now = func() time.Time { return future }
	lateReport := workercontract.Report{AgentRunID: string(lateRun.ID), LeaseToken: lateAssignment.LeaseToken, Attempt: lateRun.Attempt, Outcome: workercontract.Outcome("failed")}
	late, err := store.ReportWorkerFor(ctx, workerID, lateReport, artifactStore)
	store.now = oldNow
	if err != nil {
		t.Fatal(err)
	}
	if late.Disposition != "late" || late.LeaseState != "expired" {
		t.Fatalf("late report = %+v, want late/expired", late)
	}
	storedLate, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedLate.LatestAgentRun == nil || storedLate.LatestAgentRun.ID != lateRun.ID || storedLate.LatestAgentRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("late report changed run authority = %+v", storedLate.LatestAgentRun)
	}
	events, err = store.ListChangeEvents(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 || events[5].Type != domain.AgentRunReportLateType {
		t.Fatalf("events after late report = %+v", events)
	}
}

func TestExpiredWorkerRequiresIdleHeartbeatBeforeReuse(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, firstChange := createTestChange(t, store, t.TempDir(), "busy-worker-project-1", "busy-worker-change-1")
	_, secondChange := createTestChange(t, store, t.TempDir(), "busy-worker-project-2", "busy-worker-change-2")
	workerID := registerPlanningWorker(t, store, "busy-worker")
	firstRun, err := store.StartAgentRun(ctx, firstChange.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := store.IssueAssignment(ctx, firstRun.ID, workerID, t.TempDir(), "codex", "first", firstChange.BaseRevision, "first-workspace", nil)
	if err != nil {
		t.Fatal(err)
	}
	secondRun, err := store.StartAgentRun(ctx, secondChange.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	oldNow := store.now
	store.now = func() time.Time { return oldNow().Add(workerLeaseTTL + time.Second) }
	if available, ok, err := store.AvailableWorker(ctx, "runtime:codex"); err != nil || ok || available != "" {
		t.Fatalf("expired busy worker availability = %q/%t, err=%v", available, ok, err)
	}
	assertNoLeaseTokens(t, store)
	if _, err := store.IssueAssignment(ctx, secondRun.ID, workerID, t.TempDir(), "codex", "second", secondChange.BaseRevision, "second-workspace", nil); !errors.Is(err, ErrWorkerNotRegistered) {
		t.Fatalf("stale worker assignment error = %v, want ErrWorkerNotRegistered", err)
	}
	activeHeartbeat, err := store.HeartbeatWorker(ctx, workercontract.Heartbeat{WorkerID: workerID, AgentRunID: string(firstRun.ID), LeaseTokenSHA256: hashSecret(assignment.LeaseToken)})
	if err != nil {
		t.Fatal(err)
	}
	if activeHeartbeat.LeaseRenewed || activeHeartbeat.WorkerAvailable {
		t.Fatalf("expired active heartbeat = %+v", activeHeartbeat)
	}
	if idleHeartbeat, err := store.HeartbeatWorker(ctx, workercontract.Heartbeat{WorkerID: workerID}); err != nil || !idleHeartbeat.WorkerAvailable {
		t.Fatalf("idle heartbeat = %+v, err=%v", idleHeartbeat, err)
	}
	if available, ok, err := store.AvailableWorker(ctx, "runtime:codex"); err != nil || !ok || available != workerID {
		t.Fatalf("idle worker availability = %q/%t, err=%v", available, ok, err)
	}
	if _, err := store.IssueAssignment(ctx, secondRun.ID, workerID, t.TempDir(), "codex", "second", secondChange.BaseRevision, "second-workspace", nil); err != nil {
		t.Fatal(err)
	}
}

func assertNoLeaseTokens(t *testing.T, store *Store) {
	t.Helper()
	store.leaseMu.RLock()
	defer store.leaseMu.RUnlock()
	if len(store.leaseTokens) != 0 {
		t.Fatalf("terminal leases retained %d plaintext tokens", len(store.leaseTokens))
	}
}

func TestWorkerRestartReconcilesRunningRunWithoutRetry(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "restart-project", "restart-change")
	workerID := "worker-restart"
	secret, err := NewWorkerSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareWorker(ctx, workerID, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RegisterWorker(ctx, workercontract.Register{WorkerID: workerID, ProtocolVersion: workercontract.ProtocolVersionV1}); err != nil {
		t.Fatal(err)
	}
	run, err := store.StartAgentRun(ctx, change.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileWorkerRestart(ctx); err != nil {
		t.Fatal(err)
	}
	updated, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.ChangeStatusHumanRequired || updated.LatestAgentRun == nil || updated.LatestAgentRun.ID != run.ID || updated.LatestAgentRun.Outcome != domain.AgentRunOutcomeFailed {
		t.Fatalf("restart reconciliation = %+v", updated)
	}
	runs, err := store.ListAgentRuns(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domain.AgentRunStatusCompleted {
		t.Fatalf("reconciled runs = %+v", runs)
	}
	if _, err := store.IssueAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "retry", change.BaseRevision, "restart-workspace", nil); err == nil {
		t.Fatal("reused reconciled AgentRun for a new assignment")
	}
}

func TestWorkerMigrationPreservesExistingEventArtifacts(t *testing.T) {
	db, err := sql.Open("sqlite", "file:worker-migration-preserve?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	migrations := Migrations()
	if err := migration.NewRunner(append(migration.DefaultMigrations(), migrations[:2]...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	_, change := createTestChange(t, store, t.TempDir(), "worker-migration-project", "worker-migration-change")
	if _, err := store.StartAgentRunWithArtifacts(context.Background(), change.ID, "test", []domain.AgentRunArtifact{{ArtifactRefID: change.Intent.ID, Role: domain.ArtifactRoleInput, Ordinal: 0}}); err != nil {
		t.Fatal(err)
	}
	if err := migration.NewRunner(append(migration.DefaultMigrations(), migrations...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	migrated, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	events, err := migrated.ListChangeEvents(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || len(events[1].ArtifactRefIDs) != 1 || events[1].ArtifactRefIDs[0] != change.Intent.ID {
		t.Fatalf("migrated event artifacts = %+v", events)
	}
}

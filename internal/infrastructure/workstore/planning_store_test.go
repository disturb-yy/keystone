package workstore

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/artifact"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestPlanningCompletionPersistsMetadataLinksAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/planning-completion", "planning-project", "planning-change")
	contextContent := []byte(`{"version":"ProjectContext.v1"}`)
	start := work.StartPlanningRunRequest{
		ChangeID: change.ID, TargetStage: domain.LifecycleStageUnderstand,
		ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: "planning",
		InputArtifactRefIDs: []domain.ArtifactRefID{change.Intent.ID},
		InputArtifacts: []work.PlanningArtifactWrite{{
			Identity: domain.NewArtifactIdentity(contextContent), MediaType: "application/json",
			Kind: "project_context", SchemaVersion: "ProjectContext.v1", Summary: "项目上下文", SourceRevision: change.BaseRevision,
		}},
	}
	run, err := store.StartPlanningRun(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if !run.IsPlanning() || run.Stage != domain.LifecycleStageUnderstand || run.Attempt != 1 || len(run.Artifacts) != 2 {
		t.Fatalf("planning run = %+v", run)
	}
	if _, err := store.StartPlanningRun(ctx, start); !errors.Is(err, domain.ErrPlanningRunConflict) {
		t.Fatalf("second planning start error = %v, want conflict", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	existingRawRef, err := insertPlanningArtifactRef(ctx, tx, change, work.PlanningArtifactWrite{
		Identity: domain.NewArtifactIdentity([]byte("existing runtime candidate")), MediaType: "application/json",
		Kind: "candidate", SourceRevision: change.BaseRevision,
	}, domain.ArtifactRoleOutput, store.now().UTC())
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	inputRefs := runInputRefIDs(run)
	completion := work.CompletePlanningStageRequest{
		AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt,
		ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: "planning",
		Artifact: work.PlanningArtifactWrite{
			Identity: domain.NewArtifactIdentity([]byte(`{"kind":"understanding"}`)), MediaType: "application/json",
			Kind: "understanding", SchemaVersion: "Understanding.v1", Summary: "需求理解", SourceRevision: change.BaseRevision,
			InputArtifactRefIDs: inputRefs, RawLogArtifactRefIDs: []domain.ArtifactRefID{existingRawRef.ID},
		},
		RawLogs: []work.PlanningArtifactWrite{{
			Identity: domain.NewArtifactIdentity([]byte("runtime log")), MediaType: "text/plain",
			Kind: "stdout", SourceRevision: change.BaseRevision,
		}},
	}
	result, err := store.CompletePlanningStage(ctx, completion)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != work.PlanningCommitCommitted || result.Change.Stage != domain.LifecycleStageUnderstand || result.Change.Version != 2 {
		t.Fatalf("planning result = %+v", result)
	}
	if result.ArtifactRef.Kind != "understanding" || result.ArtifactRef.SchemaVersion != "Understanding.v1" || !sameArtifactRefIDs(result.ArtifactRef.InputArtifactRefIDs, inputRefs) || len(result.ArtifactRef.RawLogArtifactRefIDs) != 2 || result.ArtifactRef.RawLogArtifactRefIDs[0] != existingRawRef.ID {
		t.Fatalf("planning artifact ref = %+v", result.ArtifactRef)
	}
	replay, err := store.CompletePlanningStage(ctx, completion)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Disposition != work.PlanningCommitDuplicate || replay.ArtifactRef.ID != result.ArtifactRef.ID {
		t.Fatalf("planning replay = %+v", replay)
	}
	conflicting := completion
	conflicting.RawLogs = append([]work.PlanningArtifactWrite(nil), completion.RawLogs...)
	conflicting.RawLogs[0].Identity = domain.NewArtifactIdentity([]byte("different runtime log"))
	if _, err := store.CompletePlanningStage(ctx, conflicting); !errors.Is(err, domain.ErrPlanningRunConflict) {
		t.Fatalf("conflicting planning replay error = %v", err)
	}
	assertPlanningTrace(t, store, change.ID, 4, domain.StageAdvancedType)
	refs, err := store.ListArtifactRefs(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 5 || !refs[0].IsLegacy() {
		t.Fatalf("artifact refs = %+v", refs)
	}
	var commits int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM t_planning_stage_commits WHERE agent_run_id = ?`, run.ID).Scan(&commits); err != nil || commits != 1 {
		t.Fatalf("planning commits = %d, err = %v", commits, err)
	}
}

func TestPlanningCompletionRollsBackAllSQLiteFacts(t *testing.T) {
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/planning-rollback", "rollback-project", "rollback-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	if _, err := store.db.Exec(`CREATE TRIGGER test_reject_planning_advance BEFORE UPDATE OF stage ON t_changes BEGIN SELECT RAISE(ABORT, 'reject planning advance'); END`); err != nil {
		t.Fatal(err)
	}
	request := successfulPlanningCompletion(change, run, "understanding", "Understanding.v1", "understood")
	if _, err := store.CompletePlanningStage(context.Background(), request); err == nil {
		t.Fatal("planning completion unexpectedly succeeded")
	}
	runs, err := store.ListAgentRuns(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domain.AgentRunStatusRunning || len(runs[0].Artifacts) != 1 {
		t.Fatalf("runs after rollback = %+v", runs)
	}
	var refs, commits int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM t_artifact_refs WHERE change_id = ?`, change.ID).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM t_planning_stage_commits WHERE agent_run_id = ?`, run.ID).Scan(&commits); err != nil {
		t.Fatal(err)
	}
	if refs != 1 || commits != 0 {
		t.Fatalf("rollback facts refs=%d commits=%d", refs, commits)
	}
}

func TestPlanningFailureRequiresExplicitRetryTarget(t *testing.T) {
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/planning-failure", "failure-project", "failure-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	failure := work.FailPlanningStageRequest{
		AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt,
		ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: "planning",
		Failure: work.PlanningArtifactWrite{
			Identity: domain.NewArtifactIdentity([]byte("schema invalid")), MediaType: "text/plain",
			Kind: "planning_failure", Summary: "schema invalid", SourceRevision: change.BaseRevision,
			InputArtifactRefIDs: runInputRefIDs(run),
		},
	}
	failed, err := store.FailPlanningStage(context.Background(), failure)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Change.Status != domain.ChangeStatusHumanRequired || failed.Change.Stage != domain.LifecycleStageIntent || failed.Change.Version != 2 {
		t.Fatalf("failed planning change = %+v", failed.Change)
	}
	retried, err := store.ApplyDecision(context.Background(), change.ID, domain.HumanDecisionRetry, failed.Change.Version, "planning-retry", "human", "retry validated input")
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.ListAgentRuns(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != domain.ChangeStatusActive || len(runs) != 1 {
		t.Fatalf("planning retry change=%+v runs=%+v", retried, runs)
	}
	retryRun, err := store.StartPlanningRun(context.Background(), work.StartPlanningRunRequest{
		ChangeID: change.ID, TargetStage: domain.LifecycleStageUnderstand,
		ExpectedChangeVersion: retried.Version, SourceRevision: change.BaseRevision, Actor: "planning",
		InputArtifactRefIDs: []domain.ArtifactRefID{change.Intent.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if retryRun.Attempt != 2 {
		t.Fatalf("retry attempt = %d, want 2", retryRun.Attempt)
	}
}

func TestPlanningStagesStopAtTicketize(t *testing.T) {
	store := newTestStore(t)
	_, initial := createTestChange(t, store, "/tmp/planning-sequence", "sequence-project", "sequence-change")
	change := initial
	inputRef := change.Intent.ID
	stages := []struct {
		stage  domain.LifecycleStage
		kind   string
		schema string
		want   domain.LifecycleStage
	}{
		{domain.LifecycleStageUnderstand, "understanding", "Understanding.v1", domain.LifecycleStageUnderstand},
		{domain.LifecycleStageDesign, "design", "Design.v1", domain.LifecycleStageDesign},
		{domain.LifecycleStagePlan, "plan", "Plan.v1", domain.LifecycleStageTicketize},
	}
	for _, step := range stages {
		run := startPlanningRun(t, store, change, step.stage, inputRef)
		result, err := store.CompletePlanningStage(context.Background(), successfulPlanningCompletion(change, run, step.kind, step.schema, string(step.stage)))
		if err != nil {
			t.Fatalf("complete %s: %v", step.stage, err)
		}
		if result.Change.Stage != step.want || result.Change.Status != domain.ChangeStatusActive {
			t.Fatalf("complete %s change = %+v", step.stage, result.Change)
		}
		change, inputRef = result.Change, result.ArtifactRef.ID
	}
	if changes, err := store.ListRecoverablePlanningChanges(context.Background()); err != nil {
		t.Fatal(err)
	} else if len(changes) != 0 {
		t.Fatalf("Ticketize change remained recoverable = %+v", changes)
	}
}

func TestPlanningCandidateReportIsDurableNonAuthoritativeAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "candidate-project", "candidate-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	workerID := registerPlanningWorker(t, store, "planning-worker")
	available, ok, err := store.AvailableWorker(ctx, "runtime:codex")
	if err != nil || !ok || available != workerID {
		t.Fatalf("available worker = %q/%t, err=%v", available, ok, err)
	}
	assignment, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "planning-snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	if assignment.ResultMode != workercontract.ResultModePlanningCandidate {
		t.Fatalf("assignment result mode = %q", assignment.ResultMode)
	}
	if _, ok, err := store.AvailableWorker(ctx, "runtime:codex"); err != nil || ok {
		t.Fatalf("leased worker availability = %t, err=%v", ok, err)
	}
	pulled, err := store.PullAssignment(ctx, workerID)
	if err != nil || pulled == nil || pulled.ResultMode != workercontract.ResultModePlanningCandidate {
		t.Fatalf("pulled assignment = %+v, err=%v", pulled, err)
	}
	artifactStore, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	report := workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
		Outcome: "succeeded", ExitCode: &exitCode, AfterRevision: change.BaseRevision,
		Artifacts: []workercontract.Artifact{
			workerReportArtifact("candidate", []byte(`{"kind":"understanding"}`), true),
			workerReportArtifact("stdout", []byte("raw stdout"), false),
		},
	}
	response, err := store.ReportWorkerFor(ctx, workerID, report, artifactStore)
	if err != nil {
		t.Fatal(err)
	}
	if response.Disposition != "candidate_received" || response.LeaseState != "consumed" {
		t.Fatalf("candidate response = %+v", response)
	}
	assertPlanningWorkspaceCleared(t, store, run.ID)
	assertNoLeaseTokens(t, store)
	candidate, err := store.FindPlanningRunCandidate(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Outcome != domain.AgentRunOutcomeSucceeded || candidate.CandidateRef == nil || !candidate.CandidateTruncated || len(candidate.RawLogRefs) != 1 {
		t.Fatalf("planning candidate = %+v", candidate)
	}
	stored, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Stage != domain.LifecycleStageIntent || stored.Version != 1 || stored.LatestAgentRun == nil || stored.LatestAgentRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("candidate changed authority = %+v", stored)
	}
	replay, err := store.ReportWorkerFor(ctx, workerID, report, artifactStore)
	if err != nil || replay.Disposition != "duplicate" {
		t.Fatalf("candidate replay = %+v, err=%v", replay, err)
	}
	if err := store.ReconcileWorkerLost(ctx, workerID); err != nil {
		t.Fatal(err)
	}
	afterWorkerLost, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterWorkerLost.Status != domain.ChangeStatusActive || afterWorkerLost.LatestAgentRun == nil || afterWorkerLost.LatestAgentRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("worker lost overrode durable candidate = %+v", afterWorkerLost)
	}
	if err := store.ReconcileWorkerRestart(ctx); err != nil {
		t.Fatal(err)
	}
	afterRestart, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRestart.Status != domain.ChangeStatusActive || afterRestart.LatestAgentRun == nil || afterRestart.LatestAgentRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("restart lost durable candidate = %+v", afterRestart)
	}
	assertPlanningTrace(t, store, change.ID, 2, domain.AgentRunStartedType)
}

func TestPlanningRecoveryIncludesPausedCandidate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "recovery-project", "recovery-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	workerID := registerPlanningWorker(t, store, "recovery-worker")
	assignment, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "recovery-snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	artifactStore, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	_, err = store.ReportWorkerFor(ctx, workerID, workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
		Outcome: "succeeded", ExitCode: &exitCode, AfterRevision: change.BaseRevision,
		Artifacts: []workercontract.Artifact{workerReportArtifact("candidate", []byte(`{"kind":"understanding"}`), false)},
	}, artifactStore)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := store.ApplyCommand(ctx, change.ID, "pause", change.Version, "pause-planning-candidate", "test")
	if err != nil {
		t.Fatal(err)
	}
	recoverable, err := store.ListRecoverablePlanningChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoverable) != 1 || recoverable[0].ID != paused.ID || recoverable[0].Status != domain.ChangeStatusPaused || recoverable[0].LatestAgentRun == nil || recoverable[0].LatestAgentRun.ID != run.ID {
		t.Fatalf("paused candidate recovery = %+v", recoverable)
	}
}

func TestPlanningRecoveryExcludesPlanCheckpointWithoutCandidate(t *testing.T) {
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "plan-checkpoint-project", "plan-checkpoint-change")
	if _, err := store.db.Exec(`UPDATE t_changes SET stage = 'Plan' WHERE change_id = ?`, change.ID); err != nil {
		t.Fatal(err)
	}
	recoverable, err := store.ListRecoverablePlanningChanges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recoverable) != 0 {
		t.Fatalf("Plan checkpoint remained recoverable = %+v", recoverable)
	}
}

func TestPlanningSuccessfulReportRequiresCandidateButFailedReportMayOmitIt(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "candidate-required-project", "candidate-required-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	workerID := registerPlanningWorker(t, store, "candidate-required-worker")
	assignment, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "candidate-required", nil)
	if err != nil {
		t.Fatal(err)
	}
	artifactStore, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	successWithoutCandidate := workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
		Outcome: "succeeded", ExitCode: &exitCode,
		Artifacts: []workercontract.Artifact{workerReportArtifact("stdout", []byte("missing candidate"), false)},
	}
	if _, err := store.ReportWorkerFor(ctx, workerID, successWithoutCandidate, artifactStore); !errors.Is(err, ErrWorkerReportInvalid) {
		t.Fatalf("success without candidate error = %v", err)
	}
	successWithCaptureFailure := successWithoutCandidate
	successWithCaptureFailure.Artifacts = append(successWithCaptureFailure.Artifacts, workerReportArtifact("candidate", []byte(`{"kind":"understanding"}`), false))
	successWithCaptureFailure.CaptureFailures = []workercontract.CaptureFailure{{Kind: "stderr", Stage: "runtime-output", Error: "capture_failed"}}
	if _, err := store.ReportWorkerFor(ctx, workerID, successWithCaptureFailure, artifactStore); !errors.Is(err, ErrWorkerReportInvalid) {
		t.Fatalf("success with capture failure error = %v", err)
	}
	// Runtime 可以零退出，但独立证据采集失败仍必须以 failed Report 耐久收敛。
	exitCode = 0
	failed := successWithoutCandidate
	failed.Outcome = "failed"
	failed.ExitCode = &exitCode
	failed.FailureReason = "runtime_failed"
	failed.CaptureFailures = []workercontract.CaptureFailure{{Kind: "candidate", Stage: "candidate-output", Error: "capture_failed"}}
	failed.Artifacts = append(failed.Artifacts, workercontract.Artifact{Kind: "candidate", CaptureError: "capture_failed"})
	response, err := store.ReportWorkerFor(ctx, workerID, failed, artifactStore)
	if err != nil || response.Disposition != "candidate_received" {
		t.Fatalf("failed candidate-less report = %+v, err=%v", response, err)
	}
	candidate, err := store.FindPlanningRunCandidate(ctx, run.ID)
	if err != nil || candidate.CandidateRef != nil || len(candidate.RawLogRefs) != 1 || candidate.Outcome != domain.AgentRunOutcomeFailed {
		t.Fatalf("failed planning observation = %+v, err=%v", candidate, err)
	}
}

func TestPlanningExecutionLossBecomesDurableFailureCandidate(t *testing.T) {
	t.Run("expired lease", func(t *testing.T) {
		ctx := context.Background()
		store := newTestStore(t)
		_, change := createTestChange(t, store, t.TempDir(), "expired-project", "expired-change")
		run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
		workerID := registerPlanningWorker(t, store, "expired-worker")
		assignment, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "expired-snapshot", nil)
		if err != nil {
			t.Fatal(err)
		}
		oldNow := store.now
		store.now = func() time.Time { return oldNow().Add(workerLeaseTTL + time.Second) }
		if err := store.ReconcilePlanningExecutions(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := store.FindPlanningRunCandidate(ctx, run.ID); !errors.Is(err, domain.ErrPlanningCandidateNotFound) {
			t.Fatalf("expired lease candidate error = %v, want not found until Worker stops", err)
		}
		assertPlanningWorkspaceCleared(t, store, run.ID)
		assertNoLeaseTokens(t, store)
		artifactStore, err := artifact.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		exitCode := 0
		lateReport := workercontract.Report{
			AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
			Outcome: "succeeded", ExitCode: &exitCode, AfterRevision: change.BaseRevision,
			Artifacts: []workercontract.Artifact{
				workerReportArtifact("candidate", []byte(`{"kind":"understanding"}`), false),
				workerReportArtifact("stdout", []byte("late runtime evidence"), false),
			},
		}
		response, err := store.ReportWorkerFor(ctx, workerID, lateReport, artifactStore)
		if err != nil || response.Disposition != "late" || response.LeaseState != "expired" {
			t.Fatalf("expired planning report = %+v, err=%v", response, err)
		}
		candidate, err := store.FindPlanningRunCandidate(ctx, run.ID)
		if err != nil || candidate.Outcome != domain.AgentRunOutcomeFailed || candidate.FailureReason != "lease_expired" || candidate.CandidateRef == nil {
			t.Fatalf("fenced late candidate = %+v, err=%v", candidate, err)
		}
		afterReport, err := store.FindChange(ctx, change.ID)
		if err != nil {
			t.Fatal(err)
		}
		if afterReport.Status != domain.ChangeStatusActive || afterReport.LatestAgentRun == nil || afterReport.LatestAgentRun.Status != domain.AgentRunStatusRunning {
			t.Fatalf("late report changed authority before Coordinator: %+v", afterReport)
		}
		events, err := store.ListChangeEvents(ctx, change.ID)
		if err != nil || len(events) == 0 || events[len(events)-1].Type != domain.AgentRunReportLateType {
			t.Fatalf("late planning events = %+v, err=%v", events, err)
		}
		failed, err := store.FailPlanningStage(ctx, work.FailPlanningStageRequest{
			AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt,
			ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: "planning",
			Failure: work.PlanningArtifactWrite{
				Identity: domain.NewArtifactIdentity([]byte("lease expired")), MediaType: "application/json",
				Kind: "planning_failure", Summary: "lease_expired", SourceRevision: change.BaseRevision,
				InputArtifactRefIDs: runInputRefIDs(run),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		afterLate, err := store.FindChange(ctx, change.ID)
		if err != nil || afterLate.Status != failed.Change.Status || afterLate.Stage != failed.Change.Stage || afterLate.Version != failed.Change.Version {
			t.Fatalf("settled late report = %+v, err=%v", afterLate, err)
		}
		var receipts int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM t_worker_reports WHERE agent_run_id = ? AND disposition = 'late'`, run.ID).Scan(&receipts); err != nil || receipts != 1 {
			t.Fatalf("late planning receipts = %d, err=%v", receipts, err)
		}
		duplicate, err := store.ReportWorkerFor(ctx, workerID, lateReport, artifactStore)
		if err != nil || duplicate.Disposition != "duplicate" {
			t.Fatalf("duplicate late planning report = %+v, err=%v", duplicate, err)
		}
		conflictingReport := lateReport
		conflictingReport.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		conflict, err := store.ReportWorkerFor(ctx, workerID, conflictingReport, artifactStore)
		if err != nil || conflict.Disposition != "terminal_conflict" {
			t.Fatalf("conflicting late planning report = %+v, err=%v", conflict, err)
		}
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM t_worker_reports WHERE agent_run_id = ?`, run.ID).Scan(&receipts); err != nil || receipts != 1 {
			t.Fatalf("terminal lease accepted extra reports = %d, err=%v", receipts, err)
		}
	})

	t.Run("worker lost", func(t *testing.T) {
		ctx := context.Background()
		store := newTestStore(t)
		_, change := createTestChange(t, store, t.TempDir(), "lost-project", "lost-change")
		run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
		workerID := registerPlanningWorker(t, store, "lost-worker")
		if _, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "lost-snapshot", nil); err != nil {
			t.Fatal(err)
		}
		oldNow := store.now
		store.now = func() time.Time { return oldNow().Add(workerLeaseTTL + time.Second) }
		if err := store.ReconcilePlanningExecutions(ctx); err != nil {
			t.Fatal(err)
		}
		if err := store.ReconcileWorkerLost(ctx, workerID); err != nil {
			t.Fatal(err)
		}
		assertPlanningFailureCandidate(t, store, change.ID, run.ID, "worker_lost")
		assertPlanningWorkspaceCleared(t, store, run.ID)
		assertNoLeaseTokens(t, store)
	})

	t.Run("idle heartbeat acknowledges an unpulled expired lease", func(t *testing.T) {
		ctx := context.Background()
		store := newTestStore(t)
		_, change := createTestChange(t, store, t.TempDir(), "idle-project", "idle-change")
		run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
		workerID := registerPlanningWorker(t, store, "idle-worker")
		if _, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "idle-snapshot", nil); err != nil {
			t.Fatal(err)
		}
		oldNow := store.now
		store.now = func() time.Time { return oldNow().Add(workerLeaseTTL + time.Second) }
		if err := store.ReconcilePlanningExecutions(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := store.FindPlanningRunCandidate(ctx, run.ID); !errors.Is(err, domain.ErrPlanningCandidateNotFound) {
			t.Fatalf("expired lease candidate error = %v, want no candidate before idle acknowledgement", err)
		}
		response, err := store.HeartbeatWorker(ctx, workercontract.Heartbeat{WorkerID: workerID})
		if err != nil || !response.WorkerAvailable {
			t.Fatalf("idle heartbeat = %+v, err=%v", response, err)
		}
		assertPlanningFailureCandidate(t, store, change.ID, run.ID, "lease_expired")
		assertPlanningWorkspaceCleared(t, store, run.ID)
		assertNoLeaseTokens(t, store)
	})

	t.Run("daemon restarted before assignment", func(t *testing.T) {
		ctx := context.Background()
		store := newTestStore(t)
		_, change := createTestChange(t, store, t.TempDir(), "restart-project", "restart-change")
		run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
		if err := store.ReconcileWorkerRestart(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := store.FindPlanningRunCandidate(ctx, run.ID); !errors.Is(err, domain.ErrPlanningCandidateNotFound) {
			t.Fatalf("unassigned restart candidate error = %v, want recoverable run", err)
		}
		if assigned, err := store.PlanningRunAssigned(ctx, run.ID); err != nil || assigned {
			t.Fatalf("unassigned restart state = %t, err=%v", assigned, err)
		}
	})

	t.Run("daemon restarted after assignment", func(t *testing.T) {
		ctx := context.Background()
		store := newTestStore(t)
		_, change := createTestChange(t, store, t.TempDir(), "assigned-restart-project", "assigned-restart-change")
		run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
		workerID := registerPlanningWorker(t, store, "assigned-restart-worker")
		if _, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "assigned-restart-snapshot", nil); err != nil {
			t.Fatal(err)
		}
		if err := store.ReconcileWorkerRestart(ctx); err != nil {
			t.Fatal(err)
		}
		assertPlanningFailureCandidate(t, store, change.ID, run.ID, "daemon_restarted")
	})
}

func TestPlanningReportCrossingLeaseDeadlineIsLateAndFenced(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	_, change := createTestChange(t, store, t.TempDir(), "slow-report-project", "slow-report-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	workerID := registerPlanningWorker(t, store, "slow-report-worker")
	assignment, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "slow-report-snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	report := workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
		Outcome: "succeeded", ExitCode: &exitCode, AfterRevision: change.BaseRevision,
		Artifacts: []workercontract.Artifact{workerReportArtifact("candidate", []byte(`{"kind":"understanding"}`), false)},
	}
	artifacts := advancingWorkerArtifactStore{advance: func() { now = now.Add(workerLeaseTTL + time.Second) }}
	response, err := store.ReportWorkerFor(ctx, workerID, report, artifacts)
	if err != nil || response.Disposition != "late" || response.LeaseState != "expired" {
		t.Fatalf("slow planning report = %+v, err=%v", response, err)
	}
	candidate, err := store.FindPlanningRunCandidate(ctx, run.ID)
	if err != nil || candidate.Outcome != domain.AgentRunOutcomeFailed || candidate.FailureReason != "lease_expired" {
		t.Fatalf("slow planning candidate = %+v, err=%v", candidate, err)
	}
}

func TestPlanningArtifactStoreFailureBecomesDurableCandidate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "artifact-failure-project", "artifact-failure-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	workerID := registerPlanningWorker(t, store, "artifact-failure-worker")
	assignment, err := store.IssuePlanningAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "produce understanding", change.BaseRevision, "artifact-failure-snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	report := workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
		Outcome: "succeeded", ExitCode: &exitCode, AfterRevision: change.BaseRevision,
		Artifacts: []workercontract.Artifact{workerReportArtifact("candidate", []byte(`{"kind":"understanding"}`), false)},
	}
	response, err := store.ReportWorkerFor(ctx, workerID, report, failingWorkerArtifactStore{})
	if err != nil || response.Disposition != "candidate_received" || response.LeaseState != "consumed" {
		t.Fatalf("artifact failure response = %+v, err=%v", response, err)
	}
	assertPlanningFailureCandidate(t, store, change.ID, run.ID, "artifact_unavailable")
	duplicate, err := store.ReportWorkerFor(ctx, workerID, report, failingWorkerArtifactStore{})
	if err != nil || duplicate.Disposition != "duplicate" {
		t.Fatalf("artifact failure replay = %+v, err=%v", duplicate, err)
	}
}

func TestMalformedWorkerArtifactIsPermanentReportError(t *testing.T) {
	report := workercontract.Report{Artifacts: []workercontract.Artifact{{
		Kind: "candidate", ContentBase64: "not-base64", SHA256: strings.Repeat("0", 64), SizeBytes: 1,
	}}}
	if _, err := prepareReportArtifacts(report); !errors.Is(err, ErrWorkerReportInvalid) || errors.Is(err, ErrWorkerUnavailable) {
		t.Fatalf("prepareReportArtifacts() error = %v, want permanent report-invalid", err)
	}
}

type advancingWorkerArtifactStore struct {
	advance func()
}

type failingWorkerArtifactStore struct{}

func (failingWorkerArtifactStore) Put(context.Context, []byte) (domain.ArtifactIdentity, error) {
	return domain.ArtifactIdentity{}, errors.New("injected artifact store failure")
}

func (s advancingWorkerArtifactStore) Put(_ context.Context, content []byte) (domain.ArtifactIdentity, error) {
	if s.advance != nil {
		s.advance()
	}
	return domain.NewArtifactIdentity(content), nil
}

func assertPlanningWorkspaceCleared(t *testing.T, store *Store, runID domain.AgentRunID) {
	t.Helper()
	var workspacePath string
	if err := store.db.QueryRow(`SELECT workspace_path FROM t_worker_leases WHERE agent_run_id = ?`, runID).Scan(&workspacePath); err != nil {
		t.Fatal(err)
	}
	if workspacePath != "" {
		t.Fatalf("terminal planning lease retained workspace path")
	}
}

func assertPlanningFailureCandidate(t *testing.T, store *Store, changeID domain.ChangeID, runID domain.AgentRunID, reason string) {
	t.Helper()
	candidate, err := store.FindPlanningRunCandidate(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Outcome != domain.AgentRunOutcomeFailed || candidate.FailureReason != reason || candidate.CandidateRef != nil {
		t.Fatalf("planning failure candidate = %+v, want failed/%s without payload", candidate, reason)
	}
	change, err := store.FindChange(context.Background(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	if change.Status != domain.ChangeStatusActive || change.LatestAgentRun == nil || change.LatestAgentRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("system candidate changed authority before Coordinator = %+v", change)
	}
}

func TestLegacyWorkerCaptureFailureRemainsRetriable(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "legacy-capture-project", "legacy-capture-change")
	run, err := store.StartAgentRun(ctx, change.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	workerID := registerPlanningWorker(t, store, "legacy-capture-worker")
	assignment, err := store.IssueAssignment(ctx, run.ID, workerID, t.TempDir(), "codex", "execute", change.BaseRevision, "legacy-capture", nil)
	if err != nil {
		t.Fatal(err)
	}
	artifactStore, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 1
	_, err = store.ReportWorkerFor(ctx, workerID, workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: assignment.LeaseToken, Attempt: run.Attempt,
		Outcome: "failed", ExitCode: &exitCode, FailureReason: "capture_failed",
		CaptureFailures: []workercontract.CaptureFailure{{Kind: "stdout", Stage: "runtime-output", Error: "capture_failed"}},
	}, artifactStore)
	if !errors.Is(err, ErrWorkerUnavailable) {
		t.Fatalf("legacy capture failure error = %v", err)
	}
	stored, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LatestAgentRun == nil || stored.LatestAgentRun.Status != domain.AgentRunStatusRunning || stored.Status != domain.ChangeStatusActive {
		t.Fatalf("legacy capture failure changed authority = %+v", stored)
	}
}

func TestPlanningMigrationPreservesLegacyArtifactRefs(t *testing.T) {
	db, err := sql.Open("sqlite", "file:planning-migration-"+filepath.Base(t.TempDir())+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	migrations := Migrations()
	if err := migration.NewRunner(append(migration.DefaultMigrations(), migrations[:3]...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	_, change := createTestChange(t, store, "/tmp/planning-legacy", "planning-legacy-project", "planning-legacy-change")
	if err := migration.NewRunner(append(migration.DefaultMigrations(), migrations...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	refs, err := store.ListArtifactRefs(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || !refs[0].IsLegacy() {
		t.Fatalf("migrated legacy refs = %+v", refs)
	}
	if migrations[len(migrations)-1].Version != 5 {
		t.Fatalf("planning migration version = %d", migrations[len(migrations)-1].Version)
	}
}

func startPlanningRun(t *testing.T, store *Store, change domain.Change, stage domain.LifecycleStage, inputRef domain.ArtifactRefID) domain.AgentRun {
	t.Helper()
	run, err := store.StartPlanningRun(context.Background(), work.StartPlanningRunRequest{
		ChangeID: change.ID, TargetStage: stage, ExpectedChangeVersion: change.Version,
		SourceRevision: change.BaseRevision, Actor: "planning", InputArtifactRefIDs: []domain.ArtifactRefID{inputRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func successfulPlanningCompletion(change domain.Change, run domain.AgentRun, kind, schema, summary string) work.CompletePlanningStageRequest {
	return work.CompletePlanningStageRequest{
		AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt,
		ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: "planning",
		Artifact: work.PlanningArtifactWrite{
			Identity: domain.NewArtifactIdentity([]byte(kind)), MediaType: "application/json",
			Kind: kind, SchemaVersion: schema, Summary: summary, SourceRevision: change.BaseRevision,
			InputArtifactRefIDs: runInputRefIDs(run),
		},
	}
}

func runInputRefIDs(run domain.AgentRun) []domain.ArtifactRefID {
	refs := make([]domain.ArtifactRefID, 0)
	for _, artifact := range run.Artifacts {
		if artifact.Role == domain.ArtifactRoleInput {
			refs = append(refs, artifact.ArtifactRefID)
		}
	}
	return refs
}

func assertPlanningTrace(t *testing.T, store *Store, changeID domain.ChangeID, want int, lastType string) {
	t.Helper()
	events, err := store.ListChangeEvents(context.Background(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != want || events[len(events)-1].Type != lastType {
		t.Fatalf("planning events = %+v", events)
	}
}

func registerPlanningWorker(t *testing.T, store *Store, workerID string) string {
	t.Helper()
	secret, err := NewWorkerSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareWorker(context.Background(), workerID, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RegisterWorker(context.Background(), workercontract.Register{WorkerID: workerID, ProtocolVersion: workercontract.ProtocolVersionV1, Capabilities: []string{"runtime:codex"}}); err != nil {
		t.Fatal(err)
	}
	return workerID
}

func workerReportArtifact(kind string, content []byte, truncated bool) workercontract.Artifact {
	identity := domain.NewArtifactIdentity(content)
	return workercontract.Artifact{Kind: kind, ContentBase64: base64.StdEncoding.EncodeToString(content), SHA256: identity.SHA256, SizeBytes: identity.ByteLength, Truncated: truncated}
}

func TestPlanningStartRejectsRevisionAndStageFence(t *testing.T) {
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/planning-fence", "fence-project", "fence-change")
	cases := []work.StartPlanningRunRequest{
		{ChangeID: change.ID, TargetStage: domain.LifecycleStageDesign, ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: "planning", InputArtifactRefIDs: []domain.ArtifactRefID{change.Intent.ID}},
		{ChangeID: change.ID, TargetStage: domain.LifecycleStageUnderstand, ExpectedChangeVersion: change.Version, SourceRevision: strings.Repeat("f", 40), Actor: "planning", InputArtifactRefIDs: []domain.ArtifactRefID{change.Intent.ID}},
	}
	for _, request := range cases {
		if _, err := store.StartPlanningRun(context.Background(), request); !errors.Is(err, domain.ErrPlanningRunConflict) {
			t.Fatalf("planning fence request %+v error = %v", request, err)
		}
	}
}

func TestPlanningCompletionRejectsRunInputMismatch(t *testing.T) {
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/planning-input-fence", "input-fence-project", "input-fence-change")
	run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
	request := successfulPlanningCompletion(change, run, "understanding", "Understanding.v1", "understood")
	request.Artifact.InputArtifactRefIDs = nil
	if _, err := store.CompletePlanningStage(context.Background(), request); !errors.Is(err, domain.ErrPlanningRunConflict) {
		t.Fatalf("planning input mismatch error = %v", err)
	}
	stored, err := store.FindChange(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Stage != domain.LifecycleStageIntent || stored.LatestAgentRun == nil || stored.LatestAgentRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("planning input mismatch changed authority = %+v", stored)
	}
}

func TestPlanningAssignmentIsFencedAfterPauseAndCancel(t *testing.T) {
	t.Run("pause", func(t *testing.T) {
		store := newTestStore(t)
		_, change := createTestChange(t, store, t.TempDir(), "assignment-pause-project", "assignment-pause-change")
		run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
		if _, err := store.ApplyCommand(context.Background(), change.ID, "pause", change.Version, "assignment-pause-key", "test"); err != nil {
			t.Fatal(err)
		}
		workerID := registerPlanningWorker(t, store, "assignment-pause-worker")
		if _, err := store.IssuePlanningAssignment(context.Background(), run.ID, workerID, t.TempDir(), "codex", "observe", change.BaseRevision, "pause-fenced", nil); !errors.Is(err, ErrWorkerAssignmentConflict) {
			t.Fatalf("paused planning assignment error = %v, want conflict", err)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		store := newTestStore(t)
		_, change := createTestChange(t, store, t.TempDir(), "assignment-cancel-project", "assignment-cancel-change")
		run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
		if _, err := store.ApplyCommand(context.Background(), change.ID, "cancel", change.Version, "assignment-cancel-key", "test"); err != nil {
			t.Fatal(err)
		}
		workerID := registerPlanningWorker(t, store, "assignment-cancel-worker")
		if _, err := store.IssuePlanningAssignment(context.Background(), run.ID, workerID, t.TempDir(), "codex", "observe", change.BaseRevision, "cancel-fenced", nil); !errors.Is(err, ErrWorkerAssignmentConflict) {
			t.Fatalf("cancelled planning assignment error = %v, want conflict", err)
		}
	})
}

func TestPlanningCompletionIsDeferredAfterPauseAndVersionFence(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{name: "paused", command: "pause"},
		{name: "resumed version changed", command: "pause-resume"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newTestStore(t)
			_, change := createTestChange(t, store, t.TempDir(), "completion-fence-project", "completion-fence-"+test.name)
			run := startPlanningRun(t, store, change, domain.LifecycleStageUnderstand, change.Intent.ID)
			if _, err := store.ApplyCommand(context.Background(), change.ID, "pause", change.Version, "completion-pause-"+test.name, "test"); err != nil {
				t.Fatal(err)
			}
			if test.command == "pause-resume" {
				paused, err := store.FindChange(context.Background(), change.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.ApplyCommand(context.Background(), change.ID, "resume", paused.Version, "completion-resume-"+test.name, "test"); err != nil {
					t.Fatal(err)
				}
			}
			result, err := store.CompletePlanningStage(context.Background(), successfulPlanningCompletion(change, run, "understanding", "Understanding.v1", "deferred"))
			if !errors.Is(err, domain.ErrPlanningRunDeferred) || result.Disposition != work.PlanningCommitFenced {
				t.Fatalf("deferred completion result=%+v err=%v", result, err)
			}
			stored, err := store.FindChange(context.Background(), change.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.LatestAgentRun == nil || stored.LatestAgentRun.Status != domain.AgentRunStatusRunning || stored.Stage != domain.LifecycleStageIntent {
				t.Fatalf("deferred completion changed authority = %+v", stored)
			}
		})
	}
}

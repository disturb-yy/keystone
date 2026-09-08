package planning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestCoordinatorRunsPlanningStagesSeriallyAndStopsAtTicketize(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	stages := []Stage{StageUnderstand, StageDesign, StagePlan}
	checkpoints := []domain.LifecycleStage{
		domain.LifecycleStageUnderstand,
		domain.LifecycleStageDesign,
		domain.LifecycleStageTicketize,
	}

	for index, stage := range stages {
		fixture.recover(t)
		run := fixture.state.latestRun(t)
		if run.Stage != lifecycleStage(stage) || run.Status != domain.AgentRunStatusRunning {
			t.Fatalf("run %d = %#v, want running %s", index, run, stage)
		}
		if got := len(fixture.dispatcher.requests); got != index+1 {
			t.Fatalf("dispatch count after starting %s = %d, want %d", stage, got, index+1)
		}

		fixture.reportCandidate(t, run, validCandidateJSON(t, stage, testRevision, string(run.Artifacts[0].ArtifactRefID)))
		fixture.recover(t)
		if got := fixture.state.change.Stage; got != checkpoints[index] {
			t.Fatalf("checkpoint after %s = %s, want %s", stage, got, checkpoints[index])
		}
	}

	fixture.recover(t)
	if got := len(fixture.dispatcher.requests); got != len(stages) {
		t.Fatalf("dispatch count after Ticketize = %d, want %d", got, len(stages))
	}
	if fixture.state.change.Status != domain.ChangeStatusActive {
		t.Fatalf("status after Plan = %s, want active", fixture.state.change.Status)
	}
	if got := len(fixture.state.completeRequests); got != len(stages) {
		t.Fatalf("completion count = %d, want %d", got, len(stages))
	}
	if got := fixture.snapshots.closedCount(); got != len(stages) {
		t.Fatalf("closed snapshot count = %d, want %d", got, len(stages))
	}

	assertProjectContextAndRevisionChain(t, fixture)
}

func TestCoordinatorRecoverDoesNotRedispatchRunningAttempt(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.recover(t)
	firstRun := fixture.state.latestRun(t)

	fixture.recover(t)
	secondRun := fixture.state.latestRun(t)
	if secondRun.ID != firstRun.ID {
		t.Fatalf("second Recover run ID = %s, want %s", secondRun.ID, firstRun.ID)
	}
	if got := len(fixture.state.startRequests); got != 1 {
		t.Fatalf("StartPlanningRun calls = %d, want 1", got)
	}
	if got := len(fixture.dispatcher.requests); got != 1 {
		t.Fatalf("Dispatch calls = %d, want 1", got)
	}
	if got := len(fixture.snapshots.created); got != 1 {
		t.Fatalf("Materialize calls = %d, want 1", got)
	}
}

func TestCoordinatorRedispatchesSameAttemptAfterPauseFencesAssignment(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.dispatcher.before = func() { fixture.state.setStatus(domain.ChangeStatusPaused) }
	fixture.dispatcher.err = ErrDispatchFenced

	fixture.recover(t)
	run := fixture.state.latestRun(t)
	if run.Status != domain.AgentRunStatusRunning || len(fixture.state.failRequests) != 0 {
		t.Fatalf("paused pre-dispatch run = %+v, failures=%d", run, len(fixture.state.failRequests))
	}
	if fixture.snapshots.closedCount() != 1 {
		t.Fatalf("paused pre-dispatch snapshots closed = %d, want 1", fixture.snapshots.closedCount())
	}

	fixture.dispatcher.before = nil
	fixture.dispatcher.err = nil
	fixture.state.setStatus(domain.ChangeStatusActive)
	fixture.recover(t)
	resumed := fixture.state.latestRun(t)
	if resumed.ID != run.ID || resumed.Attempt != run.Attempt {
		t.Fatalf("resumed run = %+v, want same attempt %+v", resumed, run)
	}
	if len(fixture.dispatcher.requests) != 2 || !fixture.dispatcher.assigned[run.ID] {
		t.Fatalf("resumed dispatches = %d, assigned=%t", len(fixture.dispatcher.requests), fixture.dispatcher.assigned[run.ID])
	}
	if len(fixture.state.failRequests) != 0 {
		t.Fatalf("Pause/Resume manufactured %d planning failures", len(fixture.state.failRequests))
	}
}

func TestCoordinatorRecoversPausedUnassignedAttemptAfterRestart(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.dispatcher.before = func() { fixture.state.setStatus(domain.ChangeStatusPaused) }
	fixture.dispatcher.err = ErrDispatchFenced
	fixture.recover(t)
	run := fixture.state.latestRun(t)

	fixture.dispatcher.before = nil
	fixture.dispatcher.err = nil
	fixture.state.setStatus(domain.ChangeStatusActive)
	coordinator, err := NewCoordinator(fixture.state, fixture.artifacts, fixture.snapshots, fixture.dispatcher)
	if err != nil {
		t.Fatal(err)
	}
	fixture.coordinator = coordinator
	fixture.recover(t)

	resumed := fixture.state.latestRun(t)
	if resumed.ID != run.ID || resumed.Attempt != run.Attempt || len(fixture.dispatcher.requests) != 2 {
		t.Fatalf("restart resume = %+v dispatches=%d, want same attempt %+v and one redispatch", resumed, len(fixture.dispatcher.requests), run)
	}
}

func TestCoordinatorCompletesCancelledUnassignedAttemptAsFencedFact(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.dispatcher.before = func() { fixture.state.setStatus(domain.ChangeStatusCancelled) }
	fixture.dispatcher.err = ErrDispatchFenced
	fixture.recover(t)
	fixture.recover(t)

	run := fixture.state.latestRun(t)
	if fixture.state.change.Status != domain.ChangeStatusCancelled || fixture.state.change.Stage != domain.LifecycleStageIntent {
		t.Fatalf("cancelled change = %+v", fixture.state.change)
	}
	if run.Status != domain.AgentRunStatusCompleted || run.Outcome != domain.AgentRunOutcomeFailed {
		t.Fatalf("cancelled unassigned run = %+v, want terminal failed fact", run)
	}
	if got := fixture.state.completeDispositions[len(fixture.state.completeDispositions)-1]; got != work.PlanningCommitFenced {
		t.Fatalf("cancelled completion disposition = %q, want fenced", got)
	}
	if got := len(fixture.dispatcher.requests); got != 1 {
		t.Fatalf("cancelled dispatch count = %d, want 1", got)
	}
	fixture.recover(t)
	if got := len(fixture.state.failRequests); got != 1 {
		t.Fatalf("cancelled failure commits = %d, want idempotent one", got)
	}
}

func TestCoordinatorDurablyRetainsPredispatchFailureAcrossPauseAndRestart(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.dispatcher.before = func() { fixture.state.setStatus(domain.ChangeStatusPaused) }
	fixture.dispatcher.err = errors.New("injected dispatch failure")
	fixture.recover(t)
	run := fixture.state.latestRun(t)
	if _, ok := fixture.state.candidates[run.ID]; !ok {
		t.Fatal("pre-dispatch failure was not recorded as a durable candidate")
	}

	fixture.dispatcher.before = nil
	fixture.dispatcher.err = nil
	fixture.state.setStatus(domain.ChangeStatusActive)
	coordinator, err := NewCoordinator(fixture.state, fixture.artifacts, fixture.snapshots, fixture.dispatcher)
	if err != nil {
		t.Fatal(err)
	}
	fixture.coordinator = coordinator
	fixture.recover(t)

	completed := fixture.state.latestRun(t)
	if completed.ID != run.ID || completed.Status != domain.AgentRunStatusCompleted || completed.Outcome != domain.AgentRunOutcomeFailed {
		t.Fatalf("restarted failed run = %+v, want original attempt failed", completed)
	}
	if fixture.state.change.Status != domain.ChangeStatusHumanRequired || len(fixture.dispatcher.requests) != 1 {
		t.Fatalf("restarted failure status=%s dispatches=%d", fixture.state.change.Status, len(fixture.dispatcher.requests))
	}
}

func TestCoordinatorDoesNotRedispatchWhenAssignmentCommittedBeforeDispatchError(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.dispatcher.assignBeforeError = true
	fixture.dispatcher.err = errors.New("response lost after durable assignment")
	fixture.recover(t)
	run := fixture.state.latestRun(t)
	fixture.recover(t)

	if !fixture.dispatcher.assigned[run.ID] || len(fixture.dispatcher.requests) != 1 {
		t.Fatalf("assigned=%t dispatches=%d, want one durable assignment", fixture.dispatcher.assigned[run.ID], len(fixture.dispatcher.requests))
	}
	if len(fixture.state.failRequests) != 0 || fixture.state.latestRun(t).Status != domain.AgentRunStatusRunning {
		t.Fatalf("committed assignment was failed: requests=%d run=%+v", len(fixture.state.failRequests), fixture.state.latestRun(t))
	}
}

func TestCoordinatorRetriesPredispatchFailureSettlement(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.dispatcher.err = errors.New("injected dispatch failure")
	fixture.state.failErrors = []error{errors.New("injected store failure")}

	if err := fixture.coordinator.Recover(context.Background()); err == nil {
		t.Fatal("first Recover() unexpectedly hid the failed failure settlement")
	}
	if got := fixture.state.latestRun(t).Status; got != domain.AgentRunStatusRunning {
		t.Fatalf("run after failed settlement = %s, want running", got)
	}
	if got := fixture.snapshots.closedCount(); got != 1 {
		t.Fatalf("snapshot close count after dispatch failure = %d, want 1", got)
	}

	fixture.dispatcher.err = nil
	fixture.recover(t)
	if got := fixture.state.change.Status; got != domain.ChangeStatusHumanRequired {
		t.Fatalf("status after settlement retry = %s, want human_required", got)
	}
	if got := fixture.state.latestRun(t).Outcome; got != domain.AgentRunOutcomeFailed {
		t.Fatalf("run outcome after settlement retry = %s, want failed", got)
	}
	if got := len(fixture.dispatcher.requests); got != 1 {
		t.Fatalf("dispatch attempts = %d, want no redispatch", got)
	}
}

func TestCoordinatorStrictCandidateFailureRequiresHumanDecision(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.recover(t)
	run := fixture.state.latestRun(t)
	fixture.reportCandidate(t, run, []byte(`{"kind":"understanding","unknown":true}`))

	fixture.recover(t)
	if got := fixture.state.change.Status; got != domain.ChangeStatusHumanRequired {
		t.Fatalf("status after strict decode failure = %s, want human_required", got)
	}
	if got := fixture.state.change.Stage; got != domain.LifecycleStageIntent {
		t.Fatalf("stage after strict decode failure = %s, want Intent", got)
	}
	completed := fixture.state.latestRun(t)
	if completed.Status != domain.AgentRunStatusCompleted || completed.Outcome != domain.AgentRunOutcomeFailed {
		t.Fatalf("failed run = %#v", completed)
	}
	if got := len(fixture.state.failRequests); got != 1 {
		t.Fatalf("FailPlanningStage calls = %d, want 1", got)
	}
	failure := fixture.state.failRequests[0].Failure
	if failure.Summary != string(ErrorClassDecodeInvalid) {
		t.Fatalf("failure class = %q, want %q", failure.Summary, ErrorClassDecodeInvalid)
	}
	if !reflect.DeepEqual(failure.RawLogArtifactRefIDs, []domain.ArtifactRefID{fixture.state.candidates[run.ID].CandidateRef.ID}) {
		t.Fatalf("failure evidence = %v, want candidate ref", failure.RawLogArtifactRefIDs)
	}

	fixture.recover(t)
	if got := len(fixture.dispatcher.requests); got != 1 {
		t.Fatalf("dispatch count after human_required recovery = %d, want 1", got)
	}
}

func TestCoordinatorSettlesExpiredPlanningExecution(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.recover(t)
	run := fixture.state.latestRun(t)
	fixture.state.candidates[run.ID] = work.PlanningRunCandidate{
		AgentRunID: run.ID, Outcome: domain.AgentRunOutcomeFailed,
		FailureReason: "lease_expired", AfterRevision: run.SourceRevision,
	}

	fixture.recover(t)
	if fixture.state.change.Status != domain.ChangeStatusHumanRequired || fixture.state.change.Stage != domain.LifecycleStageIntent {
		t.Fatalf("expired execution change = %+v", fixture.state.change)
	}
	completed := fixture.state.latestRun(t)
	if completed.Status != domain.AgentRunStatusCompleted || completed.Outcome != domain.AgentRunOutcomeFailed {
		t.Fatalf("expired execution run = %+v", completed)
	}
	if len(fixture.state.failRequests) != 1 || fixture.state.failRequests[0].Failure.Summary != "lease_expired" {
		t.Fatalf("expired execution failure = %+v", fixture.state.failRequests)
	}
}

func TestCandidateFailureClassPreservesGuardCaptureAndRevisionEvidence(t *testing.T) {
	change := domain.Change{BaseRevision: testRevision}
	run := domain.AgentRun{ID: "run-1"}
	exitCode := 0
	tests := []struct {
		name      string
		candidate work.PlanningRunCandidate
		want      string
	}{
		{name: "guard", candidate: work.PlanningRunCandidate{AgentRunID: run.ID, Outcome: domain.AgentRunOutcomeFailed, ExitCode: &exitCode, FailureReason: "guard_violation", GuardFindings: []string{"planning_snapshot_modified"}, AfterRevision: testRevision}, want: "guard_violation"},
		{name: "capture", candidate: work.PlanningRunCandidate{AgentRunID: run.ID, Outcome: domain.AgentRunOutcomeFailed, ExitCode: &exitCode, FailureReason: "capture_failed", AfterRevision: testRevision}, want: "capture_failed"},
		{name: "revision", candidate: work.PlanningRunCandidate{AgentRunID: run.ID, Outcome: domain.AgentRunOutcomeFailed, ExitCode: &exitCode, FailureReason: "guard_violation", AfterRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, want: string(ErrorClassRevisionMismatch)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := candidateFailureClass(change, run, test.candidate); got != test.want {
				t.Fatalf("candidateFailureClass() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCoordinatorRetriesSnapshotCloseFailure(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	runID := domain.AgentRunID("run-close-retry")
	snapshot := &fakePlanningSnapshot{root: "/tmp/planning-snapshot-close-retry", revision: testRevision, closeFailures: 1}
	fixture.coordinator.rememberSnapshot(runID, fixture.state.change.ID, snapshot)

	if err := fixture.coordinator.releaseSnapshot(runID); err == nil {
		t.Fatal("first releaseSnapshot() unexpectedly succeeded")
	}
	if _, retained := fixture.coordinator.owned[runID]; !retained {
		t.Fatal("failed snapshot cleanup was not retained for retry")
	}
	if err := fixture.coordinator.releaseSnapshot(runID); err != nil {
		t.Fatal(err)
	}
	if _, retained := fixture.coordinator.owned[runID]; retained || !snapshot.closed {
		t.Fatalf("snapshot retry retained=%t closed=%t", retained, snapshot.closed)
	}
}

func TestCoordinatorHumanRetryReusesImmutableInputs(t *testing.T) {
	fixture := newCoordinatorFixture(t)
	fixture.recover(t)
	first := fixture.state.latestRun(t)
	fixture.reportCandidate(t, first, []byte(`{"kind":"understanding","unknown":true}`))
	fixture.recover(t)

	contextRefs := fixture.state.refsByKind(planningContextKind)
	if len(contextRefs) != 1 {
		t.Fatalf("ProjectContext refs after failure = %d, want 1", len(contextRefs))
	}
	fixture.state.setStatus(domain.ChangeStatusActive)
	fixture.recover(t)

	retry := fixture.state.latestRun(t)
	if retry.Attempt != 2 || retry.ID == first.ID {
		t.Fatalf("retry run = %#v, want a distinct second attempt", retry)
	}
	wantInputs := []domain.ArtifactRefID{fixture.state.change.Intent.ID, contextRefs[0].ID}
	if got := inputRefs(retry); !reflect.DeepEqual(got, wantInputs) {
		t.Fatalf("retry inputs = %v, want %v", got, wantInputs)
	}
	if got := len(fixture.state.refsByKind(planningContextKind)); got != 1 {
		t.Fatalf("ProjectContext refs after retry = %d, want immutable reuse", got)
	}
	if got := len(fixture.dispatcher.requests); got != 2 {
		t.Fatalf("dispatch count after retry = %d, want 2", got)
	}
}

func TestCoordinatorFencesPausedAndCancelledCandidates(t *testing.T) {
	t.Run("paused candidate is retained until resume", func(t *testing.T) {
		fixture := newCoordinatorFixture(t)
		fixture.recover(t)
		run := fixture.state.latestRun(t)
		fixture.reportCandidate(t, run, validCandidateJSON(t, StageUnderstand, testRevision, string(run.Artifacts[0].ArtifactRefID)))
		fixture.state.setStatus(domain.ChangeStatusPaused)

		fixture.recover(t)
		if got := len(fixture.state.completeRequests); got != 0 {
			t.Fatalf("paused completion calls = %d, want 0", got)
		}
		if got := fixture.state.change.Stage; got != domain.LifecycleStageIntent {
			t.Fatalf("paused checkpoint = %s, want Intent", got)
		}
		if got := fixture.state.latestRun(t).Status; got != domain.AgentRunStatusRunning {
			t.Fatalf("paused run status = %s, want running for resume re-evaluation", got)
		}
		if got := fixture.snapshots.closedCount(); got != 1 {
			t.Fatalf("paused snapshot close count = %d, want 1", got)
		}

		fixture.state.setStatus(domain.ChangeStatusActive)
		fixture.recover(t)
		if got := fixture.state.change.Stage; got != domain.LifecycleStageUnderstand {
			t.Fatalf("resumed checkpoint = %s, want Understand", got)
		}
		if got := len(fixture.state.completeRequests); got != 1 {
			t.Fatalf("resumed completion calls = %d, want 1", got)
		}
		if got := len(fixture.dispatcher.requests); got != 1 {
			t.Fatalf("resumed dispatch count = %d, want original attempt only", got)
		}
	})

	t.Run("cancelled candidate is durable but cannot advance", func(t *testing.T) {
		fixture := newCoordinatorFixture(t)
		fixture.recover(t)
		run := fixture.state.latestRun(t)
		fixture.reportCandidate(t, run, validCandidateJSON(t, StageUnderstand, testRevision, string(run.Artifacts[0].ArtifactRefID)))
		fixture.state.setStatus(domain.ChangeStatusCancelled)

		fixture.recover(t)
		if got := fixture.state.change.Status; got != domain.ChangeStatusCancelled {
			t.Fatalf("cancelled status = %s, want cancelled", got)
		}
		if got := fixture.state.change.Stage; got != domain.LifecycleStageIntent {
			t.Fatalf("cancelled checkpoint = %s, want Intent", got)
		}
		completed := fixture.state.latestRun(t)
		if completed.Status != domain.AgentRunStatusCompleted || completed.Outcome != domain.AgentRunOutcomeSucceeded {
			t.Fatalf("cancelled candidate run = %#v, want fenced succeeded fact", completed)
		}
		if got := fixture.state.completeDispositions[len(fixture.state.completeDispositions)-1]; got != work.PlanningCommitFenced {
			t.Fatalf("cancelled completion disposition = %q, want fenced", got)
		}

		fixture.recover(t)
		if got := len(fixture.dispatcher.requests); got != 1 {
			t.Fatalf("dispatch count after cancelled candidate = %d, want 1", got)
		}
	})
}

func assertProjectContextAndRevisionChain(t *testing.T, fixture *coordinatorFixture) {
	t.Helper()
	contextRefs := fixture.state.refsByKind(planningContextKind)
	if len(contextRefs) != 1 {
		t.Fatalf("ProjectContext refs = %d, want one durable ref", len(contextRefs))
	}
	contextRef := contextRefs[0]
	contextArtifact, err := fixture.state.FindArtifact(context.Background(), fixture.state.change.ID, contextRef.ID)
	if err != nil {
		t.Fatalf("FindArtifact(ProjectContext) error = %v", err)
	}
	contextBody, err := fixture.artifacts.Read(context.Background(), contextArtifact.Identity)
	if err != nil {
		t.Fatalf("Read(ProjectContext) error = %v", err)
	}
	var projectContext ProjectContext
	if err := json.Unmarshal(contextBody, &projectContext); err != nil {
		t.Fatalf("decode ProjectContext: %v", err)
	}
	if projectContext.ProjectID != string(fixture.state.change.ProjectID) || projectContext.RepositoryName != "repository" {
		t.Fatalf("ProjectContext = %#v", projectContext)
	}
	if contextRef.SourceRevision != testRevision {
		t.Fatalf("ProjectContext revision = %q, want %q", contextRef.SourceRevision, testRevision)
	}

	wantPrimary := fixture.state.change.Intent.ID
	for index, run := range fixture.state.runs {
		inputs := inputRefs(run)
		wantInputs := []domain.ArtifactRefID{wantPrimary, contextRef.ID}
		if !reflect.DeepEqual(inputs, wantInputs) {
			t.Fatalf("run %s inputs = %v, want %v", run.Stage, inputs, wantInputs)
		}
		if run.SourceRevision != testRevision {
			t.Fatalf("run %s revision = %q, want %q", run.Stage, run.SourceRevision, testRevision)
		}
		request := fixture.dispatcher.requests[index]
		if request.BeforeRevision != testRevision || len(request.Inputs) != 2 || request.Inputs[1].Kind != planningContextKind {
			t.Fatalf("dispatch %s fixed inputs = %#v", run.Stage, request)
		}
		kind, _, ok := contractForStage(stageFromLifecycle(run.Stage))
		if !ok {
			t.Fatalf("unexpected Planning stage %s", run.Stage)
		}
		output := fixture.state.refsByKind(string(kind))
		if len(output) != 1 {
			t.Fatalf("authority output refs for %s = %d, want 1", run.Stage, len(output))
		}
		if output[0].SourceRevision != testRevision || !reflect.DeepEqual(output[0].InputArtifactRefIDs, wantInputs) {
			t.Fatalf("output %s links/revision = %#v", run.Stage, output[0])
		}
		wantPrimary = output[0].ID
	}
	for index, materialized := range fixture.snapshots.created {
		if materialized.revision != testRevision {
			t.Fatalf("snapshot %d revision = %q, want %q", index, materialized.revision, testRevision)
		}
	}
}

func inputRefs(run domain.AgentRun) []domain.ArtifactRefID {
	result := make([]domain.ArtifactRefID, 0, len(run.Artifacts))
	for _, artifact := range run.Artifacts {
		if artifact.Role == domain.ArtifactRoleInput {
			result = append(result, artifact.ArtifactRefID)
		}
	}
	return result
}

type coordinatorFixture struct {
	state       *fakeCoordinatorState
	artifacts   *fakeCoordinatorArtifactStore
	snapshots   *fakeSnapshotMaterializer
	dispatcher  *fakePlanningDispatcher
	coordinator *Coordinator
}

func newCoordinatorFixture(t *testing.T) *coordinatorFixture {
	t.Helper()
	artifacts := &fakeCoordinatorArtifactStore{content: make(map[domain.ArtifactIdentity][]byte)}
	intentBody := []byte("实现严格可恢复的 Planning 流程")
	intentIdentity, err := artifacts.Put(context.Background(), intentBody)
	if err != nil {
		t.Fatalf("store fixture intent: %v", err)
	}
	changeID := domain.ChangeID(domain.NewProjectID())
	intentRef := domain.ArtifactRef{
		ID: domain.ArtifactRefID(domain.NewProjectID()), ChangeID: changeID,
		ArtifactID: domain.ArtifactID(domain.NewProjectID()), Role: domain.ArtifactRoleChangeIntent,
		Ordinal: 0,
	}
	intentArtifact := domain.Artifact{
		ID: intentRef.ArtifactID, Identity: intentIdentity, MediaType: "text/plain; charset=utf-8",
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	state := &fakeCoordinatorState{
		change: domain.Change{
			ID: changeID, ProjectID: domain.NewProjectID(), RepositoryRoot: "/tmp/coordinator-fixture/repository",
			Stage: domain.LifecycleStageIntent, Status: domain.ChangeStatusActive, Version: 1,
			BaseRevision: testRevision, Intent: intentRef,
			CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(),
		},
		refs: []domain.ArtifactRef{intentRef}, artifacts: map[domain.ArtifactRefID]domain.Artifact{intentRef.ID: intentArtifact},
		candidates: make(map[domain.AgentRunID]work.PlanningRunCandidate),
	}
	snapshots := &fakeSnapshotMaterializer{}
	dispatcher := &fakePlanningDispatcher{}
	coordinator, err := NewCoordinator(state, artifacts, snapshots, dispatcher)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	return &coordinatorFixture{state: state, artifacts: artifacts, snapshots: snapshots, dispatcher: dispatcher, coordinator: coordinator}
}

func (f *coordinatorFixture) recover(t *testing.T) {
	t.Helper()
	if err := f.coordinator.Recover(context.Background()); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
}

func (f *coordinatorFixture) reportCandidate(t *testing.T, run domain.AgentRun, body []byte) {
	t.Helper()
	identity, err := f.artifacts.Put(context.Background(), body)
	if err != nil {
		t.Fatalf("store candidate: %v", err)
	}
	ref := f.state.addRef(work.PlanningArtifactWrite{
		Identity: identity, MediaType: planningCandidateMediaType, Kind: "candidate",
		SourceRevision: run.SourceRevision, InputArtifactRefIDs: inputRefs(run),
	}, domain.ArtifactRoleOutput)
	exitCode := 0
	f.state.candidates[run.ID] = work.PlanningRunCandidate{
		AgentRunID: run.ID, Outcome: domain.AgentRunOutcomeSucceeded, ExitCode: &exitCode,
		AfterRevision: run.SourceRevision, CandidateRef: &ref, ReceivedAt: time.Unix(2, 0).UTC(),
	}
}

type fakeCoordinatorArtifactStore struct {
	content map[domain.ArtifactIdentity][]byte
}

func (s *fakeCoordinatorArtifactStore) Put(_ context.Context, content []byte) (domain.ArtifactIdentity, error) {
	identity := domain.NewArtifactIdentity(content)
	s.content[identity] = append([]byte(nil), content...)
	return identity, nil
}

func (s *fakeCoordinatorArtifactStore) Read(_ context.Context, identity domain.ArtifactIdentity) ([]byte, error) {
	content, ok := s.content[identity]
	if !ok {
		return nil, domain.ErrArtifactUnavailable
	}
	return append([]byte(nil), content...), nil
}

type fakeCoordinatorState struct {
	change               domain.Change
	refs                 []domain.ArtifactRef
	artifacts            map[domain.ArtifactRefID]domain.Artifact
	runs                 []domain.AgentRun
	candidates           map[domain.AgentRunID]work.PlanningRunCandidate
	startRequests        []work.StartPlanningRunRequest
	completeRequests     []work.CompletePlanningStageRequest
	failRequests         []work.FailPlanningStageRequest
	failErrors           []error
	completeDispositions []string
}

func (s *fakeCoordinatorState) ReconcilePlanningExecutions(context.Context) error { return nil }

func (s *fakeCoordinatorState) ListRecoverablePlanningChanges(context.Context) ([]domain.Change, error) {
	if s.change.Status == domain.ChangeStatusActive && (s.change.Stage == domain.LifecycleStageIntent || s.change.Stage == domain.LifecycleStageUnderstand || s.change.Stage == domain.LifecycleStageDesign) {
		return []domain.Change{s.changeView()}, nil
	}
	for _, run := range s.runs {
		if run.IsPlanning() && run.Status == domain.AgentRunStatusRunning {
			return []domain.Change{s.changeView()}, nil
		}
	}
	return nil, nil
}

func (s *fakeCoordinatorState) FindChange(_ context.Context, changeID domain.ChangeID) (domain.Change, error) {
	if changeID != s.change.ID {
		return domain.Change{}, domain.ErrChangeNotFound
	}
	return s.changeView(), nil
}

func (s *fakeCoordinatorState) ListAgentRuns(_ context.Context, changeID domain.ChangeID) ([]domain.AgentRun, error) {
	if changeID != s.change.ID {
		return nil, domain.ErrChangeNotFound
	}
	return append([]domain.AgentRun(nil), s.runs...), nil
}

func (s *fakeCoordinatorState) ListArtifactRefs(_ context.Context, changeID domain.ChangeID) ([]domain.ArtifactRef, error) {
	if changeID != s.change.ID {
		return nil, domain.ErrChangeNotFound
	}
	return append([]domain.ArtifactRef(nil), s.refs...), nil
}

func (s *fakeCoordinatorState) FindArtifact(_ context.Context, changeID domain.ChangeID, refID domain.ArtifactRefID) (domain.Artifact, error) {
	if changeID != s.change.ID {
		return domain.Artifact{}, domain.ErrChangeNotFound
	}
	artifact, ok := s.artifacts[refID]
	if !ok {
		return domain.Artifact{}, domain.ErrArtifactNotFound
	}
	return artifact, nil
}

func (s *fakeCoordinatorState) StartPlanningRun(_ context.Context, request work.StartPlanningRunRequest) (domain.AgentRun, error) {
	for _, run := range s.runs {
		if run.Status == domain.AgentRunStatusRunning {
			return domain.AgentRun{}, domain.ErrPlanningRunConflict
		}
	}
	s.startRequests = append(s.startRequests, cloneStartRequest(request))
	inputs := append([]domain.ArtifactRefID(nil), request.InputArtifactRefIDs...)
	for _, write := range request.InputArtifacts {
		ref := s.addRef(write, domain.ArtifactRoleInput)
		inputs = append(inputs, ref.ID)
	}
	attempt := 1
	for _, previous := range s.runs {
		if previous.Stage == request.TargetStage && previous.Attempt >= attempt {
			attempt = previous.Attempt + 1
		}
	}
	run := domain.AgentRun{
		ID: domain.AgentRunID(domain.NewProjectID()), ChangeID: request.ChangeID,
		Stage: request.TargetStage, Attempt: attempt, RunKind: domain.AgentRunKindPlanning,
		SourceRevision: request.SourceRevision, Status: domain.AgentRunStatusRunning,
		StartedAt: time.Unix(int64(len(s.runs)+2), 0).UTC(),
	}
	for ordinal, refID := range inputs {
		run.Artifacts = append(run.Artifacts, domain.AgentRunArtifact{ArtifactRefID: refID, Role: domain.ArtifactRoleInput, Ordinal: ordinal})
	}
	s.runs = append(s.runs, run)
	return run, nil
}

func (s *fakeCoordinatorState) RecordPlanningRunFailureCandidate(_ context.Context, runID domain.AgentRunID, reason string) error {
	if _, exists := s.candidates[runID]; exists {
		return nil
	}
	runIndex := s.runIndex(runID)
	if runIndex < 0 || s.runs[runIndex].Status != domain.AgentRunStatusRunning {
		return domain.ErrPlanningRunConflict
	}
	s.candidates[runID] = work.PlanningRunCandidate{
		AgentRunID: runID, Outcome: domain.AgentRunOutcomeFailed,
		AfterRevision: s.runs[runIndex].SourceRevision, FailureReason: reason,
		ReceivedAt: time.Unix(3, 0).UTC(),
	}
	return nil
}

func (s *fakeCoordinatorState) FindPlanningRunCandidate(_ context.Context, runID domain.AgentRunID) (work.PlanningRunCandidate, error) {
	candidate, ok := s.candidates[runID]
	if !ok {
		return work.PlanningRunCandidate{}, domain.ErrPlanningCandidateNotFound
	}
	return candidate, nil
}

func (s *fakeCoordinatorState) CompletePlanningStage(_ context.Context, request work.CompletePlanningStageRequest) (work.PlanningStageCommit, error) {
	s.completeRequests = append(s.completeRequests, cloneCompleteRequest(request))
	return s.finish(request.AgentRunID, request.Stage, request.Attempt, request.ExpectedChangeVersion, request.Artifact, domain.AgentRunOutcomeSucceeded)
}

func (s *fakeCoordinatorState) FailPlanningStage(_ context.Context, request work.FailPlanningStageRequest) (work.PlanningStageCommit, error) {
	s.failRequests = append(s.failRequests, cloneFailRequest(request))
	if len(s.failErrors) != 0 {
		err := s.failErrors[0]
		s.failErrors = s.failErrors[1:]
		return work.PlanningStageCommit{}, err
	}
	return s.finish(request.AgentRunID, request.Stage, request.Attempt, request.ExpectedChangeVersion, request.Failure, domain.AgentRunOutcomeFailed)
}

func (s *fakeCoordinatorState) finish(runID domain.AgentRunID, stage domain.LifecycleStage, attempt int, version domain.ChangeVersion, write work.PlanningArtifactWrite, outcome string) (work.PlanningStageCommit, error) {
	index := s.runIndex(runID)
	if index < 0 {
		return work.PlanningStageCommit{}, domain.ErrPlanningRunConflict
	}
	run := &s.runs[index]
	if run.Status == domain.AgentRunStatusCompleted {
		return work.PlanningStageCommit{Run: *run, Change: s.changeView(), Disposition: work.PlanningCommitDuplicate}, nil
	}
	refRole := domain.ArtifactRoleOutput
	if outcome == domain.AgentRunOutcomeFailed {
		refRole = domain.ArtifactRoleFailure
	}
	ref := s.addRef(write, refRole)
	completedAt := time.Unix(int64(len(s.runs)+10), 0).UTC()
	if err := run.Complete(outcome, completedAt); err != nil {
		return work.PlanningStageCommit{}, err
	}
	run.Artifacts = append(run.Artifacts, domain.AgentRunArtifact{ArtifactRefID: ref.ID, Role: refRole, Ordinal: 0})
	disposition := work.PlanningCommitFenced
	if s.change.Status == domain.ChangeStatusActive && s.change.Version == version && follows(s.change.Stage, stage) && run.Attempt == s.latestAttempt(stage) {
		disposition = work.PlanningCommitCommitted
		if outcome == domain.AgentRunOutcomeFailed {
			s.change.Status = domain.ChangeStatusHumanRequired
		} else if stage == domain.LifecycleStagePlan {
			s.change.Stage = domain.LifecycleStageTicketize
		} else {
			s.change.Stage = stage
		}
		s.change.Version++
	}
	s.completeDispositions = append(s.completeDispositions, disposition)
	return work.PlanningStageCommit{Run: *run, Change: s.changeView(), ArtifactRef: ref, Disposition: disposition}, nil
}

func (s *fakeCoordinatorState) addRef(write work.PlanningArtifactWrite, role string) domain.ArtifactRef {
	ordinal := 0
	for _, ref := range s.refs {
		if ref.Role == role && ref.Ordinal >= ordinal {
			ordinal = ref.Ordinal + 1
		}
	}
	ref := domain.ArtifactRef{
		ID: domain.ArtifactRefID(domain.NewProjectID()), ChangeID: s.change.ID,
		ArtifactID: domain.ArtifactID(domain.NewProjectID()), Role: role, Ordinal: ordinal,
		Kind: write.Kind, SchemaVersion: write.SchemaVersion, Summary: write.Summary,
		SourceRevision:       write.SourceRevision,
		InputArtifactRefIDs:  append([]domain.ArtifactRefID(nil), write.InputArtifactRefIDs...),
		RawLogArtifactRefIDs: append([]domain.ArtifactRefID(nil), write.RawLogArtifactRefIDs...),
	}
	s.refs = append(s.refs, ref)
	s.artifacts[ref.ID] = domain.Artifact{
		ID: ref.ArtifactID, Identity: write.Identity, MediaType: write.MediaType,
		CreatedAt: time.Unix(int64(len(s.refs)+1), 0).UTC(),
	}
	return ref
}

func (s *fakeCoordinatorState) latestRun(t *testing.T) domain.AgentRun {
	t.Helper()
	if len(s.runs) == 0 {
		t.Fatal("no AgentRun was created")
	}
	return s.runs[len(s.runs)-1]
}

func (s *fakeCoordinatorState) changeView() domain.Change {
	result := s.change
	if len(s.runs) == 0 {
		result.LatestAgentRun = nil
		return result
	}
	latest := s.runs[len(s.runs)-1]
	result.LatestAgentRun = &latest
	return result
}

func (s *fakeCoordinatorState) runIndex(runID domain.AgentRunID) int {
	for index := range s.runs {
		if s.runs[index].ID == runID {
			return index
		}
	}
	return -1
}

func (s *fakeCoordinatorState) latestAttempt(stage domain.LifecycleStage) int {
	latest := 0
	for _, run := range s.runs {
		if run.Stage == stage && run.Attempt > latest {
			latest = run.Attempt
		}
	}
	return latest
}

func (s *fakeCoordinatorState) setStatus(status domain.ChangeStatus) {
	s.change.Status = status
	s.change.Version++
}

func (s *fakeCoordinatorState) refsByKind(kind string) []domain.ArtifactRef {
	var refs []domain.ArtifactRef
	for _, ref := range s.refs {
		if ref.Kind == kind {
			refs = append(refs, ref)
		}
	}
	return refs
}

func follows(checkpoint, target domain.LifecycleStage) bool {
	return checkpoint == domain.LifecycleStageIntent && target == domain.LifecycleStageUnderstand ||
		checkpoint == domain.LifecycleStageUnderstand && target == domain.LifecycleStageDesign ||
		checkpoint == domain.LifecycleStageDesign && target == domain.LifecycleStagePlan
}

func cloneStartRequest(request work.StartPlanningRunRequest) work.StartPlanningRunRequest {
	request.InputArtifactRefIDs = append([]domain.ArtifactRefID(nil), request.InputArtifactRefIDs...)
	request.InputArtifacts = append([]work.PlanningArtifactWrite(nil), request.InputArtifacts...)
	return request
}

func cloneCompleteRequest(request work.CompletePlanningStageRequest) work.CompletePlanningStageRequest {
	request.Artifact.InputArtifactRefIDs = append([]domain.ArtifactRefID(nil), request.Artifact.InputArtifactRefIDs...)
	request.Artifact.RawLogArtifactRefIDs = append([]domain.ArtifactRefID(nil), request.Artifact.RawLogArtifactRefIDs...)
	return request
}

func cloneFailRequest(request work.FailPlanningStageRequest) work.FailPlanningStageRequest {
	request.Failure.InputArtifactRefIDs = append([]domain.ArtifactRefID(nil), request.Failure.InputArtifactRefIDs...)
	request.Failure.RawLogArtifactRefIDs = append([]domain.ArtifactRefID(nil), request.Failure.RawLogArtifactRefIDs...)
	return request
}

type fakePlanningDispatcher struct {
	requests          []DispatchRequest
	err               error
	assigned          map[domain.AgentRunID]bool
	before            func()
	assignBeforeError bool
}

func (*fakePlanningDispatcher) Select(context.Context, RuntimeCapability) (string, bool, error) {
	return "worker-local", true, nil
}

func (d *fakePlanningDispatcher) Dispatch(_ context.Context, request DispatchRequest) error {
	d.requests = append(d.requests, request)
	if d.before != nil {
		d.before()
	}
	if d.err == nil || d.assignBeforeError {
		if d.assigned == nil {
			d.assigned = make(map[domain.AgentRunID]bool)
		}
		d.assigned[request.AgentRunID] = true
	}
	return d.err
}

func (d *fakePlanningDispatcher) Assigned(_ context.Context, runID domain.AgentRunID) (bool, error) {
	return d.assigned[runID], nil
}

type fakeSnapshotMaterializer struct {
	created []*fakePlanningSnapshot
}

func (m *fakeSnapshotMaterializer) Materialize(_ context.Context, _ string, revision string) (Snapshot, error) {
	snapshot := &fakePlanningSnapshot{root: fmt.Sprintf("/tmp/planning-snapshot-%d", len(m.created)+1), revision: revision}
	m.created = append(m.created, snapshot)
	return snapshot, nil
}

func (m *fakeSnapshotMaterializer) closedCount() int {
	count := 0
	for _, snapshot := range m.created {
		if snapshot.closed {
			count++
		}
	}
	return count
}

type fakePlanningSnapshot struct {
	root          string
	revision      string
	closed        bool
	closeFailures int
}

func (s *fakePlanningSnapshot) Root() string     { return s.root }
func (s *fakePlanningSnapshot) Revision() string { return s.revision }
func (s *fakePlanningSnapshot) Close() error {
	if s.closeFailures > 0 {
		s.closeFailures--
		return errors.New("injected snapshot close failure")
	}
	if s.closed {
		return errors.New("snapshot closed twice")
	}
	s.closed = true
	return nil
}

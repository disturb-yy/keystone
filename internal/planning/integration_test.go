package planning

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/artifact"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const planningIntegrationRevision = "0123456789012345678901234567890123456789"

func TestPlanningVerticalIntegrationRecoversDurableCandidateAndReachesExecute(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "keystone.db")
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repositoryRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	artifacts, err := artifact.New(artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	firstDB, firstState := openPlanningIntegrationStore(t, databasePath)
	firstDBClosed := false
	t.Cleanup(func() {
		if !firstDBClosed {
			_ = firstDB.Close()
		}
	})
	change := createPlanningIntegrationChange(t, firstState, artifacts, repositoryRoot)
	registerPlanningIntegrationWorker(t, firstState, "planning-worker-before-restart")

	firstSnapshots := newPlanningIntegrationSnapshots(t)
	firstDispatcher := newPlanningIntegrationDispatcher(firstState)
	firstCoordinator, err := NewCoordinator(firstState, artifacts, firstSnapshots, firstDispatcher)
	if err != nil {
		t.Fatal(err)
	}

	recoverPlanningIntegration(t, firstCoordinator)
	understandRun := findPlanningIntegrationRun(t, firstState, change.ID, domain.LifecycleStageUnderstand)
	if got := len(firstDispatcher.requests); got != 1 {
		t.Fatalf("Understand dispatch count = %d, want 1", got)
	}
	recoverPlanningIntegration(t, firstCoordinator)
	if got := len(firstDispatcher.requests); got != 1 {
		t.Fatalf("repeated Recover dispatch count = %d, want 1", got)
	}

	firstDispatcher.reportCandidate(t, artifacts, understandRun, planningIntegrationCandidateJSON(t, StageUnderstand, change.BaseRevision, primaryPlanningInput(t, understandRun)))
	assertPlanningCandidateIsNonAuthoritative(t, firstState, change.ID, understandRun.ID, domain.LifecycleStageIntent)
	if err := firstCoordinator.Close(); err != nil {
		t.Fatalf("close first Coordinator: %v", err)
	}
	if got := firstSnapshots.closedCount(); got != 1 {
		t.Fatalf("first Coordinator closed snapshots = %d, want 1", got)
	}
	if err := firstDB.Close(); err != nil {
		t.Fatalf("close first SQLite handle: %v", err)
	}
	firstDBClosed = true

	secondDB, secondState := openPlanningIntegrationStore(t, databasePath)
	t.Cleanup(func() { _ = secondDB.Close() })
	if err := secondState.ReconcileWorkerRestart(ctx); err != nil {
		t.Fatalf("reconcile Worker restart: %v", err)
	}
	if candidate, err := secondState.FindPlanningRunCandidate(ctx, understandRun.ID); err != nil || candidate.CandidateRef == nil {
		t.Fatalf("durable Understand candidate after SQLite reopen = %+v, err=%v", candidate, err)
	}
	registerPlanningIntegrationWorker(t, secondState, "planning-worker-after-restart")

	secondArtifacts, err := artifact.New(artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshots := newPlanningIntegrationSnapshots(t)
	secondDispatcher := newPlanningIntegrationDispatcher(secondState)
	secondCoordinator, err := NewCoordinator(secondState, secondArtifacts, secondSnapshots, secondDispatcher)
	if err != nil {
		t.Fatal(err)
	}

	// 新 Coordinator 必须先消费耐久 candidate，而不是重复创建或派发 Understand。
	recoverPlanningIntegration(t, secondCoordinator)
	assertPlanningIntegrationCheckpoint(t, secondState, change.ID, domain.LifecycleStageUnderstand)
	understandRun = findPlanningIntegrationRun(t, secondState, change.ID, domain.LifecycleStageUnderstand)
	assertCompletedPlanningIntegrationRun(t, understandRun)
	if got := len(secondDispatcher.requests); got != 0 {
		t.Fatalf("restart recovery dispatched before settling candidate: %d requests", got)
	}

	checkpoints := []struct {
		stage      Stage
		checkpoint domain.LifecycleStage
	}{
		{stage: StageDesign, checkpoint: domain.LifecycleStageDesign},
		{stage: StagePlan, checkpoint: domain.LifecycleStageTicketize},
	}
	for index, step := range checkpoints {
		recoverPlanningIntegration(t, secondCoordinator)
		run := findPlanningIntegrationRun(t, secondState, change.ID, lifecycleStage(step.stage))
		if run.Status != domain.AgentRunStatusRunning {
			t.Fatalf("%s run status = %s, want running", step.stage, run.Status)
		}
		if got := len(secondDispatcher.requests); got != index+1 {
			t.Fatalf("%s dispatch count = %d, want %d", step.stage, got, index+1)
		}
		recoverPlanningIntegration(t, secondCoordinator)
		if got := len(secondDispatcher.requests); got != index+1 {
			t.Fatalf("repeated %s Recover dispatch count = %d, want %d", step.stage, got, index+1)
		}

		secondDispatcher.reportCandidate(t, secondArtifacts, run, planningIntegrationCandidateJSON(t, step.stage, change.BaseRevision, primaryPlanningInput(t, run)))
		current, err := secondState.FindChange(ctx, change.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Stage == step.checkpoint {
			t.Fatalf("%s Worker candidate advanced authority before Coordinator validation", step.stage)
		}
		recoverPlanningIntegration(t, secondCoordinator)
		assertPlanningIntegrationCheckpoint(t, secondState, change.ID, step.checkpoint)
		assertCompletedPlanningIntegrationRun(t, findPlanningIntegrationRun(t, secondState, change.ID, lifecycleStage(step.stage)))
	}

	recoverPlanningIntegration(t, secondCoordinator)
	if got := len(secondDispatcher.requests); got != len(checkpoints)+1 {
		t.Fatalf("Ticketize dispatch count = %d, want %d", got, len(checkpoints)+1)
	}
	ticketizeRun := findPlanningIntegrationRun(t, secondState, change.ID, domain.LifecycleStageTicketize)
	if ticketizeRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("Ticketize run status = %s, want running", ticketizeRun.Status)
	}
	secondDispatcher.reportCandidate(t, secondArtifacts, ticketizeRun, planningIntegrationTicketDraftJSON(t))
	recoverPlanningIntegration(t, secondCoordinator)
	assertPlanningIntegrationCheckpoint(t, secondState, change.ID, domain.LifecycleStageExecute)
	assertCompletedPlanningIntegrationRun(t, findPlanningIntegrationRun(t, secondState, change.ID, domain.LifecycleStageTicketize))
	if got := len(secondDispatcher.requests); got != len(checkpoints)+1 {
		t.Fatalf("settled Ticketize dispatch count = %d, want %d", got, len(checkpoints)+1)
	}
	if err := secondCoordinator.Close(); err != nil {
		t.Fatalf("close second Coordinator: %v", err)
	}
	if got := secondSnapshots.closedCount(); got != 3 {
		t.Fatalf("second Coordinator closed snapshots = %d, want 3", got)
	}
	allRevisions := append(append([]string(nil), firstSnapshots.revisions...), secondSnapshots.revisions...)
	if len(allRevisions) != 4 {
		t.Fatalf("materialized snapshots = %d, want 4", len(allRevisions))
	}
	for index, revision := range allRevisions {
		if revision != change.BaseRevision {
			t.Fatalf("snapshot %d revision = %q, want %q", index, revision, change.BaseRevision)
		}
	}

	assertPlanningIntegrationLineage(t, secondState, secondArtifacts, change, append(append([]DispatchRequest(nil), firstDispatcher.requests...), secondDispatcher.requests...))
}

func openPlanningIntegrationStore(t *testing.T, databasePath string) (*sql.DB, *workstore.Store) {
	t.Helper()
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	migrations := append(migration.DefaultMigrations(), workstore.Migrations()...)
	if err := migration.NewRunner(migrations).Apply(context.Background(), db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	state, err := workstore.New(db)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db, state
}

func createPlanningIntegrationChange(t *testing.T, state *workstore.Store, artifacts *artifact.Store, repositoryRoot string) domain.Change {
	t.Helper()
	ctx := context.Background()
	reservation, err := state.Reserve(ctx, repositoryRoot, "planning-integration-project", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	project, err := state.Finalize(
		ctx,
		"planning-integration-project",
		reservation.Intent,
		domain.ProjectManifest{Version: 1, ProjectID: reservation.Intent.ProjectID},
		domain.RepositoryBinding{Root: repositoryRoot, ManifestPath: filepath.Join(repositoryRoot, ".keystone", "project.yaml")},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domain.NewChangeIntent("实现可恢复且严格串行的 Understand、Design 与 Plan")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := artifacts.Put(ctx, []byte(intent.Original))
	if err != nil {
		t.Fatal(err)
	}
	change, err := state.CreateChange(ctx, work.ChangeCreateRecord{
		Project: project,
		Snapshot: domain.ChangeSourceSnapshot{
			RepositoryRoot: repositoryRoot,
			BaseRevision:   planningIntegrationRevision,
		},
		Intent:         intent,
		IntentIdentity: identity,
		IdempotencyKey: "planning-integration-change",
		Actor:          "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return change
}

func registerPlanningIntegrationWorker(t *testing.T, state *workstore.Store, workerID string) {
	t.Helper()
	secret, err := workstore.NewWorkerSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PrepareWorker(context.Background(), workerID, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := state.RegisterWorker(context.Background(), workercontract.Register{
		WorkerID: workerID, ProtocolVersion: workercontract.ProtocolVersionV1,
		Capabilities: []string{"runtime:codex"},
	}); err != nil {
		t.Fatal(err)
	}
}

func recoverPlanningIntegration(t *testing.T, coordinator *Coordinator) {
	t.Helper()
	if err := coordinator.Recover(context.Background()); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
}

func findPlanningIntegrationRun(t *testing.T, state *workstore.Store, changeID domain.ChangeID, stage domain.LifecycleStage) domain.AgentRun {
	t.Helper()
	runs, err := state.ListAgentRuns(context.Background(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	for index := len(runs) - 1; index >= 0; index-- {
		if runs[index].Stage == stage {
			return runs[index]
		}
	}
	t.Fatalf("no %s AgentRun found in %+v", stage, runs)
	return domain.AgentRun{}
}

func primaryPlanningInput(t *testing.T, run domain.AgentRun) string {
	t.Helper()
	for _, input := range run.Artifacts {
		if input.Role == domain.ArtifactRoleInput {
			return string(input.ArtifactRefID)
		}
	}
	t.Fatalf("run %s has no input ArtifactRef", run.ID)
	return ""
}

func assertPlanningCandidateIsNonAuthoritative(t *testing.T, state *workstore.Store, changeID domain.ChangeID, runID domain.AgentRunID, checkpoint domain.LifecycleStage) {
	t.Helper()
	candidate, err := state.FindPlanningRunCandidate(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.CandidateRef == nil || len(candidate.RawLogRefs) != 1 {
		t.Fatalf("candidate evidence = %+v, want candidate plus one raw log", candidate)
	}
	change, err := state.FindChange(context.Background(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	if change.Stage != checkpoint || change.Status != domain.ChangeStatusActive || change.LatestAgentRun == nil || change.LatestAgentRun.Status != domain.AgentRunStatusRunning {
		t.Fatalf("candidate changed authority = %+v", change)
	}
}

func assertPlanningIntegrationCheckpoint(t *testing.T, state *workstore.Store, changeID domain.ChangeID, checkpoint domain.LifecycleStage) {
	t.Helper()
	change, err := state.FindChange(context.Background(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	if change.Stage != checkpoint || change.Status != domain.ChangeStatusActive {
		t.Fatalf("Change checkpoint = %s/%s, want %s/active", change.Stage, change.Status, checkpoint)
	}
}

func assertCompletedPlanningIntegrationRun(t *testing.T, run domain.AgentRun) {
	t.Helper()
	if run.Status != domain.AgentRunStatusCompleted || run.Outcome != domain.AgentRunOutcomeSucceeded || run.CompletedAt == nil || run.Attempt != 1 || !run.IsPlanning() {
		t.Fatalf("completed Planning run = %+v", run)
	}
}

func assertPlanningIntegrationLineage(t *testing.T, state *workstore.Store, artifacts *artifact.Store, original domain.Change, dispatches []DispatchRequest) {
	t.Helper()
	ctx := context.Background()
	change, err := state.FindChange(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if change.Stage != domain.LifecycleStageExecute || change.Status != domain.ChangeStatusActive || change.Version != 5 {
		t.Fatalf("final Change = %+v, want Execute/active version 5", change)
	}
	runs, err := state.ListAgentRuns(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 4 || len(dispatches) != 4 {
		t.Fatalf("runs/dispatches = %d/%d, want 4/4", len(runs), len(dispatches))
	}

	refs, err := state.ListArtifactRefs(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	contextRefs := planningIntegrationRefsByKind(refs, planningContextKind)
	if len(contextRefs) != 1 {
		t.Fatalf("ProjectContext refs = %d, want one reused ref", len(contextRefs))
	}
	contextRef := contextRefs[0]
	contextArtifact, err := state.FindArtifact(ctx, change.ID, contextRef.ID)
	if err != nil {
		t.Fatal(err)
	}
	contextBody, err := artifacts.Read(ctx, contextArtifact.Identity)
	if err != nil {
		t.Fatal(err)
	}
	var projectContext ProjectContext
	if err := json.Unmarshal(contextBody, &projectContext); err != nil {
		t.Fatal(err)
	}
	if projectContext.ProjectID != string(change.ProjectID) || projectContext.RepositoryName != filepath.Base(change.RepositoryRoot) || contextRef.SourceRevision != change.BaseRevision {
		t.Fatalf("ProjectContext/ref = %+v/%+v", projectContext, contextRef)
	}

	wantStages := []Stage{StageUnderstand, StageDesign, StagePlan}
	wantPrimary := change.Intent.ID
	var planOutput domain.ArtifactRef
	for index, stage := range wantStages {
		run := runs[index]
		if run.Stage != lifecycleStage(stage) || run.SourceRevision != change.BaseRevision {
			t.Fatalf("run %d stage/revision = %s/%s", index, run.Stage, run.SourceRevision)
		}
		assertCompletedPlanningIntegrationRun(t, run)
		wantInputs := []domain.ArtifactRefID{wantPrimary, contextRef.ID}
		if got := planningIntegrationRunInputs(run); !reflect.DeepEqual(got, wantInputs) {
			t.Fatalf("%s run inputs = %v, want %v", stage, got, wantInputs)
		}
		dispatch := dispatches[index]
		if dispatch.AgentRunID != run.ID || dispatch.Attempt != 1 || dispatch.BeforeRevision != change.BaseRevision || len(dispatch.Inputs) != 2 || dispatch.Inputs[1].Kind != planningContextKind {
			t.Fatalf("%s dispatch = %+v", stage, dispatch)
		}
		kind, schema, ok := contractForStage(stage)
		if !ok {
			t.Fatalf("missing contract for %s", stage)
		}
		outputs := planningIntegrationOutputRefsByKind(refs, string(kind))
		if len(outputs) != 1 {
			t.Fatalf("%s authority outputs = %d, want 1", stage, len(outputs))
		}
		output := outputs[0]
		if output.SchemaVersion != schema || output.SourceRevision != change.BaseRevision || !reflect.DeepEqual(output.InputArtifactRefIDs, wantInputs) || len(output.RawLogArtifactRefIDs) != 2 {
			t.Fatalf("%s output lineage = %+v", stage, output)
		}
		wantPrimary = output.ID
		if stage == StagePlan {
			planOutput = output
		}
		candidate, err := state.FindPlanningRunCandidate(ctx, run.ID)
		if err != nil || candidate.CandidateRef == nil || len(candidate.RawLogRefs) != 1 {
			t.Fatalf("%s durable candidate = %+v, err=%v", stage, candidate, err)
		}
	}

	events, err := state.ListChangeEvents(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	ticketizeRun := runs[3]
	assertCompletedPlanningIntegrationRun(t, ticketizeRun)
	if ticketizeRun.Stage != domain.LifecycleStageTicketize {
		t.Fatalf("Ticketize run = %+v", ticketizeRun)
	}
	ticketizeInputs := planningIntegrationRunInputs(ticketizeRun)
	if len(ticketizeInputs) != 2 || ticketizeInputs[0] == planOutput.ID || ticketizeInputs[1] != contextRef.ID {
		t.Fatalf("Ticketize run inputs = %v, want copied Plan input and Context input", ticketizeInputs)
	}
	planInputRef, err := state.ListArtifactRefs(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	var copiedPlanInput domain.ArtifactRef
	for _, ref := range planInputRef {
		if ref.ID == ticketizeInputs[0] {
			copiedPlanInput = ref
			break
		}
	}
	if copiedPlanInput.Role != domain.ArtifactRoleInput || copiedPlanInput.Kind != "plan" || copiedPlanInput.ArtifactID != planOutput.ArtifactID {
		t.Fatalf("Ticketize copied Plan input = %+v, want same content as output %+v", copiedPlanInput, planOutput)
	}
	ticketizeDispatch := dispatches[3]
	if ticketizeDispatch.AgentRunID != ticketizeRun.ID || len(ticketizeDispatch.Inputs) != 2 || ticketizeDispatch.Inputs[0].Kind != string(ArtifactKindPlan) || ticketizeDispatch.Inputs[1].Kind != planningContextKind {
		t.Fatalf("Ticketize dispatch = %+v", ticketizeDispatch)
	}
	draftRefs := planningIntegrationOutputRefsByKind(refs, "ticket_draft")
	if len(draftRefs) != 1 {
		t.Fatalf("Ticketize draft outputs = %d, want 1", len(draftRefs))
	}
	candidate, err := state.FindPlanningRunCandidate(ctx, ticketizeRun.ID)
	if err != nil || candidate.CandidateRef == nil || candidate.CandidateRef.ID != draftRefs[0].ID || len(candidate.RawLogRefs) != 1 {
		t.Fatalf("Ticketize durable candidate = %+v, err=%v", candidate, err)
	}
	graph, err := state.FindTicketGraph(ctx, change.ID)
	if err != nil || graph.PlanArtifactRef.ID != planOutput.ID || graph.DraftArtifactRef.ID != draftRefs[0].ID || len(graph.Tickets) != 2 || len(graph.Dependencies) != 1 {
		t.Fatalf("Ticketize graph = %+v, err=%v", graph, err)
	}
	if len(events) != 14 || events[len(events)-1].Type != domain.StageAdvancedType {
		t.Fatalf("Planning event trace = %+v", events)
	}
	var graphEvents int
	for _, event := range events {
		if event.Type == domain.TicketGraphCreatedType {
			graphEvents++
			if event.TicketGraphID == nil || *event.TicketGraphID != graph.ID {
				t.Fatalf("TicketGraphCreated event = %+v", event)
			}
		}
	}
	if graphEvents != 1 {
		t.Fatalf("TicketGraphCreated events = %d, want 1", graphEvents)
	}
	for index, event := range events {
		if event.Sequence != index+1 {
			t.Fatalf("event %d sequence = %d, want %d", index, event.Sequence, index+1)
		}
	}
}

func planningIntegrationRunInputs(run domain.AgentRun) []domain.ArtifactRefID {
	inputs := make([]domain.ArtifactRefID, 0, len(run.Artifacts))
	for _, artifact := range run.Artifacts {
		if artifact.Role == domain.ArtifactRoleInput {
			inputs = append(inputs, artifact.ArtifactRefID)
		}
	}
	return inputs
}

func planningIntegrationRefsByKind(refs []domain.ArtifactRef, kind string) []domain.ArtifactRef {
	selected := make([]domain.ArtifactRef, 0)
	for _, ref := range refs {
		if ref.Kind == kind {
			selected = append(selected, ref)
		}
	}
	return selected
}

func planningIntegrationOutputRefsByKind(refs []domain.ArtifactRef, kind string) []domain.ArtifactRef {
	selected := make([]domain.ArtifactRef, 0)
	for _, ref := range refs {
		if ref.Role == domain.ArtifactRoleOutput && ref.Kind == kind {
			selected = append(selected, ref)
		}
	}
	return selected
}

func planningIntegrationCandidateJSON(t *testing.T, stage Stage, revision, inputID string) []byte {
	t.Helper()
	kind, schema, ok := contractForStage(stage)
	if !ok {
		t.Fatalf("missing contract for %s", stage)
	}
	envelope := ArtifactEnvelope{
		Kind: kind, SchemaVersion: schema, Stage: stage, Summary: "integration summary",
		SourceRevision: revision, InputArtifactIDs: []string{inputID},
	}
	var payload any
	switch stage {
	case StageUnderstand:
		payload = UnderstandingPayload{Problem: "problem", Goals: []string{"goal"}, Constraints: []string{}}
	case StageDesign:
		payload = DesignPayload{Approach: "approach", Decisions: []string{"decision"}, Risks: []string{}}
	case StagePlan:
		payload = PlanPayload{
			Steps:        []PlanStep{{ID: "step-1", Summary: "implement", Paths: []string{"internal/planning"}}},
			Verification: []string{"go test ./internal/planning/..."},
		}
	default:
		t.Fatalf("unsupported Planning stage %s", stage)
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Payload = encodedPayload
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func planningIntegrationTicketDraftJSON(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"schema_version":"keystone.ticket-draft.v1","tickets":[{"generation_key":"root","title":"Root ticket","scope":"internal/work","acceptance_criteria":["authority commits the graph"],"blocked_by":[]},{"generation_key":"child","title":"Child ticket","scope":"internal/planning","acceptance_criteria":["draft remains traceable"],"blocked_by":["root"]}]}`)
}

type planningIntegrationDispatcher struct {
	state       *workstore.Store
	requests    []DispatchRequest
	assignments map[domain.AgentRunID]planningIntegrationAssignment
}

type planningIntegrationAssignment struct {
	workerID   string
	assignment workercontract.Assignment
}

func newPlanningIntegrationDispatcher(state *workstore.Store) *planningIntegrationDispatcher {
	return &planningIntegrationDispatcher{state: state, assignments: make(map[domain.AgentRunID]planningIntegrationAssignment)}
}

func (d *planningIntegrationDispatcher) Select(ctx context.Context, capability RuntimeCapability) (string, bool, error) {
	if capability != RuntimeCapabilityPlanningReadOnly {
		return "", false, fmt.Errorf("unsupported Planning capability %q", capability)
	}
	return d.state.AvailableWorker(ctx, "runtime:codex")
}

func (d *planningIntegrationDispatcher) Dispatch(ctx context.Context, request DispatchRequest) error {
	inputs := make([]workercontract.ArtifactSummary, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		inputs = append(inputs, workercontract.ArtifactSummary{
			Kind: input.Kind, SHA256: input.SHA256, SizeBytes: input.SizeBytes, MediaType: input.MediaType,
		})
	}
	assignment, err := d.state.IssuePlanningAssignment(
		ctx,
		request.AgentRunID,
		request.Target,
		request.WorkspacePath,
		"codex",
		request.Instruction,
		request.BeforeRevision,
		"planning-integration-"+string(request.AgentRunID),
		inputs,
	)
	if err != nil {
		return err
	}
	if assignment.ResultMode != workercontract.ResultModePlanningCandidate {
		return fmt.Errorf("assignment result mode = %q", assignment.ResultMode)
	}
	d.requests = append(d.requests, request)
	d.assignments[request.AgentRunID] = planningIntegrationAssignment{workerID: request.Target, assignment: assignment}
	return nil
}

func (d *planningIntegrationDispatcher) Assigned(ctx context.Context, runID domain.AgentRunID) (bool, error) {
	return d.state.PlanningRunAssigned(ctx, runID)
}

func (d *planningIntegrationDispatcher) reportCandidate(t *testing.T, artifacts *artifact.Store, run domain.AgentRun, candidate []byte) {
	t.Helper()
	dispatched, ok := d.assignments[run.ID]
	if !ok {
		t.Fatalf("no Assignment for run %s", run.ID)
	}
	exitCode := 0
	report := workercontract.Report{
		AgentRunID: string(run.ID), LeaseToken: dispatched.assignment.LeaseToken,
		Outcome: workercontract.Outcome(domain.AgentRunOutcomeSucceeded), Attempt: run.Attempt,
		ExitCode: &exitCode, AfterRevision: run.SourceRevision,
		Artifacts: []workercontract.Artifact{
			planningIntegrationReportArtifact("candidate", candidate),
			planningIntegrationReportArtifact("stdout", []byte("raw "+string(run.Stage)+" output")),
		},
	}
	response, err := d.state.ReportWorkerFor(context.Background(), dispatched.workerID, report, artifacts)
	if err != nil {
		t.Fatalf("report %s candidate: %v", run.Stage, err)
	}
	if response.Disposition != "candidate_received" || response.LeaseState != "consumed" {
		t.Fatalf("report %s response = %+v", run.Stage, response)
	}
	if _, err := d.state.HeartbeatWorker(context.Background(), workercontract.Heartbeat{WorkerID: dispatched.workerID}); err != nil {
		t.Fatalf("ack %s Worker idle: %v", run.Stage, err)
	}
}

func planningIntegrationReportArtifact(kind string, content []byte) workercontract.Artifact {
	identity := domain.NewArtifactIdentity(content)
	return workercontract.Artifact{
		Kind: kind, ContentBase64: base64.StdEncoding.EncodeToString(content),
		SHA256: identity.SHA256, SizeBytes: identity.ByteLength,
	}
}

type planningIntegrationSnapshots struct {
	root      string
	created   []*planningIntegrationSnapshot
	expected  string
	revisions []string
}

func newPlanningIntegrationSnapshots(t *testing.T) *planningIntegrationSnapshots {
	t.Helper()
	return &planningIntegrationSnapshots{root: t.TempDir()}
}

func (m *planningIntegrationSnapshots) Materialize(_ context.Context, repositoryRoot, revision string) (Snapshot, error) {
	if m.expected == "" {
		m.expected = repositoryRoot
	}
	if repositoryRoot != m.expected {
		return nil, errors.New("Planning snapshot repository root changed")
	}
	root, err := os.MkdirTemp(m.root, "snapshot-")
	if err != nil {
		return nil, err
	}
	snapshot := &planningIntegrationSnapshot{root: root, revision: revision}
	m.created = append(m.created, snapshot)
	m.revisions = append(m.revisions, revision)
	return snapshot, nil
}

func (m *planningIntegrationSnapshots) closedCount() int {
	count := 0
	for _, snapshot := range m.created {
		if snapshot.closed {
			count++
		}
	}
	return count
}

type planningIntegrationSnapshot struct {
	root     string
	revision string
	closed   bool
}

func (s *planningIntegrationSnapshot) Root() string     { return s.root }
func (s *planningIntegrationSnapshot) Revision() string { return s.revision }
func (s *planningIntegrationSnapshot) Close() error {
	if s.closed {
		return errors.New("Planning snapshot closed twice")
	}
	s.closed = true
	return os.RemoveAll(s.root)
}

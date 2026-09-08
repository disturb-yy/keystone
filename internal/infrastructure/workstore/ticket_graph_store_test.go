package workstore

import (
	"context"
	"errors"
	"testing"

	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestCompleteTicketizePersistsImmutableGraphAndUsesCopiedPlanInput(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/ticket-graph-completion", "ticket-graph-project", "ticket-graph-change")
	planRef := advanceTestChangeToTicketize(t, store, change)
	run, err := store.StartTicketizeRun(ctx, work.StartPlanningRunRequest{
		ChangeID: change.ID, TargetStage: domain.LifecycleStageTicketize, ExpectedChangeVersion: 4,
		SourceRevision: change.BaseRevision, Actor: "planning", InputArtifactRefIDs: []domain.ArtifactRefID{planRef.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	inputs := planningRunInputRefIDs(run)
	if len(inputs) != 1 || inputs[0] == planRef.ID {
		t.Fatalf("ticketize inputs = %v, want copied input distinct from plan output %s", inputs, planRef.ID)
	}
	draft := insertTestPlanningArtifactRef(t, store, change, "ticket_draft", "keystone.ticket-draft.v1", "draft")
	request := work.TicketizeCompletionRequest{
		AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt, ExpectedChangeVersion: 4,
		SourceRevision: change.BaseRevision, Actor: "planning", PlanArtifactRef: planRef, DraftArtifactRef: draft,
		GeneratorName: "codex", GeneratorVersion: "ticketize.v1",
		Candidate: domain.TicketGraphCandidate{Tickets: []domain.TicketDraft{
			{GenerationKey: "root", Title: "Root ticket", Scope: "internal/work", AcceptanceCriteria: []string{"go test ./..."}},
			{GenerationKey: "child", Title: "Child ticket", Scope: "internal/planning", AcceptanceCriteria: []string{"strict draft"}, BlockedBy: []string{"root"}},
		}},
	}
	commit, err := store.CompleteTicketize(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if commit.Disposition != work.PlanningCommitCommitted || commit.Change.Stage != domain.LifecycleStageExecute || commit.Change.Version != 5 {
		t.Fatalf("ticketize commit = %+v", commit)
	}
	if len(commit.Graph.Tickets) != 2 || len(commit.Graph.Dependencies) != 1 || len(commit.Graph.StructuralFrontier()) != 1 {
		t.Fatalf("ticket graph = %+v", commit.Graph)
	}
	if commit.Graph.Tickets[0].ID == "" || commit.Graph.Tickets[0].AcceptanceCriteria[0].Text != "go test ./..." {
		t.Fatalf("ticket graph ticket = %+v", commit.Graph.Tickets[0])
	}
	replay, err := store.CompleteTicketize(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Disposition != work.PlanningCommitDuplicate || replay.Graph.ID != commit.Graph.ID {
		t.Fatalf("ticketize replay = %+v", replay)
	}
	if _, err := store.db.Exec(`UPDATE t_ticket_graphs SET generator_name = 'replaced' WHERE graph_id = ?`, commit.Graph.ID); err == nil {
		t.Fatal("ticket graph update unexpectedly succeeded")
	}
	if _, err := store.db.Exec(`DELETE FROM t_tickets WHERE graph_id = ?`, commit.Graph.ID); err == nil {
		t.Fatal("ticket delete unexpectedly succeeded")
	}
	events, err := store.ListChangeEvents(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	var graphEvents int
	for _, event := range events {
		if event.Type == domain.TicketGraphCreatedType {
			graphEvents++
			if event.TicketGraphID == nil || *event.TicketGraphID != commit.Graph.ID {
				t.Fatalf("graph event = %+v", event)
			}
		}
	}
	if graphEvents != 1 {
		t.Fatalf("graph event count = %d, want 1", graphEvents)
	}
}

func TestTicketizeCompletionIsFencedAfterPause(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/ticket-graph-fence", "ticket-fence-project", "ticket-fence-change")
	planRef := advanceTestChangeToTicketize(t, store, change)
	run, err := store.StartTicketizeRun(ctx, work.StartPlanningRunRequest{ChangeID: change.ID, TargetStage: domain.LifecycleStageTicketize, ExpectedChangeVersion: 4, SourceRevision: change.BaseRevision, Actor: "planning", InputArtifactRefIDs: []domain.ArtifactRefID{planRef.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyCommand(ctx, change.ID, "pause", 4, "pause-ticket", "human"); err != nil {
		t.Fatal(err)
	}
	draft := insertTestPlanningArtifactRef(t, store, change, "ticket_draft", "keystone.ticket-draft.v1", "draft")
	_, err = store.CompleteTicketize(ctx, work.TicketizeCompletionRequest{AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt, ExpectedChangeVersion: 4, SourceRevision: change.BaseRevision, Actor: "planning", PlanArtifactRef: planRef, DraftArtifactRef: draft, GeneratorName: "codex", GeneratorVersion: "ticketize.v1", Candidate: domain.TicketGraphCandidate{Tickets: []domain.TicketDraft{{GenerationKey: "one", Title: "One", Scope: "scope", AcceptanceCriteria: []string{"done"}}}}})
	if !errors.Is(err, domain.ErrTicketGraphConflict) && !errors.Is(err, domain.ErrTicketizeFenced) {
		t.Fatalf("fenced completion error = %v", err)
	}
	if _, err := store.FindTicketGraph(ctx, change.ID); !errors.Is(err, domain.ErrTicketGraphNotFound) {
		t.Fatalf("fenced graph error = %v", err)
	}
}

func advanceTestChangeToTicketize(t *testing.T, store *Store, initial domain.Change) domain.ArtifactRef {
	t.Helper()
	change := initial
	for _, step := range []struct {
		stage  domain.LifecycleStage
		kind   string
		schema string
	}{
		{domain.LifecycleStageUnderstand, "understanding", "Understanding.v1"},
		{domain.LifecycleStageDesign, "design", "Design.v1"},
		{domain.LifecycleStagePlan, "plan", "Plan.v1"},
	} {
		input := change.Intent.ID
		if change.Stage != domain.LifecycleStageIntent {
			refs, err := store.ListArtifactRefs(context.Background(), change.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, ref := range refs {
				if ref.Role == domain.ArtifactRoleOutput && ref.Kind == map[domain.LifecycleStage]string{domain.LifecycleStageUnderstand: "understanding", domain.LifecycleStageDesign: "design", domain.LifecycleStagePlan: "plan"}[change.Stage] {
					input = ref.ID
				}
			}
		}
		run, err := store.StartPlanningRun(context.Background(), work.StartPlanningRunRequest{ChangeID: change.ID, TargetStage: step.stage, ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: "planning", InputArtifactRefIDs: []domain.ArtifactRefID{input}})
		if err != nil {
			t.Fatal(err)
		}
		completion := successfulPlanningCompletion(change, run, step.kind, step.schema, step.kind)
		result, err := store.CompletePlanningStage(context.Background(), completion)
		if err != nil {
			t.Fatal(err)
		}
		change = result.Change
	}
	refs, err := store.ListArtifactRefs(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if ref.Role == domain.ArtifactRoleOutput && ref.Kind == "plan" {
			return ref
		}
	}
	t.Fatal("plan output not found")
	return domain.ArtifactRef{}
}

func insertTestPlanningArtifactRef(t *testing.T, store *Store, change domain.Change, kind, schema, summary string) domain.ArtifactRef {
	t.Helper()
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := insertPlanningArtifactRef(context.Background(), tx, change, work.PlanningArtifactWrite{Identity: domain.NewArtifactIdentity([]byte(summary)), MediaType: "application/json", Kind: kind, SchemaVersion: schema, Summary: summary, SourceRevision: change.BaseRevision}, domain.ArtifactRoleOutput, store.now().UTC())
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return ref
}

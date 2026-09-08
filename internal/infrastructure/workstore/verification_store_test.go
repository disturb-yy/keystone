package workstore

import (
	"context"
	"testing"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	executionapp "github.com/disturb-yy/keystone/internal/execution/application"
	"github.com/disturb-yy/keystone/internal/infrastructure/manifest"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestVerificationIntentAssignmentReportAndCommitPrecondition(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, t.TempDir(), "verification-project", "verification-change")
	planRef := advanceTestChangeToTicketize(t, store, change)
	run, err := store.StartTicketizeRun(ctx, work.StartPlanningRunRequest{ChangeID: change.ID, TargetStage: domain.LifecycleStageTicketize, ExpectedChangeVersion: 4, SourceRevision: change.BaseRevision, Actor: "planning", InputArtifactRefIDs: []domain.ArtifactRefID{planRef.ID}})
	if err != nil {
		t.Fatal(err)
	}
	draft := insertTestPlanningArtifactRef(t, store, change, "ticket_draft", "keystone.ticket-draft.v1", "draft")
	commit, err := store.CompleteTicketize(ctx, work.TicketizeCompletionRequest{AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt, ExpectedChangeVersion: 4, SourceRevision: change.BaseRevision, Actor: "planning", PlanArtifactRef: planRef, DraftArtifactRef: draft, GeneratorName: "codex", GeneratorVersion: "ticketize.v1", Candidate: domain.TicketGraphCandidate{Tickets: []domain.TicketDraft{{GenerationKey: "one", Title: "One", Scope: "scope", AcceptanceCriteria: []string{"done"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	change = commit.Change
	executeRequest := executionapp.ExecuteRequest{ProjectID: string(change.ProjectID), ChangeID: string(change.ID), ExpectedVersion: int64(change.Version), RepositoryRoot: change.RepositoryRoot, BaseRevision: change.BaseRevision, WorkspacePath: t.TempDir(), Branch: "keystone/change/verification", RequestKey: "execute-verification", RequestDigest: "execute-verification-digest", Instruction: "intent"}
	begin, err := store.BeginExecution(ctx, executeRequest)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.FinalizeExecution(ctx, executionapp.FinalizeRequest{ExecuteRequest: executeRequest, IntentID: begin.IntentID, PhysicalPath: executeRequest.WorkspacePath, HeadRevision: change.BaseRevision, WorkspacePath: executeRequest.WorkspacePath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE t_ticket_execution_states SET state = 'succeeded' WHERE ticket_id = ?`, session.Tickets[0].TicketID); err != nil {
		t.Fatal(err)
	}
	workerID, secret := "verification-worker", "verification-worker-secret-1234567890"
	if err := store.PrepareWorker(ctx, workerID, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RegisterWorker(ctx, workercontract.Register{WorkerID: workerID, ProtocolVersion: workercontract.ProtocolVersionV1, Capabilities: []string{workercontract.VerificationCapability}}); err != nil {
		t.Fatal(err)
	}
	policy := manifest.ProjectManifestV2{Version: 2, ProjectID: change.ProjectID, VerifyCommands: []manifest.V2Command{{Name: "check", Argv: []string{"go", "test"}, TimeoutSeconds: 30}}, CommitTemplate: "{ticket_title}"}
	if err := store.SaveVerificationPolicySnapshot(ctx, VerificationPolicySnapshotInput{SessionID: session.ID, ProjectID: string(change.ProjectID), ChangeID: string(change.ID), BaseRevision: change.BaseRevision, Manifest: policy}); err != nil {
		t.Fatal(err)
	}
	ticketID := string(session.Tickets[0].TicketID)
	intent, err := store.BeginVerification(ctx, VerificationIntentRequest{ProjectID: string(change.ProjectID), ChangeID: string(change.ID), TicketID: ticketID, ExpectedVersion: int(change.Version), RequestKey: "verify-key", RequestDigest: "verify-digest", InputRevision: change.BaseRevision, CandidateTreeIdentity: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil || intent.Status != "pending" {
		t.Fatalf("verification intent = %+v, err=%v", intent, err)
	}
	assignment, err := store.PullAssignment(ctx, workerID)
	if err != nil || assignment == nil || assignment.Kind != workercontract.AssignmentKindVerify {
		t.Fatalf("verification assignment = %+v, err=%v", assignment, err)
	}
	exitCode := 0
	report := workercontract.Report{
		AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, Attempt: assignment.Attempt, Outcome: "succeeded",
		Verification: &workercontract.VerificationReport{
			IntentID: intent.IntentID, Outcome: "succeeded", CandidateTreeIdentity: assignment.Verification.CandidateTreeIdentity,
			AfterRevision: change.BaseRevision, ReviewSummary: "reviewed",
			Commands: []workercontract.VerificationCommandResult{{Ordinal: 1, Name: "check", Status: "passed", ExitCode: &exitCode}},
			Criteria: []workercontract.VerificationCriterionResult{{TicketID: ticketID, Ordinal: 1, TextSHA256: textSHA256("done"), Outcome: "pass", EvidenceIDs: []string{"command:check:stdout"}}},
		},
	}
	response, err := store.ReportWorkerFor(ctx, workerID, report, nil)
	if err != nil || response.Disposition != "accepted" {
		t.Fatalf("verification report = %+v, err=%v", response, err)
	}
	current, err := store.FindChange(ctx, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	precondition, err := store.PrepareCommitIntent(ctx, CommitIntentRequest{ProjectID: string(change.ProjectID), ChangeID: string(change.ID), TicketID: ticketID, ExpectedVersion: int(current.Version), RequestKey: "commit-key", RequestDigest: "commit-digest", InputRevision: change.BaseRevision, CandidateTreeIdentity: assignment.Verification.CandidateTreeIdentity})
	if err != nil {
		t.Fatal(err)
	}
	if precondition.Message == "" || precondition.KeystoneCommitID == "" {
		t.Fatalf("commit precondition = %+v", precondition)
	}
}

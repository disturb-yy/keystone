package workstore

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	executionapp "github.com/disturb-yy/keystone/internal/execution/application"
	"github.com/disturb-yy/keystone/internal/infrastructure/artifact"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestExecutionDispatchClaimAndEvidenceCompletion(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, change := createTestChange(t, store, "/tmp/execution-store", "execution-project", "execution-change")
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

	request := executionapp.ExecuteRequest{ProjectID: string(change.ProjectID), ChangeID: string(change.ID), ExpectedVersion: int64(change.Version), RepositoryRoot: change.RepositoryRoot, BaseRevision: change.BaseRevision, WorkspacePath: "/tmp/execution-store-workspace", Branch: "keystone/change/execution", RequestKey: "execute-1", RequestDigest: "digest-1", Instruction: "intent"}
	begin, err := store.BeginExecution(ctx, request)
	if err != nil || !begin.NeedsProvision {
		t.Fatalf("begin execution = %+v, err=%v", begin, err)
	}
	session, err := store.FinalizeExecution(ctx, executionapp.FinalizeRequest{ExecuteRequest: request, IntentID: begin.IntentID, PhysicalPath: "/tmp/execution-store-workspace", HeadRevision: change.BaseRevision, WorkspacePath: request.WorkspacePath})
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "waiting" || len(session.Tickets) != 1 {
		t.Fatalf("session = %+v", session)
	}

	workerID := "execution-worker"
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
	assignment, err := store.PullAssignment(ctx, workerID)
	if err != nil || assignment == nil {
		t.Fatalf("pull assignment = %+v, err=%v", assignment, err)
	}
	claim, err := store.ClaimExecution(ctx, workerID, workercontract.ClaimRequest{AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, RuntimeClaimID: "claim-1"})
	if err != nil || claim.Disposition != "claimed" {
		t.Fatalf("claim = %+v, err=%v", claim, err)
	}
	emptyDigest := sha256.Sum256(nil)
	if err := store.RecordExecutionSnapshot(ctx, assignment.AgentRunID, WorkspaceSnapshotInput{Phase: "pre", HeadRevision: change.BaseRevision, Branch: request.Branch, DiffSHA256: hex.EncodeToString(emptyDigest[:])}); err != nil {
		t.Fatal(err)
	}
	diff := []byte("diff --git a/README.md b/README.md\n+changed\n")
	diffDigest := sha256.Sum256(diff)
	if err := store.RecordExecutionSnapshot(ctx, assignment.AgentRunID, WorkspaceSnapshotInput{Phase: "post", HeadRevision: change.BaseRevision, Branch: request.Branch, ChangedFiles: []string{"README.md"}, DiffSHA256: hex.EncodeToString(diffDigest[:]), DiffBytes: int64(len(diff))}); err != nil {
		t.Fatal(err)
	}
	artifactStore, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	response, err := store.ReportWorkerFor(ctx, workerID, workercontract.Report{AgentRunID: assignment.AgentRunID, LeaseToken: assignment.LeaseToken, Attempt: assignment.Attempt, Outcome: "succeeded", ExitCode: &exitCode, AfterRevision: change.BaseRevision, Artifacts: []workercontract.Artifact{
		{Kind: "diff", ContentBase64: base64.StdEncoding.EncodeToString(diff), SHA256: hex.EncodeToString(diffDigest[:]), SizeBytes: int64(len(diff))},
		{Kind: "changed_files", ContentBase64: base64.StdEncoding.EncodeToString([]byte("README.md\n")), SHA256: digestText([]byte("README.md\n")), SizeBytes: int64(len("README.md\n"))},
	}}, artifactStore)
	if err != nil {
		t.Fatal(err)
	}
	if response.Disposition != "accepted" {
		t.Fatalf("execution report = %+v", response)
	}
	read, err := store.FindExecution(ctx, string(change.ID))
	if err != nil {
		t.Fatal(err)
	}
	if read.Status != "completed" || read.Tickets[0].State != "succeeded" {
		t.Fatalf("execution read = %+v", read)
	}
	if err := store.FenceExecutionAssignment(ctx, assignment.AgentRunID, "late_snapshot"); err != nil {
		t.Fatalf("replay execution observation fence = %v", err)
	}
}

func digestText(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

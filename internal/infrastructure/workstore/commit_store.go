package workstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	governancedomain "github.com/disturb-yy/keystone/internal/governance/domain"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

// CommitPrecondition 是 Git 写入前由 authority 固定的 parent/tree/evidence。
type CommitPrecondition struct {
	ProjectID              string
	ChangeID               string
	TicketID               string
	WorkspacePath          string
	Branch                 string
	ExpectedParent         string
	CandidateTreeIdentity  string
	VerificationEvidenceID string
	CommitTemplateDigest   string
	CommitTemplate         string
	Message                string
	KeystoneCommitID       string
	IntentID               string
	Status                 string
	GitOID                 string
}

// PrepareCommitIntent 只在 Verify PASS 后建立唯一 CommitIntent。
func (s *Store) PrepareCommitIntent(ctx context.Context, request CommitIntentRequest) (CommitPrecondition, error) {
	if ctx == nil || strings.TrimSpace(request.ProjectID) == "" || strings.TrimSpace(request.ChangeID) == "" || strings.TrimSpace(request.TicketID) == "" || strings.TrimSpace(request.RequestKey) == "" || strings.TrimSpace(request.RequestDigest) == "" || request.ExpectedVersion < 1 {
		return CommitPrecondition{}, fmt.Errorf("prepare commit: %w", domain.ErrInvalidRequest)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommitPrecondition{}, fmt.Errorf("begin commit intent: %w", ErrCommitUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var existing CommitPrecondition
	var existingDigest string
	err = tx.QueryRowContext(ctx, `SELECT intent_id, project_id, change_id, ticket_id, request_digest, expected_parent, candidate_tree_identity, verification_evidence_id, commit_template_digest, keystone_commit_id, status, git_oid FROM t_commit_intents WHERE request_key = ?`, request.RequestKey).Scan(&existing.IntentID, &existing.ProjectID, &existing.ChangeID, &existing.TicketID, &existingDigest, &existing.ExpectedParent, &existing.CandidateTreeIdentity, &existing.VerificationEvidenceID, &existing.CommitTemplateDigest, &existing.KeystoneCommitID, &existing.Status, &existing.GitOID)
	if err == nil {
		if existing.ProjectID != request.ProjectID || existing.ChangeID != request.ChangeID || existing.TicketID != request.TicketID || existingDigest != request.RequestDigest {
			return CommitPrecondition{}, domain.ErrIdempotencyConflict
		}
		if err := readCommitPreconditionDetails(ctx, tx, &existing); err != nil {
			return CommitPrecondition{}, err
		}
		if err := tx.Commit(); err != nil {
			return CommitPrecondition{}, fmt.Errorf("replay commit intent: %w", ErrCommitUnavailable)
		}
		committed = true
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CommitPrecondition{}, fmt.Errorf("read commit intent: %w", ErrCommitUnavailable)
	}
	change, err := readChange(ctx, tx, domain.ChangeID(request.ChangeID))
	if err != nil {
		return CommitPrecondition{}, err
	}
	if string(change.ProjectID) != request.ProjectID || int(change.Version) != request.ExpectedVersion || change.Status != domain.ChangeStatusActive || change.Stage != domain.LifecycleStageExecute {
		return CommitPrecondition{}, fmt.Errorf("commit change precondition: %w", ErrVerificationConflict)
	}
	var precondition CommitPrecondition
	var title string
	err = tx.QueryRowContext(ctx, `SELECT workspace.workspace_path, workspace.branch, workspace.input_revision, evidence.evidence_id, evidence.candidate_tree_identity, snapshot.commit_template_digest, snapshot.commit_template, ticket.title FROM t_workspaces workspace JOIN t_verification_evidence evidence ON evidence.intent_id IN (SELECT intent_id FROM t_verification_intents WHERE change_id = workspace.change_id) JOIN t_verification_intents verify ON verify.intent_id = evidence.intent_id JOIN t_verification_policy_snapshots snapshot ON snapshot.snapshot_id = verify.policy_snapshot_id JOIN t_tickets ticket ON ticket.ticket_id = verify.ticket_id JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id WHERE workspace.change_id = ? AND verify.ticket_id = ? AND verify.final = 0 AND evidence.outcome = 'pass' AND graph.change_id = ? ORDER BY verify.attempt DESC, verify.created_at DESC, verify.intent_id DESC LIMIT 1`, request.ChangeID, request.TicketID, request.ChangeID).Scan(&precondition.WorkspacePath, &precondition.Branch, &precondition.ExpectedParent, &precondition.VerificationEvidenceID, &precondition.CandidateTreeIdentity, &precondition.CommitTemplateDigest, &precondition.CommitTemplate, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return CommitPrecondition{}, fmt.Errorf("commit verification evidence: %w", ErrVerificationConflict)
	}
	if err != nil {
		return CommitPrecondition{}, fmt.Errorf("read commit verification evidence: %w", ErrCommitUnavailable)
	}
	var currentInput string
	if err := tx.QueryRowContext(ctx, `SELECT input_revision FROM t_workspaces WHERE change_id = ?`, request.ChangeID).Scan(&currentInput); err != nil || currentInput != precondition.ExpectedParent {
		return CommitPrecondition{}, fmt.Errorf("commit input revision: %w", ErrVerificationConflict)
	}
	precondition.ProjectID, precondition.ChangeID, precondition.TicketID = request.ProjectID, request.ChangeID, request.TicketID
	precondition.KeystoneCommitID, precondition.IntentID = id.New(), id.New()
	precondition.Status = governancedomain.CommitIntentPending
	if request.InputRevision != "" && request.InputRevision != precondition.ExpectedParent || request.CandidateTreeIdentity != "" && request.CandidateTreeIdentity != precondition.CandidateTreeIdentity {
		return CommitPrecondition{}, fmt.Errorf("commit candidate changed: %w", ErrVerificationConflict)
	}
	message, err := governancedomain.RenderCommitMessage(precondition.CommitTemplate, request.ChangeID, request.TicketID, title)
	if err != nil {
		return CommitPrecondition{}, err
	}
	precondition.Message = message + "\n\nKeystone-Change-ID: " + request.ChangeID + "\nKeystone-Ticket-ID: " + request.TicketID + "\nKeystone-Commit-ID: " + precondition.KeystoneCommitID
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_commit_intents (intent_id, project_id, change_id, ticket_id, request_key, request_digest, expected_parent, candidate_tree_identity, verification_evidence_id, commit_template_digest, keystone_commit_id, status, message, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?)`, precondition.IntentID, request.ProjectID, request.ChangeID, request.TicketID, request.RequestKey, request.RequestDigest, precondition.ExpectedParent, precondition.CandidateTreeIdentity, precondition.VerificationEvidenceID, precondition.CommitTemplateDigest, precondition.KeystoneCommitID, precondition.Message, stamp(now), stamp(now)); err != nil {
		return CommitPrecondition{}, fmt.Errorf("insert commit intent: %w", ErrCommitUnavailable)
	}
	if err := insertGovernanceEventTx(ctx, tx, request.ProjectID, request.ChangeID, "commit_intent_created", precondition.IntentID, map[string]any{"ticket_id": request.TicketID, "keystone_commit_id": precondition.KeystoneCommitID}, stamp(now)); err != nil {
		return CommitPrecondition{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommitPrecondition{}, fmt.Errorf("commit commit intent: %w", ErrCommitUnavailable)
	}
	committed = true
	return precondition, nil
}

// CommitIntentRequest 是 Commit command 的身份和受控消息。
type CommitIntentRequest struct {
	ProjectID             string
	ChangeID              string
	TicketID              string
	ExpectedVersion       int
	RequestKey            string
	RequestDigest         string
	Message               string
	InputRevision         string
	CandidateTreeIdentity string
}

// FinalizeCommit 将 Git 已完成结果与 CommitIntent 原子收口。
func (s *Store) FinalizeCommit(ctx context.Context, request FinalizeCommitRequest) error {
	if ctx == nil || request.IntentID == "" || request.GitOID == "" || request.AfterRevision == "" {
		return fmt.Errorf("finalize commit: %w", ErrCommitUnavailable)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finalize commit: %w", ErrCommitUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var projectID, changeID, ticketID, parent, tree, evidenceID, templateDigest, key, status, keystoneID, intentMessage, existingGitOID string
	if err := tx.QueryRowContext(ctx, `SELECT project_id, change_id, ticket_id, expected_parent, candidate_tree_identity, verification_evidence_id, commit_template_digest, request_key, status, keystone_commit_id, message, git_oid FROM t_commit_intents WHERE intent_id = ?`, request.IntentID).Scan(&projectID, &changeID, &ticketID, &parent, &tree, &evidenceID, &templateDigest, &key, &status, &keystoneID, &intentMessage, &existingGitOID); err != nil {
		return fmt.Errorf("read commit intent: %w", ErrCommitUnavailable)
	}
	if status == governancedomain.CommitIntentCommitted {
		if existingGitOID != request.GitOID || parent != request.ParentRevision || tree != request.TreeIdentity || (request.Message != "" && request.Message != intentMessage) {
			return fmt.Errorf("replay commit result does not match intent: %w", ErrCommitUnavailable)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("replay committed intent: %w", ErrCommitUnavailable)
		}
		committed = true
		return nil
	}
	if status != governancedomain.CommitIntentPending || parent != request.ParentRevision || tree != request.TreeIdentity || request.AfterRevision != request.GitOID || (request.Message != "" && request.Message != intentMessage) {
		return fmt.Errorf("commit result does not match intent: %w", ErrCommitUnavailable)
	}
	now := s.now().UTC()
	message := request.Message
	if message == "" {
		message = intentMessage
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_keystone_commits (keystone_commit_id, project_id, change_id, ticket_id, parent_revision, after_revision, tree_identity, git_oid, verification_evidence_id, commit_template_digest, message, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, keystoneID, projectID, changeID, ticketID, parent, request.AfterRevision, tree, request.GitOID, evidenceID, templateDigest, message, stamp(now)); err != nil {
		return fmt.Errorf("persist keystone commit: %w", ErrCommitUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_commit_intents SET status = 'committed', git_oid = ?, updated_at = ? WHERE intent_id = ? AND status = 'pending'`, request.GitOID, stamp(now), request.IntentID); err != nil {
		return fmt.Errorf("complete commit intent: %w", ErrCommitUnavailable)
	}
	if err := insertGovernanceEventTx(ctx, tx, projectID, changeID, "keystone_commit_recorded", keystoneID, map[string]any{"ticket_id": ticketID, "git_oid": request.GitOID}, stamp(now)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_workspaces SET input_revision = ?, updated_at = ? WHERE change_id = ? AND input_revision = ?`, request.AfterRevision, stamp(now), changeID, parent); err != nil {
		return fmt.Errorf("advance workspace input revision: %w", ErrCommitUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET input_revision = ?, updated_at = ? WHERE change_id = ? AND input_revision = ?`, request.AfterRevision, stamp(now), changeID, parent); err != nil {
		return fmt.Errorf("advance execution input revision: %w", ErrCommitUnavailable)
	}
	change, err := readChange(ctx, tx, domain.ChangeID(changeID))
	if err != nil {
		return err
	}
	var total, commits int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_tickets ticket JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id WHERE graph.change_id = ?`, changeID).Scan(&total); err != nil {
		return fmt.Errorf("count commit tickets: %w", ErrCommitUnavailable)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_keystone_commits WHERE change_id = ?`, changeID).Scan(&commits); err != nil {
		return fmt.Errorf("count completed commits: %w", ErrCommitUnavailable)
	}
	if total > 0 && total == commits && change.Stage == domain.LifecycleStageExecute && change.Status == domain.ChangeStatusActive {
		next := change
		next.Stage = domain.LifecycleStageVerify
		next.Version++
		next.UpdatedAt = now
		if err := updateChangeStage(ctx, tx, change, next, now); err != nil {
			return fmt.Errorf("advance verify stage: %w", ErrCommitUnavailable)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit commit authority: %w", ErrCommitUnavailable)
	}
	committed = true
	_ = key
	return nil
}

// FinalizeCommitRequest 是 Git 对账后的 immutable identity。
type FinalizeCommitRequest struct {
	IntentID       string
	GitOID         string
	ParentRevision string
	AfterRevision  string
	TreeIdentity   string
	Message        string
}

func readCommitPreconditionDetails(ctx context.Context, tx *sql.Tx, result *CommitPrecondition) error {
	return tx.QueryRowContext(ctx, `SELECT workspace.workspace_path, workspace.branch, intents.message FROM t_workspaces workspace JOIN t_commit_intents intents ON intents.change_id = workspace.change_id WHERE workspace.change_id = ? AND intents.intent_id = ?`, result.ChangeID, result.IntentID).Scan(&result.WorkspacePath, &result.Branch, &result.Message)
}

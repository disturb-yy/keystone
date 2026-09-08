package workstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

// ExecutionReadModel 是 Dashboard/CLI 可读取的有界 M8 执行摘要。
type ExecutionReadModel struct {
	ExecutionSessionID string
	ChangeID           string
	Status             string
	WorkspaceBranch    string
	BaseRevision       string
	InputRevision      string
	Stage              string
	ChangeStatus       string
	Version            int
	Tickets            []ExecutionTicketSummary
	PendingIntentIDs   []string
	CandidateRevision  string
	FinalVerification  string
	FinalEvidenceID    string
}

// ExecutionTicketSummary 只暴露 gate、验证和 commit 身份，不暴露路径、Lease 或原始输出。
type ExecutionTicketSummary struct {
	TicketID                      string
	Ordinal                       int
	Title                         string
	State                         string
	GateStatus                    string
	VerificationStatus            string
	CommitStatus                  string
	VerificationEvidenceID        string
	VerificationCommandStatuses   []string
	VerificationCriterionOutcomes []string
	InputRevision                 string
	CommitBeforeRevision          string
	CommitAfterRevision           string
}

// ReadExecution 返回 Change 当前安全读模型。
func (s *Store) ReadExecution(ctx context.Context, changeID string) (ExecutionReadModel, error) {
	if ctx == nil || changeID == "" {
		return ExecutionReadModel{}, domain.ErrInvalidRequest
	}
	var result ExecutionReadModel
	var sessionStatus, stage, changeStatus string
	if err := s.db.QueryRowContext(ctx, `SELECT session.session_id, session.status, session.branch, session.base_revision, session.input_revision, change.stage, change.status, change.version FROM t_execution_sessions session JOIN t_changes change ON change.change_id = session.change_id WHERE session.change_id = ?`, changeID).Scan(&result.ExecutionSessionID, &sessionStatus, &result.WorkspaceBranch, &result.BaseRevision, &result.InputRevision, &stage, &changeStatus, &result.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExecutionReadModel{}, domain.ErrChangeNotFound
		}
		return ExecutionReadModel{}, fmt.Errorf("read execution model: %w", ErrVerificationUnavailable)
	}
	result.ChangeID, result.Status, result.Stage, result.ChangeStatus = changeID, sessionStatus, stage, changeStatus
	rows, err := s.db.QueryContext(ctx, `SELECT ticket.ticket_id, ticket.ordinal, ticket.title, state.state, COALESCE(vi.intent_id, ''), COALESCE(vi.status, ''), CASE WHEN kc.keystone_commit_id IS NULL THEN '' ELSE 'committed' END, COALESCE(vi.input_revision, workspace.input_revision), COALESCE(kc.parent_revision, ''), COALESCE(kc.after_revision, '') FROM t_tickets ticket JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id JOIN t_ticket_execution_states state ON state.graph_id = ticket.graph_id AND state.ticket_id = ticket.ticket_id LEFT JOIN t_verification_intents vi ON vi.intent_id = (SELECT latest.intent_id FROM t_verification_intents latest WHERE latest.change_id = graph.change_id AND latest.ticket_id = ticket.ticket_id AND latest.final = 0 ORDER BY latest.attempt DESC, latest.created_at DESC LIMIT 1) LEFT JOIN t_keystone_commits kc ON kc.change_id = graph.change_id AND kc.ticket_id = ticket.ticket_id LEFT JOIN t_workspaces workspace ON workspace.change_id = graph.change_id WHERE graph.change_id = ? ORDER BY ticket.ordinal`, changeID)
	if err != nil {
		return ExecutionReadModel{}, fmt.Errorf("list execution tickets: %w", ErrVerificationUnavailable)
	}
	type ticketWithIntent struct {
		ticket   ExecutionTicketSummary
		intentID string
	}
	loadedTickets := make([]ticketWithIntent, 0)
	for rows.Next() {
		var ticket ExecutionTicketSummary
		var intentID string
		if err := rows.Scan(&ticket.TicketID, &ticket.Ordinal, &ticket.Title, &ticket.State, &intentID, &ticket.VerificationStatus, &ticket.CommitStatus, &ticket.InputRevision, &ticket.CommitBeforeRevision, &ticket.CommitAfterRevision); err != nil {
			_ = rows.Close()
			return ExecutionReadModel{}, fmt.Errorf("scan execution ticket: %w", ErrVerificationUnavailable)
		}
		loadedTickets = append(loadedTickets, ticketWithIntent{ticket: ticket, intentID: intentID})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return ExecutionReadModel{}, fmt.Errorf("read execution tickets: %w", ErrVerificationUnavailable)
	}
	if err := rows.Close(); err != nil {
		return ExecutionReadModel{}, fmt.Errorf("close execution tickets: %w", ErrVerificationUnavailable)
	}
	result.Tickets = make([]ExecutionTicketSummary, 0, len(loadedTickets))
	for _, loaded := range loadedTickets {
		ticket := loaded.ticket
		intentID := loaded.intentID
		if intentID != "" {
			_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(evidence_id, '') FROM t_verification_evidence WHERE intent_id = ?`, intentID).Scan(&ticket.VerificationEvidenceID)
			commandRows, commandErr := s.db.QueryContext(ctx, `SELECT status FROM t_verification_command_results WHERE intent_id = ? ORDER BY ordinal`, intentID)
			if commandErr != nil {
				return ExecutionReadModel{}, fmt.Errorf("read verification command summaries: %w", ErrVerificationUnavailable)
			}
			for commandRows.Next() {
				var status string
				if err := commandRows.Scan(&status); err != nil {
					_ = commandRows.Close()
					return ExecutionReadModel{}, fmt.Errorf("scan verification command summary: %w", ErrVerificationUnavailable)
				}
				ticket.VerificationCommandStatuses = append(ticket.VerificationCommandStatuses, status)
			}
			if err := commandRows.Err(); err != nil {
				_ = commandRows.Close()
				return ExecutionReadModel{}, fmt.Errorf("read verification command summaries: %w", ErrVerificationUnavailable)
			}
			_ = commandRows.Close()
			criterionRows, criterionErr := s.db.QueryContext(ctx, `SELECT outcome FROM t_verification_criterion_results WHERE intent_id = ? ORDER BY ticket_id, ordinal`, intentID)
			if criterionErr != nil {
				return ExecutionReadModel{}, fmt.Errorf("read verification criterion summaries: %w", ErrVerificationUnavailable)
			}
			for criterionRows.Next() {
				var outcome string
				if err := criterionRows.Scan(&outcome); err != nil {
					_ = criterionRows.Close()
					return ExecutionReadModel{}, fmt.Errorf("scan verification criterion summary: %w", ErrVerificationUnavailable)
				}
				ticket.VerificationCriterionOutcomes = append(ticket.VerificationCriterionOutcomes, outcome)
			}
			if err := criterionRows.Err(); err != nil {
				_ = criterionRows.Close()
				return ExecutionReadModel{}, fmt.Errorf("read verification criterion summaries: %w", ErrVerificationUnavailable)
			}
			_ = criterionRows.Close()
		}
		ticket.GateStatus = ticket.State
		if ticket.VerificationStatus == "" && ticket.CommitStatus == "" && ticket.State == "succeeded" {
			ticket.GateStatus = "verify_pending"
		}
		result.Tickets = append(result.Tickets, ticket)
	}
	intentRows, err := s.db.QueryContext(ctx, `SELECT intent_id FROM t_verification_intents WHERE change_id = ? AND status IN ('pending', 'running') ORDER BY created_at, intent_id`, changeID)
	if err != nil {
		return ExecutionReadModel{}, fmt.Errorf("read pending verification intents: %w", ErrVerificationUnavailable)
	}
	defer intentRows.Close()
	for intentRows.Next() {
		var intentID string
		if err := intentRows.Scan(&intentID); err != nil {
			return ExecutionReadModel{}, fmt.Errorf("scan pending verification intent: %w", ErrVerificationUnavailable)
		}
		result.PendingIntentIDs = append(result.PendingIntentIDs, intentID)
	}
	if err := intentRows.Err(); err != nil {
		_ = intentRows.Close()
		return ExecutionReadModel{}, fmt.Errorf("read pending verification intents: %w", ErrVerificationUnavailable)
	}
	if err := intentRows.Close(); err != nil {
		return ExecutionReadModel{}, fmt.Errorf("close pending verification intents: %w", ErrVerificationUnavailable)
	}
	_ = s.db.QueryRowContext(ctx, `SELECT revision FROM t_candidate_revisions WHERE change_id = ?`, changeID).Scan(&result.CandidateRevision)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(status, '') FROM t_verification_intents WHERE change_id = ? AND final = 1 ORDER BY attempt DESC, created_at DESC LIMIT 1`, changeID).Scan(&result.FinalVerification)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(evidence_id, '') FROM t_verification_evidence WHERE intent_id = (SELECT intent_id FROM t_verification_intents WHERE change_id = ? AND final = 1 ORDER BY attempt DESC, created_at DESC LIMIT 1)`, changeID).Scan(&result.FinalEvidenceID)
	return result, nil
}

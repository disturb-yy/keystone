package workstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

type executionFenceItem struct {
	runID           string
	ticketID        string
	authorizationID string
}

// FenceExecutionAssignment 将无法建立可信 Snapshot 的已 Claim Assignment 收敛为人工处理。
// 该入口只处理执行事实，不删除或回滚 Worktree；重复调用只保留第一次围栏结果。
func (s *Store) FenceExecutionAssignment(ctx context.Context, agentRunID, reason string) error {
	if ctx == nil || agentRunID == "" {
		return ErrWorkerLeaseInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin execution observation fence: %w", ErrWorkerUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var runID, changeID, leaseID, ticketID, authorizationID, sessionID, epochID, runStatus, leaseState string
	err = tx.QueryRowContext(ctx, `
SELECT run.agent_run_id, run.change_id, lease.lease_id,
       state.ticket_id, state.authorization_id, authorization.session_id,
       authorization.epoch_id, run.status, lease.state
FROM t_agent_runs run
JOIN t_worker_leases lease ON lease.agent_run_id = run.agent_run_id
JOIN t_ticket_execution_states state ON state.agent_run_id = run.agent_run_id
JOIN t_execution_authorizations authorization ON authorization.authorization_id = state.authorization_id
	WHERE run.agent_run_id = ?`, agentRunID).Scan(&runID, &changeID, &leaseID, &ticketID, &authorizationID, &sessionID, &epochID, &runStatus, &leaseState)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrWorkerLeaseInvalid
	}
	if err != nil {
		return fmt.Errorf("read execution observation fence: %w", ErrWorkerUnavailable)
	}
	if runStatus != "running" || leaseState != "active" {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit execution observation replay: %w", ErrWorkerUnavailable)
		}
		committed = true
		s.forgetLeaseToken(leaseID)
		return nil
	}
	change, err := readChange(ctx, tx, domain.ChangeID(changeID))
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'revoked', claim_state = 'fenced' WHERE lease_id = ? AND state = 'active'`, leaseID); err != nil {
		return fmt.Errorf("fence execution observation lease: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = 'failed', completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, stamp(now), runID); err != nil {
		return fmt.Errorf("complete execution observation run: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_ticket_execution_states SET state = 'human_required', updated_at = ? WHERE ticket_id = ? AND agent_run_id = ? AND state = 'assigned'`, stamp(now), ticketID, runID); err != nil {
		return fmt.Errorf("fence execution observation ticket: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_authorizations SET status = 'human_required', updated_at = ? WHERE authorization_id = ? AND status IN ('authorized', 'assigned', 'claimed')`, stamp(now), authorizationID); err != nil {
		return fmt.Errorf("fence execution observation authorization: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_dispatch_epochs SET status = 'fenced', fenced_at = ? WHERE epoch_id = ? AND status IN ('queued', 'active')`, stamp(now), epochID); err != nil {
		return fmt.Errorf("fence execution observation epoch: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET status = 'human_required', updated_at = ? WHERE session_id = ? AND status NOT IN ('completed', 'cancelled')`, stamp(now), sessionID); err != nil {
		return fmt.Errorf("fence execution observation session: %w", ErrWorkerUnavailable)
	}
	runIdentity := domain.AgentRunID(runID)
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, "execution_fence:"+reason, now, &runIdentity, nil, nil); err != nil {
		return fmt.Errorf("record execution observation completion: %w", ErrWorkerUnavailable)
	}
	if change.Status == domain.ChangeStatusActive {
		next, transitionErr := change.EnterHumanRequired()
		if transitionErr != nil {
			return fmt.Errorf("fence execution observation change: %w", transitionErr)
		}
		next.UpdatedAt = now
		if err := updateChangeStatus(ctx, tx, change, next, now); err != nil {
			return fmt.Errorf("update execution observation change: %w", err)
		}
		if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.ChangeHumanRequiredType, "execution_fence:"+reason, now, &runIdentity, nil, nil); err != nil {
			return fmt.Errorf("record execution observation recovery: %w", ErrWorkerUnavailable)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit execution observation fence: %w", ErrWorkerUnavailable)
	}
	committed = true
	s.forgetLeaseToken(leaseID)
	return nil
}

// fenceExecutionTx 将 Pause/Cancel 竞争窗口内的执行 Lease、Ticket 和 Session 一起围栏。
// Workspace 保留在原处；该函数不执行任何 Git 清理或回滚。
func fenceExecutionTx(ctx context.Context, tx *sql.Tx, change domain.Change, reason string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `
SELECT state.agent_run_id, state.ticket_id, state.authorization_id
FROM t_ticket_execution_states state
JOIN t_execution_authorizations authorization ON authorization.authorization_id = state.authorization_id
JOIN t_execution_sessions session ON session.session_id = authorization.session_id
JOIN t_agent_runs run ON run.agent_run_id = state.agent_run_id
WHERE session.change_id = ? AND run.status = 'running' AND state.state = 'assigned'`, change.ID)
	if err != nil {
		return fmt.Errorf("list execution fence runs: %w", err)
	}
	items := make([]executionFenceItem, 0)
	for rows.Next() {
		var item executionFenceItem
		if err := rows.Scan(&item.runID, &item.ticketID, &item.authorizationID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan execution fence run: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close execution fence runs: %w", err)
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'revoked', claim_state = 'fenced' WHERE agent_run_id = ? AND state IN ('active', 'expired')`, item.runID); err != nil {
			return fmt.Errorf("fence execution lease: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = 'failed', completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, stamp(now), item.runID); err != nil {
			return fmt.Errorf("fence execution run: %w", err)
		}
		nextTicketState := "pending"
		if reason == "cancel" {
			nextTicketState = "human_required"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_ticket_execution_states SET state = ?, updated_at = ? WHERE ticket_id = ? AND agent_run_id = ?`, nextTicketState, stamp(now), item.ticketID, item.runID); err != nil {
			return fmt.Errorf("fence execution ticket: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_execution_authorizations SET status = 'fenced', updated_at = ? WHERE authorization_id = ?`, stamp(now), item.authorizationID); err != nil {
			return fmt.Errorf("fence execution authorization: %w", err)
		}
		runID := domain.AgentRunID(item.runID)
		if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, "execution_fence:"+reason, now, &runID, nil, nil); err != nil {
			return fmt.Errorf("record execution fence completion: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_dispatch_epochs SET status = 'fenced', fenced_at = ? WHERE session_id IN (SELECT session_id FROM t_execution_sessions WHERE change_id = ?) AND status IN ('queued', 'active')`, stamp(now), change.ID); err != nil {
		return fmt.Errorf("fence execution epochs: %w", err)
	}
	nextSessionStatus := "waiting"
	if reason == "cancel" {
		nextSessionStatus = "cancelled"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET status = ?, updated_at = ? WHERE change_id = ? AND status <> 'completed'`, nextSessionStatus, stamp(now), change.ID); err != nil {
		return fmt.Errorf("fence execution session: %w", err)
	}
	return nil
}

func hasExecutionSession(ctx context.Context, tx *sql.Tx, changeID domain.ChangeID) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_execution_sessions WHERE change_id = ?`, changeID).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect execution session: %w", err)
	}
	return count != 0, nil
}

// fenceExecutionRunTx 收敛 Daemon/Worker 重启后已完成失败的执行 run。
func fenceExecutionRunTx(ctx context.Context, tx *sql.Tx, runID domain.AgentRunID, now time.Time) error {
	var ticketID, authorizationID, sessionID, epochID string
	err := tx.QueryRowContext(ctx, `SELECT state.ticket_id, state.authorization_id, authorization.session_id, authorization.epoch_id FROM t_ticket_execution_states state JOIN t_execution_authorizations authorization ON authorization.authorization_id = state.authorization_id WHERE state.agent_run_id = ?`, runID).Scan(&ticketID, &authorizationID, &sessionID, &epochID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read restarted execution state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_ticket_execution_states SET state = 'human_required', updated_at = ? WHERE ticket_id = ? AND agent_run_id = ? AND state = 'assigned'`, stamp(now), ticketID, runID); err != nil {
		return fmt.Errorf("fence restarted execution ticket: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_authorizations SET status = 'human_required', updated_at = ? WHERE authorization_id = ?`, stamp(now), authorizationID); err != nil {
		return fmt.Errorf("fence restarted execution authorization: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_dispatch_epochs SET status = 'fenced', fenced_at = ? WHERE epoch_id = ? AND status IN ('queued', 'active')`, stamp(now), epochID); err != nil {
		return fmt.Errorf("fence restarted execution epoch: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET status = 'human_required', updated_at = ? WHERE session_id = ? AND status NOT IN ('completed', 'cancelled')`, stamp(now), sessionID); err != nil {
		return fmt.Errorf("fence restarted execution session: %w", err)
	}
	return nil
}

func reconcileExpiredExecutionLeases(ctx context.Context, tx *sql.Tx, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `
SELECT lease.agent_run_id
FROM t_worker_leases lease
JOIN t_agent_runs run ON run.agent_run_id = lease.agent_run_id
JOIN t_ticket_execution_states state ON state.agent_run_id = run.agent_run_id
WHERE lease.state = 'active' AND lease.expires_at <= ? AND run.status = 'running'
ORDER BY run.started_at, run.agent_run_id`, stamp(now))
	if err != nil {
		return fmt.Errorf("list expired execution leases: %w", err)
	}
	var runIDs []domain.AgentRunID
	for rows.Next() {
		var runID domain.AgentRunID
		if err := rows.Scan(&runID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan expired execution lease: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close expired execution leases: %w", err)
	}
	for _, runID := range runIDs {
		run, err := readAgentRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		if run.Status != domain.AgentRunStatusRunning {
			continue
		}
		change, err := readChange(ctx, tx, run.ChangeID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired', claim_state = 'fenced' WHERE agent_run_id = ? AND state = 'active'`, runID); err != nil {
			return fmt.Errorf("expire execution lease: %w", err)
		}
		if err := run.Complete(domain.AgentRunOutcomeFailed, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = 'failed', completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, stamp(now), run.ID); err != nil {
			return fmt.Errorf("complete expired execution run: %w", err)
		}
		actor := "execution_lease_expired"
		if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, actor, now, &run.ID, nil, nil); err != nil {
			return err
		}
		if err := fenceExecutionRunTx(ctx, tx, run.ID, now); err != nil {
			return err
		}
		if change.Status == domain.ChangeStatusActive {
			next, err := change.EnterHumanRequired()
			if err != nil {
				return err
			}
			next.UpdatedAt = now
			if err := updateChangeStatus(ctx, tx, change, next, now); err != nil {
				return err
			}
			if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.ChangeHumanRequiredType, actor, now, &run.ID, nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) reconcileExpiredExecutionLeases(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin expired execution reconciliation: %w", ErrWorkerUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := reconcileExpiredExecutionLeases(ctx, tx, s.now().UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit expired execution reconciliation: %w", ErrWorkerUnavailable)
	}
	committed = true
	return s.forgetTerminalLeaseTokens(ctx)
}

// resumeExecutionTx 为 Pause 后的 Session 创建新的 DispatchEpoch；不会改写 Workspace。
func resumeExecutionTx(ctx context.Context, tx *sql.Tx, changeID domain.ChangeID, now time.Time) error {
	var sessionID, epochID, epochStatus string
	err := tx.QueryRowContext(ctx, `SELECT session_id, current_epoch_id FROM t_execution_sessions WHERE change_id = ? AND status = 'waiting'`, changeID).Scan(&sessionID, &epochID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read execution resume session: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT status FROM t_execution_dispatch_epochs WHERE epoch_id = ?`, epochID).Scan(&epochStatus); err != nil {
		return fmt.Errorf("read execution resume epoch: %w", err)
	}
	if epochStatus != "fenced" {
		return nil
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT sequence FROM t_execution_dispatch_epochs WHERE epoch_id = ?`, epochID).Scan(&sequence); err != nil {
		return fmt.Errorf("read execution epoch sequence: %w", err)
	}
	nextID := id.New()
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_execution_dispatch_epochs (epoch_id, session_id, sequence, status, created_at) VALUES (?, ?, ?, 'queued', ?)`, nextID, sessionID, sequence+1, stamp(now)); err != nil {
		return fmt.Errorf("create execution resume epoch: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET current_epoch_id = ?, status = 'waiting', updated_at = ? WHERE session_id = ?`, nextID, stamp(now), sessionID); err != nil {
		return fmt.Errorf("activate execution resume session: %w", err)
	}
	return nil
}

// retryExecutionTx 只释放失败 Ticket；已成功 Ticket 和 Workspace 保持不变。
func retryExecutionTx(ctx context.Context, tx *sql.Tx, changeID domain.ChangeID, now time.Time) error {
	var sessionID, epochID string
	if err := tx.QueryRowContext(ctx, `SELECT session_id, current_epoch_id FROM t_execution_sessions WHERE change_id = ? AND status = 'human_required'`, changeID).Scan(&sessionID, &epochID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("read execution retry session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_ticket_execution_states SET state = 'pending', authorization_id = NULL, agent_run_id = NULL, updated_at = ? WHERE graph_id IN (SELECT graph.graph_id FROM t_ticket_graphs graph WHERE graph.change_id = ?) AND state = 'human_required'`, stamp(now), changeID); err != nil {
		return fmt.Errorf("reset failed execution tickets: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_dispatch_epochs SET status = 'fenced', fenced_at = ? WHERE epoch_id = ? AND status IN ('queued', 'active', 'fenced')`, stamp(now), epochID); err != nil {
		return fmt.Errorf("fence retry epoch: %w", err)
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT sequence FROM t_execution_dispatch_epochs WHERE epoch_id = ?`, epochID).Scan(&sequence); err != nil {
		return fmt.Errorf("read retry epoch sequence: %w", err)
	}
	nextID := id.New()
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_execution_dispatch_epochs (epoch_id, session_id, sequence, status, created_at) VALUES (?, ?, ?, 'queued', ?)`, nextID, sessionID, sequence+1, stamp(now)); err != nil {
		return fmt.Errorf("create retry epoch: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET status = 'waiting', current_epoch_id = ?, updated_at = ? WHERE session_id = ?`, nextID, stamp(now), sessionID); err != nil {
		return fmt.Errorf("queue retry session: %w", err)
	}
	return nil
}

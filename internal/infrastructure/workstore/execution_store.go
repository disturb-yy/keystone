package workstore

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	executionapp "github.com/disturb-yy/keystone/internal/execution/application"
	executiondomain "github.com/disturb-yy/keystone/internal/execution/domain"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

// BeginExecution 在任何 Git 写操作前持久化唯一 provisioning intent。
func (s *Store) BeginExecution(ctx context.Context, request executionapp.ExecuteRequest) (executionapp.BeginResult, error) {
	if ctx == nil {
		return executionapp.BeginResult{}, fmt.Errorf("begin execution: %w", executiondomain.ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return executionapp.BeginResult{}, fmt.Errorf("begin execution transaction: %w", executionapp.ErrExecutionUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	change, err := readChange(ctx, tx, domain.ChangeID(request.ChangeID))
	if err != nil {
		return executionapp.BeginResult{}, err
	}
	if string(change.ProjectID) != request.ProjectID || change.RepositoryRoot != request.RepositoryRoot || change.BaseRevision != request.BaseRevision {
		return executionapp.BeginResult{}, fmt.Errorf("begin execution: %w", executiondomain.ErrConflict)
	}
	var sessionID, sessionKey, sessionDigest, sessionStatus, sessionBranch, sessionBase, sessionInput, workspaceID string
	err = tx.QueryRowContext(ctx, `SELECT session_id, request_key, request_digest, status, branch, base_revision, input_revision, workspace_id FROM t_execution_sessions WHERE change_id = ?`, request.ChangeID).Scan(&sessionID, &sessionKey, &sessionDigest, &sessionStatus, &sessionBranch, &sessionBase, &sessionInput, &workspaceID)
	if err == nil {
		if sessionKey != request.RequestKey && (sessionStatus == executiondomain.SessionStatusWaiting || sessionStatus == executiondomain.SessionStatusRunning) {
			return executionapp.BeginResult{}, executionapp.ErrExecutionInProgress
		}
		if sessionKey != request.RequestKey || sessionDigest != request.RequestDigest || sessionBranch != request.Branch || sessionBase != request.BaseRevision {
			return executionapp.BeginResult{}, executionapp.ErrIdempotencyConflict
		}
		session, readErr := readExecutionSession(ctx, tx, request.ChangeID)
		if readErr != nil {
			return executionapp.BeginResult{}, readErr
		}
		if err := tx.Commit(); err != nil {
			return executionapp.BeginResult{}, fmt.Errorf("commit execution replay: %w", executionapp.ErrExecutionUnavailable)
		}
		committed = true
		return executionapp.BeginResult{Session: session, NeedsProvision: workspaceID == ""}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return executionapp.BeginResult{}, fmt.Errorf("read execution session: %w", executionapp.ErrExecutionUnavailable)
	}
	if int64(change.Version) != request.ExpectedVersion {
		return executionapp.BeginResult{}, domain.ErrChangeVersionConflict
	}
	if change.Stage != domain.LifecycleStageExecute || change.Status != domain.ChangeStatusActive {
		return executionapp.BeginResult{}, fmt.Errorf("begin execution: %w", domain.ErrLifecycleTransitionInvalid)
	}
	if _, err := readTicketGraph(ctx, tx, change.ID); err != nil {
		return executionapp.BeginResult{}, err
	}
	var intentID, intentProject, intentChange, intentRoot, intentPath, intentRevision, intentBranch, intentKey, intentDigest, intentStatus string
	err = tx.QueryRowContext(ctx, `SELECT intent_id, project_id, change_id, repository_root, workspace_path, base_revision, branch, request_key, request_digest, status FROM t_workspace_provisioning_intents WHERE change_id = ?`, request.ChangeID).Scan(&intentID, &intentProject, &intentChange, &intentRoot, &intentPath, &intentRevision, &intentBranch, &intentKey, &intentDigest, &intentStatus)
	if err == nil {
		if intentProject != request.ProjectID || intentChange != request.ChangeID || intentRoot != request.RepositoryRoot || intentPath != request.WorkspacePath || intentRevision != request.BaseRevision || intentBranch != request.Branch || intentKey != request.RequestKey || intentDigest != request.RequestDigest {
			return executionapp.BeginResult{}, executionapp.ErrIdempotencyConflict
		}
		if intentStatus == "human_required" {
			return executionapp.BeginResult{}, fmt.Errorf("begin execution intent: %w", executiondomain.ErrFenced)
		}
		if err := tx.Commit(); err != nil {
			return executionapp.BeginResult{}, fmt.Errorf("commit execution intent replay: %w", executionapp.ErrExecutionUnavailable)
		}
		committed = true
		return executionapp.BeginResult{IntentID: intentID, NeedsProvision: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return executionapp.BeginResult{}, fmt.Errorf("read execution provisioning intent: %w", executionapp.ErrExecutionUnavailable)
	}
	intentID = id.New()
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_workspace_provisioning_intents (intent_id, project_id, change_id, repository_root, workspace_path, base_revision, branch, request_key, request_digest, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`, intentID, request.ProjectID, request.ChangeID, request.RepositoryRoot, request.WorkspacePath, request.BaseRevision, request.Branch, request.RequestKey, request.RequestDigest, stamp(now), stamp(now)); err != nil {
		return executionapp.BeginResult{}, fmt.Errorf("insert execution provisioning intent: %w", executionapp.ErrExecutionUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return executionapp.BeginResult{}, fmt.Errorf("commit execution provisioning intent: %w", executionapp.ErrExecutionUnavailable)
	}
	committed = true
	return executionapp.BeginResult{IntentID: intentID, NeedsProvision: true}, nil
}

// FinalizeExecution 在 SourceControl 成功后原子创建 Workspace、Session、Epoch 和 Ticket 状态。
func (s *Store) FinalizeExecution(ctx context.Context, request executionapp.FinalizeRequest) (executionapp.ExecutionSession, error) {
	if ctx == nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("finalize execution: %w", executiondomain.ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("begin finalize execution: %w", executionapp.ErrExecutionUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var intentKey, intentDigest, intentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT request_key, request_digest, status FROM t_workspace_provisioning_intents WHERE intent_id = ? AND change_id = ?`, request.IntentID, request.ChangeID).Scan(&intentKey, &intentDigest, &intentStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return executionapp.ExecutionSession{}, fmt.Errorf("finalize execution intent: %w", executiondomain.ErrConflict)
		}
		return executionapp.ExecutionSession{}, fmt.Errorf("read execution intent: %w", executionapp.ErrExecutionUnavailable)
	}
	if intentKey != request.RequestKey || intentDigest != request.RequestDigest || intentStatus == "human_required" {
		return executionapp.ExecutionSession{}, fmt.Errorf("finalize execution intent: %w", executiondomain.ErrConflict)
	}
	if request.WorkspacePath == "" || filepath.Clean(request.WorkspacePath) != request.WorkspacePath || request.PhysicalPath == "" || request.HeadRevision != request.BaseRevision {
		return executionapp.ExecutionSession{}, fmt.Errorf("finalize execution workspace: %w", executiondomain.ErrConflict)
	}
	change, err := readChange(ctx, tx, domain.ChangeID(request.ChangeID))
	if err != nil {
		return executionapp.ExecutionSession{}, err
	}
	if change.Status != domain.ChangeStatusActive || change.Stage != domain.LifecycleStageExecute || change.Version <= 0 {
		return executionapp.ExecutionSession{}, fmt.Errorf("finalize execution change: %w", domain.ErrLifecycleTransitionInvalid)
	}
	if existing, existingErr := readExecutionSession(ctx, tx, request.ChangeID); existingErr == nil {
		if err := tx.Commit(); err != nil {
			return executionapp.ExecutionSession{}, fmt.Errorf("commit execution replay: %w", executionapp.ErrExecutionUnavailable)
		}
		committed = true
		return existing, nil
	} else if !errors.Is(existingErr, domain.ErrChangeNotFound) && !errors.Is(existingErr, domain.ErrTicketGraphNotFound) && !errors.Is(existingErr, sql.ErrNoRows) {
		// readExecutionSession uses ErrChangeNotFound only for a missing session;
		// preserve real storage errors instead of trying a second write.
		if !strings.Contains(existingErr.Error(), "execution session not found") {
			return executionapp.ExecutionSession{}, existingErr
		}
	}
	graph, err := readTicketGraph(ctx, tx, change.ID)
	if err != nil {
		return executionapp.ExecutionSession{}, err
	}
	var maxSequence sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(dispatch_sequence) FROM t_execution_sessions WHERE project_id = ?`, request.ProjectID).Scan(&maxSequence); err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("read execution queue sequence: %w", executionapp.ErrExecutionUnavailable)
	}
	sequence := int64(1)
	if maxSequence.Valid {
		sequence = maxSequence.Int64 + 1
	}
	sessionID := id.New()
	epochID := id.New()
	workspaceID := "workspace-" + shortDigest(request.ChangeID)
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_workspaces (workspace_id, project_id, change_id, repository_root, workspace_path, physical_path, branch, base_revision, input_revision, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, workspaceID, request.ProjectID, request.ChangeID, request.RepositoryRoot, request.WorkspacePath, request.PhysicalPath, request.Branch, request.BaseRevision, request.BaseRevision, stamp(now), stamp(now)); err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("insert execution workspace: %w", executionapp.ErrExecutionUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_execution_sessions (session_id, project_id, change_id, request_key, request_digest, status, branch, base_revision, input_revision, workspace_id, workspace_path, instruction, dispatch_sequence, current_epoch_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'waiting', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, sessionID, request.ProjectID, request.ChangeID, request.RequestKey, request.RequestDigest, request.Branch, request.BaseRevision, request.BaseRevision, workspaceID, request.WorkspacePath, request.Instruction, sequence, epochID, stamp(now), stamp(now)); err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("insert execution session: %w", executionapp.ErrExecutionUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_execution_dispatch_epochs (epoch_id, session_id, sequence, status, created_at) VALUES (?, ?, 1, 'queued', ?)`, epochID, sessionID, stamp(now)); err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("insert execution dispatch epoch: %w", executionapp.ErrExecutionUnavailable)
	}
	for _, ticket := range graph.Tickets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_ticket_execution_states (graph_id, ticket_id, ordinal, state, updated_at) VALUES (?, ?, ?, 'pending', ?)`, graph.ID, ticket.ID, ticket.Ordinal, stamp(now)); err != nil {
			return executionapp.ExecutionSession{}, fmt.Errorf("insert execution ticket state: %w", executionapp.ErrExecutionUnavailable)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_workspace_provisioning_intents SET status = 'provisioned', updated_at = ? WHERE intent_id = ? AND status IN ('pending', 'provisioned')`, stamp(now), request.IntentID); err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("finalize execution intent: %w", executionapp.ErrExecutionUnavailable)
	}
	session, err := readExecutionSession(ctx, tx, request.ChangeID)
	if err != nil {
		return executionapp.ExecutionSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("commit execution session: %w", executionapp.ErrExecutionUnavailable)
	}
	committed = true
	return session, nil
}

// FindExecution 返回不含内部路径、Prompt 和 Lease 的执行读取模型。
func (s *Store) FindExecution(ctx context.Context, changeID string) (executionapp.ExecutionSession, error) {
	if ctx == nil || strings.TrimSpace(changeID) == "" {
		return executionapp.ExecutionSession{}, fmt.Errorf("find execution: %w", executiondomain.ErrInvalid)
	}
	return readExecutionSession(ctx, s.db, changeID)
}

// ExecutionAssignmentInfo 是 Daemon 采集 Snapshot 时使用的内部目标，不属于 Worker Protocol。
type ExecutionAssignmentInfo struct {
	LeaseID       string
	WorkspacePath string
	WorkspaceID   string
	SessionID     string
	EpochID       string
	TicketID      string
	InputRevision string
}

// WorkspaceSnapshotInput 是 SourceControl 观察值到 authority 的窄映射。
type WorkspaceSnapshotInput struct {
	Phase        string
	HeadRevision string
	Branch       string
	ChangedFiles []string
	DiffSHA256   string
	DiffBytes    int64
	HasUntracked bool
}

// ExecutionAssignmentInfo 返回当前 Claim 的内部 Workspace 目标。
func (s *Store) ExecutionAssignmentInfo(ctx context.Context, agentRunID string) (ExecutionAssignmentInfo, error) {
	var info ExecutionAssignmentInfo
	err := s.db.QueryRowContext(ctx, `
SELECT lease.lease_id, session.workspace_path, session.workspace_id, session.session_id,
       authorization.epoch_id, authorization.ticket_id, session.input_revision
FROM t_worker_leases lease
JOIN t_agent_runs run ON run.agent_run_id = lease.agent_run_id
JOIN t_ticket_execution_states state ON state.agent_run_id = run.agent_run_id
JOIN t_execution_authorizations authorization ON authorization.authorization_id = state.authorization_id
JOIN t_execution_sessions session ON session.session_id = authorization.session_id
WHERE lease.agent_run_id = ? AND run.status = 'running'`, agentRunID).Scan(&info.LeaseID, &info.WorkspacePath, &info.WorkspaceID, &info.SessionID, &info.EpochID, &info.TicketID, &info.InputRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	if err != nil {
		return ExecutionAssignmentInfo{}, fmt.Errorf("read execution assignment info: %w", ErrWorkerUnavailable)
	}
	return info, nil
}

// ExecutionAssignmentInfoFor 只向匹配 Worker 和 Lease token 的调用方返回 Snapshot 目标。
func (s *Store) ExecutionAssignmentInfoFor(ctx context.Context, workerID, agentRunID, leaseToken string) (ExecutionAssignmentInfo, error) {
	if ctx == nil || strings.TrimSpace(workerID) == "" || strings.TrimSpace(agentRunID) == "" || strings.TrimSpace(leaseToken) == "" {
		return ExecutionAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	var info ExecutionAssignmentInfo
	var leaseWorker, tokenDigest, expiresAt string
	err := s.db.QueryRowContext(ctx, `
SELECT lease.lease_id, lease.worker_id, lease.token_sha256,
       session.workspace_path, session.workspace_id, session.session_id,
       authorization.epoch_id, authorization.ticket_id, session.input_revision,
       lease.expires_at
FROM t_worker_leases lease
JOIN t_agent_runs run ON run.agent_run_id = lease.agent_run_id
JOIN t_ticket_execution_states state ON state.agent_run_id = run.agent_run_id
JOIN t_execution_authorizations authorization ON authorization.authorization_id = state.authorization_id
JOIN t_execution_sessions session ON session.session_id = authorization.session_id
WHERE lease.agent_run_id = ? AND run.status = 'running' AND lease.state = 'active'`, agentRunID).Scan(
		&info.LeaseID, &leaseWorker, &tokenDigest, &info.WorkspacePath, &info.WorkspaceID,
		&info.SessionID, &info.EpochID, &info.TicketID, &info.InputRevision, &expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	if err != nil {
		return ExecutionAssignmentInfo{}, fmt.Errorf("read execution assignment identity: %w", ErrWorkerUnavailable)
	}
	if leaseWorker != workerID || subtle.ConstantTimeCompare([]byte(tokenDigest), []byte(hashSecret(leaseToken))) != 1 {
		return ExecutionAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	deadline, deadlineErr := parseStamp(expiresAt)
	if deadlineErr != nil || !s.now().UTC().Before(deadline) {
		return ExecutionAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	return info, nil
}

// RecordExecutionSnapshot 在 Claim 围栏内追加一份不可变 Snapshot。
func (s *Store) RecordExecutionSnapshot(ctx context.Context, agentRunID string, input WorkspaceSnapshotInput) error {
	if input.Phase != "pre" && input.Phase != "post" || input.HeadRevision == "" || input.Branch == "" || input.DiffSHA256 == "" || input.DiffBytes < 0 {
		return fmt.Errorf("record execution snapshot: %w", domain.ErrInvalidRequest)
	}
	info, err := s.ExecutionAssignmentInfo(ctx, agentRunID)
	if err != nil {
		return err
	}
	var state, claimState string
	var expires string
	if err := s.db.QueryRowContext(ctx, `SELECT state, claim_state, expires_at FROM t_worker_leases WHERE lease_id = ?`, info.LeaseID).Scan(&state, &claimState, &expires); err != nil {
		return fmt.Errorf("read execution snapshot lease: %w", ErrWorkerUnavailable)
	}
	deadline, deadlineErr := parseStamp(expires)
	if state != "active" || claimState != "claimed" || deadlineErr != nil || !s.now().UTC().Before(deadline) {
		return ErrWorkerLeaseInvalid
	}
	if input.HeadRevision != info.InputRevision {
		return fmt.Errorf("record execution snapshot: %w", executiondomain.ErrConflict)
	}
	files, err := json.Marshal(executiondomain.NormalizeFiles(input.ChangedFiles))
	if err != nil {
		return fmt.Errorf("encode execution snapshot files: %w", ErrWorkerUnavailable)
	}
	snapshotID := id.New()
	_, err = s.db.ExecContext(ctx, `INSERT INTO t_workspace_snapshots (snapshot_id, workspace_id, session_id, epoch_id, ticket_id, phase, input_revision, head_revision, branch, changed_files_json, diff_sha256, diff_bytes, has_untracked, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshotID, info.WorkspaceID, info.SessionID, info.EpochID, info.TicketID, input.Phase, info.InputRevision, input.HeadRevision, input.Branch, string(files), input.DiffSHA256, input.DiffBytes, boolInt(input.HasUntracked), stamp(s.now().UTC()))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil
		}
		return fmt.Errorf("persist execution snapshot: %w", ErrWorkerUnavailable)
	}
	return nil
}

// IssueNextExecutionAssignment 按 Project FIFO 和 Canonical Ticket 顺序发放一张 edit Assignment。
// Worker Pull 只调用该方法取得已持久化的 Lease，不会因此启动 Runtime。
func (s *Store) IssueNextExecutionAssignment(ctx context.Context, workerID string) (*workercontract.Assignment, error) {
	if ctx == nil || strings.TrimSpace(workerID) == "" {
		return nil, ErrWorkerNotRegistered
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin execution dispatch: %w", ErrWorkerUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := reconcileExpiredExecutionLeases(ctx, tx, s.now().UTC()); err != nil {
		return nil, err
	}
	var workerStatus, capabilitiesJSON string
	var idleAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, capabilities_json, last_heartbeat_at FROM t_worker_instances WHERE worker_id = ?`, workerID).Scan(&workerStatus, &capabilitiesJSON, &idleAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkerNotRegistered
		}
		return nil, fmt.Errorf("read execution worker: %w", ErrWorkerUnavailable)
	}
	if workerStatus != "registered" || !idleAt.Valid {
		return nil, nil
	}
	var capabilities []string
	if err := json.Unmarshal([]byte(capabilitiesJSON), &capabilities); err != nil {
		return nil, fmt.Errorf("decode execution worker capabilities: %w", ErrWorkerUnavailable)
	}
	if !containsWorkerCapability(capabilities, "runtime:"+executiondomain.DefaultRuntime) {
		return nil, nil
	}
	var sessionID, projectID, changeID, graphID, ticketID, title, scope, workspaceID, workspacePath, baseRevision, instruction string
	var ordinal int
	const candidateQuery = `
SELECT session.session_id, session.project_id, session.change_id, graph.graph_id,
       ticket.ticket_id, ticket.ordinal, ticket.title, ticket.scope,
       session.workspace_id, session.workspace_path, session.base_revision, session.instruction
FROM t_execution_sessions session
JOIN t_changes change ON change.change_id = session.change_id AND change.project_id = session.project_id
JOIN t_ticket_graphs graph ON graph.change_id = session.change_id AND graph.project_id = session.project_id
JOIN t_ticket_execution_states state ON state.graph_id = graph.graph_id
JOIN t_tickets ticket ON ticket.graph_id = graph.graph_id AND ticket.ticket_id = state.ticket_id
WHERE session.status = 'waiting'
  AND change.stage = 'Execute'
  AND change.status = 'active'
  AND state.state = 'pending'
  AND NOT EXISTS (
      SELECT 1 FROM t_execution_sessions earlier
      WHERE earlier.project_id = session.project_id
        AND earlier.dispatch_sequence < session.dispatch_sequence
        AND earlier.status IN ('waiting', 'running')
  )
  AND NOT EXISTS (
      SELECT 1 FROM t_ticket_dependencies dependency
      WHERE dependency.graph_id = graph.graph_id
        AND dependency.dependent_ticket_id = ticket.ticket_id
        AND NOT EXISTS (
            SELECT 1 FROM t_ticket_execution_states blocker
            WHERE blocker.graph_id = graph.graph_id
              AND blocker.ticket_id = dependency.blocker_ticket_id
              AND blocker.state = 'succeeded'
        )
  )
  AND NOT EXISTS (
      SELECT 1
      FROM t_execution_authorizations authorization
      JOIN t_worker_leases lease ON lease.agent_run_id = (
          SELECT execution_state.agent_run_id
          FROM t_ticket_execution_states execution_state
          WHERE execution_state.authorization_id = authorization.authorization_id
      )
      WHERE authorization.session_id = session.session_id
        AND lease.state = 'active'
  )
ORDER BY session.dispatch_sequence, ticket.ordinal, session.session_id
LIMIT 1`
	err = tx.QueryRowContext(ctx, candidateQuery).Scan(&sessionID, &projectID, &changeID, &graphID, &ticketID, &ordinal, &title, &scope, &workspaceID, &workspacePath, &baseRevision, &instruction)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit empty execution dispatch: %w", ErrWorkerUnavailable)
		}
		committed = true
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select execution frontier: %w", ErrWorkerUnavailable)
	}
	var activeProjectLeases int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_execution_authorizations authorization JOIN t_ticket_execution_states state ON state.authorization_id = authorization.authorization_id JOIN t_worker_leases lease ON lease.agent_run_id = state.agent_run_id WHERE authorization.session_id IN (SELECT session_id FROM t_execution_sessions WHERE project_id = ?) AND lease.state = 'active'`, projectID).Scan(&activeProjectLeases); err != nil {
		return nil, fmt.Errorf("inspect project execution slot: %w", ErrWorkerUnavailable)
	}
	if activeProjectLeases != 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit occupied execution slot: %w", ErrWorkerUnavailable)
		}
		committed = true
		return nil, nil
	}
	var maxAttempt sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM t_agent_runs WHERE change_id = ? AND stage = 'Execute'`, changeID).Scan(&maxAttempt); err != nil {
		return nil, fmt.Errorf("read execution attempt: %w", ErrWorkerUnavailable)
	}
	attempt := 1
	if maxAttempt.Valid {
		attempt = int(maxAttempt.Int64) + 1
	}
	runID := id.New()
	authorizationID := id.New()
	leaseID := id.New()
	epochID := ""
	if err := tx.QueryRowContext(ctx, `SELECT current_epoch_id FROM t_execution_sessions WHERE session_id = ?`, sessionID).Scan(&epochID); err != nil || epochID == "" {
		return nil, fmt.Errorf("read execution epoch: %w", ErrWorkerUnavailable)
	}
	created := s.now().UTC()
	fullInstruction := strings.TrimSpace(instruction)
	if fullInstruction != "" {
		fullInstruction += "\n\n"
	}
	fullInstruction += "Ticket: " + title + "\nScope:\n" + scope
	if len([]byte(fullInstruction)) > executiondomain.MaxInstructionBytes {
		fullInstruction = truncateUTF8(fullInstruction, executiondomain.MaxInstructionBytes)
	}
	inputDigest := digestString(fullInstruction)
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_agent_runs (agent_run_id, project_id, change_id, stage, attempt, status, outcome, started_at, run_kind, source_revision, ticket_id, authorization_id) VALUES (?, ?, ?, 'Execute', ?, 'running', '', ?, '', '', ?, ?)`, runID, projectID, changeID, attempt, stamp(created), ticketID, authorizationID); err != nil {
		return nil, fmt.Errorf("insert execution agent run: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_execution_authorizations (authorization_id, session_id, epoch_id, graph_id, ticket_id, policy_version, execution_mode, runtime, instruction, input_digest, timeout_seconds, workspace_id, branch, input_revision, decision_digest, status, created_at, updated_at) SELECT ?, ?, ?, ?, ?, ?, 'edit', 'codex', ?, ?, 1800, session.workspace_id, session.branch, session.input_revision, session.request_digest, 'assigned', ?, ? FROM t_execution_sessions session WHERE session.session_id = ?`, authorizationID, sessionID, epochID, graphID, ticketID, executiondomain.ExecutionPolicyVersion, fullInstruction, inputDigest, stamp(created), stamp(created), sessionID); err != nil {
		return nil, fmt.Errorf("insert execution authorization: %w", ErrWorkerUnavailable)
	}
	token, err := NewWorkerSecret()
	if err != nil {
		return nil, err
	}
	expires := created.Add(workerLeaseTTL)
	inputJSON := []byte("[]")
	envelopeDigest := digestString(fmt.Sprintf("%s\x00%s\x001800", authorizationID, inputDigest))
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_worker_leases (lease_id, agent_run_id, worker_id, attempt, token_sha256, state, expires_at, workspace_id, workspace_path, runtime, instruction, before_revision, input_artifacts_json, created_at, result_mode, runtime_claim_id, claim_state, execution_mode, timeout_seconds, envelope_digest) VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, 'codex', ?, ?, ?, ?, '', '', '', 'edit', 1800, ?)`, leaseID, runID, workerID, attempt, hashSecret(token), stamp(expires), workspaceID, workspacePath, fullInstruction, baseRevision, string(inputJSON), stamp(created), envelopeDigest); err != nil {
		return nil, fmt.Errorf("insert execution worker lease: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_ticket_execution_states SET state = 'assigned', authorization_id = ?, agent_run_id = ?, updated_at = ? WHERE graph_id = ? AND ticket_id = ? AND state = 'pending'`, authorizationID, runID, stamp(created), graphID, ticketID); err != nil {
		return nil, fmt.Errorf("assign execution ticket: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET status = 'running', updated_at = ? WHERE session_id = ? AND status = 'waiting'`, stamp(created), sessionID); err != nil {
		return nil, fmt.Errorf("start execution session: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_execution_dispatch_epochs SET status = 'active' WHERE epoch_id = ? AND status = 'queued'`, epochID); err != nil {
		return nil, fmt.Errorf("activate execution epoch: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_instances SET last_heartbeat_at = NULL WHERE worker_id = ? AND status = 'registered'`, workerID); err != nil {
		return nil, fmt.Errorf("occupy execution worker: %w", ErrWorkerUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit execution assignment: %w", ErrWorkerUnavailable)
	}
	committed = true
	s.leaseMu.Lock()
	s.leaseTokens[leaseID] = token
	s.leaseMu.Unlock()
	return &workercontract.Assignment{AgentRunID: runID, LeaseToken: token, WorkspaceID: workspaceID, WorkspacePath: workspacePath, Runtime: executiondomain.DefaultRuntime, ExecutionMode: executiondomain.DefaultExecutionMode, TimeoutSeconds: int(executiondomain.DefaultTimeout / time.Second), LeaseExpiresAt: stamp(expires), Instruction: fullInstruction, BeforeRevision: baseRevision, Attempt: attempt}, nil
}

func readExecutionSession(ctx context.Context, queryer sqlQueryContext, changeID string) (executionapp.ExecutionSession, error) {
	var session executionapp.ExecutionSession
	var status, branch, baseRevision, inputRevision, workspaceID string
	var sessionID, requestKey, requestDigest string
	var sequence int64
	err := queryer.QueryRowContext(ctx, `SELECT session_id, request_key, request_digest, status, branch, base_revision, input_revision, workspace_id, dispatch_sequence FROM t_execution_sessions WHERE change_id = ?`, changeID).Scan(&sessionID, &requestKey, &requestDigest, &status, &branch, &baseRevision, &inputRevision, &workspaceID, &sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return executionapp.ExecutionSession{}, fmt.Errorf("execution session not found: %w", domain.ErrChangeNotFound)
	}
	if err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("read execution session: %w", executionapp.ErrExecutionUnavailable)
	}
	rows, err := queryer.QueryContext(ctx, `SELECT state.ordinal, state.ticket_id, ticket.title, state.state FROM t_ticket_execution_states state JOIN t_tickets ticket ON ticket.ticket_id = state.ticket_id JOIN t_execution_sessions session ON session.change_id = ? JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id AND graph.change_id = session.change_id WHERE session.change_id = ? ORDER BY state.ordinal`, changeID, changeID)
	if err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("read execution ticket states: %w", executionapp.ErrExecutionUnavailable)
	}
	defer rows.Close()
	tickets := make([]executionapp.TicketState, 0)
	for rows.Next() {
		var ticket executionapp.TicketState
		if err := rows.Scan(&ticket.Ordinal, &ticket.TicketID, &ticket.Title, &ticket.State); err != nil {
			return executionapp.ExecutionSession{}, fmt.Errorf("scan execution ticket state: %w", executionapp.ErrExecutionUnavailable)
		}
		tickets = append(tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		return executionapp.ExecutionSession{}, fmt.Errorf("read execution ticket states: %w", executionapp.ErrExecutionUnavailable)
	}
	if len(tickets) == 0 {
		return executionapp.ExecutionSession{}, fmt.Errorf("execution ticket states are missing: %w", domain.ErrTicketGraphNotFound)
	}
	session = executionapp.ExecutionSession{ID: sessionID, ChangeID: changeID, Status: status, Branch: branch, BaseRevision: baseRevision, InputRevision: inputRevision, WorkspaceID: workspaceID, Tickets: tickets}
	return session, nil
}

func digestString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func shortDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || len([]byte(value)) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

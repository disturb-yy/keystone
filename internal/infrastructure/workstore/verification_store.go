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
	"strings"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	governancedomain "github.com/disturb-yy/keystone/internal/governance/domain"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/infrastructure/manifest"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

var (
	// ErrVerificationUnavailable 表示验证状态暂时不可读取或持久化。
	ErrVerificationUnavailable = errors.New("verification persistence is unavailable")
	// ErrVerificationConflict 表示当前 Gate、revision 或幂等身份不匹配。
	ErrVerificationConflict = errors.New("verification conflicts with current authority")
	// ErrCommitUnavailable 表示 Git 与 Commit authority 尚未形成可证明结果。
	ErrCommitUnavailable = errors.New("commit is not safely recoverable")
)

// VerificationPolicySnapshotInput 是 Execute 后从 BaseRevision 读取的 V2 配置。
type VerificationPolicySnapshotInput struct {
	SessionID    string
	ProjectID    string
	ChangeID     string
	BaseRevision string
	Manifest     manifest.ProjectManifestV2
}

// VerificationIntentRequest 是 Verify/FinalVerify 的已规范化输入。
type VerificationIntentRequest struct {
	ProjectID             string
	ChangeID              string
	TicketID              string
	ExpectedVersion       int
	RequestKey            string
	RequestDigest         string
	InputRevision         string
	CandidateTreeIdentity string
	Final                 bool
	SnapshotBranch        string
	SnapshotChangedFiles  []string
	SnapshotDiffSHA256    string
	SnapshotDiffBytes     int64
	SnapshotHasUntracked  bool
}

// VerificationIntentResult 是 202 回执需要的稳定 Intent 摘要。
type VerificationIntentResult struct {
	IntentID   string
	ChangeID   string
	TicketID   string
	Status     string
	Final      bool
	Replayed   bool
	AgentRunID string
}

// SaveVerificationPolicySnapshot 在 Execute 接受后固定 BaseRevision 策略；重复调用只返回原快照。
func (s *Store) SaveVerificationPolicySnapshot(ctx context.Context, input VerificationPolicySnapshotInput) error {
	if ctx == nil || input.SessionID == "" || input.ProjectID == "" || input.ChangeID == "" || input.BaseRevision == "" {
		return fmt.Errorf("save verification policy: %w", ErrVerificationUnavailable)
	}
	if err := input.Manifest.Validate(); err != nil {
		return fmt.Errorf("save verification policy: %w", err)
	}
	commands, err := json.Marshal(input.Manifest.VerifyCommands)
	if err != nil {
		return fmt.Errorf("encode verification policy: %w", ErrVerificationUnavailable)
	}
	verificationDigest, templateDigest, err := input.Manifest.Digests()
	if err != nil {
		return fmt.Errorf("digest verification policy: %w", ErrVerificationUnavailable)
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO t_verification_policy_snapshots (snapshot_id, project_id, change_id, execution_session_id, base_revision, verification_digest, commit_template_digest, commands_json, commit_template, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id.New(), input.ProjectID, input.ChangeID, input.SessionID, input.BaseRevision, verificationDigest, templateDigest, string(commands), input.Manifest.CommitTemplate, stamp(s.now().UTC()))
	if err != nil {
		return fmt.Errorf("save verification policy: %w", ErrVerificationUnavailable)
	}
	return nil
}

// BeginVerification 持久化 Intent；没有可用策略时仍记录 human_required 事实但不发放 Assignment。
func (s *Store) BeginVerification(ctx context.Context, request VerificationIntentRequest) (VerificationIntentResult, error) {
	if ctx == nil || strings.TrimSpace(request.ProjectID) == "" || strings.TrimSpace(request.ChangeID) == "" || strings.TrimSpace(request.RequestKey) == "" || strings.TrimSpace(request.RequestDigest) == "" || request.ExpectedVersion < 1 || request.InputRevision == "" || request.CandidateTreeIdentity == "" {
		return VerificationIntentResult{}, fmt.Errorf("begin verification: %w", domain.ErrInvalidRequest)
	}
	if request.Final && request.TicketID != "" || !request.Final && request.TicketID == "" {
		return VerificationIntentResult{}, fmt.Errorf("begin verification scope: %w", domain.ErrInvalidRequest)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VerificationIntentResult{}, fmt.Errorf("begin verification transaction: %w", ErrVerificationUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var existing VerificationIntentResult
	var existingFinal int
	var existingDigest string
	var existingProjectID string
	err = tx.QueryRowContext(ctx, `SELECT intent_id, project_id, change_id, ticket_id, request_digest, status, final, COALESCE(agent_run_id, '') FROM t_verification_intents WHERE request_key = ?`, request.RequestKey).Scan(&existing.IntentID, &existingProjectID, &existing.ChangeID, &existing.TicketID, &existingDigest, &existing.Status, &existingFinal, &existing.AgentRunID)
	if err == nil {
		existing.Final = existingFinal != 0
		if existingProjectID != request.ProjectID || existing.ChangeID != request.ChangeID || existing.TicketID != request.TicketID || existing.Final != request.Final || existingDigest != request.RequestDigest {
			return VerificationIntentResult{}, domain.ErrIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return VerificationIntentResult{}, fmt.Errorf("replay verification intent: %w", ErrVerificationUnavailable)
		}
		committed = true
		existing.Replayed = true
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return VerificationIntentResult{}, fmt.Errorf("read verification intent: %w", ErrVerificationUnavailable)
	}
	change, err := readChange(ctx, tx, domain.ChangeID(request.ChangeID))
	if err != nil {
		return VerificationIntentResult{}, err
	}
	if string(change.ProjectID) != request.ProjectID || int(change.Version) != request.ExpectedVersion || change.Status != domain.ChangeStatusActive {
		return VerificationIntentResult{}, fmt.Errorf("begin verification precondition: %w", ErrVerificationConflict)
	}
	if err := validateVerificationScope(ctx, tx, change, request); err != nil {
		return VerificationIntentResult{}, err
	}
	var activeIntentID string
	if err := tx.QueryRowContext(ctx, `SELECT intent_id FROM t_verification_intents WHERE change_id = ? AND ticket_id = ? AND final = ? AND status IN ('pending', 'running') LIMIT 1`, request.ChangeID, request.TicketID, boolInt(request.Final)).Scan(&activeIntentID); err == nil {
		return VerificationIntentResult{}, fmt.Errorf("verification scope already active: %w", ErrVerificationConflict)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return VerificationIntentResult{}, fmt.Errorf("read active verification scope: %w", ErrVerificationUnavailable)
	}
	snapshotID, policyDigest := "", ""
	err = tx.QueryRowContext(ctx, `SELECT snapshot_id, verification_digest FROM t_verification_policy_snapshots WHERE execution_session_id = (SELECT session_id FROM t_execution_sessions WHERE change_id = ?)`, request.ChangeID).Scan(&snapshotID, &policyDigest)
	if errors.Is(err, sql.ErrNoRows) {
		// 不合法或缺失的 BaseRevision policy 不伪造 snapshot，Intent 仍留下人审事实。
		snapshotID, policyDigest = "", ""
	} else if err != nil {
		return VerificationIntentResult{}, fmt.Errorf("read verification policy: %w", ErrVerificationUnavailable)
	}
	status := governancedomain.VerificationPending
	if snapshotID == "" {
		status = governancedomain.VerificationHuman
	}
	var maxAttempt sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM t_verification_intents WHERE change_id = ? AND ticket_id = ? AND final = ?`, request.ChangeID, request.TicketID, boolInt(request.Final)).Scan(&maxAttempt); err != nil {
		return VerificationIntentResult{}, fmt.Errorf("read verification attempt: %w", ErrVerificationUnavailable)
	}
	attempt := 1
	if maxAttempt.Valid {
		attempt = int(maxAttempt.Int64) + 1
	}
	now := s.now().UTC()
	intentID := id.New()
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_verification_intents (intent_id, project_id, change_id, ticket_id, final, policy_snapshot_id, policy_digest, input_revision, candidate_tree_identity, request_key, request_digest, status, attempt, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, intentID, request.ProjectID, request.ChangeID, request.TicketID, boolInt(request.Final), nullableString(snapshotID), policyDigest, request.InputRevision, request.CandidateTreeIdentity, request.RequestKey, request.RequestDigest, status, attempt, stamp(now), stamp(now)); err != nil {
		return VerificationIntentResult{}, fmt.Errorf("insert verification intent: %w", ErrVerificationUnavailable)
	}
	if err := insertVerificationSnapshotTx(ctx, tx, intentID, VerificationSnapshotInput{Phase: "before", InputRevision: request.InputRevision, HeadRevision: request.InputRevision, Branch: request.SnapshotBranch, ChangedFiles: request.SnapshotChangedFiles, DiffSHA256: request.SnapshotDiffSHA256, DiffBytes: request.SnapshotDiffBytes, HasUntracked: request.SnapshotHasUntracked, TreeIdentity: request.CandidateTreeIdentity}, stamp(now)); err != nil {
		return VerificationIntentResult{}, err
	}
	if err := insertGovernanceEventTx(ctx, tx, request.ProjectID, request.ChangeID, verificationOperation(request.Final)+"_intent_created", intentID, map[string]any{"final": request.Final, "status": status}, stamp(now)); err != nil {
		return VerificationIntentResult{}, err
	}
	if status == governancedomain.VerificationHuman {
		next, transitionErr := change.EnterHumanRequired()
		if transitionErr != nil {
			return VerificationIntentResult{}, fmt.Errorf("fence invalid verification policy: %w", ErrVerificationConflict)
		}
		next.UpdatedAt = now
		if err := updateChangeStatus(ctx, tx, change, next, now); err != nil {
			return VerificationIntentResult{}, fmt.Errorf("update verification recovery boundary: %w", ErrVerificationUnavailable)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_verification_receipts (idempotency_key, operation, project_id, change_id, ticket_id, request_fingerprint, response_body, status_code, created_at) VALUES (?, ?, ?, ?, ?, ?, '{}', 202, ?)`, request.RequestKey, verificationOperation(request.Final), request.ProjectID, request.ChangeID, request.TicketID, request.RequestDigest, stamp(now)); err != nil {
		return VerificationIntentResult{}, fmt.Errorf("record verification receipt: %w", ErrVerificationUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return VerificationIntentResult{}, fmt.Errorf("commit verification intent: %w", ErrVerificationUnavailable)
	}
	committed = true
	return VerificationIntentResult{IntentID: intentID, ChangeID: request.ChangeID, TicketID: request.TicketID, Status: status, Final: request.Final}, nil
}

func validateVerificationScope(ctx context.Context, tx *sql.Tx, change domain.Change, request VerificationIntentRequest) error {
	if request.Final {
		if change.Stage != domain.LifecycleStageVerify {
			return fmt.Errorf("final verification stage: %w", ErrVerificationConflict)
		}
		var total, committed int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_tickets ticket JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id WHERE graph.change_id = ?`, request.ChangeID).Scan(&total); err != nil {
			return fmt.Errorf("count final tickets: %w", ErrVerificationUnavailable)
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_keystone_commits WHERE change_id = ?`, request.ChangeID).Scan(&committed); err != nil {
			return fmt.Errorf("count final commits: %w", ErrVerificationUnavailable)
		}
		if total == 0 || total != committed {
			return fmt.Errorf("final verification tickets are incomplete: %w", ErrVerificationConflict)
		}
		return nil
	}
	if change.Stage != domain.LifecycleStageExecute {
		return fmt.Errorf("ticket verification stage: %w", ErrVerificationConflict)
	}
	var state, inputRevision string
	err := tx.QueryRowContext(ctx, `SELECT state, workspace.input_revision FROM t_ticket_execution_states state JOIN t_tickets ticket ON ticket.graph_id = state.graph_id AND ticket.ticket_id = state.ticket_id JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id JOIN t_workspaces workspace ON workspace.change_id = graph.change_id WHERE graph.change_id = ? AND ticket.ticket_id = ?`, request.ChangeID, request.TicketID).Scan(&state, &inputRevision)
	if errors.Is(err, sql.ErrNoRows) || state != "succeeded" || inputRevision != request.InputRevision {
		return fmt.Errorf("ticket verification gate: %w", ErrVerificationConflict)
	}
	var commits int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_keystone_commits WHERE change_id = ? AND ticket_id = ?`, request.ChangeID, request.TicketID).Scan(&commits); err != nil {
		return fmt.Errorf("read ticket commit gate: %w", ErrVerificationUnavailable)
	}
	if commits != 0 {
		return fmt.Errorf("ticket already committed: %w", ErrVerificationConflict)
	}
	var passed int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_verification_evidence evidence JOIN t_verification_intents intent ON intent.intent_id = evidence.intent_id WHERE intent.change_id = ? AND intent.ticket_id = ? AND intent.final = 0 AND evidence.outcome = 'pass'`, request.ChangeID, request.TicketID).Scan(&passed); err != nil {
		return fmt.Errorf("read ticket verification gate: %w", ErrVerificationUnavailable)
	}
	if passed != 0 {
		return fmt.Errorf("ticket already has pass evidence: %w", ErrVerificationConflict)
	}
	return nil
}

func verificationOperation(final bool) string {
	if final {
		return "final_verify"
	}
	return "verify"
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// IssueNextVerificationAssignment 只为声明 verification-v1 的 Worker 发放验证任务。
func (s *Store) IssueNextVerificationAssignment(ctx context.Context, workerID string) (*workercontract.Assignment, error) {
	if ctx == nil || strings.TrimSpace(workerID) == "" {
		return nil, ErrWorkerNotRegistered
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin verification dispatch: %w", ErrVerificationUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var status, capabilitiesJSON string
	var idleAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, capabilities_json, last_heartbeat_at FROM t_worker_instances WHERE worker_id = ?`, workerID).Scan(&status, &capabilitiesJSON, &idleAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkerNotRegistered
		}
		return nil, fmt.Errorf("read verification worker: %w", ErrVerificationUnavailable)
	}
	if status != "registered" || !idleAt.Valid {
		return nil, nil
	}
	var activeLeaseCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_worker_leases WHERE worker_id = ? AND state = 'active'`, workerID).Scan(&activeLeaseCount); err != nil {
		return nil, fmt.Errorf("inspect verification worker leases: %w", ErrVerificationUnavailable)
	}
	if activeLeaseCount != 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit occupied verification worker: %w", ErrVerificationUnavailable)
		}
		committed = true
		return nil, nil
	}
	var capabilities []string
	if err := json.Unmarshal([]byte(capabilitiesJSON), &capabilities); err != nil || !containsWorkerCapability(capabilities, workercontract.VerificationCapability) {
		return nil, nil
	}
	var intentID, projectID, changeID, ticketID, inputRevision, treeIdentity, snapshotID, commandsJSON, policyDigest, workspaceID, workspacePath string
	var final int
	err = tx.QueryRowContext(ctx, `SELECT intent.intent_id, intent.project_id, intent.change_id, intent.ticket_id, intent.input_revision, intent.candidate_tree_identity, intent.policy_snapshot_id, snapshot.commands_json, snapshot.verification_digest, workspace.workspace_id, workspace.workspace_path, intent.final FROM t_verification_intents intent JOIN t_verification_policy_snapshots snapshot ON snapshot.snapshot_id = intent.policy_snapshot_id JOIN t_workspaces workspace ON workspace.change_id = intent.change_id JOIN t_changes change ON change.change_id = intent.change_id WHERE intent.status = 'pending' AND change.status = 'active' AND ((intent.final = 0 AND change.stage = 'Execute') OR (intent.final = 1 AND change.stage = 'Verify')) ORDER BY intent.created_at, intent.intent_id LIMIT 1`).Scan(&intentID, &projectID, &changeID, &ticketID, &inputRevision, &treeIdentity, &snapshotID, &commandsJSON, &policyDigest, &workspaceID, &workspacePath, &final)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit empty verification dispatch: %w", ErrVerificationUnavailable)
		}
		committed = true
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select verification intent: %w", ErrVerificationUnavailable)
	}
	var commands []manifest.V2Command
	if err := json.Unmarshal([]byte(commandsJSON), &commands); err != nil {
		return nil, fmt.Errorf("decode verification commands: %w", ErrVerificationUnavailable)
	}
	criteria, err := readVerificationCriteria(ctx, tx, changeID, ticketID, final != 0)
	if err != nil {
		return nil, err
	}
	reviewInput, err := verificationReviewInput(changeID, ticketID, criteria)
	if err != nil {
		return nil, err
	}
	if len([]byte(reviewInput)) > governancedomain.MaxReviewInputBytes {
		return nil, fmt.Errorf("verification review input: %w", ErrVerificationConflict)
	}
	var attempt int
	if err := tx.QueryRowContext(ctx, `SELECT attempt FROM t_verification_intents WHERE intent_id = ?`, intentID).Scan(&attempt); err != nil {
		return nil, fmt.Errorf("read verification attempt: %w", ErrVerificationUnavailable)
	}
	stage := string(domain.LifecycleStageVerify)
	if final != 0 {
		stage = string(domain.LifecycleStageFinalVerify)
	}
	var latestRunAttempt sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM t_agent_runs WHERE change_id = ? AND stage = ?`, changeID, stage).Scan(&latestRunAttempt); err != nil {
		return nil, fmt.Errorf("read verification run attempt: %w", ErrVerificationUnavailable)
	}
	runAttempt := 1
	if latestRunAttempt.Valid {
		runAttempt = int(latestRunAttempt.Int64) + 1
	}
	runID, leaseID := id.New(), id.New()
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_agent_runs (agent_run_id, project_id, change_id, stage, attempt, status, outcome, started_at, run_kind, source_revision, ticket_id, authorization_id) VALUES (?, ?, ?, ?, ?, 'running', '', ?, '', ?, ?, '')`, runID, projectID, changeID, stage, runAttempt, stamp(now), inputRevision, ticketID); err != nil {
		return nil, fmt.Errorf("insert verification agent run: %w", ErrVerificationUnavailable)
	}
	token, err := NewWorkerSecret()
	if err != nil {
		return nil, err
	}
	expires := now.Add(workerLeaseTTL)
	envelopeDigest := digestString(intentID + "\x00" + treeIdentity + "\x00" + policyDigest)
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_worker_leases (lease_id, agent_run_id, worker_id, attempt, token_sha256, state, expires_at, workspace_id, workspace_path, runtime, instruction, before_revision, input_artifacts_json, created_at, result_mode, runtime_claim_id, claim_state, execution_mode, timeout_seconds, envelope_digest) VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, 'verification', '', ?, '[]', ?, '', '', '', 'inspect', 1800, ?)`, leaseID, runID, workerID, runAttempt, hashSecret(token), stamp(expires), workspaceID, workspacePath, inputRevision, stamp(now), envelopeDigest); err != nil {
		return nil, fmt.Errorf("persist verification lease: %w", ErrVerificationUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_verification_intents SET status = 'running', agent_run_id = ?, updated_at = ? WHERE intent_id = ? AND status = 'pending'`, runID, stamp(now), intentID); err != nil {
		return nil, fmt.Errorf("start verification intent: %w", ErrVerificationUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_instances SET last_heartbeat_at = NULL WHERE worker_id = ? AND status = 'registered'`, workerID); err != nil {
		return nil, fmt.Errorf("occupy verification worker: %w", ErrVerificationUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit verification dispatch: %w", ErrVerificationUnavailable)
	}
	committed = true
	s.leaseMu.Lock()
	s.leaseTokens[leaseID] = token
	s.leaseMu.Unlock()
	verificationCommands := make([]workercontract.VerificationCommand, 0, len(commands))
	for _, command := range commands {
		verificationCommands = append(verificationCommands, workercontract.VerificationCommand{Name: command.Name, Argv: command.Argv, TimeoutSeconds: command.TimeoutSeconds})
	}
	return &workercontract.Assignment{Kind: workercontract.AssignmentKindVerify, AgentRunID: runID, LeaseToken: token, WorkspaceID: workspaceID, WorkspacePath: workspacePath, Runtime: "verification", ExecutionMode: "inspect", TimeoutSeconds: 1800, LeaseExpiresAt: stamp(expires), BeforeRevision: inputRevision, Attempt: runAttempt, Verification: &workercontract.VerificationAssignment{IntentID: intentID, TicketID: ticketID, Final: final != 0, PolicyDigest: policyDigest, InputRevision: inputRevision, CandidateTreeIdentity: treeIdentity, Commands: verificationCommands, Criteria: criteria, ReviewInput: reviewInput}}, nil
}

func readVerificationCriteria(ctx context.Context, queryer sqlQueryContext, changeID, ticketID string, final bool) ([]workercontract.AcceptanceCriterionRef, error) {
	query := `SELECT criteria.ticket_id, criteria.ordinal, criteria.text FROM t_ticket_acceptance_criteria criteria JOIN t_tickets ticket ON ticket.ticket_id = criteria.ticket_id JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id WHERE graph.change_id = ?`
	args := []any{changeID}
	if !final {
		query += ` AND criteria.ticket_id = ?`
		args = append(args, ticketID)
	}
	query += ` ORDER BY ticket.ordinal, criteria.ordinal`
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read verification criteria: %w", ErrVerificationUnavailable)
	}
	defer rows.Close()
	result := make([]workercontract.AcceptanceCriterionRef, 0)
	for rows.Next() {
		var ref workercontract.AcceptanceCriterionRef
		if err := rows.Scan(&ref.TicketID, &ref.Ordinal, &ref.Text); err != nil {
			return nil, fmt.Errorf("scan verification criterion: %w", ErrVerificationUnavailable)
		}
		ref.TextSHA256 = textSHA256(ref.Text)
		result = append(result, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read verification criteria: %w", ErrVerificationUnavailable)
	}
	return result, nil
}

func verificationReviewInput(changeID, ticketID string, criteria []workercontract.AcceptanceCriterionRef) (string, error) {
	value, err := json.Marshal(struct {
		ChangeID string                                  `json:"change_id"`
		TicketID string                                  `json:"ticket_id,omitempty"`
		Criteria []workercontract.AcceptanceCriterionRef `json:"criteria"`
	}{changeID, ticketID, criteria})
	if err != nil {
		return "", fmt.Errorf("encode verification review input: %w", ErrVerificationUnavailable)
	}
	return string(value), nil
}

func textSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// VerificationAssignmentInfo 是 Worker Report 前独立 Snapshot 观察所需的内部身份。
type VerificationAssignmentInfo struct {
	LeaseID               string
	WorkspacePath         string
	Branch                string
	InputRevision         string
	CandidateTreeIdentity string
}

// VerificationAssignmentInfoFor 只向持有匹配 Lease 的 Worker 返回验证目标。
func (s *Store) VerificationAssignmentInfoFor(ctx context.Context, workerID, agentRunID, leaseToken string) (VerificationAssignmentInfo, error) {
	if ctx == nil || workerID == "" || agentRunID == "" || leaseToken == "" {
		return VerificationAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	var info VerificationAssignmentInfo
	var worker, tokenDigest, state, expires string
	err := s.db.QueryRowContext(ctx, `SELECT lease.lease_id, lease.worker_id, lease.token_sha256, lease.state, lease.expires_at, workspace.workspace_path, workspace.branch, intent.input_revision, intent.candidate_tree_identity FROM t_worker_leases lease JOIN t_verification_intents intent ON intent.agent_run_id = lease.agent_run_id JOIN t_workspaces workspace ON workspace.change_id = intent.change_id WHERE lease.agent_run_id = ?`, agentRunID).Scan(&info.LeaseID, &worker, &tokenDigest, &state, &expires, &info.WorkspacePath, &info.Branch, &info.InputRevision, &info.CandidateTreeIdentity)
	if errors.Is(err, sql.ErrNoRows) {
		return VerificationAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	if err != nil || worker != workerID || subtle.ConstantTimeCompare([]byte(tokenDigest), []byte(hashSecret(leaseToken))) != 1 || state != "active" {
		return VerificationAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	deadline, err := parseStamp(expires)
	if err != nil || !s.now().UTC().Before(deadline) {
		return VerificationAssignmentInfo{}, ErrWorkerLeaseInvalid
	}
	return info, nil
}

// FenceVerificationAssignment 在独立观察发现候选变化时固定人审边界，不做清理或回滚。
func (s *Store) FenceVerificationAssignment(ctx context.Context, agentRunID, reason string) error {
	if ctx == nil || agentRunID == "" {
		return ErrWorkerLeaseInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin verification fence: %w", ErrVerificationUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var leaseID, intentID, changeID, status string
	if err := tx.QueryRowContext(ctx, `SELECT lease.lease_id, intent.intent_id, intent.change_id, lease.state FROM t_worker_leases lease JOIN t_verification_intents intent ON intent.agent_run_id = lease.agent_run_id WHERE lease.agent_run_id = ?`, agentRunID).Scan(&leaseID, &intentID, &changeID, &status); err != nil {
		return ErrWorkerLeaseInvalid
	}
	if status == "active" {
		if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'revoked', claim_state = 'fenced', workspace_path = '' WHERE lease_id = ? AND state = 'active'`, leaseID); err != nil {
			return fmt.Errorf("revoke verification lease: %w", ErrVerificationUnavailable)
		}
	}
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE t_verification_intents SET status = 'human_required', updated_at = ? WHERE intent_id = ? AND status IN ('pending', 'running')`, stamp(now), intentID); err != nil {
		return fmt.Errorf("fence verification intent: %w", ErrVerificationUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = 'human_required', completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, stamp(now), agentRunID); err != nil {
		return fmt.Errorf("fence verification run: %w", ErrVerificationUnavailable)
	}
	change, err := readChange(ctx, tx, domain.ChangeID(changeID))
	if err != nil {
		return err
	}
	if change.Status == domain.ChangeStatusActive {
		next, transitionErr := change.EnterHumanRequired()
		if transitionErr != nil {
			return transitionErr
		}
		next.UpdatedAt = now
		if err := updateChangeStatus(ctx, tx, change, next, now); err != nil {
			return err
		}
	}
	if reason == "" {
		reason = "verification_snapshot_changed"
	}
	if err := insertGovernanceEventTx(ctx, tx, string(change.ProjectID), changeID, "verification_fenced", intentID, map[string]any{"reason": reason}, stamp(now)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit verification fence: %w", ErrVerificationUnavailable)
	}
	committed = true
	s.forgetLeaseToken(leaseID)
	return nil
}

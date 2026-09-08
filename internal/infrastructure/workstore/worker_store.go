package workstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	workerHeartbeatInterval   = 5 * time.Second
	workerLeaseTTL            = 30 * time.Second
	maxWorkerArtifactBytes    = 16 << 20
	maxPlanningCandidateBytes = 1 << 20
	maxChangedFilesBytes      = 1 << 20
	// 47 MiB 原始内容经 base64 与 JSON 封装后仍可进入 64 MiB Worker Protocol body。
	maxWorkerReportBytes = 47 << 20
)

var (
	// ErrWorkerUnauthorized 表示 Worker 身份或 Bearer secret 无法验证。
	ErrWorkerUnauthorized = errors.New("worker is unauthorized")
	// ErrWorkerNotRegistered 表示 Worker 尚未完成注册或已被撤销。
	ErrWorkerNotRegistered = errors.New("worker is not registered")
	// ErrWorkerLeaseInvalid 表示 Report 没有匹配的 Lease。
	ErrWorkerLeaseInvalid = errors.New("worker lease is invalid")
	// ErrWorkerUnavailable 表示 Report 在写入权威状态前需要重试。
	ErrWorkerUnavailable = errors.New("worker report is temporarily unavailable")
	// ErrWorkerAssignmentConflict 表示 AgentRun 已经拥有不可复用的 Lease。
	ErrWorkerAssignmentConflict = errors.New("agent run already has a worker lease")
	// ErrWorkerCapabilityUnavailable 表示 Worker 未声明 Assignment 所需 Runtime。
	ErrWorkerCapabilityUnavailable = errors.New("worker capability is unavailable")
	// ErrWorkerReportInvalid 表示 Report 不符合 Worker 可提交的传输语义。
	ErrWorkerReportInvalid = errors.New("worker report is invalid")
)

// WorkerArtifactStore 是 Worker Report 写 Artifact 的最小端口，便于测试注入故障。
type WorkerArtifactStore interface {
	Put(context.Context, []byte) (domain.ArtifactIdentity, error)
}

// WorkerHeartbeatInterval 返回 Worker 默认心跳间隔。
func WorkerHeartbeatInterval() time.Duration { return workerHeartbeatInterval }

// WorkerLeaseTTL 返回新 Lease 的默认有效期。
func WorkerLeaseTTL() time.Duration { return workerLeaseTTL }

// WorkerProcessLost 判断受监管 Worker 是否已经越过可证明的心跳或 Lease 边界。
// 本方法只报告事实；Supervisor 必须先停稳 OS 进程树，再调用 ReconcileWorkerLost。
func (s *Store) WorkerProcessLost(ctx context.Context, workerID string) (bool, error) {
	if ctx == nil || strings.TrimSpace(workerID) == "" {
		return false, fmt.Errorf("inspect worker process: %w", domain.ErrInvalidRequest)
	}
	var status, created string
	var registeredAt, heartbeatAt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT status, created_at, registered_at, last_heartbeat_at FROM t_worker_instances WHERE worker_id = ?`, workerID).Scan(&status, &created, &registeredAt, &heartbeatAt)
	if errors.Is(err, sqlErrNoRows()) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect worker process: %w", ErrWorkerUnavailable)
	}
	if status == "revoked" {
		return true, nil
	}
	var leaseState, leaseExpires string
	err = s.db.QueryRowContext(ctx, `
SELECT lease.state, lease.expires_at
FROM t_worker_leases lease
JOIN t_agent_runs run ON run.agent_run_id = lease.agent_run_id
WHERE lease.worker_id = ?
  AND lease.report_digest IS NULL
  AND run.status = 'running'
  AND NOT (
      run.run_kind = 'planning'
      AND EXISTS (
          SELECT 1 FROM t_planning_run_candidates candidate
          WHERE candidate.agent_run_id = run.agent_run_id
      )
  )
ORDER BY lease.created_at DESC, lease.lease_id DESC
LIMIT 1`, workerID).Scan(&leaseState, &leaseExpires)
	if err == nil {
		deadline, parseErr := parseStamp(leaseExpires)
		if parseErr != nil {
			return false, fmt.Errorf("inspect worker lease deadline: %w", ErrWorkerUnavailable)
		}
		return leaseState != "active" || !s.now().UTC().Before(deadline), nil
	}
	if !errors.Is(err, sqlErrNoRows()) {
		return false, fmt.Errorf("inspect worker lease: %w", ErrWorkerUnavailable)
	}

	latest, err := parseStamp(created)
	if err != nil {
		return false, fmt.Errorf("inspect worker creation: %w", ErrWorkerUnavailable)
	}
	for _, value := range []sql.NullString{registeredAt, heartbeatAt} {
		if !value.Valid {
			continue
		}
		observed, parseErr := parseStamp(value.String)
		if parseErr != nil {
			return false, fmt.Errorf("inspect worker heartbeat: %w", ErrWorkerUnavailable)
		}
		if observed.After(latest) {
			latest = observed
		}
	}
	var consumedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(consumed_at) FROM t_worker_leases WHERE worker_id = ?`, workerID).Scan(&consumedAt); err != nil {
		return false, fmt.Errorf("inspect worker completion: %w", ErrWorkerUnavailable)
	}
	if consumedAt.Valid {
		observed, parseErr := parseStamp(consumedAt.String)
		if parseErr != nil {
			return false, fmt.Errorf("inspect worker completion: %w", ErrWorkerUnavailable)
		}
		if observed.After(latest) {
			latest = observed
		}
	}
	return !s.now().UTC().Before(latest.Add(workerLeaseTTL)), nil
}

// NewWorkerSecret 生成只在当前 Worker 进程和启动管道中存在的 secret。
func NewWorkerSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate worker secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// PrepareWorker 写入 WorkerInstance 的不可逆 secret 摘要；不会持久化 secret 明文。
func (s *Store) PrepareWorker(ctx context.Context, workerID, secret string) error {
	if ctx == nil || strings.TrimSpace(workerID) == "" || len(secret) < 32 {
		return fmt.Errorf("prepare worker: %w", ErrWorkerUnauthorized)
	}
	digest := hashSecret(secret)
	capabilities, err := json.Marshal([]string{})
	if err != nil {
		return fmt.Errorf("encode worker capabilities: %w", err)
	}
	now := stamp(s.now().UTC())
	if _, err := s.db.ExecContext(ctx, `INSERT INTO t_worker_instances (worker_id, protocol_version, capabilities_json, secret_sha256, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`, workerID, workercontract.ProtocolVersionV1, string(capabilities), digest, now); err != nil {
		return fmt.Errorf("persist worker instance: %w", err)
	}
	return nil
}

// AuthenticateWorker 以常量时间比较校验当前 WorkerInstance 的 Bearer secret。
func (s *Store) AuthenticateWorker(ctx context.Context, workerID, secret string) error {
	if ctx == nil || strings.TrimSpace(workerID) == "" || secret == "" {
		return ErrWorkerUnauthorized
	}
	var expected, status string
	err := s.db.QueryRowContext(ctx, `SELECT secret_sha256, status FROM t_worker_instances WHERE worker_id = ?`, workerID).Scan(&expected, &status)
	if errors.Is(err, sqlErrNoRows()) {
		return ErrWorkerUnauthorized
	}
	if err != nil {
		return fmt.Errorf("authenticate worker: %w", ErrWorkerUnavailable)
	}
	if status != "pending" && status != "registered" {
		return ErrWorkerUnauthorized
	}
	actual := hashSecret(secret)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return ErrWorkerUnauthorized
	}
	return nil
}

// WorkerIDForSecret 从当前已注册 Worker 中解析 Bearer secret 对应的身份。
func (s *Store) WorkerIDForSecret(ctx context.Context, secret string) (string, error) {
	if ctx == nil || secret == "" {
		return "", ErrWorkerUnauthorized
	}
	actual := hashSecret(secret)
	rows, err := s.db.QueryContext(ctx, `SELECT worker_id, secret_sha256, status FROM t_worker_instances WHERE status = 'registered'`)
	if err != nil {
		return "", fmt.Errorf("read worker credentials: %w", ErrWorkerUnavailable)
	}
	defer rows.Close()
	for rows.Next() {
		var workerID, expected, status string
		if err := rows.Scan(&workerID, &expected, &status); err != nil {
			return "", fmt.Errorf("scan worker credentials: %w", ErrWorkerUnavailable)
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1 && status == "registered" {
			return workerID, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read worker credentials: %w", ErrWorkerUnavailable)
	}
	return "", ErrWorkerUnauthorized
}

// RegisterWorker 记录已鉴权 Worker 的能力，并返回非敏感会话参数。
func (s *Store) RegisterWorker(ctx context.Context, request workercontract.Register) (workercontract.RegisterResponse, error) {
	if err := request.Validate(); err != nil || request.ProtocolVersion != workercontract.ProtocolVersionV1 {
		return workercontract.RegisterResponse{}, fmt.Errorf("register worker: %w", ErrWorkerNotRegistered)
	}
	capabilities := append([]string(nil), request.Capabilities...)
	sort.Strings(capabilities)
	encoded, err := json.Marshal(capabilities)
	if err != nil {
		return workercontract.RegisterResponse{}, fmt.Errorf("encode worker capabilities: %w", ErrWorkerUnavailable)
	}
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE t_worker_instances SET protocol_version = ?, capabilities_json = ?, status = 'registered', registered_at = COALESCE(registered_at, ?), last_heartbeat_at = ? WHERE worker_id = ? AND status IN ('pending', 'registered')`, request.ProtocolVersion, string(encoded), stamp(now), stamp(now), request.WorkerID)
	if err != nil {
		return workercontract.RegisterResponse{}, fmt.Errorf("register worker: %w", ErrWorkerUnavailable)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return workercontract.RegisterResponse{}, ErrWorkerNotRegistered
	}
	return workercontract.RegisterResponse{
		WorkerID:              request.WorkerID,
		ProtocolVersion:       request.ProtocolVersion,
		HeartbeatIntervalSecs: int(workerHeartbeatInterval / time.Second),
		LeaseTTLSeconds:       int(workerLeaseTTL / time.Second),
		Capabilities:          capabilities,
	}, nil
}

// AvailableWorker 返回具备 capability 且当前没有 active Lease 的首个已注册 Worker。
func (s *Store) AvailableWorker(ctx context.Context, capability string) (string, bool, error) {
	if ctx == nil || strings.TrimSpace(capability) == "" {
		return "", false, fmt.Errorf("find available worker: %w", domain.ErrInvalidRequest)
	}
	if err := s.reconcileExpiredExecutionLeases(ctx); err != nil {
		return "", false, err
	}
	now := stamp(s.now().UTC())
	if _, err := s.db.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired', workspace_path = CASE WHEN result_mode = 'planning_candidate' THEN '' ELSE workspace_path END WHERE state = 'active' AND expires_at <= ?`, now); err != nil {
		return "", false, fmt.Errorf("expire worker leases: %w", ErrWorkerUnavailable)
	}
	if err := s.forgetTerminalLeaseTokens(ctx); err != nil {
		return "", false, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT worker_id, capabilities_json FROM t_worker_instances w WHERE status = 'registered' AND last_heartbeat_at IS NOT NULL AND NOT EXISTS (SELECT 1 FROM t_worker_leases l WHERE l.worker_id = w.worker_id AND l.state = 'active') ORDER BY registered_at, worker_id`)
	if err != nil {
		return "", false, fmt.Errorf("list available workers: %w", ErrWorkerUnavailable)
	}
	defer rows.Close()
	for rows.Next() {
		var workerID, encoded string
		if err := rows.Scan(&workerID, &encoded); err != nil {
			return "", false, fmt.Errorf("scan available worker: %w", ErrWorkerUnavailable)
		}
		var capabilities []string
		if err := json.Unmarshal([]byte(encoded), &capabilities); err != nil {
			return "", false, fmt.Errorf("decode worker capabilities: %w", ErrWorkerUnavailable)
		}
		if containsWorkerCapability(capabilities, capability) {
			return workerID, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("list available workers: %w", ErrWorkerUnavailable)
	}
	return "", false, nil
}

// HeartbeatWorker 只为当前 Worker 的匹配 active Lease 续租。
func (s *Store) HeartbeatWorker(ctx context.Context, request workercontract.Heartbeat) (workercontract.HeartbeatResponse, error) {
	if err := request.Validate(); err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("heartbeat worker: %w", ErrWorkerNotRegistered)
	}
	if err := s.reconcileExpiredExecutionLeases(ctx); err != nil {
		return workercontract.HeartbeatResponse{}, err
	}
	now := s.now().UTC()
	if request.AgentRunID == "" {
		return s.heartbeatIdleWorker(ctx, request.WorkerID, now)
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM t_worker_instances WHERE worker_id = ?`, request.WorkerID).Scan(&status); err != nil {
		if errors.Is(err, sqlErrNoRows()) {
			return workercontract.HeartbeatResponse{}, ErrWorkerNotRegistered
		}
		return workercontract.HeartbeatResponse{}, fmt.Errorf("read worker status: %w", ErrWorkerUnavailable)
	}
	if status != "registered" {
		return workercontract.HeartbeatResponse{}, ErrWorkerNotRegistered
	}
	response := workercontract.HeartbeatResponse{WorkerID: request.WorkerID}
	var leaseID, tokenDigest, state, expires string
	err := s.db.QueryRowContext(ctx, `SELECT lease_id, token_sha256, state, expires_at FROM t_worker_leases WHERE agent_run_id = ? AND worker_id = ?`, request.AgentRunID, request.WorkerID).Scan(&leaseID, &tokenDigest, &state, &expires)
	if errors.Is(err, sqlErrNoRows()) {
		return response, nil
	}
	if err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("read worker lease: %w", ErrWorkerUnavailable)
	}
	response.LeaseExpiresAt = expires
	if state != "active" || request.LeaseTokenSHA256 == "" || subtle.ConstantTimeCompare([]byte(tokenDigest), []byte(request.LeaseTokenSHA256)) != 1 {
		return response, nil
	}
	deadline, err := parseStamp(expires)
	if err != nil || !now.Before(deadline) {
		if result, updateErr := s.db.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired', workspace_path = CASE WHEN result_mode = 'planning_candidate' THEN '' ELSE workspace_path END WHERE lease_id = ? AND state = 'active'`, leaseID); updateErr == nil {
			if count, rowsErr := result.RowsAffected(); rowsErr == nil && count == 1 {
				s.forgetLeaseToken(leaseID)
			}
		}
		return response, nil
	}
	newDeadline := now.Add(workerLeaseTTL)
	result, err := s.db.ExecContext(ctx, `UPDATE t_worker_leases SET expires_at = ? WHERE lease_id = ? AND state = 'active' AND expires_at = ?`, stamp(newDeadline), leaseID, expires)
	if err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("renew worker lease: %w", ErrWorkerUnavailable)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("renew worker lease: %w", ErrWorkerUnavailable)
	}
	if count != 1 {
		return response, nil
	}
	response.LeaseRenewed = true
	response.LeaseExpiresAt = stamp(newDeadline)
	return response, nil
}

// heartbeatIdleWorker 把 Worker 的空闲声明作为“当前没有 Runtime 使用旧 Workspace”的显式确认。
// 只有这个确认、终态 Report 或 Supervisor 进程退出才能让过期 Planning run 形成失败候选。
func (s *Store) heartbeatIdleWorker(ctx context.Context, workerID string, now time.Time) (workercontract.HeartbeatResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("begin idle worker heartbeat: %w", ErrWorkerUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM t_worker_instances WHERE worker_id = ?`, workerID).Scan(&status); err != nil || status != "registered" {
		return workercontract.HeartbeatResponse{}, ErrWorkerNotRegistered
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired', workspace_path = '' WHERE worker_id = ? AND result_mode = 'planning_candidate' AND state = 'active' AND expires_at <= ?`, workerID, stamp(now)); err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("expire idle worker planning leases: %w", ErrWorkerUnavailable)
	}
	runIDs, err := runningPlanningRunIDs(ctx, tx, workerID, false)
	if err != nil {
		return workercontract.HeartbeatResponse{}, err
	}
	for _, runID := range runIDs {
		if err := insertPlanningSystemCandidate(ctx, tx, runID, "lease_expired", now); err != nil {
			return workercontract.HeartbeatResponse{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_instances SET last_heartbeat_at = ? WHERE worker_id = ? AND status = 'registered'`, stamp(now), workerID); err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("record worker heartbeat: %w", ErrWorkerUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("commit idle worker heartbeat: %w", ErrWorkerUnavailable)
	}
	committed = true
	if err := s.forgetTerminalLeaseTokens(ctx); err != nil {
		return workercontract.HeartbeatResponse{}, err
	}
	return workercontract.HeartbeatResponse{WorkerID: workerID, WorkerAvailable: true}, nil
}

// IssueAssignment 为已创建的 running AgentRun 发放一个不可复用 Lease。
func (s *Store) IssueAssignment(ctx context.Context, runID domain.AgentRunID, workerID, workspacePath, runtime, instruction, beforeRevision, workspaceID string, inputs []workercontract.ArtifactSummary) (workercontract.Assignment, error) {
	return s.issueAssignment(ctx, runID, workerID, workspacePath, runtime, instruction, beforeRevision, workspaceID, inputs, "")
}

// IssuePlanningAssignment 为 Planning run 发放只返回非权威 candidate 的 Lease。
func (s *Store) IssuePlanningAssignment(ctx context.Context, runID domain.AgentRunID, workerID, workspacePath, runtime, instruction, beforeRevision, workspaceID string, inputs []workercontract.ArtifactSummary) (workercontract.Assignment, error) {
	return s.issueAssignment(ctx, runID, workerID, workspacePath, runtime, instruction, beforeRevision, workspaceID, inputs, workercontract.ResultModePlanningCandidate)
}

// PlanningRunAssigned 报告当前 Planning run 是否已经持有过不可复用的 Assignment Lease。
func (s *Store) PlanningRunAssigned(ctx context.Context, runID domain.AgentRunID) (bool, error) {
	if ctx == nil || runID == "" {
		return false, domain.ErrInvalidRequest
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_worker_leases lease JOIN t_agent_runs run ON run.agent_run_id = lease.agent_run_id WHERE lease.agent_run_id = ? AND run.run_kind = 'planning'`, runID).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect planning assignment: %w", ErrWorkerUnavailable)
	}
	return count != 0, nil
}

func (s *Store) issueAssignment(ctx context.Context, runID domain.AgentRunID, workerID, workspacePath, runtime, instruction, beforeRevision, workspaceID string, inputs []workercontract.ArtifactSummary, resultMode string) (workercontract.Assignment, error) {
	if ctx == nil || runID == "" || strings.TrimSpace(workerID) == "" || strings.TrimSpace(runtime) == "" {
		return workercontract.Assignment{}, fmt.Errorf("issue worker assignment: %w", domain.ErrInvalidRequest)
	}
	workspacePath, err := filepath.Abs(filepath.Clean(workspacePath))
	if err != nil || workspacePath == "" {
		return workercontract.Assignment{}, fmt.Errorf("issue worker assignment: %w", domain.ErrInvalidRequest)
	}
	info, err := os.Stat(workspacePath)
	if err != nil || !info.IsDir() {
		return workercontract.Assignment{}, fmt.Errorf("issue worker assignment: %w", domain.ErrRepositoryUnsupported)
	}
	for _, input := range inputs {
		if err := validateArtifactSummary(input); err != nil {
			return workercontract.Assignment{}, err
		}
	}
	if workspaceID == "" {
		digest := sha256.Sum256([]byte(workspacePath))
		workspaceID = "workspace-" + hex.EncodeToString(digest[:8])
	}
	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return workercontract.Assignment{}, fmt.Errorf("encode worker assignment inputs: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workercontract.Assignment{}, fmt.Errorf("begin worker assignment: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var workerStatus, capabilitiesJSON string
	var idleAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, capabilities_json, last_heartbeat_at FROM t_worker_instances WHERE worker_id = ?`, workerID).Scan(&workerStatus, &capabilitiesJSON, &idleAt); err != nil {
		if errors.Is(err, sqlErrNoRows()) {
			return workercontract.Assignment{}, ErrWorkerNotRegistered
		}
		return workercontract.Assignment{}, fmt.Errorf("read assignment worker: %w", err)
	}
	if workerStatus != "registered" || !idleAt.Valid {
		return workercontract.Assignment{}, ErrWorkerNotRegistered
	}
	var capabilities []string
	if err := json.Unmarshal([]byte(capabilitiesJSON), &capabilities); err != nil || !containsWorkerCapability(capabilities, "runtime:"+runtime) {
		return workercontract.Assignment{}, ErrWorkerCapabilityUnavailable
	}
	run, err := readAgentRun(ctx, tx, runID)
	if err != nil {
		return workercontract.Assignment{}, err
	}
	if run.Status != domain.AgentRunStatusRunning {
		return workercontract.Assignment{}, fmt.Errorf("issue worker assignment: %w", ErrWorkerAssignmentConflict)
	}
	if (resultMode == workercontract.ResultModePlanningCandidate) != run.IsPlanning() {
		return workercontract.Assignment{}, fmt.Errorf("issue worker assignment result mode: %w", ErrWorkerAssignmentConflict)
	}
	if run.IsPlanning() {
		if beforeRevision != run.SourceRevision {
			return workercontract.Assignment{}, fmt.Errorf("issue planning assignment revision: %w", ErrWorkerAssignmentConflict)
		}
		change, changeErr := readChange(ctx, tx, run.ChangeID)
		if changeErr != nil {
			return workercontract.Assignment{}, changeErr
		}
		current, currentErr := isCurrentPlanningRun(ctx, tx, change, run)
		if currentErr != nil {
			return workercontract.Assignment{}, currentErr
		}
		if change.Status != domain.ChangeStatusActive || !current {
			return workercontract.Assignment{}, fmt.Errorf("issue planning assignment authority fence: %w", ErrWorkerAssignmentConflict)
		}
	}
	var existingState string
	err = tx.QueryRowContext(ctx, `SELECT state FROM t_worker_leases WHERE agent_run_id = ?`, runID).Scan(&existingState)
	if err == nil {
		return workercontract.Assignment{}, fmt.Errorf("issue worker assignment: %w", ErrWorkerAssignmentConflict)
	}
	if !errors.Is(err, sqlErrNoRows()) {
		return workercontract.Assignment{}, fmt.Errorf("read existing worker lease: %w", err)
	}
	token, err := NewWorkerSecret()
	if err != nil {
		return workercontract.Assignment{}, err
	}
	leaseID := id.New()
	now := s.now().UTC()
	expires := now.Add(workerLeaseTTL)
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_worker_leases (lease_id, agent_run_id, worker_id, attempt, token_sha256, state, expires_at, workspace_id, workspace_path, runtime, instruction, before_revision, input_artifacts_json, created_at, result_mode) VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?, ?)`, leaseID, run.ID, workerID, run.Attempt, hashSecret(token), stamp(expires), workspaceID, workspacePath, runtime, instruction, beforeRevision, string(inputJSON), stamp(now), resultMode); err != nil {
		return workercontract.Assignment{}, fmt.Errorf("persist worker lease: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_instances SET last_heartbeat_at = NULL WHERE worker_id = ? AND status = 'registered'`, workerID); err != nil {
		return workercontract.Assignment{}, fmt.Errorf("mark worker busy: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return workercontract.Assignment{}, fmt.Errorf("commit worker assignment: %w", err)
	}
	committed = true
	s.leaseMu.Lock()
	s.leaseTokens[leaseID] = token
	s.leaseMu.Unlock()
	return workercontract.Assignment{
		AgentRunID:     string(run.ID),
		LeaseToken:     token,
		WorkspaceID:    workspaceID,
		WorkspacePath:  workspacePath,
		Runtime:        runtime,
		ResultMode:     resultMode,
		LeaseExpiresAt: stamp(expires),
		Instruction:    instruction,
		BeforeRevision: beforeRevision,
		Attempt:        run.Attempt,
		InputArtifacts: append([]workercontract.ArtifactSummary(nil), inputs...),
	}, nil
}

// PullAssignment 返回当前 Worker 的单一 active Assignment；没有任务时返回 nil。
func (s *Store) PullAssignment(ctx context.Context, workerID string) (*workercontract.Assignment, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, ErrWorkerNotRegistered
	}
	if err := s.reconcileExpiredExecutionLeases(ctx); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired', workspace_path = CASE WHEN result_mode = 'planning_candidate' THEN '' ELSE workspace_path END WHERE worker_id = ? AND state = 'active' AND expires_at <= ?`, workerID, stamp(now)); err != nil {
		return nil, fmt.Errorf("expire worker leases: %w", ErrWorkerUnavailable)
	}
	if err := s.forgetTerminalLeaseTokens(ctx); err != nil {
		return nil, err
	}
	var leaseID, agentRunID, state, expires, workspaceID, workspacePath, runtime, resultMode, executionMode, instruction, beforeRevision, inputJSON string
	var attempt, timeoutSeconds int
	err := s.db.QueryRowContext(ctx, `SELECT lease_id, agent_run_id, state, expires_at, workspace_id, workspace_path, runtime, result_mode, execution_mode, instruction, before_revision, attempt, timeout_seconds, input_artifacts_json FROM t_worker_leases WHERE worker_id = ? AND state = 'active' ORDER BY created_at, lease_id LIMIT 1`, workerID).Scan(&leaseID, &agentRunID, &state, &expires, &workspaceID, &workspacePath, &runtime, &resultMode, &executionMode, &instruction, &beforeRevision, &attempt, &timeoutSeconds, &inputJSON)
	if errors.Is(err, sqlErrNoRows()) {
		return s.IssueNextExecutionAssignment(ctx, workerID)
	}
	if err != nil {
		return nil, fmt.Errorf("read worker assignment: %w", ErrWorkerUnavailable)
	}
	var inputs []workercontract.ArtifactSummary
	if err := json.Unmarshal([]byte(inputJSON), &inputs); err != nil {
		return nil, fmt.Errorf("decode worker assignment inputs: %w", ErrWorkerUnavailable)
	}
	s.leaseMu.RLock()
	token := s.leaseTokens[leaseID]
	s.leaseMu.RUnlock()
	if token == "" {
		return nil, fmt.Errorf("worker lease secret is unavailable: %w", ErrWorkerUnavailable)
	}
	return &workercontract.Assignment{
		AgentRunID:     agentRunID,
		LeaseToken:     token,
		LeaseExpiresAt: expires,
		WorkspaceID:    workspaceID,
		WorkspacePath:  workspacePath,
		Runtime:        runtime,
		ResultMode:     resultMode,
		ExecutionMode:  executionMode,
		TimeoutSeconds: timeoutSeconds,
		Instruction:    instruction,
		BeforeRevision: beforeRevision,
		Attempt:        attempt,
		InputArtifacts: inputs,
	}, nil
}

// ClaimExecution 原子绑定唯一 RuntimeClaim；重复提交同一 Claim 只返回 duplicate。
func (s *Store) ClaimExecution(ctx context.Context, workerID string, request workercontract.ClaimRequest) (workercontract.ClaimResponse, error) {
	if ctx == nil || strings.TrimSpace(workerID) == "" {
		return workercontract.ClaimResponse{}, ErrWorkerUnauthorized
	}
	if err := request.Validate(); err != nil {
		return workercontract.ClaimResponse{}, ErrWorkerReportInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workercontract.ClaimResponse{}, fmt.Errorf("begin runtime claim: %w", ErrWorkerUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var leaseID, leaseWorker, tokenDigest, state, currentClaim, claimState, expiresAt, executionMode string
	err = tx.QueryRowContext(ctx, `SELECT lease_id, worker_id, token_sha256, state, runtime_claim_id, claim_state, expires_at, execution_mode FROM t_worker_leases WHERE agent_run_id = ?`, request.AgentRunID).Scan(&leaseID, &leaseWorker, &tokenDigest, &state, &currentClaim, &claimState, &expiresAt, &executionMode)
	if errors.Is(err, sql.ErrNoRows) {
		return workercontract.ClaimResponse{}, ErrWorkerLeaseInvalid
	}
	if err != nil {
		return workercontract.ClaimResponse{}, fmt.Errorf("read runtime claim lease: %w", ErrWorkerUnavailable)
	}
	if leaseWorker != workerID || subtle.ConstantTimeCompare([]byte(tokenDigest), []byte(hashSecret(request.LeaseToken))) != 1 || executionMode != "edit" {
		return workercontract.ClaimResponse{}, ErrWorkerLeaseInvalid
	}
	deadline, deadlineErr := parseStamp(expiresAt)
	if state != "active" || deadlineErr != nil || !s.now().UTC().Before(deadline) {
		return workercontract.ClaimResponse{}, ErrWorkerLeaseInvalid
	}
	if claimState == "claimed" || currentClaim != "" {
		if currentClaim == request.RuntimeClaimID && claimState == "claimed" {
			if err := tx.Commit(); err != nil {
				return workercontract.ClaimResponse{}, fmt.Errorf("commit duplicate runtime claim: %w", ErrWorkerUnavailable)
			}
			committed = true
			return workercontract.ClaimResponse{Disposition: "duplicate", AgentRunID: request.AgentRunID, RuntimeClaimID: currentClaim, LeaseExpiresAt: expiresAt}, nil
		}
		return workercontract.ClaimResponse{}, ErrWorkerAssignmentConflict
	}
	var claimedCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_worker_leases WHERE worker_id = ? AND state = 'active' AND claim_state = 'claimed'`, workerID).Scan(&claimedCount); err != nil {
		return workercontract.ClaimResponse{}, fmt.Errorf("inspect worker runtime claims: %w", ErrWorkerUnavailable)
	}
	if claimedCount != 0 {
		return workercontract.ClaimResponse{}, ErrWorkerAssignmentConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET runtime_claim_id = ?, claim_state = 'claimed' WHERE lease_id = ? AND state = 'active' AND claim_state = ''`, request.RuntimeClaimID, leaseID); err != nil {
		return workercontract.ClaimResponse{}, fmt.Errorf("persist runtime claim: %w", ErrWorkerUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return workercontract.ClaimResponse{}, fmt.Errorf("commit runtime claim: %w", ErrWorkerUnavailable)
	}
	committed = true
	return workercontract.ClaimResponse{Disposition: "claimed", AgentRunID: request.AgentRunID, RuntimeClaimID: request.RuntimeClaimID, LeaseExpiresAt: expiresAt}, nil
}

// ReportWorker 校验独立证据并原子完成 AgentRun、ArtifactRef、Lease 和 Event。
// 该方法供内部受控测试 seam 使用；HTTP Handler 应调用 ReportWorkerFor 绑定鉴权身份。
func (s *Store) ReportWorker(ctx context.Context, request workercontract.Report, artifacts WorkerArtifactStore) (workercontract.ReportResponse, error) {
	return s.reportWorker(ctx, "", request, artifacts)
}

// ReportWorkerFor 将 Report 绑定到已经通过 Bearer 鉴权的 WorkerInstance。
func (s *Store) ReportWorkerFor(ctx context.Context, workerID string, request workercontract.Report, artifacts WorkerArtifactStore) (workercontract.ReportResponse, error) {
	if strings.TrimSpace(workerID) == "" {
		return workercontract.ReportResponse{}, ErrWorkerUnauthorized
	}
	return s.reportWorker(ctx, workerID, request, artifacts)
}

func (s *Store) reportWorker(ctx context.Context, workerID string, request workercontract.Report, artifacts WorkerArtifactStore) (workercontract.ReportResponse, error) {
	if err := request.Validate(); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("report worker: %w", err)
	}
	if (request.Outcome != workercontract.Outcome(domain.AgentRunOutcomeSucceeded) && request.Outcome != workercontract.Outcome(domain.AgentRunOutcomeFailed)) || request.Attempt < 0 {
		return workercontract.ReportResponse{}, ErrWorkerReportInvalid
	}
	prepared, err := prepareReportArtifacts(request)
	if err != nil {
		return workercontract.ReportResponse{}, err
	}
	digest, err := digestReport(request)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("digest worker report: %w", ErrWorkerUnavailable)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("begin worker report: %w", ErrWorkerUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var priorDisposition, priorOutcome, priorAgentRun string
	err = tx.QueryRowContext(ctx, `SELECT disposition, outcome, COALESCE(agent_run_id, '') FROM t_worker_reports WHERE report_digest = ?`, digest).Scan(&priorDisposition, &priorOutcome, &priorAgentRun)
	if err == nil {
		if commitErr := tx.Commit(); commitErr != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("commit duplicate worker report: %w", ErrWorkerUnavailable)
		}
		committed = true
		disposition := priorDisposition
		if disposition == "accepted" || disposition == "accepted_fenced" || disposition == "candidate_received" || disposition == "late" {
			disposition = "duplicate"
		}
		return workercontract.ReportResponse{Disposition: disposition, AgentRunID: priorAgentRun, RetrySameReport: false}, nil
	}
	if !errors.Is(err, sqlErrNoRows()) {
		return workercontract.ReportResponse{}, fmt.Errorf("read worker report receipt: %w", ErrWorkerUnavailable)
	}
	run, err := readAgentRun(ctx, tx, domain.AgentRunID(request.AgentRunID))
	if err != nil {
		return workercontract.ReportResponse{}, err
	}
	change, err := readChange(ctx, tx, run.ChangeID)
	if err != nil {
		return workercontract.ReportResponse{}, err
	}
	lease, err := readWorkerLease(ctx, tx, request.AgentRunID)
	if err != nil {
		return workercontract.ReportResponse{}, ErrWorkerLeaseInvalid
	}
	if subtle.ConstantTimeCompare([]byte(lease.TokenSHA256), []byte(hashSecret(request.LeaseToken))) != 1 || lease.WorkerID == "" || (workerID != "" && lease.WorkerID != workerID) {
		return workercontract.ReportResponse{}, ErrWorkerLeaseInvalid
	}
	planningCandidateMode := run.IsPlanning() && lease.ResultMode == workercontract.ResultModePlanningCandidate
	if run.IsPlanning() != (lease.ResultMode == workercontract.ResultModePlanningCandidate) {
		return workercontract.ReportResponse{}, ErrWorkerReportInvalid
	}
	ticketID, authorizationID, sessionID, epochID, executionRun, executionInfoErr := executionRunInfo(ctx, tx, run.ID)
	if executionInfoErr != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("read execution report identity: %w", ErrWorkerUnavailable)
	}
	if !planningCandidateMode && reportHasCaptureFailures(request) && !executionRun {
		return workercontract.ReportResponse{}, fmt.Errorf("worker evidence capture failed: %w", ErrWorkerUnavailable)
	}
	if lease.State == "consumed" {
		if lease.ReportDigest == digest {
			if err := tx.Commit(); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("commit duplicate worker report: %w", ErrWorkerUnavailable)
			}
			committed = true
			s.forgetLeaseToken(lease.LeaseID)
			return workercontract.ReportResponse{Disposition: "duplicate", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
		}
		if err := tx.Commit(); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("commit terminal worker report conflict: %w", ErrWorkerUnavailable)
		}
		committed = true
		s.forgetLeaseToken(lease.LeaseID)
		return workercontract.ReportResponse{Disposition: "terminal_conflict", ErrorCode: "terminal_conflict", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
	}
	if lease.ReportDigest != "" {
		if err := tx.Commit(); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("commit terminal worker report fence: %w", ErrWorkerUnavailable)
		}
		committed = true
		s.forgetLeaseToken(lease.LeaseID)
		if lease.ReportDigest == digest {
			return workercontract.ReportResponse{Disposition: "duplicate", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
		}
		return workercontract.ReportResponse{Disposition: "terminal_conflict", ErrorCode: "terminal_conflict", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
	}
	now := s.now().UTC()
	deadline, deadlineErr := parseStamp(lease.ExpiresAt)
	attemptMatches := request.Attempt == 0 || request.Attempt == run.Attempt
	current := run.Status == domain.AgentRunStatusRunning && lease.State == "active" && deadlineErr == nil && now.Before(deadline) && lease.Attempt == run.Attempt && attemptMatches
	if current {
		currentRun, currentErr := isCurrentAgentRun(ctx, tx, change, run)
		if planningCandidateMode {
			currentRun, currentErr = isCurrentPlanningRun(ctx, tx, change, run)
		}
		if currentErr != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("check current worker run: %w", ErrWorkerUnavailable)
		}
		if !currentRun {
			current = false
		}
	}
	if executionRun && current {
		return s.persistExecutionReport(ctx, tx, request, digest, prepared, run, change, lease, artifacts, ticketID, authorizationID, sessionID, epochID)
	}
	if !current {
		if lease.State == "active" {
			nextState := "revoked"
			if deadlineErr != nil || !now.Before(deadline) {
				nextState = "expired"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = ?, workspace_path = CASE WHEN result_mode = 'planning_candidate' THEN '' ELSE workspace_path END WHERE lease_id = ? AND state = 'active'`, nextState, lease.LeaseID); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("close non-current worker lease: %w", ErrWorkerUnavailable)
			}
			lease.State = nextState
		}
		if planningCandidateMode && run.Status == domain.AgentRunStatusRunning {
			exists, candidateErr := planningCandidateExists(ctx, tx, run.ID)
			if candidateErr != nil {
				return workercontract.ReportResponse{}, candidateErr
			}
			if !exists {
				response, candidateErr := s.persistPlanningCandidateReport(ctx, tx, request, digest, prepared, run, change, lease, artifacts, false)
				if candidateErr != nil {
					return workercontract.ReportResponse{}, candidateErr
				}
				if err := tx.Commit(); err != nil {
					return workercontract.ReportResponse{}, fmt.Errorf("commit late planning candidate report: %w", ErrWorkerUnavailable)
				}
				committed = true
				s.forgetLeaseToken(lease.LeaseID)
				return response, nil
			}
		}
		if err := storePreparedReportArtifacts(ctx, prepared, artifacts); err != nil {
			return workercontract.ReportResponse{}, err
		}
		response, lateErr := s.persistLateReport(ctx, tx, request, digest, prepared, run, change, lease, "lease_not_active")
		if lateErr != nil {
			return workercontract.ReportResponse{}, lateErr
		}
		if err := tx.Commit(); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("commit late worker report: %w", ErrWorkerUnavailable)
		}
		committed = true
		s.forgetLeaseToken(lease.LeaseID)
		return response, nil
	}
	if planningCandidateMode {
		response, candidateErr := s.persistPlanningCandidateReport(ctx, tx, request, digest, prepared, run, change, lease, artifacts, true)
		if candidateErr != nil {
			return workercontract.ReportResponse{}, candidateErr
		}
		if err := tx.Commit(); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("commit planning candidate report: %w", ErrWorkerUnavailable)
		}
		committed = true
		s.forgetLeaseToken(lease.LeaseID)
		return response, nil
	}
	if containsPreparedWorkerArtifact(prepared, "candidate") {
		return workercontract.ReportResponse{}, ErrWorkerReportInvalid
	}
	outcome := reportOutcome(request)
	role := domain.ArtifactRoleOutput
	if outcome != domain.AgentRunOutcomeSucceeded {
		role = domain.ArtifactRoleFailure
	}
	if err := storePreparedReportArtifacts(ctx, prepared, artifacts); err != nil {
		return workercontract.ReportResponse{}, err
	}
	refs, runArtifacts, err := insertWorkerArtifacts(ctx, tx, change, prepared, role, now)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("persist worker artifacts: %w", ErrWorkerUnavailable)
	}
	if err := run.Complete(outcome, now); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("complete worker run: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = ?, outcome = ?, completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, run.Status, run.Outcome, stamp(now), run.ID); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("complete worker agent run: %w", ErrWorkerUnavailable)
	}
	if err := insertAgentRunArtifacts(ctx, tx, run.ID, runArtifacts); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("link worker artifacts: %w", ErrWorkerUnavailable)
	}
	run.Artifacts = append(run.Artifacts, runArtifacts...)
	actor := "worker:" + request.AgentRunID
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, actor, now, &run.ID, nil, refs); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record worker completion: %w", ErrWorkerUnavailable)
	}
	disposition := "accepted"
	if change.Status == domain.ChangeStatusActive {
		if outcome == domain.AgentRunOutcomeFailed {
			next, transitionErr := change.EnterHumanRequired()
			if transitionErr != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("fence failed worker result: %w", ErrWorkerUnavailable)
			}
			next.UpdatedAt = now
			if err := updateChangeStatus(ctx, tx, change, next, now); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("update failed worker change: %w", ErrWorkerUnavailable)
			}
			if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.ChangeHumanRequiredType, actor, now, &run.ID, nil, refs); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("record worker recovery boundary: %w", ErrWorkerUnavailable)
			}
		} else if nextStage, ok := advanceStage(change.Stage); ok {
			next := change
			next.Stage = nextStage
			next.Version++
			next.UpdatedAt = now
			if err := updateChangeStage(ctx, tx, change, next, now); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("advance worker change: %w", ErrWorkerUnavailable)
			}
			if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.StageAdvancedType, actor, now, &run.ID, nil, refs); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("record worker stage advance: %w", ErrWorkerUnavailable)
			}
		}
	} else {
		disposition = "accepted_fenced"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'consumed', consumed_at = ?, report_digest = ?, report_disposition = ?, report_outcome = ? WHERE lease_id = ? AND state = 'active'`, stamp(now), digest, disposition, outcome, lease.LeaseID); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("consume worker lease: %w", ErrWorkerUnavailable)
	}
	if err := insertWorkerReportReceipt(ctx, tx, digest, request.AgentRunID, lease.LeaseID, lease.WorkerID, disposition, "", outcome, now); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record worker report receipt: %w", ErrWorkerUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("commit worker report: %w", ErrWorkerUnavailable)
	}
	committed = true
	s.forgetLeaseToken(lease.LeaseID)
	return workercontract.ReportResponse{Disposition: disposition, AgentRunID: request.AgentRunID, LeaseState: "consumed"}, nil
}

func executionRunInfo(ctx context.Context, tx *sql.Tx, runID domain.AgentRunID) (ticketID, authorizationID, sessionID, epochID string, execution bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT state.ticket_id, state.authorization_id, authorization.session_id, authorization.epoch_id FROM t_ticket_execution_states state JOIN t_execution_authorizations authorization ON authorization.authorization_id = state.authorization_id WHERE state.agent_run_id = ?`, runID).Scan(&ticketID, &authorizationID, &sessionID, &epochID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", "", false, err
	}
	return ticketID, authorizationID, sessionID, epochID, true, nil
}

type executionSnapshotRecord struct {
	ID            string
	Phase         string
	InputRevision string
	HeadRevision  string
	Branch        string
	ChangedFiles  []string
	DiffSHA256    string
	DiffBytes     int64
	HasUntracked  bool
}

func readExecutionSnapshot(ctx context.Context, tx *sql.Tx, sessionID, ticketID, phase string) (executionSnapshotRecord, bool, error) {
	var record executionSnapshotRecord
	var changedJSON string
	var untracked int
	err := tx.QueryRowContext(ctx, `SELECT snapshot_id, phase, input_revision, head_revision, branch, changed_files_json, diff_sha256, diff_bytes, has_untracked FROM t_workspace_snapshots WHERE session_id = ? AND ticket_id = ? AND phase = ?`, sessionID, ticketID, phase).Scan(&record.ID, &record.Phase, &record.InputRevision, &record.HeadRevision, &record.Branch, &changedJSON, &record.DiffSHA256, &record.DiffBytes, &untracked)
	if errors.Is(err, sql.ErrNoRows) {
		return executionSnapshotRecord{}, false, nil
	}
	if err != nil {
		return executionSnapshotRecord{}, false, err
	}
	if err := json.Unmarshal([]byte(changedJSON), &record.ChangedFiles); err != nil {
		return executionSnapshotRecord{}, false, err
	}
	record.HasUntracked = untracked != 0
	return record, true, nil
}

func preparedArtifact(artifacts []preparedWorkerArtifact, kind string) (preparedWorkerArtifact, bool) {
	for _, artifact := range artifacts {
		if artifact.request.Kind == kind {
			return artifact, true
		}
	}
	return preparedWorkerArtifact{}, false
}

func validateExecutionEvidence(request workercontract.Report, prepared []preparedWorkerArtifact, pre, post executionSnapshotRecord, hasPre, hasPost bool) bool {
	if request.Outcome != workercontract.Outcome(domain.AgentRunOutcomeSucceeded) || request.ExitCode == nil || *request.ExitCode != 0 || request.AfterRevision == "" || request.AfterRevision != post.InputRevision || len(request.GuardFindings) != 0 || reportHasCaptureFailures(request) || !hasPre || !hasPost {
		return false
	}
	if pre.InputRevision != post.InputRevision || pre.HeadRevision != pre.InputRevision || post.HeadRevision != post.InputRevision || pre.Branch == "" || post.Branch != pre.Branch || pre.DiffBytes != 0 || len(pre.ChangedFiles) != 0 || pre.HasUntracked || post.HasUntracked || post.DiffBytes <= 0 || len(post.ChangedFiles) == 0 {
		return false
	}
	diff, hasDiff := preparedArtifact(prepared, "diff")
	changed, hasChanged := preparedArtifact(prepared, "changed_files")
	if !hasDiff || !hasChanged || diff.request.Truncated || changed.request.Truncated || len(diff.content) == 0 || len(changed.content) == 0 || diff.identity.SHA256 != post.DiffSHA256 || diff.identity.ByteLength != post.DiffBytes {
		return false
	}
	workerFiles := strings.Split(strings.TrimSuffix(string(changed.content), "\n"), "\n")
	if len(workerFiles) == 1 && workerFiles[0] == "" {
		return false
	}
	if !sameStringSlice(workerFiles, post.ChangedFiles) {
		return false
	}
	return true
}

func sameStringSlice(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	if len(left) != len(right) {
		return false
	}
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *Store) persistExecutionReport(ctx context.Context, tx *sql.Tx, request workercontract.Report, digest string, prepared []preparedWorkerArtifact, run domain.AgentRun, change domain.Change, lease workerLease, artifacts WorkerArtifactStore, ticketID, authorizationID, sessionID, epochID string) (workercontract.ReportResponse, error) {
	if containsPreparedWorkerArtifact(prepared, "candidate") {
		return workercontract.ReportResponse{}, ErrWorkerReportInvalid
	}
	pre, hasPre, err := readExecutionSnapshot(ctx, tx, sessionID, ticketID, "pre")
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("read execution pre snapshot: %w", ErrWorkerUnavailable)
	}
	post, hasPost, err := readExecutionSnapshot(ctx, tx, sessionID, ticketID, "post")
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("read execution post snapshot: %w", ErrWorkerUnavailable)
	}
	if err := storePreparedReportArtifacts(ctx, prepared, artifacts); err != nil {
		return workercontract.ReportResponse{}, err
	}
	valid := validateExecutionEvidence(request, prepared, pre, post, hasPre, hasPost)
	now := s.now().UTC()
	outcome := domain.AgentRunOutcomeHumanRequired
	role := domain.ArtifactRoleFailure
	if valid {
		outcome = domain.AgentRunOutcomeSucceeded
		role = domain.ArtifactRoleOutput
	}
	refs, runArtifacts, err := insertWorkerArtifacts(ctx, tx, change, prepared, role, now)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("persist execution artifacts: %w", ErrWorkerUnavailable)
	}
	if err := run.Complete(outcome, now); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("complete execution run: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = ?, completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, outcome, stamp(now), run.ID); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("complete execution agent run: %w", ErrWorkerUnavailable)
	}
	if err := insertAgentRunArtifacts(ctx, tx, run.ID, runArtifacts); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("link execution artifacts: %w", ErrWorkerUnavailable)
	}
	diffRef, deltaRef := executionEvidenceRefs(prepared, refs)
	if valid {
		changedJSON, marshalErr := json.Marshal(post.ChangedFiles)
		if marshalErr != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("encode execution delta: %w", ErrWorkerUnavailable)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_ticket_execution_evidence (evidence_id, session_id, epoch_id, ticket_id, agent_run_id, lease_id, input_revision, pre_snapshot_id, post_snapshot_id, complete_diff_artifact_ref_id, ticket_delta_artifact_ref_id, complete_diff_sha256, changed_files_json, outcome, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'succeeded', ?)`, id.New(), sessionID, epochID, ticketID, run.ID, lease.LeaseID, post.InputRevision, pre.ID, post.ID, diffRef, deltaRef, post.DiffSHA256, string(changedJSON), stamp(now)); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("persist execution evidence: %w", ErrWorkerUnavailable)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_ticket_execution_states SET state = 'succeeded', updated_at = ? WHERE graph_id = (SELECT graph_id FROM t_tickets WHERE ticket_id = ?) AND ticket_id = ? AND agent_run_id = ?`, stamp(now), ticketID, ticketID, run.ID); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("complete execution ticket: %w", ErrWorkerUnavailable)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_execution_authorizations SET status = 'completed', updated_at = ? WHERE authorization_id = ?`, stamp(now), authorizationID); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("complete execution authorization: %w", ErrWorkerUnavailable)
		}
	} else {
		if hasPre && hasPost {
			changedJSON, marshalErr := json.Marshal(post.ChangedFiles)
			if marshalErr != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("encode fenced execution delta: %w", ErrWorkerUnavailable)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO t_ticket_execution_evidence (evidence_id, session_id, epoch_id, ticket_id, agent_run_id, lease_id, input_revision, pre_snapshot_id, post_snapshot_id, complete_diff_artifact_ref_id, ticket_delta_artifact_ref_id, complete_diff_sha256, changed_files_json, outcome, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'human_required', ?)`, id.New(), sessionID, epochID, ticketID, run.ID, lease.LeaseID, post.InputRevision, pre.ID, post.ID, diffRef, deltaRef, post.DiffSHA256, string(changedJSON), stamp(now)); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("persist fenced execution evidence: %w", ErrWorkerUnavailable)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_ticket_execution_states SET state = 'human_required', updated_at = ? WHERE graph_id = (SELECT graph_id FROM t_tickets WHERE ticket_id = ?) AND ticket_id = ? AND agent_run_id = ?`, stamp(now), ticketID, ticketID, run.ID); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("fence execution ticket: %w", ErrWorkerUnavailable)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_execution_authorizations SET status = 'human_required', updated_at = ? WHERE authorization_id = ?`, stamp(now), authorizationID); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("fence execution authorization: %w", ErrWorkerUnavailable)
		}
	}
	actor := "worker:" + request.AgentRunID
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, actor, now, &run.ID, nil, refs); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record execution completion: %w", ErrWorkerUnavailable)
	}
	disposition := "accepted"
	if !valid {
		disposition = "accepted_fenced"
		if change.Status == domain.ChangeStatusActive {
			next, transitionErr := change.EnterHumanRequired()
			if transitionErr != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("fence execution change: %w", ErrWorkerUnavailable)
			}
			next.UpdatedAt = now
			if err := updateChangeStatus(ctx, tx, change, next, now); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("update execution recovery boundary: %w", ErrWorkerUnavailable)
			}
			if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.ChangeHumanRequiredType, actor, now, &run.ID, nil, refs); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("record execution recovery boundary: %w", ErrWorkerUnavailable)
			}
		}
	} else {
		var pending int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_ticket_execution_states state JOIN t_tickets ticket ON ticket.ticket_id = state.ticket_id JOIN t_ticket_graphs graph ON graph.graph_id = ticket.graph_id WHERE graph.change_id = ? AND state.state IN ('pending', 'assigned')`, change.ID).Scan(&pending); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("read remaining execution tickets: %w", ErrWorkerUnavailable)
		}
		if pending == 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET status = 'completed', updated_at = ? WHERE session_id = ?`, stamp(now), sessionID); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("complete execution session: %w", ErrWorkerUnavailable)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE t_execution_dispatch_epochs SET status = 'completed' WHERE epoch_id = ?`, epochID); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("complete execution epoch: %w", ErrWorkerUnavailable)
			}
		} else {
			newEpoch := id.New()
			if _, err := tx.ExecContext(ctx, `INSERT INTO t_execution_dispatch_epochs (epoch_id, session_id, sequence, status, created_at) SELECT ?, session_id, sequence + 1, 'queued', ? FROM t_execution_dispatch_epochs WHERE epoch_id = ?`, newEpoch, stamp(now), epochID); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("queue next execution epoch: %w", ErrWorkerUnavailable)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE t_execution_dispatch_epochs SET status = 'completed' WHERE epoch_id = ?`, epochID); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("complete execution epoch: %w", ErrWorkerUnavailable)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE t_execution_sessions SET status = 'waiting', current_epoch_id = ?, updated_at = ? WHERE session_id = ?`, newEpoch, stamp(now), sessionID); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("queue execution session: %w", ErrWorkerUnavailable)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'consumed', consumed_at = ?, report_digest = ?, report_disposition = ?, report_outcome = ? WHERE lease_id = ? AND state = 'active'`, stamp(now), digest, disposition, outcome, lease.LeaseID); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("consume execution lease: %w", ErrWorkerUnavailable)
	}
	if err := insertWorkerReportReceipt(ctx, tx, digest, request.AgentRunID, lease.LeaseID, lease.WorkerID, disposition, "", outcome, now); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record execution report receipt: %w", ErrWorkerUnavailable)
	}
	if err := tx.Commit(); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("commit execution report: %w", ErrWorkerUnavailable)
	}
	s.forgetLeaseToken(lease.LeaseID)
	return workercontract.ReportResponse{Disposition: disposition, AgentRunID: request.AgentRunID, LeaseState: "consumed"}, nil
}

func executionEvidenceRefs(prepared []preparedWorkerArtifact, refs []domain.ArtifactRefID) (string, string) {
	var diffOrdinal, changedOrdinal = -1, -1
	for index, item := range prepared {
		switch item.request.Kind {
		case "diff":
			diffOrdinal = index
		case "changed_files":
			changedOrdinal = index
		}
	}
	var diffRef, changedRef string
	if diffOrdinal >= 0 && diffOrdinal < len(refs) {
		diffRef = string(refs[diffOrdinal])
	}
	if changedOrdinal >= 0 && changedOrdinal < len(refs) {
		changedRef = string(refs[changedOrdinal])
	}
	return diffRef, changedRef
}

// ReconcilePlanningExecutions 只把超时 Planning Lease 标记为失效，不据此推断 Runtime 已停止。
// Worker 的 late Report、Supervisor 的进程退出或 Daemon restart 才能形成可收敛候选。
func (s *Store) ReconcilePlanningExecutions(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin planning execution reconciliation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired', workspace_path = '' WHERE result_mode = 'planning_candidate' AND state = 'active' AND expires_at <= ?`, stamp(now)); err != nil {
		return fmt.Errorf("expire planning leases: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit planning execution reconciliation: %w", err)
	}
	committed = true
	if err := s.forgetTerminalLeaseTokens(ctx); err != nil {
		return err
	}
	return nil
}

// ReconcileWorkerRestart 撤销旧 Lease；Planning run 先形成失败候选，其余 run 直接收敛。
func (s *Store) ReconcileWorkerRestart(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin worker restart reconciliation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_instances SET status = 'revoked' WHERE status IN ('pending', 'registered')`); err != nil {
		return fmt.Errorf("revoke worker instances: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'revoked', workspace_path = CASE WHEN result_mode = 'planning_candidate' THEN '' ELSE workspace_path END WHERE state = 'active'`); err != nil {
		return fmt.Errorf("revoke worker leases: %w", err)
	}
	planningRunIDs, err := runningPlanningRunIDs(ctx, tx, "", true)
	if err != nil {
		return err
	}
	for _, runID := range planningRunIDs {
		if err := insertPlanningSystemCandidate(ctx, tx, runID, "daemon_restarted", now); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT run.agent_run_id FROM t_agent_runs run WHERE run.status = 'running' AND run.run_kind <> 'planning' ORDER BY run.started_at, run.agent_run_id`)
	if err != nil {
		return fmt.Errorf("list running worker runs: %w", err)
	}
	var runIDs []domain.AgentRunID
	for rows.Next() {
		var runID domain.AgentRunID
		if err := rows.Scan(&runID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan running worker run: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close running worker runs: %w", err)
	}
	for _, runID := range runIDs {
		run, err := readAgentRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		change, err := readChange(ctx, tx, run.ChangeID)
		if err != nil {
			return err
		}
		if err := run.Complete(domain.AgentRunOutcomeFailed, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = 'failed', completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, stamp(now), run.ID); err != nil {
			return fmt.Errorf("complete restarted worker run: %w", err)
		}
		actor := "daemon_restarted"
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit worker restart reconciliation: %w", err)
	}
	committed = true
	if err := s.forgetTerminalLeaseTokens(ctx); err != nil {
		return err
	}
	return nil
}

// ReconcileWorkerLost 撤销单个崩溃 Worker 的 Lease，并按 run kind 收敛其 running AgentRun。
func (s *Store) ReconcileWorkerLost(ctx context.Context, workerID string) error {
	if strings.TrimSpace(workerID) == "" {
		return ErrWorkerNotRegistered
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin worker lost reconciliation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_instances SET status = 'revoked' WHERE worker_id = ? AND status IN ('pending', 'registered')`, workerID); err != nil {
		return fmt.Errorf("revoke lost worker: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'revoked', workspace_path = CASE WHEN result_mode = 'planning_candidate' THEN '' ELSE workspace_path END WHERE worker_id = ? AND state = 'active'`, workerID); err != nil {
		return fmt.Errorf("revoke lost worker leases: %w", err)
	}
	planningRunIDs, err := runningPlanningRunIDs(ctx, tx, workerID, false)
	if err != nil {
		return err
	}
	for _, runID := range planningRunIDs {
		if err := insertPlanningSystemCandidate(ctx, tx, runID, "worker_lost", now); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT lease.agent_run_id FROM t_worker_leases lease JOIN t_agent_runs run ON run.agent_run_id = lease.agent_run_id WHERE lease.worker_id = ? AND lease.state IN ('expired', 'revoked') AND run.status = 'running' AND run.run_kind <> 'planning'`, workerID)
	if err != nil {
		return fmt.Errorf("list lost worker runs: %w", err)
	}
	var runIDs []domain.AgentRunID
	for rows.Next() {
		var runID domain.AgentRunID
		if err := rows.Scan(&runID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan lost worker run: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close lost worker runs: %w", err)
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
		if err := run.Complete(domain.AgentRunOutcomeFailed, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = 'failed', completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, stamp(now), run.ID); err != nil {
			return fmt.Errorf("complete lost worker run: %w", err)
		}
		actor := "worker_lost"
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit worker lost reconciliation: %w", err)
	}
	committed = true
	if err := s.forgetTerminalLeaseTokens(ctx); err != nil {
		return err
	}
	return nil
}

func runningPlanningRunIDs(ctx context.Context, tx *sql.Tx, workerID string, includeUnassigned bool) ([]domain.AgentRunID, error) {
	query := `
SELECT run.agent_run_id
FROM t_agent_runs run
JOIN t_worker_leases lease ON lease.agent_run_id = run.agent_run_id
WHERE run.run_kind = 'planning'
	  AND run.status = 'running'
	  AND lease.result_mode = 'planning_candidate'
	  AND lease.state IN ('expired', 'revoked')
	  AND lease.report_digest IS NULL
	  AND NOT EXISTS (
      SELECT 1 FROM t_planning_run_candidates candidate
      WHERE candidate.agent_run_id = run.agent_run_id
  )
ORDER BY run.started_at, run.agent_run_id`
	var rows *sql.Rows
	var err error
	if includeUnassigned {
		rows, err = tx.QueryContext(ctx, query)
	} else {
		query = `
SELECT run.agent_run_id
FROM t_agent_runs run
JOIN t_worker_leases lease ON lease.agent_run_id = run.agent_run_id
WHERE run.run_kind = 'planning'
  AND run.status = 'running'
  AND lease.worker_id = ?
  AND lease.state IN ('expired', 'revoked')
  AND lease.report_digest IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM t_planning_run_candidates candidate
      WHERE candidate.agent_run_id = run.agent_run_id
  )
ORDER BY run.started_at, run.agent_run_id`
		rows, err = tx.QueryContext(ctx, query, workerID)
	}
	if err != nil {
		return nil, fmt.Errorf("list running planning executions: %w", err)
	}
	defer rows.Close()
	var runIDs []domain.AgentRunID
	for rows.Next() {
		var runID domain.AgentRunID
		if err := rows.Scan(&runID); err != nil {
			return nil, fmt.Errorf("scan running planning execution: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read running planning executions: %w", err)
	}
	return runIDs, nil
}

func insertPlanningSystemCandidate(ctx context.Context, tx *sql.Tx, runID domain.AgentRunID, reason string, now time.Time) error {
	if !validPlanningFailureReason(reason) {
		return fmt.Errorf("planning execution failure reason is invalid: %w", domain.ErrInvalidRequest)
	}
	digestInput := "planning-system-candidate:v1:" + reason + ":" + string(runID)
	digest := sha256.Sum256([]byte(digestInput))
	result, err := tx.ExecContext(ctx, `
INSERT INTO t_planning_run_candidates (
    agent_run_id, report_digest, outcome, exit_code, after_revision,
    failure_reason, guard_findings_json, candidate_truncated, received_at
)
SELECT run.agent_run_id, ?, 'failed', NULL, run.source_revision, ?, '[]', 0, ?
FROM t_agent_runs run
WHERE run.agent_run_id = ?
  AND run.run_kind = 'planning'
  AND run.status = 'running'
  AND NOT EXISTS (
      SELECT 1 FROM t_planning_run_candidates candidate
      WHERE candidate.agent_run_id = run.agent_run_id
  )`, hex.EncodeToString(digest[:]), reason, stamp(now), runID)
	if err != nil {
		return fmt.Errorf("record planning execution failure: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record planning execution failure: %w", err)
	}
	if count == 1 {
		return nil
	}
	exists, err := planningCandidateExists(ctx, tx, runID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return domain.ErrPlanningRunConflict
}

func validPlanningFailureReason(reason string) bool {
	switch reason {
	case "agent_run_mismatch", "artifact_unavailable", "authority_fenced", "capture_failed",
		"change_cancelled", "coordination_failed", "daemon_restarted", "decode_invalid",
		"dispatch_failed", "guard_violation", "input_invalid", "lease_expired", "lease_revoked",
		"limit_exceeded", "revision_mismatch", "runtime_failed", "runtime_timeout",
		"schema_invalid", "snapshot_failed", "worker_lost":
		return true
	default:
		return false
	}
}

type workerLease struct {
	LeaseID       string
	AgentRunID    string
	WorkerID      string
	Attempt       int
	TokenSHA256   string
	State         string
	ExpiresAt     string
	ReportDigest  string
	ReportOutcome string
	ResultMode    string
}

func readWorkerLease(ctx context.Context, queryer sqlQueryer, runID string) (workerLease, error) {
	var lease workerLease
	err := queryer.QueryRowContext(ctx, `SELECT lease_id, agent_run_id, worker_id, attempt, token_sha256, state, expires_at, COALESCE(report_digest, ''), COALESCE(report_outcome, ''), result_mode FROM t_worker_leases WHERE agent_run_id = ?`, runID).Scan(&lease.LeaseID, &lease.AgentRunID, &lease.WorkerID, &lease.Attempt, &lease.TokenSHA256, &lease.State, &lease.ExpiresAt, &lease.ReportDigest, &lease.ReportOutcome, &lease.ResultMode)
	if errors.Is(err, sqlErrNoRows()) {
		return workerLease{}, ErrWorkerLeaseInvalid
	}
	if err != nil {
		return workerLease{}, err
	}
	return lease, nil
}

type preparedWorkerArtifact struct {
	request  workercontract.Artifact
	content  []byte
	identity domain.ArtifactIdentity
}

func prepareReportArtifacts(request workercontract.Report) ([]preparedWorkerArtifact, error) {
	prepared := make([]preparedWorkerArtifact, 0, len(request.Artifacts))
	total := 0
	seen := make(map[string]struct{}, len(request.Artifacts))
	for _, item := range request.Artifacts {
		limit := maxWorkerArtifactBytes
		if item.Kind == "changed_files" {
			limit = maxChangedFilesBytes
		} else if item.Kind == "candidate" {
			limit = maxPlanningCandidateBytes
		}
		if _, ok := seen[item.Kind]; ok || !validWorkerArtifactKind(item.Kind) {
			return nil, fmt.Errorf("worker artifact kind is invalid: %w", ErrWorkerReportInvalid)
		}
		seen[item.Kind] = struct{}{}
		if item.CaptureError != "" {
			if len(item.CaptureError) > 8<<10 || !utf8.ValidString(item.CaptureError) {
				return nil, fmt.Errorf("worker artifact capture error is invalid: %w", ErrWorkerReportInvalid)
			}
			continue
		}
		content, err := base64.StdEncoding.DecodeString(item.ContentBase64)
		if err != nil || int64(len(content)) != item.SizeBytes || item.SizeBytes < 0 || len(content) > limit {
			return nil, fmt.Errorf("worker artifact size or encoding is invalid: %w", ErrWorkerReportInvalid)
		}
		identity := domain.NewArtifactIdentity(content)
		if identity.SHA256 != item.SHA256 {
			return nil, fmt.Errorf("worker artifact digest is invalid: %w", ErrWorkerReportInvalid)
		}
		if item.Kind == "changed_files" {
			if err := validateChangedFiles(content); err != nil {
				return nil, fmt.Errorf("worker changed files are invalid: %w", ErrWorkerReportInvalid)
			}
		}
		total += len(content)
		if total > maxWorkerReportBytes {
			return nil, fmt.Errorf("worker report exceeds total artifact limit: %w", ErrWorkerReportInvalid)
		}
		prepared = append(prepared, preparedWorkerArtifact{request: item, content: content, identity: identity})
	}
	return prepared, nil
}

func reportHasCaptureFailures(request workercontract.Report) bool {
	if len(request.CaptureFailures) != 0 {
		return true
	}
	for _, artifact := range request.Artifacts {
		if artifact.CaptureError != "" {
			return true
		}
	}
	return false
}

func storePreparedReportArtifacts(ctx context.Context, prepared []preparedWorkerArtifact, store WorkerArtifactStore) error {
	if len(prepared) == 0 {
		return nil
	}
	if store == nil {
		return fmt.Errorf("worker artifact store is unavailable: %w", ErrWorkerUnavailable)
	}
	for _, item := range prepared {
		stored, err := store.Put(ctx, item.content)
		if err != nil || stored != item.identity {
			return fmt.Errorf("store worker artifact: %w", ErrWorkerUnavailable)
		}
	}
	return nil
}

func validateChangedFiles(content []byte) error {
	if len(content) == 0 {
		return nil
	}
	text := strings.TrimSuffix(string(content), "\n")
	paths := strings.Split(text, "\n")
	if !sort.StringsAreSorted(paths) {
		return errors.New("changed files are not sorted")
	}
	for index, filePath := range paths {
		if filePath == "" || strings.ContainsRune(filePath, '\x00') || strings.Contains(filePath, "\\") || strings.Contains(filePath, ":") || pathpkg.IsAbs(filePath) || filepath.IsAbs(filePath) || filepath.Clean(filePath) != filePath || pathpkg.Clean(filePath) != filePath || filePath == "." || filePath == ".." || strings.HasPrefix(filePath, "../") || strings.Contains(filePath, "/../") {
			return errors.New("changed files contain a non-workspace-relative path")
		}
		for _, component := range strings.Split(filePath, "/") {
			if component == "" || component == "." || component == ".." {
				return errors.New("changed files contain a non-workspace-relative path")
			}
		}
		if index > 0 && paths[index-1] == filePath {
			return errors.New("changed files contain duplicates")
		}
	}
	return nil
}

func validWorkerArtifactKind(kind string) bool {
	switch kind {
	case "stdout", "stderr", "diff", "changed_files", "candidate":
		return true
	default:
		return false
	}
}

func containsPreparedWorkerArtifact(artifacts []preparedWorkerArtifact, kind string) bool {
	for _, artifact := range artifacts {
		if artifact.request.Kind == kind {
			return true
		}
	}
	return false
}

func containsWorkerCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if capability == required || capability == strings.TrimPrefix(required, "runtime:") {
			return true
		}
	}
	return false
}

func validateArtifactSummary(summary workercontract.ArtifactSummary) error {
	identity := domain.ArtifactIdentity{SHA256: summary.SHA256, ByteLength: summary.SizeBytes}
	if err := identity.Validate(); err != nil || summary.SizeBytes > maxWorkerArtifactBytes {
		return fmt.Errorf("worker input artifact is invalid: %w", domain.ErrInvalidRequest)
	}
	return nil
}

func insertWorkerArtifacts(ctx context.Context, tx *sql.Tx, change domain.Change, prepared []preparedWorkerArtifact, role string, now time.Time) ([]domain.ArtifactRefID, []domain.AgentRunArtifact, error) {
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(ordinal), -1) + 1 FROM t_artifact_refs WHERE change_id = ? AND role = ?`, change.ID, role).Scan(&next); err != nil {
		return nil, nil, err
	}
	refs := make([]domain.ArtifactRefID, 0, len(prepared))
	runArtifacts := make([]domain.AgentRunArtifact, 0, len(prepared))
	for _, item := range prepared {
		stored, err := ensureArtifact(ctx, tx, item.identity, mediaTypeForWorkerArtifact(item.request.Kind), now)
		if err != nil {
			return nil, nil, err
		}
		ref := domain.ArtifactRef{ID: domain.ArtifactRefID(id.New()), ChangeID: change.ID, ArtifactID: stored.ID, Role: role, Ordinal: next}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifact_refs (artifact_ref_id, project_id, change_id, artifact_id, role, ordinal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, ref.ID, change.ProjectID, change.ID, stored.ID, role, next, stamp(now)); err != nil {
			return nil, nil, err
		}
		refs = append(refs, ref.ID)
		runArtifacts = append(runArtifacts, domain.AgentRunArtifact{ArtifactRefID: ref.ID, Role: role, Ordinal: next})
		next++
	}
	return refs, runArtifacts, nil
}

func mediaTypeForWorkerArtifact(kind string) string {
	if kind == "candidate" {
		return "application/json; charset=utf-8"
	}
	if kind == "diff" {
		return "text/x-diff; charset=utf-8"
	}
	if kind == "changed_files" {
		return "text/plain; charset=utf-8"
	}
	return "text/plain; charset=utf-8"
}

func (s *Store) persistLateReport(ctx context.Context, tx *sql.Tx, request workercontract.Report, digest string, prepared []preparedWorkerArtifact, run domain.AgentRun, change domain.Change, lease workerLease, reason string) (workercontract.ReportResponse, error) {
	now := s.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET consumed_at = ?, report_digest = ?, report_disposition = 'late', report_outcome = ?, workspace_path = '' WHERE lease_id = ? AND state IN ('expired', 'revoked') AND report_digest IS NULL`, stamp(now), digest, reportOutcome(request), lease.LeaseID)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("claim late worker report: %w", ErrWorkerUnavailable)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return workercontract.ReportResponse{}, fmt.Errorf("claim late worker report: %w", ErrWorkerUnavailable)
	}
	role := domain.ArtifactRoleOutput
	if reportOutcome(request) != domain.AgentRunOutcomeSucceeded {
		role = domain.ArtifactRoleFailure
	}
	refs, _, err := insertLateWorkerArtifacts(ctx, tx, change, prepared, role, now)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("persist late worker artifacts: %w", ErrWorkerUnavailable)
	}
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunReportLateType, "worker_late", now, &run.ID, nil, refs); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record late worker report: %w", ErrWorkerUnavailable)
	}
	if err := insertWorkerReportReceipt(ctx, tx, digest, request.AgentRunID, lease.LeaseID, lease.WorkerID, "late", reason, reportOutcome(request), now); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record late worker report receipt: %w", ErrWorkerUnavailable)
	}
	return workercontract.ReportResponse{Disposition: "late", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
}

func insertLateWorkerArtifacts(ctx context.Context, tx *sql.Tx, change domain.Change, prepared []preparedWorkerArtifact, role string, now time.Time) ([]domain.ArtifactRefID, []domain.AgentRunArtifact, error) {
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(ordinal), -1) + 1 FROM t_artifact_refs WHERE change_id = ? AND role = ?`, change.ID, role).Scan(&next); err != nil {
		return nil, nil, err
	}
	refs := make([]domain.ArtifactRefID, 0, len(prepared))
	runArtifacts := make([]domain.AgentRunArtifact, 0, len(prepared))
	for _, item := range prepared {
		stored, err := ensureArtifact(ctx, tx, item.identity, mediaTypeForWorkerArtifact(item.request.Kind), now)
		if err != nil {
			return nil, nil, err
		}
		ref := domain.ArtifactRef{ID: domain.ArtifactRefID(id.New()), ChangeID: change.ID, ArtifactID: stored.ID, Role: role, Ordinal: next}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifact_refs (artifact_ref_id, project_id, change_id, artifact_id, role, ordinal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, ref.ID, change.ProjectID, change.ID, stored.ID, role, next, stamp(now)); err != nil {
			return nil, nil, err
		}
		next++
		refs = append(refs, ref.ID)
		runArtifacts = append(runArtifacts, domain.AgentRunArtifact{ArtifactRefID: ref.ID, Role: role, Ordinal: next - 1})
	}
	return refs, runArtifacts, nil
}

func insertWorkerReportReceipt(ctx context.Context, tx *sql.Tx, digest, runID, leaseID, workerID, disposition, reason, outcome string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO t_worker_reports (report_id, report_digest, agent_run_id, lease_id, worker_id, disposition, reason, outcome, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id.New(), digest, runID, leaseID, workerID, disposition, reason, outcome, stamp(now))
	return err
}

func digestReport(report workercontract.Report) (string, error) {
	encoded, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func reportOutcome(report workercontract.Report) string {
	if report.Outcome == workercontract.Outcome(domain.AgentRunOutcomeFailed) ||
		strings.TrimSpace(report.FailureReason) != "" || len(report.GuardFindings) != 0 || reportHasCaptureFailures(report) {
		return domain.AgentRunOutcomeFailed
	}
	if report.ExitCode != nil && *report.ExitCode != 0 {
		return domain.AgentRunOutcomeFailed
	}
	if report.Outcome == workercontract.Outcome(domain.AgentRunOutcomeSucceeded) {
		return domain.AgentRunOutcomeSucceeded
	}
	return domain.AgentRunOutcomeFailed
}

func hashSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func (s *Store) forgetLeaseToken(leaseID string) {
	s.leaseMu.Lock()
	delete(s.leaseTokens, leaseID)
	s.leaseMu.Unlock()
}

func (s *Store) forgetTerminalLeaseTokens(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT lease_id FROM t_worker_leases WHERE state <> 'active'`)
	if err != nil {
		return fmt.Errorf("list terminal worker lease secrets: %w", ErrWorkerUnavailable)
	}
	defer rows.Close()
	var leaseIDs []string
	for rows.Next() {
		var leaseID string
		if err := rows.Scan(&leaseID); err != nil {
			return fmt.Errorf("scan terminal worker lease secret: %w", ErrWorkerUnavailable)
		}
		leaseIDs = append(leaseIDs, leaseID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read terminal worker lease secrets: %w", ErrWorkerUnavailable)
	}
	s.leaseMu.Lock()
	for _, leaseID := range leaseIDs {
		delete(s.leaseTokens, leaseID)
	}
	s.leaseMu.Unlock()
	return nil
}

// sqlErrNoRows 隔离 database/sql 的查询缺失哨兵，避免 DTO 层感知 SQL 细节。
func sqlErrNoRows() error { return sql.ErrNoRows }

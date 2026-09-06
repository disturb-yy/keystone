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
	"path/filepath"
	"sort"
	"strings"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	workerHeartbeatInterval = 5 * time.Second
	workerLeaseTTL          = 30 * time.Second
	maxWorkerArtifactBytes  = 16 << 20
	maxChangedFilesBytes    = 1 << 20
	maxWorkerReportBytes    = 64 << 20
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

// HeartbeatWorker 只为当前 Worker 的匹配 active Lease 续租。
func (s *Store) HeartbeatWorker(ctx context.Context, request workercontract.Heartbeat) (workercontract.HeartbeatResponse, error) {
	if err := request.Validate(); err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("heartbeat worker: %w", ErrWorkerNotRegistered)
	}
	now := s.now().UTC()
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
	response := workercontract.HeartbeatResponse{WorkerID: request.WorkerID, WorkerAvailable: true}
	if request.AgentRunID == "" {
		if _, err := s.db.ExecContext(ctx, `UPDATE t_worker_instances SET last_heartbeat_at = ? WHERE worker_id = ?`, stamp(now), request.WorkerID); err != nil {
			return workercontract.HeartbeatResponse{}, fmt.Errorf("record worker heartbeat: %w", ErrWorkerUnavailable)
		}
		return response, nil
	}
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
		_, _ = s.db.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired' WHERE lease_id = ? AND state = 'active'`, leaseID)
		return response, nil
	}
	newDeadline := now.Add(workerLeaseTTL)
	if _, err := s.db.ExecContext(ctx, `UPDATE t_worker_leases SET expires_at = ? WHERE lease_id = ? AND state = 'active'`, stamp(newDeadline), leaseID); err != nil {
		return workercontract.HeartbeatResponse{}, fmt.Errorf("renew worker lease: %w", ErrWorkerUnavailable)
	}
	response.LeaseRenewed = true
	response.LeaseExpiresAt = stamp(newDeadline)
	return response, nil
}

// IssueAssignment 为已创建的 running AgentRun 发放一个不可复用 Lease。
func (s *Store) IssueAssignment(ctx context.Context, runID domain.AgentRunID, workerID, workspacePath, runtime, instruction, beforeRevision, workspaceID string, inputs []workercontract.ArtifactSummary) (workercontract.Assignment, error) {
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
	if err := tx.QueryRowContext(ctx, `SELECT status, capabilities_json FROM t_worker_instances WHERE worker_id = ?`, workerID).Scan(&workerStatus, &capabilitiesJSON); err != nil {
		if errors.Is(err, sqlErrNoRows()) {
			return workercontract.Assignment{}, ErrWorkerNotRegistered
		}
		return workercontract.Assignment{}, fmt.Errorf("read assignment worker: %w", err)
	}
	if workerStatus != "registered" {
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_worker_leases (lease_id, agent_run_id, worker_id, attempt, token_sha256, state, expires_at, workspace_id, workspace_path, runtime, instruction, before_revision, input_artifacts_json, created_at) VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?)`, leaseID, run.ID, workerID, run.Attempt, hashSecret(token), stamp(expires), workspaceID, workspacePath, runtime, instruction, beforeRevision, string(inputJSON), stamp(now)); err != nil {
		return workercontract.Assignment{}, fmt.Errorf("persist worker lease: %w", err)
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
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired' WHERE worker_id = ? AND state = 'active' AND expires_at <= ?`, workerID, stamp(now)); err != nil {
		return nil, fmt.Errorf("expire worker leases: %w", ErrWorkerUnavailable)
	}
	var leaseID, agentRunID, state, expires, workspaceID, workspacePath, runtime, instruction, beforeRevision, inputJSON string
	var attempt int
	err := s.db.QueryRowContext(ctx, `SELECT lease_id, agent_run_id, state, expires_at, workspace_id, workspace_path, runtime, instruction, before_revision, attempt, input_artifacts_json FROM t_worker_leases WHERE worker_id = ? AND state = 'active' ORDER BY created_at, lease_id LIMIT 1`, workerID).Scan(&leaseID, &agentRunID, &state, &expires, &workspaceID, &workspacePath, &runtime, &instruction, &beforeRevision, &attempt, &inputJSON)
	if errors.Is(err, sqlErrNoRows()) {
		return nil, nil
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
		Instruction:    instruction,
		BeforeRevision: beforeRevision,
		Attempt:        attempt,
		InputArtifacts: inputs,
	}, nil
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
		if disposition == "accepted" || disposition == "accepted_fenced" {
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
	if lease.State == "consumed" {
		if lease.ReportDigest == digest {
			if err := tx.Commit(); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("commit duplicate worker report: %w", ErrWorkerUnavailable)
			}
			committed = true
			return workercontract.ReportResponse{Disposition: "duplicate", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
		}
		return workercontract.ReportResponse{Disposition: "terminal_conflict", ErrorCode: "terminal_conflict", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
	}
	if run.Status != domain.AgentRunStatusRunning {
		return workercontract.ReportResponse{Disposition: "terminal_conflict", ErrorCode: "terminal_conflict", AgentRunID: request.AgentRunID, LeaseState: lease.State}, nil
	}
	now := s.now().UTC()
	deadline, deadlineErr := parseStamp(lease.ExpiresAt)
	attemptMatches := request.Attempt == 0 || request.Attempt == run.Attempt
	current := lease.State == "active" && deadlineErr == nil && now.Before(deadline) && lease.Attempt == run.Attempt && attemptMatches
	if current {
		currentRun, currentErr := isCurrentAgentRun(ctx, tx, change, run)
		if currentErr != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("check current worker run: %w", ErrWorkerUnavailable)
		}
		if !currentRun {
			current = false
		}
	}
	if !current {
		if lease.State == "active" && (deadlineErr != nil || !now.Before(deadline)) {
			if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'expired' WHERE lease_id = ? AND state = 'active'`, lease.LeaseID); err != nil {
				return workercontract.ReportResponse{}, fmt.Errorf("expire worker lease: %w", ErrWorkerUnavailable)
			}
			lease.State = "expired"
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
		return response, nil
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
	return workercontract.ReportResponse{Disposition: disposition, AgentRunID: request.AgentRunID, LeaseState: "consumed"}, nil
}

// ReconcileWorkerRestart 撤销旧 Lease，并将仍 running 的 AgentRun 收敛为 daemon_restarted。
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
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'revoked' WHERE state = 'active'`); err != nil {
		return fmt.Errorf("revoke worker leases: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT agent_run_id FROM t_agent_runs WHERE status = 'running' ORDER BY started_at, agent_run_id`)
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
	return nil
}

// ReconcileWorkerLost 撤销单个崩溃 Worker 的 Lease，并固定其仍 running 的 AgentRun。
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
	if _, err := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'revoked' WHERE worker_id = ? AND state = 'active'`, workerID); err != nil {
		return fmt.Errorf("revoke lost worker leases: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT agent_run_id FROM t_worker_leases WHERE worker_id = ? AND state = 'revoked' AND report_digest IS NULL`, workerID)
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
	return nil
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
}

func readWorkerLease(ctx context.Context, queryer sqlQueryer, runID string) (workerLease, error) {
	var lease workerLease
	err := queryer.QueryRowContext(ctx, `SELECT lease_id, agent_run_id, worker_id, attempt, token_sha256, state, expires_at, COALESCE(report_digest, ''), COALESCE(report_outcome, '') FROM t_worker_leases WHERE agent_run_id = ?`, runID).Scan(&lease.LeaseID, &lease.AgentRunID, &lease.WorkerID, &lease.Attempt, &lease.TokenSHA256, &lease.State, &lease.ExpiresAt, &lease.ReportDigest, &lease.ReportOutcome)
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
	if len(request.CaptureFailures) > 0 {
		return nil, fmt.Errorf("worker evidence capture failed: %w", ErrWorkerUnavailable)
	}
	prepared := make([]preparedWorkerArtifact, 0, len(request.Artifacts))
	total := 0
	seen := make(map[string]struct{}, len(request.Artifacts))
	for _, item := range request.Artifacts {
		limit := maxWorkerArtifactBytes
		if item.Kind == "changed_files" {
			limit = maxChangedFilesBytes
		}
		if _, ok := seen[item.Kind]; ok || !validWorkerArtifactKind(item.Kind) {
			return nil, fmt.Errorf("worker artifact kind is invalid: %w", ErrWorkerUnavailable)
		}
		seen[item.Kind] = struct{}{}
		if item.CaptureError != "" {
			return nil, fmt.Errorf("worker artifact capture failed: %w", ErrWorkerUnavailable)
		}
		content, err := base64.StdEncoding.DecodeString(item.ContentBase64)
		if err != nil || int64(len(content)) != item.SizeBytes || item.SizeBytes < 0 || len(content) > limit {
			return nil, fmt.Errorf("worker artifact size or encoding is invalid: %w", ErrWorkerUnavailable)
		}
		identity := domain.NewArtifactIdentity(content)
		if identity.SHA256 != item.SHA256 {
			return nil, fmt.Errorf("worker artifact digest is invalid: %w", ErrWorkerUnavailable)
		}
		if item.Kind == "changed_files" {
			if err := validateChangedFiles(content); err != nil {
				return nil, fmt.Errorf("worker changed files are invalid: %w", ErrWorkerUnavailable)
			}
		}
		total += len(content)
		if total > maxWorkerReportBytes {
			return nil, fmt.Errorf("worker report exceeds total artifact limit: %w", ErrWorkerUnavailable)
		}
		prepared = append(prepared, preparedWorkerArtifact{request: item, content: content, identity: identity})
	}
	return prepared, nil
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
	for index, path := range paths {
		if path == "" || strings.ContainsRune(path, '\x00') || strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || strings.HasPrefix(path, "../") || path == ".." {
			return errors.New("changed files contain a non-workspace-relative path")
		}
		if index > 0 && paths[index-1] == path {
			return errors.New("changed files contain duplicates")
		}
	}
	return nil
}

func validWorkerArtifactKind(kind string) bool {
	switch kind {
	case "stdout", "stderr", "diff", "changed_files":
		return true
	default:
		return false
	}
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
	refs := make([]domain.ArtifactRefID, 0, len(prepared))
	runArtifacts := make([]domain.AgentRunArtifact, 0, len(prepared))
	for ordinal, item := range prepared {
		stored, err := ensureArtifact(ctx, tx, item.identity, mediaTypeForWorkerArtifact(item.request.Kind), now)
		if err != nil {
			return nil, nil, err
		}
		ref := domain.ArtifactRef{ID: domain.ArtifactRefID(id.New()), ChangeID: change.ID, ArtifactID: stored.ID, Role: role, Ordinal: ordinal}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifact_refs (artifact_ref_id, project_id, change_id, artifact_id, role, ordinal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, ref.ID, change.ProjectID, change.ID, stored.ID, role, ordinal, stamp(now)); err != nil {
			return nil, nil, err
		}
		refs = append(refs, ref.ID)
		runArtifacts = append(runArtifacts, domain.AgentRunArtifact{ArtifactRefID: ref.ID, Role: role, Ordinal: ordinal})
	}
	return refs, runArtifacts, nil
}

func mediaTypeForWorkerArtifact(kind string) string {
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
	if report.ExitCode != nil {
		if *report.ExitCode == 0 {
			return domain.AgentRunOutcomeSucceeded
		}
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

// sqlErrNoRows 隔离 database/sql 的查询缺失哨兵，避免 DTO 层感知 SQL 细节。
func sqlErrNoRows() error { return sql.ErrNoRows }

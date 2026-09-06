package workstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const changeSchemaSQL = `
CREATE TABLE t_artifacts (
    artifact_id TEXT PRIMARY KEY NOT NULL,
    sha256 TEXT NOT NULL,
    byte_length INTEGER NOT NULL CHECK (byte_length >= 0),
    media_type TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(sha256, byte_length)
);
CREATE TABLE t_changes (
    change_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    repository_root TEXT NOT NULL,
    stage TEXT NOT NULL,
    status TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version >= 1),
    base_revision TEXT NOT NULL,
    intent_artifact_ref_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(change_id, project_id),
    FOREIGN KEY(intent_artifact_ref_id, change_id, project_id)
        REFERENCES t_artifact_refs(artifact_ref_id, change_id, project_id)
        DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE t_artifact_refs (
    artifact_ref_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES t_artifacts(artifact_id),
    role TEXT NOT NULL CHECK (role IN ('change_intent', 'input', 'output', 'failure')),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    created_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id),
    UNIQUE(change_id, role, ordinal),
    UNIQUE(artifact_ref_id, change_id, project_id)
);
CREATE TABLE t_agent_runs (
    agent_run_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    stage TEXT NOT NULL,
    attempt INTEGER NOT NULL CHECK (attempt >= 1),
    status TEXT NOT NULL CHECK (status IN ('running', 'completed')),
    outcome TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    completed_at TEXT,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id),
    UNIQUE(change_id, stage, attempt),
    CHECK ((status = 'running' AND outcome = '' AND completed_at IS NULL) OR
           (status = 'completed' AND outcome IN ('succeeded', 'failed', 'human_required') AND completed_at IS NOT NULL))
);
CREATE TABLE t_agent_run_artifacts (
    agent_run_id TEXT NOT NULL REFERENCES t_agent_runs(agent_run_id),
    artifact_ref_id TEXT NOT NULL REFERENCES t_artifact_refs(artifact_ref_id),
    role TEXT NOT NULL CHECK (role IN ('input', 'output', 'failure')),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY(agent_run_id, role, ordinal),
    UNIQUE(agent_run_id, artifact_ref_id)
);
CREATE TABLE t_human_decisions (
    decision_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('retry', 'cancel')),
    actor TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);
CREATE TABLE t_change_command_receipts (
    idempotency_key TEXT PRIMARY KEY NOT NULL,
    operation TEXT NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    request_payload TEXT NOT NULL DEFAULT '',
    status_code INTEGER NOT NULL,
    response_body TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);
ALTER TABLE t_project_events RENAME TO t_project_events_v2;
CREATE TABLE t_project_events (
    event_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    type TEXT NOT NULL CHECK (type IN ('ProjectInitialized', 'ChangeCreated', 'AgentRunStarted', 'AgentRunCompleted', 'StageAdvanced', 'ChangePaused', 'ChangeResumed', 'ChangeHumanRequired', 'HumanDecisionRecorded', 'ChangeCancelled')),
    occurred_at TEXT NOT NULL,
    event_sequence INTEGER NOT NULL CHECK (event_sequence >= 1),
    change_id TEXT,
    agent_run_id TEXT REFERENCES t_agent_runs(agent_run_id),
    decision_id TEXT REFERENCES t_human_decisions(decision_id),
    actor TEXT NOT NULL DEFAULT '',
    UNIQUE(change_id, event_sequence),
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id),
    FOREIGN KEY(agent_run_id) REFERENCES t_agent_runs(agent_run_id),
    FOREIGN KEY(decision_id) REFERENCES t_human_decisions(decision_id)
);
CREATE UNIQUE INDEX ux_project_initialized_event
    ON t_project_events(project_id, type) WHERE change_id IS NULL;
CREATE TABLE t_event_artifacts (
    event_id TEXT NOT NULL REFERENCES t_project_events(event_id),
    artifact_ref_id TEXT NOT NULL REFERENCES t_artifact_refs(artifact_ref_id),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY(event_id, ordinal),
    UNIQUE(event_id, artifact_ref_id)
);
INSERT INTO t_project_events (event_id, project_id, type, occurred_at, event_sequence, actor)
SELECT event_id, project_id, type, occurred_at, 1, '' FROM t_project_events_v2;
DROP TABLE t_project_events_v2;
CREATE INDEX ix_changes_project_created ON t_changes(project_id, created_at DESC, change_id DESC);
CREATE INDEX ix_events_change_sequence ON t_project_events(change_id, event_sequence);
CREATE INDEX ix_agent_runs_change_started ON t_agent_runs(change_id, started_at, agent_run_id);
CREATE INDEX ix_decisions_change_created ON t_human_decisions(change_id, created_at, decision_id);
CREATE UNIQUE INDEX ux_change_intent_artifact_ref
    ON t_artifact_refs(change_id) WHERE role = 'change_intent';

CREATE TRIGGER tr_change_events_no_update
BEFORE UPDATE ON t_project_events
BEGIN
    SELECT RAISE(ABORT, 'change events are append-only');
END;
CREATE TRIGGER tr_change_events_no_delete
BEFORE DELETE ON t_project_events
BEGIN
    SELECT RAISE(ABORT, 'change events are append-only');
END;
CREATE TRIGGER tr_artifacts_no_update
BEFORE UPDATE ON t_artifacts
BEGIN
    SELECT RAISE(ABORT, 'artifacts are immutable');
END;
CREATE TRIGGER tr_artifacts_no_delete
BEFORE DELETE ON t_artifacts
BEGIN
    SELECT RAISE(ABORT, 'artifacts are immutable');
END;
CREATE TRIGGER tr_artifact_refs_no_update
BEFORE UPDATE ON t_artifact_refs
BEGIN
    SELECT RAISE(ABORT, 'artifact references are append-only');
END;
CREATE TRIGGER tr_artifact_refs_no_delete
BEFORE DELETE ON t_artifact_refs
BEGIN
    SELECT RAISE(ABORT, 'artifact references are append-only');
END;
CREATE TRIGGER tr_event_artifacts_no_update
BEFORE UPDATE ON t_event_artifacts
BEGIN
    SELECT RAISE(ABORT, 'event artifact references are append-only');
END;
CREATE TRIGGER tr_event_artifacts_no_delete
BEFORE DELETE ON t_event_artifacts
BEGIN
    SELECT RAISE(ABORT, 'event artifact references are append-only');
END;
CREATE TRIGGER tr_agent_run_artifacts_no_update
BEFORE UPDATE ON t_agent_run_artifacts
BEGIN
    SELECT RAISE(ABORT, 'agent run artifact references are append-only');
END;
CREATE TRIGGER tr_agent_run_artifacts_no_delete
BEFORE DELETE ON t_agent_run_artifacts
BEGIN
    SELECT RAISE(ABORT, 'agent run artifact references are append-only');
END;
CREATE TRIGGER tr_human_decisions_no_update
BEFORE UPDATE ON t_human_decisions
BEGIN
    SELECT RAISE(ABORT, 'human decisions are append-only');
END;
CREATE TRIGGER tr_human_decisions_no_delete
BEFORE DELETE ON t_human_decisions
BEGIN
    SELECT RAISE(ABORT, 'human decisions are append-only');
END;
CREATE TRIGGER tr_change_receipts_no_update
BEFORE UPDATE ON t_change_command_receipts
BEGIN
    SELECT RAISE(ABORT, 'change command receipts are append-only');
END;
CREATE TRIGGER tr_change_receipts_no_delete
BEFORE DELETE ON t_change_command_receipts
BEGIN
    SELECT RAISE(ABORT, 'change command receipts are append-only');
END;
CREATE TRIGGER tr_agent_runs_no_delete
BEFORE DELETE ON t_agent_runs
BEGIN
    SELECT RAISE(ABORT, 'agent runs are append-only');
END;
CREATE TRIGGER tr_agent_runs_only_complete
BEFORE UPDATE ON t_agent_runs
WHEN OLD.status <> 'running'
  OR NEW.status <> 'completed'
  OR NEW.agent_run_id <> OLD.agent_run_id
  OR NEW.project_id <> OLD.project_id
  OR NEW.change_id <> OLD.change_id
  OR NEW.stage <> OLD.stage
  OR NEW.attempt <> OLD.attempt
  OR NEW.started_at <> OLD.started_at
  OR NEW.outcome NOT IN ('succeeded', 'failed', 'human_required')
  OR NEW.completed_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'agent runs only allow running to completed');
END;
CREATE TRIGGER tr_event_artifact_ownership
BEFORE INSERT ON t_event_artifacts
WHEN NOT EXISTS (
    SELECT 1
    FROM t_project_events e
    JOIN t_artifact_refs r ON r.artifact_ref_id = NEW.artifact_ref_id
    WHERE e.event_id = NEW.event_id
      AND e.change_id IS NOT NULL
      AND e.change_id = r.change_id
      AND e.project_id = r.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'event artifact belongs to another change');
END;
CREATE TRIGGER tr_agent_run_artifact_ownership
BEFORE INSERT ON t_agent_run_artifacts
WHEN NOT EXISTS (
    SELECT 1
    FROM t_agent_runs run
    JOIN t_artifact_refs ref ON ref.artifact_ref_id = NEW.artifact_ref_id
    WHERE run.agent_run_id = NEW.agent_run_id
      AND run.change_id = ref.change_id
      AND run.project_id = ref.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'agent run artifact belongs to another change');
END;
CREATE TRIGGER tr_event_run_ownership
BEFORE INSERT ON t_project_events
WHEN NEW.agent_run_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM t_agent_runs
    WHERE agent_run_id = NEW.agent_run_id
      AND change_id = NEW.change_id
      AND project_id = NEW.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'event agent run belongs to another change');
END;
CREATE TRIGGER tr_event_decision_ownership
BEFORE INSERT ON t_project_events
WHEN NEW.decision_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM t_human_decisions
    WHERE decision_id = NEW.decision_id
      AND change_id = NEW.change_id
      AND project_id = NEW.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'event decision belongs to another change');
END;
CREATE TRIGGER tr_event_aggregate_shape
BEFORE INSERT ON t_project_events
WHEN (NEW.type = 'ProjectInitialized' AND NEW.change_id IS NOT NULL)
  OR (NEW.type <> 'ProjectInitialized' AND NEW.change_id IS NULL)
BEGIN
    SELECT RAISE(ABORT, 'event aggregate shape is invalid');
END;
CREATE TRIGGER tr_event_run_shape
BEFORE INSERT ON t_project_events
WHEN (NEW.type IN ('AgentRunStarted', 'AgentRunCompleted', 'StageAdvanced', 'ChangeHumanRequired') AND NEW.agent_run_id IS NULL)
  OR (NEW.type NOT IN ('AgentRunStarted', 'AgentRunCompleted', 'StageAdvanced', 'ChangeHumanRequired') AND NEW.agent_run_id IS NOT NULL)
BEGIN
    SELECT RAISE(ABORT, 'event agent run association is invalid');
END;
CREATE TRIGGER tr_event_decision_shape
BEFORE INSERT ON t_project_events
WHEN (NEW.type = 'HumanDecisionRecorded' AND NEW.decision_id IS NULL)
  OR (NEW.type <> 'HumanDecisionRecorded' AND NEW.decision_id IS NOT NULL)
BEGIN
    SELECT RAISE(ABORT, 'event decision association is invalid');
END;
CREATE TRIGGER tr_agent_run_artifact_role
BEFORE INSERT ON t_agent_run_artifacts
WHEN NOT EXISTS (
    SELECT 1 FROM t_artifact_refs
    WHERE artifact_ref_id = NEW.artifact_ref_id
      AND (role = NEW.role OR (NEW.role = 'input' AND role = 'change_intent'))
)
BEGIN
    SELECT RAISE(ABORT, 'agent run artifact role does not match reference');
END;
CREATE TRIGGER tr_change_intent_ref_role
BEFORE INSERT ON t_artifact_refs
WHEN NEW.role <> 'change_intent' AND EXISTS (
    SELECT 1 FROM t_changes
    WHERE change_id = NEW.change_id
      AND project_id = NEW.project_id
      AND intent_artifact_ref_id = NEW.artifact_ref_id
)
BEGIN
    SELECT RAISE(ABORT, 'change intent reference role is invalid');
END;
CREATE TRIGGER tr_change_intent_change_role
BEFORE INSERT ON t_changes
WHEN EXISTS (
    SELECT 1 FROM t_artifact_refs
    WHERE artifact_ref_id = NEW.intent_artifact_ref_id
      AND change_id = NEW.change_id
      AND project_id = NEW.project_id
      AND role <> 'change_intent'
)
BEGIN
    SELECT RAISE(ABORT, 'change intent reference role is invalid');
END;
CREATE TRIGGER tr_change_intent_ref_ownership
BEFORE INSERT ON t_artifact_refs
WHEN NEW.role = 'change_intent' AND NOT EXISTS (
    SELECT 1 FROM t_changes
    WHERE change_id = NEW.change_id
      AND project_id = NEW.project_id
      AND intent_artifact_ref_id = NEW.artifact_ref_id
)
BEGIN
    SELECT RAISE(ABORT, 'change intent reference does not match change');
END;
CREATE TRIGGER tr_change_intent_ref_update_ownership
BEFORE UPDATE OF intent_artifact_ref_id ON t_changes
WHEN NEW.intent_artifact_ref_id <> OLD.intent_artifact_ref_id
  AND NOT EXISTS (
    SELECT 1 FROM t_artifact_refs
    WHERE artifact_ref_id = NEW.intent_artifact_ref_id
      AND change_id = NEW.change_id
      AND project_id = NEW.project_id
      AND role = 'change_intent'
  )
BEGIN
    SELECT RAISE(ABORT, 'change intent reference does not belong to change');
END;`

func changeMigration() migration.Migration {
	return migration.Migration{Version: 3, Name: "create_change_lifecycle", SQL: changeSchemaSQL}
}

// FindProjectByRoot 查询已注册且仍绑定该 Repository root 的 Project。
func (s *Store) FindProjectByRoot(ctx context.Context, root string) (*domain.Project, error) {
	return readProjectByRoot(ctx, s.db, root)
}

// ReplayCreate 在读取 Git 源快照前返回已经成功的同一创建请求。
func (s *Store) ReplayCreate(ctx context.Context, key string, projectID domain.ProjectID, root, intent string) (domain.Change, bool, error) {
	receipt, err := readChangeReceipt(ctx, s.db, key)
	if err != nil {
		return domain.Change{}, false, err
	}
	if receipt == nil {
		return domain.Change{}, false, nil
	}
	if receipt.Operation != "create" {
		return domain.Change{}, false, fmt.Errorf("create receipt operation differs: %w", domain.ErrIdempotencyConflict)
	}
	payload, err := decodeChangeReceiptPayload(receipt.ResponseBody)
	if err != nil {
		return domain.Change{}, false, err
	}
	change, err := changeFromReceiptView(payload.Change)
	if err != nil {
		return domain.Change{}, false, err
	}
	if change.ID == "" || string(change.ID) != receipt.ChangeID || change.ProjectID != projectID || change.RepositoryRoot != root || receipt.RequestPayload != intent {
		return domain.Change{}, false, fmt.Errorf("create receipt request differs: %w", domain.ErrIdempotencyConflict)
	}
	if err := change.Validate(); err != nil {
		return domain.Change{}, false, fmt.Errorf("validate change receipt: %w", domain.ErrInternal)
	}
	return change, true, nil
}

// CreateChange 在一个事务中保存 Change、Intent ArtifactRef、ChangeCreated 和 Receipt。
func (s *Store) CreateChange(ctx context.Context, record work.ChangeCreateRecord) (change domain.Change, err error) {
	if err := validateCreateRecord(record); err != nil {
		return change, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return change, fmt.Errorf("begin change creation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	fingerprint := createFingerprint(record)
	receipt, err := readChangeReceipt(ctx, tx, record.IdempotencyKey)
	if err != nil {
		return change, err
	}
	if receipt != nil {
		if receipt.Operation != "create" || receipt.RequestFingerprint != fingerprint {
			return change, fmt.Errorf("create receipt request differs: %w", domain.ErrIdempotencyConflict)
		}
		change, err = replayChange(receipt)
		if err != nil {
			return change, err
		}
		if err := tx.Commit(); err != nil {
			return domain.Change{}, fmt.Errorf("commit change replay: %w", err)
		}
		committed = true
		return change, nil
	}
	created := s.now().UTC()
	artifact, err := ensureArtifact(ctx, tx, record.IntentIdentity, "text/plain; charset=utf-8", created)
	if err != nil {
		return change, err
	}
	changeID := domain.ChangeID(id.New())
	refID := domain.ArtifactRefID(id.New())
	intentRef := domain.ArtifactRef{ID: refID, ChangeID: changeID, ArtifactID: artifact.ID, Role: domain.ArtifactRoleChangeIntent, Ordinal: 0}
	change, err = domain.NewChange(record.Project.Identity.ProjectID, changeID, record.Project.Binding.Root, record.Snapshot.BaseRevision, intentRef, created)
	if err != nil {
		return change, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_changes (change_id, project_id, repository_root, stage, status, version, base_revision, intent_artifact_ref_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, change.ID, change.ProjectID, change.RepositoryRoot, change.Stage, change.Status, change.Version, change.BaseRevision, intentRef.ID, stamp(created), stamp(created)); err != nil {
		return change, fmt.Errorf("insert change: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifact_refs (artifact_ref_id, project_id, change_id, artifact_id, role, ordinal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, refID, change.ProjectID, change.ID, artifact.ID, intentRef.Role, intentRef.Ordinal, stamp(created)); err != nil {
		return change, fmt.Errorf("insert change intent artifact reference: %w", err)
	}
	if err := s.insertEvent(ctx, tx, change.ProjectID, change.ID, domain.ChangeCreatedType, record.Actor, created, nil, nil, []domain.ArtifactRefID{refID}); err != nil {
		return change, err
	}
	if err := insertChangeReceipt(ctx, tx, record.IdempotencyKey, "create", change, record.Intent.Original, fingerprint, 201, created); err != nil {
		return change, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Change{}, fmt.Errorf("commit change creation: %w", err)
	}
	committed = true
	return change, nil
}

// FindChange 查询 Change 及其最新 AgentRun 摘要。
func (s *Store) FindChange(ctx context.Context, changeID domain.ChangeID) (domain.Change, error) {
	return readChange(ctx, s.db, changeID)
}

// ListChanges 按 created_at、change_id 倒序返回一个 Repository 的 Change。
func (s *Store) ListChanges(ctx context.Context, root string) ([]domain.Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT change_id FROM t_changes WHERE repository_root = ? ORDER BY created_at DESC, change_id DESC`, root)
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	changeIDs := make([]domain.ChangeID, 0)
	for rows.Next() {
		var changeID domain.ChangeID
		if err := rows.Scan(&changeID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan change id: %w", err)
		}
		changeIDs = append(changeIDs, changeID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("read changes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close changes: %w", err)
	}
	changes := make([]domain.Change, 0, len(changeIDs))
	for _, changeID := range changeIDs {
		change, err := readChange(ctx, s.db, changeID)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, nil
}

// ApplyCommand 以旧 ChangeVersion 为条件执行 Pause、Resume 或 Cancel。
func (s *Store) ApplyCommand(ctx context.Context, changeID domain.ChangeID, command string, expected domain.ChangeVersion, key, actor string) (change domain.Change, err error) {
	fingerprint := commandFingerprint(command, changeID, expected, actor)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return change, fmt.Errorf("begin change command: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	receipt, err := readChangeReceipt(ctx, tx, key)
	if err != nil {
		return change, err
	}
	if receipt != nil {
		if receipt.Operation != command || receipt.RequestFingerprint != fingerprint {
			return change, fmt.Errorf("change command receipt request differs: %w", domain.ErrIdempotencyConflict)
		}
		change, err = replayChange(receipt)
		if err != nil {
			return change, err
		}
		if err := tx.Commit(); err != nil {
			return domain.Change{}, fmt.Errorf("commit change command replay: %w", err)
		}
		committed = true
		return change, nil
	}
	change, err = readChange(ctx, tx, changeID)
	if err != nil {
		return change, err
	}
	if change.Version != expected {
		return change, fmt.Errorf("change version is %d, expected %d: %w", change.Version, expected, domain.ErrChangeVersionConflict)
	}
	updated, err := change.Transition(command)
	if err != nil {
		return change, err
	}
	updatedAt := s.now().UTC()
	updated.UpdatedAt = updatedAt
	if err := updateChangeStatus(ctx, tx, change, updated, updatedAt); err != nil {
		return change, err
	}
	eventType := map[string]string{"pause": domain.ChangePausedType, "resume": domain.ChangeResumedType, "cancel": domain.ChangeCancelledType}[command]
	if err := s.insertEvent(ctx, tx, change.ProjectID, change.ID, eventType, actor, updatedAt, nil, nil, nil); err != nil {
		return change, err
	}
	if err := insertChangeReceipt(ctx, tx, key, command, updated, "", fingerprint, 200, updatedAt); err != nil {
		return change, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Change{}, fmt.Errorf("commit change command: %w", err)
	}
	committed = true
	return updated, nil
}

// ApplyDecision 持久化 HumanDecision，并在 retry 时创建新的 running AgentRun。
func (s *Store) ApplyDecision(ctx context.Context, changeID domain.ChangeID, decision string, expected domain.ChangeVersion, key, actor, reason string) (change domain.Change, err error) {
	fingerprint := commandFingerprint("decision:"+decision, changeID, expected, actor, reason)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return change, fmt.Errorf("begin human decision: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	receipt, err := readChangeReceipt(ctx, tx, key)
	if err != nil {
		return change, err
	}
	if receipt != nil {
		if receipt.Operation != "decision:"+decision || receipt.RequestFingerprint != fingerprint {
			return change, fmt.Errorf("human decision receipt request differs: %w", domain.ErrIdempotencyConflict)
		}
		change, err = replayChange(receipt)
		if err != nil {
			return change, err
		}
		if err := tx.Commit(); err != nil {
			return domain.Change{}, fmt.Errorf("commit human decision replay: %w", err)
		}
		committed = true
		return change, nil
	}
	change, err = readChange(ctx, tx, changeID)
	if err != nil {
		return change, err
	}
	if change.Version != expected {
		return change, fmt.Errorf("change version is %d, expected %d: %w", change.Version, expected, domain.ErrChangeVersionConflict)
	}
	updated, err := change.ApplyHumanDecision(decision)
	if err != nil {
		return change, err
	}
	created := s.now().UTC()
	updated.UpdatedAt = created
	decisionID := domain.HumanDecisionID(id.New())
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_human_decisions (decision_id, project_id, change_id, kind, actor, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, decisionID, change.ProjectID, change.ID, decision, actor, reason, stamp(created)); err != nil {
		return change, fmt.Errorf("insert human decision: %w", err)
	}
	if err := updateChangeStatus(ctx, tx, change, updated, created); err != nil {
		return change, err
	}
	if err := s.insertEvent(ctx, tx, change.ProjectID, change.ID, domain.HumanDecisionRecordedType, actor, created, nil, &decisionID, nil); err != nil {
		return change, err
	}
	if decision == domain.HumanDecisionRetry {
		run, runErr := insertAgentRun(ctx, tx, change.ProjectID, change.ID, change.Stage, created, nil)
		if runErr != nil {
			return change, runErr
		}
		updated.LatestAgentRun = &run
		if err := s.insertEvent(ctx, tx, change.ProjectID, change.ID, domain.AgentRunStartedType, actor, created, &run.ID, nil, nil); err != nil {
			return change, err
		}
	} else if decision == domain.HumanDecisionCancel {
		if err := s.insertEvent(ctx, tx, change.ProjectID, change.ID, domain.ChangeCancelledType, actor, created, nil, nil, nil); err != nil {
			return change, err
		}
	}
	if err := insertChangeReceipt(ctx, tx, key, "decision:"+decision, updated, "", fingerprint, 200, created); err != nil {
		return change, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Change{}, fmt.Errorf("commit human decision: %w", err)
	}
	committed = true
	return updated, nil
}

// StartAgentRun 创建一个没有额外输入 Artifact 的阶段尝试。
func (s *Store) StartAgentRun(ctx context.Context, changeID domain.ChangeID, actor string) (domain.AgentRun, error) {
	return s.StartAgentRunWithArtifacts(ctx, changeID, actor, nil)
}

// StartAgentRunWithArtifacts 创建阶段尝试并追加有序输入 Artifact 关联。
func (s *Store) StartAgentRunWithArtifacts(ctx context.Context, changeID domain.ChangeID, actor string, artifacts []domain.AgentRunArtifact) (run domain.AgentRun, err error) {
	if err := validateRunArtifactRoles(artifacts, domain.ArtifactRoleInput); err != nil {
		return run, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return run, fmt.Errorf("begin agent run: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	change, err := readChange(ctx, tx, changeID)
	if err != nil {
		return run, err
	}
	if change.Status != domain.ChangeStatusActive {
		return run, fmt.Errorf("start agent run: %w", domain.ErrLifecycleTransitionInvalid)
	}
	created := s.now().UTC()
	run, err = insertAgentRun(ctx, tx, change.ProjectID, change.ID, change.Stage, created, artifacts)
	if err != nil {
		return run, err
	}
	if err := s.insertEvent(ctx, tx, change.ProjectID, change.ID, domain.AgentRunStartedType, actor, created, &run.ID, nil, agentRunArtifactRefIDs(artifacts)); err != nil {
		return run, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, fmt.Errorf("commit agent run: %w", err)
	}
	committed = true
	return run, nil
}

// CompleteAgentRun 固定 AgentRun 终态，并按结果协调 Change 状态。
func (s *Store) CompleteAgentRun(ctx context.Context, runID domain.AgentRunID, outcome string, actor string) (domain.AgentRun, error) {
	return s.CompleteAgentRunWithArtifacts(ctx, runID, outcome, actor, nil)
}

// CompleteAgentRunWithArtifacts 固定 AgentRun 终态并追加输出或失败 Artifact 关联。
func (s *Store) CompleteAgentRunWithArtifacts(ctx context.Context, runID domain.AgentRunID, outcome string, actor string, artifacts []domain.AgentRunArtifact) (run domain.AgentRun, err error) {
	role := domain.ArtifactRoleOutput
	if outcome == domain.AgentRunOutcomeFailed || outcome == domain.AgentRunOutcomeHumanRequired {
		role = domain.ArtifactRoleFailure
	}
	if err := validateRunArtifactRoles(artifacts, role); err != nil {
		return run, err
	}
	return s.completeAgentRun(ctx, runID, outcome, actor, artifacts, nil)
}

// CompleteAgentRunWithArtifactInputs 在一个事务中登记内容对应的 ArtifactRef 并完成 AgentRun。
func (s *Store) CompleteAgentRunWithArtifactInputs(ctx context.Context, runID domain.AgentRunID, outcome string, actor string, inputs []work.AgentRunArtifactInput) (run domain.AgentRun, err error) {
	role := domain.ArtifactRoleOutput
	if outcome == domain.AgentRunOutcomeFailed || outcome == domain.AgentRunOutcomeHumanRequired {
		role = domain.ArtifactRoleFailure
	}
	if err := validateArtifactInputs(inputs, role); err != nil {
		return run, err
	}
	return s.completeAgentRun(ctx, runID, outcome, actor, nil, inputs)
}

func (s *Store) completeAgentRun(ctx context.Context, runID domain.AgentRunID, outcome, actor string, artifacts []domain.AgentRunArtifact, inputs []work.AgentRunArtifactInput) (run domain.AgentRun, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return run, fmt.Errorf("begin agent run completion: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	run, err = readAgentRun(ctx, tx, runID)
	if err != nil {
		return run, err
	}
	completed := s.now().UTC()
	if err := run.Complete(outcome, completed); err != nil {
		return run, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = ?, outcome = ?, completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, run.Status, run.Outcome, stamp(completed), run.ID); err != nil {
		return run, fmt.Errorf("complete agent run: %w", err)
	}
	change, err := readChange(ctx, tx, run.ChangeID)
	if err != nil {
		return run, err
	}
	if len(inputs) > 0 {
		generated, inputErr := insertArtifactInputs(ctx, tx, change, inputs, completed)
		if inputErr != nil {
			return run, inputErr
		}
		artifacts = append(artifacts, generated...)
	}
	if err := insertAgentRunArtifacts(ctx, tx, run.ID, artifacts); err != nil {
		return run, err
	}
	run.Artifacts = append(run.Artifacts, artifacts...)
	artifactRefIDs := agentRunArtifactRefIDs(artifacts)
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, actor, completed, &run.ID, nil, artifactRefIDs); err != nil {
		return run, err
	}
	currentRun, err := isCurrentAgentRun(ctx, tx, change, run)
	if err != nil {
		return run, err
	}
	if change.CanAdvanceLateResult() && currentRun {
		next := change
		if outcome == domain.AgentRunOutcomeFailed || outcome == domain.AgentRunOutcomeHumanRequired {
			next, err = change.EnterHumanRequired()
			if err == nil {
				next.UpdatedAt = completed
				err = updateChangeStatus(ctx, tx, change, next, completed)
			}
			if err == nil {
				err = insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.ChangeHumanRequiredType, actor, completed, &run.ID, nil, artifactRefIDs)
			}
		} else if nextStage, ok := advanceStage(change.Stage); ok {
			next.Stage = nextStage
			next.Version++
			next.UpdatedAt = completed
			err = updateChangeStage(ctx, tx, change, next, completed)
			if err == nil {
				err = insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.StageAdvancedType, actor, completed, &run.ID, nil, artifactRefIDs)
			}
		}
		if err != nil {
			return run, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, fmt.Errorf("commit agent run completion: %w", err)
	}
	committed = true
	return run, nil
}

// ListChangeEvents 返回 Change Event 的 EventSequence 升序历史。
func (s *Store) ListChangeEvents(ctx context.Context, changeID domain.ChangeID) ([]domain.ChangeEvent, error) {
	if _, err := readChange(ctx, s.db, changeID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, project_id, change_id, event_sequence, type, occurred_at, actor, agent_run_id, decision_id FROM t_project_events WHERE change_id = ? ORDER BY event_sequence`, changeID)
	if err != nil {
		return nil, fmt.Errorf("list change events: %w", err)
	}
	events := make([]domain.ChangeEvent, 0)
	defer rows.Close()
	for rows.Next() {
		var event domain.ChangeEvent
		var eventChangeID, occurred, agentRunID, decisionID sql.NullString
		if err := rows.Scan(&event.EventID, &event.ProjectID, &eventChangeID, &event.Sequence, &event.Type, &occurred, &event.Actor, &agentRunID, &decisionID); err != nil {
			return nil, fmt.Errorf("scan change event: %w", err)
		}
		event.ChangeID = domain.ChangeID(eventChangeID.String)
		event.OccurredAt, err = parseStamp(occurred.String)
		if err != nil {
			return nil, err
		}
		if agentRunID.Valid {
			value := domain.AgentRunID(agentRunID.String)
			event.AgentRunID = &value
		}
		if decisionID.Valid {
			value := domain.HumanDecisionID(decisionID.String)
			event.DecisionID = &value
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read change events: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close change events: %w", err)
	}
	for index := range events {
		refs, err := listEventArtifactRefs(ctx, s.db, events[index].EventID)
		if err != nil {
			return nil, err
		}
		events[index].ArtifactRefIDs = refs
	}
	return events, nil
}

// ListAgentRuns 返回 AgentRun 的创建顺序历史。
func (s *Store) ListAgentRuns(ctx context.Context, changeID domain.ChangeID) ([]domain.AgentRun, error) {
	if _, err := readChange(ctx, s.db, changeID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT agent_run_id FROM t_agent_runs WHERE change_id = ? ORDER BY started_at, agent_run_id`, changeID)
	if err != nil {
		return nil, fmt.Errorf("list agent runs: %w", err)
	}
	runIDs := make([]domain.AgentRunID, 0)
	for rows.Next() {
		var runID domain.AgentRunID
		if err := rows.Scan(&runID); err != nil {
			return nil, err
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	runs := make([]domain.AgentRun, 0, len(runIDs))
	for _, runID := range runIDs {
		run, err := readAgentRun(ctx, s.db, runID)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// ListArtifactRefs 返回 Change ArtifactRef 的角色和序号顺序。
func (s *Store) ListArtifactRefs(ctx context.Context, changeID domain.ChangeID) ([]domain.ArtifactRef, error) {
	if _, err := readChange(ctx, s.db, changeID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_ref_id, artifact_id, role, ordinal FROM t_artifact_refs WHERE change_id = ? ORDER BY created_at, artifact_ref_id`, changeID)
	if err != nil {
		return nil, fmt.Errorf("list artifact refs: %w", err)
	}
	defer rows.Close()
	refs := make([]domain.ArtifactRef, 0)
	for rows.Next() {
		var ref domain.ArtifactRef
		if err := rows.Scan(&ref.ID, &ref.ArtifactID, &ref.Role, &ref.Ordinal); err != nil {
			return nil, err
		}
		ref.ChangeID = changeID
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// ListHumanDecisions 返回按创建时间和 ID 排序的人工决定历史。
func (s *Store) ListHumanDecisions(ctx context.Context, changeID domain.ChangeID) ([]domain.HumanDecision, error) {
	if _, err := readChange(ctx, s.db, changeID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT decision_id, change_id, kind, actor, reason, created_at FROM t_human_decisions WHERE change_id = ? ORDER BY created_at, decision_id`, changeID)
	if err != nil {
		return nil, fmt.Errorf("list human decisions: %w", err)
	}
	defer rows.Close()
	decisions := make([]domain.HumanDecision, 0)
	for rows.Next() {
		var decision domain.HumanDecision
		var created string
		if err := rows.Scan(&decision.ID, &decision.ChangeID, &decision.Kind, &decision.Actor, &decision.Reason, &created); err != nil {
			return nil, err
		}
		decision.CreatedAt, err = parseStamp(created)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, rows.Err()
}

// FindArtifact 查询属于指定 Change 的 ArtifactRef 对应内容摘要。
func (s *Store) FindArtifact(ctx context.Context, changeID domain.ChangeID, refID domain.ArtifactRefID) (domain.Artifact, error) {
	var artifact domain.Artifact
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT a.artifact_id, a.sha256, a.byte_length, a.media_type, a.created_at FROM t_artifacts a JOIN t_artifact_refs r ON r.artifact_id = a.artifact_id WHERE r.change_id = ? AND r.artifact_ref_id = ?`, changeID, refID).Scan(&artifact.ID, &artifact.Identity.SHA256, &artifact.Identity.ByteLength, &artifact.MediaType, &created)
	if errors.Is(err, sql.ErrNoRows) {
		if _, changeErr := readChange(ctx, s.db, changeID); changeErr != nil {
			return domain.Artifact{}, changeErr
		}
		return domain.Artifact{}, domain.ErrArtifactNotFound
	}
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("find artifact: %w", err)
	}
	artifact.CreatedAt, err = parseStamp(created)
	if err != nil {
		return domain.Artifact{}, err
	}
	return artifact, nil
}

type changeReceipt struct {
	Operation          string
	ProjectID          string
	ChangeID           string
	RequestFingerprint string
	RequestPayload     string
	StatusCode         int
	ResponseBody       string
}

type changeReceiptPayload struct {
	Change receiptChangeView `json:"change"`
}

type receiptChangeView struct {
	ChangeID       string                 `json:"change_id"`
	ProjectID      string                 `json:"project_id"`
	RepositoryRoot string                 `json:"repository_root"`
	Stage          string                 `json:"stage"`
	Status         string                 `json:"status"`
	Version        int                    `json:"version"`
	BaseRevision   string                 `json:"base_revision"`
	IntentArtifact receiptArtifactRefView `json:"intent_artifact"`
	LatestAgentRun *receiptAgentRunView   `json:"latest_agent_run"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

type receiptArtifactRefView struct {
	ArtifactRefID string `json:"artifact_ref_id"`
	ArtifactID    string `json:"artifact_id"`
	Role          string `json:"role"`
	Ordinal       int    `json:"ordinal"`
}

type receiptAgentRunView struct {
	AgentRunID  string                     `json:"agent_run_id"`
	ChangeID    string                     `json:"change_id"`
	Stage       string                     `json:"stage"`
	Attempt     int                        `json:"attempt"`
	Status      string                     `json:"status"`
	Outcome     string                     `json:"outcome"`
	Artifacts   []receiptAgentArtifactView `json:"artifacts"`
	StartedAt   string                     `json:"started_at"`
	CompletedAt *string                    `json:"completed_at"`
}

type receiptAgentArtifactView struct {
	ArtifactRefID string `json:"artifact_ref_id"`
	Role          string `json:"role"`
	Ordinal       int    `json:"ordinal"`
}

func receiptChangeViewFor(change domain.Change) receiptChangeView {
	view := receiptChangeView{
		ChangeID: string(change.ID), ProjectID: string(change.ProjectID), RepositoryRoot: change.RepositoryRoot,
		Stage: string(change.Stage), Status: string(change.Status), Version: int(change.Version), BaseRevision: change.BaseRevision,
		IntentArtifact: receiptArtifactRefView{ArtifactRefID: string(change.Intent.ID), ArtifactID: string(change.Intent.ArtifactID), Role: change.Intent.Role, Ordinal: change.Intent.Ordinal},
		CreatedAt:      stamp(change.CreatedAt), UpdatedAt: stamp(change.UpdatedAt),
	}
	if change.LatestAgentRun != nil {
		run := change.LatestAgentRun
		runView := receiptAgentRunView{AgentRunID: string(run.ID), ChangeID: string(run.ChangeID), Stage: string(run.Stage), Attempt: run.Attempt, Status: run.Status, Outcome: run.Outcome, StartedAt: stamp(run.StartedAt)}
		runView.Artifacts = make([]receiptAgentArtifactView, 0, len(run.Artifacts))
		for _, artifact := range run.Artifacts {
			runView.Artifacts = append(runView.Artifacts, receiptAgentArtifactView{ArtifactRefID: string(artifact.ArtifactRefID), Role: artifact.Role, Ordinal: artifact.Ordinal})
		}
		if run.CompletedAt != nil {
			completedAt := stamp(*run.CompletedAt)
			runView.CompletedAt = &completedAt
		}
		view.LatestAgentRun = &runView
	}
	return view
}

func changeFromReceiptView(view receiptChangeView) (domain.Change, error) {
	createdAt, err := parseStamp(view.CreatedAt)
	if err != nil {
		return domain.Change{}, err
	}
	updatedAt, err := parseStamp(view.UpdatedAt)
	if err != nil {
		return domain.Change{}, err
	}
	change := domain.Change{
		ID: domain.ChangeID(view.ChangeID), ProjectID: domain.ProjectID(view.ProjectID), RepositoryRoot: view.RepositoryRoot,
		Stage: domain.LifecycleStage(view.Stage), Status: domain.ChangeStatus(view.Status), Version: domain.ChangeVersion(view.Version), BaseRevision: view.BaseRevision,
		Intent:    domain.ArtifactRef{ID: domain.ArtifactRefID(view.IntentArtifact.ArtifactRefID), ChangeID: domain.ChangeID(view.ChangeID), ArtifactID: domain.ArtifactID(view.IntentArtifact.ArtifactID), Role: view.IntentArtifact.Role, Ordinal: view.IntentArtifact.Ordinal},
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	if view.LatestAgentRun != nil {
		startedAt, parseErr := parseStamp(view.LatestAgentRun.StartedAt)
		if parseErr != nil {
			return domain.Change{}, parseErr
		}
		run := domain.AgentRun{ID: domain.AgentRunID(view.LatestAgentRun.AgentRunID), ChangeID: domain.ChangeID(view.LatestAgentRun.ChangeID), Stage: domain.LifecycleStage(view.LatestAgentRun.Stage), Attempt: view.LatestAgentRun.Attempt, Status: view.LatestAgentRun.Status, Outcome: view.LatestAgentRun.Outcome, StartedAt: startedAt}
		run.Artifacts = make([]domain.AgentRunArtifact, 0, len(view.LatestAgentRun.Artifacts))
		for _, artifact := range view.LatestAgentRun.Artifacts {
			run.Artifacts = append(run.Artifacts, domain.AgentRunArtifact{ArtifactRefID: domain.ArtifactRefID(artifact.ArtifactRefID), Role: artifact.Role, Ordinal: artifact.Ordinal})
		}
		if view.LatestAgentRun.CompletedAt != nil {
			completedAt, parseErr := parseStamp(*view.LatestAgentRun.CompletedAt)
			if parseErr != nil {
				return domain.Change{}, parseErr
			}
			run.CompletedAt = &completedAt
		}
		change.LatestAgentRun = &run
	}
	return change, nil
}

func readChangeReceipt(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (*changeReceipt, error) {
	var receipt changeReceipt
	err := queryer.QueryRowContext(ctx, `SELECT operation, project_id, change_id, request_fingerprint, request_payload, status_code, response_body FROM t_change_command_receipts WHERE idempotency_key = ?`, key).Scan(&receipt.Operation, &receipt.ProjectID, &receipt.ChangeID, &receipt.RequestFingerprint, &receipt.RequestPayload, &receipt.StatusCode, &receipt.ResponseBody)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read change receipt: %w", err)
	}
	return &receipt, nil
}

func insertChangeReceipt(ctx context.Context, tx *sql.Tx, key, operation string, change domain.Change, intent, fingerprint string, status int, now time.Time) error {
	body, err := json.Marshal(changeReceiptPayload{Change: receiptChangeViewFor(change)})
	if err != nil {
		return fmt.Errorf("encode change receipt: %w", err)
	}
	body = append(body, '\n')
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_change_command_receipts (idempotency_key, operation, project_id, change_id, request_fingerprint, request_payload, status_code, response_body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, key, operation, change.ProjectID, change.ID, fingerprint, intent, status, string(body), stamp(now)); err != nil {
		return fmt.Errorf("insert change receipt: %w", err)
	}
	return nil
}

func replayChange(receipt *changeReceipt) (domain.Change, error) {
	payload, err := decodeChangeReceiptPayload(receipt.ResponseBody)
	if err != nil {
		return domain.Change{}, err
	}
	change, err := changeFromReceiptView(payload.Change)
	if err != nil {
		return domain.Change{}, err
	}
	if change.ID == "" || string(change.ID) != receipt.ChangeID || string(change.ProjectID) != receipt.ProjectID {
		return domain.Change{}, fmt.Errorf("change receipt response does not match change: %w", domain.ErrInternal)
	}
	if err := change.Validate(); err != nil {
		return domain.Change{}, fmt.Errorf("validate change receipt: %w", domain.ErrInternal)
	}
	return change, nil
}

func decodeChangeReceiptPayload(body string) (changeReceiptPayload, error) {
	var payload changeReceiptPayload
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, fmt.Errorf("decode change receipt response: %w", domain.ErrInternal)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return payload, fmt.Errorf("decode change receipt response: %w", domain.ErrInternal)
		}
		return payload, fmt.Errorf("decode change receipt response: %w", domain.ErrInternal)
	}
	return payload, nil
}

func ensureArtifact(ctx context.Context, tx *sql.Tx, identity domain.ArtifactIdentity, mediaType string, now time.Time) (domain.Artifact, error) {
	if err := identity.Validate(); err != nil {
		return domain.Artifact{}, err
	}
	var artifact domain.Artifact
	var created string
	err := tx.QueryRowContext(ctx, `SELECT artifact_id, sha256, byte_length, media_type, created_at FROM t_artifacts WHERE sha256 = ? AND byte_length = ?`, identity.SHA256, identity.ByteLength).Scan(&artifact.ID, &artifact.Identity.SHA256, &artifact.Identity.ByteLength, &artifact.MediaType, &created)
	if err == nil {
		artifact.CreatedAt, err = parseStamp(created)
		return artifact, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Artifact{}, fmt.Errorf("read existing artifact record: %w", err)
	}
	artifact = domain.Artifact{ID: domain.ArtifactID(id.New()), Identity: identity, MediaType: mediaType, CreatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifacts (artifact_id, sha256, byte_length, media_type, created_at) VALUES (?, ?, ?, ?, ?)`, artifact.ID, identity.SHA256, identity.ByteLength, mediaType, stamp(now)); err != nil {
		if existingErr := tx.QueryRowContext(ctx, `SELECT artifact_id, sha256, byte_length, media_type, created_at FROM t_artifacts WHERE sha256 = ? AND byte_length = ?`, identity.SHA256, identity.ByteLength).Scan(&artifact.ID, &artifact.Identity.SHA256, &artifact.Identity.ByteLength, &artifact.MediaType, &created); existingErr == nil {
			artifact.CreatedAt, existingErr = parseStamp(created)
			return artifact, existingErr
		}
		return domain.Artifact{}, fmt.Errorf("insert artifact record: %w", err)
	}
	return artifact, nil
}

func insertAgentRun(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, changeID domain.ChangeID, stage domain.LifecycleStage, started time.Time, artifacts []domain.AgentRunArtifact) (domain.AgentRun, error) {
	var max sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM t_agent_runs WHERE change_id = ? AND stage = ?`, changeID, stage).Scan(&max); err != nil {
		return domain.AgentRun{}, fmt.Errorf("read agent run attempt: %w", err)
	}
	attempt := 1
	if max.Valid {
		attempt = int(max.Int64) + 1
	}
	run := domain.AgentRun{ID: domain.AgentRunID(id.New()), ChangeID: changeID, Stage: stage, Attempt: attempt, Status: domain.AgentRunStatusRunning, StartedAt: started}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_agent_runs (agent_run_id, project_id, change_id, stage, attempt, status, outcome, started_at) SELECT ?, ?, ?, ?, ?, ?, '', ?`, run.ID, projectID, changeID, stage, attempt, run.Status, stamp(started)); err != nil {
		return domain.AgentRun{}, fmt.Errorf("insert agent run: %w", err)
	}
	if err := insertAgentRunArtifacts(ctx, tx, run.ID, artifacts); err != nil {
		return domain.AgentRun{}, err
	}
	run.Artifacts = append(run.Artifacts, artifacts...)
	return run, nil
}

func insertAgentRunArtifacts(ctx context.Context, tx *sql.Tx, runID domain.AgentRunID, artifacts []domain.AgentRunArtifact) error {
	for _, artifact := range artifacts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_agent_run_artifacts (agent_run_id, artifact_ref_id, role, ordinal) VALUES (?, ?, ?, ?)`, runID, artifact.ArtifactRefID, artifact.Role, artifact.Ordinal); err != nil {
			return fmt.Errorf("insert agent run artifact: %w", err)
		}
	}
	return nil
}

func insertArtifactInputs(ctx context.Context, tx *sql.Tx, change domain.Change, inputs []work.AgentRunArtifactInput, created time.Time) ([]domain.AgentRunArtifact, error) {
	artifacts := make([]domain.AgentRunArtifact, 0, len(inputs))
	for _, input := range inputs {
		artifact, err := ensureArtifact(ctx, tx, input.Identity, input.MediaType, created)
		if err != nil {
			return nil, err
		}
		ref := domain.ArtifactRef{ID: domain.ArtifactRefID(id.New()), ChangeID: change.ID, ArtifactID: artifact.ID, Role: input.Role, Ordinal: input.Ordinal}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifact_refs (artifact_ref_id, project_id, change_id, artifact_id, role, ordinal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, ref.ID, change.ProjectID, ref.ChangeID, ref.ArtifactID, ref.Role, ref.Ordinal, stamp(created)); err != nil {
			return nil, fmt.Errorf("insert agent run artifact reference: %w", err)
		}
		artifacts = append(artifacts, domain.AgentRunArtifact{ArtifactRefID: ref.ID, Role: input.Role, Ordinal: input.Ordinal})
	}
	return artifacts, nil
}

func validateRunArtifactRoles(artifacts []domain.AgentRunArtifact, allowed ...string) error {
	allowedRoles := make(map[string]struct{}, len(allowed))
	for _, role := range allowed {
		allowedRoles[role] = struct{}{}
	}
	for _, artifact := range artifacts {
		if err := artifact.Validate(); err != nil {
			return err
		}
		if _, ok := allowedRoles[artifact.Role]; !ok {
			return fmt.Errorf("%w: agent run artifact role is not allowed for this operation", domain.ErrInvalidRequest)
		}
	}
	return nil
}

func validateArtifactInputs(inputs []work.AgentRunArtifactInput, allowed string) error {
	for _, input := range inputs {
		if err := input.Identity.Validate(); err != nil {
			return err
		}
		if input.MediaType == "" || input.Role != allowed || input.Ordinal < 0 {
			return fmt.Errorf("%w: agent run artifact input is invalid", domain.ErrInvalidRequest)
		}
	}
	return nil
}

type sqlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type sqlQueryContext interface {
	sqlQueryer
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readAgentRun(ctx context.Context, queryer sqlQueryContext, runID domain.AgentRunID) (domain.AgentRun, error) {
	var run domain.AgentRun
	var started, completed sql.NullString
	err := queryer.QueryRowContext(ctx, `SELECT agent_run_id, change_id, stage, attempt, status, outcome, started_at, completed_at FROM t_agent_runs WHERE agent_run_id = ?`, runID).Scan(&run.ID, &run.ChangeID, &run.Stage, &run.Attempt, &run.Status, &run.Outcome, &started, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AgentRun{}, domain.ErrChangeNotFound
	}
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("read agent run: %w", err)
	}
	run.StartedAt, err = parseStamp(started.String)
	if err != nil {
		return domain.AgentRun{}, err
	}
	if completed.Valid {
		value, parseErr := parseStamp(completed.String)
		if parseErr != nil {
			return domain.AgentRun{}, parseErr
		}
		run.CompletedAt = &value
	}
	rows, err := queryer.QueryContext(ctx, `SELECT artifact_ref_id, role, ordinal FROM t_agent_run_artifacts WHERE agent_run_id = ? ORDER BY CASE role WHEN 'input' THEN 0 WHEN 'output' THEN 1 ELSE 2 END, ordinal`, run.ID)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("read agent run artifacts: %w", err)
	}
	defer rows.Close()
	run.Artifacts = make([]domain.AgentRunArtifact, 0)
	for rows.Next() {
		var artifact domain.AgentRunArtifact
		if err := rows.Scan(&artifact.ArtifactRefID, &artifact.Role, &artifact.Ordinal); err != nil {
			return domain.AgentRun{}, fmt.Errorf("scan agent run artifact: %w", err)
		}
		run.Artifacts = append(run.Artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return domain.AgentRun{}, fmt.Errorf("read agent run artifacts: %w", err)
	}
	return run, nil
}

func readChange(ctx context.Context, queryer sqlQueryContext, changeID domain.ChangeID) (domain.Change, error) {
	var change domain.Change
	var created, updated string
	err := queryer.QueryRowContext(ctx, `SELECT change_id, project_id, repository_root, stage, status, version, base_revision, intent_artifact_ref_id, created_at, updated_at FROM t_changes WHERE change_id = ?`, changeID).Scan(&change.ID, &change.ProjectID, &change.RepositoryRoot, &change.Stage, &change.Status, &change.Version, &change.BaseRevision, &change.Intent.ID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Change{}, domain.ErrChangeNotFound
	}
	if err != nil {
		return domain.Change{}, fmt.Errorf("read change: %w", err)
	}
	change.CreatedAt, err = parseStamp(created)
	if err != nil {
		return domain.Change{}, err
	}
	change.UpdatedAt, err = parseStamp(updated)
	if err != nil {
		return domain.Change{}, err
	}
	if err := queryer.QueryRowContext(ctx, `SELECT change_id, artifact_id, role, ordinal FROM t_artifact_refs WHERE artifact_ref_id = ? AND change_id = ? AND project_id = ?`, change.Intent.ID, change.ID, change.ProjectID).Scan(&change.Intent.ChangeID, &change.Intent.ArtifactID, &change.Intent.Role, &change.Intent.Ordinal); err != nil {
		return domain.Change{}, fmt.Errorf("read change intent reference: %w", err)
	}
	var latest domain.AgentRunID
	err = queryer.QueryRowContext(ctx, `SELECT agent_run_id FROM t_agent_runs WHERE change_id = ? ORDER BY started_at DESC, agent_run_id DESC LIMIT 1`, change.ID).Scan(&latest)
	if err == nil {
		run, runErr := readAgentRun(ctx, queryer, latest)
		if runErr != nil {
			return domain.Change{}, runErr
		}
		change.LatestAgentRun = &run
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.Change{}, fmt.Errorf("read latest agent run: %w", err)
	}
	return change, nil
}

func updateChangeStatus(ctx context.Context, tx *sql.Tx, old, next domain.Change, updatedAt time.Time) error {
	result, err := tx.ExecContext(ctx, `UPDATE t_changes SET status = ?, version = ?, updated_at = ? WHERE change_id = ? AND version = ?`, next.Status, next.Version, stamp(updatedAt), old.ID, old.Version)
	if err != nil {
		return fmt.Errorf("update change status: %w", err)
	}
	return checkVersionUpdate(result)
}

func updateChangeStage(ctx context.Context, tx *sql.Tx, old, next domain.Change, updatedAt time.Time) error {
	result, err := tx.ExecContext(ctx, `UPDATE t_changes SET stage = ?, status = ?, version = ?, updated_at = ? WHERE change_id = ? AND version = ?`, next.Stage, next.Status, next.Version, stamp(updatedAt), old.ID, old.Version)
	if err != nil {
		return fmt.Errorf("update change stage: %w", err)
	}
	return checkVersionUpdate(result)
}

func checkVersionUpdate(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check change version update: %w", err)
	}
	if count != 1 {
		return domain.ErrChangeVersionConflict
	}
	return nil
}

func isCurrentAgentRun(ctx context.Context, tx *sql.Tx, change domain.Change, run domain.AgentRun) (bool, error) {
	if run.Stage != change.Stage {
		return false, nil
	}
	var latestAttempt sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM t_agent_runs WHERE change_id = ? AND stage = ?`, change.ID, change.Stage).Scan(&latestAttempt); err != nil {
		return false, fmt.Errorf("read current agent run attempt: %w", err)
	}
	return latestAttempt.Valid && int(latestAttempt.Int64) == run.Attempt, nil
}

func agentRunArtifactRefIDs(artifacts []domain.AgentRunArtifact) []domain.ArtifactRefID {
	if len(artifacts) == 0 {
		return nil
	}
	refs := make([]domain.ArtifactRefID, 0, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, artifact.ArtifactRefID)
	}
	return refs
}

func (s *Store) insertEvent(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, changeID domain.ChangeID, eventType, actor string, occurred time.Time, runID *domain.AgentRunID, decisionID *domain.HumanDecisionID, refs []domain.ArtifactRefID) error {
	return insertEventTx(ctx, tx, projectID, changeID, eventType, actor, occurred, runID, decisionID, refs)
}

func insertEventTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, changeID domain.ChangeID, eventType, actor string, occurred time.Time, runID *domain.AgentRunID, decisionID *domain.HumanDecisionID, refs []domain.ArtifactRefID) error {
	sequence, err := nextChangeSequence(ctx, tx, changeID)
	if err != nil {
		return err
	}
	eventID := id.New()
	var runValue, decisionValue any
	if runID != nil {
		runValue = string(*runID)
	}
	if decisionID != nil {
		decisionValue = string(*decisionID)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_project_events (event_id, project_id, type, occurred_at, event_sequence, change_id, agent_run_id, decision_id, actor) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, eventID, projectID, eventType, stamp(occurred), sequence, changeID, runValue, decisionValue, actor); err != nil {
		return fmt.Errorf("insert change event: %w", err)
	}
	for ordinal, refID := range refs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_event_artifacts (event_id, artifact_ref_id, ordinal) VALUES (?, ?, ?)`, eventID, refID, ordinal); err != nil {
			return fmt.Errorf("insert event artifact reference: %w", err)
		}
	}
	return nil
}

func nextChangeSequence(ctx context.Context, tx *sql.Tx, changeID domain.ChangeID) (int, error) {
	var sequence sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(event_sequence) FROM t_project_events WHERE change_id = ?`, changeID).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("read change event sequence: %w", err)
	}
	if !sequence.Valid {
		return 1, nil
	}
	return int(sequence.Int64) + 1, nil
}

func listEventArtifactRefs(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, eventID string) ([]domain.ArtifactRefID, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT artifact_ref_id FROM t_event_artifacts WHERE event_id = ? ORDER BY ordinal`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list event artifact references: %w", err)
	}
	defer rows.Close()
	refs := make([]domain.ArtifactRefID, 0)
	for rows.Next() {
		var refID domain.ArtifactRefID
		if err := rows.Scan(&refID); err != nil {
			return nil, err
		}
		refs = append(refs, refID)
	}
	return refs, rows.Err()
}

func validateCreateRecord(record work.ChangeCreateRecord) error {
	if err := record.Intent.Validate(); err != nil {
		return err
	}
	if err := record.IntentIdentity.Validate(); err != nil {
		return err
	}
	if record.IntentIdentity != domain.NewArtifactIdentity([]byte(record.Intent.Original)) {
		return fmt.Errorf("create change record: intent artifact identity differs: %w", domain.ErrInvalidRequest)
	}
	if strings.TrimSpace(record.IdempotencyKey) == "" || record.Project.Identity.ProjectID.Validate() != nil || record.Project.Binding.Validate() != nil || record.Snapshot.RepositoryRoot != record.Project.Binding.Root || record.Snapshot.BaseRevision == "" {
		return fmt.Errorf("create change record: %w", domain.ErrInvalidRequest)
	}
	return nil
}

func createFingerprint(record work.ChangeCreateRecord) string {
	return fingerprint("create", string(record.Project.Identity.ProjectID), record.Project.Binding.Root, record.Snapshot.BaseRevision, record.Intent.Original, record.IntentIdentity.SHA256)
}

func commandFingerprint(operation string, changeID domain.ChangeID, expected domain.ChangeVersion, parts ...string) string {
	return fingerprint(append([]string{operation, string(changeID), fmt.Sprintf("%d", expected)}, parts...)...)
}

func fingerprint(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func stamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseStamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid timestamp", domain.ErrInternal)
	}
	return parsed.UTC(), nil
}

func advanceStage(stage domain.LifecycleStage) (domain.LifecycleStage, bool) {
	stages := []domain.LifecycleStage{domain.LifecycleStageIntent, domain.LifecycleStageUnderstand, domain.LifecycleStageDesign, domain.LifecycleStagePlan, domain.LifecycleStageTicketize, domain.LifecycleStageExecute, domain.LifecycleStageVerify, domain.LifecycleStageFinalVerify}
	for index, current := range stages {
		if current == stage && index+1 < len(stages) {
			return stages[index+1], true
		}
	}
	return "", false
}

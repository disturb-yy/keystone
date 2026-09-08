package workstore

import "github.com/disturb-yy/keystone/internal/infrastructure/migration"

// executionSchemaSQL 为 Ticket 09 建立执行会话、Worktree、调度和证据事实。
// 既有 AgentRun/Lease 表继续作为 Worker authority；本迁移只扩展其关联字段。
const executionSchemaSQL = `
CREATE TABLE t_workspace_provisioning_intents (
    intent_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    repository_root TEXT NOT NULL,
    workspace_path TEXT NOT NULL,
    base_revision TEXT NOT NULL,
    branch TEXT NOT NULL,
    request_key TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'provisioned', 'human_required')),
    failure_code TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(change_id),
    UNIQUE(request_key),
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_execution_sessions (
    session_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    request_key TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('waiting', 'running', 'human_required', 'cancelled', 'completed')),
    branch TEXT NOT NULL,
    base_revision TEXT NOT NULL,
    input_revision TEXT NOT NULL,
    workspace_id TEXT NOT NULL DEFAULT '',
    workspace_path TEXT NOT NULL DEFAULT '',
    instruction TEXT NOT NULL DEFAULT '',
    dispatch_sequence INTEGER NOT NULL CHECK (dispatch_sequence >= 1),
    current_epoch_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(change_id),
    UNIQUE(project_id, dispatch_sequence),
    UNIQUE(request_key),
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_workspaces (
    workspace_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    repository_root TEXT NOT NULL,
    workspace_path TEXT NOT NULL,
    physical_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    base_revision TEXT NOT NULL,
    input_revision TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(change_id),
    UNIQUE(workspace_id, change_id),
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_execution_dispatch_epochs (
    epoch_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL REFERENCES t_execution_sessions(session_id),
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    status TEXT NOT NULL CHECK (status IN ('queued', 'active', 'fenced', 'completed')),
    created_at TEXT NOT NULL,
    fenced_at TEXT,
    UNIQUE(session_id, sequence)
);

CREATE TABLE t_execution_authorizations (
    authorization_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL REFERENCES t_execution_sessions(session_id),
    epoch_id TEXT NOT NULL REFERENCES t_execution_dispatch_epochs(epoch_id),
    graph_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    execution_mode TEXT NOT NULL CHECK (execution_mode = 'edit'),
    runtime TEXT NOT NULL CHECK (runtime = 'codex'),
    instruction TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    timeout_seconds INTEGER NOT NULL CHECK (timeout_seconds = 1800),
    workspace_id TEXT NOT NULL,
    branch TEXT NOT NULL,
    input_revision TEXT NOT NULL,
    decision_digest TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('authorized', 'assigned', 'claimed', 'completed', 'fenced', 'human_required')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
	UNIQUE(epoch_id, ticket_id),
	FOREIGN KEY(graph_id, ticket_id) REFERENCES t_tickets(graph_id, ticket_id)
);

CREATE TABLE t_ticket_execution_states (
    graph_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 1),
    state TEXT NOT NULL CHECK (state IN ('pending', 'assigned', 'succeeded', 'human_required')),
    authorization_id TEXT,
    agent_run_id TEXT,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(graph_id, ticket_id),
    UNIQUE(authorization_id),
    UNIQUE(agent_run_id),
    FOREIGN KEY(graph_id, ticket_id) REFERENCES t_tickets(graph_id, ticket_id),
    FOREIGN KEY(authorization_id) REFERENCES t_execution_authorizations(authorization_id),
    FOREIGN KEY(agent_run_id) REFERENCES t_agent_runs(agent_run_id)
);

CREATE TABLE t_workspace_snapshots (
    snapshot_id TEXT PRIMARY KEY NOT NULL,
    workspace_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES t_execution_sessions(session_id),
    epoch_id TEXT NOT NULL REFERENCES t_execution_dispatch_epochs(epoch_id),
    ticket_id TEXT NOT NULL,
    phase TEXT NOT NULL CHECK (phase IN ('pre', 'post')),
    input_revision TEXT NOT NULL,
    head_revision TEXT NOT NULL,
    branch TEXT NOT NULL,
    changed_files_json TEXT NOT NULL,
    diff_sha256 TEXT NOT NULL,
    diff_bytes INTEGER NOT NULL CHECK (diff_bytes >= 0),
    has_untracked INTEGER NOT NULL CHECK (has_untracked IN (0, 1)),
    created_at TEXT NOT NULL,
    UNIQUE(session_id, ticket_id, phase),
    FOREIGN KEY(ticket_id) REFERENCES t_tickets(ticket_id)
);

CREATE TABLE t_ticket_execution_evidence (
    evidence_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL REFERENCES t_execution_sessions(session_id),
    epoch_id TEXT NOT NULL REFERENCES t_execution_dispatch_epochs(epoch_id),
    ticket_id TEXT NOT NULL,
    agent_run_id TEXT NOT NULL REFERENCES t_agent_runs(agent_run_id),
    lease_id TEXT NOT NULL REFERENCES t_worker_leases(lease_id),
    input_revision TEXT NOT NULL,
    pre_snapshot_id TEXT NOT NULL REFERENCES t_workspace_snapshots(snapshot_id),
    post_snapshot_id TEXT NOT NULL REFERENCES t_workspace_snapshots(snapshot_id),
    complete_diff_artifact_ref_id TEXT,
    ticket_delta_artifact_ref_id TEXT,
    complete_diff_sha256 TEXT NOT NULL,
    changed_files_json TEXT NOT NULL,
	outcome TEXT NOT NULL CHECK (outcome IN ('succeeded', 'failed', 'human_required')),
	created_at TEXT NOT NULL,
	UNIQUE(agent_run_id),
	FOREIGN KEY(ticket_id) REFERENCES t_tickets(ticket_id)
);

ALTER TABLE t_agent_runs ADD COLUMN ticket_id TEXT NOT NULL DEFAULT '';
ALTER TABLE t_agent_runs ADD COLUMN authorization_id TEXT NOT NULL DEFAULT '';
ALTER TABLE t_worker_leases ADD COLUMN runtime_claim_id TEXT NOT NULL DEFAULT '';
ALTER TABLE t_worker_leases ADD COLUMN claim_state TEXT NOT NULL DEFAULT '' CHECK (claim_state IN ('', 'claimed', 'fenced'));
ALTER TABLE t_worker_leases ADD COLUMN execution_mode TEXT NOT NULL DEFAULT '' CHECK (execution_mode IN ('', 'edit', 'inspect'));
ALTER TABLE t_worker_leases ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE t_worker_leases ADD COLUMN envelope_digest TEXT NOT NULL DEFAULT '';

CREATE INDEX ix_execution_sessions_queue ON t_execution_sessions(project_id, status, dispatch_sequence);
CREATE INDEX ix_execution_epochs_session ON t_execution_dispatch_epochs(session_id, sequence);
CREATE INDEX ix_execution_authorizations_queue ON t_execution_authorizations(status, session_id, ticket_id);
CREATE UNIQUE INDEX ux_execution_ticket_active_authorization ON t_execution_authorizations(session_id, ticket_id) WHERE status IN ('authorized', 'assigned', 'claimed');
CREATE INDEX ix_execution_snapshots_ticket ON t_workspace_snapshots(session_id, ticket_id, phase);
CREATE INDEX ix_execution_evidence_ticket ON t_ticket_execution_evidence(session_id, ticket_id);
CREATE UNIQUE INDEX ux_execution_success_evidence ON t_ticket_execution_evidence(session_id, ticket_id) WHERE outcome = 'succeeded';

CREATE TRIGGER tr_execution_sessions_no_delete
BEFORE DELETE ON t_execution_sessions
BEGIN
    SELECT RAISE(ABORT, 'execution sessions are append-only');
END;
CREATE TRIGGER tr_execution_epochs_no_delete
BEFORE DELETE ON t_execution_dispatch_epochs
BEGIN
    SELECT RAISE(ABORT, 'execution dispatch epochs are append-only');
END;
CREATE TRIGGER tr_execution_authorizations_no_delete
BEFORE DELETE ON t_execution_authorizations
BEGIN
    SELECT RAISE(ABORT, 'execution authorizations are append-only');
END;
CREATE TRIGGER tr_execution_snapshots_no_update
BEFORE UPDATE ON t_workspace_snapshots
BEGIN
    SELECT RAISE(ABORT, 'workspace snapshots are immutable');
END;
CREATE TRIGGER tr_execution_snapshots_no_delete
BEFORE DELETE ON t_workspace_snapshots
BEGIN
    SELECT RAISE(ABORT, 'workspace snapshots are append-only');
END;
CREATE TRIGGER tr_execution_evidence_no_update
BEFORE UPDATE ON t_ticket_execution_evidence
BEGIN
    SELECT RAISE(ABORT, 'ticket execution evidence is immutable');
END;
CREATE TRIGGER tr_execution_evidence_no_delete
BEFORE DELETE ON t_ticket_execution_evidence
BEGIN
    SELECT RAISE(ABORT, 'ticket execution evidence is append-only');
END;
CREATE TRIGGER tr_execution_success_ticket_shape
BEFORE INSERT ON t_ticket_execution_evidence
WHEN NEW.outcome = 'succeeded' AND EXISTS (
    SELECT 1 FROM t_ticket_execution_evidence
    WHERE session_id = NEW.session_id AND ticket_id = NEW.ticket_id AND outcome = 'succeeded'
)
BEGIN
    SELECT RAISE(ABORT, 'ticket already has successful execution evidence');
END;`

func executionMigration() migration.Migration {
	return migration.Migration{Version: 7, Name: "create_execution_authority", SQL: executionSchemaSQL}
}

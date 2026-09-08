package workstore

import "github.com/disturb-yy/keystone/internal/infrastructure/migration"

// verificationSchemaSQL 为 M8 增加策略快照、验证证据和 Commit 恢复锚点。
// 这些表只追加新的事实，不重写 Ticket 09 的表和 Migration checksum。
const verificationSchemaSQL = `
CREATE TABLE t_verification_policy_snapshots (
    snapshot_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    execution_session_id TEXT NOT NULL UNIQUE REFERENCES t_execution_sessions(session_id),
    base_revision TEXT NOT NULL,
    verification_digest TEXT NOT NULL,
    commit_template_digest TEXT NOT NULL,
    commands_json TEXT NOT NULL,
    commit_template TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_verification_intents (
    intent_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL DEFAULT '',
    final INTEGER NOT NULL CHECK (final IN (0, 1)),
    policy_snapshot_id TEXT REFERENCES t_verification_policy_snapshots(snapshot_id),
    policy_digest TEXT NOT NULL,
    input_revision TEXT NOT NULL,
    candidate_tree_identity TEXT NOT NULL,
    request_key TEXT NOT NULL UNIQUE,
    request_digest TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'pass', 'fail', 'human_required', 'unavailable')),
    attempt INTEGER NOT NULL CHECK (attempt >= 1),
    agent_run_id TEXT REFERENCES t_agent_runs(agent_run_id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(change_id, ticket_id, final, attempt),
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);
CREATE UNIQUE INDEX ux_verification_active_scope
    ON t_verification_intents(change_id, ticket_id, final)
    WHERE status IN ('pending', 'running');

CREATE TABLE t_verification_snapshots (
    snapshot_id TEXT PRIMARY KEY NOT NULL,
    intent_id TEXT NOT NULL REFERENCES t_verification_intents(intent_id),
    phase TEXT NOT NULL CHECK (phase IN ('before', 'after')),
    input_revision TEXT NOT NULL,
    head_revision TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT '',
    changed_files_json TEXT NOT NULL DEFAULT '[]',
    diff_sha256 TEXT NOT NULL DEFAULT '',
    diff_bytes INTEGER NOT NULL DEFAULT 0 CHECK (diff_bytes >= 0),
    has_untracked INTEGER NOT NULL DEFAULT 0 CHECK (has_untracked IN (0, 1)),
    tree_identity TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(intent_id, phase)
);

CREATE TABLE t_verification_command_results (
    intent_id TEXT NOT NULL REFERENCES t_verification_intents(intent_id),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 1),
    name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('passed', 'failed', 'timed_out', 'not_run')),
    exit_code INTEGER,
    stdout_sha256 TEXT NOT NULL DEFAULT '',
    stdout_bytes INTEGER NOT NULL DEFAULT 0 CHECK (stdout_bytes >= 0),
    stdout_truncated INTEGER NOT NULL DEFAULT 0 CHECK (stdout_truncated IN (0, 1)),
    stderr_sha256 TEXT NOT NULL DEFAULT '',
    stderr_bytes INTEGER NOT NULL DEFAULT 0 CHECK (stderr_bytes >= 0),
    stderr_truncated INTEGER NOT NULL DEFAULT 0 CHECK (stderr_truncated IN (0, 1)),
    PRIMARY KEY(intent_id, ordinal)
);

CREATE TABLE t_verification_criterion_results (
    intent_id TEXT NOT NULL REFERENCES t_verification_intents(intent_id),
    ticket_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 1),
    text_sha256 TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('pass', 'fail', 'human_required', 'not_run')),
    evidence_ids_json TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY(intent_id, ticket_id, ordinal)
);

CREATE TABLE t_verification_evidence (
    evidence_id TEXT PRIMARY KEY NOT NULL,
    intent_id TEXT NOT NULL UNIQUE REFERENCES t_verification_intents(intent_id),
    agent_run_id TEXT REFERENCES t_agent_runs(agent_run_id),
    outcome TEXT NOT NULL CHECK (outcome IN ('pass', 'fail', 'human_required', 'unavailable')),
    input_revision TEXT NOT NULL,
    candidate_tree_identity TEXT NOT NULL,
    report_digest TEXT NOT NULL,
    review_summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE t_verification_receipts (
    idempotency_key TEXT PRIMARY KEY NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN ('verify', 'final_verify')),
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL DEFAULT '',
    request_fingerprint TEXT NOT NULL,
    response_body TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_commit_intents (
    intent_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL,
    request_key TEXT NOT NULL UNIQUE,
    request_digest TEXT NOT NULL,
    expected_parent TEXT NOT NULL,
    candidate_tree_identity TEXT NOT NULL,
    verification_evidence_id TEXT NOT NULL REFERENCES t_verification_evidence(evidence_id),
    commit_template_digest TEXT NOT NULL,
    keystone_commit_id TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'committed', 'human_required')),
    git_oid TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    failure_code TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_keystone_commits (
    keystone_commit_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL UNIQUE,
    parent_revision TEXT NOT NULL,
    after_revision TEXT NOT NULL,
    tree_identity TEXT NOT NULL,
    git_oid TEXT NOT NULL UNIQUE,
    verification_evidence_id TEXT NOT NULL REFERENCES t_verification_evidence(evidence_id),
    commit_template_digest TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_candidate_revisions (
    candidate_revision_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL UNIQUE,
    revision TEXT NOT NULL,
    tree_identity TEXT NOT NULL,
    verification_evidence_id TEXT NOT NULL REFERENCES t_verification_evidence(evidence_id),
    created_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE TABLE t_governance_events (
    event_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}',
    occurred_at TEXT NOT NULL,
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id)
);

CREATE INDEX ix_verification_intents_change ON t_verification_intents(change_id, created_at, intent_id);
CREATE INDEX ix_verification_snapshots_intent ON t_verification_snapshots(intent_id, phase);
CREATE INDEX ix_verification_commands_intent ON t_verification_command_results(intent_id, ordinal);
CREATE INDEX ix_verification_criteria_intent ON t_verification_criterion_results(intent_id, ticket_id, ordinal);
CREATE INDEX ix_commit_intents_change ON t_commit_intents(change_id, ticket_id, created_at);
CREATE INDEX ix_governance_events_change ON t_governance_events(change_id, occurred_at, event_id);

CREATE TRIGGER tr_verification_evidence_no_update
BEFORE UPDATE ON t_verification_evidence
BEGIN
    SELECT RAISE(ABORT, 'verification evidence is immutable');
END;
CREATE TRIGGER tr_verification_evidence_no_delete
BEFORE DELETE ON t_verification_evidence
BEGIN
    SELECT RAISE(ABORT, 'verification evidence is append-only');
END;
CREATE TRIGGER tr_verification_policy_no_update
BEFORE UPDATE ON t_verification_policy_snapshots
BEGIN
    SELECT RAISE(ABORT, 'verification policy snapshots are immutable');
END;
CREATE TRIGGER tr_verification_policy_no_delete
BEFORE DELETE ON t_verification_policy_snapshots
BEGIN
    SELECT RAISE(ABORT, 'verification policy snapshots are append-only');
END;
CREATE TRIGGER tr_keystone_commits_no_update
BEFORE UPDATE ON t_keystone_commits
BEGIN
    SELECT RAISE(ABORT, 'keystone commits are immutable');
END;
CREATE TRIGGER tr_keystone_commits_no_delete
BEFORE DELETE ON t_keystone_commits
BEGIN
    SELECT RAISE(ABORT, 'keystone commits are append-only');
END;
CREATE TRIGGER tr_candidate_revisions_no_update
BEFORE UPDATE ON t_candidate_revisions
BEGIN
    SELECT RAISE(ABORT, 'candidate revisions are immutable');
END;
CREATE TRIGGER tr_candidate_revisions_no_delete
BEFORE DELETE ON t_candidate_revisions
BEGIN
    SELECT RAISE(ABORT, 'candidate revisions are append-only');
END;
CREATE TRIGGER tr_verification_snapshots_no_update
BEFORE UPDATE ON t_verification_snapshots
BEGIN
    SELECT RAISE(ABORT, 'verification snapshots are immutable');
END;
CREATE TRIGGER tr_verification_snapshots_no_delete
BEFORE DELETE ON t_verification_snapshots
BEGIN
    SELECT RAISE(ABORT, 'verification snapshots are append-only');
END;
CREATE TRIGGER tr_governance_events_no_update
BEFORE UPDATE ON t_governance_events
BEGIN
    SELECT RAISE(ABORT, 'governance events are immutable');
END;
CREATE TRIGGER tr_governance_events_no_delete
BEFORE DELETE ON t_governance_events
BEGIN
    SELECT RAISE(ABORT, 'governance events are append-only');
END;`

func verificationMigration() migration.Migration {
	return migration.Migration{Version: 8, Name: "create_verification_commit_authority", SQL: verificationSchemaSQL}
}

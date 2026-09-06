package workstore

import "github.com/disturb-yy/keystone/internal/infrastructure/migration"

const workerSchemaSQL = `
DROP TRIGGER IF EXISTS tr_change_events_no_update;
DROP TRIGGER IF EXISTS tr_change_events_no_delete;
DROP TRIGGER IF EXISTS tr_event_artifact_ownership;
DROP TRIGGER IF EXISTS tr_event_run_ownership;
DROP TRIGGER IF EXISTS tr_event_decision_ownership;
DROP TRIGGER IF EXISTS tr_event_aggregate_shape;
DROP TRIGGER IF EXISTS tr_event_run_shape;
DROP TRIGGER IF EXISTS tr_event_decision_shape;
DROP TRIGGER IF EXISTS tr_event_artifacts_no_update;
DROP TRIGGER IF EXISTS tr_event_artifacts_no_delete;
DROP INDEX IF EXISTS ux_project_initialized_event;
DROP INDEX IF EXISTS ix_events_change_sequence;

ALTER TABLE t_event_artifacts RENAME TO t_event_artifacts_v3;
ALTER TABLE t_project_events RENAME TO t_project_events_v3;

CREATE TABLE t_project_events_v4 (
    event_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    type TEXT NOT NULL CHECK (type IN ('ProjectInitialized', 'ChangeCreated', 'AgentRunStarted', 'AgentRunCompleted', 'AgentRunReportLate', 'StageAdvanced', 'ChangePaused', 'ChangeResumed', 'ChangeHumanRequired', 'HumanDecisionRecorded', 'ChangeCancelled')),
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
CREATE TABLE t_event_artifacts_v4 (
    event_id TEXT NOT NULL REFERENCES t_project_events_v4(event_id),
    artifact_ref_id TEXT NOT NULL REFERENCES t_artifact_refs(artifact_ref_id),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY(event_id, ordinal),
    UNIQUE(event_id, artifact_ref_id)
);
INSERT INTO t_project_events_v4 (event_id, project_id, type, occurred_at, event_sequence, change_id, agent_run_id, decision_id, actor)
SELECT event_id, project_id, type, occurred_at, event_sequence, change_id, agent_run_id, decision_id, actor
FROM t_project_events_v3;
INSERT INTO t_event_artifacts_v4 (event_id, artifact_ref_id, ordinal)
SELECT event_id, artifact_ref_id, ordinal
FROM t_event_artifacts_v3;
DROP TABLE t_event_artifacts_v3;
DROP TABLE t_project_events_v3;
ALTER TABLE t_project_events_v4 RENAME TO t_project_events;
ALTER TABLE t_event_artifacts_v4 RENAME TO t_event_artifacts;

CREATE UNIQUE INDEX ux_project_initialized_event
    ON t_project_events(project_id, type) WHERE change_id IS NULL;
CREATE INDEX ix_events_change_sequence ON t_project_events(change_id, event_sequence);

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
WHEN (NEW.type IN ('AgentRunStarted', 'AgentRunCompleted', 'AgentRunReportLate', 'StageAdvanced', 'ChangeHumanRequired') AND NEW.agent_run_id IS NULL)
  OR (NEW.type NOT IN ('AgentRunStarted', 'AgentRunCompleted', 'AgentRunReportLate', 'StageAdvanced', 'ChangeHumanRequired') AND NEW.agent_run_id IS NOT NULL)
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

CREATE TABLE t_worker_instances (
    worker_id TEXT PRIMARY KEY NOT NULL,
    protocol_version TEXT NOT NULL,
    capabilities_json TEXT NOT NULL,
    secret_sha256 TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'registered', 'revoked')),
    created_at TEXT NOT NULL,
    registered_at TEXT,
    last_heartbeat_at TEXT
);
CREATE TABLE t_worker_leases (
    lease_id TEXT PRIMARY KEY NOT NULL,
    agent_run_id TEXT NOT NULL UNIQUE REFERENCES t_agent_runs(agent_run_id),
    worker_id TEXT NOT NULL REFERENCES t_worker_instances(worker_id),
    attempt INTEGER NOT NULL CHECK (attempt >= 1),
    token_sha256 TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('active', 'expired', 'revoked', 'consumed')),
    expires_at TEXT NOT NULL,
    workspace_id TEXT NOT NULL DEFAULT '',
    workspace_path TEXT NOT NULL,
    runtime TEXT NOT NULL,
    instruction TEXT NOT NULL DEFAULT '',
    before_revision TEXT NOT NULL DEFAULT '',
    input_artifacts_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    consumed_at TEXT,
    report_digest TEXT,
    report_disposition TEXT,
    report_outcome TEXT
);
CREATE TABLE t_worker_reports (
    report_id TEXT PRIMARY KEY NOT NULL,
    report_digest TEXT NOT NULL UNIQUE,
    agent_run_id TEXT REFERENCES t_agent_runs(agent_run_id),
    lease_id TEXT REFERENCES t_worker_leases(lease_id),
    worker_id TEXT REFERENCES t_worker_instances(worker_id),
    disposition TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX ix_worker_instances_status ON t_worker_instances(status);
CREATE INDEX ix_worker_leases_worker_state ON t_worker_leases(worker_id, state, expires_at);
CREATE INDEX ix_worker_leases_run_state ON t_worker_leases(agent_run_id, state);
CREATE INDEX ix_worker_reports_run_created ON t_worker_reports(agent_run_id, created_at);

CREATE TRIGGER tr_worker_instances_no_delete
BEFORE DELETE ON t_worker_instances
BEGIN
    SELECT RAISE(ABORT, 'worker instances are append-only');
END;
CREATE TRIGGER tr_worker_reports_no_update
BEFORE UPDATE ON t_worker_reports
BEGIN
    SELECT RAISE(ABORT, 'worker reports are append-only');
END;
CREATE TRIGGER tr_worker_reports_no_delete
BEFORE DELETE ON t_worker_reports
BEGIN
    SELECT RAISE(ABORT, 'worker reports are append-only');
END;
CREATE TRIGGER tr_worker_lease_binding
BEFORE INSERT ON t_worker_leases
WHEN NOT EXISTS (
    SELECT 1 FROM t_agent_runs
    WHERE agent_run_id = NEW.agent_run_id AND attempt = NEW.attempt AND status = 'running'
)
BEGIN
    SELECT RAISE(ABORT, 'worker lease does not match running agent run');
END;
CREATE TRIGGER tr_worker_lease_no_delete
BEFORE DELETE ON t_worker_leases
BEGIN
    SELECT RAISE(ABORT, 'worker leases are append-only');
END;`

func workerMigration() migration.Migration {
	return migration.Migration{Version: 4, Name: "create_worker_runtime_authority", SQL: workerSchemaSQL}
}

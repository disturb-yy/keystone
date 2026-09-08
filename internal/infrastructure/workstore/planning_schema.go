package workstore

import "github.com/disturb-yy/keystone/internal/infrastructure/migration"

const planningSchemaSQL = `
ALTER TABLE t_artifact_refs ADD COLUMN kind TEXT NOT NULL DEFAULT '';
ALTER TABLE t_artifact_refs ADD COLUMN schema_version TEXT NOT NULL DEFAULT '';
ALTER TABLE t_artifact_refs ADD COLUMN summary TEXT NOT NULL DEFAULT '';
ALTER TABLE t_artifact_refs ADD COLUMN source_revision TEXT NOT NULL DEFAULT '';

ALTER TABLE t_agent_runs ADD COLUMN run_kind TEXT NOT NULL DEFAULT ''
    CHECK (run_kind IN ('', 'planning'));
ALTER TABLE t_agent_runs ADD COLUMN source_revision TEXT NOT NULL DEFAULT '';
ALTER TABLE t_worker_leases ADD COLUMN result_mode TEXT NOT NULL DEFAULT ''
    CHECK (result_mode IN ('', 'planning_candidate'));

CREATE UNIQUE INDEX ux_planning_running_change
    ON t_agent_runs(change_id) WHERE run_kind = 'planning' AND status = 'running';
CREATE INDEX ix_planning_recovery
    ON t_changes(status, stage, updated_at, change_id);

CREATE TABLE t_artifact_ref_links (
    artifact_ref_id TEXT NOT NULL REFERENCES t_artifact_refs(artifact_ref_id),
    linked_artifact_ref_id TEXT NOT NULL REFERENCES t_artifact_refs(artifact_ref_id),
    relation TEXT NOT NULL CHECK (relation IN ('input', 'raw_log')),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY(artifact_ref_id, relation, ordinal),
    UNIQUE(artifact_ref_id, relation, linked_artifact_ref_id)
);

CREATE TABLE t_planning_run_candidates (
    agent_run_id TEXT PRIMARY KEY NOT NULL REFERENCES t_agent_runs(agent_run_id),
    report_digest TEXT NOT NULL UNIQUE,
    outcome TEXT NOT NULL CHECK (outcome IN ('succeeded', 'failed')),
    exit_code INTEGER,
    after_revision TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    guard_findings_json TEXT NOT NULL DEFAULT '[]',
    candidate_truncated INTEGER NOT NULL DEFAULT 0 CHECK (candidate_truncated IN (0, 1)),
    received_at TEXT NOT NULL
);
CREATE TABLE t_planning_candidate_artifacts (
    agent_run_id TEXT NOT NULL REFERENCES t_planning_run_candidates(agent_run_id),
    artifact_ref_id TEXT NOT NULL REFERENCES t_artifact_refs(artifact_ref_id),
    kind TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY(agent_run_id, ordinal),
    UNIQUE(agent_run_id, artifact_ref_id)
);

CREATE TABLE t_planning_stage_commits (
    agent_run_id TEXT PRIMARY KEY NOT NULL REFERENCES t_agent_runs(agent_run_id),
    artifact_ref_id TEXT NOT NULL UNIQUE REFERENCES t_artifact_refs(artifact_ref_id),
    outcome TEXT NOT NULL CHECK (outcome IN ('succeeded', 'failed')),
    disposition TEXT NOT NULL CHECK (disposition IN ('committed', 'fenced')),
    completed_at TEXT NOT NULL
);

DROP TRIGGER tr_agent_run_artifact_role;
CREATE TRIGGER tr_agent_run_artifact_role
BEFORE INSERT ON t_agent_run_artifacts
WHEN NOT EXISTS (
    SELECT 1 FROM t_artifact_refs
    WHERE artifact_ref_id = NEW.artifact_ref_id
      AND (
          role = NEW.role
          OR (NEW.role = 'input' AND role IN ('change_intent', 'input', 'output'))
      )
)
BEGIN
    SELECT RAISE(ABORT, 'agent run artifact role does not match reference');
END;

CREATE TRIGGER tr_planning_run_identity_no_update
BEFORE UPDATE ON t_agent_runs
WHEN NEW.run_kind <> OLD.run_kind OR NEW.source_revision <> OLD.source_revision
BEGIN
    SELECT RAISE(ABORT, 'agent run planning identity is immutable');
END;
CREATE TRIGGER tr_planning_run_serial_start
BEFORE INSERT ON t_agent_runs
WHEN (NEW.run_kind = 'planning' AND EXISTS (
        SELECT 1 FROM t_agent_runs
        WHERE change_id = NEW.change_id AND status = 'running'
    ))
  OR (NEW.run_kind <> 'planning' AND EXISTS (
        SELECT 1 FROM t_agent_runs
        WHERE change_id = NEW.change_id AND run_kind = 'planning' AND status = 'running'
    ))
BEGIN
    SELECT RAISE(ABORT, 'planning agent runs are serial per change');
END;
CREATE TRIGGER tr_artifact_ref_links_no_update
BEFORE UPDATE ON t_artifact_ref_links
BEGIN
    SELECT RAISE(ABORT, 'artifact reference links are append-only');
END;
CREATE TRIGGER tr_artifact_ref_links_no_delete
BEFORE DELETE ON t_artifact_ref_links
BEGIN
    SELECT RAISE(ABORT, 'artifact reference links are append-only');
END;
CREATE TRIGGER tr_artifact_ref_link_ownership
BEFORE INSERT ON t_artifact_ref_links
WHEN NOT EXISTS (
    SELECT 1
    FROM t_artifact_refs source
    JOIN t_artifact_refs linked
      ON linked.artifact_ref_id = NEW.linked_artifact_ref_id
    WHERE source.artifact_ref_id = NEW.artifact_ref_id
      AND source.change_id = linked.change_id
      AND source.project_id = linked.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'artifact reference link belongs to another change');
END;
CREATE TRIGGER tr_planning_candidates_no_update
BEFORE UPDATE ON t_planning_run_candidates
BEGIN
    SELECT RAISE(ABORT, 'planning candidates are append-only');
END;
CREATE TRIGGER tr_planning_candidates_no_delete
BEFORE DELETE ON t_planning_run_candidates
BEGIN
    SELECT RAISE(ABORT, 'planning candidates are append-only');
END;
CREATE TRIGGER tr_planning_candidate_artifacts_no_update
BEFORE UPDATE ON t_planning_candidate_artifacts
BEGIN
    SELECT RAISE(ABORT, 'planning candidate artifacts are append-only');
END;
CREATE TRIGGER tr_planning_candidate_artifacts_no_delete
BEFORE DELETE ON t_planning_candidate_artifacts
BEGIN
    SELECT RAISE(ABORT, 'planning candidate artifacts are append-only');
END;
CREATE TRIGGER tr_planning_stage_commits_no_update
BEFORE UPDATE ON t_planning_stage_commits
BEGIN
    SELECT RAISE(ABORT, 'planning stage commits are append-only');
END;
CREATE TRIGGER tr_planning_stage_commits_no_delete
BEFORE DELETE ON t_planning_stage_commits
BEGIN
    SELECT RAISE(ABORT, 'planning stage commits are append-only');
END;
CREATE TRIGGER tr_planning_candidate_run_shape
BEFORE INSERT ON t_planning_run_candidates
WHEN NOT EXISTS (
    SELECT 1 FROM t_agent_runs
    WHERE agent_run_id = NEW.agent_run_id
      AND run_kind = 'planning'
      AND status = 'running'
)
BEGIN
    SELECT RAISE(ABORT, 'planning candidate requires a running planning run');
END;
CREATE TRIGGER tr_planning_commit_run_shape
BEFORE INSERT ON t_planning_stage_commits
WHEN NOT EXISTS (
    SELECT 1 FROM t_agent_runs
    WHERE agent_run_id = NEW.agent_run_id
      AND run_kind = 'planning'
      AND status = 'completed'
)
BEGIN
    SELECT RAISE(ABORT, 'planning commit requires a completed planning run');
END;`

func planningMigration() migration.Migration {
	return migration.Migration{Version: 5, Name: "create_planning_authority", SQL: planningSchemaSQL}
}

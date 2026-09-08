package workstore

import "github.com/disturb-yy/keystone/internal/infrastructure/migration"

// ticketGraphSchemaSQL 将 Ticket Graph、Ticket 及其来源和统一事件扩展为不可变事实。
const ticketGraphSchemaSQL = `
DROP TRIGGER IF EXISTS tr_change_events_no_update;
DROP TRIGGER IF EXISTS tr_change_events_no_delete;
DROP TRIGGER IF EXISTS tr_event_artifact_ownership;
DROP TRIGGER IF EXISTS tr_event_run_ownership;
DROP TRIGGER IF EXISTS tr_event_decision_ownership;
DROP TRIGGER IF EXISTS tr_event_graph_ownership;
DROP TRIGGER IF EXISTS tr_event_aggregate_shape;
DROP TRIGGER IF EXISTS tr_event_run_shape;
DROP TRIGGER IF EXISTS tr_event_decision_shape;
DROP TRIGGER IF EXISTS tr_event_graph_shape;
DROP TRIGGER IF EXISTS tr_event_artifacts_no_update;
DROP TRIGGER IF EXISTS tr_event_artifacts_no_delete;
DROP INDEX IF EXISTS ux_project_initialized_event;
DROP INDEX IF EXISTS ix_events_change_sequence;

ALTER TABLE t_event_artifacts RENAME TO t_event_artifacts_v5;
ALTER TABLE t_project_events RENAME TO t_project_events_v5;

CREATE TABLE t_ticket_graphs (
    graph_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    change_id TEXT NOT NULL,
    base_revision TEXT NOT NULL,
    plan_artifact_ref_id TEXT NOT NULL,
    draft_artifact_ref_id TEXT NOT NULL,
    ticketize_agent_run_id TEXT NOT NULL REFERENCES t_agent_runs(agent_run_id),
    generator_name TEXT NOT NULL CHECK (trim(generator_name) <> ''),
    generator_version TEXT NOT NULL CHECK (trim(generator_version) <> ''),
    created_at TEXT NOT NULL CHECK (trim(created_at) <> ''),
    UNIQUE(change_id),
    UNIQUE(graph_id, change_id),
    UNIQUE(graph_id, change_id, project_id),
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id),
    FOREIGN KEY(plan_artifact_ref_id, change_id, project_id)
        REFERENCES t_artifact_refs(artifact_ref_id, change_id, project_id),
    FOREIGN KEY(draft_artifact_ref_id, change_id, project_id)
        REFERENCES t_artifact_refs(artifact_ref_id, change_id, project_id)
);

CREATE TABLE t_tickets (
    ticket_id TEXT PRIMARY KEY NOT NULL,
    graph_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 1),
    generation_key TEXT NOT NULL,
    title TEXT NOT NULL CHECK (trim(title) <> '' AND length(title) <= 256),
    scope TEXT NOT NULL CHECK (trim(scope) <> '' AND length(CAST(scope AS BLOB)) <= 8192),
    CHECK (length(generation_key) BETWEEN 1 AND 64
       AND generation_key NOT GLOB '*[^a-z0-9._-]*'
       AND substr(generation_key, 1, 1) NOT GLOB '[._-]'),
	UNIQUE(graph_id, ordinal),
	UNIQUE(graph_id, generation_key),
	UNIQUE(graph_id, ticket_id),
	UNIQUE(ticket_id, graph_id),
    FOREIGN KEY(graph_id) REFERENCES t_ticket_graphs(graph_id)
        DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE t_ticket_acceptance_criteria (
    graph_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 1),
    text TEXT NOT NULL CHECK (trim(text) <> '' AND length(CAST(text AS BLOB)) <= 8192),
    PRIMARY KEY(graph_id, ticket_id, ordinal),
    UNIQUE(graph_id, ticket_id, text),
    FOREIGN KEY(graph_id, ticket_id)
        REFERENCES t_tickets(graph_id, ticket_id)
        DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE t_ticket_dependencies (
    graph_id TEXT NOT NULL,
    dependent_ticket_id TEXT NOT NULL,
    blocker_ticket_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind = 'BLOCKED_BY'),
    PRIMARY KEY(graph_id, dependent_ticket_id, blocker_ticket_id),
    FOREIGN KEY(graph_id, dependent_ticket_id)
        REFERENCES t_tickets(graph_id, ticket_id)
        DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(graph_id, blocker_ticket_id)
        REFERENCES t_tickets(graph_id, ticket_id)
        DEFERRABLE INITIALLY DEFERRED,
    CHECK (dependent_ticket_id <> blocker_ticket_id)
);

CREATE TABLE t_project_events_v6 (
    event_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    type TEXT NOT NULL CHECK (type IN ('ProjectInitialized', 'ChangeCreated', 'AgentRunStarted', 'AgentRunCompleted', 'AgentRunReportLate', 'StageAdvanced', 'ChangePaused', 'ChangeResumed', 'ChangeHumanRequired', 'HumanDecisionRecorded', 'ChangeCancelled', 'TicketGraphCreated')),
    occurred_at TEXT NOT NULL,
    event_sequence INTEGER NOT NULL CHECK (event_sequence >= 1),
    change_id TEXT,
    agent_run_id TEXT REFERENCES t_agent_runs(agent_run_id),
    decision_id TEXT REFERENCES t_human_decisions(decision_id),
    ticket_graph_id TEXT,
    actor TEXT NOT NULL DEFAULT '',
    UNIQUE(change_id, event_sequence),
    FOREIGN KEY(change_id, project_id) REFERENCES t_changes(change_id, project_id),
    FOREIGN KEY(agent_run_id) REFERENCES t_agent_runs(agent_run_id),
    FOREIGN KEY(decision_id) REFERENCES t_human_decisions(decision_id),
    FOREIGN KEY(ticket_graph_id, change_id, project_id) REFERENCES t_ticket_graphs(graph_id, change_id, project_id)
);

CREATE TABLE t_event_artifacts_v6 (
    event_id TEXT NOT NULL REFERENCES t_project_events_v6(event_id),
    artifact_ref_id TEXT NOT NULL REFERENCES t_artifact_refs(artifact_ref_id),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY(event_id, ordinal),
    UNIQUE(event_id, artifact_ref_id)
);

INSERT INTO t_project_events_v6 (event_id, project_id, type, occurred_at, event_sequence, change_id, agent_run_id, decision_id, actor)
SELECT event_id, project_id, type, occurred_at, event_sequence, change_id, agent_run_id, decision_id, actor
FROM t_project_events_v5;
INSERT INTO t_event_artifacts_v6 (event_id, artifact_ref_id, ordinal)
SELECT event_id, artifact_ref_id, ordinal
FROM t_event_artifacts_v5;
DROP TABLE t_event_artifacts_v5;
DROP TABLE t_project_events_v5;
ALTER TABLE t_project_events_v6 RENAME TO t_project_events;
ALTER TABLE t_event_artifacts_v6 RENAME TO t_event_artifacts;

CREATE UNIQUE INDEX ux_project_initialized_event
    ON t_project_events(project_id, type) WHERE change_id IS NULL;
CREATE INDEX ix_events_change_sequence ON t_project_events(change_id, event_sequence);
CREATE INDEX ix_ticket_graphs_project_change ON t_ticket_graphs(project_id, change_id);
CREATE INDEX ix_tickets_graph_ordinal ON t_tickets(graph_id, ordinal);
CREATE INDEX ix_ticket_dependencies_blocker ON t_ticket_dependencies(graph_id, blocker_ticket_id);

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
CREATE TRIGGER tr_event_graph_ownership
BEFORE INSERT ON t_project_events
WHEN NEW.ticket_graph_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM t_ticket_graphs
    WHERE graph_id = NEW.ticket_graph_id
      AND change_id = NEW.change_id
      AND project_id = NEW.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'event ticket graph belongs to another change');
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
WHEN (NEW.type IN ('AgentRunStarted', 'AgentRunCompleted', 'AgentRunReportLate', 'StageAdvanced', 'ChangeHumanRequired', 'TicketGraphCreated') AND NEW.agent_run_id IS NULL)
  OR (NEW.type NOT IN ('AgentRunStarted', 'AgentRunCompleted', 'AgentRunReportLate', 'StageAdvanced', 'ChangeHumanRequired', 'TicketGraphCreated') AND NEW.agent_run_id IS NOT NULL)
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
CREATE TRIGGER tr_event_graph_shape
BEFORE INSERT ON t_project_events
WHEN (NEW.type = 'TicketGraphCreated' AND NEW.ticket_graph_id IS NULL)
  OR (NEW.type <> 'TicketGraphCreated' AND NEW.ticket_graph_id IS NOT NULL)
BEGIN
    SELECT RAISE(ABORT, 'event ticket graph association is invalid');
END;

CREATE TRIGGER tr_ticket_graph_source_shape
BEFORE INSERT ON t_ticket_graphs
WHEN NOT EXISTS (
    SELECT 1 FROM t_artifact_refs
    WHERE artifact_ref_id = NEW.plan_artifact_ref_id
      AND project_id = NEW.project_id
      AND change_id = NEW.change_id
      AND role = 'output'
      AND kind = 'plan'
      AND schema_version = 'Plan.v1'
      AND source_revision = NEW.base_revision
)
  OR NOT EXISTS (
    SELECT 1 FROM t_artifact_refs
    WHERE artifact_ref_id = NEW.draft_artifact_ref_id
      AND project_id = NEW.project_id
      AND change_id = NEW.change_id
      AND role = 'output'
      AND kind = 'ticket_draft'
      AND schema_version = 'keystone.ticket-draft.v1'
      AND source_revision = NEW.base_revision
)
BEGIN
    SELECT RAISE(ABORT, 'ticket graph source artifacts are invalid');
END;
CREATE TRIGGER tr_ticket_graph_not_empty
AFTER INSERT ON t_ticket_graphs
WHEN (SELECT COUNT(*) FROM t_tickets WHERE graph_id = NEW.graph_id) NOT BETWEEN 1 AND 64
  OR EXISTS (
      SELECT 1 FROM t_tickets ticket
      WHERE ticket.graph_id = NEW.graph_id
        AND (SELECT COUNT(*) FROM t_ticket_acceptance_criteria criterion
             WHERE criterion.graph_id = ticket.graph_id AND criterion.ticket_id = ticket.ticket_id) NOT BETWEEN 1 AND 32
  )
BEGIN
    SELECT RAISE(ABORT, 'ticket graph final shape is invalid');
END;
CREATE TRIGGER tr_ticket_graph_authority_shape
BEFORE INSERT ON t_ticket_graphs
WHEN NOT EXISTS (
    SELECT 1 FROM t_changes
    WHERE change_id = NEW.change_id AND project_id = NEW.project_id
      AND stage = 'Ticketize' AND status = 'active'
)
  OR NOT EXISTS (
    SELECT 1 FROM t_agent_runs
    WHERE agent_run_id = NEW.ticketize_agent_run_id
      AND project_id = NEW.project_id AND change_id = NEW.change_id
      AND stage = 'Ticketize' AND run_kind = 'planning'
      AND source_revision = NEW.base_revision
      AND status = 'completed' AND outcome = 'succeeded'
)
  OR NOT EXISTS (
    SELECT 1
    FROM t_agent_run_artifacts run_artifact
    JOIN t_agent_runs run ON run.agent_run_id = run_artifact.agent_run_id
    JOIN t_artifact_refs ref ON ref.artifact_ref_id = run_artifact.artifact_ref_id
    WHERE run_artifact.agent_run_id = NEW.ticketize_agent_run_id
      AND run_artifact.role = 'output'
      AND run_artifact.artifact_ref_id = NEW.draft_artifact_ref_id
      AND ref.change_id = NEW.change_id AND ref.project_id = NEW.project_id
      AND ref.role = 'output' AND ref.kind = 'ticket_draft'
      AND ref.schema_version = 'keystone.ticket-draft.v1'
      AND ref.source_revision = NEW.base_revision
)
  OR NOT EXISTS (
    SELECT 1
    FROM t_agent_run_artifacts run_artifact
    JOIN t_agent_runs run ON run.agent_run_id = run_artifact.agent_run_id
    JOIN t_artifact_refs ref ON ref.artifact_ref_id = run_artifact.artifact_ref_id
    WHERE run.stage = 'Plan' AND run.run_kind = 'planning'
      AND run.change_id = NEW.change_id AND run.project_id = NEW.project_id
      AND run.source_revision = NEW.base_revision
      AND run.status = 'completed' AND run.outcome = 'succeeded'
      AND run_artifact.role = 'output'
      AND run_artifact.artifact_ref_id = NEW.plan_artifact_ref_id
      AND ref.change_id = NEW.change_id AND ref.project_id = NEW.project_id
      AND ref.role = 'output' AND ref.kind = 'plan'
      AND ref.schema_version = 'Plan.v1' AND ref.source_revision = NEW.base_revision
)
BEGIN
    SELECT RAISE(ABORT, 'ticket graph authority sources are invalid');
END;
CREATE TRIGGER tr_ticket_graphs_no_update
BEFORE UPDATE ON t_ticket_graphs
BEGIN
    SELECT RAISE(ABORT, 'ticket graphs are immutable');
END;
CREATE TRIGGER tr_ticket_graphs_no_delete
BEFORE DELETE ON t_ticket_graphs
BEGIN
    SELECT RAISE(ABORT, 'ticket graphs are immutable');
END;
CREATE TRIGGER tr_tickets_no_update
BEFORE UPDATE ON t_tickets
BEGIN
    SELECT RAISE(ABORT, 'tickets are immutable');
END;
CREATE TRIGGER tr_tickets_no_delete
BEFORE DELETE ON t_tickets
BEGIN
    SELECT RAISE(ABORT, 'tickets are immutable');
END;
CREATE TRIGGER tr_ticket_acceptance_criteria_no_update
BEFORE UPDATE ON t_ticket_acceptance_criteria
BEGIN
    SELECT RAISE(ABORT, 'ticket acceptance criteria are immutable');
END;
CREATE TRIGGER tr_ticket_acceptance_criteria_no_delete
BEFORE DELETE ON t_ticket_acceptance_criteria
BEGIN
    SELECT RAISE(ABORT, 'ticket acceptance criteria are immutable');
END;
CREATE TRIGGER tr_ticket_dependencies_no_update
BEFORE UPDATE ON t_ticket_dependencies
BEGIN
    SELECT RAISE(ABORT, 'ticket dependencies are immutable');
END;
CREATE TRIGGER tr_ticket_dependencies_no_delete
BEFORE DELETE ON t_ticket_dependencies
BEGIN
    SELECT RAISE(ABORT, 'ticket dependencies are immutable');
END;
CREATE TRIGGER tr_ticket_dependency_acyclic
BEFORE INSERT ON t_ticket_dependencies
WHEN EXISTS (
    WITH RECURSIVE reachable(ticket_id) AS (
        SELECT blocker_ticket_id
        FROM t_ticket_dependencies
        WHERE graph_id = NEW.graph_id
          AND dependent_ticket_id = NEW.blocker_ticket_id
        UNION
        SELECT dependency.blocker_ticket_id
        FROM t_ticket_dependencies dependency
        JOIN reachable ON reachable.ticket_id = dependency.dependent_ticket_id
        WHERE dependency.graph_id = NEW.graph_id
    )
    SELECT 1 FROM reachable WHERE ticket_id = NEW.dependent_ticket_id
)
BEGIN
    SELECT RAISE(ABORT, 'ticket dependencies must be acyclic');
END;
CREATE TRIGGER tr_change_execute_requires_graph
BEFORE UPDATE OF stage ON t_changes
WHEN NEW.stage = 'Execute'
  AND NOT EXISTS (SELECT 1 FROM t_ticket_graphs WHERE change_id = NEW.change_id)
BEGIN
    SELECT RAISE(ABORT, 'execute checkpoint requires ticket graph');
END;`

func ticketGraphMigration() migration.Migration {
	return migration.Migration{Version: 6, Name: "create_canonical_ticket_graph", SQL: ticketGraphSchemaSQL}
}

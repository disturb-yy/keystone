package workstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	ticketDraftKind      = "ticket_draft"
	ticketDraftSchemaV1  = "keystone.ticket-draft.v1"
	ticketizeGeneratorV1 = "ticketize.v1"
)

// StartTicketizeRun 创建 Ticketize attempt；Change 仍停留在 Ticketize checkpoint。
func (s *Store) StartTicketizeRun(ctx context.Context, request work.StartPlanningRunRequest) (domain.AgentRun, error) {
	if request.TargetStage != domain.LifecycleStageTicketize {
		return domain.AgentRun{}, fmt.Errorf("start ticketize run: %w", domain.ErrInvalidRequest)
	}
	if len(request.InputArtifactRefIDs) == 0 {
		return domain.AgentRun{}, fmt.Errorf("start ticketize run input: %w", domain.ErrInvalidRequest)
	}
	// Plan output 是 Graph 的审计来源，但不能直接复用为 Ticketize input。
	// 为它创建内容相同的 input ArtifactRef，保持来源引用和执行输入的角色分离。
	planOutput, err := readArtifactRef(ctx, s.db, request.ChangeID, request.InputArtifactRefIDs[0])
	if err != nil {
		return domain.AgentRun{}, err
	}
	if planOutput.Role != domain.ArtifactRoleOutput || planOutput.Kind != "plan" || planOutput.SchemaVersion != "Plan.v1" {
		return domain.AgentRun{}, domain.ErrPlanningRunConflict
	}
	if planOutput.SourceRevision != request.SourceRevision {
		return domain.AgentRun{}, domain.ErrPlanningRunConflict
	}
	if err := requireSuccessfulPlanOutput(ctx, s.db, request.ChangeID, planOutput.ID, request.SourceRevision); err != nil {
		return domain.AgentRun{}, err
	}
	artifact, err := s.FindArtifact(ctx, request.ChangeID, planOutput.ID)
	if err != nil {
		return domain.AgentRun{}, err
	}
	request.InputArtifactRefIDs = append([]domain.ArtifactRefID(nil), request.InputArtifactRefIDs[1:]...)
	request.InputArtifacts = append([]work.PlanningArtifactWrite{{
		Identity: artifact.Identity, MediaType: artifact.MediaType, Kind: planOutput.Kind,
		SchemaVersion: planOutput.SchemaVersion, Summary: planOutput.Summary, SourceRevision: planOutput.SourceRevision,
		InputArtifactRefIDs:  append([]domain.ArtifactRefID(nil), planOutput.InputArtifactRefIDs...),
		RawLogArtifactRefIDs: append([]domain.ArtifactRefID(nil), planOutput.RawLogArtifactRefIDs...),
	}}, request.InputArtifacts...)
	return s.StartPlanningRun(ctx, request)
}

func requireSuccessfulPlanOutput(ctx context.Context, queryer sqlQueryContext, changeID domain.ChangeID, refID domain.ArtifactRefID, sourceRevision string) error {
	var count int
	err := queryer.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM t_agent_run_artifacts run_artifact
JOIN t_agent_runs run ON run.agent_run_id = run_artifact.agent_run_id
WHERE run.change_id = ?
	AND run.stage = 'Plan'
	AND run.run_kind = 'planning'
	AND run.source_revision = ?
	AND run.status = 'completed'
  AND run.outcome = 'succeeded'
  AND run_artifact.role = 'output'
	  AND run_artifact.artifact_ref_id = ?`, changeID, sourceRevision, refID).Scan(&count)
	if err != nil {
		return fmt.Errorf("verify successful plan output: %w", err)
	}
	if count != 1 {
		return domain.ErrPlanningRunConflict
	}
	return nil
}

// FenceTicketizeRun 完成已被 Pause/Cancel 围栏的 Ticketize run，但不改写 Change checkpoint。
func (s *Store) FenceTicketizeRun(ctx context.Context, runID domain.AgentRunID, reason string) (err error) {
	if runID == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("fence ticketize run: %w", domain.ErrInvalidRequest)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ticketize fence: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	run, err := readAgentRun(ctx, tx, runID)
	if err != nil {
		return err
	}
	if run.Stage != domain.LifecycleStageTicketize || run.Status != domain.AgentRunStatusRunning {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit idempotent ticketize fence: %w", err)
		}
		committed = true
		return nil
	}
	completed := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = 'completed', outcome = 'failed', completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, stamp(completed), run.ID); err != nil {
		return fmt.Errorf("complete fenced ticketize run: %w", err)
	}
	change, err := readChange(ctx, tx, run.ChangeID)
	if err != nil {
		return err
	}
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, "ticketize_fence:"+reason, completed, &run.ID, nil, agentRunArtifactRefIDs(run.Artifacts)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ticketize fence: %w", err)
	}
	committed = true
	return nil
}

// CompleteTicketize 在一个事务中提交不可变 Graph、Ticket、依赖、事件和生命周期推进。
func (s *Store) CompleteTicketize(ctx context.Context, request work.TicketizeCompletionRequest) (result work.TicketizeCommit, err error) {
	candidate, err := domain.NormalizeTicketGraphCandidate(request.Candidate)
	if err != nil {
		return result, err
	}
	request.Candidate = candidate
	if err := validateTicketizeCompletionRequest(request); err != nil {
		return result, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin ticketize completion: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()

	run, err := readAgentRun(ctx, tx, request.AgentRunID)
	if err != nil {
		return result, err
	}
	if err := validatePlanningRunIdentity(run, domain.LifecycleStageTicketize, request.Attempt, request.SourceRevision); err != nil {
		return result, err
	}
	change, err := readChange(ctx, tx, run.ChangeID)
	if err != nil {
		return result, err
	}
	if graph, graphErr := readTicketGraph(ctx, tx, change.ID); graphErr == nil {
		requestedPlan, planErr := readArtifactRef(ctx, tx, change.ID, request.PlanArtifactRef.ID)
		requestedDraft, draftErr := readArtifactRef(ctx, tx, change.ID, request.DraftArtifactRef.ID)
		if planErr != nil || draftErr != nil || graph.TicketizeAgentRunID != run.ID || graph.PlanArtifactRef.ID != requestedPlan.ID || graph.PlanArtifactRef.ArtifactID != requestedPlan.ArtifactID || graph.DraftArtifactRef.ArtifactID != requestedDraft.ArtifactID {
			return result, domain.ErrTicketGraphConflict
		}
		if err := tx.Commit(); err != nil {
			return result, fmt.Errorf("commit duplicate ticketize completion: %w", err)
		}
		committed = true
		return work.TicketizeCommit{Graph: graph, Run: run, Change: change, Disposition: work.PlanningCommitDuplicate}, nil
	} else if !errors.Is(graphErr, domain.ErrTicketGraphNotFound) {
		return result, graphErr
	}
	if run.Status != domain.AgentRunStatusRunning {
		return result, domain.ErrTicketGraphConflict
	}
	if change.Status != domain.ChangeStatusActive || change.Stage != domain.LifecycleStageTicketize || change.Version != request.ExpectedChangeVersion || change.BaseRevision != request.SourceRevision {
		return work.TicketizeCommit{Run: run, Change: change, Disposition: work.PlanningCommitFenced}, domain.ErrTicketizeFenced
	}
	if current, currentErr := isCurrentPlanningRun(ctx, tx, change, run); currentErr != nil {
		return result, currentErr
	} else if !current {
		return work.TicketizeCommit{Run: run, Change: change, Disposition: work.PlanningCommitFenced}, domain.ErrTicketizeFenced
	}
	planRef, err := readArtifactRef(ctx, tx, change.ID, request.PlanArtifactRef.ID)
	if err != nil {
		return result, err
	}
	draftRef, err := readArtifactRef(ctx, tx, change.ID, request.DraftArtifactRef.ID)
	if err != nil {
		return result, err
	}
	if err := validateTicketizeSourceRefs(planRef, draftRef, change, run); err != nil {
		return result, err
	}
	inputs := planningRunInputRefIDs(run)
	if len(inputs) == 0 {
		return result, domain.ErrTicketGraphConflict
	}
	planInputRef, err := readArtifactRef(ctx, tx, change.ID, inputs[0])
	if err != nil || planInputRef.Role != domain.ArtifactRoleInput || planInputRef.ArtifactID != planRef.ArtifactID || planInputRef.Kind != planRef.Kind || planInputRef.SchemaVersion != planRef.SchemaVersion || planInputRef.SourceRevision != planRef.SourceRevision {
		return result, domain.ErrTicketGraphConflict
	}

	created := s.now().UTC()
	graphID := domain.TicketGraphID(id.New())
	ticketIDs := make(map[string]domain.TicketID, len(candidate.Tickets))
	for index, draft := range candidate.Tickets {
		ticketID := domain.TicketID(id.New())
		ticketIDs[draft.GenerationKey] = ticketID
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_tickets (ticket_id, graph_id, ordinal, generation_key, title, scope) VALUES (?, ?, ?, ?, ?, ?)`, ticketID, graphID, index+1, draft.GenerationKey, strings.TrimSpace(draft.Title), strings.TrimSpace(draft.Scope)); err != nil {
			return result, fmt.Errorf("insert canonical ticket: %w", err)
		}
		for criterionIndex, criterion := range draft.AcceptanceCriteria {
			if _, err := tx.ExecContext(ctx, `INSERT INTO t_ticket_acceptance_criteria (graph_id, ticket_id, ordinal, text) VALUES (?, ?, ?, ?)`, graphID, ticketID, criterionIndex+1, strings.TrimSpace(criterion)); err != nil {
				return result, fmt.Errorf("insert ticket acceptance criterion: %w", err)
			}
		}
	}
	for _, draft := range candidate.Tickets {
		for _, blocker := range draft.BlockedBy {
			if _, err := tx.ExecContext(ctx, `INSERT INTO t_ticket_dependencies (graph_id, dependent_ticket_id, blocker_ticket_id, kind) VALUES (?, ?, ?, ?)`, graphID, ticketIDs[draft.GenerationKey], ticketIDs[blocker], domain.TicketDependencyBlockedBy); err != nil {
				return result, fmt.Errorf("insert ticket dependency: %w", err)
			}
		}
	}
	generatorName := strings.TrimSpace(request.GeneratorName)
	generatorVersion := strings.TrimSpace(request.GeneratorVersion)
	if generatorName == "" {
		generatorName = ticketizeGeneratorV1
	}
	if generatorVersion == "" {
		generatorVersion = "1"
	}
	if err := finishPlanningRun(ctx, tx, &run, draftRef, request.RawLogRefs, domain.AgentRunOutcomeSucceeded, created); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_ticket_graphs (graph_id, project_id, change_id, base_revision, plan_artifact_ref_id, draft_artifact_ref_id, ticketize_agent_run_id, generator_name, generator_version, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, graphID, change.ProjectID, change.ID, change.BaseRevision, planRef.ID, draftRef.ID, run.ID, generatorName, generatorVersion, stamp(created)); err != nil {
		return result, fmt.Errorf("insert ticket graph: %w", err)
	}
	graph, err := readTicketGraph(ctx, tx, change.ID)
	if err != nil {
		return result, err
	}
	completionRefs := append([]domain.ArtifactRefID{draftRef.ID}, artifactRefIDs(request.RawLogRefs)...)
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, request.Actor, created, &run.ID, nil, completionRefs); err != nil {
		return result, err
	}
	graphEventRefs := append([]domain.ArtifactRefID{planRef.ID, draftRef.ID}, artifactRefIDs(request.RawLogRefs)...)
	if err := insertEventWithGraphTx(ctx, tx, change.ProjectID, change.ID, domain.TicketGraphCreatedType, request.Actor, created, &run.ID, nil, &graphID, graphEventRefs); err != nil {
		return result, err
	}
	updated := change
	updated.Stage = domain.LifecycleStageExecute
	updated.Version++
	updated.UpdatedAt = created
	updated.LatestAgentRun = &run
	if err := updateChangeStage(ctx, tx, change, updated, created); err != nil {
		return result, err
	}
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.StageAdvancedType, request.Actor, created, &run.ID, nil, []domain.ArtifactRefID{planRef.ID, draftRef.ID}); err != nil {
		return result, err
	}
	if err := recordPlanningCommit(ctx, tx, run.ID, draftRef.ID, domain.AgentRunOutcomeSucceeded, work.PlanningCommitCommitted, created); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit ticketize completion: %w", err)
	}
	committed = true
	graph, err = readTicketGraph(ctx, s.db, change.ID)
	if err != nil {
		return result, err
	}
	return work.TicketizeCommit{Graph: graph, Run: run, Change: updated, Disposition: work.PlanningCommitCommitted}, nil
}

// FindTicketGraph 查询 Change 的唯一 Canonical Ticket Graph；不会暴露 generation_key。
func (s *Store) FindTicketGraph(ctx context.Context, changeID domain.ChangeID) (domain.CanonicalTicketGraph, error) {
	if _, err := readChange(ctx, s.db, changeID); err != nil {
		return domain.CanonicalTicketGraph{}, err
	}
	return readTicketGraph(ctx, s.db, changeID)
}

func validateTicketizeCompletionRequest(request work.TicketizeCompletionRequest) error {
	if request.AgentRunID == "" || request.Stage != domain.LifecycleStageTicketize || request.Attempt < 1 || request.ExpectedChangeVersion < 1 || request.SourceRevision == "" || strings.TrimSpace(request.Actor) == "" || request.PlanArtifactRef.ID == "" || request.DraftArtifactRef.ID == "" {
		return fmt.Errorf("ticketize completion: %w", domain.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.GeneratorName) != request.GeneratorName || strings.TrimSpace(request.GeneratorVersion) != request.GeneratorVersion {
		return fmt.Errorf("ticketize generator metadata: %w", domain.ErrInvalidRequest)
	}
	for _, ref := range request.RawLogRefs {
		if ref.ID == "" {
			return fmt.Errorf("ticketize raw log reference: %w", domain.ErrInvalidRequest)
		}
	}
	return nil
}

func validateTicketizeSourceRefs(planRef, draftRef domain.ArtifactRef, change domain.Change, run domain.AgentRun) error {
	if planRef.ChangeID != change.ID || planRef.Role != domain.ArtifactRoleOutput || planRef.Kind != "plan" || planRef.SchemaVersion != "Plan.v1" || planRef.SourceRevision != change.BaseRevision {
		return domain.ErrTicketGraphConflict
	}
	if draftRef.ChangeID != change.ID || draftRef.Role != domain.ArtifactRoleOutput || draftRef.Kind != ticketDraftKind || draftRef.SchemaVersion != ticketDraftSchemaV1 || draftRef.SourceRevision != run.SourceRevision {
		return domain.ErrTicketGraphConflict
	}
	return nil
}

func readTicketGraph(ctx context.Context, queryer sqlQueryContext, changeID domain.ChangeID) (domain.CanonicalTicketGraph, error) {
	var graph domain.CanonicalTicketGraph
	var created string
	err := queryer.QueryRowContext(ctx, `SELECT graph_id, project_id, change_id, base_revision, plan_artifact_ref_id, draft_artifact_ref_id, ticketize_agent_run_id, generator_name, generator_version, created_at FROM t_ticket_graphs WHERE change_id = ?`, changeID).
		Scan(&graph.ID, &graph.ProjectID, &graph.ChangeID, &graph.BaseRevision, &graph.PlanArtifactRef.ID, &graph.DraftArtifactRef.ID, &graph.TicketizeAgentRunID, &graph.GeneratorName, &graph.GeneratorVersion, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return graph, domain.ErrTicketGraphNotFound
	}
	if err != nil {
		return graph, fmt.Errorf("read ticket graph: %w", err)
	}
	graph.PlanArtifactRef, err = readArtifactRef(ctx, queryer, graph.ChangeID, graph.PlanArtifactRef.ID)
	if err != nil {
		return graph, err
	}
	graph.DraftArtifactRef, err = readArtifactRef(ctx, queryer, graph.ChangeID, graph.DraftArtifactRef.ID)
	if err != nil {
		return graph, err
	}
	graph.CreatedAt, err = parseStamp(created)
	if err != nil {
		return graph, err
	}
	rows, err := queryer.QueryContext(ctx, `SELECT ticket_id, graph_id, ordinal, title, scope FROM t_tickets WHERE graph_id = ? ORDER BY ordinal`, graph.ID)
	if err != nil {
		return graph, fmt.Errorf("list canonical tickets: %w", err)
	}
	tickets := make([]domain.CanonicalTicket, 0)
	for rows.Next() {
		var ticket domain.CanonicalTicket
		if err := rows.Scan(&ticket.ID, &ticket.GraphID, &ticket.Ordinal, &ticket.Title, &ticket.Scope); err != nil {
			_ = rows.Close()
			return graph, fmt.Errorf("scan canonical ticket: %w", err)
		}
		tickets = append(tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return graph, fmt.Errorf("read canonical tickets: %w", err)
	}
	if err := rows.Close(); err != nil {
		return graph, fmt.Errorf("close canonical tickets: %w", err)
	}
	for index := range tickets {
		criteria, err := readTicketAcceptanceCriteria(ctx, queryer, graph.ID, tickets[index].ID)
		if err != nil {
			return graph, err
		}
		tickets[index].AcceptanceCriteria = criteria
	}
	graph.Tickets = tickets
	dependencies, err := readTicketDependencies(ctx, queryer, graph.ID)
	if err != nil {
		return graph, err
	}
	graph.Dependencies = dependencies
	if err := graph.Validate(); err != nil {
		return domain.CanonicalTicketGraph{}, fmt.Errorf("validate stored ticket graph: %w", domain.ErrInternal)
	}
	return graph, nil
}

func readTicketAcceptanceCriteria(ctx context.Context, queryer sqlQueryContext, graphID domain.TicketGraphID, ticketID domain.TicketID) ([]domain.TicketAcceptanceCriterion, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, text FROM t_ticket_acceptance_criteria WHERE graph_id = ? AND ticket_id = ? ORDER BY ordinal`, graphID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list ticket acceptance criteria: %w", err)
	}
	defer rows.Close()
	criteria := make([]domain.TicketAcceptanceCriterion, 0)
	for rows.Next() {
		var criterion domain.TicketAcceptanceCriterion
		if err := rows.Scan(&criterion.Ordinal, &criterion.Text); err != nil {
			return nil, fmt.Errorf("scan ticket acceptance criterion: %w", err)
		}
		criteria = append(criteria, criterion)
	}
	return criteria, rows.Err()
}

func readTicketDependencies(ctx context.Context, queryer sqlQueryContext, graphID domain.TicketGraphID) ([]domain.TicketDependency, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT d.graph_id, d.dependent_ticket_id, d.blocker_ticket_id, d.kind FROM t_ticket_dependencies d JOIN t_tickets dependent ON dependent.graph_id = d.graph_id AND dependent.ticket_id = d.dependent_ticket_id JOIN t_tickets blocker ON blocker.graph_id = d.graph_id AND blocker.ticket_id = d.blocker_ticket_id WHERE d.graph_id = ? ORDER BY dependent.ordinal, blocker.ordinal`, graphID)
	if err != nil {
		return nil, fmt.Errorf("list ticket dependencies: %w", err)
	}
	defer rows.Close()
	dependencies := make([]domain.TicketDependency, 0)
	for rows.Next() {
		var dependency domain.TicketDependency
		if err := rows.Scan(&dependency.GraphID, &dependency.DependentTicketID, &dependency.BlockerTicketID, &dependency.Kind); err != nil {
			return nil, fmt.Errorf("scan ticket dependency: %w", err)
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, rows.Err()
}

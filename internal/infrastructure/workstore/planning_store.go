package workstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

// StartPlanningRun 在成功前保留当前 checkpoint，并为显式目标 stage 创建唯一 running attempt。
func (s *Store) StartPlanningRun(ctx context.Context, request work.StartPlanningRunRequest) (run domain.AgentRun, err error) {
	if err := validatePlanningStart(request); err != nil {
		return run, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return run, fmt.Errorf("begin planning run: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	change, err := readChange(ctx, tx, request.ChangeID)
	if err != nil {
		return run, err
	}
	if err := validatePlanningStartFence(ctx, tx, change, request); err != nil {
		return run, err
	}
	created := s.now().UTC()
	inputs, err := s.preparePlanningRunInputs(ctx, tx, change, request, created)
	if err != nil {
		return run, err
	}
	run, err = insertPlanningAgentRun(ctx, tx, change, request, created, inputs)
	if err != nil {
		return run, err
	}
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunStartedType, request.Actor, created, &run.ID, nil, agentRunArtifactRefIDs(inputs)); err != nil {
		return run, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, fmt.Errorf("commit planning run: %w", err)
	}
	committed = true
	return run, nil
}

// RecordPlanningRunFailureCandidate 把调度前的确定性失败幂等保存为非权威候选。
// Coordinator 之后仍必须经过统一的 completion fence 才能推进 Change 或进入 human_required。
func (s *Store) RecordPlanningRunFailureCandidate(ctx context.Context, runID domain.AgentRunID, reason string) (err error) {
	if ctx == nil || runID == "" || !validPlanningFailureReason(reason) {
		return fmt.Errorf("record planning failure candidate: %w", domain.ErrInvalidRequest)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin planning failure candidate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	if err := insertPlanningSystemCandidate(ctx, tx, runID, reason, s.now().UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit planning failure candidate: %w", err)
	}
	committed = true
	return nil
}

// CompletePlanningStage 原子保存权威结构化 Artifact、完成 AgentRun 并条件推进 checkpoint。
func (s *Store) CompletePlanningStage(ctx context.Context, request work.CompletePlanningStageRequest) (work.PlanningStageCommit, error) {
	completion := planningCompletion{
		runID: request.AgentRunID, stage: request.Stage, attempt: request.Attempt,
		expectedVersion: request.ExpectedChangeVersion, sourceRevision: request.SourceRevision,
		actor: request.Actor, artifact: request.Artifact, rawLogs: request.RawLogs,
		outcome: domain.AgentRunOutcomeSucceeded,
	}
	return s.completePlanningStage(ctx, completion)
}

// FailPlanningStage 原子保存失败证据、完成 AgentRun，并仅对当前 attempt 进入 human_required。
func (s *Store) FailPlanningStage(ctx context.Context, request work.FailPlanningStageRequest) (work.PlanningStageCommit, error) {
	completion := planningCompletion{
		runID: request.AgentRunID, stage: request.Stage, attempt: request.Attempt,
		expectedVersion: request.ExpectedChangeVersion, sourceRevision: request.SourceRevision,
		actor: request.Actor, artifact: request.Failure, rawLogs: request.RawLogs,
		outcome: domain.AgentRunOutcomeFailed,
	}
	return s.completePlanningStage(ctx, completion)
}

// ListRecoverablePlanningChanges 返回可启动下一阶段或仍有 running Planning run 待收敛的 Change。
func (s *Store) ListRecoverablePlanningChanges(ctx context.Context) ([]domain.Change, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.change_id
FROM t_changes c
WHERE (c.status = 'active' AND c.stage IN ('Intent', 'Understand', 'Design', 'Ticketize'))
   AND (
       c.stage <> 'Ticketize'
       OR NOT EXISTS (
           SELECT 1 FROM t_agent_runs r
           WHERE r.change_id = c.change_id AND r.stage = 'Ticketize'
       )
       OR EXISTS (
           SELECT 1
           FROM t_project_events control
           WHERE control.change_id = c.change_id
             AND control.type IN ('ChangeResumed', 'HumanDecisionRecorded')
             AND control.event_sequence > COALESCE((
                 SELECT MAX(started.event_sequence)
                 FROM t_project_events started
                 JOIN t_agent_runs run ON run.agent_run_id = started.agent_run_id
                 WHERE started.change_id = c.change_id
                   AND started.type = 'AgentRunStarted'
                   AND run.stage = 'Ticketize'
             ), 0)
             AND (control.type = 'ChangeResumed' OR EXISTS (
                 SELECT 1 FROM t_human_decisions decision
                 WHERE decision.decision_id = control.decision_id
                   AND decision.kind = 'retry'
             ))
       )
   )
   OR EXISTS (
       SELECT 1
       FROM t_agent_runs r
       WHERE r.change_id = c.change_id
         AND r.run_kind = 'planning'
         AND r.status = 'running'
   )
ORDER BY c.updated_at, c.change_id`)
	if err != nil {
		return nil, fmt.Errorf("list recoverable planning changes: %w", err)
	}
	var ids []domain.ChangeID
	for rows.Next() {
		var changeID domain.ChangeID
		if err := rows.Scan(&changeID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan recoverable planning change: %w", err)
		}
		ids = append(ids, changeID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("read recoverable planning changes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close recoverable planning changes: %w", err)
	}
	changes := make([]domain.Change, 0, len(ids))
	for _, changeID := range ids {
		change, err := readChange(ctx, s.db, changeID)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, nil
}

// FindPlanningRunCandidate 查询 Worker 已保存、尚未获得 lifecycle authority 的候选。
func (s *Store) FindPlanningRunCandidate(ctx context.Context, runID domain.AgentRunID) (work.PlanningRunCandidate, error) {
	var candidate work.PlanningRunCandidate
	var exitCode sql.NullInt64
	var candidateTruncated int
	var guardJSON, received string
	err := s.db.QueryRowContext(ctx, `SELECT agent_run_id, outcome, exit_code, after_revision, failure_reason, guard_findings_json, candidate_truncated, received_at FROM t_planning_run_candidates WHERE agent_run_id = ?`, runID).
		Scan(&candidate.AgentRunID, &candidate.Outcome, &exitCode, &candidate.AfterRevision, &candidate.FailureReason, &guardJSON, &candidateTruncated, &received)
	if errors.Is(err, sql.ErrNoRows) {
		return candidate, domain.ErrPlanningCandidateNotFound
	}
	if err != nil {
		return candidate, fmt.Errorf("find planning candidate: %w", err)
	}
	if exitCode.Valid {
		value := int(exitCode.Int64)
		candidate.ExitCode = &value
	}
	candidate.CandidateTruncated = candidateTruncated == 1
	if err := json.Unmarshal([]byte(guardJSON), &candidate.GuardFindings); err != nil {
		return candidate, fmt.Errorf("decode planning candidate findings: %w", domain.ErrInternal)
	}
	candidate.ReceivedAt, err = parseStamp(received)
	if err != nil {
		return candidate, err
	}
	refs, err := listPlanningCandidateRefs(ctx, s.db, runID)
	if err != nil {
		return candidate, err
	}
	for index := range refs {
		if (refs[index].Kind == "candidate" || refs[index].Kind == "ticket_draft") && candidate.CandidateRef == nil {
			value := refs[index]
			candidate.CandidateRef = &value
			continue
		}
		candidate.RawLogRefs = append(candidate.RawLogRefs, refs[index])
	}
	return candidate, nil
}

type planningCompletion struct {
	runID           domain.AgentRunID
	stage           domain.LifecycleStage
	attempt         int
	expectedVersion domain.ChangeVersion
	sourceRevision  string
	actor           string
	artifact        work.PlanningArtifactWrite
	rawLogs         []work.PlanningArtifactWrite
	outcome         string
}

func (s *Store) completePlanningStage(ctx context.Context, completion planningCompletion) (result work.PlanningStageCommit, err error) {
	if err := validatePlanningCompletion(completion); err != nil {
		return result, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin planning completion: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	result, replayed, err := s.replayPlanningCompletion(ctx, tx, completion)
	if err != nil || replayed {
		if err == nil {
			err = tx.Commit()
			committed = err == nil
		}
		return result, err
	}
	result, err = s.persistPlanningCompletion(ctx, tx, completion)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return work.PlanningStageCommit{}, fmt.Errorf("commit planning completion: %w", err)
	}
	committed = true
	return result, nil
}

func (s *Store) replayPlanningCompletion(ctx context.Context, tx *sql.Tx, completion planningCompletion) (work.PlanningStageCommit, bool, error) {
	run, err := readAgentRun(ctx, tx, completion.runID)
	if err != nil {
		return work.PlanningStageCommit{}, false, err
	}
	if err := validatePlanningRunIdentity(run, completion.stage, completion.attempt, completion.sourceRevision); err != nil {
		return work.PlanningStageCommit{}, false, err
	}
	if run.Status == domain.AgentRunStatusRunning {
		return work.PlanningStageCommit{}, false, nil
	}
	var refID domain.ArtifactRefID
	var outcome string
	err = tx.QueryRowContext(ctx, `SELECT artifact_ref_id, outcome FROM t_planning_stage_commits WHERE agent_run_id = ?`, run.ID).Scan(&refID, &outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return work.PlanningStageCommit{}, false, domain.ErrPlanningRunConflict
	}
	if err != nil {
		return work.PlanningStageCommit{}, false, fmt.Errorf("read planning completion: %w", err)
	}
	if outcome != completion.outcome {
		return work.PlanningStageCommit{}, false, domain.ErrPlanningRunConflict
	}
	ref, err := readArtifactRef(ctx, tx, run.ChangeID, refID)
	if err != nil || ref.Role != planningOutcomeRole(completion.outcome) || !planningArtifactMatches(ctx, tx, ref, completion.artifact, true) {
		return work.PlanningStageCommit{}, false, domain.ErrPlanningRunConflict
	}
	change, err := readChange(ctx, tx, run.ChangeID)
	if err != nil {
		return work.PlanningStageCommit{}, false, err
	}
	rawRefs, err := readArtifactRefs(ctx, tx, run.ChangeID, ref.RawLogArtifactRefIDs)
	if err != nil {
		return work.PlanningStageCommit{}, false, err
	}
	if !planningRawLogsMatch(ctx, tx, rawRefs, completion) {
		return work.PlanningStageCommit{}, false, domain.ErrPlanningRunConflict
	}
	return work.PlanningStageCommit{Run: run, Change: change, ArtifactRef: ref, RawLogRefs: rawRefs, Disposition: work.PlanningCommitDuplicate}, true, nil
}

func (s *Store) persistPlanningCompletion(ctx context.Context, tx *sql.Tx, completion planningCompletion) (work.PlanningStageCommit, error) {
	run, err := readAgentRun(ctx, tx, completion.runID)
	if err != nil {
		return work.PlanningStageCommit{}, err
	}
	if err := validatePlanningRunIdentity(run, completion.stage, completion.attempt, completion.sourceRevision); err != nil {
		return work.PlanningStageCommit{}, err
	}
	if !sameArtifactRefIDs(planningRunInputRefIDs(run), completion.artifact.InputArtifactRefIDs) {
		return work.PlanningStageCommit{}, domain.ErrPlanningRunConflict
	}
	change, err := readChange(ctx, tx, run.ChangeID)
	if err != nil || change.BaseRevision != completion.sourceRevision {
		return work.PlanningStageCommit{}, domain.ErrPlanningRunConflict
	}
	if change.Status == domain.ChangeStatusPaused || (change.Status == domain.ChangeStatusActive && change.Version != completion.expectedVersion) {
		return work.PlanningStageCommit{Run: run, Change: change, Disposition: work.PlanningCommitFenced}, domain.ErrPlanningRunDeferred
	}
	created := s.now().UTC()
	role := planningOutcomeRole(completion.outcome)
	rawRefs, err := s.persistPlanningRawLogs(ctx, tx, change, role, completion, created)
	if err != nil {
		return work.PlanningStageCommit{}, err
	}
	artifactWrite := completion.artifact
	// rawRefs 已按“既有 candidate/report 引用 + 本次新增 raw log”返回完整顺序；
	// 直接替换可避免把既有引用追加两次，触发 append-only link 的重复围栏。
	artifactWrite.RawLogArtifactRefIDs = artifactRefIDs(rawRefs)
	ref, err := insertPlanningArtifactRef(ctx, tx, change, artifactWrite, role, created)
	if err != nil {
		return work.PlanningStageCommit{}, err
	}
	if err := finishPlanningRun(ctx, tx, &run, ref, rawRefs, completion.outcome, created); err != nil {
		return work.PlanningStageCommit{}, err
	}
	refs := append([]domain.ArtifactRefID{ref.ID}, artifactRefIDs(rawRefs)...)
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunCompletedType, completion.actor, created, &run.ID, nil, refs); err != nil {
		return work.PlanningStageCommit{}, err
	}
	disposition, updated, err := applyPlanningResult(ctx, tx, change, run, completion, created, refs)
	if err != nil {
		return work.PlanningStageCommit{}, err
	}
	if err := recordPlanningCommit(ctx, tx, run.ID, ref.ID, completion.outcome, disposition, created); err != nil {
		return work.PlanningStageCommit{}, err
	}
	return work.PlanningStageCommit{Run: run, Change: updated, ArtifactRef: ref, RawLogRefs: rawRefs, Disposition: disposition}, nil
}

func validatePlanningStart(request work.StartPlanningRunRequest) error {
	if request.ChangeID == "" || request.ExpectedChangeVersion < 1 || strings.TrimSpace(request.Actor) == "" || request.SourceRevision == "" {
		return fmt.Errorf("start planning run: %w", domain.ErrInvalidRequest)
	}
	if request.TargetStage != domain.LifecycleStageUnderstand && request.TargetStage != domain.LifecycleStageDesign && request.TargetStage != domain.LifecycleStagePlan && request.TargetStage != domain.LifecycleStageTicketize {
		return fmt.Errorf("start planning run stage: %w", domain.ErrInvalidRequest)
	}
	if len(request.InputArtifactRefIDs)+len(request.InputArtifacts) == 0 {
		return fmt.Errorf("start planning run inputs: %w", domain.ErrInvalidRequest)
	}
	return nil
}

func validatePlanningStartFence(ctx context.Context, tx *sql.Tx, change domain.Change, request work.StartPlanningRunRequest) error {
	if change.Status != domain.ChangeStatusActive || change.Version != request.ExpectedChangeVersion || change.BaseRevision != request.SourceRevision || !planningTargetFollows(change.Stage, request.TargetStage) {
		return domain.ErrPlanningRunConflict
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_agent_runs WHERE change_id = ? AND status = 'running'`, change.ID).Scan(&count); err != nil {
		return fmt.Errorf("read running planning run: %w", err)
	}
	if count != 0 {
		return domain.ErrPlanningRunConflict
	}
	return nil
}

func (s *Store) preparePlanningRunInputs(ctx context.Context, tx *sql.Tx, change domain.Change, request work.StartPlanningRunRequest, created time.Time) ([]domain.AgentRunArtifact, error) {
	var refs []domain.ArtifactRef
	appendWrites := func() error {
		for _, input := range request.InputArtifacts {
			ref, err := insertPlanningArtifactRef(ctx, tx, change, input, domain.ArtifactRoleInput, created)
			if err != nil {
				return err
			}
			refs = append(refs, ref)
		}
		return nil
	}
	// Ticketize 的第一输入必须是 Plan 的同内容 input Ref；它由
	// StartTicketizeRun 作为 InputArtifact 写入，已有 Context Ref 随后追加。
	if request.TargetStage == domain.LifecycleStageTicketize {
		if err := appendWrites(); err != nil {
			return nil, err
		}
		inputRefs, err := readArtifactRefs(ctx, tx, change.ID, request.InputArtifactRefIDs)
		if err != nil {
			return nil, err
		}
		refs = append(refs, inputRefs...)
	} else {
		var err error
		refs, err = readArtifactRefs(ctx, tx, change.ID, request.InputArtifactRefIDs)
		if err != nil {
			return nil, err
		}
		if err := appendWrites(); err != nil {
			return nil, err
		}
	}
	seen := make(map[domain.ArtifactRefID]struct{}, len(refs))
	artifacts := make([]domain.AgentRunArtifact, 0, len(refs))
	for ordinal, ref := range refs {
		if _, ok := seen[ref.ID]; ok || ref.Role == domain.ArtifactRoleFailure {
			return nil, fmt.Errorf("planning run input: %w", domain.ErrInvalidRequest)
		}
		seen[ref.ID] = struct{}{}
		artifacts = append(artifacts, domain.AgentRunArtifact{ArtifactRefID: ref.ID, Role: domain.ArtifactRoleInput, Ordinal: ordinal})
	}
	return artifacts, nil
}

func insertPlanningAgentRun(ctx context.Context, tx *sql.Tx, change domain.Change, request work.StartPlanningRunRequest, created time.Time, artifacts []domain.AgentRunArtifact) (domain.AgentRun, error) {
	var latest sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM t_agent_runs WHERE change_id = ? AND stage = ?`, change.ID, request.TargetStage).Scan(&latest); err != nil {
		return domain.AgentRun{}, fmt.Errorf("read planning attempt: %w", err)
	}
	attempt := 1
	if latest.Valid {
		attempt = int(latest.Int64) + 1
	}
	run := domain.AgentRun{ID: domain.AgentRunID(id.New()), ChangeID: change.ID, Stage: request.TargetStage, Attempt: attempt, RunKind: domain.AgentRunKindPlanning, SourceRevision: request.SourceRevision, Status: domain.AgentRunStatusRunning, StartedAt: created, Artifacts: append([]domain.AgentRunArtifact(nil), artifacts...)}
	if err := run.Validate(); err != nil {
		return domain.AgentRun{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_agent_runs (agent_run_id, project_id, change_id, stage, attempt, status, outcome, started_at, run_kind, source_revision) VALUES (?, ?, ?, ?, ?, 'running', '', ?, 'planning', ?)`, run.ID, change.ProjectID, change.ID, run.Stage, run.Attempt, stamp(created), run.SourceRevision); err != nil {
		return domain.AgentRun{}, fmt.Errorf("insert planning run: %w", err)
	}
	if err := insertAgentRunArtifacts(ctx, tx, run.ID, artifacts); err != nil {
		return domain.AgentRun{}, err
	}
	return run, nil
}

func validatePlanningCompletion(completion planningCompletion) error {
	if completion.runID == "" || completion.attempt < 1 || completion.expectedVersion < 1 || completion.sourceRevision == "" || strings.TrimSpace(completion.actor) == "" {
		return fmt.Errorf("planning completion: %w", domain.ErrInvalidRequest)
	}
	if completion.outcome != domain.AgentRunOutcomeSucceeded && completion.outcome != domain.AgentRunOutcomeFailed {
		return fmt.Errorf("planning completion outcome: %w", domain.ErrInvalidRequest)
	}
	if err := validatePlanningArtifactWrite(completion.artifact, completion.sourceRevision); err != nil {
		return err
	}
	for _, rawLog := range completion.rawLogs {
		if err := validatePlanningArtifactWrite(rawLog, completion.sourceRevision); err != nil {
			return err
		}
	}
	return nil
}

func validatePlanningArtifactWrite(artifact work.PlanningArtifactWrite, sourceRevision string) error {
	if err := artifact.Identity.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(artifact.MediaType) == "" || strings.TrimSpace(artifact.Kind) == "" || artifact.Kind != strings.TrimSpace(artifact.Kind) || len(artifact.Kind) > 64 || artifact.SourceRevision != sourceRevision {
		return fmt.Errorf("planning artifact metadata: %w", domain.ErrInvalidRequest)
	}
	if len(artifact.SchemaVersion) > 128 || !utf8.ValidString(artifact.SchemaVersion) || len([]rune(artifact.Summary)) > 256 || !utf8.ValidString(artifact.Summary) {
		return fmt.Errorf("planning artifact metadata: %w", domain.ErrInvalidRequest)
	}
	return nil
}

func validatePlanningRunIdentity(run domain.AgentRun, stage domain.LifecycleStage, attempt int, revision string) error {
	if !run.IsPlanning() || run.Stage != stage || run.Attempt != attempt || run.SourceRevision != revision {
		return domain.ErrPlanningRunConflict
	}
	return nil
}

func planningTargetFollows(checkpoint, target domain.LifecycleStage) bool {
	return (checkpoint == domain.LifecycleStageIntent && target == domain.LifecycleStageUnderstand) ||
		(checkpoint == domain.LifecycleStageUnderstand && target == domain.LifecycleStageDesign) ||
		(checkpoint == domain.LifecycleStageDesign && target == domain.LifecycleStagePlan) ||
		(checkpoint == domain.LifecycleStageTicketize && target == domain.LifecycleStageTicketize)
}

func planningOutcomeRole(outcome string) string {
	if outcome == domain.AgentRunOutcomeSucceeded {
		return domain.ArtifactRoleOutput
	}
	return domain.ArtifactRoleFailure
}

func insertPlanningArtifactRef(ctx context.Context, tx *sql.Tx, change domain.Change, input work.PlanningArtifactWrite, role string, created time.Time) (domain.ArtifactRef, error) {
	if err := validatePlanningArtifactWrite(input, change.BaseRevision); err != nil {
		return domain.ArtifactRef{}, err
	}
	artifact, err := ensureArtifact(ctx, tx, input.Identity, input.MediaType, created)
	if err != nil {
		return domain.ArtifactRef{}, err
	}
	ordinal, err := nextArtifactRefOrdinal(ctx, tx, change.ID, role)
	if err != nil {
		return domain.ArtifactRef{}, err
	}
	ref := domain.ArtifactRef{ID: domain.ArtifactRefID(id.New()), ChangeID: change.ID, ArtifactID: artifact.ID, Role: role, Ordinal: ordinal, Kind: input.Kind, SchemaVersion: input.SchemaVersion, Summary: input.Summary, SourceRevision: input.SourceRevision, InputArtifactRefIDs: append([]domain.ArtifactRefID(nil), input.InputArtifactRefIDs...), RawLogArtifactRefIDs: append([]domain.ArtifactRefID(nil), input.RawLogArtifactRefIDs...)}
	if err := ref.Validate(); err != nil {
		return domain.ArtifactRef{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifact_refs (artifact_ref_id, project_id, change_id, artifact_id, role, ordinal, created_at, kind, schema_version, summary, source_revision) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, ref.ID, change.ProjectID, change.ID, ref.ArtifactID, ref.Role, ref.Ordinal, stamp(created), ref.Kind, ref.SchemaVersion, ref.Summary, ref.SourceRevision); err != nil {
		return domain.ArtifactRef{}, fmt.Errorf("insert planning artifact reference: %w", err)
	}
	if err := insertArtifactRefLinks(ctx, tx, ref.ID, "input", ref.InputArtifactRefIDs); err != nil {
		return domain.ArtifactRef{}, err
	}
	if err := insertArtifactRefLinks(ctx, tx, ref.ID, "raw_log", ref.RawLogArtifactRefIDs); err != nil {
		return domain.ArtifactRef{}, err
	}
	return ref, nil
}

func nextArtifactRefOrdinal(ctx context.Context, tx *sql.Tx, changeID domain.ChangeID, role string) (int, error) {
	var ordinal int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(ordinal), -1) + 1 FROM t_artifact_refs WHERE change_id = ? AND role = ?`, changeID, role).Scan(&ordinal); err != nil {
		return 0, fmt.Errorf("read planning artifact ordinal: %w", err)
	}
	return ordinal, nil
}

func insertArtifactRefLinks(ctx context.Context, tx *sql.Tx, source domain.ArtifactRefID, relation string, refs []domain.ArtifactRefID) error {
	for ordinal, refID := range refs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_artifact_ref_links (artifact_ref_id, linked_artifact_ref_id, relation, ordinal) VALUES (?, ?, ?, ?)`, source, refID, relation, ordinal); err != nil {
			return fmt.Errorf("insert planning artifact %s link: %w", relation, err)
		}
	}
	return nil
}

func (s *Store) persistPlanningRawLogs(ctx context.Context, tx *sql.Tx, change domain.Change, role string, completion planningCompletion, created time.Time) ([]domain.ArtifactRef, error) {
	refs, err := readArtifactRefs(ctx, tx, change.ID, completion.artifact.RawLogArtifactRefIDs)
	if err != nil {
		return nil, err
	}
	for _, rawLog := range completion.rawLogs {
		ref, err := insertPlanningArtifactRef(ctx, tx, change, rawLog, role, created)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func finishPlanningRun(ctx context.Context, tx *sql.Tx, run *domain.AgentRun, artifact domain.ArtifactRef, rawRefs []domain.ArtifactRef, outcome string, completed time.Time) error {
	if err := run.Complete(outcome, completed); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE t_agent_runs SET status = ?, outcome = ?, completed_at = ? WHERE agent_run_id = ? AND status = 'running'`, run.Status, run.Outcome, stamp(completed), run.ID)
	if err != nil {
		return fmt.Errorf("complete planning run: %w", err)
	}
	if err := checkVersionUpdate(result); err != nil {
		return domain.ErrPlanningRunConflict
	}
	links := []domain.AgentRunArtifact{{ArtifactRefID: artifact.ID, Role: artifact.Role, Ordinal: 0}}
	next := map[string]int{artifact.Role: 1}
	for _, ref := range rawRefs {
		links = append(links, domain.AgentRunArtifact{ArtifactRefID: ref.ID, Role: ref.Role, Ordinal: next[ref.Role]})
		next[ref.Role]++
	}
	if err := insertAgentRunArtifacts(ctx, tx, run.ID, links); err != nil {
		return err
	}
	run.Artifacts = append(run.Artifacts, links...)
	return nil
}

func applyPlanningResult(ctx context.Context, tx *sql.Tx, change domain.Change, run domain.AgentRun, completion planningCompletion, completed time.Time, refs []domain.ArtifactRefID) (string, domain.Change, error) {
	current, err := isCurrentPlanningRun(ctx, tx, change, run)
	if err != nil {
		return "", change, err
	}
	if !current || change.Status != domain.ChangeStatusActive || change.Version != completion.expectedVersion {
		return work.PlanningCommitFenced, change, nil
	}
	updated := change
	if completion.outcome == domain.AgentRunOutcomeFailed {
		updated, err = change.EnterHumanRequired()
		if err == nil {
			err = updateChangeStatus(ctx, tx, change, updated, completed)
		}
	} else {
		updated.Stage = run.Stage
		if run.Stage == domain.LifecycleStagePlan {
			updated.Stage = domain.LifecycleStageTicketize
		}
		updated.Version++
		err = updateChangeStage(ctx, tx, change, updated, completed)
	}
	if err != nil {
		return "", change, err
	}
	updated.UpdatedAt = completed
	eventType := domain.StageAdvancedType
	if completion.outcome == domain.AgentRunOutcomeFailed {
		eventType = domain.ChangeHumanRequiredType
	}
	if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, eventType, completion.actor, completed, &run.ID, nil, refs); err != nil {
		return "", change, err
	}
	return work.PlanningCommitCommitted, updated, nil
}

func isCurrentPlanningRun(ctx context.Context, tx *sql.Tx, change domain.Change, run domain.AgentRun) (bool, error) {
	if !planningTargetFollows(change.Stage, run.Stage) || change.BaseRevision != run.SourceRevision {
		return false, nil
	}
	var latest sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM t_agent_runs WHERE change_id = ? AND stage = ? AND run_kind = 'planning'`, change.ID, run.Stage).Scan(&latest); err != nil {
		return false, fmt.Errorf("read current planning attempt: %w", err)
	}
	return latest.Valid && int(latest.Int64) == run.Attempt, nil
}

func recordPlanningCommit(ctx context.Context, tx *sql.Tx, runID domain.AgentRunID, refID domain.ArtifactRefID, outcome, disposition string, completed time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO t_planning_stage_commits (agent_run_id, artifact_ref_id, outcome, disposition, completed_at) VALUES (?, ?, ?, ?, ?)`, runID, refID, outcome, disposition, stamp(completed))
	if err != nil {
		return fmt.Errorf("record planning completion: %w", err)
	}
	return nil
}

func artifactRefIDs(refs []domain.ArtifactRef) []domain.ArtifactRefID {
	ids := make([]domain.ArtifactRefID, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	return ids
}

func planningRunInputRefIDs(run domain.AgentRun) []domain.ArtifactRefID {
	refs := make([]domain.ArtifactRefID, 0)
	for _, artifact := range run.Artifacts {
		if artifact.Role == domain.ArtifactRoleInput {
			refs = append(refs, artifact.ArtifactRefID)
		}
	}
	return refs
}

func planningArtifactMatches(ctx context.Context, queryer sqlQueryContext, ref domain.ArtifactRef, input work.PlanningArtifactWrite, allowAdditionalRawLogs bool) bool {
	rawLogsMatch := sameArtifactRefIDs(ref.RawLogArtifactRefIDs, input.RawLogArtifactRefIDs)
	if allowAdditionalRawLogs {
		rawLogsMatch = hasArtifactRefPrefix(ref.RawLogArtifactRefIDs, input.RawLogArtifactRefIDs)
	}
	if ref.Kind != input.Kind || ref.SchemaVersion != input.SchemaVersion || ref.Summary != input.Summary || ref.SourceRevision != input.SourceRevision || !sameArtifactRefIDs(ref.InputArtifactRefIDs, input.InputArtifactRefIDs) || !rawLogsMatch {
		return false
	}
	var identity domain.ArtifactIdentity
	err := queryer.QueryRowContext(ctx, `SELECT sha256, byte_length FROM t_artifacts WHERE artifact_id = ?`, ref.ArtifactID).Scan(&identity.SHA256, &identity.ByteLength)
	return err == nil && identity == input.Identity
}

func planningRawLogsMatch(ctx context.Context, queryer sqlQueryContext, refs []domain.ArtifactRef, completion planningCompletion) bool {
	existing := completion.artifact.RawLogArtifactRefIDs
	if len(refs) != len(existing)+len(completion.rawLogs) {
		return false
	}
	for index, refID := range existing {
		if refs[index].ID != refID {
			return false
		}
	}
	role := planningOutcomeRole(completion.outcome)
	for index, input := range completion.rawLogs {
		ref := refs[len(existing)+index]
		if ref.Role != role || !planningArtifactMatches(ctx, queryer, ref, input, false) {
			return false
		}
	}
	return true
}

func hasArtifactRefPrefix(all, prefix []domain.ArtifactRefID) bool {
	return len(all) >= len(prefix) && sameArtifactRefIDs(all[:len(prefix)], prefix)
}

func sameArtifactRefIDs(left, right []domain.ArtifactRefID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *Store) persistPlanningCandidateReport(ctx context.Context, tx *sql.Tx, request workercontract.Report, digest string, prepared []preparedWorkerArtifact, run domain.AgentRun, change domain.Change, lease workerLease, artifacts WorkerArtifactStore, eligible bool) (workercontract.ReportResponse, error) {
	if err := validatePlanningCandidateReport(request, prepared); err != nil {
		return workercontract.ReportResponse{}, err
	}
	if err := storePreparedReportArtifacts(ctx, prepared, artifacts); err != nil {
		if run.Stage == domain.LifecycleStageTicketize {
			// Ticketize 的候选尚未形成任何权威成功或失败事实；内容存储
			// 暂不可用时回滚本次事务，保留 Lease 让同一 Report 可重送。
			return workercontract.ReportResponse{}, fmt.Errorf("store ticketize report artifacts: %w", ErrWorkerUnavailable)
		}
		return s.persistPlanningArtifactFailureCandidate(ctx, tx, request, digest, run, change, lease, eligible)
	}
	now := s.now().UTC()
	disposition, leaseState, candidateOutcome, failureReason, err := settlePlanningCandidateLease(ctx, tx, request, digest, lease, now, eligible)
	if err != nil {
		return workercontract.ReportResponse{}, err
	}
	role := planningOutcomeRole(candidateOutcome)
	refs, err := insertPlanningCandidateRefs(ctx, tx, change, run, prepared, role, now)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("persist planning candidate artifacts: %w", ErrWorkerUnavailable)
	}
	guardJSON, err := json.Marshal(request.GuardFindings)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("encode planning candidate findings: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_planning_run_candidates (agent_run_id, report_digest, outcome, exit_code, after_revision, failure_reason, guard_findings_json, candidate_truncated, received_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.ID, digest, candidateOutcome, request.ExitCode, request.AfterRevision, failureReason, string(guardJSON), planningCandidateTruncated(prepared), stamp(now)); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record planning candidate: %w", ErrWorkerUnavailable)
	}
	for ordinal, ref := range refs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_planning_candidate_artifacts (agent_run_id, artifact_ref_id, kind, ordinal) VALUES (?, ?, ?, ?)`, run.ID, ref.ID, ref.Kind, ordinal); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("link planning candidate artifact: %w", ErrWorkerUnavailable)
		}
	}
	if disposition == "late" {
		artifactIDs := make([]domain.ArtifactRefID, 0, len(refs))
		for _, ref := range refs {
			artifactIDs = append(artifactIDs, ref.ID)
		}
		if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunReportLateType, "worker_late", now, &run.ID, nil, artifactIDs); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("record late planning report: %w", ErrWorkerUnavailable)
		}
	}
	return workercontract.ReportResponse{Disposition: disposition, AgentRunID: request.AgentRunID, LeaseState: leaseState}, nil
}

// persistPlanningArtifactFailureCandidate 在外部内容存储不可用时仍用 SQLite
// 固定本次 Report 的终态，避免 Worker 对同一不可落盘结果无限重试。
func (s *Store) persistPlanningArtifactFailureCandidate(ctx context.Context, tx *sql.Tx, request workercontract.Report, digest string, run domain.AgentRun, change domain.Change, lease workerLease, eligible bool) (workercontract.ReportResponse, error) {
	failure := request
	failure.Outcome = workercontract.Outcome(domain.AgentRunOutcomeFailed)
	failure.FailureReason = "artifact_unavailable"
	now := s.now().UTC()
	disposition, leaseState, _, failureReason, err := settlePlanningCandidateLease(ctx, tx, failure, digest, lease, now, eligible)
	if err != nil {
		return workercontract.ReportResponse{}, err
	}
	guardJSON, err := json.Marshal(request.GuardFindings)
	if err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("encode unavailable planning artifact findings: %w", ErrWorkerUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_planning_run_candidates (agent_run_id, report_digest, outcome, exit_code, after_revision, failure_reason, guard_findings_json, candidate_truncated, received_at) VALUES (?, ?, 'failed', ?, ?, ?, ?, 0, ?)`, run.ID, digest, request.ExitCode, request.AfterRevision, failureReason, string(guardJSON), stamp(now)); err != nil {
		return workercontract.ReportResponse{}, fmt.Errorf("record unavailable planning artifact candidate: %w", ErrWorkerUnavailable)
	}
	if disposition == "late" {
		if err := insertEventTx(ctx, tx, change.ProjectID, change.ID, domain.AgentRunReportLateType, "worker_late", now, &run.ID, nil, nil); err != nil {
			return workercontract.ReportResponse{}, fmt.Errorf("record unavailable late planning report: %w", ErrWorkerUnavailable)
		}
	}
	return workercontract.ReportResponse{Disposition: disposition, AgentRunID: request.AgentRunID, LeaseState: leaseState}, nil
}

func planningCandidateExists(ctx context.Context, queryer sqlQueryer, runID domain.AgentRunID) (bool, error) {
	var count int
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_planning_run_candidates WHERE agent_run_id = ?`, runID).Scan(&count); err != nil {
		return false, fmt.Errorf("check planning candidate: %w", ErrWorkerUnavailable)
	}
	return count != 0, nil
}

func validatePlanningCandidateReport(request workercontract.Report, prepared []preparedWorkerArtifact) error {
	if len(request.FailureReason) > 8<<10 || len(request.GuardFindings) > 64 {
		return ErrWorkerReportInvalid
	}
	for _, finding := range request.GuardFindings {
		if len(finding) > 8<<10 || !utf8.ValidString(finding) {
			return ErrWorkerReportInvalid
		}
	}
	if len(request.CaptureFailures) > 64 {
		return ErrWorkerReportInvalid
	}
	for _, failure := range request.CaptureFailures {
		if strings.TrimSpace(failure.Kind) == "" || strings.TrimSpace(failure.Stage) == "" || len(failure.Kind) > 64 || len(failure.Stage) > 128 || len(failure.Error) > 8<<10 || !utf8.ValidString(failure.Kind) || !utf8.ValidString(failure.Stage) || !utf8.ValidString(failure.Error) {
			return ErrWorkerReportInvalid
		}
	}
	candidates := 0
	for _, artifact := range prepared {
		if artifact.request.Kind == "candidate" {
			candidates++
		}
	}
	declaredSuccess := request.Outcome == workercontract.Outcome(domain.AgentRunOutcomeSucceeded)
	if candidates > 1 || (declaredSuccess && (reportOutcome(request) != domain.AgentRunOutcomeSucceeded || candidates != 1)) {
		return ErrWorkerReportInvalid
	}
	return nil
}

func planningCandidateTruncated(artifacts []preparedWorkerArtifact) bool {
	for _, artifact := range artifacts {
		if artifact.request.Kind == "candidate" {
			return artifact.request.Truncated
		}
	}
	return false
}

func insertPlanningCandidateRefs(ctx context.Context, tx *sql.Tx, change domain.Change, run domain.AgentRun, prepared []preparedWorkerArtifact, role string, now time.Time) ([]domain.ArtifactRef, error) {
	inputs := make([]domain.ArtifactRefID, 0)
	for _, artifact := range run.Artifacts {
		if artifact.Role == domain.ArtifactRoleInput {
			inputs = append(inputs, artifact.ArtifactRefID)
		}
	}
	ordered := append([]preparedWorkerArtifact(nil), prepared...)
	stableCandidateFirst(ordered)
	refs := make([]domain.ArtifactRef, 0, len(ordered))
	for _, item := range ordered {
		kind := item.request.Kind
		schemaVersion := ""
		summary := ""
		if item.request.Kind == "candidate" && run.Stage == domain.LifecycleStageTicketize {
			kind = "ticket_draft"
			schemaVersion = "keystone.ticket-draft.v1"
			summary = "Ticket Draft candidate"
		}
		write := work.PlanningArtifactWrite{Identity: item.identity, MediaType: mediaTypeForWorkerArtifact(item.request.Kind), Kind: kind, SchemaVersion: schemaVersion, Summary: summary, SourceRevision: run.SourceRevision}
		if item.request.Kind == "candidate" {
			write.InputArtifactRefIDs = append([]domain.ArtifactRefID(nil), inputs...)
		}
		ref, err := insertPlanningArtifactRef(ctx, tx, change, write, role, now)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func stableCandidateFirst(artifacts []preparedWorkerArtifact) {
	for index := range artifacts {
		if artifacts[index].request.Kind != "candidate" || index == 0 {
			continue
		}
		candidate := artifacts[index]
		copy(artifacts[1:index+1], artifacts[0:index])
		artifacts[0] = candidate
		return
	}
}

func settlePlanningCandidateLease(ctx context.Context, tx *sql.Tx, request workercontract.Report, digest string, lease workerLease, now time.Time, eligible bool) (disposition, state, candidateOutcome, failureReason string, err error) {
	var expiresAt string
	if err := tx.QueryRowContext(ctx, `SELECT state, expires_at FROM t_worker_leases WHERE lease_id = ?`, lease.LeaseID).Scan(&state, &expiresAt); err != nil {
		return "", "", "", "", fmt.Errorf("refresh planning candidate lease: %w", ErrWorkerUnavailable)
	}
	deadline, deadlineErr := parseStamp(expiresAt)
	if eligible && state == "active" && deadlineErr == nil && now.Before(deadline) {
		result, updateErr := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = 'consumed', consumed_at = ?, report_digest = ?, report_disposition = 'candidate_received', report_outcome = ?, workspace_path = '' WHERE lease_id = ? AND state = 'active' AND expires_at = ?`, stamp(now), digest, reportOutcome(request), lease.LeaseID, expiresAt)
		if updateErr != nil {
			return "", "", "", "", fmt.Errorf("consume planning candidate lease: %w", ErrWorkerUnavailable)
		}
		count, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return "", "", "", "", fmt.Errorf("consume planning candidate lease: %w", ErrWorkerUnavailable)
		}
		if count == 1 {
			if receiptErr := insertWorkerReportReceipt(ctx, tx, digest, request.AgentRunID, lease.LeaseID, lease.WorkerID, "candidate_received", "", reportOutcome(request), now); receiptErr != nil {
				return "", "", "", "", fmt.Errorf("record planning candidate report: %w", ErrWorkerUnavailable)
			}
			return "candidate_received", "consumed", reportOutcome(request), request.FailureReason, nil
		}
		if err := tx.QueryRowContext(ctx, `SELECT state, expires_at FROM t_worker_leases WHERE lease_id = ?`, lease.LeaseID).Scan(&state, &expiresAt); err != nil {
			return "", "", "", "", fmt.Errorf("refresh fenced planning lease: %w", ErrWorkerUnavailable)
		}
		deadline, deadlineErr = parseStamp(expiresAt)
	}
	if state == "active" {
		nextState := "revoked"
		if deadlineErr != nil || !now.Before(deadline) {
			nextState = "expired"
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE t_worker_leases SET state = ?, workspace_path = '' WHERE lease_id = ? AND state = 'active' AND expires_at = ?`, nextState, lease.LeaseID, expiresAt)
		if updateErr != nil {
			return "", "", "", "", fmt.Errorf("fence late planning lease: %w", ErrWorkerUnavailable)
		}
		count, rowsErr := result.RowsAffected()
		if rowsErr != nil || count != 1 {
			return "", "", "", "", fmt.Errorf("fence late planning lease: %w", ErrWorkerUnavailable)
		}
		state = nextState
	}
	if state != "expired" && state != "revoked" {
		return "", "", "", "", fmt.Errorf("record late planning report: %w", ErrWorkerUnavailable)
	}
	failureReason = "lease_revoked"
	if state == "expired" {
		failureReason = "lease_expired"
	}
	result, updateErr := tx.ExecContext(ctx, `UPDATE t_worker_leases SET consumed_at = ?, report_digest = ?, report_disposition = 'late', report_outcome = ?, workspace_path = '' WHERE lease_id = ? AND state = ? AND report_digest IS NULL`, stamp(now), digest, reportOutcome(request), lease.LeaseID, state)
	if updateErr != nil {
		return "", "", "", "", fmt.Errorf("record late planning lease: %w", ErrWorkerUnavailable)
	}
	count, rowsErr := result.RowsAffected()
	if rowsErr != nil || count != 1 {
		return "", "", "", "", fmt.Errorf("record late planning lease: %w", ErrWorkerUnavailable)
	}
	if receiptErr := insertWorkerReportReceipt(ctx, tx, digest, request.AgentRunID, lease.LeaseID, lease.WorkerID, "late", "lease_not_active", reportOutcome(request), now); receiptErr != nil {
		return "", "", "", "", fmt.Errorf("record late planning report receipt: %w", ErrWorkerUnavailable)
	}
	return "late", state, domain.AgentRunOutcomeFailed, failureReason, nil
}

func readArtifactRefs(ctx context.Context, queryer sqlQueryContext, changeID domain.ChangeID, ids []domain.ArtifactRefID) ([]domain.ArtifactRef, error) {
	refs := make([]domain.ArtifactRef, 0, len(ids))
	for _, refID := range ids {
		ref, err := readArtifactRef(ctx, queryer, changeID, refID)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func readArtifactRef(ctx context.Context, queryer sqlQueryContext, changeID domain.ChangeID, refID domain.ArtifactRefID) (domain.ArtifactRef, error) {
	var ref domain.ArtifactRef
	err := queryer.QueryRowContext(ctx, `SELECT artifact_ref_id, change_id, artifact_id, role, ordinal, kind, schema_version, summary, source_revision FROM t_artifact_refs WHERE artifact_ref_id = ? AND change_id = ?`, refID, changeID).
		Scan(&ref.ID, &ref.ChangeID, &ref.ArtifactID, &ref.Role, &ref.Ordinal, &ref.Kind, &ref.SchemaVersion, &ref.Summary, &ref.SourceRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return ref, domain.ErrArtifactNotFound
	}
	if err != nil {
		return ref, fmt.Errorf("read artifact reference: %w", err)
	}
	ref.InputArtifactRefIDs, err = readArtifactRefLinks(ctx, queryer, ref.ID, "input")
	if err != nil {
		return domain.ArtifactRef{}, err
	}
	ref.RawLogArtifactRefIDs, err = readArtifactRefLinks(ctx, queryer, ref.ID, "raw_log")
	return ref, err
}

func readArtifactRefLinks(ctx context.Context, queryer sqlQueryContext, refID domain.ArtifactRefID, relation string) ([]domain.ArtifactRefID, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT linked_artifact_ref_id FROM t_artifact_ref_links WHERE artifact_ref_id = ? AND relation = ? ORDER BY ordinal`, refID, relation)
	if err != nil {
		return nil, fmt.Errorf("read artifact reference %s links: %w", relation, err)
	}
	defer rows.Close()
	var refs []domain.ArtifactRefID
	for rows.Next() {
		var linked domain.ArtifactRefID
		if err := rows.Scan(&linked); err != nil {
			return nil, err
		}
		refs = append(refs, linked)
	}
	return refs, rows.Err()
}

func listPlanningCandidateRefs(ctx context.Context, queryer sqlQueryContext, runID domain.AgentRunID) ([]domain.ArtifactRef, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT r.change_id, a.artifact_ref_id FROM t_planning_candidate_artifacts a JOIN t_artifact_refs r ON r.artifact_ref_id = a.artifact_ref_id WHERE a.agent_run_id = ? ORDER BY a.ordinal`, runID)
	if err != nil {
		return nil, fmt.Errorf("list planning candidate artifacts: %w", err)
	}
	var changeID domain.ChangeID
	var ids []domain.ArtifactRefID
	for rows.Next() {
		var refID domain.ArtifactRefID
		if err := rows.Scan(&changeID, &refID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, refID)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return readArtifactRefs(ctx, queryer, changeID, ids)
}

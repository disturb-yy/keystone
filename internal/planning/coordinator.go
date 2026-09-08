package planning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	planningActor               = "planning_coordinator"
	planningFailureSchemaV1     = "PlanningFailure.v1"
	planningFailureKind         = "planning_failure"
	planningFailureEvidenceKind = "planning_failure_evidence"
	planningFailureMediaType    = "application/json; charset=utf-8"
	planningCandidateMediaType  = "application/json; charset=utf-8"
	planningContextKind         = "project_context"
	planningContextSummary      = "Planning project context"
	ticketizeGeneratorV1        = "ticketize.v1"
)

// CoordinatorState 是 Planning 对 Work authority 的查询与提交边界。
type CoordinatorState interface {
	work.PlanningStatePort
	FindChange(context.Context, domain.ChangeID) (domain.Change, error)
	ListAgentRuns(context.Context, domain.ChangeID) ([]domain.AgentRun, error)
	ListArtifactRefs(context.Context, domain.ChangeID) ([]domain.ArtifactRef, error)
	FindArtifact(context.Context, domain.ChangeID, domain.ArtifactRefID) (domain.Artifact, error)
}

// ArtifactStore 是 Coordinator 读取输入和写入失败说明使用的内容寻址存储端口。
type ArtifactStore interface {
	Put(context.Context, []byte) (domain.ArtifactIdentity, error)
	Read(context.Context, domain.ArtifactIdentity) ([]byte, error)
}

// Snapshot 是一次固定 revision、需要显式清理的隔离 source handle。
type Snapshot interface {
	Root() string
	Revision() string
	Close() error
}

// SnapshotMaterializer 只为当前 Planning attempt 创建隔离 source Snapshot。
type SnapshotMaterializer interface {
	Materialize(context.Context, string, string) (Snapshot, error)
}

// DispatchArtifact 是下发给 Worker 的非内容输入摘要。
type DispatchArtifact struct {
	Kind      string
	SHA256    string
	SizeBytes int64
	MediaType string
}

// DispatchRequest 是 Coordinator 交给执行边界的单次 Planning Assignment。
type DispatchRequest struct {
	Target         string
	AgentRunID     domain.AgentRunID
	Attempt        int
	WorkspacePath  string
	Instruction    string
	BeforeRevision string
	Inputs         []DispatchArtifact
}

// Dispatcher 选择具备能力的执行目标并原子登记 Assignment。
type Dispatcher interface {
	Select(context.Context, RuntimeCapability) (target string, available bool, err error)
	Assigned(context.Context, domain.AgentRunID) (bool, error)
	Dispatch(context.Context, DispatchRequest) error
}

// Coordinator 串行协调耐久 Change、非权威 Runtime candidate 与权威 Work 提交。
type Coordinator struct {
	state      CoordinatorState
	artifacts  ArtifactStore
	snapshots  SnapshotMaterializer
	dispatcher Dispatcher
	strategies map[Stage]StageStrategy

	reconcileMu sync.Mutex
	snapshotMu  sync.Mutex
	owned       map[domain.AgentRunID]ownedSnapshot
}

// NewCoordinator 创建只通过公开端口协调的 Planning Application Service。
func NewCoordinator(state CoordinatorState, artifacts ArtifactStore, snapshots SnapshotMaterializer, dispatcher Dispatcher) (*Coordinator, error) {
	if state == nil || artifacts == nil || snapshots == nil || dispatcher == nil {
		return nil, errors.New("create planning coordinator: all ports are required")
	}
	understand, err := NewStrategy(StageUnderstand, nil, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	design, err := NewStrategy(StageDesign, nil, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	plan, err := NewStrategy(StagePlan, nil, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Coordinator{
		state: state, artifacts: artifacts, snapshots: snapshots, dispatcher: dispatcher,
		strategies: map[Stage]StageStrategy{StageUnderstand: understand, StageDesign: design, StagePlan: plan},
		owned:      make(map[domain.AgentRunID]ownedSnapshot),
	}, nil
}

// Recover 从 Workstore 的耐久查询结果恢复或推进当前有效的 Planning attempt。
func (c *Coordinator) Recover(ctx context.Context) error {
	if ctx == nil {
		return errors.New("recover planning: nil context")
	}
	c.reconcileMu.Lock()
	defer c.reconcileMu.Unlock()
	if err := c.state.ReconcilePlanningExecutions(ctx); err != nil {
		return fmt.Errorf("reconcile planning executions: %w", err)
	}
	if err := c.releaseFinishedSnapshots(ctx); err != nil {
		return err
	}
	changes, err := c.state.ListRecoverablePlanningChanges(ctx)
	if err != nil {
		return fmt.Errorf("recover planning changes: %w", err)
	}
	var recoverErr error
	for _, change := range changes {
		if err := c.reconcileChange(ctx, change); err != nil {
			recoverErr = errors.Join(recoverErr, fmt.Errorf("reconcile planning change %s: %w", change.ID, err))
		}
	}
	return recoverErr
}

// Close 清理当前进程拥有的全部临时 Snapshot；可重复调用。
func (c *Coordinator) Close() error {
	if c == nil {
		return nil
	}
	c.reconcileMu.Lock()
	defer c.reconcileMu.Unlock()
	c.snapshotMu.Lock()
	runIDs := make([]domain.AgentRunID, 0, len(c.owned))
	for runID := range c.owned {
		runIDs = append(runIDs, runID)
	}
	c.snapshotMu.Unlock()
	var closeErr error
	for _, runID := range runIDs {
		closeErr = errors.Join(closeErr, c.releaseSnapshot(runID))
	}
	return closeErr
}

func (c *Coordinator) reconcileChange(ctx context.Context, change domain.Change) error {
	run, err := c.runningPlanningRun(ctx, change)
	if err != nil {
		return err
	}
	if run != nil {
		candidate, err := c.state.FindPlanningRunCandidate(ctx, run.ID)
		if errors.Is(err, domain.ErrPlanningCandidateNotFound) {
			assigned, assignedErr := c.dispatcher.Assigned(ctx, run.ID)
			if assignedErr != nil {
				return fmt.Errorf("inspect planning assignment: %w", assignedErr)
			}
			if assigned {
				return nil
			}
			if closeErr := c.releaseSnapshot(run.ID); closeErr != nil {
				return closeErr
			}
			if change.Status == domain.ChangeStatusPaused {
				return nil
			}
			switch change.Status {
			case domain.ChangeStatusActive:
				return c.dispatchExistingRun(ctx, change, *run)
			case domain.ChangeStatusCancelled:
				return c.failBeforeCandidate(ctx, change, *run, "change_cancelled")
			case domain.ChangeStatusHumanRequired:
				return c.failBeforeCandidate(ctx, change, *run, "authority_fenced")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("find candidate: %w", err)
		}
		if change.Status == domain.ChangeStatusPaused {
			return c.releaseSnapshot(run.ID)
		}
		if run.Stage == domain.LifecycleStageTicketize {
			return c.settleTicketizeCandidate(ctx, change, *run, candidate)
		}
		return c.settleCandidate(ctx, change, *run, candidate)
	}
	if change.LatestAgentRun != nil && change.LatestAgentRun.Status == domain.AgentRunStatusRunning {
		return nil
	}
	if change.Status != domain.ChangeStatusActive {
		return nil
	}
	return c.startStage(ctx, change)
}

func (c *Coordinator) startStage(ctx context.Context, change domain.Change) error {
	stage, ok := targetStage(change.Stage)
	if !ok {
		return nil
	}
	if stage == StageTicketize {
		return c.startTicketizeStage(ctx, change)
	}
	strategy := c.strategies[stage]
	target, available, err := c.dispatcher.Select(ctx, strategy.Definition().Capability)
	if err != nil {
		return fmt.Errorf("select planning worker: %w", err)
	}
	if !available {
		return nil
	}
	input, inputRef, inputArtifact, preparationErr := c.loadStageInput(ctx, change, stage)
	if inputRef.ID == "" {
		inputRef = change.Intent
		input = newStageInput(change, stage, inputRef.ID)
	}
	inputRefIDs := []domain.ArtifactRefID{inputRef.ID}
	var inputWrites []work.PlanningArtifactWrite
	contextArtifact, contextRef, contextErr := c.prepareProjectContext(ctx, change, input.ProjectContext)
	if contextErr == nil {
		if contextRef.ID != "" {
			inputRefIDs = append(inputRefIDs, contextRef.ID)
		} else {
			inputWrites = append(inputWrites, work.PlanningArtifactWrite{
				Identity: contextArtifact.Identity, MediaType: contextArtifact.MediaType,
				Kind: planningContextKind, SchemaVersion: ProjectContextSchemaV1,
				Summary: planningContextSummary, SourceRevision: change.BaseRevision,
			})
		}
	}
	run, err := c.state.StartPlanningRun(ctx, work.StartPlanningRunRequest{
		ChangeID: change.ID, TargetStage: lifecycleStage(stage), ExpectedChangeVersion: change.Version,
		SourceRevision: change.BaseRevision, Actor: planningActor,
		InputArtifactRefIDs: inputRefIDs, InputArtifacts: inputWrites,
	})
	if errors.Is(err, domain.ErrPlanningRunConflict) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("start planning run: %w", err)
	}
	if contextErr != nil {
		return c.failBeforeCandidate(ctx, change, run, "artifact_unavailable")
	}
	if preparationErr != nil {
		return c.failBeforeCandidate(ctx, change, run, classifyCoordinatorError(preparationErr))
	}
	return c.dispatchPreparedRun(ctx, change, run, target, strategy, input, inputArtifact, contextArtifact)
}

func (c *Coordinator) startTicketizeStage(ctx context.Context, change domain.Change) error {
	port, ok := c.state.(work.TicketizeStatePort)
	if !ok {
		return nil
	}
	target, available, err := c.dispatcher.Select(ctx, RuntimeCapabilityPlanningReadOnly)
	if err != nil {
		return fmt.Errorf("select ticketize worker: %w", err)
	}
	if !available {
		return nil
	}
	input, planRef, planArtifact, err := c.loadTicketizeInput(ctx, change)
	if err != nil {
		if errors.Is(err, domain.ErrTicketizeUnavailable) {
			return err
		}
		return fmt.Errorf("load ticketize input: %w", err)
	}
	inputRefs := []domain.ArtifactRefID{planRef.ID}
	var inputWrites []work.PlanningArtifactWrite
	contextArtifact, contextRef, contextErr := c.prepareProjectContext(ctx, change, input.ProjectContext)
	if contextErr != nil {
		return errors.Join(fmt.Errorf("prepare ticketize context: %w", contextErr), domain.ErrTicketizeUnavailable)
	}
	if contextRef.ID != "" {
		inputRefs = append(inputRefs, contextRef.ID)
	} else {
		inputWrites = append(inputWrites, work.PlanningArtifactWrite{Identity: contextArtifact.Identity, MediaType: contextArtifact.MediaType, Kind: planningContextKind, SchemaVersion: ProjectContextSchemaV1, Summary: planningContextSummary, SourceRevision: change.BaseRevision})
	}
	run, err := port.StartTicketizeRun(ctx, work.StartPlanningRunRequest{
		ChangeID: change.ID, TargetStage: domain.LifecycleStageTicketize, ExpectedChangeVersion: change.Version,
		SourceRevision: change.BaseRevision, Actor: planningActor, InputArtifactRefIDs: inputRefs, InputArtifacts: inputWrites,
	})
	if errors.Is(err, domain.ErrPlanningRunConflict) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("start ticketize run: %w", err)
	}
	return c.dispatchTicketizePreparedRun(ctx, change, run, target, input, planArtifact, contextArtifact)
}

func (c *Coordinator) dispatchExistingRun(ctx context.Context, change domain.Change, run domain.AgentRun) error {
	stage := stageFromLifecycle(run.Stage)
	if stage == StageTicketize {
		return c.dispatchExistingTicketizeRun(ctx, change, run)
	}
	strategy, ok := c.strategies[stage]
	if !ok {
		return c.failBeforeCandidate(ctx, change, run, "coordination_failed")
	}
	target, available, err := c.dispatcher.Select(ctx, strategy.Definition().Capability)
	if err != nil {
		return fmt.Errorf("select planning worker: %w", err)
	}
	if !available {
		return nil
	}
	input, _, inputArtifact, err := c.loadStageInput(ctx, change, stage)
	if err != nil {
		return c.failBeforeCandidate(ctx, change, run, classifyCoordinatorError(err))
	}
	contextArtifact, _, err := c.prepareProjectContext(ctx, change, input.ProjectContext)
	if err != nil {
		return c.failBeforeCandidate(ctx, change, run, "artifact_unavailable")
	}
	return c.dispatchPreparedRun(ctx, change, run, target, strategy, input, inputArtifact, contextArtifact)
}

func (c *Coordinator) dispatchExistingTicketizeRun(ctx context.Context, change domain.Change, run domain.AgentRun) error {
	input, _, planArtifact, err := c.loadTicketizeInput(ctx, change)
	if err != nil {
		if errors.Is(err, domain.ErrTicketizeUnavailable) {
			return err
		}
		return c.failBeforeCandidate(ctx, change, run, classifyCoordinatorError(err))
	}
	target, available, err := c.dispatcher.Select(ctx, RuntimeCapabilityPlanningReadOnly)
	if err != nil {
		return fmt.Errorf("select ticketize worker: %w", err)
	}
	if !available {
		return nil
	}
	contextArtifact, _, err := c.prepareProjectContext(ctx, change, input.ProjectContext)
	if err != nil {
		return errors.Join(fmt.Errorf("prepare ticketize context: %w", err), domain.ErrTicketizeUnavailable)
	}
	return c.dispatchTicketizePreparedRun(ctx, change, run, target, input, planArtifact, contextArtifact)
}

func (c *Coordinator) dispatchTicketizePreparedRun(ctx context.Context, change domain.Change, run domain.AgentRun, target string, input TicketGenerationInput, planArtifact, contextArtifact domain.Artifact) error {
	instruction, err := BuildTicketizePrompt(input)
	if err != nil {
		return c.failBeforeCandidate(ctx, change, run, classifyCoordinatorError(err))
	}
	snapshot, err := c.snapshots.Materialize(ctx, change.RepositoryRoot, change.BaseRevision)
	if err != nil {
		return c.failBeforeCandidate(ctx, change, run, "snapshot_failed")
	}
	if snapshot == nil || snapshot.Root() == "" || snapshot.Revision() != change.BaseRevision {
		var closeErr error
		if snapshot != nil {
			closeErr = snapshot.Close()
		}
		return errors.Join(c.failBeforeCandidate(ctx, change, run, "revision_mismatch"), closeErr)
	}
	c.rememberSnapshot(run.ID, change.ID, snapshot)
	err = c.dispatcher.Dispatch(ctx, DispatchRequest{
		Target: target, AgentRunID: run.ID, Attempt: run.Attempt, WorkspacePath: snapshot.Root(), Instruction: instruction,
		BeforeRevision: change.BaseRevision,
		Inputs:         []DispatchArtifact{{Kind: string(ArtifactKindPlan), SHA256: planArtifact.Identity.SHA256, SizeBytes: planArtifact.Identity.ByteLength, MediaType: planArtifact.MediaType}, {Kind: planningContextKind, SHA256: contextArtifact.Identity.SHA256, SizeBytes: contextArtifact.Identity.ByteLength, MediaType: contextArtifact.MediaType}},
	})
	if err != nil {
		assigned, assignedErr := c.dispatcher.Assigned(ctx, run.ID)
		if assignedErr != nil {
			return errors.Join(fmt.Errorf("inspect failed ticketize dispatch: %w", assignedErr), err)
		}
		if assigned {
			return nil
		}
		closeErr := c.releaseSnapshot(run.ID)
		if errors.Is(err, ErrDispatchFenced) {
			return closeErr
		}
		return errors.Join(c.failBeforeCandidate(ctx, change, run, "dispatch_failed"), closeErr)
	}
	return nil
}

func (c *Coordinator) dispatchPreparedRun(ctx context.Context, change domain.Change, run domain.AgentRun, target string, strategy StageStrategy, input StageInput, inputArtifact, contextArtifact domain.Artifact) error {
	stage := stageFromLifecycle(run.Stage)
	request, err := strategy.Prepare(input)
	if err != nil {
		return c.failBeforeCandidate(ctx, change, run, classifyCoordinatorError(err))
	}
	snapshot, err := c.snapshots.Materialize(ctx, change.RepositoryRoot, change.BaseRevision)
	if err != nil {
		return c.failBeforeCandidate(ctx, change, run, "snapshot_failed")
	}
	if snapshot == nil || snapshot.Root() == "" || snapshot.Revision() != change.BaseRevision {
		var closeErr error
		if snapshot != nil {
			closeErr = snapshot.Close()
		}
		return errors.Join(c.failBeforeCandidate(ctx, change, run, "revision_mismatch"), closeErr)
	}
	c.rememberSnapshot(run.ID, change.ID, snapshot)
	dispatchInputs := []DispatchArtifact{{
		Kind: inputKind(stage), SHA256: inputArtifact.Identity.SHA256,
		SizeBytes: inputArtifact.Identity.ByteLength, MediaType: inputArtifact.MediaType,
	}, {
		Kind: planningContextKind, SHA256: contextArtifact.Identity.SHA256,
		SizeBytes: contextArtifact.Identity.ByteLength, MediaType: contextArtifact.MediaType,
	}}
	err = c.dispatcher.Dispatch(ctx, DispatchRequest{
		Target: target, AgentRunID: run.ID, Attempt: run.Attempt, WorkspacePath: snapshot.Root(),
		Instruction: request.Instruction, BeforeRevision: change.BaseRevision,
		Inputs: dispatchInputs,
	})
	if err != nil {
		assigned, assignedErr := c.dispatcher.Assigned(ctx, run.ID)
		if assignedErr != nil {
			return errors.Join(fmt.Errorf("inspect failed planning dispatch: %w", assignedErr), err)
		}
		if assigned {
			return nil
		}
		closeErr := c.releaseSnapshot(run.ID)
		if errors.Is(err, ErrDispatchFenced) {
			return closeErr
		}
		return errors.Join(c.failBeforeCandidate(ctx, change, run, "dispatch_failed"), closeErr)
	}
	return nil
}

func (c *Coordinator) failBeforeCandidate(ctx context.Context, change domain.Change, run domain.AgentRun, class string) error {
	if err := c.state.RecordPlanningRunFailureCandidate(ctx, run.ID, class); err != nil {
		return fmt.Errorf("record planning failure candidate: %w", err)
	}
	latest, err := c.state.FindChange(ctx, change.ID)
	if err != nil {
		return fmt.Errorf("reload planning failure authority: %w", err)
	}
	if latest.Status == domain.ChangeStatusPaused {
		return c.releaseSnapshot(run.ID)
	}
	candidate, err := c.state.FindPlanningRunCandidate(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("reload planning failure candidate: %w", err)
	}
	return c.settleCandidate(ctx, latest, run, candidate)
}

func (c *Coordinator) runningPlanningRun(ctx context.Context, change domain.Change) (*domain.AgentRun, error) {
	if run := change.LatestAgentRun; run != nil && run.Status == domain.AgentRunStatusRunning && run.IsPlanning() {
		copy := *run
		return &copy, nil
	}
	runs, err := c.state.ListAgentRuns(ctx, change.ID)
	if err != nil {
		return nil, fmt.Errorf("list planning runs: %w", err)
	}
	var selected *domain.AgentRun
	for index := range runs {
		if runs[index].Status != domain.AgentRunStatusRunning || !runs[index].IsPlanning() {
			continue
		}
		if selected != nil {
			return nil, errors.New("multiple running planning runs violate authority")
		}
		copy := runs[index]
		selected = &copy
	}
	return selected, nil
}

func (c *Coordinator) settleCandidate(ctx context.Context, change domain.Change, run domain.AgentRun, result work.PlanningRunCandidate) error {
	evidence := candidateEvidenceRefs(result)
	failureClass := candidateFailureClass(change, run, result)
	if failureClass != "" {
		if err := c.failRun(ctx, change, run, failureClass, evidence); err != nil {
			if errors.Is(err, domain.ErrPlanningRunDeferred) {
				return c.releaseSnapshot(run.ID)
			}
			return err
		}
		return c.releaseSnapshot(run.ID)
	}
	input, _, _, err := c.loadStageInput(ctx, change, stageFromLifecycle(run.Stage))
	if err != nil {
		if failErr := c.failRun(ctx, change, run, classifyCoordinatorError(err), evidence); failErr != nil {
			return errors.Join(err, failErr)
		}
		return c.releaseSnapshot(run.ID)
	}
	candidateArtifact, err := c.state.FindArtifact(ctx, change.ID, result.CandidateRef.ID)
	if err != nil {
		return fmt.Errorf("find candidate artifact: %w", err)
	}
	body, err := c.artifacts.Read(ctx, candidateArtifact.Identity)
	if err != nil {
		if failErr := c.failRun(ctx, change, run, "artifact_unavailable", evidence); failErr != nil {
			return errors.Join(err, failErr)
		}
		return c.releaseSnapshot(run.ID)
	}
	strategy := c.strategies[stageFromLifecycle(run.Stage)]
	candidate, err := strategy.Decode(body, input)
	if err != nil {
		if failErr := c.failRun(ctx, change, run, classifyCoordinatorError(err), evidence); failErr != nil {
			return errors.Join(err, failErr)
		}
		return c.releaseSnapshot(run.ID)
	}
	envelope := candidate.Envelope()
	commit, err := c.state.CompletePlanningStage(ctx, work.CompletePlanningStageRequest{
		AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt,
		ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: planningActor,
		Artifact: work.PlanningArtifactWrite{
			Identity: candidateArtifact.Identity, MediaType: planningCandidateMediaType,
			Kind: string(envelope.Kind), SchemaVersion: envelope.SchemaVersion, Summary: envelope.Summary,
			SourceRevision: envelope.SourceRevision, InputArtifactRefIDs: runInputArtifactRefIDs(run),
			RawLogArtifactRefIDs: artifactRefIDs(evidence),
		},
	})
	if err != nil {
		if errors.Is(err, domain.ErrPlanningRunDeferred) {
			return c.releaseSnapshot(run.ID)
		}
		return fmt.Errorf("complete planning stage: %w", err)
	}
	if commit.Disposition == work.PlanningCommitCommitted || commit.Disposition == work.PlanningCommitDuplicate || commit.Disposition == work.PlanningCommitFenced {
		return c.releaseSnapshot(run.ID)
	}
	return nil
}

func (c *Coordinator) settleTicketizeCandidate(ctx context.Context, change domain.Change, run domain.AgentRun, result work.PlanningRunCandidate) error {
	evidence := candidateEvidenceRefs(result)
	failureClass := candidateFailureClass(change, run, result)
	if failureClass == string(ErrorClassRuntimeFailed) || failureClass == string(ErrorClassRuntimeTimeout) {
		failureClass = "generator_failed"
	}
	if failureClass != "" {
		if isTicketizeFencedFailure(failureClass) {
			if fence, ok := c.state.(work.TicketizeFencePort); ok {
				if err := fence.FenceTicketizeRun(ctx, run.ID, failureClass); err != nil {
					return fmt.Errorf("fence ticketize run: %w", err)
				}
			}
			return c.releaseSnapshot(run.ID)
		}
		if err := c.failRun(ctx, change, run, failureClass, evidence); err != nil {
			if errors.Is(err, domain.ErrPlanningRunDeferred) || errors.Is(err, domain.ErrTicketizeFenced) {
				return c.releaseSnapshot(run.ID)
			}
			return err
		}
		return c.releaseSnapshot(run.ID)
	}
	if result.CandidateRef == nil {
		return c.failRun(ctx, change, run, string(ErrorClassDecodeInvalid), evidence)
	}
	candidateArtifact, err := c.state.FindArtifact(ctx, change.ID, result.CandidateRef.ID)
	if err != nil {
		return errors.Join(fmt.Errorf("find ticket draft artifact: %w", err), domain.ErrTicketizeUnavailable)
	}
	body, err := c.artifacts.Read(ctx, candidateArtifact.Identity)
	if err != nil {
		return errors.Join(fmt.Errorf("read ticket draft artifact: %w", err), domain.ErrTicketizeUnavailable)
	}
	candidate, err := DecodeTicketDraft(body)
	if err != nil {
		failureClass := classifyCoordinatorError(err)
		if errors.Is(err, domain.ErrTicketDraftInvalid) {
			failureClass = "draft_invalid"
		}
		if failErr := c.failRun(ctx, change, run, failureClass, evidence); failErr != nil {
			return errors.Join(err, failErr)
		}
		return c.releaseSnapshot(run.ID)
	}
	_, planRef, _, err := c.loadTicketizeInput(ctx, change)
	if err != nil {
		if errors.Is(err, domain.ErrTicketizeUnavailable) {
			return err
		}
		if failErr := c.failRun(ctx, change, run, classifyCoordinatorError(err), evidence); failErr != nil {
			return errors.Join(err, failErr)
		}
		return c.releaseSnapshot(run.ID)
	}
	port, ok := c.state.(work.TicketizeStatePort)
	if !ok {
		return fmt.Errorf("complete ticketize: %w", domain.ErrUnavailable)
	}
	commit, err := port.CompleteTicketize(ctx, work.TicketizeCompletionRequest{
		AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt, ExpectedChangeVersion: change.Version,
		SourceRevision: change.BaseRevision, Actor: planningActor, PlanArtifactRef: planRef, DraftArtifactRef: *result.CandidateRef,
		Candidate: candidate, RawLogRefs: result.RawLogRefs, GeneratorName: "codex", GeneratorVersion: ticketizeGeneratorV1,
	})
	if err != nil {
		if errors.Is(err, domain.ErrTicketizeFenced) || errors.Is(err, domain.ErrPlanningRunDeferred) {
			return c.releaseSnapshot(run.ID)
		}
		return fmt.Errorf("complete ticketize: %w", err)
	}
	if commit.Disposition == work.PlanningCommitCommitted || commit.Disposition == work.PlanningCommitDuplicate || commit.Disposition == work.PlanningCommitFenced {
		return c.releaseSnapshot(run.ID)
	}
	return nil
}

func isTicketizeFencedFailure(class string) bool {
	switch class {
	case "authority_fenced", "change_cancelled", "daemon_restarted", "lease_expired", "lease_revoked", "worker_lost":
		return true
	default:
		return false
	}
}

func (c *Coordinator) loadStageInput(ctx context.Context, change domain.Change, stage Stage) (StageInput, domain.ArtifactRef, domain.Artifact, error) {
	ref, err := c.inputRef(ctx, change, stage)
	if err != nil {
		return StageInput{}, domain.ArtifactRef{}, domain.Artifact{}, err
	}
	input := newStageInput(change, stage, ref.ID)
	artifact, err := c.state.FindArtifact(ctx, change.ID, ref.ID)
	if err != nil {
		return input, ref, domain.Artifact{}, err
	}
	body, err := c.artifacts.Read(ctx, artifact.Identity)
	if err != nil {
		return input, ref, artifact, err
	}
	if stage == StageUnderstand {
		input.Intent = string(body)
		return input, ref, artifact, nil
	}
	upstreamStage := StageUnderstand
	if stage == StagePlan {
		upstreamStage = StageDesign
	}
	primaryInputs := ref.InputArtifactRefIDs
	if len(primaryInputs) > 1 {
		primaryInputs = primaryInputs[:1]
	}
	expected := CandidateExpectation{
		Stage: upstreamStage, SourceRevision: change.BaseRevision,
		InputArtifactIDs: artifactRefIDStrings(primaryInputs),
	}
	upstream, err := NewStrictDecoder(nil).DecodeCandidate(body, expected)
	if err != nil {
		return StageInput{}, ref, artifact, err
	}
	input.Upstream = &upstream
	return input, ref, artifact, nil
}

func (c *Coordinator) loadTicketizeInput(ctx context.Context, change domain.Change) (TicketGenerationInput, domain.ArtifactRef, domain.Artifact, error) {
	ref, err := c.inputRef(ctx, change, StageTicketize)
	if err != nil {
		return TicketGenerationInput{}, domain.ArtifactRef{}, domain.Artifact{}, errors.Join(err, domain.ErrTicketizeUnavailable)
	}
	artifact, err := c.state.FindArtifact(ctx, change.ID, ref.ID)
	if err != nil {
		return TicketGenerationInput{}, ref, domain.Artifact{}, errors.Join(err, domain.ErrTicketizeUnavailable)
	}
	body, err := c.artifacts.Read(ctx, artifact.Identity)
	if err != nil {
		return TicketGenerationInput{}, ref, artifact, errors.Join(err, domain.ErrTicketizeUnavailable)
	}
	primaryInputs := ref.InputArtifactRefIDs
	if len(primaryInputs) > 1 {
		primaryInputs = primaryInputs[:1]
	}
	plan, err := NewStrictDecoder(nil).DecodeCandidate(body, CandidateExpectation{Stage: StagePlan, SourceRevision: change.BaseRevision, InputArtifactIDs: artifactRefIDStrings(primaryInputs)})
	if err != nil {
		return TicketGenerationInput{}, ref, artifact, err
	}
	planPayload, ok := plan.Plan()
	if !ok {
		return TicketGenerationInput{}, ref, artifact, planningError(ErrorClassSchemaInvalid, "plan")
	}
	return TicketGenerationInput{BaseRevision: change.BaseRevision, Plan: planPayload, PlanArtifactID: string(ref.ID), ProjectContext: ProjectContext{SchemaVersion: ProjectContextSchemaV1, ProjectID: string(change.ProjectID), RepositoryName: filepath.Base(change.RepositoryRoot)}}, ref, artifact, nil
}

func newStageInput(change domain.Change, stage Stage, refID domain.ArtifactRefID) StageInput {
	return StageInput{
		BaseRevision: change.BaseRevision,
		ProjectContext: ProjectContext{
			SchemaVersion: ProjectContextSchemaV1,
			ProjectID:     string(change.ProjectID), RepositoryName: filepath.Base(change.RepositoryRoot),
		},
		InputArtifactID: string(refID),
	}
}

func (c *Coordinator) prepareProjectContext(ctx context.Context, change domain.Change, value ProjectContext) (domain.Artifact, domain.ArtifactRef, error) {
	if err := NewSchemaValidator().ValidateProjectContext(value); err != nil {
		return domain.Artifact{}, domain.ArtifactRef{}, err
	}
	content, err := json.Marshal(value)
	if err != nil {
		return domain.Artifact{}, domain.ArtifactRef{}, fmt.Errorf("encode project context: %w", err)
	}
	expected := domain.NewArtifactIdentity(content)
	refs, err := c.state.ListArtifactRefs(ctx, change.ID)
	if err != nil {
		return domain.Artifact{}, domain.ArtifactRef{}, err
	}
	for _, ref := range refs {
		if ref.Kind != planningContextKind || ref.SchemaVersion != ProjectContextSchemaV1 || ref.SourceRevision != change.BaseRevision {
			continue
		}
		artifact, findErr := c.state.FindArtifact(ctx, change.ID, ref.ID)
		if findErr != nil {
			return domain.Artifact{}, domain.ArtifactRef{}, findErr
		}
		if artifact.Identity != expected {
			return domain.Artifact{}, domain.ArtifactRef{}, errors.New("project context differs from durable planning input")
		}
		stored, readErr := c.artifacts.Read(ctx, artifact.Identity)
		if readErr != nil || domain.NewArtifactIdentity(stored) != expected {
			return domain.Artifact{}, domain.ArtifactRef{}, fmt.Errorf("read project context: %w", domain.ErrArtifactUnavailable)
		}
		return artifact, ref, nil
	}
	stored, err := c.artifacts.Put(ctx, content)
	if err != nil {
		return domain.Artifact{}, domain.ArtifactRef{}, fmt.Errorf("store project context: %w", err)
	}
	if stored != expected {
		return domain.Artifact{}, domain.ArtifactRef{}, errors.New("stored project context identity differs")
	}
	return domain.Artifact{Identity: stored, MediaType: planningCandidateMediaType}, domain.ArtifactRef{}, nil
}

func (c *Coordinator) inputRef(ctx context.Context, change domain.Change, stage Stage) (domain.ArtifactRef, error) {
	if stage == StageUnderstand {
		return change.Intent, nil
	}
	kind := ArtifactKindUnderstanding
	schema := UnderstandingSchemaV1
	if stage == StagePlan {
		kind, schema = ArtifactKindDesign, DesignSchemaV1
	} else if stage == StageTicketize {
		kind, schema = ArtifactKindPlan, PlanSchemaV1
	}
	refs, err := c.state.ListArtifactRefs(ctx, change.ID)
	if err != nil {
		return domain.ArtifactRef{}, err
	}
	var selected domain.ArtifactRef
	for _, ref := range refs {
		if ref.Role == domain.ArtifactRoleOutput && ref.Kind == string(kind) && ref.SchemaVersion == schema && ref.SourceRevision == change.BaseRevision && (selected.ID == "" || ref.Ordinal > selected.Ordinal) {
			selected = ref
		}
	}
	if selected.ID == "" {
		return domain.ArtifactRef{}, fmt.Errorf("planning upstream artifact: %w", domain.ErrArtifactNotFound)
	}
	return selected, nil
}

func (c *Coordinator) failRun(ctx context.Context, change domain.Change, run domain.AgentRun, class string, evidence []domain.ArtifactRef) error {
	content, err := json.Marshal(planningFailure{
		SchemaVersion: planningFailureSchemaV1, Stage: stageFromLifecycle(run.Stage),
		Class: class, SourceRevision: change.BaseRevision, AgentRunID: string(run.ID),
	})
	if err != nil {
		return fmt.Errorf("encode planning failure: %w", err)
	}
	identity, putErr := c.artifacts.Put(ctx, content)
	failure := work.PlanningArtifactWrite{
		Identity: identity, MediaType: planningFailureMediaType, Kind: planningFailureKind,
		SchemaVersion: planningFailureSchemaV1, Summary: class, SourceRevision: change.BaseRevision,
		InputArtifactRefIDs: runInputArtifactRefIDs(run), RawLogArtifactRefIDs: artifactRefIDs(evidence),
	}
	if putErr != nil {
		failure, err = c.fallbackFailureWrite(ctx, change, run, evidence)
		if err != nil {
			return errors.Join(fmt.Errorf("store planning failure: %w", putErr), err)
		}
	}
	_, err = c.state.FailPlanningStage(ctx, work.FailPlanningStageRequest{
		AgentRunID: run.ID, Stage: run.Stage, Attempt: run.Attempt,
		ExpectedChangeVersion: change.Version, SourceRevision: change.BaseRevision, Actor: planningActor,
		Failure: failure,
	})
	if err != nil {
		return fmt.Errorf("fail planning stage: %w", err)
	}
	return nil
}

func (c *Coordinator) fallbackFailureWrite(ctx context.Context, change domain.Change, run domain.AgentRun, evidence []domain.ArtifactRef) (work.PlanningArtifactWrite, error) {
	var ref domain.ArtifactRef
	if len(evidence) != 0 {
		ref = evidence[0]
	} else {
		inputs := runInputArtifactRefIDs(run)
		if len(inputs) == 0 {
			return work.PlanningArtifactWrite{}, errors.New("planning failure evidence is unavailable")
		}
		refs, err := c.state.ListArtifactRefs(ctx, change.ID)
		if err != nil {
			return work.PlanningArtifactWrite{}, err
		}
		for _, candidate := range refs {
			if candidate.ID == inputs[0] {
				ref = candidate
				break
			}
		}
		if ref.ID == "" {
			return work.PlanningArtifactWrite{}, errors.New("planning failure input evidence is unavailable")
		}
	}
	artifact, err := c.state.FindArtifact(ctx, change.ID, ref.ID)
	if err != nil {
		return work.PlanningArtifactWrite{}, err
	}
	return work.PlanningArtifactWrite{
		Identity: artifact.Identity, MediaType: artifact.MediaType, Kind: planningFailureEvidenceKind,
		Summary: "artifact_unavailable", SourceRevision: change.BaseRevision,
		InputArtifactRefIDs: runInputArtifactRefIDs(run), RawLogArtifactRefIDs: artifactRefIDs(evidence),
	}, nil
}

func candidateFailureClass(change domain.Change, run domain.AgentRun, result work.PlanningRunCandidate) string {
	if result.AgentRunID != run.ID {
		return "agent_run_mismatch"
	}
	if result.AfterRevision != "" && result.AfterRevision != change.BaseRevision {
		return string(ErrorClassRevisionMismatch)
	}
	if len(result.GuardFindings) != 0 {
		return "guard_violation"
	}
	if result.Outcome != domain.AgentRunOutcomeSucceeded || result.ExitCode == nil || *result.ExitCode != 0 {
		switch result.FailureReason {
		case "runtime_timeout":
			return string(ErrorClassRuntimeTimeout)
		case "capture_failed":
			return "capture_failed"
		case "agent_run_mismatch", "artifact_unavailable", "authority_fenced", "change_cancelled",
			"coordination_failed", "daemon_restarted", "decode_invalid", "dispatch_failed",
			"guard_violation", "input_invalid", "lease_expired", "lease_revoked", "limit_exceeded",
			"revision_mismatch", "runtime_failed", "schema_invalid",
			"snapshot_failed", "worker_lost":
			return result.FailureReason
		}
		return string(ErrorClassRuntimeFailed)
	}
	if result.AfterRevision != change.BaseRevision {
		return string(ErrorClassRevisionMismatch)
	}
	if result.CandidateRef == nil {
		return string(ErrorClassDecodeInvalid)
	}
	if result.CandidateTruncated {
		return string(ErrorClassLimitExceeded)
	}
	return ""
}

func classifyCoordinatorError(err error) string {
	if class := ClassifyError(err); class != "" {
		return string(class)
	}
	switch {
	case errors.Is(err, domain.ErrBaseRevisionUnavailable):
		return string(ErrorClassRevisionMismatch)
	case errors.Is(err, domain.ErrArtifactNotFound), errors.Is(err, domain.ErrArtifactUnavailable):
		return "artifact_unavailable"
	default:
		return "coordination_failed"
	}
}

func targetStage(checkpoint domain.LifecycleStage) (Stage, bool) {
	switch checkpoint {
	case domain.LifecycleStageIntent:
		return StageUnderstand, true
	case domain.LifecycleStageUnderstand:
		return StageDesign, true
	case domain.LifecycleStageDesign:
		return StagePlan, true
	case domain.LifecycleStageTicketize:
		return StageTicketize, true
	default:
		return "", false
	}
}

func lifecycleStage(stage Stage) domain.LifecycleStage     { return domain.LifecycleStage(stage) }
func stageFromLifecycle(stage domain.LifecycleStage) Stage { return Stage(stage) }

func inputKind(stage Stage) string {
	if stage == StageUnderstand {
		return string(ArtifactKindIntent)
	}
	if stage == StageDesign {
		return string(ArtifactKindUnderstanding)
	}
	if stage == StageTicketize {
		return string(ArtifactKindPlan)
	}
	return string(ArtifactKindDesign)
}

func artifactRefIDsFromStrings(values []string) ([]domain.ArtifactRefID, error) {
	refs := make([]domain.ArtifactRefID, 0, len(values))
	for _, value := range values {
		ref := domain.ArtifactRefID(value)
		if ref == "" {
			return nil, errors.New("planning artifact input reference is empty")
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func artifactRefIDStrings(values []domain.ArtifactRefID) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func artifactRefIDs(values []domain.ArtifactRef) []domain.ArtifactRefID {
	result := make([]domain.ArtifactRefID, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}

func runInputArtifactRefIDs(run domain.AgentRun) []domain.ArtifactRefID {
	result := make([]domain.ArtifactRefID, 0)
	for _, artifact := range run.Artifacts {
		if artifact.Role == domain.ArtifactRoleInput {
			result = append(result, artifact.ArtifactRefID)
		}
	}
	return result
}

func candidateEvidenceRefs(candidate work.PlanningRunCandidate) []domain.ArtifactRef {
	result := make([]domain.ArtifactRef, 0, len(candidate.RawLogRefs)+1)
	if candidate.CandidateRef != nil {
		result = append(result, *candidate.CandidateRef)
	}
	return append(result, candidate.RawLogRefs...)
}

func (c *Coordinator) rememberSnapshot(runID domain.AgentRunID, changeID domain.ChangeID, snapshot Snapshot) {
	c.snapshotMu.Lock()
	c.owned[runID] = ownedSnapshot{changeID: changeID, snapshot: snapshot}
	c.snapshotMu.Unlock()
}

func (c *Coordinator) releaseSnapshot(runID domain.AgentRunID) error {
	c.snapshotMu.Lock()
	ownedSnapshot := c.owned[runID]
	c.snapshotMu.Unlock()
	if ownedSnapshot.snapshot != nil {
		if err := ownedSnapshot.snapshot.Close(); err != nil {
			return fmt.Errorf("close planning snapshot: %w", err)
		}
	}
	c.snapshotMu.Lock()
	delete(c.owned, runID)
	c.snapshotMu.Unlock()
	return nil
}

func (c *Coordinator) releaseFinishedSnapshots(ctx context.Context) error {
	c.snapshotMu.Lock()
	runIDs := make([]domain.AgentRunID, 0, len(c.owned))
	for runID := range c.owned {
		runIDs = append(runIDs, runID)
	}
	c.snapshotMu.Unlock()
	var closeErr error
	for _, runID := range runIDs {
		c.snapshotMu.Lock()
		ownedSnapshot := c.owned[runID]
		c.snapshotMu.Unlock()
		change, err := c.state.FindChange(ctx, ownedSnapshot.changeID)
		if err != nil {
			return fmt.Errorf("inspect owned planning snapshot: %w", err)
		}
		if change.LatestAgentRun == nil || change.LatestAgentRun.ID != runID || change.LatestAgentRun.Status != domain.AgentRunStatusRunning {
			closeErr = errors.Join(closeErr, c.releaseSnapshot(runID))
		}
	}
	return closeErr
}

type ownedSnapshot struct {
	changeID domain.ChangeID
	snapshot Snapshot
}

type planningFailure struct {
	SchemaVersion  string `json:"schema_version"`
	Stage          Stage  `json:"stage"`
	Class          string `json:"class"`
	SourceRevision string `json:"source_revision"`
	AgentRunID     string `json:"agent_run_id"`
}

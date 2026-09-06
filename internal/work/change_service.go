package work

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

// ChangeProjectPort 查询已经由 Project Bootstrap 注册的 RepositoryBinding。
type ChangeProjectPort interface {
	FindProjectByRoot(context.Context, string) (*domain.Project, error)
}

// ChangeSnapshotPort 在不写 Git 的前提下取得稳定源快照。
type ChangeSnapshotPort interface {
	Snapshot(context.Context, string) (domain.ChangeSourceSnapshot, error)
}

// ChangeRepositoryDiscoverPort 将绝对调用路径解析为物理 Repository root。
// 该端口是可选的；测试或其他 Adapter 可以直接按已知 root 提供 Project。
type ChangeRepositoryDiscoverPort interface {
	Discover(context.Context, string) (domain.RepositoryBinding, error)
}

// ArtifactStorePort 负责 Artifact 内容的物理原子写入和摘要校验读取。
type ArtifactStorePort interface {
	Put(context.Context, []byte) (domain.ArtifactIdentity, error)
	Read(context.Context, domain.ArtifactIdentity) ([]byte, error)
}

// ChangeCreateRecord 是写入 Change 权威状态所需的已核验输入。
type ChangeCreateRecord struct {
	Project        domain.Project
	Snapshot       domain.ChangeSourceSnapshot
	Intent         domain.ChangeIntent
	IntentIdentity domain.ArtifactIdentity
	IdempotencyKey string
	Actor          string
}

// ChangeStatePort 是 Change SQLite adapter 的窄 Application 边界。
type ChangeStatePort interface {
	CreateChange(context.Context, ChangeCreateRecord) (domain.Change, error)
	FindChange(context.Context, domain.ChangeID) (domain.Change, error)
	ListChanges(context.Context, string) ([]domain.Change, error)
	ApplyCommand(context.Context, domain.ChangeID, string, domain.ChangeVersion, string, string) (domain.Change, error)
	ApplyDecision(context.Context, domain.ChangeID, string, domain.ChangeVersion, string, string, string) (domain.Change, error)
	ListChangeEvents(context.Context, domain.ChangeID) ([]domain.ChangeEvent, error)
	ListAgentRuns(context.Context, domain.ChangeID) ([]domain.AgentRun, error)
	ListArtifactRefs(context.Context, domain.ChangeID) ([]domain.ArtifactRef, error)
	ListHumanDecisions(context.Context, domain.ChangeID) ([]domain.HumanDecision, error)
	FindArtifact(context.Context, domain.ChangeID, domain.ArtifactRefID) (domain.Artifact, error)
}

// ChangeCreateReplayPort 在读取源快照前查询已成功的创建 Receipt。
type ChangeCreateReplayPort interface {
	ReplayCreate(context.Context, string, domain.ProjectID, string, string) (domain.Change, bool, error)
}

// AgentRunStatePort 是后续 Worker/Strategy 写入 AgentRun 事实的窄端口。
type AgentRunStatePort interface {
	StartAgentRunWithArtifacts(context.Context, domain.ChangeID, string, []domain.AgentRunArtifact) (domain.AgentRun, error)
	CompleteAgentRunWithArtifacts(context.Context, domain.AgentRunID, string, string, []domain.AgentRunArtifact) (domain.AgentRun, error)
}

// AgentRunArtifactInput 是尚未登记为 ArtifactRef 的执行证据字节。
type AgentRunArtifactInput struct {
	Content   []byte
	Identity  domain.ArtifactIdentity
	MediaType string
	Role      string
	Ordinal   int
}

// AgentRunArtifactInputStatePort 在同一事务中登记 ArtifactRef 并完成 AgentRun。
type AgentRunArtifactInputStatePort interface {
	CompleteAgentRunWithArtifactInputs(context.Context, domain.AgentRunID, string, string, []AgentRunArtifactInput) (domain.AgentRun, error)
}

// ChangeCreateRequest 是 Change 创建用例输入。
type ChangeCreateRequest struct {
	RepositoryPath string
	Intent         string
	IdempotencyKey string
	Actor          string
}

// ChangeCommandRequest 是 Pause、Resume、Cancel 的用例输入。
type ChangeCommandRequest struct {
	ChangeID        domain.ChangeID
	Command         string
	ExpectedVersion domain.ChangeVersion
	IdempotencyKey  string
	Actor           string
}

// ChangeDecisionRequest 是 retry、cancel 人工决定的用例输入。
type ChangeDecisionRequest struct {
	ChangeID        domain.ChangeID
	Decision        string
	ExpectedVersion domain.ChangeVersion
	IdempotencyKey  string
	Actor           string
	Reason          string
}

// ChangeService 编排 Change 的 Project、Git、Artifact 和 SQLite 端口。
type ChangeService struct {
	projects  ChangeProjectPort
	snapshot  ChangeSnapshotPort
	artifacts ArtifactStorePort
	state     ChangeStatePort
}

// NewChangeService 创建 Change Application Service。
func NewChangeService(projects ChangeProjectPort, snapshot ChangeSnapshotPort, artifacts ArtifactStorePort, state ChangeStatePort) (*ChangeService, error) {
	if projects == nil || snapshot == nil || artifacts == nil || state == nil {
		return nil, errors.New("create change service: all ports are required")
	}
	return &ChangeService{projects: projects, snapshot: snapshot, artifacts: artifacts, state: state}, nil
}

// Create 创建一条 Intent/active Change，并保存不可变原始 Intent Artifact。
func (s *ChangeService) Create(ctx context.Context, request ChangeCreateRequest) (domain.Change, error) {
	if ctx == nil {
		return domain.Change{}, fmt.Errorf("create change: %w", domain.ErrInvalidRequest)
	}
	root, err := normalizeChangeRepositoryPath(request.RepositoryPath)
	if err != nil {
		return domain.Change{}, err
	}
	intent, err := domain.NewChangeIntent(request.Intent)
	if err != nil {
		return domain.Change{}, err
	}
	if request.IdempotencyKey == "" {
		return domain.Change{}, fmt.Errorf("create change: %w", domain.ErrInvalidRequest)
	}
	project, resolvedRoot, err := s.resolveProject(ctx, root)
	if err != nil {
		return domain.Change{}, fmt.Errorf("find project for change: %w", err)
	}
	if replayPort, ok := s.state.(ChangeCreateReplayPort); ok {
		replay, found, err := replayPort.ReplayCreate(ctx, request.IdempotencyKey, project.Identity.ProjectID, resolvedRoot, intent.Original)
		if err != nil {
			return domain.Change{}, fmt.Errorf("replay change creation: %w", err)
		}
		if found {
			return replay, nil
		}
	}
	snapshot, err := s.snapshot.Snapshot(ctx, project.Binding.Root)
	if err != nil {
		return domain.Change{}, fmt.Errorf("snapshot change source: %w", err)
	}
	identity, err := s.artifacts.Put(ctx, []byte(intent.Original))
	if err != nil {
		return domain.Change{}, fmt.Errorf("store change intent artifact: %w", err)
	}
	if expected := domain.NewArtifactIdentity([]byte(intent.Original)); identity != expected {
		return domain.Change{}, fmt.Errorf("stored change intent identity differs: %w", domain.ErrInternal)
	}
	return s.state.CreateChange(ctx, ChangeCreateRecord{
		Project: projectValue(project), Snapshot: snapshot, Intent: intent,
		IntentIdentity: identity, IdempotencyKey: request.IdempotencyKey, Actor: request.Actor,
	})
}

// Show 查询单一 Change。
func (s *ChangeService) Show(ctx context.Context, changeID domain.ChangeID) (domain.Change, error) {
	if ctx == nil {
		return domain.Change{}, fmt.Errorf("show change: %w", domain.ErrInvalidRequest)
	}
	return s.state.FindChange(ctx, changeID)
}

// List 按已注册 Repository root 查询 Change。
func (s *ChangeService) List(ctx context.Context, repositoryPath string) ([]domain.Change, error) {
	if ctx == nil {
		return nil, fmt.Errorf("list changes: %w", domain.ErrInvalidRequest)
	}
	root, err := normalizeChangeRepositoryPath(repositoryPath)
	if err != nil {
		return nil, err
	}
	_, resolvedRoot, err := s.resolveProject(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("find project for change list: %w", err)
	}
	return s.state.ListChanges(ctx, resolvedRoot)
}

// Command 执行一个带版本前置条件的普通 Change 控制命令。
func (s *ChangeService) Command(ctx context.Context, request ChangeCommandRequest) (domain.Change, error) {
	if err := validateChangeWriteRequest(ctx, request.ChangeID, request.IdempotencyKey, request.ExpectedVersion); err != nil {
		return domain.Change{}, err
	}
	switch request.Command {
	case "pause", "resume", "cancel":
	default:
		return domain.Change{}, fmt.Errorf("change command: %w", domain.ErrLifecycleTransitionInvalid)
	}
	return s.state.ApplyCommand(ctx, request.ChangeID, request.Command, request.ExpectedVersion, request.IdempotencyKey, request.Actor)
}

// Decide 只允许在 human_required 状态提交 retry 或 cancel。
func (s *ChangeService) Decide(ctx context.Context, request ChangeDecisionRequest) (domain.Change, error) {
	if err := validateChangeWriteRequest(ctx, request.ChangeID, request.IdempotencyKey, request.ExpectedVersion); err != nil {
		return domain.Change{}, err
	}
	if request.Decision != domain.HumanDecisionRetry && request.Decision != domain.HumanDecisionCancel {
		return domain.Change{}, fmt.Errorf("change decision: %w", domain.ErrLifecycleTransitionInvalid)
	}
	return s.state.ApplyDecision(ctx, request.ChangeID, request.Decision, request.ExpectedVersion, request.IdempotencyKey, request.Actor, request.Reason)
}

// StartAgentRun 保存一个阶段尝试及其输入 Artifact 关联。
func (s *ChangeService) StartAgentRun(ctx context.Context, changeID domain.ChangeID, actor string, inputRefs []domain.ArtifactRefID) (domain.AgentRun, error) {
	if ctx == nil || changeID == "" {
		return domain.AgentRun{}, fmt.Errorf("start agent run: %w", domain.ErrInvalidRequest)
	}
	artifacts := make([]domain.AgentRunArtifact, 0, len(inputRefs))
	for ordinal, refID := range inputRefs {
		artifacts = append(artifacts, domain.AgentRunArtifact{ArtifactRefID: refID, Role: domain.ArtifactRoleInput, Ordinal: ordinal})
	}
	port, ok := s.state.(AgentRunStatePort)
	if !ok {
		return domain.AgentRun{}, fmt.Errorf("start agent run: %w", domain.ErrUnavailable)
	}
	return port.StartAgentRunWithArtifacts(ctx, changeID, actor, artifacts)
}

// CompleteAgentRun 保存 AgentRun 终态及其输出或失败 Artifact 关联。
func (s *ChangeService) CompleteAgentRun(ctx context.Context, runID domain.AgentRunID, outcome, actor string, artifacts []domain.AgentRunArtifact) (domain.AgentRun, error) {
	if ctx == nil || runID == "" {
		return domain.AgentRun{}, fmt.Errorf("complete agent run: %w", domain.ErrInvalidRequest)
	}
	port, ok := s.state.(AgentRunStatePort)
	if !ok {
		return domain.AgentRun{}, fmt.Errorf("complete agent run: %w", domain.ErrUnavailable)
	}
	return port.CompleteAgentRunWithArtifacts(ctx, runID, outcome, actor, artifacts)
}

// CompleteAgentRunWithContent 原子登记输出/失败 ArtifactRef 并完成 AgentRun。
func (s *ChangeService) CompleteAgentRunWithContent(ctx context.Context, runID domain.AgentRunID, outcome, actor string, artifacts []AgentRunArtifactInput) (domain.AgentRun, error) {
	if ctx == nil || runID == "" {
		return domain.AgentRun{}, fmt.Errorf("complete agent run content: %w", domain.ErrInvalidRequest)
	}
	for index := range artifacts {
		if artifacts[index].MediaType == "" {
			artifacts[index].MediaType = "text/plain; charset=utf-8"
		}
	}
	port, ok := s.state.(AgentRunArtifactInputStatePort)
	if !ok {
		return domain.AgentRun{}, fmt.Errorf("complete agent run content: %w", domain.ErrUnavailable)
	}
	inputs := make([]AgentRunArtifactInput, 0, len(artifacts))
	for _, artifact := range artifacts {
		identity := domain.NewArtifactIdentity(artifact.Content)
		stored, err := s.artifacts.Put(ctx, artifact.Content)
		if err != nil {
			return domain.AgentRun{}, fmt.Errorf("store agent run artifact: %w", err)
		}
		if stored != identity {
			return domain.AgentRun{}, fmt.Errorf("stored agent run artifact identity differs: %w", domain.ErrInternal)
		}
		artifactInput := artifact
		artifactInput.Identity = identity
		artifactInput.Content = nil
		inputs = append(inputs, artifactInput)
	}
	return port.CompleteAgentRunWithArtifactInputs(ctx, runID, outcome, actor, inputs)
}

// Events 查询 Change Event Trace。
func (s *ChangeService) Events(ctx context.Context, changeID domain.ChangeID) ([]domain.ChangeEvent, error) {
	if ctx == nil {
		return nil, fmt.Errorf("list change events: %w", domain.ErrInvalidRequest)
	}
	return s.state.ListChangeEvents(ctx, changeID)
}

// Runs 查询 AgentRun Trace。
func (s *ChangeService) Runs(ctx context.Context, changeID domain.ChangeID) ([]domain.AgentRun, error) {
	if ctx == nil {
		return nil, fmt.Errorf("list agent runs: %w", domain.ErrInvalidRequest)
	}
	return s.state.ListAgentRuns(ctx, changeID)
}

// Artifacts 查询 Change 的 ArtifactRef Trace。
func (s *ChangeService) Artifacts(ctx context.Context, changeID domain.ChangeID) ([]domain.ArtifactRef, error) {
	if ctx == nil {
		return nil, fmt.Errorf("list artifact references: %w", domain.ErrInvalidRequest)
	}
	return s.state.ListArtifactRefs(ctx, changeID)
}

// Decisions 查询 HumanDecision Trace。
func (s *ChangeService) Decisions(ctx context.Context, changeID domain.ChangeID) ([]domain.HumanDecision, error) {
	if ctx == nil {
		return nil, fmt.Errorf("list human decisions: %w", domain.ErrInvalidRequest)
	}
	return s.state.ListHumanDecisions(ctx, changeID)
}

// ArtifactContent 读取并重新校验 Artifact 原文。
func (s *ChangeService) ArtifactContent(ctx context.Context, changeID domain.ChangeID, refID domain.ArtifactRefID) (domain.Artifact, []byte, error) {
	if ctx == nil {
		return domain.Artifact{}, nil, fmt.Errorf("read artifact content: %w", domain.ErrInvalidRequest)
	}
	artifact, err := s.state.FindArtifact(ctx, changeID, refID)
	if err != nil {
		return domain.Artifact{}, nil, err
	}
	content, err := s.artifacts.Read(ctx, artifact.Identity)
	if err != nil {
		return domain.Artifact{}, nil, fmt.Errorf("read artifact content: %w", err)
	}
	return artifact, content, nil
}

func validateChangeWriteRequest(ctx context.Context, changeID domain.ChangeID, key string, version domain.ChangeVersion) error {
	if ctx == nil || changeID == "" || key == "" || version < 1 {
		return fmt.Errorf("change write request: %w", domain.ErrInvalidRequest)
	}
	return nil
}

// ChangeErrorCode 返回 Change HTTP 边界可使用的稳定错误代码。
func ChangeErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, domain.ErrRepositoryDirty):
		return "repository_dirty"
	case errors.Is(err, domain.ErrBaseRevisionUnavailable):
		return "base_revision_unavailable"
	case errors.Is(err, domain.ErrSourceSnapshotUnstable):
		return "source_snapshot_unstable"
	case errors.Is(err, domain.ErrProjectNotFound):
		return "project_not_found"
	case errors.Is(err, domain.ErrChangeNotFound):
		return "change_not_found"
	case errors.Is(err, domain.ErrArtifactNotFound):
		return "artifact_not_found"
	case errors.Is(err, domain.ErrArtifactUnavailable), errors.Is(err, domain.ErrUnavailable):
		return "unavailable"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return "idempotency_conflict"
	case errors.Is(err, domain.ErrChangeVersionConflict):
		return "change_version_conflict"
	case errors.Is(err, domain.ErrLifecycleTransitionInvalid):
		return "lifecycle_transition_invalid"
	case errors.Is(err, domain.ErrHumanDecisionRequired):
		return "human_decision_required"
	default:
		return "internal_error"
	}
}

func normalizeChangeRepositoryPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", fmt.Errorf("change repository path: %w", domain.ErrInvalidRequest)
	}
	return path, nil
}

func (s *ChangeService) resolveProject(ctx context.Context, root string) (*domain.Project, string, error) {
	if ctx == nil {
		return nil, "", fmt.Errorf("resolve change project: %w", domain.ErrInvalidRequest)
	}
	project, err := s.projects.FindProjectByRoot(ctx, root)
	if err == nil {
		if project == nil {
			return nil, "", domain.ErrProjectNotFound
		}
		return project, project.Binding.Root, nil
	}
	if !errors.Is(err, domain.ErrProjectNotFound) {
		return nil, "", err
	}
	discoverer, ok := s.snapshot.(ChangeRepositoryDiscoverPort)
	if !ok {
		return nil, "", err
	}
	binding, discoverErr := discoverer.Discover(ctx, root)
	if discoverErr != nil {
		return nil, "", err
	}
	project, err = s.projects.FindProjectByRoot(ctx, binding.Root)
	if err != nil {
		return nil, "", err
	}
	if project == nil {
		return nil, "", domain.ErrProjectNotFound
	}
	return project, project.Binding.Root, nil
}

func projectValue(project *domain.Project) domain.Project {
	if project == nil {
		return domain.Project{}
	}
	return *project
}

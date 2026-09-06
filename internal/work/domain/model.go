package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// ProjectInitializedType 是 V1 唯一允许查询的 Project Event 类型。
	ProjectInitializedType = "ProjectInitialized"
	IntentPending          = "pending"
	IntentFailed           = "failed"

	// LifecycleStage 是 Change 已确认的耐久检查点。
	LifecycleStageIntent      LifecycleStage = "Intent"
	LifecycleStageUnderstand  LifecycleStage = "Understand"
	LifecycleStageDesign      LifecycleStage = "Design"
	LifecycleStagePlan        LifecycleStage = "Plan"
	LifecycleStageTicketize   LifecycleStage = "Ticketize"
	LifecycleStageExecute     LifecycleStage = "Execute"
	LifecycleStageVerify      LifecycleStage = "Verify"
	LifecycleStageFinalVerify LifecycleStage = "FinalVerify"

	// ChangeStatus 是 Change 的运行控制状态。
	ChangeStatusActive         ChangeStatus = "active"
	ChangeStatusPaused         ChangeStatus = "paused"
	ChangeStatusHumanRequired  ChangeStatus = "human_required"
	ChangeStatusCancelled      ChangeStatus = "cancelled"
	ChangeStatusIntegrateReady ChangeStatus = "integrate_ready"

	// AgentRunStatus 表示一次 AgentRun 的生命周期。
	AgentRunStatusRunning        = "running"
	AgentRunStatusCompleted      = "completed"
	AgentRunOutcomeSucceeded     = "succeeded"
	AgentRunOutcomeFailed        = "failed"
	AgentRunOutcomeHumanRequired = "human_required"

	// HumanDecisionKind 是人类恢复决策的固定集合。
	HumanDecisionRetry  = "retry"
	HumanDecisionCancel = "cancel"

	// ChangeEventType 是 M3 对外公开的 Change Event 类型。
	ChangeCreatedType         = "ChangeCreated"
	AgentRunStartedType       = "AgentRunStarted"
	AgentRunCompletedType     = "AgentRunCompleted"
	StageAdvancedType         = "StageAdvanced"
	ChangePausedType          = "ChangePaused"
	ChangeResumedType         = "ChangeResumed"
	ChangeHumanRequiredType   = "ChangeHumanRequired"
	HumanDecisionRecordedType = "HumanDecisionRecorded"
	ChangeCancelledType       = "ChangeCancelled"

	// ArtifactRole 是 ArtifactRef 在 Change Trace 中的稳定角色。
	ArtifactRoleChangeIntent = "change_intent"
	ArtifactRoleInput        = "input"
	ArtifactRoleOutput       = "output"
	ArtifactRoleFailure      = "failure"
)

const maxChangeIntentBytes = 64 * 1024

// LifecycleStage 表示 Change 的最近耐久检查点。
type LifecycleStage string

// ChangeStatus 表示 Change 当前是否允许新的协调。
type ChangeStatus string

// ChangeID 是 Change 的小写 canonical UUIDv7 身份。
type ChangeID string

// ArtifactID 是内容身份记录的 UUIDv7。
type ArtifactID string

// ArtifactRefID 是 Change 内 Artifact 关联的 UUIDv7。
type ArtifactRefID string

// AgentRunID 是一次阶段执行尝试的 UUIDv7。
type AgentRunID string

// HumanDecisionID 是一次人工恢复决定的 UUIDv7。
type HumanDecisionID string

// ChangeVersion 是 Change 权威状态的单调版本。
type ChangeVersion int

// ChangeSourceSnapshot 是连续只读 Git 检查得到的稳定源版本。
type ChangeSourceSnapshot struct {
	RepositoryRoot string
	BaseRevision   string
}

// ChangeIntent 保存用户原始输入及其有界展示摘要。
type ChangeIntent struct {
	Original string
	Summary  string
}

// ArtifactIdentity 用 SHA-256 与字节长度描述 Artifact 内容。
type ArtifactIdentity struct {
	SHA256     string
	ByteLength int64
}

// ArtifactRef 是业务对象对 Artifact 内容的稳定关联。
type ArtifactRef struct {
	ID         ArtifactRefID
	ChangeID   ChangeID
	ArtifactID ArtifactID
	Role       string
	Ordinal    int
}

// Validate 检查 ArtifactRef 的身份、归属和稳定角色。
func (r ArtifactRef) Validate() error {
	if err := validateUUIDv7(string(r.ID), "artifact_ref_id"); err != nil {
		return err
	}
	if err := validateUUIDv7(string(r.ChangeID), "change_id"); err != nil {
		return err
	}
	if err := validateUUIDv7(string(r.ArtifactID), "artifact_id"); err != nil {
		return err
	}
	if !validArtifactRole(r.Role) || r.Ordinal < 0 {
		return fmt.Errorf("%w: artifact reference fields are invalid", ErrInvalidRequest)
	}
	return nil
}

// Artifact 描述可被业务引用的内容摘要，不包含物理路径。
type Artifact struct {
	ID        ArtifactID
	Identity  ArtifactIdentity
	MediaType string
	CreatedAt time.Time
}

// AgentRunArtifact 是 AgentRun 与输入、输出或失败证据的有序关联。
type AgentRunArtifact struct {
	ArtifactRefID ArtifactRefID
	Role          string
	Ordinal       int
}

// AgentRun 是一个阶段的一次不可改写尝试。
type AgentRun struct {
	ID          AgentRunID
	ChangeID    ChangeID
	Stage       LifecycleStage
	Attempt     int
	Status      string
	Outcome     string
	StartedAt   time.Time
	CompletedAt *time.Time
	Artifacts   []AgentRunArtifact
}

// HumanDecision 是 human_required 状态上的人工恢复事实。
type HumanDecision struct {
	ID        HumanDecisionID
	ChangeID  ChangeID
	Kind      string
	Actor     string
	Reason    string
	CreatedAt time.Time
}

// ChangeEvent 是统一 Event 账本中的追加式 Change 事实。
type ChangeEvent struct {
	EventID        string
	ProjectID      ProjectID
	ChangeID       ChangeID
	Sequence       int
	Type           string
	OccurredAt     time.Time
	Actor          string
	ArtifactRefIDs []ArtifactRefID
	AgentRunID     *AgentRunID
	DecisionID     *HumanDecisionID
}

// Change 是绑定 Project、BaseRevision 和生命周期状态的权威事实。
type Change struct {
	ID             ChangeID
	ProjectID      ProjectID
	RepositoryRoot string
	Stage          LifecycleStage
	Status         ChangeStatus
	Version        ChangeVersion
	BaseRevision   string
	Intent         ArtifactRef
	LatestAgentRun *AgentRun
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewChange 创建 Intent/active/version=1 的 Change。
func NewChange(projectID ProjectID, changeID ChangeID, repositoryRoot, baseRevision string, intent ArtifactRef, now time.Time) (Change, error) {
	change := Change{ID: changeID, ProjectID: projectID, RepositoryRoot: repositoryRoot, Stage: LifecycleStageIntent, Status: ChangeStatusActive, Version: 1, BaseRevision: baseRevision, Intent: intent, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	if err := change.Validate(); err != nil {
		return Change{}, err
	}
	return change, nil
}

// Validate 检查 Change 的创建态和生命周期字段。
func (c Change) Validate() error {
	if err := validateUUIDv7(string(c.ID), "change_id"); err != nil {
		return err
	}
	if err := c.ProjectID.Validate(); err != nil {
		return fmt.Errorf("change project: %w", err)
	}
	if c.RepositoryRoot == "" || !filepath.IsAbs(c.RepositoryRoot) || filepath.Clean(c.RepositoryRoot) != c.RepositoryRoot {
		return fmt.Errorf("%w: change repository root must be normalized absolute path", ErrInvalidRequest)
	}
	if !validLifecycleStage(c.Stage) || !validChangeStatus(c.Status) || c.Version < 1 || !validObjectID(c.BaseRevision) {
		return fmt.Errorf("%w: change fields are invalid", ErrInvalidRequest)
	}
	if err := c.Intent.Validate(); err != nil {
		return fmt.Errorf("change intent artifact reference: %w", err)
	}
	if c.Intent.ChangeID != c.ID || c.Intent.Role != ArtifactRoleChangeIntent || c.Intent.Ordinal != 0 {
		return fmt.Errorf("%w: change intent artifact reference does not belong to change", ErrInvalidRequest)
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: change timestamps are required", ErrInvalidRequest)
	}
	return nil
}

// Transition 执行普通 Pause、Resume、Cancel 命令并返回新状态。
func (c Change) Transition(command string) (Change, error) {
	var next ChangeStatus
	switch command {
	case "pause":
		if c.Status != ChangeStatusActive {
			return Change{}, fmt.Errorf("%w: pause from %s", ErrLifecycleTransitionInvalid, c.Status)
		}
		next = ChangeStatusPaused
	case "resume":
		if c.Status != ChangeStatusPaused {
			return Change{}, fmt.Errorf("%w: resume from %s", ErrLifecycleTransitionInvalid, c.Status)
		}
		next = ChangeStatusActive
	case "cancel":
		if c.Status != ChangeStatusActive && c.Status != ChangeStatusPaused && c.Status != ChangeStatusHumanRequired {
			return Change{}, fmt.Errorf("%w: cancel from %s", ErrLifecycleTransitionInvalid, c.Status)
		}
		next = ChangeStatusCancelled
	default:
		return Change{}, fmt.Errorf("%w: unknown command %q", ErrLifecycleTransitionInvalid, command)
	}
	c.Status = next
	c.Version++
	return c, nil
}

// ApplyHumanDecision validates and applies a retry/cancel decision.
func (c Change) ApplyHumanDecision(kind string) (Change, error) {
	if c.Status != ChangeStatusHumanRequired {
		return Change{}, fmt.Errorf("%w: status is %s", ErrHumanDecisionRequired, c.Status)
	}
	switch kind {
	case HumanDecisionRetry:
		c.Status = ChangeStatusActive
	case HumanDecisionCancel:
		c.Status = ChangeStatusCancelled
	default:
		return Change{}, fmt.Errorf("%w: unknown decision %q", ErrLifecycleTransitionInvalid, kind)
	}
	c.Version++
	return c, nil
}

// EnterHumanRequired 将可重试失败固定为人类决定状态。
func (c Change) EnterHumanRequired() (Change, error) {
	if c.Status != ChangeStatusActive {
		return Change{}, fmt.Errorf("%w: status is %s", ErrLifecycleTransitionInvalid, c.Status)
	}
	c.Status = ChangeStatusHumanRequired
	c.Version++
	return c, nil
}

// CanAdvanceLateResult 返回当前状态是否允许晚到结果推进生命周期。
func (c Change) CanAdvanceLateResult() bool { return c.Status == ChangeStatusActive }

// NextAttempt 返回当前 Stage 的下一次尝试编号。
func NextAttempt(current int) (int, error) {
	if current < 0 {
		return 0, fmt.Errorf("%w: attempt must not be negative", ErrInvalidRequest)
	}
	return current + 1, nil
}

// Validate 检查 ChangeIntent 的输入约束。
func (i ChangeIntent) Validate() error {
	if !utf8.ValidString(i.Original) || len([]byte(i.Original)) > maxChangeIntentBytes || strings.TrimSpace(i.Original) == "" {
		return fmt.Errorf("%w: change intent is invalid", ErrInvalidRequest)
	}
	if i.Summary != changeIntentSummary(i.Original) {
		return fmt.Errorf("%w: change intent summary is not canonical", ErrInvalidRequest)
	}
	return nil
}

// NewChangeIntent 校验并计算有界摘要，原文保持不变。
func NewChangeIntent(original string) (ChangeIntent, error) {
	intent := ChangeIntent{Original: original}
	if !utf8.ValidString(original) || len([]byte(original)) > maxChangeIntentBytes || strings.TrimSpace(original) == "" {
		return ChangeIntent{}, fmt.Errorf("%w: change intent is invalid", ErrInvalidRequest)
	}
	intent.Summary = changeIntentSummary(original)
	return intent, nil
}

func changeIntentSummary(original string) string {
	fields := strings.Fields(original)
	runes := []rune(strings.Join(fields, " "))
	if len(runes) > 256 {
		runes = runes[:256]
	}
	return string(runes)
}

// NewArtifactIdentity 计算内容摘要和长度。
func NewArtifactIdentity(content []byte) ArtifactIdentity {
	digest := sha256.Sum256(content)
	return ArtifactIdentity{SHA256: hex.EncodeToString(digest[:]), ByteLength: int64(len(content))}
}

// Validate 检查 ArtifactIdentity 的 SHA-256 表达。
func (i ArtifactIdentity) Validate() error {
	if len(i.SHA256) != sha256.Size*2 || strings.ToLower(i.SHA256) != i.SHA256 || i.ByteLength < 0 {
		return fmt.Errorf("%w: artifact identity is invalid", ErrInvalidRequest)
	}
	if _, err := hex.DecodeString(i.SHA256); err != nil {
		return fmt.Errorf("%w: artifact identity is invalid", ErrInvalidRequest)
	}
	return nil
}

// Complete 将 running AgentRun 一次性置为 completed。
func (r *AgentRun) Complete(outcome string, completedAt time.Time) error {
	if r == nil || r.Status != AgentRunStatusRunning || r.CompletedAt != nil || !validAgentRunOutcome(outcome) || completedAt.IsZero() {
		return fmt.Errorf("%w: agent run completion is invalid", ErrInvalidRequest)
	}
	completedAt = completedAt.UTC()
	r.Status, r.Outcome, r.CompletedAt = AgentRunStatusCompleted, outcome, &completedAt
	return nil
}

// Validate 检查 AgentRun 状态和终态字段。
func (r AgentRun) Validate() error {
	if err := validateUUIDv7(string(r.ID), "agent_run_id"); err != nil || validateUUIDv7(string(r.ChangeID), "change_id") != nil || r.Attempt < 1 || !validLifecycleStage(r.Stage) || !validAgentRunStatus(r.Status) || r.StartedAt.IsZero() {
		return fmt.Errorf("%w: agent run is invalid", ErrInvalidRequest)
	}
	if r.Status == AgentRunStatusCompleted && (r.CompletedAt == nil || !validAgentRunOutcome(r.Outcome)) {
		return fmt.Errorf("%w: completed agent run needs outcome and completion time", ErrInvalidRequest)
	}
	if r.Status == AgentRunStatusRunning && (r.CompletedAt != nil || r.Outcome != "") {
		return fmt.Errorf("%w: running agent run cannot have completion fields", ErrInvalidRequest)
	}
	for _, artifact := range r.Artifacts {
		if err := artifact.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate 检查 AgentRun 与 ArtifactRef 的稳定关联字段。
func (a AgentRunArtifact) Validate() error {
	if err := validateUUIDv7(string(a.ArtifactRefID), "artifact_ref_id"); err != nil {
		return err
	}
	if a.Role != ArtifactRoleInput && a.Role != ArtifactRoleOutput && a.Role != ArtifactRoleFailure {
		return fmt.Errorf("%w: agent run artifact role is invalid", ErrInvalidRequest)
	}
	if a.Ordinal < 0 {
		return fmt.Errorf("%w: agent run artifact ordinal is invalid", ErrInvalidRequest)
	}
	return nil
}

func validArtifactRole(role string) bool {
	return role == ArtifactRoleChangeIntent || role == ArtifactRoleInput || role == ArtifactRoleOutput || role == ArtifactRoleFailure
}

// Validate 检查人工决策类型和 Change 归属。
func (d HumanDecision) Validate() error {
	if err := validateUUIDv7(string(d.ID), "decision_id"); err != nil {
		return fmt.Errorf("%w: human decision identity is invalid", ErrInvalidRequest)
	}
	if err := validateUUIDv7(string(d.ChangeID), "change_id"); err != nil {
		return fmt.Errorf("%w: human decision identity is invalid", ErrInvalidRequest)
	}
	if (d.Kind != HumanDecisionRetry && d.Kind != HumanDecisionCancel) || d.CreatedAt.IsZero() {
		return fmt.Errorf("%w: human decision is invalid", ErrInvalidRequest)
	}
	return nil
}

func validLifecycleStage(stage LifecycleStage) bool {
	switch stage {
	case LifecycleStageIntent, LifecycleStageUnderstand, LifecycleStageDesign, LifecycleStagePlan, LifecycleStageTicketize, LifecycleStageExecute, LifecycleStageVerify, LifecycleStageFinalVerify:
		return true
	default:
		return false
	}
}

func validChangeStatus(status ChangeStatus) bool {
	switch status {
	case ChangeStatusActive, ChangeStatusPaused, ChangeStatusHumanRequired, ChangeStatusCancelled, ChangeStatusIntegrateReady:
		return true
	default:
		return false
	}
}

func validAgentRunStatus(status string) bool {
	return status == AgentRunStatusRunning || status == AgentRunStatusCompleted
}

func validAgentRunOutcome(outcome string) bool {
	return outcome == AgentRunOutcomeSucceeded || outcome == AgentRunOutcomeFailed || outcome == AgentRunOutcomeHumanRequired
}

func validateUUIDv7(value, field string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 || parsed.String() != value {
		return fmt.Errorf("%w: %s must be lowercase canonical UUIDv7", ErrInvalidRequest, field)
	}
	return nil
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	if strings.ToLower(value) != value {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}

// ProjectID 是由 ProjectManifest 持有的稳定身份。
type ProjectID string

// NewProjectID 生成新的小写 canonical UUIDv7。
func NewProjectID() ProjectID {
	value, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("generate project UUIDv7: %v", err))
	}
	return ProjectID(value.String())
}

// Validate 检查 ProjectID 的 V1 表达。
func (id ProjectID) Validate() error {
	parsed, err := uuid.Parse(string(id))
	if err != nil || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 || parsed.String() != string(id) {
		return fmt.Errorf("%w: project_id must be lowercase canonical UUIDv7", ErrManifestInvalid)
	}
	return nil
}

// RepositoryIdentity 表示一个 Project 的稳定 Repository 身份。
type RepositoryIdentity struct{ ProjectID ProjectID }

// RepositoryBinding 表示当前物理、非 bare Git 主工作树根。
type RepositoryBinding struct {
	Root         string
	ManifestPath string
}

// Validate 检查 Binding 的路径表达。
func (b RepositoryBinding) Validate() error {
	if b.Root == "" || !filepath.IsAbs(b.Root) || filepath.Clean(b.Root) != b.Root {
		return fmt.Errorf("%w: repository binding root must be normalized absolute path", ErrInvalidRequest)
	}
	if b.ManifestPath == "" || !filepath.IsAbs(b.ManifestPath) {
		return fmt.Errorf("%w: manifest path must be absolute", ErrInvalidRequest)
	}
	return nil
}

// ProjectManifest 是 Repository 中严格版本化的 Project 身份表达。
type ProjectManifest struct {
	Version   int
	ProjectID ProjectID
}

// Validate 检查 Manifest 的业务字段。
func (m ProjectManifest) Validate() error {
	if m.Version != 1 {
		return fmt.Errorf("%w: unsupported manifest version", ErrManifestInvalid)
	}
	return m.ProjectID.Validate()
}

// Project 是当前 LocalStateRoot 内的权威 Project 快照。
type Project struct {
	Identity  RepositoryIdentity
	Binding   RepositoryBinding
	CreatedAt time.Time
}

// ProjectInitialized 是 Project 首次成为权威记录时的不可变事实。
type ProjectInitialized struct {
	EventID    string
	ProjectID  ProjectID
	OccurredAt time.Time
}

// ProjectInitializationIntent 是 Manifest 与 SQLite 之间可恢复的中间状态。
type ProjectInitializationIntent struct {
	ID             string
	ProjectID      ProjectID
	RepositoryRoot string
	IdempotencyKey string
	Status         string
	FailureCode    string
}

// ProjectInitializationReceipt 是一次幂等初始化请求的终态记录。
type ProjectInitializationReceipt struct {
	IdempotencyKey string
	RepositoryRoot string
	ProjectID      ProjectID
	Status         string
	FailureCode    string
}

// NormalizeRoot 返回绝对路径的清洁表达。
func NormalizeRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: repository path must be absolute", ErrInvalidRequest)
	}
	return filepath.Clean(path), nil
}

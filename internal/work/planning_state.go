package work

import (
	"context"
	"time"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	// PlanningCommitCommitted 表示当前 Planning attempt 已推进权威 Change checkpoint。
	PlanningCommitCommitted = "committed"
	// PlanningCommitFenced 表示结果被保存为事实，但没有推进权威 Change。
	PlanningCommitFenced = "fenced"
	// PlanningCommitDuplicate 表示相同 AgentRun 已有不可改写的 Planning 终态。
	PlanningCommitDuplicate = "duplicate"
)

// PlanningArtifactWrite 描述 Artifact 内容先落盘后交给 Workstore 登记的权威摘要。
// RawLogArtifactRefIDs 只引用已经由 candidate Report 保存的同一 Change ArtifactRef。
type PlanningArtifactWrite struct {
	Identity             domain.ArtifactIdentity
	MediaType            string
	Kind                 string
	SchemaVersion        string
	Summary              string
	SourceRevision       string
	InputArtifactRefIDs  []domain.ArtifactRefID
	RawLogArtifactRefIDs []domain.ArtifactRefID
}

// StartPlanningRunRequest 使用显式目标 stage 创建一次 Planning attempt。
// Change.Stage 是成功前的耐久 checkpoint，不会在 start 时提前推进。
type StartPlanningRunRequest struct {
	ChangeID              domain.ChangeID
	TargetStage           domain.LifecycleStage
	ExpectedChangeVersion domain.ChangeVersion
	SourceRevision        string
	Actor                 string
	InputArtifactRefIDs   []domain.ArtifactRefID
	InputArtifacts        []PlanningArtifactWrite
}

// CompletePlanningStageRequest 提交已经过 Coordinator 验证的结构化 Artifact。
type CompletePlanningStageRequest struct {
	AgentRunID            domain.AgentRunID
	Stage                 domain.LifecycleStage
	Attempt               int
	ExpectedChangeVersion domain.ChangeVersion
	SourceRevision        string
	Actor                 string
	Artifact              PlanningArtifactWrite
	RawLogs               []PlanningArtifactWrite
}

// FailPlanningStageRequest 将一次 Planning attempt 固定为失败并保存失败证据。
type FailPlanningStageRequest struct {
	AgentRunID            domain.AgentRunID
	Stage                 domain.LifecycleStage
	Attempt               int
	ExpectedChangeVersion domain.ChangeVersion
	SourceRevision        string
	Actor                 string
	Failure               PlanningArtifactWrite
	RawLogs               []PlanningArtifactWrite
}

// PlanningStageCommit 是 Planning completion 的幂等、可围栏结果。
type PlanningStageCommit struct {
	Run         domain.AgentRun
	Change      domain.Change
	ArtifactRef domain.ArtifactRef
	RawLogRefs  []domain.ArtifactRef
	Disposition string
}

// PlanningRunCandidate 是 Worker Report 保存的非权威候选及运行观察。
type PlanningRunCandidate struct {
	AgentRunID         domain.AgentRunID
	Outcome            string
	ExitCode           *int
	AfterRevision      string
	FailureReason      string
	GuardFindings      []string
	CandidateRef       *domain.ArtifactRef
	CandidateTruncated bool
	RawLogRefs         []domain.ArtifactRef
	ReceivedAt         time.Time
}

// PlanningStatePort 是 Coordinator 使用的窄 Work authority 边界。
type PlanningStatePort interface {
	ReconcilePlanningExecutions(context.Context) error
	StartPlanningRun(context.Context, StartPlanningRunRequest) (domain.AgentRun, error)
	RecordPlanningRunFailureCandidate(context.Context, domain.AgentRunID, string) error
	FindPlanningRunCandidate(context.Context, domain.AgentRunID) (PlanningRunCandidate, error)
	CompletePlanningStage(context.Context, CompletePlanningStageRequest) (PlanningStageCommit, error)
	FailPlanningStage(context.Context, FailPlanningStageRequest) (PlanningStageCommit, error)
	ListRecoverablePlanningChanges(context.Context) ([]domain.Change, error)
}

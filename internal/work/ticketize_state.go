package work

import (
	"context"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

// TicketizeCompletionRequest 是已经由 Planning 严格校验的 Ticket Graph 提交请求。
// Draft 和 Plan ArtifactRef 必须是同一 Change、固定 base revision 的耐久引用。
type TicketizeCompletionRequest struct {
	AgentRunID            domain.AgentRunID
	Stage                 domain.LifecycleStage
	Attempt               int
	ExpectedChangeVersion domain.ChangeVersion
	SourceRevision        string
	Actor                 string
	PlanArtifactRef       domain.ArtifactRef
	DraftArtifactRef      domain.ArtifactRef
	RawLogRefs            []domain.ArtifactRef
	Candidate             domain.TicketGraphCandidate
	GeneratorName         string
	GeneratorVersion      string
}

// TicketizeCommit 是 Ticketize 原子提交的结果和幂等 disposition。
type TicketizeCommit struct {
	Graph       domain.CanonicalTicketGraph
	Run         domain.AgentRun
	Change      domain.Change
	Disposition string
}

// TicketizeStatePort 是 Ticketize 对 Work authority 的专用窄端口。
type TicketizeStatePort interface {
	StartTicketizeRun(context.Context, StartPlanningRunRequest) (domain.AgentRun, error)
	CompleteTicketize(context.Context, TicketizeCompletionRequest) (TicketizeCommit, error)
	FindTicketGraph(context.Context, domain.ChangeID) (domain.CanonicalTicketGraph, error)
}

// TicketizeFencePort 允许 Change 控制命令围栏尚未提交 Graph 的 Ticketize attempt。
type TicketizeFencePort interface {
	FenceTicketizeRun(context.Context, domain.AgentRunID, string) error
}

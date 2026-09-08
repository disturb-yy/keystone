// Package planning 定义 Understand、Design、Plan 的纯候选 Contract 与策略边界。
package planning

import "encoding/json"

const (
	// ProjectContextSchemaV1 是 Ticket 07 唯一支持的项目元数据快照版本。
	ProjectContextSchemaV1 = "ProjectContext.v1"
	UnderstandingSchemaV1  = "Understanding.v1"
	DesignSchemaV1         = "Design.v1"
	PlanSchemaV1           = "Plan.v1"

	MaxProjectContextBytes = 64 << 10
	MaxArtifactBytes       = 1 << 20
	MaxTextFieldBytes      = 8 << 10
	MaxSummaryRunes        = 256
	MaxArrayItems          = 64
	MaxPlanSteps           = 32
	MaxRawLogBytes         = 64 << 10
	MaxIntentBytes         = 64 << 10
)

// Stage 是 Planning 的严格串行阶段名。
type Stage string

const (
	StageUnderstand Stage = "Understand"
	StageDesign     Stage = "Design"
	StagePlan       Stage = "Plan"
)

// ArtifactKind 是 Planning 输入和候选输出的语义类型。
type ArtifactKind string

const (
	ArtifactKindIntent        ArtifactKind = "intent"
	ArtifactKindUnderstanding ArtifactKind = "understanding"
	ArtifactKindDesign        ArtifactKind = "design"
	ArtifactKindPlan          ArtifactKind = "plan"
)

// RuntimeCapability 是 Strategy 允许请求的窄 Runtime 能力。
type RuntimeCapability string

const RuntimeCapabilityPlanningReadOnly RuntimeCapability = "planning.read_only.v1"

// ProjectContext 是不含宿主绝对路径、凭据或源码的有界项目元数据快照。
type ProjectContext struct {
	SchemaVersion  string   `json:"schema_version"`
	ProjectID      string   `json:"project_id"`
	RepositoryName string   `json:"repository_name,omitempty"`
	Languages      []string `json:"languages,omitempty"`
	RelevantPaths  []string `json:"relevant_paths,omitempty"`
	Conventions    []string `json:"conventions,omitempty"`
}

// ArtifactEnvelope 是 Daemon 可在验证后纳入权威 Artifact 的候选结构。
// Artifact 身份、Change/AgentRun 归属和创建时间不接受 Runtime 提供。
type ArtifactEnvelope struct {
	Kind             ArtifactKind    `json:"kind"`
	SchemaVersion    string          `json:"schema_version"`
	Stage            Stage           `json:"stage"`
	Summary          string          `json:"summary"`
	SourceRevision   string          `json:"source_revision"`
	InputArtifactIDs []string        `json:"input_artifact_ids"`
	Payload          json.RawMessage `json:"payload"`
}

// UnderstandingPayload 固定 Understand 候选的最小 schema。
type UnderstandingPayload struct {
	Problem     string   `json:"problem"`
	Goals       []string `json:"goals"`
	Constraints []string `json:"constraints"`
}

// DesignPayload 固定 Design 候选的最小 schema。
type DesignPayload struct {
	Approach  string   `json:"approach"`
	Decisions []string `json:"decisions"`
	Risks     []string `json:"risks"`
}

// PlanPayload 固定 Plan 候选的最小 schema。
type PlanPayload struct {
	Steps        []PlanStep `json:"steps"`
	Verification []string   `json:"verification"`
}

// PlanStep 是有序 Plan 中的一个有界步骤，不表达 Canonical Ticket Graph。
type PlanStep struct {
	ID      string   `json:"id"`
	Summary string   `json:"summary"`
	Paths   []string `json:"paths"`
}

// CandidateExpectation 固定本次 decoder 允许接受的阶段、revision 和输入关联。
type CandidateExpectation struct {
	Stage            Stage
	SourceRevision   string
	InputArtifactIDs []string
}

// Candidate 是通过 strict decoder 和 validator 后的非权威候选。
// 内部验证标记防止调用方用普通 struct literal 伪造已验证上游输入。
type Candidate struct {
	envelope      ArtifactEnvelope
	understanding *UnderstandingPayload
	design        *DesignPayload
	plan          *PlanPayload
	rawJSON       []byte
	validated     bool
}

// Envelope 返回候选 envelope 的防御性副本。
func (c Candidate) Envelope() ArtifactEnvelope { return cloneEnvelope(c.envelope) }

// RawJSON 返回 Runtime 原始结构化候选的防御性副本。
func (c Candidate) RawJSON() []byte { return append([]byte(nil), c.rawJSON...) }

// Validated 表示候选是否来自已成功的 strict decoder/validator。
func (c Candidate) Validated() bool { return c.validated }

// Understanding 返回 Understand payload 的防御性副本。
func (c Candidate) Understanding() (UnderstandingPayload, bool) {
	if c.understanding == nil {
		return UnderstandingPayload{}, false
	}
	return cloneUnderstanding(*c.understanding), true
}

// Design 返回 Design payload 的防御性副本。
func (c Candidate) Design() (DesignPayload, bool) {
	if c.design == nil {
		return DesignPayload{}, false
	}
	return cloneDesign(*c.design), true
}

// Plan 返回 Plan payload 的防御性副本。
func (c Candidate) Plan() (PlanPayload, bool) {
	if c.plan == nil {
		return PlanPayload{}, false
	}
	return clonePlan(*c.plan), true
}

// Validator 是 ProjectContext 与候选 schema 的纯校验端口。
type Validator interface {
	ValidateProjectContext(ProjectContext) error
	ValidateCandidate(Candidate, CandidateExpectation) error
}

// CandidateDecoder 将 Runtime bytes 转换为已验证但尚未提交的候选。
type CandidateDecoder interface {
	DecodeCandidate([]byte, CandidateExpectation) (Candidate, error)
}

func cloneEnvelope(value ArtifactEnvelope) ArtifactEnvelope {
	value.InputArtifactIDs = append([]string(nil), value.InputArtifactIDs...)
	value.Payload = append(json.RawMessage(nil), value.Payload...)
	return value
}

func cloneUnderstanding(value UnderstandingPayload) UnderstandingPayload {
	value.Goals = append([]string(nil), value.Goals...)
	value.Constraints = append([]string(nil), value.Constraints...)
	return value
}

func cloneDesign(value DesignPayload) DesignPayload {
	value.Decisions = append([]string(nil), value.Decisions...)
	value.Risks = append([]string(nil), value.Risks...)
	return value
}

func clonePlan(value PlanPayload) PlanPayload {
	value.Verification = append([]string(nil), value.Verification...)
	value.Steps = append([]PlanStep(nil), value.Steps...)
	for index := range value.Steps {
		value.Steps[index].Paths = append([]string(nil), value.Steps[index].Paths...)
	}
	return value
}

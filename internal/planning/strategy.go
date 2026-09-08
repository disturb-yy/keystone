package planning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// StageDefinition 固定阶段的 schema、单一上游类型和允许的 Runtime capability。
type StageDefinition struct {
	Stage             Stage
	SchemaVersion     string
	RequiredInputKind ArtifactKind
	Capability        RuntimeCapability
}

// DefinitionForStage 返回 Ticket 07 支持阶段的冻结定义。
func DefinitionForStage(stage Stage) (StageDefinition, error) {
	kind, schema, ok := contractForStage(stage)
	if !ok {
		return StageDefinition{}, planningError(ErrorClassInputInvalid, "stage")
	}
	definition := StageDefinition{Stage: stage, SchemaVersion: schema, Capability: RuntimeCapabilityPlanningReadOnly}
	switch kind {
	case ArtifactKindUnderstanding:
		definition.RequiredInputKind = ArtifactKindIntent
	case ArtifactKindDesign:
		definition.RequiredInputKind = ArtifactKindUnderstanding
	case ArtifactKindPlan:
		definition.RequiredInputKind = ArtifactKindDesign
	}
	return definition, nil
}

// NextStage 返回严格串行链中的下一个 Planning 阶段；Plan 后由 Coordinator 进入 Ticketize。
func NextStage(stage Stage) (Stage, bool) {
	switch stage {
	case StageUnderstand:
		return StageDesign, true
	case StageDesign:
		return StagePlan, true
	default:
		return "", false
	}
}

// StageInput 只携带固定 revision、ProjectContext 和一个被授权的上游 Artifact。
type StageInput struct {
	BaseRevision    string
	ProjectContext  ProjectContext
	InputArtifactID string
	Intent          string
	Upstream        *Candidate
}

// RuntimeRequest 是 Strategy 准备的非权威候选生成请求。
type RuntimeRequest struct {
	Stage         Stage
	SchemaVersion string
	Capability    RuntimeCapability
	BaseRevision  string
	Instruction   string
}

// RuntimeResult 是 Runtime 的候选 bytes 和原始日志观察，不表达阶段成功。
type RuntimeResult struct {
	CandidateOutput []byte
	RawLog          []byte
	ExitCode        int
	TimedOut        bool
}

// Runtime 是便于核心测试和本机 adapter 接入的窄候选生成端口。
type Runtime interface {
	Run(context.Context, RuntimeRequest) (RuntimeResult, error)
}

// Clock 为执行观察时间提供可替换 seam。
type Clock interface {
	Now() time.Time
}

// CapturedBytes 保存有界前缀及其身份，不把截断内容伪装成完整输出。
type CapturedBytes struct {
	Content           []byte
	SHA256            string
	SizeBytes         int64
	OriginalSizeBytes int64
	Truncated         bool
}

// RuntimeObservation 保存 Coordinator 可转化为 Failure/raw-log Artifact 的有界事实。
type RuntimeObservation struct {
	StartedAt       time.Time
	CompletedAt     time.Time
	ExitCode        int
	TimedOut        bool
	CandidateOutput CapturedBytes
	RawLog          CapturedBytes
}

// StrategyResult 保存已验证候选和 Runtime 观察；候选仍需 Coordinator 权威提交。
type StrategyResult struct {
	Candidate   Candidate
	Observation RuntimeObservation
}

// StageStrategy 支持生产异步 Prepare/Decode，也提供 fake Runtime 可测的同步 Execute。
type StageStrategy interface {
	Definition() StageDefinition
	Prepare(StageInput) (RuntimeRequest, error)
	Decode([]byte, StageInput) (Candidate, error)
	Execute(context.Context, StageInput) (StrategyResult, error)
}

// Strategy 是不持有 Change、AgentRun、Artifact store 或生命周期 authority 的纯策略。
type Strategy struct {
	definition StageDefinition
	runtime    Runtime
	validator  Validator
	decoder    CandidateDecoder
	clock      Clock
}

// NewStrategy 创建指定阶段策略；runtime 可以为空以供异步 Coordinator 只使用 Prepare/Decode。
func NewStrategy(stage Stage, runtime Runtime, validator Validator, decoder CandidateDecoder, clock Clock) (*Strategy, error) {
	definition, err := DefinitionForStage(stage)
	if err != nil {
		return nil, err
	}
	if validator == nil {
		validator = NewSchemaValidator()
	}
	if decoder == nil {
		decoder = NewStrictDecoder(validator)
	}
	if clock == nil {
		clock = systemClock{}
	}
	return &Strategy{definition: definition, runtime: runtime, validator: validator, decoder: decoder, clock: clock}, nil
}

// NewUnderstandStrategy 创建使用默认 validator、decoder 和 clock 的 Understand 策略。
func NewUnderstandStrategy(runtime Runtime) *Strategy {
	strategy, _ := NewStrategy(StageUnderstand, runtime, nil, nil, nil)
	return strategy
}

// NewDesignStrategy 创建使用默认 validator、decoder 和 clock 的 Design 策略。
func NewDesignStrategy(runtime Runtime) *Strategy {
	strategy, _ := NewStrategy(StageDesign, runtime, nil, nil, nil)
	return strategy
}

// NewPlanStrategy 创建使用默认 validator、decoder 和 clock 的 Plan 策略。
func NewPlanStrategy(runtime Runtime) *Strategy {
	strategy, _ := NewStrategy(StagePlan, runtime, nil, nil, nil)
	return strategy
}

// Definition 返回该 Strategy 的不可变阶段声明。
func (s *Strategy) Definition() StageDefinition { return s.definition }

// Prepare 校验固定输入并构造可异步下发的 Runtime 请求。
func (s *Strategy) Prepare(input StageInput) (RuntimeRequest, error) {
	if err := s.validateInput(input); err != nil {
		return RuntimeRequest{}, err
	}
	instruction, err := buildPrompt(s.definition, input)
	if err != nil {
		return RuntimeRequest{}, err
	}
	return RuntimeRequest{
		Stage: s.definition.Stage, SchemaVersion: s.definition.SchemaVersion,
		Capability: s.definition.Capability, BaseRevision: input.BaseRevision,
		Instruction: instruction,
	}, nil
}

// Decode 对异步 Runtime 返回的候选单独执行严格解码与校验。
func (s *Strategy) Decode(output []byte, input StageInput) (Candidate, error) {
	if err := s.validateInput(input); err != nil {
		return Candidate{}, err
	}
	return s.decoder.DecodeCandidate(output, s.expectation(input))
}

// Execute 通过 injected Runtime 运行一次同步测试 seam，并且只返回非权威候选。
func (s *Strategy) Execute(ctx context.Context, input StageInput) (StrategyResult, error) {
	if ctx == nil {
		return StrategyResult{}, planningError(ErrorClassInputInvalid, "context")
	}
	request, err := s.Prepare(input)
	if err != nil {
		return StrategyResult{}, err
	}
	if s.runtime == nil {
		return StrategyResult{}, planningError(ErrorClassRuntimeFailed, "runtime")
	}
	startedAt := s.clock.Now().UTC()
	runtimeResult, runtimeErr := s.runtime.Run(ctx, request)
	completedAt := s.clock.Now().UTC()
	result := StrategyResult{Observation: newRuntimeObservation(startedAt, completedAt, runtimeResult)}
	if err := classifyRuntimeResult(runtimeResult, runtimeErr); err != nil {
		return result, err
	}
	candidate, err := s.decoder.DecodeCandidate(runtimeResult.CandidateOutput, s.expectation(input))
	if err != nil {
		return result, err
	}
	result.Candidate = candidate
	return result, nil
}

func (s *Strategy) validateInput(input StageInput) error {
	if !validRevision(input.BaseRevision) {
		return planningError(ErrorClassInputInvalid, "base_revision")
	}
	if err := s.validator.ValidateProjectContext(input.ProjectContext); err != nil {
		return err
	}
	if err := validateIdentifier(input.InputArtifactID, "input_artifact_id", ErrorClassInputInvalid); err != nil {
		return err
	}
	if s.definition.RequiredInputKind == ArtifactKindIntent {
		return validateIntentInput(input)
	}
	return s.validateUpstreamInput(input)
}

func validateIntentInput(input StageInput) error {
	if input.Upstream != nil || !utf8.ValidString(input.Intent) || len(input.Intent) > MaxIntentBytes {
		return planningError(ErrorClassInputInvalid, "intent")
	}
	if strings.TrimSpace(input.Intent) == "" {
		return planningError(ErrorClassInputInvalid, "intent")
	}
	return nil
}

func (s *Strategy) validateUpstreamInput(input StageInput) error {
	if input.Intent != "" || input.Upstream == nil || !input.Upstream.validated {
		return planningError(ErrorClassInputInvalid, "upstream")
	}
	envelope := input.Upstream.envelope
	expectedStage := StageUnderstand
	if s.definition.RequiredInputKind == ArtifactKindDesign {
		expectedStage = StageDesign
	}
	if envelope.Kind != s.definition.RequiredInputKind {
		return planningError(ErrorClassInputInvalid, "upstream.kind")
	}
	if envelope.SourceRevision != input.BaseRevision {
		return planningError(ErrorClassRevisionMismatch, "upstream.source_revision")
	}
	expected := CandidateExpectation{Stage: expectedStage, SourceRevision: input.BaseRevision, InputArtifactIDs: envelope.InputArtifactIDs}
	return s.validator.ValidateCandidate(*input.Upstream, expected)
}

func (s *Strategy) expectation(input StageInput) CandidateExpectation {
	return CandidateExpectation{
		Stage: s.definition.Stage, SourceRevision: input.BaseRevision,
		InputArtifactIDs: []string{input.InputArtifactID},
	}
}

func classifyRuntimeResult(result RuntimeResult, err error) error {
	if result.TimedOut || errors.Is(err, context.DeadlineExceeded) {
		return planningError(ErrorClassRuntimeTimeout, "runtime")
	}
	if err != nil || result.ExitCode != 0 {
		return planningError(ErrorClassRuntimeFailed, "runtime")
	}
	if len(result.CandidateOutput) == 0 {
		return planningError(ErrorClassDecodeInvalid, "candidate_output")
	}
	return nil
}

func newRuntimeObservation(startedAt, completedAt time.Time, result RuntimeResult) RuntimeObservation {
	return RuntimeObservation{
		StartedAt: startedAt, CompletedAt: completedAt,
		ExitCode: result.ExitCode, TimedOut: result.TimedOut,
		CandidateOutput: captureBytes(result.CandidateOutput, MaxArtifactBytes),
		RawLog:          captureBytes(result.RawLog, MaxRawLogBytes),
	}
}

func captureBytes(value []byte, limit int) CapturedBytes {
	originalSize := len(value)
	truncated := originalSize > limit
	if truncated {
		value = value[:limit]
	}
	content := append([]byte(nil), value...)
	digest := sha256.Sum256(content)
	return CapturedBytes{
		Content: content, SHA256: hex.EncodeToString(digest[:]),
		SizeBytes: int64(len(content)), OriginalSizeBytes: int64(originalSize), Truncated: truncated,
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

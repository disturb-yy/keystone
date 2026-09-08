package planning

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStageDefinitionsAreStrictlyOrderedAndBounded(t *testing.T) {
	tests := []struct {
		stage     Stage
		inputKind ArtifactKind
		next      Stage
		hasNext   bool
	}{
		{stage: StageUnderstand, inputKind: ArtifactKindIntent, next: StageDesign, hasNext: true},
		{stage: StageDesign, inputKind: ArtifactKindUnderstanding, next: StagePlan, hasNext: true},
		{stage: StagePlan, inputKind: ArtifactKindDesign, hasNext: false},
	}
	for _, test := range tests {
		t.Run(string(test.stage), func(t *testing.T) {
			definition, err := DefinitionForStage(test.stage)
			if err != nil {
				t.Fatal(err)
			}
			if definition.RequiredInputKind != test.inputKind || definition.Capability != RuntimeCapabilityPlanningReadOnly {
				t.Fatalf("DefinitionForStage() = %#v", definition)
			}
			next, ok := NextStage(test.stage)
			if next != test.next || ok != test.hasNext {
				t.Fatalf("NextStage() = %q, %t, want %q, %t", next, ok, test.next, test.hasNext)
			}
		})
	}
	if _, err := DefinitionForStage("Execute"); !errors.Is(err, ErrInputInvalid) {
		t.Fatalf("unknown stage error = %v, want ErrInputInvalid", err)
	}
}

func TestStrategyPrepareBuildsDeterministicAuthorizedRequest(t *testing.T) {
	for _, stage := range []Stage{StageUnderstand, StageDesign, StagePlan} {
		t.Run(string(stage), func(t *testing.T) {
			strategy, err := NewStrategy(stage, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			input := validStageInput(t, stage, testRevision)
			first, err := strategy.Prepare(input)
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			second, err := strategy.Prepare(input)
			if err != nil || first != second {
				t.Fatalf("Prepare() is not deterministic: first=%#v second=%#v err=%v", first, second, err)
			}
			definition := strategy.Definition()
			if first.Stage != stage || first.SchemaVersion != definition.SchemaVersion || first.Capability != RuntimeCapabilityPlanningReadOnly || first.BaseRevision != testRevision {
				t.Fatalf("RuntimeRequest = %#v", first)
			}
			for _, forbidden := range []string{"/home/jadon", "WORKER_PROTOCOL_SECRET", "DATABASE_URL"} {
				if strings.Contains(first.Instruction, forbidden) {
					t.Fatalf("prompt contains forbidden runtime data %q", forbidden)
				}
			}
			if !strings.Contains(first.Instruction, testRevision) || !strings.Contains(first.Instruction, string(definition.RequiredInputKind)) {
				t.Fatalf("prompt does not preserve fixed inputs: %s", first.Instruction)
			}
		})
	}
}

func TestStrategyPrepareAndDecodeDoNotRequireSynchronousRuntime(t *testing.T) {
	strategy := NewUnderstandStrategy(nil)
	input := validStageInput(t, StageUnderstand, testRevision)
	request, err := strategy.Prepare(input)
	if err != nil || request.Instruction == "" {
		t.Fatalf("Prepare() = %#v, %v", request, err)
	}
	candidate, err := strategy.Decode(validCandidateJSON(t, StageUnderstand, testRevision, input.InputArtifactID), input)
	if err != nil || !candidate.Validated() {
		t.Fatalf("Decode() candidate validated = %t, error = %v", candidate.Validated(), err)
	}
}

func TestStrategyRejectsMissingOrReplacedUpstream(t *testing.T) {
	understanding := decodeCandidate(t, StageUnderstand, testRevision, "intent-ref")
	otherRevision := strings.Repeat("a", 40)
	understandingAtOtherRevision := decodeCandidate(t, StageUnderstand, otherRevision, "intent-ref")
	tests := []struct {
		name     string
		strategy *Strategy
		input    StageInput
		want     error
	}{
		{name: "design missing upstream", strategy: NewDesignStrategy(nil), input: StageInput{BaseRevision: testRevision, ProjectContext: validProjectContext(), InputArtifactID: "understanding-ref"}, want: ErrInputInvalid},
		{name: "plan receives understanding", strategy: NewPlanStrategy(nil), input: StageInput{BaseRevision: testRevision, ProjectContext: validProjectContext(), InputArtifactID: "design-ref", Upstream: &understanding}, want: ErrInputInvalid},
		{name: "upstream revision replaced", strategy: NewDesignStrategy(nil), input: StageInput{BaseRevision: testRevision, ProjectContext: validProjectContext(), InputArtifactID: "understanding-ref", Upstream: &understandingAtOtherRevision}, want: ErrRevisionMismatch},
		{name: "understand receives upstream", strategy: NewUnderstandStrategy(nil), input: StageInput{BaseRevision: testRevision, ProjectContext: validProjectContext(), InputArtifactID: "intent-ref", Intent: "intent", Upstream: &understanding}, want: ErrInputInvalid},
		{name: "unvalidated upstream", strategy: NewDesignStrategy(nil), input: StageInput{BaseRevision: testRevision, ProjectContext: validProjectContext(), InputArtifactID: "understanding-ref", Upstream: &Candidate{}}, want: ErrInputInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.strategy.Prepare(test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("Prepare() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestStrategyRejectsInvalidContextRevisionAndIntent(t *testing.T) {
	tests := []struct {
		name  string
		input StageInput
		want  error
	}{
		{name: "unsupported context", input: StageInput{BaseRevision: testRevision, ProjectContext: ProjectContext{SchemaVersion: "ProjectContext.v2", ProjectID: "project-1"}, InputArtifactID: "intent-ref", Intent: "intent"}, want: ErrInputInvalid},
		{name: "invalid revision", input: StageInput{BaseRevision: "HEAD", ProjectContext: validProjectContext(), InputArtifactID: "intent-ref", Intent: "intent"}, want: ErrInputInvalid},
		{name: "empty intent", input: StageInput{BaseRevision: testRevision, ProjectContext: validProjectContext(), InputArtifactID: "intent-ref", Intent: " \n\t"}, want: ErrInputInvalid},
		{name: "oversized intent", input: StageInput{BaseRevision: testRevision, ProjectContext: validProjectContext(), InputArtifactID: "intent-ref", Intent: strings.Repeat("x", MaxIntentBytes+1)}, want: ErrInputInvalid},
	}
	strategy := NewUnderstandStrategy(nil)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := strategy.Prepare(test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("Prepare() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestStrategyExecuteReturnsValidatedCandidateAndObservation(t *testing.T) {
	input := validStageInput(t, StageUnderstand, testRevision)
	runtime := &fakeRuntime{result: RuntimeResult{
		CandidateOutput: validCandidateJSON(t, StageUnderstand, testRevision, input.InputArtifactID),
		RawLog:          []byte("runtime log"),
	}}
	startedAt := time.Date(2026, 9, 7, 10, 0, 0, 0, time.FixedZone("test", 8*60*60))
	completedAt := startedAt.Add(time.Second)
	clock := &fakeClock{values: []time.Time{startedAt, completedAt}}
	strategy, err := NewStrategy(StageUnderstand, runtime, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}
	result, err := strategy.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runtime.calls != 1 || runtime.request.Stage != StageUnderstand || !result.Candidate.Validated() {
		t.Fatalf("runtime/candidate state = calls:%d request:%#v candidate:%t", runtime.calls, runtime.request, result.Candidate.Validated())
	}
	if !result.Observation.StartedAt.Equal(startedAt.UTC()) || !result.Observation.CompletedAt.Equal(completedAt.UTC()) {
		t.Fatalf("observation times = %s/%s", result.Observation.StartedAt, result.Observation.CompletedAt)
	}
	if string(result.Observation.RawLog.Content) != "runtime log" || result.Observation.RawLog.SHA256 == "" {
		t.Fatalf("raw log capture = %#v", result.Observation.RawLog)
	}
}

func TestStrategyExecuteClassifiesFailuresAndPreservesObservation(t *testing.T) {
	input := validStageInput(t, StageUnderstand, testRevision)
	validOutput := validCandidateJSON(t, StageUnderstand, testRevision, input.InputArtifactID)
	wrongRevision := validCandidateJSON(t, StageUnderstand, strings.Repeat("a", 40), input.InputArtifactID)
	missingGoal := []byte(`{"kind":"understanding","schema_version":"Understanding.v1","stage":"Understand","summary":"summary","source_revision":"` + testRevision + `","input_artifact_ids":["intent-ref"],"payload":{"problem":"problem","goals":[],"constraints":[]}}`)
	tests := []struct {
		name       string
		result     RuntimeResult
		runtimeErr error
		want       error
	}{
		{name: "nonzero exit", result: RuntimeResult{CandidateOutput: validOutput, RawLog: []byte("failed"), ExitCode: 2}, want: ErrRuntimeFailed},
		{name: "timeout flag", result: RuntimeResult{CandidateOutput: validOutput, RawLog: []byte("timeout"), TimedOut: true}, want: ErrRuntimeTimeout},
		{name: "deadline error", result: RuntimeResult{RawLog: []byte("deadline")}, runtimeErr: context.DeadlineExceeded, want: ErrRuntimeTimeout},
		{name: "start failure", result: RuntimeResult{RawLog: []byte("start")}, runtimeErr: errors.New("start failed at /secret/path"), want: ErrRuntimeFailed},
		{name: "empty output", result: RuntimeResult{RawLog: []byte("empty")}, want: ErrDecodeInvalid},
		{name: "invalid json", result: RuntimeResult{CandidateOutput: []byte(`not-json`), RawLog: []byte("invalid")}, want: ErrDecodeInvalid},
		{name: "schema invalid", result: RuntimeResult{CandidateOutput: missingGoal, RawLog: []byte("schema")}, want: ErrSchemaInvalid},
		{name: "revision mismatch", result: RuntimeResult{CandidateOutput: wrongRevision, RawLog: []byte("revision")}, want: ErrRevisionMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeRuntime{result: test.result, err: test.runtimeErr}
			clock := &fakeClock{values: []time.Time{time.Unix(1, 0), time.Unix(2, 0)}}
			strategy, err := NewStrategy(StageUnderstand, runtime, nil, nil, clock)
			if err != nil {
				t.Fatal(err)
			}
			result, err := strategy.Execute(context.Background(), input)
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
			if runtime.calls != 1 || string(result.Observation.RawLog.Content) != string(test.result.RawLog) {
				t.Fatalf("failure observation = %#v, calls = %d", result.Observation, runtime.calls)
			}
			if test.runtimeErr != nil && strings.Contains(err.Error(), "/secret/path") {
				t.Fatalf("runtime error leaked concrete detail: %v", err)
			}
		})
	}
}

func TestStrategyObservationBoundsRawOutputAndLog(t *testing.T) {
	input := validStageInput(t, StageUnderstand, testRevision)
	runtime := &fakeRuntime{result: RuntimeResult{
		CandidateOutput: make([]byte, MaxArtifactBytes+17),
		RawLog:          make([]byte, MaxRawLogBytes+23),
		ExitCode:        1,
	}}
	strategy := NewUnderstandStrategy(runtime)
	result, err := strategy.Execute(context.Background(), input)
	if !errors.Is(err, ErrRuntimeFailed) {
		t.Fatalf("Execute() error = %v, want ErrRuntimeFailed", err)
	}
	assertCaptureBound(t, result.Observation.CandidateOutput, MaxArtifactBytes, MaxArtifactBytes+17)
	assertCaptureBound(t, result.Observation.RawLog, MaxRawLogBytes, MaxRawLogBytes+23)
}

func validStageInput(t *testing.T, stage Stage, revision string) StageInput {
	t.Helper()
	switch stage {
	case StageUnderstand:
		return StageInput{BaseRevision: revision, ProjectContext: validProjectContext(), InputArtifactID: "intent-ref", Intent: "implement planning"}
	case StageDesign:
		upstream := decodeCandidate(t, StageUnderstand, revision, "intent-ref")
		return StageInput{BaseRevision: revision, ProjectContext: validProjectContext(), InputArtifactID: "understanding-ref", Upstream: &upstream}
	default:
		upstream := decodeCandidate(t, StageDesign, revision, "understanding-ref")
		return StageInput{BaseRevision: revision, ProjectContext: validProjectContext(), InputArtifactID: "design-ref", Upstream: &upstream}
	}
}

func validProjectContext() ProjectContext {
	return ProjectContext{
		SchemaVersion: ProjectContextSchemaV1, ProjectID: "project-1", RepositoryName: "keystone",
		Languages: []string{"Go"}, RelevantPaths: []string{"internal/planning"}, Conventions: []string{"中文注释"},
	}
}

func decodeCandidate(t *testing.T, stage Stage, revision, inputID string) Candidate {
	t.Helper()
	candidate, err := NewStrictDecoder(nil).DecodeCandidate(validCandidateJSON(t, stage, revision, inputID), expectation(stage, revision, inputID))
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

type fakeRuntime struct {
	result  RuntimeResult
	err     error
	calls   int
	request RuntimeRequest
}

func (runtime *fakeRuntime) Run(_ context.Context, request RuntimeRequest) (RuntimeResult, error) {
	runtime.calls++
	runtime.request = request
	return runtime.result, runtime.err
}

type fakeClock struct {
	values []time.Time
	index  int
}

func (clock *fakeClock) Now() time.Time {
	value := clock.values[clock.index]
	clock.index++
	return value
}

func assertCaptureBound(t *testing.T, capture CapturedBytes, size, originalSize int) {
	t.Helper()
	if len(capture.Content) != size || capture.SizeBytes != int64(size) || capture.OriginalSizeBytes != int64(originalSize) || !capture.Truncated || capture.SHA256 == "" {
		t.Fatalf("capture = %#v", capture)
	}
}

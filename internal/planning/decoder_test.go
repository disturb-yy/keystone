package planning

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const testRevision = "0123456789012345678901234567890123456789"

func TestStrictDecoderAcceptsAllPlanningSchemas(t *testing.T) {
	tests := []struct {
		stage Stage
		check func(t *testing.T, candidate Candidate)
	}{
		{stage: StageUnderstand, check: func(t *testing.T, candidate Candidate) {
			payload, ok := candidate.Understanding()
			if !ok || payload.Problem != "problem" {
				t.Fatalf("Understanding() = %#v, %t", payload, ok)
			}
		}},
		{stage: StageDesign, check: func(t *testing.T, candidate Candidate) {
			payload, ok := candidate.Design()
			if !ok || payload.Approach != "approach" {
				t.Fatalf("Design() = %#v, %t", payload, ok)
			}
		}},
		{stage: StagePlan, check: func(t *testing.T, candidate Candidate) {
			payload, ok := candidate.Plan()
			if !ok || len(payload.Steps) != 1 || payload.Steps[0].Paths[0] != "internal/planning/contract.go" {
				t.Fatalf("Plan() = %#v, %t", payload, ok)
			}
		}},
	}
	decoder := NewStrictDecoder(nil)
	for _, test := range tests {
		t.Run(string(test.stage), func(t *testing.T) {
			body := validCandidateJSON(t, test.stage, testRevision, "input-ref")
			candidate, err := decoder.DecodeCandidate(body, expectation(test.stage, testRevision, "input-ref"))
			if err != nil {
				t.Fatalf("DecodeCandidate() error = %v", err)
			}
			if !candidate.Validated() || !bytes.Equal(candidate.RawJSON(), body) {
				t.Fatalf("candidate validation/raw state = %t/%q", candidate.Validated(), candidate.RawJSON())
			}
			test.check(t, candidate)
		})
	}
}

func TestStrictDecoderRejectsAmbiguousCandidateJSON(t *testing.T) {
	base := fmt.Sprintf(`{"kind":"understanding","schema_version":"Understanding.v1","stage":"Understand","summary":"summary","source_revision":"%s","input_artifact_ids":["input-ref"],"payload":{"problem":"problem","goals":["goal"],"constraints":[]}}`, testRevision)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "empty", body: nil},
		{name: "array", body: []byte(`[]`)},
		{name: "unknown envelope field", body: []byte(strings.Replace(base, `"payload":`, `"extra":true,"payload":`, 1))},
		{name: "duplicate envelope field", body: []byte(strings.Replace(base, `"summary":"summary"`, `"summary":"summary","summary":"other"`, 1))},
		{name: "unknown payload field", body: []byte(strings.Replace(base, `"constraints":[]`, `"constraints":[],"extra":true`, 1))},
		{name: "duplicate payload field", body: []byte(strings.Replace(base, `"problem":"problem"`, `"problem":"problem","problem":"other"`, 1))},
		{name: "mixed-case envelope alias", body: []byte(strings.Replace(base, `"kind":"understanding"`, `"kind":"understanding","Kind":"design"`, 1))},
		{name: "mixed-case payload alias", body: []byte(strings.Replace(base, `"problem":"problem"`, `"problem":"problem","Problem":"other"`, 1))},
		{name: "trailing value", body: []byte(base + ` {}`)},
		{name: "invalid utf8", body: append([]byte(base), 0xff)},
	}
	decoder := NewStrictDecoder(nil)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decoder.DecodeCandidate(test.body, expectation(StageUnderstand, testRevision, "input-ref"))
			if !errors.Is(err, ErrDecodeInvalid) {
				t.Fatalf("DecodeCandidate() error = %v, want ErrDecodeInvalid", err)
			}
		})
	}
}

func TestStrictDecoderRejectsMissingAndEmptyPayloadFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing summary", body: fmt.Sprintf(`{"kind":"understanding","schema_version":"Understanding.v1","stage":"Understand","source_revision":"%s","input_artifact_ids":["input-ref"],"payload":{"problem":"problem","goals":["goal"],"constraints":[]}}`, testRevision)},
		{name: "empty payload", body: fmt.Sprintf(`{"kind":"understanding","schema_version":"Understanding.v1","stage":"Understand","summary":"summary","source_revision":"%s","input_artifact_ids":["input-ref"],"payload":{}}`, testRevision)},
		{name: "missing required array", body: fmt.Sprintf(`{"kind":"understanding","schema_version":"Understanding.v1","stage":"Understand","summary":"summary","source_revision":"%s","input_artifact_ids":["input-ref"],"payload":{"problem":"problem","goals":["goal"]}}`, testRevision)},
	}
	decoder := NewStrictDecoder(nil)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decoder.DecodeCandidate([]byte(test.body), expectation(StageUnderstand, testRevision, "input-ref"))
			if !errors.Is(err, ErrSchemaInvalid) {
				t.Fatalf("DecodeCandidate() error = %v, want ErrSchemaInvalid", err)
			}
		})
	}
}

func TestStrictDecoderRejectsRevisionAndInputReferenceMismatch(t *testing.T) {
	decoder := NewStrictDecoder(nil)
	body := validCandidateJSON(t, StageUnderstand, testRevision, "input-ref")
	_, err := decoder.DecodeCandidate(body, expectation(StageUnderstand, strings.Repeat("a", 40), "input-ref"))
	if !errors.Is(err, ErrRevisionMismatch) || ClassifyError(err) != ErrorClassRevisionMismatch {
		t.Fatalf("revision mismatch error = %v, class = %q", err, ClassifyError(err))
	}
	_, err = decoder.DecodeCandidate(body, expectation(StageUnderstand, testRevision, "other-ref"))
	if !errors.Is(err, ErrSchemaInvalid) {
		t.Fatalf("input reference mismatch error = %v, want ErrSchemaInvalid", err)
	}
}

func TestStrictDecoderEnforcesCandidateBounds(t *testing.T) {
	tests := []struct {
		name  string
		stage Stage
		edit  func(*ArtifactEnvelope, any)
	}{
		{name: "summary runes", stage: StageUnderstand, edit: func(envelope *ArtifactEnvelope, _ any) {
			envelope.Summary = strings.Repeat("界", MaxSummaryRunes+1)
		}},
		{name: "text bytes", stage: StageUnderstand, edit: func(_ *ArtifactEnvelope, payload any) {
			payload.(*UnderstandingPayload).Problem = strings.Repeat("x", MaxTextFieldBytes+1)
		}},
		{name: "ordinary array", stage: StageDesign, edit: func(_ *ArtifactEnvelope, payload any) {
			payload.(*DesignPayload).Risks = repeatedStrings(MaxArrayItems + 1)
		}},
		{name: "plan steps", stage: StagePlan, edit: func(_ *ArtifactEnvelope, payload any) {
			step := PlanStep{ID: "step", Summary: "summary", Paths: []string{"internal/planning"}}
			payload.(*PlanPayload).Steps = make([]PlanStep, MaxPlanSteps+1)
			for index := range payload.(*PlanPayload).Steps {
				payload.(*PlanPayload).Steps[index] = step
			}
		}},
	}
	decoder := NewStrictDecoder(nil)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope, payload := validEnvelopeAndPayload(test.stage, testRevision, "input-ref")
			test.edit(&envelope, payload)
			body := marshalEnvelope(t, envelope, payload)
			_, err := decoder.DecodeCandidate(body, expectation(test.stage, testRevision, "input-ref"))
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("DecodeCandidate() error = %v, want ErrLimitExceeded", err)
			}
		})
	}
	oversized := bytes.Repeat([]byte("x"), MaxArtifactBytes+1)
	if _, err := decoder.DecodeCandidate(oversized, expectation(StageUnderstand, testRevision, "input-ref")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("oversized candidate error = %v, want ErrLimitExceeded", err)
	}
}

func TestStrictDecoderRejectsUnsafePlanPaths(t *testing.T) {
	paths := []string{"/etc/passwd", "../outside", "internal/../outside", `C:\\outside`, `internal\\planning`, "internal/file:stream", "internal/file\x00name", "internal/file\nother"}
	decoder := NewStrictDecoder(nil)
	for _, unsafePath := range paths {
		t.Run(unsafePath, func(t *testing.T) {
			envelope, payload := validEnvelopeAndPayload(StagePlan, testRevision, "design-ref")
			payload.(*PlanPayload).Steps[0].Paths = []string{unsafePath}
			body := marshalEnvelope(t, envelope, payload)
			_, err := decoder.DecodeCandidate(body, expectation(StagePlan, testRevision, "design-ref"))
			if !errors.Is(err, ErrSchemaInvalid) {
				t.Fatalf("unsafe path error = %v, want ErrSchemaInvalid", err)
			}
			var fieldError *PlanningError
			if !errors.As(err, &fieldError) || !strings.Contains(fieldError.Field, "paths") {
				t.Fatalf("unsafe path field context = %#v", fieldError)
			}
		})
	}
}

func TestStrictDecoderValidatesProjectContext(t *testing.T) {
	decoder := NewStrictDecoder(nil)
	valid := []byte(`{"schema_version":"ProjectContext.v1","project_id":"project-1","repository_name":"keystone","languages":["Go"],"relevant_paths":["internal/planning"],"conventions":["中文注释"]}`)
	context, err := decoder.DecodeProjectContext(valid)
	if err != nil || context.ProjectID != "project-1" {
		t.Fatalf("DecodeProjectContext() = %#v, %v", context, err)
	}
	tests := []struct {
		name string
		body []byte
		want error
	}{
		{name: "unknown", body: []byte(`{"schema_version":"ProjectContext.v1","project_id":"project-1","root":"/tmp/repo"}`), want: ErrDecodeInvalid},
		{name: "duplicate", body: []byte(`{"schema_version":"ProjectContext.v1","project_id":"one","project_id":"two"}`), want: ErrDecodeInvalid},
		{name: "absolute relevant path", body: []byte(`{"schema_version":"ProjectContext.v1","project_id":"project-1","relevant_paths":["/tmp/repo"]}`), want: ErrInputInvalid},
		{name: "absolute repository name", body: []byte(`{"schema_version":"ProjectContext.v1","project_id":"project-1","repository_name":"C:/repo"}`), want: ErrInputInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decoder.DecodeProjectContext(test.body)
			if !errors.Is(err, test.want) {
				t.Fatalf("DecodeProjectContext() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSchemaValidatorEnforcesProjectContextTotalBound(t *testing.T) {
	context := ProjectContext{
		SchemaVersion: ProjectContextSchemaV1,
		ProjectID:     "project-1",
		Conventions:   repeatedStringsWithSize(9, 8_000),
	}
	err := NewSchemaValidator().ValidateProjectContext(context)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("ValidateProjectContext() error = %v, want ErrLimitExceeded", err)
	}
}

func TestPlanningErrorDoesNotExposeCandidateOrParserDetail(t *testing.T) {
	secret := "database-password-do-not-log"
	body := []byte(`{"kind":"understanding","unknown":"` + secret + `"}`)
	_, err := NewStrictDecoder(nil).DecodeCandidate(body, expectation(StageUnderstand, testRevision, "input-ref"))
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error leaked raw detail: %v", err)
	}
}

func validCandidateJSON(t *testing.T, stage Stage, revision, inputID string) []byte {
	t.Helper()
	envelope, payload := validEnvelopeAndPayload(stage, revision, inputID)
	return marshalEnvelope(t, envelope, payload)
}

func validEnvelopeAndPayload(stage Stage, revision, inputID string) (ArtifactEnvelope, any) {
	kind, schema, _ := contractForStage(stage)
	envelope := ArtifactEnvelope{
		Kind: kind, SchemaVersion: schema, Stage: stage, Summary: "summary",
		SourceRevision: revision, InputArtifactIDs: []string{inputID},
	}
	switch stage {
	case StageUnderstand:
		return envelope, &UnderstandingPayload{Problem: "problem", Goals: []string{"goal"}, Constraints: []string{}}
	case StageDesign:
		return envelope, &DesignPayload{Approach: "approach", Decisions: []string{"decision"}, Risks: []string{}}
	default:
		return envelope, &PlanPayload{
			Steps:        []PlanStep{{ID: "step-1", Summary: "implement", Paths: []string{"internal/planning/contract.go"}}},
			Verification: []string{"go test ./internal/planning/..."},
		}
	}
}

func marshalEnvelope(t *testing.T, envelope ArtifactEnvelope, payload any) []byte {
	t.Helper()
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Payload = encodedPayload
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func expectation(stage Stage, revision, inputID string) CandidateExpectation {
	return CandidateExpectation{Stage: stage, SourceRevision: revision, InputArtifactIDs: []string{inputID}}
}

func repeatedStrings(count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = "item"
	}
	return values
}

func repeatedStringsWithSize(count, size int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = strings.Repeat("x", size)
	}
	return values
}

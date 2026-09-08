package planning

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// StrictDecoder 拒绝模糊 JSON，并在返回 Candidate 前调用 schema validator。
type StrictDecoder struct {
	validator Validator
}

// NewStrictDecoder 创建严格 decoder；validator 为空时使用冻结规则实现。
func NewStrictDecoder(validator Validator) *StrictDecoder {
	if validator == nil {
		validator = NewSchemaValidator()
	}
	return &StrictDecoder{validator: validator}
}

// DecodeProjectContext 解码一个有界、无未知或重复字段的 ProjectContext.v1 object。
func (d *StrictDecoder) DecodeProjectContext(body []byte) (ProjectContext, error) {
	var context ProjectContext
	if err := strictDecodeObject(body, MaxProjectContextBytes, &context); err != nil {
		return ProjectContext{}, err
	}
	if err := d.validator.ValidateProjectContext(context); err != nil {
		return ProjectContext{}, err
	}
	return cloneProjectContext(context), nil
}

// DecodeCandidate 解码、类型化并校验一个 Runtime 候选，不产生权威状态。
func (d *StrictDecoder) DecodeCandidate(body []byte, expected CandidateExpectation) (Candidate, error) {
	var envelope ArtifactEnvelope
	if err := strictDecodeObject(body, MaxArtifactBytes, &envelope); err != nil {
		return Candidate{}, err
	}
	candidate := Candidate{
		envelope: cloneEnvelope(envelope),
		rawJSON:  append([]byte(nil), body...),
	}
	if err := decodeTypedPayload(envelope.Stage, envelope.Payload, &candidate); err != nil {
		return Candidate{}, err
	}
	if err := d.validator.ValidateCandidate(candidate, expected); err != nil {
		return Candidate{}, err
	}
	candidate.validated = true
	return candidate, nil
}

func decodeTypedPayload(stage Stage, body []byte, candidate *Candidate) error {
	switch stage {
	case StageUnderstand:
		var payload UnderstandingPayload
		if err := strictDecodeObject(body, MaxArtifactBytes, &payload); err != nil {
			return err
		}
		candidate.understanding = &payload
	case StageDesign:
		var payload DesignPayload
		if err := strictDecodeObject(body, MaxArtifactBytes, &payload); err != nil {
			return err
		}
		candidate.design = &payload
	case StagePlan:
		var payload PlanPayload
		if err := strictDecodeObject(body, MaxArtifactBytes, &payload); err != nil {
			return err
		}
		candidate.plan = &payload
	default:
		return planningError(ErrorClassSchemaInvalid, "stage")
	}
	return nil
}

func strictDecodeObject(body []byte, limit int, destination any) error {
	if len(body) == 0 {
		return planningError(ErrorClassDecodeInvalid, "body")
	}
	if len(body) > limit {
		return planningError(ErrorClassLimitExceeded, "body")
	}
	if !utf8.Valid(body) {
		return planningError(ErrorClassDecodeInvalid, "body")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return planningError(ErrorClassDecodeInvalid, "body")
	}
	if err := validateUniqueJSON(trimmed); err != nil {
		return planningError(ErrorClassDecodeInvalid, "body")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return planningError(ErrorClassDecodeInvalid, "body")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return planningError(ErrorClassDecodeInvalid, "body")
	}
	return nil
}

func validateUniqueJSON(value []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	if err := walkJSON(decoder); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func walkJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, nested := token.(json.Delim)
	if !nested {
		return nil
	}
	if delimiter == '[' {
		for decoder.More() {
			if err := walkJSON(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if delimiter != '{' {
		return errors.New("unexpected JSON delimiter")
	}
	return walkJSONObject(decoder)
}

func walkJSONObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("object key is not a string")
		}
		if key != strings.ToLower(key) {
			return errors.New("JSON key is not canonical lowercase")
		}
		if _, exists := seen[key]; exists {
			return errors.New("duplicate JSON key")
		}
		seen[key] = struct{}{}
		if err := walkJSON(decoder); err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
}

func cloneProjectContext(value ProjectContext) ProjectContext {
	value.Languages = append([]string(nil), value.Languages...)
	value.RelevantPaths = append([]string(nil), value.RelevantPaths...)
	value.Conventions = append([]string(nil), value.Conventions...)
	return value
}

package planning

import (
	"encoding/json"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SchemaValidator 实现 Ticket 07 冻结的 Context、Artifact 和字段上限。
type SchemaValidator struct{}

// NewSchemaValidator 创建无外部依赖的确定性 validator。
func NewSchemaValidator() *SchemaValidator { return &SchemaValidator{} }

// ValidateProjectContext 检查版本、字段、相对路径和编码后总大小。
func (v *SchemaValidator) ValidateProjectContext(context ProjectContext) error {
	if context.SchemaVersion != ProjectContextSchemaV1 {
		return planningError(ErrorClassInputInvalid, "project_context.schema_version")
	}
	if err := validateIdentifier(context.ProjectID, "project_context.project_id", ErrorClassInputInvalid); err != nil {
		return err
	}
	if err := validateOptionalText(context.RepositoryName, "project_context.repository_name", ErrorClassInputInvalid); err != nil {
		return err
	}
	if looksAbsoluteHostPath(context.RepositoryName) {
		return planningError(ErrorClassInputInvalid, "project_context.repository_name")
	}
	if err := validateOptionalStringArray(context.Languages, "project_context.languages", ErrorClassInputInvalid); err != nil {
		return err
	}
	if err := validateOptionalStringArray(context.Conventions, "project_context.conventions", ErrorClassInputInvalid); err != nil {
		return err
	}
	if err := validateOptionalPaths(context.RelevantPaths, "project_context.relevant_paths", ErrorClassInputInvalid); err != nil {
		return err
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		return planningError(ErrorClassInputInvalid, "project_context")
	}
	if len(encoded) > MaxProjectContextBytes {
		return planningError(ErrorClassLimitExceeded, "project_context")
	}
	return nil
}

// ValidateCandidate 检查 envelope、阶段 payload 和本次固定输入是否一致。
func (v *SchemaValidator) ValidateCandidate(candidate Candidate, expected CandidateExpectation) error {
	if err := validateExpectation(expected); err != nil {
		return err
	}
	if err := validateCandidateSize(candidate); err != nil {
		return err
	}
	if err := validateEnvelope(candidate.envelope); err != nil {
		return err
	}
	if err := matchExpectation(candidate.envelope, expected); err != nil {
		return err
	}
	switch expected.Stage {
	case StageUnderstand:
		return validateUnderstandingCandidate(candidate)
	case StageDesign:
		return validateDesignCandidate(candidate)
	case StagePlan:
		return validatePlanCandidate(candidate)
	default:
		return planningError(ErrorClassInputInvalid, "expectation.stage")
	}
}

func validateExpectation(expected CandidateExpectation) error {
	if _, _, ok := contractForStage(expected.Stage); !ok {
		return planningError(ErrorClassInputInvalid, "expectation.stage")
	}
	if !validRevision(expected.SourceRevision) {
		return planningError(ErrorClassInputInvalid, "expectation.source_revision")
	}
	if len(expected.InputArtifactIDs) != 1 {
		return planningError(ErrorClassInputInvalid, "expectation.input_artifact_ids")
	}
	return validateIdentifier(expected.InputArtifactIDs[0], "expectation.input_artifact_ids", ErrorClassInputInvalid)
}

func validateCandidateSize(candidate Candidate) error {
	if len(candidate.rawJSON) > MaxArtifactBytes {
		return planningError(ErrorClassLimitExceeded, "artifact")
	}
	if len(candidate.rawJSON) != 0 {
		return nil
	}
	encoded, err := json.Marshal(candidate.envelope)
	if err != nil {
		return planningError(ErrorClassSchemaInvalid, "artifact")
	}
	if len(encoded) > MaxArtifactBytes {
		return planningError(ErrorClassLimitExceeded, "artifact")
	}
	return nil
}

func validateEnvelope(envelope ArtifactEnvelope) error {
	kind, schema, ok := contractForStage(envelope.Stage)
	if !ok || envelope.Kind != kind {
		return planningError(ErrorClassSchemaInvalid, "kind")
	}
	if envelope.SchemaVersion != schema {
		return planningError(ErrorClassSchemaInvalid, "schema_version")
	}
	if err := validateSummary(envelope.Summary); err != nil {
		return err
	}
	if !validRevision(envelope.SourceRevision) {
		return planningError(ErrorClassSchemaInvalid, "source_revision")
	}
	if len(envelope.InputArtifactIDs) != 1 {
		return planningError(ErrorClassSchemaInvalid, "input_artifact_ids")
	}
	return validateIdentifier(envelope.InputArtifactIDs[0], "input_artifact_ids", ErrorClassSchemaInvalid)
}

func matchExpectation(envelope ArtifactEnvelope, expected CandidateExpectation) error {
	if envelope.Stage != expected.Stage {
		return planningError(ErrorClassSchemaInvalid, "stage")
	}
	if envelope.SourceRevision != expected.SourceRevision {
		return planningError(ErrorClassRevisionMismatch, "source_revision")
	}
	if !equalStrings(envelope.InputArtifactIDs, expected.InputArtifactIDs) {
		return planningError(ErrorClassSchemaInvalid, "input_artifact_ids")
	}
	return nil
}

func validateUnderstandingCandidate(candidate Candidate) error {
	if candidate.understanding == nil || candidate.design != nil || candidate.plan != nil {
		return planningError(ErrorClassSchemaInvalid, "payload")
	}
	payload := *candidate.understanding
	if err := validateRequiredText(payload.Problem, "payload.problem", ErrorClassSchemaInvalid); err != nil {
		return err
	}
	if err := validateStringArray(payload.Goals, "payload.goals", true, ErrorClassSchemaInvalid); err != nil {
		return err
	}
	return validateStringArray(payload.Constraints, "payload.constraints", false, ErrorClassSchemaInvalid)
}

func validateDesignCandidate(candidate Candidate) error {
	if candidate.design == nil || candidate.understanding != nil || candidate.plan != nil {
		return planningError(ErrorClassSchemaInvalid, "payload")
	}
	payload := *candidate.design
	if err := validateRequiredText(payload.Approach, "payload.approach", ErrorClassSchemaInvalid); err != nil {
		return err
	}
	if err := validateStringArray(payload.Decisions, "payload.decisions", true, ErrorClassSchemaInvalid); err != nil {
		return err
	}
	return validateStringArray(payload.Risks, "payload.risks", false, ErrorClassSchemaInvalid)
}

func validatePlanCandidate(candidate Candidate) error {
	if candidate.plan == nil || candidate.understanding != nil || candidate.design != nil {
		return planningError(ErrorClassSchemaInvalid, "payload")
	}
	payload := *candidate.plan
	if len(payload.Steps) == 0 {
		return planningError(ErrorClassSchemaInvalid, "payload.steps")
	}
	if len(payload.Steps) > MaxPlanSteps {
		return planningError(ErrorClassLimitExceeded, "payload.steps")
	}
	for index, step := range payload.Steps {
		if err := validatePlanStep(step, fieldAt("payload.steps", index)); err != nil {
			return err
		}
	}
	return validateStringArray(payload.Verification, "payload.verification", true, ErrorClassSchemaInvalid)
}

func validatePlanStep(step PlanStep, field string) error {
	if err := validateIdentifier(step.ID, field+".id", ErrorClassSchemaInvalid); err != nil {
		return err
	}
	if err := validateRequiredText(step.Summary, field+".summary", ErrorClassSchemaInvalid); err != nil {
		return err
	}
	return validatePaths(step.Paths, field+".paths", true, ErrorClassSchemaInvalid)
}

func validateSummary(value string) error {
	if err := validateRequiredText(value, "summary", ErrorClassSchemaInvalid); err != nil {
		return err
	}
	if utf8.RuneCountInString(value) > MaxSummaryRunes {
		return planningError(ErrorClassLimitExceeded, "summary")
	}
	return nil
}

func validateStringArray(values []string, field string, requireItem bool, class ErrorClass) error {
	if values == nil || (requireItem && len(values) == 0) {
		return planningError(class, field)
	}
	if len(values) > MaxArrayItems {
		return planningError(ErrorClassLimitExceeded, field)
	}
	for index, value := range values {
		if err := validateRequiredText(value, fieldAt(field, index), class); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalStringArray(values []string, field string, class ErrorClass) error {
	if values == nil {
		return nil
	}
	return validateStringArray(values, field, false, class)
}

func validatePaths(values []string, field string, required bool, class ErrorClass) error {
	if values == nil || (required && len(values) == 0) {
		return planningError(class, field)
	}
	if len(values) > MaxArrayItems {
		return planningError(ErrorClassLimitExceeded, field)
	}
	for index, value := range values {
		if err := validateRelativePath(value, fieldAt(field, index), class); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalPaths(values []string, field string, class ErrorClass) error {
	if values == nil {
		return nil
	}
	return validatePaths(values, field, false, class)
}

func validateRelativePath(value, field string, class ErrorClass) error {
	if err := validateRequiredText(value, field, class); err != nil {
		return err
	}
	if value == "." || path.IsAbs(value) || path.Clean(value) != value || strings.ContainsAny(value, "\\:") {
		return planningError(class, field)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return planningError(class, field)
		}
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." {
			return planningError(class, field)
		}
	}
	return nil
}

func looksAbsoluteHostPath(value string) bool {
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") {
		return true
	}
	return len(value) >= 2 && value[1] == ':'
}

func validateIdentifier(value, field string, class ErrorClass) error {
	if err := validateRequiredText(value, field, class); err != nil {
		return err
	}
	if strings.TrimSpace(value) != value {
		return planningError(class, field)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return planningError(class, field)
		}
	}
	return nil
}

func validateOptionalText(value, field string, class ErrorClass) error {
	if value == "" {
		return nil
	}
	return validateRequiredText(value, field, class)
}

func validateRequiredText(value, field string, class ErrorClass) error {
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" {
		return planningError(class, field)
	}
	if len([]byte(value)) > MaxTextFieldBytes {
		return planningError(ErrorClassLimitExceeded, field)
	}
	return nil
}

func validRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || (character > '9' && character < 'a') || character > 'f' {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func contractForStage(stage Stage) (ArtifactKind, string, bool) {
	switch stage {
	case StageUnderstand:
		return ArtifactKindUnderstanding, UnderstandingSchemaV1, true
	case StageDesign:
		return ArtifactKindDesign, DesignSchemaV1, true
	case StagePlan:
		return ArtifactKindPlan, PlanSchemaV1, true
	default:
		return "", "", false
	}
}

func fieldAt(field string, index int) string {
	const digits = "0123456789"
	if index < 10 {
		return field + "[" + string(digits[index]) + "]"
	}
	return field + "[*]"
}

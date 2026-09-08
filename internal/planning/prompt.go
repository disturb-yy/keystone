package planning

import (
	"encoding/json"
)

const promptPreamble = "Produce exactly one UTF-8 JSON object for the requested Keystone Planning stage. Return no prose, markdown, status, file list, or lifecycle claim."

type promptDocument struct {
	Stage          Stage                `json:"stage"`
	SchemaVersion  string               `json:"schema_version"`
	BaseRevision   string               `json:"base_revision"`
	ProjectContext ProjectContext       `json:"project_context"`
	Input          promptInput          `json:"input"`
	OutputContract promptOutputContract `json:"output_contract"`
}

type promptInput struct {
	ArtifactID string          `json:"artifact_id"`
	Kind       ArtifactKind    `json:"kind"`
	Intent     string          `json:"intent,omitempty"`
	Upstream   json.RawMessage `json:"upstream,omitempty"`
}

type promptOutputContract struct {
	Kind                 ArtifactKind `json:"kind"`
	SchemaVersion        string       `json:"schema_version"`
	SourceRevision       string       `json:"source_revision"`
	InputArtifactIDs     []string     `json:"input_artifact_ids"`
	RequiredEnvelopeKeys []string     `json:"required_envelope_keys"`
	RequiredPayloadKeys  []string     `json:"required_payload_keys"`
	RequiredPlanStepKeys []string     `json:"required_plan_step_keys,omitempty"`
	PathsMustBeRelative  bool         `json:"paths_must_be_relative"`
	MaxSummaryRunes      int          `json:"max_summary_runes"`
	MaxTextFieldBytes    int          `json:"max_text_field_bytes"`
	MaxArrayItems        int          `json:"max_array_items"`
	MaxPlanSteps         int          `json:"max_plan_steps"`
}

func buildPrompt(definition StageDefinition, input StageInput) (string, error) {
	kind, _, ok := contractForStage(definition.Stage)
	if !ok {
		return "", planningError(ErrorClassInputInvalid, "stage")
	}
	document := promptDocument{
		Stage: definition.Stage, SchemaVersion: definition.SchemaVersion,
		BaseRevision: input.BaseRevision, ProjectContext: cloneProjectContext(input.ProjectContext),
		Input: promptInput{ArtifactID: input.InputArtifactID, Kind: definition.RequiredInputKind},
		OutputContract: promptOutputContract{
			Kind: kind, SchemaVersion: definition.SchemaVersion,
			SourceRevision: input.BaseRevision, InputArtifactIDs: []string{input.InputArtifactID},
			RequiredEnvelopeKeys: []string{"kind", "schema_version", "stage", "summary", "source_revision", "input_artifact_ids", "payload"},
			RequiredPayloadKeys:  payloadKeys(definition.Stage), PathsMustBeRelative: definition.Stage == StagePlan,
			MaxSummaryRunes: MaxSummaryRunes, MaxTextFieldBytes: MaxTextFieldBytes,
			MaxArrayItems: MaxArrayItems, MaxPlanSteps: MaxPlanSteps,
		},
	}
	if definition.Stage == StagePlan {
		document.OutputContract.RequiredPlanStepKeys = []string{"id", "summary", "paths"}
	}
	if definition.RequiredInputKind == ArtifactKindIntent {
		document.Input.Intent = input.Intent
	} else {
		document.Input.Upstream = json.RawMessage(input.Upstream.RawJSON())
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", planningError(ErrorClassInputInvalid, "prompt")
	}
	return promptPreamble + "\n" + string(encoded), nil
}

func payloadKeys(stage Stage) []string {
	switch stage {
	case StageUnderstand:
		return []string{"problem", "goals", "constraints"}
	case StageDesign:
		return []string{"approach", "decisions", "risks"}
	case StagePlan:
		return []string{"steps", "verification"}
	default:
		return nil
	}
}

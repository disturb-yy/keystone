package controlplane

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestVersionPrefixAndIdempotencyKeyHeader(t *testing.T) {
	if VersionPrefix != "/v1" {
		t.Fatalf("VersionPrefix = %q, want %q", VersionPrefix, "/v1")
	}
	if IdempotencyKeyHeader != "Idempotency-Key" {
		t.Fatalf("IdempotencyKeyHeader = %q, want %q", IdempotencyKeyHeader, "Idempotency-Key")
	}
}

func TestHealthResponseJSON(t *testing.T) {
	tests := []struct {
		name  string
		input HealthResponse
		want  string
	}{
		{name: "ready", input: HealthResponse{Ready: true}, want: `{"ready":true}`},
		{name: "not ready", input: HealthResponse{Ready: false}, want: `{"ready":false}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(encoded) != tt.want {
				t.Fatalf("json.Marshal() = %s, want %s", encoded, tt.want)
			}

			var decoded HealthResponse
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if decoded != tt.input {
				t.Fatalf("decoded = %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

func TestDaemonStatusResponseJSON(t *testing.T) {
	tests := []struct {
		name  string
		input DaemonStatusResponse
		want  string
	}{
		{
			name: "ready status",
			input: DaemonStatusResponse{
				DatabasePath:           "/tmp/keystone/keystone.db",
				SchemaMigrationVersion: 1,
				DaemonReadiness:        true,
				DaemonInstanceID:       "daemon-123",
			},
			want: `{"database_path":"/tmp/keystone/keystone.db","schema_migration_version":1,"daemon_readiness":true,"daemon_instance_id":"daemon-123"}`,
		},
		{
			name:  "zero value keeps all fields",
			input: DaemonStatusResponse{},
			want:  `{"database_path":"","schema_migration_version":0,"daemon_readiness":false,"daemon_instance_id":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(encoded) != tt.want {
				t.Fatalf("json.Marshal() = %s, want %s", encoded, tt.want)
			}

			var decoded DaemonStatusResponse
			if err := json.Unmarshal([]byte(tt.want), &decoded); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if decoded != tt.input {
				t.Fatalf("decoded = %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

func TestDaemonStopRequestJSON(t *testing.T) {
	tests := []struct {
		name  string
		input DaemonStopRequest
		want  string
	}{
		{
			name:  "current instance is required",
			input: DaemonStopRequest{DaemonInstanceID: "daemon-123"},
			want:  `{"daemon_instance_id":"daemon-123"}`,
		},
		{
			name:  "zero value keeps required field",
			input: DaemonStopRequest{},
			want:  `{"daemon_instance_id":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(encoded) != tt.want {
				t.Fatalf("json.Marshal() = %s, want %s", encoded, tt.want)
			}

			var decoded DaemonStopRequest
			if err := json.Unmarshal([]byte(tt.want), &decoded); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if decoded != tt.input {
				t.Fatalf("decoded = %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

func TestDaemonStopResponseJSON(t *testing.T) {
	tests := []struct {
		name  string
		input DaemonStopResponse
		want  string
	}{
		{
			name:  "accepted by current instance",
			input: DaemonStopResponse{Accepted: true, DaemonInstanceID: "daemon-123"},
			want:  `{"accepted":true,"daemon_instance_id":"daemon-123"}`,
		},
		{
			name:  "zero value",
			input: DaemonStopResponse{},
			want:  `{"accepted":false,"daemon_instance_id":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(encoded) != tt.want {
				t.Fatalf("json.Marshal() = %s, want %s", encoded, tt.want)
			}

			var decoded DaemonStopResponse
			if err := json.Unmarshal([]byte(tt.want), &decoded); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if decoded != tt.input {
				t.Fatalf("decoded = %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

func TestErrorEnvelopeJSON(t *testing.T) {
	tests := []struct {
		name  string
		input ErrorEnvelope
		want  string
	}{
		{
			name:  "with request id",
			input: ErrorEnvelope{Code: "invalid_request", Message: "request is invalid", RequestID: "req-123"},
			want:  `{"code":"invalid_request","message":"request is invalid","request_id":"req-123"}`,
		},
		{
			name:  "without request id",
			input: ErrorEnvelope{Code: "unavailable", Message: "service is unavailable"},
			want:  `{"code":"unavailable","message":"service is unavailable"}`,
		},
		{
			name:  "required fields are not omitted",
			input: ErrorEnvelope{},
			want:  `{"code":"","message":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(encoded) != tt.want {
				t.Fatalf("json.Marshal() = %s, want %s", encoded, tt.want)
			}

			var decoded ErrorEnvelope
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if decoded != tt.input {
				t.Fatalf("decoded = %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

func TestErrorEnvelopeJSONHasOnlyContractFields(t *testing.T) {
	encoded, err := json.Marshal(ErrorEnvelope{
		Code:      "failed",
		Message:   "safe message",
		RequestID: "req-456",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	wantFields := map[string]json.RawMessage{
		"code":       json.RawMessage(`"failed"`),
		"message":    json.RawMessage(`"safe message"`),
		"request_id": json.RawMessage(`"req-456"`),
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("fields = %v, want %v", fields, wantFields)
	}
}

func TestArtifactRefDTOJSON(t *testing.T) {
	tests := []struct {
		name  string
		input ArtifactRefDTO
		want  string
	}{
		{
			name: "legacy metadata is omitted",
			input: ArtifactRefDTO{
				ArtifactRefID: "0199daba-7777-7000-8000-000000000001",
				ArtifactID:    "0199daba-7777-7000-8000-000000000002",
				Role:          "input",
				Ordinal:       0,
			},
			want: `{"artifact_ref_id":"0199daba-7777-7000-8000-000000000001","artifact_id":"0199daba-7777-7000-8000-000000000002","role":"input","ordinal":0}`,
		},
		{
			name: "planning metadata is exposed",
			input: ArtifactRefDTO{
				ArtifactRefID:        "0199daba-7777-7000-8000-000000000003",
				ArtifactID:           "0199daba-7777-7000-8000-000000000004",
				Role:                 "output",
				Ordinal:              1,
				Kind:                 "understanding",
				SchemaVersion:        "Understanding.v1",
				Summary:              "已确认目标与约束",
				SourceRevision:       "0123456789abcdef0123456789abcdef01234567",
				InputArtifactRefIDs:  []string{"0199daba-7777-7000-8000-000000000005"},
				RawLogArtifactRefIDs: []string{"0199daba-7777-7000-8000-000000000006"},
			},
			want: `{"artifact_ref_id":"0199daba-7777-7000-8000-000000000003","artifact_id":"0199daba-7777-7000-8000-000000000004","role":"output","ordinal":1,"kind":"understanding","schema_version":"Understanding.v1","summary":"已确认目标与约束","source_revision":"0123456789abcdef0123456789abcdef01234567","input_artifact_ref_ids":["0199daba-7777-7000-8000-000000000005"],"raw_log_artifact_ref_ids":["0199daba-7777-7000-8000-000000000006"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.want)
		})
	}
}

func TestTicketGraphReadModelJSONHidesGenerationKeyAndExecutionFields(t *testing.T) {
	input := TicketGraphReadModel{
		GraphID: "0199daba-7777-7000-8000-000000000010", ChangeID: "0199daba-7777-7000-8000-000000000011", ProjectID: "0199daba-7777-7000-8000-000000000012",
		BaseRevision: "0123456789abcdef0123456789abcdef01234567", TicketizeAgentRunID: "0199daba-7777-7000-8000-000000000013", GeneratorName: "codex", GeneratorVersion: "ticketize.v1", CreatedAt: "2026-09-08T00:00:00Z",
		Tickets: []CanonicalTicketDTO{{TicketID: "0199daba-7777-7000-8000-000000000014", Ordinal: 1, Title: "Ticket", Scope: "scope", AcceptanceCriteria: []string{"done"}}},
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"generation_key", "execution_state", "lease_token", "workspace_path"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("TicketGraphReadModel contains forbidden field %q: %s", forbidden, text)
		}
	}
	var decoded TicketGraphReadModel
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GraphID != input.GraphID || len(decoded.Tickets) != 1 || decoded.Tickets[0].AcceptanceCriteria[0] != "done" {
		t.Fatalf("decoded graph = %+v", decoded)
	}
}

func TestAgentRunDTOJSON(t *testing.T) {
	tests := []struct {
		name  string
		input AgentRunDTO
		want  string
	}{
		{
			name: "legacy metadata is omitted",
			input: AgentRunDTO{
				AgentRunID: "0199daba-7777-7000-8000-000000000007",
				ChangeID:   "0199daba-7777-7000-8000-000000000008",
				Stage:      "Understand",
				Attempt:    1,
				Status:     "running",
				Artifacts:  []AgentRunArtifactDTO{},
				StartedAt:  "2026-09-08T08:00:00Z",
			},
			want: `{"agent_run_id":"0199daba-7777-7000-8000-000000000007","change_id":"0199daba-7777-7000-8000-000000000008","stage":"Understand","attempt":1,"status":"running","outcome":"","artifacts":[],"started_at":"2026-09-08T08:00:00Z","completed_at":null}`,
		},
		{
			name: "planning metadata is exposed",
			input: AgentRunDTO{
				AgentRunID:     "0199daba-7777-7000-8000-000000000009",
				ChangeID:       "0199daba-7777-7000-8000-00000000000a",
				Stage:          "Plan",
				Attempt:        2,
				RunKind:        "planning",
				SourceRevision: "0123456789abcdef0123456789abcdef01234567",
				Status:         "running",
				Artifacts:      []AgentRunArtifactDTO{},
				StartedAt:      "2026-09-08T08:01:00Z",
			},
			want: `{"agent_run_id":"0199daba-7777-7000-8000-000000000009","change_id":"0199daba-7777-7000-8000-00000000000a","stage":"Plan","attempt":2,"run_kind":"planning","source_revision":"0123456789abcdef0123456789abcdef01234567","status":"running","outcome":"","artifacts":[],"started_at":"2026-09-08T08:01:00Z","completed_at":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.want)
		})
	}
}

func assertJSONRoundTrip[T any](t *testing.T, input T, want string) {
	t.Helper()

	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(encoded) != want {
		t.Fatalf("json.Marshal() = %s, want %s", encoded, want)
	}

	var decoded T
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, input) {
		t.Fatalf("decoded = %+v, want %+v", decoded, input)
	}
}

func TestParseIdempotencyKey(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      IdempotencyKey
		wantError bool
	}{
		{
			name:  "opaque value is preserved",
			value: "request/2026-09-04 opaque-token",
			want:  IdempotencyKey("request/2026-09-04 opaque-token"),
		},
		{name: "empty value", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIdempotencyKey(tt.value)
			if tt.wantError {
				if err == nil {
					t.Fatal("ParseIdempotencyKey() error = nil, want error")
				}
				if !errors.Is(err, ErrEmptyIdempotencyKey) {
					t.Fatalf("ParseIdempotencyKey() error = %v, want ErrEmptyIdempotencyKey", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseIdempotencyKey() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseIdempotencyKey(%q) = %q, want %q", tt.value, got, tt.want)
			}
			if got.String() != tt.value {
				t.Fatalf("IdempotencyKey.String() = %q, want %q", got.String(), tt.value)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("IdempotencyKey.Validate() error = %v", err)
			}
		})
	}
}

func TestIdempotencyKeyValidate(t *testing.T) {
	if err := IdempotencyKey("").Validate(); !errors.Is(err, ErrEmptyIdempotencyKey) {
		t.Fatalf("empty key validation error = %v, want ErrEmptyIdempotencyKey", err)
	}
}

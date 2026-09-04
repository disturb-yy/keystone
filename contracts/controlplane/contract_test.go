package controlplane

import (
	"encoding/json"
	"errors"
	"reflect"
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

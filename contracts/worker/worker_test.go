package worker

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestJSONMarshalIncludesRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "register",
			value: Register{
				WorkerID:        "worker-1",
				ProtocolVersion: "v1",
				Capabilities:    []string{"runtime:codex"},
			},
			want: `{"worker_id":"worker-1","protocol_version":"v1","capabilities":["runtime:codex"]}`,
		},
		{
			name:  "heartbeat",
			value: Heartbeat{WorkerID: "worker-1"},
			want:  `{"worker_id":"worker-1"}`,
		},
		{
			name: "assignment",
			value: Assignment{
				AgentRunID:    "run-1",
				LeaseToken:    "lease-opaque",
				WorkspacePath: "/tmp/keystone-workspace",
				Runtime:       "codex",
			},
			want: `{"agent_run_id":"run-1","lease_token":"lease-opaque","workspace_path":"/tmp/keystone-workspace","runtime":"codex"}`,
		},
		{
			name: "report",
			value: Report{
				AgentRunID: "run-1",
				LeaseToken: "lease-opaque",
				Outcome:    Outcome("runtime_error"),
			},
			want: `{"agent_run_id":"run-1","lease_token":"lease-opaque","outcome":"runtime_error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("json.Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestJSONUnmarshalMinimumPayloads(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		decode func([]byte) (any, error)
		want   any
	}{
		{
			name:  "register",
			input: `{"worker_id":"worker-1","protocol_version":"v1","capabilities":[]}`,
			decode: func(input []byte) (any, error) {
				var value Register
				err := json.Unmarshal(input, &value)
				return value, err
			},
			want: Register{
				WorkerID:        "worker-1",
				ProtocolVersion: "v1",
				Capabilities:    []string{},
			},
		},
		{
			name:  "heartbeat",
			input: `{"worker_id":"worker-1"}`,
			decode: func(input []byte) (any, error) {
				var value Heartbeat
				err := json.Unmarshal(input, &value)
				return value, err
			},
			want: Heartbeat{WorkerID: "worker-1"},
		},
		{
			name:  "assignment",
			input: `{"agent_run_id":"run-1","lease_token":"lease-opaque","workspace_path":"/tmp/keystone-workspace","runtime":"codex"}`,
			decode: func(input []byte) (any, error) {
				var value Assignment
				err := json.Unmarshal(input, &value)
				return value, err
			},
			want: Assignment{
				AgentRunID:    "run-1",
				LeaseToken:    "lease-opaque",
				WorkspacePath: "/tmp/keystone-workspace",
				Runtime:       "codex",
			},
		},
		{
			name:  "report accepts extensible outcome",
			input: `{"agent_run_id":"run-1","lease_token":"lease-opaque","outcome":"future_runtime_result"}`,
			decode: func(input []byte) (any, error) {
				var value Report
				err := json.Unmarshal(input, &value)
				return value, err
			},
			want: Report{
				AgentRunID: "run-1",
				LeaseToken: "lease-opaque",
				Outcome:    Outcome("future_runtime_result"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.decode([]byte(tt.input))
			if err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("json.Unmarshal() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

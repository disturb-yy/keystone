package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewWritesStructuredJSON(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, slog.LevelInfo)

	logger.Info("started", "component", "foundation", "attempt", 2)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("parse logger output: %v", err)
	}
	if got := record["msg"]; got != "started" {
		t.Fatalf("msg = %v, want %q", got, "started")
	}
	if got := record["level"]; got != "INFO" {
		t.Fatalf("level = %v, want %q", got, "INFO")
	}
	if got := record["component"]; got != "foundation" {
		t.Fatalf("component = %v, want %q", got, "foundation")
	}
	if got := record["attempt"]; got != float64(2) {
		t.Fatalf("attempt = %v, want %v", got, 2)
	}
}

func TestNewFiltersBelowConfiguredLevel(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		write func(*slog.Logger)
		want  string
	}{
		{
			name:  "debug is filtered at info",
			level: slog.LevelInfo,
			write: func(logger *slog.Logger) { logger.Debug("hidden") },
		},
		{
			name:  "error is retained at info",
			level: slog.LevelInfo,
			write: func(logger *slog.Logger) { logger.Error("visible") },
			want:  "visible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := New(&output, tt.level)
			tt.write(logger)

			if tt.want == "" {
				if output.Len() != 0 {
					t.Fatalf("output = %q, want empty", output.String())
				}
				return
			}

			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatalf("parse logger output: %v", err)
			}
			if got := record["msg"]; got != tt.want {
				t.Fatalf("msg = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestNewDoesNotChangeDefaultLogger(t *testing.T) {
	defaultBefore := slog.Default()
	var output bytes.Buffer

	_ = New(&output, slog.LevelDebug)

	if defaultAfter := slog.Default(); defaultAfter != defaultBefore {
		t.Fatalf("slog.Default() changed from %p to %p", defaultBefore, defaultAfter)
	}
}

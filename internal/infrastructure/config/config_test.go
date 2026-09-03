package config

import (
	"errors"
	"log/slog"
	"os"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      slog.Level
		wantError bool
	}{
		{name: "debug", value: "debug", want: slog.LevelDebug},
		{name: "debug standard representation", value: "DEBUG", want: slog.LevelDebug},
		{name: "info mixed case", value: "iNfO", want: slog.LevelInfo},
		{name: "info standard representation", value: "INFO", want: slog.LevelInfo},
		{name: "warn", value: "warn", want: slog.LevelWarn},
		{name: "warn standard representation", value: "WARN", want: slog.LevelWarn},
		{name: "error", value: "error", want: slog.LevelError},
		{name: "error standard representation", value: "ERROR", want: slog.LevelError},
		{name: "standard offset representation", value: "INFO+2", want: slog.LevelInfo + 2},
		{name: "unknown", value: "verbose", wantError: true},
		{name: "empty", value: "", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLevel(tt.value)
			if tt.wantError {
				if err == nil {
					t.Fatal("ParseLevel() error = nil, want error")
				}
				if !errors.Is(err, ErrInvalidLogLevel) {
					t.Fatalf("ParseLevel() error = %v, want ErrInvalidLogLevel", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseLevel() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseLevel(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		value     *string
		want      slog.Level
		wantError bool
	}{
		{name: "unset defaults to info", want: slog.LevelInfo},
		{name: "configured level", value: stringPointer("DEBUG"), want: slog.LevelDebug},
		{name: "invalid level", value: stringPointer("verbose"), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setLogLevelEnv(t, tt.value)

			got, err := Load()
			if tt.wantError {
				if err == nil {
					t.Fatal("Load() error = nil, want error")
				}
				if !errors.Is(err, ErrInvalidLogLevel) {
					t.Fatalf("Load() error = %v, want ErrInvalidLogLevel", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error = %v", err)
			}
			if got.LogLevel != tt.want {
				t.Fatalf("Load().LogLevel = %v, want %v", got.LogLevel, tt.want)
			}
		})
	}
}

func setLogLevelEnv(t *testing.T, value *string) {
	t.Helper()

	previous, wasSet := os.LookupEnv(LogLevelEnv)
	t.Cleanup(func() {
		if wasSet {
			if err := os.Setenv(LogLevelEnv, previous); err != nil {
				t.Errorf("restore %s: %v", LogLevelEnv, err)
			}
			return
		}
		if err := os.Unsetenv(LogLevelEnv); err != nil {
			t.Errorf("unset %s: %v", LogLevelEnv, err)
		}
	})

	if value == nil {
		if err := os.Unsetenv(LogLevelEnv); err != nil {
			t.Fatalf("unset %s: %v", LogLevelEnv, err)
		}
		return
	}
	if err := os.Setenv(LogLevelEnv, *value); err != nil {
		t.Fatalf("set %s: %v", LogLevelEnv, err)
	}
}

func stringPointer(value string) *string {
	return &value
}

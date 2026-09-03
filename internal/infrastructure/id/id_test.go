package id

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestNewUUIDv4(t *testing.T) {
	value, err := New()
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	if len(value) != 36 {
		t.Fatalf("UUID length = %d, want 36", len(value))
	}
	if value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		t.Fatalf("UUID separators are invalid: %q", value)
	}

	bytes, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	if err != nil {
		t.Fatalf("decode UUID %q: %v", value, err)
	}
	if len(bytes) != 16 {
		t.Fatalf("decoded UUID length = %d, want 16", len(bytes))
	}
	if version := bytes[6] >> 4; version != 4 {
		t.Fatalf("UUID version = %d, want 4", version)
	}
	if variant := bytes[8] & 0xc0; variant != 0x80 {
		t.Fatalf("UUID variant bits = %#x, want %#x", variant, 0x80)
	}
	for _, character := range value {
		if character >= 'A' && character <= 'F' {
			t.Fatalf("UUID contains uppercase hexadecimal character: %q", value)
		}
	}
}

func TestNewReturnsDifferentValues(t *testing.T) {
	const count = 32
	values := make(map[string]struct{}, count)

	for index := 0; index < count; index++ {
		value, err := New()
		if err != nil {
			t.Fatalf("New() call %d unexpected error = %v", index, err)
		}
		if _, exists := values[value]; exists {
			t.Fatalf("New() returned duplicate UUID %q on call %d", value, index)
		}
		values[value] = struct{}{}
	}
}

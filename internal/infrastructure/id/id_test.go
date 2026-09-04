package id

import (
	"strings"
	"testing"
	"uuid"
)

func TestNewUUIDv7(t *testing.T) {
	value := New()
	if len(value) != 36 {
		t.Fatalf("UUID length = %d, want 36", len(value))
	}
	if value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		t.Fatalf("UUID separators are invalid: %q", value)
	}

	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("decode UUID %q: %v", value, err)
	}
	if version := parsed[6] >> 4; version != 7 {
		t.Fatalf("UUID version = %d, want 7", version)
	}
	if variant := parsed[8] & 0xc0; variant != 0x80 {
		t.Fatalf("UUID variant bits = %#x, want %#x", variant, 0x80)
	}
	if value != strings.ToLower(value) {
		t.Fatalf("UUID is not lowercase: %q", value)
	}
}

func TestNewReturnsIncreasingUniqueValues(t *testing.T) {
	const count = 32
	values := make(map[string]struct{}, count)
	previous := ""

	for index := 0; index < count; index++ {
		value := New()
		if previous != "" && value <= previous {
			t.Fatalf("New() value %q is not greater than previous value %q", value, previous)
		}
		if _, exists := values[value]; exists {
			t.Fatalf("New() returned duplicate UUID %q on call %d", value, index)
		}
		values[value] = struct{}{}
		previous = value
	}
}

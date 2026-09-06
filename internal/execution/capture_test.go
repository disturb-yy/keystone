package execution

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestBoundedBufferMarksTruncationAndKeepsPrefix(t *testing.T) {
	buffer := newBoundedBuffer(4)
	if _, err := buffer.Write([]byte("abcdef")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := buffer.String(); got != "abcd" {
		t.Fatalf("buffer = %q, want %q", got, "abcd")
	}
	if !buffer.truncated {
		t.Fatal("buffer.truncated = false, want true")
	}
	artifact := buffer.artifact(ArtifactStdout)
	if err := artifact.Validate(); err != nil {
		t.Fatalf("artifact.Validate() error = %v", err)
	}
}

func TestNormalizeChangedFilesSortsDeduplicatesAndRejectsEscape(t *testing.T) {
	got, err := NormalizeChangedFiles([]string{"b.go", "./a.go", "b.go"})
	if err != nil {
		t.Fatalf("NormalizeChangedFiles() error = %v", err)
	}
	if want := []string{"a.go", "b.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeChangedFiles() = %#v, want %#v", got, want)
	}
	if _, err := NormalizeChangedFiles([]string{"../outside.txt"}); err == nil {
		t.Fatal("NormalizeChangedFiles() error = nil, want path escape error")
	}
}

func TestSanitizeEnvironmentRemovesProtocolAndDatabaseSecrets(t *testing.T) {
	got := SanitizeEnvironment([]string{
		"PATH=/bin",
		"HOME=/home/user",
		"WORKER_PROTOCOL_SECRET=secret",
		"LEASE_TOKEN=token",
		"KEYSTONE_DB_DSN=file.db",
		"LANG=C",
	})
	joined := strings.Join(got, "\n")
	for _, forbidden := range []string{"WORKER_PROTOCOL_SECRET", "LEASE_TOKEN", "KEYSTONE_DB_DSN"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("environment contains forbidden key %q: %q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "LANG=C") {
		t.Fatalf("environment lost safe values: %q", joined)
	}
	_ = context.Background()
}

package execution

import (
	"context"
	"reflect"
	"strings"
	"testing"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
)

func TestPlanningCandidateKeepsPriorityWithinTotalLimit(t *testing.T) {
	result := RuntimeResult{
		Stdout:    NewArtifact(ArtifactStdout, []byte("12345678"), false, nil),
		Candidate: NewArtifact(ArtifactCandidate, []byte("json"), false, nil),
	}
	EnforceTotalLimit(&result, Limits{TotalBytes: 6})
	if got := string(result.Candidate.Content); got != "json" || result.Candidate.Truncated {
		t.Fatalf("candidate = %q, truncated=%t", got, result.Candidate.Truncated)
	}
	if got := string(result.Stdout.Content); got != "12" || !result.Stdout.Truncated {
		t.Fatalf("stdout = %q, truncated=%t", got, result.Stdout.Truncated)
	}
}

func TestDefaultTotalLimitFitsWorkerProtocolWireBudget(t *testing.T) {
	raw := DefaultLimits().TotalBytes
	base64UpperBound := ((raw + 2) / 3) * 4
	const metadataAllowance = int64(1 << 20)
	if base64UpperBound+metadataAllowance >= int64(workercontract.MaxBodyBytes) {
		t.Fatalf("wire upper bound %d does not fit protocol body %d", base64UpperBound+metadataAllowance, workercontract.MaxBodyBytes)
	}
}

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
	for _, file := range []string{"../outside.txt", "..", "./..", "a/..", "C:/outside.txt", "a//b", "a/"} {
		if _, err := NormalizeChangedFiles([]string{file}); err == nil {
			t.Fatalf("NormalizeChangedFiles(%q) error = nil, want path validation error", file)
		}
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

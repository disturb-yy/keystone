package artifact_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/disturb-yy/keystone/internal/infrastructure/artifact"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestStorePutReadAndReuse(t *testing.T) {
	store, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("intent\n")
	identity, err := store.Put(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := store.Put(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	if reused != identity {
		t.Fatalf("reused identity = %+v, want %+v", reused, identity)
	}
	got, err := store.Read(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("content = %q, want %q", got, content)
	}
}

func TestStoreReadRejectsMissingAndCorruptContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	store, err := artifact.New(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := store.Put(context.Background(), []byte("valid"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, identity.SHA256), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), identity); !errors.Is(err, domain.ErrArtifactUnavailable) {
		t.Fatalf("corrupt read error = %v, want ErrArtifactUnavailable", err)
	}
	if _, err := store.Read(context.Background(), domain.ArtifactIdentity{SHA256: identity.SHA256, ByteLength: 99}); !errors.Is(err, domain.ErrArtifactUnavailable) {
		t.Fatalf("missing identity read error = %v, want ErrArtifactUnavailable", err)
	}
}

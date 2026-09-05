package manifest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestEnsureCreatesAndRepeatsStrictManifest(t *testing.T) {
	root := t.TempDir()
	binding := domain.RepositoryBinding{Root: root, ManifestPath: filepath.Join(root, ".keystone", "project.yaml")}
	id := domain.NewProjectID()
	store := Store{}
	got, err := store.Ensure(context.Background(), binding, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != id {
		t.Fatalf("project id = %q, want %q", got.ProjectID, id)
	}
	wantBytes := []byte("version: 1\nproject_id: " + string(id) + "\n")
	data, err := os.ReadFile(binding.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(wantBytes) {
		t.Fatalf("manifest = %q, want %q", data, wantBytes)
	}
	if _, err := store.Ensure(context.Background(), binding, domain.NewProjectID()); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRejectsInvalidManifestWithoutChangingBytes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".keystone")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "project.yaml")
	original := []byte("version: 1\nunknown: value\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	binding := domain.RepositoryBinding{Root: root, ManifestPath: path}
	_, err := (Store{}).Ensure(context.Background(), binding, domain.NewProjectID())
	if !errors.Is(err, domain.ErrManifestInvalid) {
		t.Fatalf("Ensure() error = %v, want ErrManifestInvalid", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(original) {
		t.Fatalf("manifest bytes changed: %q", data)
	}
}

func TestEnsureRejectsManifestSymlink(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".keystone")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside.yaml")
	if err := os.WriteFile(target, []byte("version: 1\nproject_id: "+string(domain.NewProjectID())+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "project.yaml")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	binding := domain.RepositoryBinding{Root: root, ManifestPath: path}
	_, err := (Store{}).Ensure(context.Background(), binding, domain.NewProjectID())
	if !errors.Is(err, domain.ErrManifestInvalid) {
		t.Fatalf("Ensure() error = %v, want ErrManifestInvalid", err)
	}
}

func TestEnsureAcceptsCRLFManifestWithoutRewritingIt(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".keystone")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	id := domain.NewProjectID()
	path := filepath.Join(dir, "project.yaml")
	original := []byte("version: 1\r\nproject_id: " + string(id) + "\r\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	binding := domain.RepositoryBinding{Root: root, ManifestPath: path}
	manifest, err := (Store{}).Ensure(context.Background(), binding, domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ProjectID != id {
		t.Fatalf("project id = %q, want %q", manifest.ProjectID, id)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("manifest bytes changed from CRLF: %q", data)
	}
}

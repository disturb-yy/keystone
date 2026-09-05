package manifest

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestEnsureRejectsNonRFCVariantUUIDv7Manifest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".keystone", "project.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("version: 1\nproject_id: 0191a6c0-0000-7000-c000-000000000000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{}).Ensure(context.Background(), domain.RepositoryBinding{Root: root, ManifestPath: path}, domain.NewProjectID())
	if !errors.Is(err, domain.ErrManifestInvalid) {
		t.Fatalf("Ensure() error = %v, want ErrManifestInvalid", err)
	}
}

func TestEnsureCleansIncompleteCreatedFileAndRetryCanRecover(t *testing.T) {
	root := t.TempDir()
	binding := domain.RepositoryBinding{Root: root, ManifestPath: filepath.Join(root, ".keystone", "project.yaml")}
	if err := os.MkdirAll(filepath.Dir(binding.ManifestPath), 0755); err != nil {
		t.Fatal(err)
	}
	store := Store{openFile: func(path string, flags int, mode os.FileMode) (manifestFile, error) {
		file, err := os.OpenFile(path, flags, mode)
		if err != nil {
			return nil, err
		}
		return shortWriteFile{file: file}, nil
	}}
	_, err := store.Ensure(context.Background(), binding, domain.NewProjectID())
	if !errors.Is(err, domain.ErrManifestUnavailable) {
		t.Fatalf("Ensure() error = %v, want ErrManifestUnavailable", err)
	}
	if _, statErr := os.Stat(binding.ManifestPath); !os.IsNotExist(statErr) {
		t.Fatalf("incomplete manifest stat error = %v, want not exist", statErr)
	}
	if _, err := (Store{}).Ensure(context.Background(), binding, domain.NewProjectID()); err != nil {
		t.Fatalf("retry Ensure() error = %v", err)
	}
}

func TestEnsureConcurrentCreationPublishesCompleteManifest(t *testing.T) {
	root := t.TempDir()
	binding := domain.RepositoryBinding{Root: root, ManifestPath: filepath.Join(root, ".keystone", "project.yaml")}
	started := make(chan struct{})
	release := make(chan struct{})
	var blockOnce sync.Once
	var releaseOnce sync.Once
	openFile := func(path string, flags int, mode os.FileMode) (manifestFile, error) {
		file, err := os.OpenFile(path, flags, mode)
		if err != nil {
			return nil, err
		}
		block := false
		blockOnce.Do(func() { block = true })
		return &blockingWriteFile{file: file, block: block, started: started, release: release}, nil
	}
	store := Store{openFile: openFile}
	first := make(chan error, 1)
	go func() {
		_, err := store.Ensure(context.Background(), binding, domain.NewProjectID())
		first <- err
	}()
	<-started
	second := make(chan error, 1)
	go func() {
		_, err := store.Ensure(context.Background(), binding, domain.NewProjectID())
		second <- err
	}()
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("concurrent Ensure() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent Ensure() did not publish a complete manifest")
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-first; err != nil {
		t.Fatalf("first Ensure() error = %v", err)
	}
	data, err := os.ReadFile(binding.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parse(data); err != nil {
		t.Fatalf("published manifest parse error = %v", err)
	}
}

type shortWriteFile struct {
	file *os.File
}

func (f shortWriteFile) Write([]byte) (int, error) { return 0, io.ErrShortWrite }
func (f shortWriteFile) Sync() error               { return f.file.Sync() }
func (f shortWriteFile) Close() error              { return f.file.Close() }

type blockingWriteFile struct {
	file    *os.File
	block   bool
	started chan struct{}
	release chan struct{}
}

func (f *blockingWriteFile) Write(data []byte) (int, error) {
	if f.block {
		close(f.started)
		<-f.release
	}
	return f.file.Write(data)
}

func (f *blockingWriteFile) Sync() error  { return f.file.Sync() }
func (f *blockingWriteFile) Close() error { return f.file.Close() }

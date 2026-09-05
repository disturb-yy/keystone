package localstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveIsDeterministicAndHasNoSideEffects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := filepath.Join(home, ".keystone"); p.Root != want {
		t.Fatalf("Root = %q, want %q", p.Root, want)
	}
	if _, err := os.Stat(p.Root); !os.IsNotExist(err) {
		t.Fatalf("Resolve created root, stat error = %v", err)
	}
	wantPaths := Paths{
		Root:          filepath.Join(home, ".keystone"),
		StateDir:      filepath.Join(home, ".keystone", "state"),
		DatabasePath:  filepath.Join(home, ".keystone", "state", "keystone.db"),
		ArtifactsDir:  filepath.Join(home, ".keystone", "artifacts"),
		WorkspacesDir: filepath.Join(home, ".keystone", "workspaces"),
		RuntimeDir:    filepath.Join(home, ".keystone", "runtime"),
		LockPath:      filepath.Join(home, ".keystone", "runtime", "keystone.lock"),
		MetadataPath:  filepath.Join(home, ".keystone", "runtime", "instance.json"),
	}
	if p != wantPaths {
		t.Fatalf("Resolve() = %+v, want %+v", p, wantPaths)
	}

	relative, err := Resolve(filepath.Join("test-data", "..", "state"))
	if err != nil {
		t.Fatalf("Resolve(relative) error = %v", err)
	}
	want, err := filepath.Abs("state")
	if err != nil {
		t.Fatal(err)
	}
	if relative.Root != want {
		t.Fatalf("relative Root = %q, want %q", relative.Root, want)
	}
}

func TestInitializeCreatesOwnerOnlyDirectories(t *testing.T) {
	if !supportsOwnerOnlyModes(t) {
		t.Skip("temporary filesystem does not expose POSIX owner-only modes")
	}
	p, err := Resolve(t.TempDir() + "/nested/state")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Initialize(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{p.Root, p.StateDir, p.ArtifactsDir, p.WorkspacesDir, p.RuntimeDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0700 {
			t.Errorf("mode(%s) = %o, want 700", dir, got)
		}
	}
}

func TestAcquireIsExclusiveAndReleaseAllowsReacquire(t *testing.T) {
	p, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Initialize(); err != nil {
		t.Fatal(err)
	}
	first, err := Acquire(p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(p)
	if !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("second Acquire() error = %v, want ErrAlreadyLocked", err)
	}
	if second != nil {
		t.Fatal("second Acquire() returned a lock")
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	reacquired, err := Acquire(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := reacquired.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestDifferentRootsCanHoldIndependentLocks(t *testing.T) {
	firstPaths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	secondPaths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := firstPaths.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := secondPaths.Initialize(); err != nil {
		t.Fatal(err)
	}

	firstLock, err := Acquire(firstPaths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := firstLock.Release(); err != nil {
			t.Errorf("release first lock: %v", err)
		}
	})
	secondLock, err := Acquire(secondPaths)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondLock.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestStaleMetadataDoesNotAffectTheAuthoritativeLock(t *testing.T) {
	p, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Initialize(); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Release(); err != nil {
			t.Errorf("release lock: %v", err)
		}
	}()
	if err := PublishMetadata(p, Metadata{PID: 10, Endpoint: "127.0.0.1:10", InstanceID: "active", StartedAt: "2026-09-04T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearMetadata(p, "stale"); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(p)
	if !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("second Acquire() error = %v, want ErrAlreadyLocked", err)
	}
	if second != nil {
		t.Fatal("second Acquire() returned a lock")
	}
}

func TestMetadataPublishesAndClearsOnlyMatchingInstance(t *testing.T) {
	ownerOnlyModes := supportsOwnerOnlyModes(t)
	p, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Initialize(); err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{PID: 42, Endpoint: "127.0.0.1:1", InstanceID: "one", StartedAt: "2026-09-04T00:00:00Z"}
	if err := PublishMetadata(p, metadata); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p.MetadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); ownerOnlyModes && got != 0600 {
		t.Fatalf("metadata mode = %o, want 600", got)
	}
	if err := ClearMetadata(p, "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.MetadataPath); err != nil {
		t.Fatalf("metadata removed for other instance: %v", err)
	}
	if err := ClearMetadata(p, "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.MetadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata remains, stat error = %v", err)
	}
}

func supportsOwnerOnlyModes(t *testing.T) bool {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, "mode-probe-dir")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatalf("create mode probe directory: %v", err)
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("stat mode probe directory: %v", err)
	}
	file, err := os.OpenFile(filepath.Join(root, "mode-probe-file"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("create mode probe file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close mode probe file: %v", err)
	}
	fileInfo, err := os.Stat(filepath.Join(root, "mode-probe-file"))
	if err != nil {
		t.Fatalf("stat mode probe file: %v", err)
	}
	return directoryInfo.Mode().Perm() == 0700 && fileInfo.Mode().Perm() == 0600
}

func TestMetadataReplacementPublishesTheLatestCompleteRecord(t *testing.T) {
	p, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Initialize(); err != nil {
		t.Fatal(err)
	}

	first := Metadata{PID: 1, Endpoint: "127.0.0.1:1", InstanceID: "first", StartedAt: "2026-09-04T00:00:00Z"}
	second := Metadata{PID: 2, Endpoint: "127.0.0.1:2", InstanceID: "second", StartedAt: "2026-09-04T00:01:00Z"}
	if err := PublishMetadata(p, first); err != nil {
		t.Fatal(err)
	}
	if err := PublishMetadata(p, second); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(p.MetadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var got Metadata
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != second {
		t.Fatalf("metadata = %+v, want %+v", got, second)
	}
}

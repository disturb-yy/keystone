package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestMaterializeSnapshotPinsRevisionAndLeavesSourceUnchanged(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "--initial-branch=source")
	writeSnapshotFixture(t, filepath.Join(root, "tracked.txt"), "first revision")
	runGit(t, root, "add", "tracked.txt")
	commitSnapshotFixture(t, root, "first")
	baseRevision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")

	writeSnapshotFixture(t, filepath.Join(root, "tracked.txt"), "second revision")
	writeSnapshotFixture(t, filepath.Join(root, "future.txt"), "future-only")
	runGit(t, root, "add", "tracked.txt")
	runGit(t, root, "add", "future.txt")
	commitSnapshotFixture(t, root, "second")
	futureRevision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	writeSnapshotFixture(t, filepath.Join(root, "tracked.txt"), "dirty source")
	writeSnapshotFixture(t, filepath.Join(root, "untracked.txt"), "source-only")

	sourceHead := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	sourceStatus := gitValue(t, root, "status", "--porcelain=v1", "--untracked-files=all")
	snapshot := materializeSnapshotFixture(t, root, baseRevision)

	if snapshot.Revision() != baseRevision {
		t.Fatalf("Revision() = %q, want fixed base revision", snapshot.Revision())
	}
	if snapshot.Root() == root || !filepath.IsAbs(snapshot.Root()) {
		t.Fatalf("Root() did not return an isolated absolute path")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Dir(snapshot.Root()))
		if err != nil {
			t.Fatal(err)
		}
		if permission := info.Mode().Perm(); permission != 0700 {
			t.Fatalf("temporary root permission = %04o, want 0700", permission)
		}
	}
	if head := gitValue(t, snapshot.Root(), "rev-parse", "--verify", "HEAD^{commit}"); head != baseRevision {
		t.Fatalf("snapshot HEAD = %q, want %q", head, baseRevision)
	}
	if status := gitValue(t, snapshot.Root(), "status", "--porcelain=v1", "--untracked-files=all"); status != "" {
		t.Fatalf("snapshot status = %q, want clean", status)
	}
	trackedPath, err := snapshot.Resolve("tracked.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content := readSnapshotFixture(t, trackedPath); content != "first revision" {
		t.Fatalf("snapshot content = %q, want historical content", content)
	}
	if _, err := os.Stat(filepath.Join(snapshot.Root(), "untracked.txt")); !os.IsNotExist(err) {
		t.Fatalf("source untracked file appeared in snapshot: %v", err)
	}
	if command := exec.Command("git", "-C", snapshot.Root(), "cat-file", "-e", futureRevision+"^{commit}"); command.Run() == nil {
		t.Fatal("snapshot object database retained a post-base commit")
	}
	if output := gitValue(t, snapshot.Root(), "for-each-ref", "--format=%(refname)"); output != "" {
		t.Fatalf("snapshot retained source refs: %q", output)
	}
	if snapshotTreeContains(t, filepath.Join(snapshot.Root(), ".git"), root) {
		t.Fatal("snapshot Git metadata retained the source absolute path")
	}

	if head := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}"); head != sourceHead {
		t.Fatalf("source HEAD changed from %q to %q", sourceHead, head)
	}
	if status := gitValue(t, root, "status", "--porcelain=v1", "--untracked-files=all"); status != sourceStatus {
		t.Fatalf("source status changed from %q to %q", sourceStatus, status)
	}
	if content := readSnapshotFixture(t, filepath.Join(root, "tracked.txt")); content != "dirty source" {
		t.Fatalf("source content = %q, want unchanged dirty content", content)
	}
}

func TestMaterializeSnapshotSameRevisionIsConsistent(t *testing.T) {
	root := committedRepository(t)
	baseRevision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	first := materializeSnapshotFixture(t, root, baseRevision)
	second := materializeSnapshotFixture(t, root, baseRevision)

	firstPath, err := first.Resolve("tracked.txt")
	if err != nil {
		t.Fatal(err)
	}
	secondPath, err := second.Resolve("tracked.txt")
	if err != nil {
		t.Fatal(err)
	}
	if first.Root() == second.Root() {
		t.Fatal("independent snapshots share a temporary root")
	}
	if got, want := readSnapshotFixture(t, firstPath), readSnapshotFixture(t, secondPath); got != want {
		t.Fatalf("same revision produced different content: %q != %q", got, want)
	}
}

func TestIsolatedSnapshotResolveRejectsPathEscape(t *testing.T) {
	root := committedRepository(t)
	baseRevision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	snapshot := materializeSnapshotFixture(t, root, baseRevision)

	invalidPaths := []string{
		"",
		".",
		"../outside.txt",
		"nested/../tracked.txt",
		"nested\\tracked.txt",
		filepath.Join(snapshot.Root(), "tracked.txt"),
	}
	for _, value := range invalidPaths {
		if _, err := snapshot.Resolve(value); !errors.Is(err, domain.ErrInvalidRequest) {
			t.Fatalf("Resolve(%q) error = %v, want ErrInvalidRequest", value, err)
		}
	}
	if _, err := snapshot.Resolve("missing.txt"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Resolve(missing) error = %v, want ErrUnavailable", err)
	}
}

func TestMaterializeSnapshotRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires environment-specific privilege")
	}
	root := committedRepository(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeSnapshotFixture(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "escape")
	commitSnapshotFixture(t, root, "add escaping symlink")
	baseRevision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")

	_, err := (Git{}).MaterializeSnapshot(context.Background(), root, baseRevision)
	if !errors.Is(err, domain.ErrRepositoryUnsupported) {
		t.Fatalf("MaterializeSnapshot() error = %v, want ErrRepositoryUnsupported", err)
	}
	assertSnapshotErrorSanitized(t, err, root, outside)
}

func TestConfiguredSnapshotBaseAndReset(t *testing.T) {
	root := committedRepository(t)
	base := filepath.Join(t.TempDir(), "planning-snapshots")
	git := Git{SnapshotBase: base}
	if err := git.ResetSnapshots(); err != nil {
		t.Fatal(err)
	}
	revision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	snapshot, err := git.MaterializeSnapshot(context.Background(), root, revision)
	if err != nil {
		t.Fatal(err)
	}
	if !withinSnapshotRoot(base, snapshot.Root()) {
		t.Fatalf("snapshot root escaped configured base")
	}
	sentinel := filepath.Join(base, "keep")
	writeSnapshotFixture(t, sentinel, "keep")
	if err := git.ResetSnapshots(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(snapshot.Root())); !os.IsNotExist(err) {
		t.Fatalf("ResetSnapshots() retained Keystone snapshot: %v", err)
	}
	if content := readSnapshotFixture(t, sentinel); content != "keep" {
		t.Fatalf("ResetSnapshots() changed unrelated entry = %q", content)
	}
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializeSnapshotNeverUsesSourceRepositoryAsTemporaryBase(t *testing.T) {
	root := committedRepository(t)
	temporaryBase := filepath.Join(root, "tmp")
	if err := os.Mkdir(temporaryBase, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", temporaryBase)
	entriesBefore, err := os.ReadDir(temporaryBase)
	if err != nil {
		t.Fatal(err)
	}
	before := gitValue(t, root, "status", "--porcelain=v1", "--untracked-files=all")
	revision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")

	snapshot, err := (Git{}).MaterializeSnapshot(context.Background(), root, revision)
	if runtime.GOOS == "windows" {
		if err != nil && !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("MaterializeSnapshot() error = %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if snapshot != nil {
		if withinSnapshotRoot(root, snapshot.Root()) {
			t.Fatal("snapshot was materialized inside source repository")
		}
		if err := snapshot.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if after := gitValue(t, root, "status", "--porcelain=v1", "--untracked-files=all"); after != before {
		t.Fatalf("source status changed from %q to %q", before, after)
	}
	entriesAfter, err := os.ReadDir(temporaryBase)
	if err != nil {
		t.Fatal(err)
	}
	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("source temporary base was touched: before=%v after=%v", entriesBefore, entriesAfter)
	}
}

func TestConfiguredSnapshotBaseOverlapIsRejectedBeforeCreatingDirectories(t *testing.T) {
	root := committedRepository(t)
	base := filepath.Join(root, "generated", "planning-snapshots")
	before := gitValue(t, root, "status", "--porcelain=v1", "--untracked-files=all")
	revision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")

	_, err := (Git{SnapshotBase: base}).MaterializeSnapshot(context.Background(), root, revision)
	if !errors.Is(err, domain.ErrRepositoryUnsupported) {
		t.Fatalf("MaterializeSnapshot() error = %v, want ErrRepositoryUnsupported", err)
	}
	if _, err := os.Stat(filepath.Join(root, "generated")); !os.IsNotExist(err) {
		t.Fatalf("overlapping snapshot preflight created source directories: %v", err)
	}
	if after := gitValue(t, root, "status", "--porcelain=v1", "--untracked-files=all"); after != before {
		t.Fatalf("source status changed from %q to %q", before, after)
	}
}

func TestConfiguredSnapshotBaseResolvesParentSymlinkBeforeCreatingDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires environment-specific privilege")
	}
	root := committedRepository(t)
	outside := t.TempDir()
	linkedParent := filepath.Join(outside, "source-link")
	if err := os.Symlink(root, linkedParent); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(linkedParent, "planning-snapshots")
	revision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")

	_, err := (Git{SnapshotBase: base}).MaterializeSnapshot(context.Background(), root, revision)
	if !errors.Is(err, domain.ErrRepositoryUnsupported) {
		t.Fatalf("MaterializeSnapshot() error = %v, want ErrRepositoryUnsupported", err)
	}
	if _, err := os.Stat(filepath.Join(root, "planning-snapshots")); !os.IsNotExist(err) {
		t.Fatalf("symlinked snapshot preflight created source directories: %v", err)
	}
}

func TestMaterializeSnapshotValidatesRootAndCommit(t *testing.T) {
	root := committedRepository(t)
	baseRevision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	subdirectory := filepath.Join(root, "nested")
	if err := os.Mkdir(subdirectory, 0755); err != nil {
		t.Fatal(err)
	}
	blobRevision := gitValue(t, root, "hash-object", "tracked.txt")

	tests := []struct {
		name     string
		root     string
		revision string
		want     error
	}{
		{name: "relative root", root: ".", revision: baseRevision, want: domain.ErrInvalidRequest},
		{name: "unclean root", root: root + string(os.PathSeparator) + ".", revision: baseRevision, want: domain.ErrInvalidRequest},
		{name: "subdirectory", root: subdirectory, revision: baseRevision, want: domain.ErrInvalidRequest},
		{name: "short revision", root: root, revision: baseRevision[:12], want: domain.ErrInvalidRequest},
		{name: "blob object", root: root, revision: blobRevision, want: domain.ErrBaseRevisionUnavailable},
		{name: "missing commit", root: root, revision: strings.Repeat("f", len(baseRevision)), want: domain.ErrBaseRevisionUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Git{}).MaterializeSnapshot(context.Background(), test.root, test.revision)
			if !errors.Is(err, test.want) {
				t.Fatalf("MaterializeSnapshot() error = %v, want %v", err, test.want)
			}
			assertSnapshotErrorSanitized(t, err, root)
		})
	}
}

func TestMaterializeSnapshotCleansFailedClone(t *testing.T) {
	root := committedRepository(t)
	temporaryRoot, err := createSnapshotTempRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := removeSnapshotTree(temporaryRoot); err != nil {
			t.Errorf("clean test temporary root: %v", err)
		}
	})
	t.Setenv("TMPDIR", temporaryRoot)
	revision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	snapshot, err := materializeSnapshot(context.Background(), root, revision, func(context.Context, string, string, string) error {
		return fmt.Errorf("injected materialization failure: %w", domain.ErrUnavailable)
	})
	if snapshot != nil {
		t.Fatal("failed materialization returned a snapshot")
	}
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("MaterializeSnapshot() error = %v, want ErrUnavailable", err)
	}
	assertSnapshotErrorSanitized(t, err, root, temporaryRoot)
	entries, readErr := os.ReadDir(temporaryRoot)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed materialization left temporary entries: %v", entries)
	}
}

func TestIsolatedSnapshotCloseIsIdempotent(t *testing.T) {
	root := committedRepository(t)
	baseRevision := gitValue(t, root, "rev-parse", "--verify", "HEAD^{commit}")
	snapshot := materializeSnapshotFixture(t, root, baseRevision)
	temporaryRoot := filepath.Dir(snapshot.Root())

	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(temporaryRoot); !os.IsNotExist(err) {
		t.Fatalf("snapshot temporary root still exists after Close: %v", err)
	}
	if err := snapshot.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := snapshot.Resolve("tracked.txt"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Resolve() after Close error = %v, want ErrUnavailable", err)
	}
}

func materializeSnapshotFixture(t *testing.T, root, revision string) *IsolatedSnapshot {
	t.Helper()
	snapshot, err := (Git{}).MaterializeSnapshot(context.Background(), root, revision)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := snapshot.Close(); err != nil {
			t.Errorf("close snapshot: %v", err)
		}
	})
	return snapshot
}

func commitSnapshotFixture(t *testing.T, root, message string) {
	t.Helper()
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", message)
}

func gitValue(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}

func writeSnapshotFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readSnapshotFixture(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func assertSnapshotErrorSanitized(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("error exposed an absolute path: %v", err)
		}
	}
}

func snapshotTreeContains(t *testing.T, root, needle string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(content), needle) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

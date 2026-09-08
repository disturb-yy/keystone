package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

const isolatedSnapshotPrefix = "keystone-planning-snapshot-"

// IsolatedSnapshot 是固定 revision 的进程内临时 source handle。
// Root 返回的绝对路径只能传给本次 Planning 执行，调用方不得持久化或发布该路径。
// IsolatedSnapshot 在首次使用后不得复制。
type IsolatedSnapshot struct {
	root        string
	revision    string
	cleanupRoot string

	mu       sync.RWMutex
	closed   bool
	closeErr error
}

// Root 返回隔离 checkout 的临时根路径；该路径只在 Close 前有效。
func (s *IsolatedSnapshot) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// Revision 返回隔离 checkout 已验证的完整 commit object ID。
func (s *IsolatedSnapshot) Revision() string {
	if s == nil {
		return ""
	}
	return s.revision
}

// Resolve 将 portable 相对路径解析到 Snapshot 内，并拒绝穿越和 symlink 逃逸。
func (s *IsolatedSnapshot) Resolve(relativePath string) (string, error) {
	if s == nil || !validSnapshotRelativePath(relativePath) {
		return "", fmt.Errorf("resolve isolated snapshot path: %w", domain.ErrInvalidRequest)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed || s.root == "" {
		return "", fmt.Errorf("resolve isolated snapshot path: %w", domain.ErrUnavailable)
	}
	candidate := filepath.Join(s.root, filepath.FromSlash(relativePath))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve isolated snapshot path: %w", domain.ErrUnavailable)
	}
	if !withinSnapshotRoot(s.root, resolved) {
		return "", fmt.Errorf("resolve isolated snapshot path: %w", domain.ErrInvalidRequest)
	}
	return resolved, nil
}

// Close 幂等删除隔离 checkout 及其私有临时父目录。
func (s *IsolatedSnapshot) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed && s.closeErr == nil {
		return nil
	}
	s.closed = true
	if s.cleanupRoot == "" {
		s.closeErr = fmt.Errorf("clean isolated snapshot: %w", domain.ErrInvalidRequest)
		return s.closeErr
	}
	s.closeErr = removeSnapshotTree(s.cleanupRoot)
	return s.closeErr
}

// MaterializeSnapshot 创建固定 baseRevision 的独立临时 clone。
// 源 Repository 可以有未提交变化；materialization 只消费其 commit object database。
func (git Git) MaterializeSnapshot(ctx context.Context, repositoryRoot, baseRevision string) (snapshot *IsolatedSnapshot, resultErr error) {
	return materializeSnapshotAt(ctx, repositoryRoot, baseRevision, git.SnapshotBase, materializeSnapshotClone)
}

func materializeSnapshot(ctx context.Context, repositoryRoot, baseRevision string, materialize func(context.Context, string, string, string) error) (snapshot *IsolatedSnapshot, resultErr error) {
	return materializeSnapshotAt(ctx, repositoryRoot, baseRevision, "", materialize)
}

func materializeSnapshotAt(ctx context.Context, repositoryRoot, baseRevision, snapshotBase string, materialize func(context.Context, string, string, string) error) (snapshot *IsolatedSnapshot, resultErr error) {
	if err := validateMaterializeRequest(ctx, repositoryRoot, baseRevision); err != nil {
		return nil, err
	}
	if materialize == nil {
		return nil, fmt.Errorf("materialize isolated snapshot: %w", domain.ErrInvalidRequest)
	}
	if err := verifySnapshotSource(ctx, repositoryRoot, baseRevision); err != nil {
		return nil, err
	}
	temporaryRoot, err := createSnapshotTempRootFor(snapshotBase, repositoryRoot)
	if err != nil {
		return nil, err
	}
	cleanupRequired := true
	defer func() {
		if !cleanupRequired {
			return
		}
		if cleanupErr := removeSnapshotTree(temporaryRoot); cleanupErr != nil {
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()

	checkoutRoot := filepath.Join(temporaryRoot, "source")
	if err := materialize(ctx, repositoryRoot, checkoutRoot, baseRevision); err != nil {
		return nil, err
	}
	if err := validateSnapshotSymlinks(checkoutRoot); err != nil {
		return nil, err
	}
	if err := restrictSnapshotPermissions(temporaryRoot); err != nil {
		return nil, err
	}
	cleanupRequired = false
	return &IsolatedSnapshot{root: checkoutRoot, revision: baseRevision, cleanupRoot: temporaryRoot}, nil
}

func validateMaterializeRequest(ctx context.Context, repositoryRoot, baseRevision string) error {
	if ctx == nil || repositoryRoot == "" || !filepath.IsAbs(repositoryRoot) || filepath.Clean(repositoryRoot) != repositoryRoot || !validGitOID(baseRevision) {
		return fmt.Errorf("materialize isolated snapshot: %w", domain.ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("materialize isolated snapshot: %w", err)
	}
	return nil
}

func verifySnapshotSource(ctx context.Context, repositoryRoot, baseRevision string) error {
	binding, err := (Git{}).Discover(ctx, repositoryRoot)
	if err != nil {
		return fmt.Errorf("verify isolated snapshot repository: %w", err)
	}
	if binding.Root != repositoryRoot {
		return fmt.Errorf("verify isolated snapshot repository root: %w", domain.ErrInvalidRequest)
	}
	resolved, err := snapshotGitOutput(ctx, repositoryRoot, "rev-parse", "--verify", baseRevision+"^{commit}")
	if err != nil {
		return snapshotGitError(ctx, "verify isolated snapshot revision", err, domain.ErrBaseRevisionUnavailable)
	}
	if strings.TrimSpace(resolved) != baseRevision {
		return fmt.Errorf("verify isolated snapshot revision: %w", domain.ErrBaseRevisionUnavailable)
	}
	return nil
}

func materializeSnapshotClone(ctx context.Context, repositoryRoot, checkoutRoot, baseRevision string) error {
	if err := os.Mkdir(checkoutRoot, 0700); err != nil {
		return fmt.Errorf("create isolated snapshot checkout: %w", domain.ErrUnavailable)
	}
	objectFormat := "sha1"
	if len(baseRevision) == 64 {
		objectFormat = "sha256"
	}
	if err := runSnapshotGitAt(ctx, checkoutRoot, "init", "--quiet", "--initial-branch=keystone-snapshot", "--object-format="+objectFormat); err != nil {
		return snapshotGitError(ctx, "initialize isolated snapshot", err, domain.ErrUnavailable)
	}
	if err := runSnapshotGitAt(ctx, checkoutRoot, "-c", "protocol.file.allow=always", "fetch", "--quiet", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", "--depth=1", "--", repositoryRoot, baseRevision); err != nil {
		return snapshotGitError(ctx, "fetch isolated snapshot revision", err, domain.ErrBaseRevisionUnavailable)
	}
	if err := runSnapshotGitAt(ctx, checkoutRoot, "checkout", "--quiet", "--detach", baseRevision); err != nil {
		return snapshotGitError(ctx, "checkout isolated snapshot revision", err, domain.ErrBaseRevisionUnavailable)
	}
	resolved, err := snapshotGitOutput(ctx, checkoutRoot, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(resolved) != baseRevision {
		return snapshotGitError(ctx, "verify isolated snapshot checkout", err, domain.ErrBaseRevisionUnavailable)
	}
	status, err := snapshotGitOutput(ctx, checkoutRoot, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil || strings.TrimSpace(status) != "" {
		return snapshotGitError(ctx, "verify isolated snapshot status", err, domain.ErrUnavailable)
	}
	refs, err := snapshotGitOutput(ctx, checkoutRoot, "for-each-ref", "--format=%(refname)")
	if err != nil || strings.TrimSpace(refs) != "" {
		return snapshotGitError(ctx, "verify isolated snapshot refs", err, domain.ErrUnavailable)
	}
	if err := os.RemoveAll(filepath.Join(checkoutRoot, ".git", "logs")); err != nil {
		return fmt.Errorf("remove isolated snapshot reflogs: %w", domain.ErrUnavailable)
	}
	return nil
}

func validSnapshotRelativePath(value string) bool {
	if value == "" || value == "." || strings.ContainsAny(value, "\\\x00:") || path.IsAbs(value) || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return false
	}
	if path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func withinSnapshotRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func validateSnapshotSymlinks(root string) error {
	return filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("inspect isolated snapshot entry: %w", domain.ErrRepositoryUnsupported)
		}
		if current == filepath.Join(root, ".git") && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(current)
		if err != nil || filepath.IsAbs(target) || filepath.VolumeName(target) != "" {
			return fmt.Errorf("isolated snapshot contains unsupported symlink: %w", domain.ErrRepositoryUnsupported)
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil || !withinSnapshotRoot(root, resolved) {
			return fmt.Errorf("isolated snapshot contains escaping symlink: %w", domain.ErrRepositoryUnsupported)
		}
		return nil
	})
}

func createSnapshotTempRoot() (string, error) {
	return createSnapshotTempRootAt("")
}

func createSnapshotTempRootAt(configuredBase string) (string, error) {
	return createSnapshotTempRootFor(configuredBase, "")
}

func createSnapshotTempRootFor(configuredBase, repositoryRoot string) (string, error) {
	if configuredBase != "" {
		base, err := preflightSnapshotBase(configuredBase, repositoryRoot)
		if err != nil {
			return "", err
		}
		base, err = prepareSnapshotBase(base)
		if err != nil {
			return "", err
		}
		root, err := createSnapshotRootIn(base)
		if err != nil {
			return "", err
		}
		if snapshotSourcePathsOverlap(repositoryRoot, root) {
			return "", errors.Join(
				fmt.Errorf("planning snapshot root overlaps source repository: %w", domain.ErrRepositoryUnsupported),
				removeSnapshotTree(root),
			)
		}
		return root, nil
	}
	temporaryBases := []string{os.TempDir()}
	if runtime.GOOS != "windows" && filepath.Clean(os.TempDir()) != string(os.PathSeparator)+"tmp" {
		temporaryBases = append(temporaryBases, string(os.PathSeparator)+"tmp")
	}
	for _, candidate := range temporaryBases {
		base, err := prospectivePhysicalPath(candidate)
		if err != nil || snapshotRootFallsInsideSource(repositoryRoot, base) {
			continue
		}
		root, err := createSnapshotRootIn(base)
		if err != nil {
			continue
		}
		if snapshotSourcePathsOverlap(repositoryRoot, root) {
			_ = removeSnapshotTree(root)
			continue
		}
		return root, nil
	}
	return "", fmt.Errorf("create isolated snapshot: %w", domain.ErrUnavailable)
}

func preflightSnapshotBase(base, repositoryRoot string) (string, error) {
	if err := validateSnapshotBasePath(base); err != nil {
		return "", err
	}
	resolved, err := prospectivePhysicalPath(base)
	if err != nil {
		return "", fmt.Errorf("resolve planning snapshot base: %w", domain.ErrRepositoryUnsupported)
	}
	if snapshotRootFallsInsideSource(repositoryRoot, resolved) {
		return "", fmt.Errorf("planning snapshot base overlaps source repository: %w", domain.ErrRepositoryUnsupported)
	}
	return resolved, nil
}

func prospectivePhysicalPath(value string) (string, error) {
	if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", domain.ErrInvalidRequest
	}
	cursor := value
	missing := make([]string, 0)
	for {
		_, err := os.Lstat(cursor)
		if err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(cursor)
			if resolveErr != nil {
				return "", resolveErr
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", err
		}
		missing = append(missing, filepath.Base(cursor))
		cursor = parent
	}
}

func snapshotSourcePathsOverlap(repositoryRoot, snapshotRoot string) bool {
	if repositoryRoot == "" {
		return false
	}
	return withinSnapshotRoot(repositoryRoot, snapshotRoot) || withinSnapshotRoot(snapshotRoot, repositoryRoot)
}

func snapshotRootFallsInsideSource(repositoryRoot, snapshotRoot string) bool {
	return repositoryRoot != "" && withinSnapshotRoot(repositoryRoot, snapshotRoot)
}

func prepareSnapshotBase(base string) (string, error) {
	if err := validateSnapshotBasePath(base); err != nil {
		return "", err
	}
	resolved, err := prospectivePhysicalPath(base)
	if err != nil {
		return "", fmt.Errorf("prepare planning snapshot base: %w", domain.ErrRepositoryUnsupported)
	}
	base = resolved
	if filepath.Base(base) != "planning-snapshots" {
		return "", fmt.Errorf("prepare planning snapshot base: %w", domain.ErrInvalidRequest)
	}
	if info, err := os.Lstat(base); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("prepare planning snapshot base: %w", domain.ErrRepositoryUnsupported)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("prepare planning snapshot base: %w", domain.ErrUnavailable)
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", fmt.Errorf("create planning snapshot base: %w", domain.ErrUnavailable)
	}
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("verify planning snapshot base: %w", domain.ErrRepositoryUnsupported)
	}
	if err := restrictSnapshotPermissions(base); err != nil {
		return "", err
	}
	return base, nil
}

func validateSnapshotBasePath(base string) error {
	if !filepath.IsAbs(base) || filepath.Clean(base) != base || filepath.Base(base) != "planning-snapshots" {
		return fmt.Errorf("prepare planning snapshot base: %w", domain.ErrInvalidRequest)
	}
	return nil
}

func createSnapshotRootIn(base string) (string, error) {
	root, err := os.MkdirTemp(base, isolatedSnapshotPrefix)
	if err != nil {
		return "", fmt.Errorf("create isolated snapshot: %w", domain.ErrUnavailable)
	}
	if err := restrictSnapshotPermissions(root); err == nil {
		resolved, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr == nil && filepath.IsAbs(resolved) && filepath.Clean(resolved) == resolved {
			return resolved, nil
		}
	}
	if err := removeSnapshotTree(root); err != nil {
		return "", err
	}
	return "", fmt.Errorf("create isolated snapshot: %w", domain.ErrUnavailable)
}

// ResetSnapshots 删除同一 Daemon 数据根中上次异常退出遗留的 Planning Snapshot。
// 只清理固定父目录内由 Keystone 前缀创建的直接子项。
func (git Git) ResetSnapshots() error {
	base, err := prepareSnapshotBase(git.SnapshotBase)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return fmt.Errorf("list planning snapshot base: %w", domain.ErrUnavailable)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), isolatedSnapshotPrefix) {
			continue
		}
		if err := removeSnapshotTree(filepath.Join(base, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func restrictSnapshotPermissions(root string) error {
	if err := os.Chmod(root, 0700); err != nil {
		return fmt.Errorf("restrict isolated snapshot permissions: %w", domain.ErrUnavailable)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("verify isolated snapshot permissions: %w", domain.ErrUnavailable)
	}
	return nil
}

func snapshotGitOutput(ctx context.Context, root string, args ...string) (string, error) {
	command := snapshotGitCommand(ctx, append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	return string(output), err
}

func runSnapshotGitAt(ctx context.Context, root string, args ...string) error {
	return runSnapshotGit(ctx, append([]string{"-C", root}, args...)...)
}

func runSnapshotGit(ctx context.Context, args ...string) error {
	return snapshotGitCommand(ctx, args...).Run()
}

func snapshotGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = snapshotGitEnvironment()
	return command
}

func snapshotGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	)
}

func snapshotGitError(ctx context.Context, operation string, commandErr, classification error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	var exitError *exec.ExitError
	if errors.As(commandErr, &exitError) {
		return fmt.Errorf("%s (git exit %d): %w", operation, exitError.ExitCode(), classification)
	}
	return fmt.Errorf("%s: %w", operation, classification)
}

func removeSnapshotTree(root string) error {
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("clean isolated snapshot: %w", domain.ErrUnavailable)
	}
	return nil
}

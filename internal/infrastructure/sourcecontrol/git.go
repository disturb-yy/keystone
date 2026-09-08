// Package sourcecontrol 提供 Ticket 09 的受约束 Git Worktree 适配器。
package sourcecontrol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	executiondomain "github.com/disturb-yy/keystone/internal/execution/domain"
	"github.com/disturb-yy/keystone/internal/infrastructure/repository"
	workdomain "github.com/disturb-yy/keystone/internal/work/domain"
)

var (
	ErrInvalidRequest        = errors.New("invalid source control request")
	ErrRepositoryDirty       = errors.New("source repository is dirty")
	ErrRevisionMismatch      = errors.New("source revision does not match")
	ErrBranchConflict        = errors.New("source branch conflicts")
	ErrWorkspaceConflict     = errors.New("workspace identity conflicts")
	ErrCommitNotFound        = errors.New("controlled commit is not present at HEAD")
	ErrUnsupportedRepository = errors.New("source repository topology is unsupported")
	ErrGitUnavailable        = errors.New("git is unavailable")
)

// Adapter 是仅允许执行固定 Git 子命令的 SourceControl 实现。
type Adapter struct {
	GitPath string
}

// Git 是 Adapter 的语义别名，便于 Daemon composition 表达 Git 依赖。
type Git = Adapter

// ProvisionRequest 描述一次在源 Repository 上创建或恢复 Worktree 的请求。
type ProvisionRequest struct {
	RepositoryRoot string
	WorkspacePath  string
	Branch         string
	BaseRevision   string
}

// ProvisionResult 返回可供内部持久化的 Worktree 身份；绝对路径不得越过内部边界。
type ProvisionResult struct {
	WorkspacePath string
	PhysicalPath  string
	Branch        string
	HeadRevision  string
}

// Snapshot 是完整未提交状态的 Git 观察值。
type Snapshot struct {
	HeadRevision string
	Branch       string
	Diff         []byte
	ChangedFiles []string
	HasUntracked bool
	// TreeIdentity 是把当前候选纳入临时 index 后得到的 tree object identity。
	TreeIdentity string
}

// Provision 校验源、branch 和物理身份后创建或恢复唯一 Worktree。
func (a Adapter) Provision(ctx context.Context, request ProvisionRequest) (ProvisionResult, error) {
	if ctx == nil {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrInvalidRequest)
	}
	root, err := normalizeExistingDir(request.RepositoryRoot)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrInvalidRequest)
	}
	workspace, err := normalizeWorkspacePath(request.WorkspacePath)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrInvalidRequest)
	}
	if err := executiondomain.ValidateBranch(request.Branch); err != nil {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrInvalidRequest)
	}
	if !validRevision(request.BaseRevision) {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrInvalidRequest)
	}
	if pathsOverlap(root, workspaceCandidate(workspace)) {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrWorkspaceConflict)
	}
	if err := a.preflightSource(ctx, root, request.BaseRevision); err != nil {
		return ProvisionResult{}, err
	}
	if info, statErr := os.Lstat(workspace); statErr == nil {
		if !info.IsDir() {
			return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrWorkspaceConflict)
		}
		return a.reconcileExisting(ctx, root, workspace, request)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrWorkspaceConflict)
	}
	if exists, err := a.branchExists(ctx, root, request.Branch); err != nil {
		return ProvisionResult{}, err
	} else if exists {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrBranchConflict)
	}
	if err := os.MkdirAll(filepath.Dir(workspace), 0o700); err != nil {
		return ProvisionResult{}, fmt.Errorf("provision worktree parent: %w", ErrWorkspaceConflict)
	}
	if _, _, err := a.run(ctx, root, "worktree", "add", "-b", request.Branch, workspace, request.BaseRevision); err != nil {
		return ProvisionResult{}, fmt.Errorf("provision worktree: %w", ErrWorkspaceConflict)
	}
	return a.readProvisioned(ctx, workspace, request.Branch, request.BaseRevision)
}

// Observe 在不执行 Git 写操作的前提下读取 Workspace 的 revision、branch 和完整状态。
func (a Adapter) Observe(ctx context.Context, workspace, expectedRevision string) (Snapshot, error) {
	path, err := normalizeExistingDir(workspace)
	if err != nil || !validRevision(expectedRevision) {
		return Snapshot{}, fmt.Errorf("observe workspace: %w", ErrInvalidRequest)
	}
	head, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || !validRevision(strings.TrimSpace(string(head))) {
		return Snapshot{}, fmt.Errorf("observe workspace head: %w", ErrGitUnavailable)
	}
	headValue := strings.TrimSpace(string(head))
	if !strings.EqualFold(headValue, expectedRevision) {
		return Snapshot{}, fmt.Errorf("observe workspace head: %w", ErrRevisionMismatch)
	}
	branch, err := a.output(ctx, path, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) == "" {
		return Snapshot{}, fmt.Errorf("observe workspace branch: %w", ErrWorkspaceConflict)
	}
	diff, err := a.output(ctx, path, "diff", "--binary", "--no-ext-diff", "HEAD", "--")
	if err != nil {
		return Snapshot{}, fmt.Errorf("observe workspace diff: %w", ErrGitUnavailable)
	}
	status, err := a.output(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return Snapshot{}, fmt.Errorf("observe workspace status: %w", ErrGitUnavailable)
	}
	files, untracked, err := parseStatus(status)
	if err != nil {
		return Snapshot{}, fmt.Errorf("observe workspace status: %w", ErrGitUnavailable)
	}
	return Snapshot{HeadRevision: headValue, Branch: strings.TrimSpace(string(branch)), Diff: append([]byte(nil), diff...), ChangedFiles: files, HasUntracked: untracked}, nil
}

// ReadManifestAtRevision 从固定 BaseRevision 读取策略文件，不读取候选 Workspace 的当前内容。
func (a Adapter) ReadManifestAtRevision(ctx context.Context, workspace, revision string) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("read manifest revision: %w", ErrInvalidRequest)
	}
	path, err := normalizeExistingDir(workspace)
	if err != nil || !validRevision(revision) {
		return nil, fmt.Errorf("read manifest revision: %w", ErrInvalidRequest)
	}
	content, err := a.output(ctx, path, "show", revision+":.keystone/project.yaml")
	if err != nil {
		return nil, fmt.Errorf("read manifest revision: %w", ErrRevisionMismatch)
	}
	return append([]byte(nil), content...), nil
}

func (a Adapter) preflightSource(ctx context.Context, root, revision string) error {
	bare, err := a.output(ctx, root, "rev-parse", "--is-bare-repository")
	if err != nil || strings.TrimSpace(string(bare)) != "false" {
		return fmt.Errorf("preflight source: %w", ErrUnsupportedRepository)
	}
	workTree, err := a.output(ctx, root, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(workTree)) != "true" {
		return fmt.Errorf("preflight source: %w", ErrUnsupportedRepository)
	}
	top, err := a.output(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(strings.TrimSpace(string(top))) != root {
		return fmt.Errorf("preflight source: %w", ErrUnsupportedRepository)
	}
	snapshot, snapshotErr := (repository.Git{GitPath: a.git()}).Snapshot(ctx, root)
	if snapshotErr != nil {
		switch {
		case errors.Is(snapshotErr, workdomain.ErrRepositoryDirty):
			return fmt.Errorf("preflight source: %w", ErrRepositoryDirty)
		case errors.Is(snapshotErr, workdomain.ErrBaseRevisionUnavailable):
			return fmt.Errorf("preflight source: %w", ErrRevisionMismatch)
		default:
			return fmt.Errorf("preflight source: %w", ErrGitUnavailable)
		}
	}
	if !strings.EqualFold(snapshot.BaseRevision, revision) {
		return fmt.Errorf("preflight source: %w", ErrRevisionMismatch)
	}
	return nil
}

func (a Adapter) branchExists(ctx context.Context, root, branch string) (bool, error) {
	if _, err := a.output(ctx, root, "check-ref-format", "--branch", branch); err != nil {
		return false, fmt.Errorf("validate branch: %w", ErrInvalidRequest)
	}
	command := exec.CommandContext(ctx, a.git(), "-C", root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	command.Env = gitEnvironment()
	if err := command.Run(); err == nil {
		return true, nil
	} else if exitCode(err) == 1 {
		return false, nil
	} else {
		return false, fmt.Errorf("check branch: %w", ErrGitUnavailable)
	}
}

func (a Adapter) reconcileExisting(ctx context.Context, root, workspace string, request ProvisionRequest) (ProvisionResult, error) {
	physical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("reconcile worktree: %w", ErrWorkspaceConflict)
	}
	physical, err = filepath.Abs(filepath.Clean(physical))
	if err != nil || pathsOverlap(root, physical) {
		return ProvisionResult{}, fmt.Errorf("reconcile worktree: %w", ErrWorkspaceConflict)
	}
	list, err := a.output(ctx, root, "worktree", "list", "--porcelain")
	if err != nil || !worktreeListContains(list, physical) {
		return ProvisionResult{}, fmt.Errorf("reconcile worktree: %w", ErrWorkspaceConflict)
	}
	return a.readProvisioned(ctx, workspace, request.Branch, request.BaseRevision)
}

func (a Adapter) readProvisioned(ctx context.Context, workspace, branch, revision string) (ProvisionResult, error) {
	snapshot, err := a.Observe(ctx, workspace, revision)
	if err != nil {
		return ProvisionResult{}, err
	}
	if snapshot.Branch != branch {
		return ProvisionResult{}, fmt.Errorf("read provisioned worktree: %w", ErrWorkspaceConflict)
	}
	physical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("read provisioned worktree identity: %w", ErrWorkspaceConflict)
	}
	physical, err = filepath.Abs(filepath.Clean(physical))
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("read provisioned worktree identity: %w", ErrWorkspaceConflict)
	}
	return ProvisionResult{WorkspacePath: filepath.Clean(workspace), PhysicalPath: physical, Branch: snapshot.Branch, HeadRevision: snapshot.HeadRevision}, nil
}

func (a Adapter) output(ctx context.Context, path string, args ...string) ([]byte, error) {
	_, output, err := a.run(ctx, path, args...)
	return output, err
}

func (a Adapter) run(ctx context.Context, path string, args ...string) (int, []byte, error) {
	command := exec.CommandContext(ctx, a.git(), append([]string{"-C", path}, args...)...)
	command.Env = gitEnvironment()
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		return exitCode(err), nil, err
	}
	return 0, stdout.Bytes(), nil
}

func (a Adapter) git() string {
	if strings.TrimSpace(a.GitPath) != "" {
		return a.GitPath
	}
	return "git"
}

func gitEnvironment() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "GIT_OPTIONAL_LOCKS=") {
			continue
		}
		env = append(env, item)
	}
	return append(env, "GIT_OPTIONAL_LOCKS=0")
}

func normalizeExistingDir(value string) (string, error) {
	if value == "" || !filepath.IsAbs(value) {
		return "", ErrInvalidRequest
	}
	path, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", ErrInvalidRequest
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Clean(physical))
}

func normalizeWorkspacePath(value string) (string, error) {
	if value == "" || !filepath.IsAbs(value) {
		return "", ErrInvalidRequest
	}
	return filepath.Abs(filepath.Clean(value))
}

func workspaceCandidate(path string) string {
	path = filepath.Clean(path)
	if physical, err := filepath.EvalSymlinks(path); err == nil {
		if absolute, absErr := filepath.Abs(filepath.Clean(physical)); absErr == nil {
			return absolute
		}
	}
	parent := filepath.Dir(path)
	for {
		physicalParent, err := filepath.EvalSymlinks(parent)
		if err == nil {
			relative, relErr := filepath.Rel(parent, path)
			if relErr == nil {
				return filepath.Join(physicalParent, relative)
			}
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	return path
}

func pathsOverlap(first, second string) bool {
	first, _ = filepath.Abs(filepath.Clean(first))
	second, _ = filepath.Abs(filepath.Clean(second))
	relative, err := filepath.Rel(first, second)
	if err == nil && (relative == "." || (relative != "" && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))) {
		return true
	}
	relative, err = filepath.Rel(second, first)
	return err == nil && (relative == "." || (relative != "" && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

func parseStatus(value []byte) ([]string, bool, error) {
	parts := bytes.Split(value, []byte{0})
	files := make([]string, 0, len(parts))
	untracked := false
	for index := 0; index < len(parts); index++ {
		part := parts[index]
		if len(part) == 0 {
			continue
		}
		if len(part) < 3 {
			return nil, false, ErrGitUnavailable
		}
		if len(part) >= 2 && part[0] == '?' && part[1] == '?' {
			untracked = true
		}
		paths := [][]byte{part[3:]}
		if part[0] == 'R' || part[0] == 'C' || part[1] == 'R' || part[1] == 'C' {
			if index+1 >= len(parts) || len(parts[index+1]) == 0 {
				return nil, false, ErrGitUnavailable
			}
			paths = append(paths, parts[index+1])
			index++
		}
		for _, rawPath := range paths {
			path := strings.TrimSpace(string(rawPath))
			if path == "" {
				return nil, false, ErrGitUnavailable
			}
			if filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
				return nil, false, ErrGitUnavailable
			}
			files = append(files, filepath.ToSlash(path))
		}
	}
	sort.Strings(files)
	unique := files[:0]
	for _, file := range files {
		if len(unique) == 0 || unique[len(unique)-1] != file {
			unique = append(unique, file)
		}
	}
	return unique, untracked, nil
}

func worktreeListContains(value []byte, physical string) bool {
	lines := strings.Split(string(value), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				resolved, _ = filepath.Abs(filepath.Clean(resolved))
				if resolved == physical {
					return true
				}
			}
		}
	}
	return false
}

func validRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

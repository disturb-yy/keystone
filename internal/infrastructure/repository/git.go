// Package repository 提供 Project Bootstrap 使用的只读 Git 适配器。
package repository

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

// Git 提供真实 Git Repository 的拓扑识别。
type Git struct{}

// Snapshot 连续读取两次干净状态和 HEAD，确认 Change 的 BaseRevision 稳定。
//
// 该方法只执行 Git 读命令，不创建锁，也不改变 Repository 内容。
func (Git) Snapshot(ctx context.Context, root string) (domain.ChangeSourceSnapshot, error) {
	if ctx == nil || root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return domain.ChangeSourceSnapshot{}, fmt.Errorf("snapshot repository: %w", domain.ErrInvalidRequest)
	}
	first, err := readSnapshotPoint(ctx, root)
	if err != nil {
		return domain.ChangeSourceSnapshot{}, err
	}
	second, err := readSnapshotPoint(ctx, root)
	if err != nil {
		return domain.ChangeSourceSnapshot{}, err
	}
	if first != second {
		return domain.ChangeSourceSnapshot{}, fmt.Errorf("repository HEAD changed during snapshot: %w", domain.ErrSourceSnapshotUnstable)
	}
	return domain.ChangeSourceSnapshot{RepositoryRoot: root, BaseRevision: first}, nil
}

// Discover 将调用路径解析为物理 RepositoryBinding。
func (Git) Discover(ctx context.Context, path string) (domain.RepositoryBinding, error) {
	if ctx == nil {
		return domain.RepositoryBinding{}, fmt.Errorf("discover repository: %w", domain.ErrInvalidRequest)
	}
	if path == "" || !filepath.IsAbs(path) {
		return domain.RepositoryBinding{}, fmt.Errorf("discover repository: %w", domain.ErrInvalidRequest)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return domain.RepositoryBinding{}, fmt.Errorf("discover repository: %w", domain.ErrRepositoryUnsupported)
	}
	bare, err := gitOutput(ctx, path, "rev-parse", "--is-bare-repository")
	if err != nil || strings.TrimSpace(bare) == "true" {
		return domain.RepositoryBinding{}, fmt.Errorf("discover repository: %w", domain.ErrRepositoryUnsupported)
	}
	inside, err := gitOutput(ctx, path, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return domain.RepositoryBinding{}, fmt.Errorf("discover repository: %w", domain.ErrRepositoryUnsupported)
	}
	rootText, err := gitOutput(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return domain.RepositoryBinding{}, fmt.Errorf("discover repository: %w", domain.ErrRepositoryUnsupported)
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(rootText))
	if err != nil {
		return domain.RepositoryBinding{}, fmt.Errorf("resolve repository root: %w", domain.ErrRepositoryUnsupported)
	}
	root, err = filepath.Abs(filepath.Clean(root))
	if err != nil {
		return domain.RepositoryBinding{}, fmt.Errorf("normalize repository root: %w", domain.ErrRepositoryUnsupported)
	}
	if err := rejectLinkedWorktree(ctx, root); err != nil {
		return domain.RepositoryBinding{}, err
	}
	binding := domain.RepositoryBinding{Root: root, ManifestPath: filepath.Join(root, ".keystone", "project.yaml")}
	if err := binding.Validate(); err != nil {
		return domain.RepositoryBinding{}, err
	}
	return binding, nil
}

// RootExists 判断旧 Binding 是否仍可验证存在。
func (Git) RootExists(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("previous repository root is not a directory: %w", domain.ErrProjectIdentityConflict)
		}
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat previous repository root: %w", err)
}

func gitOutput(ctx context.Context, path string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	env := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "GIT_OPTIONAL_LOCKS=") {
			continue
		}
		env = append(env, value)
	}
	command.Env = append(env, "GIT_OPTIONAL_LOCKS=0")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func readSnapshotPoint(ctx context.Context, root string) (string, error) {
	status, err := gitOutput(ctx, root, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return "", fmt.Errorf("read repository status: %w", domain.ErrUnavailable)
	}
	if strings.TrimSpace(status) != "" {
		return "", fmt.Errorf("repository has uncommitted changes: %w", domain.ErrRepositoryDirty)
	}
	head, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve repository HEAD: %w", domain.ErrBaseRevisionUnavailable)
	}
	head = strings.TrimSpace(head)
	if !validGitOID(head) {
		return "", fmt.Errorf("repository HEAD is not a complete object id: %w", domain.ErrBaseRevisionUnavailable)
	}
	return head, nil
}

func validGitOID(value string) bool {
	if (len(value) != 40 && len(value) != 64) || strings.ToLower(value) != value {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}

func rejectLinkedWorktree(ctx context.Context, root string) error {
	gitPath := filepath.Join(root, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return fmt.Errorf("inspect repository topology: %w", domain.ErrRepositoryUnsupported)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	gitDir, err := gitOutput(ctx, root, "rev-parse", "--git-dir")
	if err != nil || strings.Contains(filepath.ToSlash(strings.TrimSpace(gitDir)), "/worktrees/") {
		return fmt.Errorf("linked worktree is unsupported: %w", domain.ErrRepositoryUnsupported)
	}
	return nil
}

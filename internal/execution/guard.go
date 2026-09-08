package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CommandRunner 是 Guard 运行 Git 观察命令的测试 seam。
type CommandRunner func(context.Context, ...string) ([]byte, error)

// GitEvidence 是执行前后从 Workspace 独立采集的 Git 事实。
type GitEvidence struct {
	Revision     string
	Diff         []byte
	ChangedFiles []string
}

// Guard 校验 Workspace 并为 Runtime 准备有限的 Git 命令约束。
type Guard struct {
	RunGit    CommandRunner
	GitBinary string
}

// NewGuard 创建使用本机 Git 的 Guard。
func NewGuard() *Guard { return &Guard{RunGit: defaultGitRunner} }

// ValidateWorkspace 要求 Workspace 是已经存在且可写的 Git root。
func (g *Guard) ValidateWorkspace(ctx context.Context, workspace string) (string, error) {
	if ctx == nil {
		return "", errors.New("validate workspace: nil context")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("validate workspace: resolve path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("validate workspace: stat: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0222 == 0 {
		return "", fmt.Errorf("validate workspace: path is not a writable directory")
	}
	rootOutput, err := g.git(ctx, absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("validate workspace: resolve git root: %w", err)
	}
	root := filepath.Clean(strings.TrimSpace(string(rootOutput)))
	if root != absolute {
		return "", fmt.Errorf("validate workspace: assigned path is not the git root")
	}
	return absolute, nil
}

// Head 返回 Workspace 当前 HEAD；无提交的仓库返回可分类错误。
func (g *Guard) Head(ctx context.Context, workspace string) (string, error) {
	value, err := g.git(ctx, workspace, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("read workspace head: %w", err)
	}
	revision := strings.TrimSpace(string(value))
	if revision == "" {
		return "", errors.New("read workspace head: empty revision")
	}
	return revision, nil
}

// Evidence 采集 diff 和去重排序的 Workspace 相对 changed files。
func (g *Guard) Evidence(ctx context.Context, workspace string, limits Limits) (GitEvidence, error) {
	revision, err := g.Head(ctx, workspace)
	if err != nil {
		return GitEvidence{}, err
	}
	diff, err := g.git(ctx, workspace, "diff", "--binary", "--no-ext-diff", "--")
	if err != nil {
		return GitEvidence{}, fmt.Errorf("read workspace diff: %w", err)
	}
	status, err := g.git(ctx, workspace, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--")
	if err != nil {
		return GitEvidence{}, fmt.Errorf("read workspace status: %w", err)
	}
	files, err := parseChangedFiles(string(status))
	if err != nil {
		return GitEvidence{}, err
	}
	files, err = NormalizeChangedFiles(files)
	if err != nil {
		return GitEvidence{}, err
	}
	return GitEvidence{Revision: revision, Diff: limitBytes(diff, limits.DiffBytes), ChangedFiles: files}, nil
}

// CheckRevision 返回执行前后 HEAD 变化的 Guard finding。
func (g *Guard) CheckRevision(before, after string) []string {
	if before == "" || after == "" || before == after {
		return nil
	}
	return []string{"workspace HEAD changed during runtime"}
}

// PrepareEnvironment 构造经过过滤且带 Git 拒绝 wrapper 的 Runtime 环境。
func (g *Guard) PrepareEnvironment(ctx context.Context, base []string) ([]string, func(), error) {
	return g.PrepareEnvironmentAt(ctx, base, "")
}

// PrepareEnvironmentAt 在已验证的临时父目录内创建 Git 拒绝 wrapper；空父目录保持旧调用兼容。
func (g *Guard) PrepareEnvironmentAt(ctx context.Context, base []string, tempBase string) ([]string, func(), error) {
	if ctx == nil {
		return nil, func() {}, errors.New("prepare runtime environment: nil context")
	}
	if tempBase != "" {
		if !filepath.IsAbs(tempBase) || filepath.Clean(tempBase) != tempBase {
			return nil, func() {}, errors.New("prepare runtime environment: invalid temporary base")
		}
		info, err := os.Lstat(tempBase)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, func() {}, errors.New("prepare runtime environment: invalid temporary base")
		}
	}
	filtered := SanitizeEnvironment(base)
	realGit := g.GitBinary
	if realGit == "" {
		var err error
		realGit, err = exec.LookPath("git")
		if err != nil {
			return nil, func() {}, fmt.Errorf("prepare runtime environment: locate git: %w", err)
		}
	}
	directory, err := os.MkdirTemp(tempBase, "keystone-git-guard-")
	if err != nil {
		return nil, func() {}, fmt.Errorf("prepare runtime environment: create wrapper directory: %w", err)
	}
	path, err := writeGitWrapper(directory, realGit)
	if err != nil {
		_ = os.RemoveAll(directory)
		return nil, func() {}, err
	}
	pathValue := lookupEnv(filtered, "PATH")
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	filtered = setEnv(filtered, "PATH", filepath.Dir(path)+string(os.PathListSeparator)+pathValue)
	if tempBase != "" {
		// Runtime 自身及其子进程的临时文件也必须落在受控目录；只移动 Git wrapper
		// 仍会允许继承的 TMPDIR/TEMP/TMP 指回原始 Repository。
		filtered = setEnv(filtered, "TMPDIR", directory)
		filtered = setEnv(filtered, "TEMP", directory)
		filtered = setEnv(filtered, "TMP", directory)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	return filtered, cleanup, nil
}

func (g *Guard) git(ctx context.Context, workspace string, args ...string) ([]byte, error) {
	runner := g.RunGit
	if runner == nil {
		runner = defaultGitRunner
	}
	return runner(ctx, append([]string{"-C", workspace}, args...)...)
}

func defaultGitRunner(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	return command.Output()
}

func parseChangedFiles(value string) ([]string, error) {
	parts := strings.Split(value, "\x00")
	files := make([]string, 0, len(parts))
	for index := 0; index < len(parts); index++ {
		part := parts[index]
		if part == "" {
			continue
		}
		if len(part) < 4 {
			return nil, fmt.Errorf("parse workspace status: malformed entry")
		}
		file := part[3:]
		files = append(files, file)
		if strings.Contains(part[:2], "R") || strings.Contains(part[:2], "C") {
			if index+1 >= len(parts) || parts[index+1] == "" {
				return nil, fmt.Errorf("parse workspace status: missing rename target")
			}
			files = append(files, parts[index+1])
			index++
		}
	}
	return files, nil
}

func limitBytes(value []byte, limit int64) []byte {
	if limit <= 0 || int64(len(value)) <= limit {
		return append([]byte(nil), value...)
	}
	return append([]byte(nil), value[:limit]...)
}

func writeGitWrapper(directory, realGit string) (string, error) {
	if runtime.GOOS == "windows" {
		path := filepath.Join(directory, "git.cmd")
		content := "@echo off\r\nfor %%A in (%*) do (\r\n  if /I \"%%~A\"==\"commit\" exit /B 126\r\n  if /I \"%%~A\"==\"push\" exit /B 126\r\n  if /I \"%%~A\"==\"merge\" exit /B 126\r\n)\r\n\"" + realGit + "\" %*\r\n"
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			return "", fmt.Errorf("write git wrapper: %w", err)
		}
		return path, nil
	}
	path := filepath.Join(directory, "git")
	content := "#!/bin/sh\nfor arg in \"$@\"; do case \"$arg\" in commit|push|merge) echo 'git command denied by Keystone ExecutionGuard' >&2; exit 126;; esac; done\nexec " + shellQuote(realGit) + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		return "", fmt.Errorf("write git wrapper: %w", err)
	}
	return path, nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func lookupEnv(environment []string, key string) string {
	prefix := key + "="
	for _, value := range environment {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}

func setEnv(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, current := range environment {
		if !strings.HasPrefix(current, prefix) {
			result = append(result, current)
		}
	}
	return append(result, prefix+value)
}

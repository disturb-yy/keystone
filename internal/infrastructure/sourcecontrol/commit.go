package sourcecontrol

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CandidateIdentity 是 Verify/Commit 之间固定的候选 tree 身份。
type CandidateIdentity struct {
	HeadRevision string
	Branch       string
	TreeIdentity string
	Diff         []byte
	ChangedFiles []string
	HasUntracked bool
}

// CommitRequest 描述一次受 parent/tree/trailer 约束的 Git commit。
type CommitRequest struct {
	WorkspacePath  string
	ExpectedBranch string
	ExpectedParent string
	ExpectedTree   string
	Message        string
}

// CommitResult 返回已完成 commit 的可对账身份。
type CommitResult struct {
	GitOID         string
	ParentRevision string
	AfterRevision  string
	TreeIdentity   string
	Message        string
}

// Candidate 读取 Workspace 当前候选，不改变真实 index。
func (a Adapter) Candidate(ctx context.Context, workspace, expectedRevision string) (CandidateIdentity, error) {
	path, err := normalizeExistingDir(workspace)
	if err != nil || !validRevision(expectedRevision) {
		return CandidateIdentity{}, fmt.Errorf("candidate workspace: %w", ErrInvalidRequest)
	}
	head, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(head)) != expectedRevision {
		return CandidateIdentity{}, fmt.Errorf("candidate workspace head: %w", ErrRevisionMismatch)
	}
	branch, err := a.output(ctx, path, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) == "" {
		return CandidateIdentity{}, fmt.Errorf("candidate workspace branch: %w", ErrWorkspaceConflict)
	}
	diff, err := a.output(ctx, path, "diff", "--binary", "--no-ext-diff", "HEAD", "--")
	if err != nil {
		return CandidateIdentity{}, fmt.Errorf("candidate workspace diff: %w", ErrGitUnavailable)
	}
	status, err := a.output(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return CandidateIdentity{}, fmt.Errorf("candidate workspace status: %w", ErrGitUnavailable)
	}
	files, untracked, err := parseStatus(status)
	if err != nil {
		return CandidateIdentity{}, fmt.Errorf("candidate workspace status: %w", ErrGitUnavailable)
	}
	tree, err := a.candidateTree(ctx, path)
	if err != nil {
		return CandidateIdentity{}, err
	}
	return CandidateIdentity{HeadRevision: strings.TrimSpace(string(head)), Branch: strings.TrimSpace(string(branch)), TreeIdentity: tree, Diff: append([]byte(nil), diff...), ChangedFiles: files, HasUntracked: untracked}, nil
}

// Commit 执行一次无 hook 的受控 commit，并校验 direct parent、tree 和 clean 状态。
func (a Adapter) Commit(ctx context.Context, request CommitRequest) (CommitResult, error) {
	if ctx == nil || request.Message == "" || !validRevision(request.ExpectedParent) || !validRevision(request.ExpectedTree) {
		return CommitResult{}, fmt.Errorf("commit workspace: %w", ErrInvalidRequest)
	}
	if err := validateKeystoneCommitMessage(request.Message); err != nil {
		return CommitResult{}, err
	}
	path, err := normalizeExistingDir(request.WorkspacePath)
	if err != nil {
		return CommitResult{}, fmt.Errorf("commit workspace: %w", ErrInvalidRequest)
	}
	head, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(head)) != request.ExpectedParent {
		return CommitResult{}, fmt.Errorf("commit parent: %w", ErrRevisionMismatch)
	}
	if err := a.checkExpectedBranch(ctx, path, request.ExpectedBranch); err != nil {
		return CommitResult{}, err
	}
	status, err := a.output(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return CommitResult{}, fmt.Errorf("inspect commit workspace: %w", ErrGitUnavailable)
	}
	_, hasUntracked, err := parseStatus(status)
	if err != nil {
		return CommitResult{}, fmt.Errorf("inspect commit workspace: %w", ErrGitUnavailable)
	}
	if hasUntracked {
		return CommitResult{}, fmt.Errorf("commit workspace has residual untracked files: %w", ErrWorkspaceConflict)
	}
	currentTree, err := a.candidateTree(ctx, path)
	if err != nil {
		return CommitResult{}, fmt.Errorf("inspect commit candidate tree: %w", ErrGitUnavailable)
	}
	if currentTree != request.ExpectedTree {
		return CommitResult{}, fmt.Errorf("commit candidate tree: %w", ErrRevisionMismatch)
	}
	if _, err := a.runOutput(ctx, path, "add", "--all", "--"); err != nil {
		return CommitResult{}, fmt.Errorf("stage candidate: %w", ErrGitUnavailable)
	}
	tree, err := a.output(ctx, path, "write-tree")
	if err != nil || strings.TrimSpace(string(tree)) != request.ExpectedTree {
		return CommitResult{}, fmt.Errorf("commit tree: %w", ErrRevisionMismatch)
	}
	if _, err := a.output(ctx, path, "var", "GIT_AUTHOR_IDENT"); err != nil {
		return CommitResult{}, fmt.Errorf("commit identity: %w", ErrWorkspaceConflict)
	}
	if _, err := a.output(ctx, path, "var", "GIT_COMMITTER_IDENT"); err != nil {
		return CommitResult{}, fmt.Errorf("commit identity: %w", ErrWorkspaceConflict)
	}
	messageFile, err := os.CreateTemp("", "keystone-commit-message-")
	if err != nil {
		return CommitResult{}, fmt.Errorf("create commit message: %w", ErrGitUnavailable)
	}
	messagePath := messageFile.Name()
	defer os.Remove(messagePath)
	if _, err := messageFile.WriteString(request.Message); err != nil {
		_ = messageFile.Close()
		return CommitResult{}, fmt.Errorf("write commit message: %w", ErrGitUnavailable)
	}
	if err := messageFile.Close(); err != nil {
		return CommitResult{}, fmt.Errorf("close commit message: %w", ErrGitUnavailable)
	}
	if _, err := a.runOutput(ctx, path, "commit", "--no-verify", "-F", messagePath); err != nil {
		return CommitResult{}, fmt.Errorf("create commit: %w", ErrGitUnavailable)
	}
	after, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || !validRevision(strings.TrimSpace(string(after))) {
		return CommitResult{}, fmt.Errorf("read commit revision: %w", ErrGitUnavailable)
	}
	parent, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^1^{commit}")
	if err != nil || strings.TrimSpace(string(parent)) != request.ExpectedParent {
		return CommitResult{}, fmt.Errorf("verify commit parent: %w", ErrRevisionMismatch)
	}
	actualTree, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil || strings.TrimSpace(string(actualTree)) != request.ExpectedTree {
		return CommitResult{}, fmt.Errorf("verify commit tree: %w", ErrRevisionMismatch)
	}
	status, err = a.output(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil || len(status) != 0 {
		return CommitResult{}, fmt.Errorf("verify commit clean workspace: %w", ErrWorkspaceConflict)
	}
	actualMessage, err := a.output(ctx, path, "show", "-s", "--format=%B", "HEAD")
	if err != nil || strings.TrimSpace(string(actualMessage)) != strings.TrimSpace(request.Message) {
		return CommitResult{}, fmt.Errorf("verify commit message: %w", ErrRevisionMismatch)
	}
	if err := validateKeystoneCommitMessage(strings.TrimSpace(string(actualMessage))); err != nil {
		return CommitResult{}, fmt.Errorf("verify commit trailers: %w", ErrRevisionMismatch)
	}
	return CommitResult{GitOID: strings.TrimSpace(string(after)), ParentRevision: request.ExpectedParent, AfterRevision: strings.TrimSpace(string(after)), TreeIdentity: request.ExpectedTree, Message: request.Message}, nil
}

// ReconcileCommit 只对账 HEAD 的唯一直接结果，不搜索 reflog 或猜测其它候选。
// 返回 ErrCommitNotFound 时仍可在同一 CommitIntent 下尝试一次受控 Git 写入。
func (a Adapter) ReconcileCommit(ctx context.Context, request CommitRequest) (CommitResult, error) {
	if ctx == nil || request.Message == "" || !validRevision(request.ExpectedParent) || !validRevision(request.ExpectedTree) {
		return CommitResult{}, fmt.Errorf("reconcile commit: %w", ErrInvalidRequest)
	}
	if err := validateKeystoneCommitMessage(request.Message); err != nil {
		return CommitResult{}, err
	}
	path, err := normalizeExistingDir(request.WorkspacePath)
	if err != nil {
		return CommitResult{}, fmt.Errorf("reconcile commit: %w", ErrInvalidRequest)
	}
	if err := a.checkExpectedBranch(ctx, path, request.ExpectedBranch); err != nil {
		return CommitResult{}, err
	}
	head, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return CommitResult{}, fmt.Errorf("reconcile commit head: %w", ErrGitUnavailable)
	}
	after := strings.TrimSpace(string(head))
	if after == request.ExpectedParent {
		return CommitResult{}, ErrCommitNotFound
	}
	parent, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^1^{commit}")
	if err != nil || strings.TrimSpace(string(parent)) != request.ExpectedParent {
		return CommitResult{}, fmt.Errorf("reconcile commit parent: %w", ErrRevisionMismatch)
	}
	tree, err := a.output(ctx, path, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil || strings.TrimSpace(string(tree)) != request.ExpectedTree {
		return CommitResult{}, fmt.Errorf("reconcile commit tree: %w", ErrRevisionMismatch)
	}
	status, err := a.output(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil || len(status) != 0 {
		return CommitResult{}, fmt.Errorf("reconcile commit clean workspace: %w", ErrWorkspaceConflict)
	}
	message, err := a.output(ctx, path, "show", "-s", "--format=%B", "HEAD")
	if err != nil || strings.TrimSpace(string(message)) != strings.TrimSpace(request.Message) {
		return CommitResult{}, fmt.Errorf("reconcile commit message: %w", ErrRevisionMismatch)
	}
	if err := validateKeystoneCommitMessage(strings.TrimSpace(string(message))); err != nil {
		return CommitResult{}, fmt.Errorf("reconcile commit trailers: %w", ErrRevisionMismatch)
	}
	return CommitResult{GitOID: after, ParentRevision: request.ExpectedParent, AfterRevision: after, TreeIdentity: request.ExpectedTree, Message: request.Message}, nil
}

func (a Adapter) checkExpectedBranch(ctx context.Context, path, expected string) error {
	if expected == "" {
		return nil
	}
	branch, err := a.output(ctx, path, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) != expected {
		return fmt.Errorf("commit branch: %w", ErrBranchConflict)
	}
	return nil
}

func validateKeystoneCommitMessage(message string) error {
	counts := map[string]int{
		"Keystone-Change-ID": 0,
		"Keystone-Ticket-ID": 0,
		"Keystone-Commit-ID": 0,
	}
	for _, line := range strings.Split(strings.TrimSpace(message), "\n") {
		for name := range counts {
			if strings.HasPrefix(line, name+":") {
				if strings.TrimSpace(strings.TrimPrefix(line, name+":")) == "" {
					return fmt.Errorf("commit trailer %s is empty: %w", name, ErrInvalidRequest)
				}
				counts[name]++
			}
		}
	}
	for name, count := range counts {
		if count != 1 {
			return fmt.Errorf("commit trailer %s must occur exactly once: %w", name, ErrInvalidRequest)
		}
	}
	return nil
}

func (a Adapter) candidateTree(ctx context.Context, path string) (string, error) {
	file, err := os.CreateTemp("", "keystone-candidate-index-")
	if err != nil {
		return "", fmt.Errorf("create candidate index: %w", ErrGitUnavailable)
	}
	indexPath := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(indexPath)
		return "", fmt.Errorf("close candidate index: %w", ErrGitUnavailable)
	}
	defer os.Remove(indexPath)
	if _, err := a.runWithIndex(ctx, path, indexPath, "read-tree", "HEAD"); err != nil {
		return "", fmt.Errorf("prepare candidate index: %w", ErrGitUnavailable)
	}
	if _, err := a.runWithIndex(ctx, path, indexPath, "add", "--all", "--"); err != nil {
		return "", fmt.Errorf("stage candidate index: %w", ErrGitUnavailable)
	}
	tree, err := a.outputWithIndex(ctx, path, indexPath, "write-tree")
	if err != nil || !validRevision(strings.TrimSpace(string(tree))) {
		return "", fmt.Errorf("write candidate tree: %w", ErrGitUnavailable)
	}
	return strings.TrimSpace(string(tree)), nil
}

func (a Adapter) runOutput(ctx context.Context, path string, args ...string) ([]byte, error) {
	_, output, err := a.run(ctx, path, args...)
	return output, err
}

func (a Adapter) runWithIndex(ctx context.Context, path, index string, args ...string) ([]byte, error) {
	command := a.command(ctx, path, args...)
	command.Env = append(gitEnvironment(), "GIT_INDEX_FILE="+index)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func (a Adapter) outputWithIndex(ctx context.Context, path, index string, args ...string) ([]byte, error) {
	return a.runWithIndex(ctx, path, index, args...)
}

// command 构造固定 git -C 调用，避免把 workspace 内容交给 shell。
func (a Adapter) command(ctx context.Context, path string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, a.git(), append([]string{"-C", path}, args...)...)
}

// Package domain 定义 Ticket 09 的纯执行领域对象。
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DefaultBranchPrefix    = "keystone/change/"
	DefaultRuntime         = "codex"
	DefaultExecutionMode   = "edit"
	DefaultTimeout         = 30 * time.Minute
	MaxInstructionBytes    = 64 << 10
	ExecutionPolicyVersion = "DefaultExecutionAuthorizationPolicy.v1"
	SessionStatusWaiting   = "waiting"
	SessionStatusRunning   = "running"
	SessionStatusHuman     = "human_required"
	SessionStatusCancelled = "cancelled"
	SessionStatusCompleted = "completed"
	EpochStatusQueued      = "queued"
	EpochStatusActive      = "active"
	EpochStatusFenced      = "fenced"
	EpochStatusCompleted   = "completed"
	TicketStatePending     = "pending"
	TicketStateAssigned    = "assigned"
	TicketStateSucceeded   = "succeeded"
	TicketStateHuman       = "human_required"
)

var branchNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

var (
	ErrInvalid  = errors.New("invalid execution value")
	ErrConflict = errors.New("execution value conflicts with current authority")
	ErrFenced   = errors.New("execution value is fenced")
)

// Workspace 表示已经由 SourceControl 核验过的 Change Worktree 身份。
type Workspace struct {
	ID             string
	ProjectID      string
	ChangeID       string
	RepositoryRoot string
	Path           string
	PhysicalPath   string
	Branch         string
	BaseRevision   string
	InputRevision  string
}

// Validate 校验 Workspace 的路径、branch 和不可变输入 revision。
func (w Workspace) Validate() error {
	if strings.TrimSpace(w.ID) == "" || strings.TrimSpace(w.ProjectID) == "" || strings.TrimSpace(w.ChangeID) == "" {
		return fmt.Errorf("%w: workspace identity is required", ErrInvalid)
	}
	for name, value := range map[string]string{"repository_root": w.RepositoryRoot, "workspace_path": w.Path, "physical_path": w.PhysicalPath} {
		if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return fmt.Errorf("%w: %s must be a normalized absolute path", ErrInvalid, name)
		}
	}
	if !validRevision(w.BaseRevision) || w.InputRevision != w.BaseRevision {
		return fmt.Errorf("%w: workspace revision is invalid", ErrInvalid)
	}
	if err := ValidateBranch(w.Branch); err != nil {
		return err
	}
	return nil
}

// WorkspaceProvisioningIntent 是 Git 写操作之前落盘的恢复事实。
type WorkspaceProvisioningIntent struct {
	ID             string
	ProjectID      string
	ChangeID       string
	RepositoryRoot string
	WorkspacePath  string
	BaseRevision   string
	Branch         string
	RequestKey     string
	RequestDigest  string
	Status         string
}

// Validate 检查 provisioning intent 是否足以唯一描述一次 Execute。
func (i WorkspaceProvisioningIntent) Validate() error {
	if strings.TrimSpace(i.ID) == "" || strings.TrimSpace(i.ProjectID) == "" || strings.TrimSpace(i.ChangeID) == "" || strings.TrimSpace(i.RequestKey) == "" {
		return fmt.Errorf("%w: provisioning intent identity is required", ErrInvalid)
	}
	if i.WorkspacePath == "" || !filepath.IsAbs(i.WorkspacePath) || filepath.Clean(i.WorkspacePath) != i.WorkspacePath || !validRevision(i.BaseRevision) {
		return fmt.Errorf("%w: provisioning intent path or revision is invalid", ErrInvalid)
	}
	if err := ValidateBranch(i.Branch); err != nil {
		return err
	}
	if i.RequestDigest == "" {
		return fmt.Errorf("%w: provisioning request digest is required", ErrInvalid)
	}
	return nil
}

// ExecutionSession 是 Change 唯一的执行会话摘要。
type ExecutionSession struct {
	ID             string
	ProjectID      string
	ChangeID       string
	Status         string
	Branch         string
	BaseRevision   string
	InputRevision  string
	WorkspaceID    string
	RequestKey     string
	RequestDigest  string
	DispatchSeq    int64
	CurrentEpochID string
}

// Validate 校验会话状态及 Ticket 09 的全局输入 revision 不变量。
func (s ExecutionSession) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.ProjectID) == "" || strings.TrimSpace(s.ChangeID) == "" || s.DispatchSeq < 1 {
		return fmt.Errorf("%w: execution session identity is invalid", ErrInvalid)
	}
	if !validSessionStatus(s.Status) || !validRevision(s.BaseRevision) || s.InputRevision != s.BaseRevision {
		return fmt.Errorf("%w: execution session state is invalid", ErrInvalid)
	}
	if err := ValidateBranch(s.Branch); err != nil {
		return err
	}
	return nil
}

// DispatchEpoch 是一次有序、可围栏的调度代次。
type DispatchEpoch struct {
	ID        string
	SessionID string
	Sequence  int64
	Status    string
}

// Validate 校验 DispatchEpoch 的单调序号和状态。
func (e DispatchEpoch) Validate() error {
	if e.ID == "" || e.SessionID == "" || e.Sequence < 1 || !validEpochStatus(e.Status) {
		return fmt.Errorf("%w: dispatch epoch is invalid", ErrInvalid)
	}
	return nil
}

// ExecutionAuthorization 是发放 Assignment 前的结构性授权事实。
type ExecutionAuthorization struct {
	ID             string
	SessionID      string
	EpochID        string
	TicketID       string
	PolicyVersion  string
	ExecutionMode  string
	Runtime        string
	Instruction    string
	InputDigest    string
	Timeout        time.Duration
	WorkspaceID    string
	Branch         string
	InputRevision  string
	DecisionDigest string
	Status         string
}

// Validate 校验默认策略的合取输入和有界指令。
func (a ExecutionAuthorization) Validate() error {
	if a.ID == "" || a.SessionID == "" || a.EpochID == "" || a.TicketID == "" || a.PolicyVersion != ExecutionPolicyVersion {
		return fmt.Errorf("%w: authorization identity or policy is invalid", ErrInvalid)
	}
	if a.ExecutionMode != DefaultExecutionMode || a.Runtime != DefaultRuntime || a.Timeout != DefaultTimeout || a.WorkspaceID == "" || !validRevision(a.InputRevision) {
		return fmt.Errorf("%w: authorization execution envelope is invalid", ErrInvalid)
	}
	if len([]byte(a.Instruction)) == 0 || len([]byte(a.Instruction)) > MaxInstructionBytes {
		return fmt.Errorf("%w: authorization instruction is out of bounds", ErrInvalid)
	}
	if err := ValidateBranch(a.Branch); err != nil {
		return err
	}
	return nil
}

// WorkspaceSnapshot 是同一 WorkspaceInputRevision 下的一次 Git 观察。
type WorkspaceSnapshot struct {
	ID            string
	WorkspaceID   string
	TicketID      string
	Phase         string
	InputRevision string
	HeadRevision  string
	Branch        string
	ChangedFiles  []string
	DiffSHA256    string
	DiffBytes     int64
	HasUntracked  bool
}

// Validate 校验 Snapshot 的 phase、revision 和相对文件路径。
func (s WorkspaceSnapshot) Validate() error {
	if s.ID == "" || s.WorkspaceID == "" || s.TicketID == "" || (s.Phase != "pre" && s.Phase != "post") || !validRevision(s.InputRevision) || s.HeadRevision != s.InputRevision || s.Branch == "" {
		return fmt.Errorf("%w: workspace snapshot is invalid", ErrInvalid)
	}
	for _, file := range s.ChangedFiles {
		if file == "" || filepath.IsAbs(file) || filepath.Clean(file) != file || file == "." || strings.HasPrefix(file, ".."+string(filepath.Separator)) || file == ".." {
			return fmt.Errorf("%w: snapshot changed file is not relative", ErrInvalid)
		}
	}
	return nil
}

// TicketDelta 描述一张 Ticket 对完整 Workspace Diff 的规范化投影。
type TicketDelta struct {
	TicketID       string
	ChangedFiles   []string
	CompleteDiff   string
	CompleteSHA256 string
}

// Validate 要求成功证据具有非空、可复算的完整 Diff 和 Delta。
func (d TicketDelta) Validate() error {
	if d.TicketID == "" || len(d.ChangedFiles) == 0 || d.CompleteDiff == "" || d.CompleteSHA256 == "" {
		return fmt.Errorf("%w: ticket delta must be non-empty", ErrInvalid)
	}
	for _, file := range d.ChangedFiles {
		if filepath.IsAbs(file) || filepath.Clean(file) != file || file == "." || file == ".." || strings.HasPrefix(file, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: ticket delta file is not relative", ErrInvalid)
		}
	}
	digest := sha256.Sum256([]byte(d.CompleteDiff))
	if !strings.EqualFold(d.CompleteSHA256, hex.EncodeToString(digest[:])) {
		return fmt.Errorf("%w: ticket delta digest does not match content", ErrInvalid)
	}
	return nil
}

// TicketExecutionEvidence 是一张 Ticket 的不可变双重 Diff 证据摘要。
type TicketExecutionEvidence struct {
	ID             string
	SessionID      string
	EpochID        string
	TicketID       string
	AgentRunID     string
	LeaseID        string
	InputRevision  string
	PreSnapshotID  string
	PostSnapshotID string
	Delta          TicketDelta
	Outcome        string
}

// Validate 校验 Evidence 的引用闭包和成功结果。
func (e TicketExecutionEvidence) Validate() error {
	if e.ID == "" || e.SessionID == "" || e.EpochID == "" || e.TicketID == "" || e.AgentRunID == "" || e.LeaseID == "" || e.PreSnapshotID == "" || e.PostSnapshotID == "" || !validRevision(e.InputRevision) {
		return fmt.Errorf("%w: ticket execution evidence identity is incomplete", ErrInvalid)
	}
	if err := e.Delta.Validate(); err != nil {
		return err
	}
	if e.Outcome != "succeeded" {
		return fmt.Errorf("%w: evidence outcome is not successful", ErrInvalid)
	}
	return nil
}

// ValidateBranch 使用 SourceControl 之外的纯结构规则约束用户 branch。
func ValidateBranch(branch string) error {
	if branch == "" || !branchNamePattern.MatchString(branch) || strings.HasPrefix(branch, "refs/") || strings.Contains(branch, "..") || strings.Contains(branch, "//") || strings.HasSuffix(branch, ".") || strings.HasSuffix(branch, "/") {
		return fmt.Errorf("%w: branch is invalid", ErrInvalid)
	}
	return nil
}

// DefaultBranch 为 Change 生成可追溯的默认 branch。
func DefaultBranch(changeID string) string { return DefaultBranchPrefix + changeID }

// RequestDigest 为规范化 Execute 输入生成稳定摘要。
func RequestDigest(changeID string, expectedVersion int64, key, branch string) string {
	value := fmt.Sprintf("%s\x00%d\x00%s\x00%s", changeID, expectedVersion, key, branch)
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// NormalizeFiles 返回排序、去重后的相对路径集合。
func NormalizeFiles(files []string) []string {
	seen := make(map[string]struct{}, len(files))
	result := make([]string, 0, len(files))
	for _, file := range files {
		file = filepath.Clean(strings.TrimSpace(file))
		if file == "." || file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}

func validRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func validSessionStatus(value string) bool {
	switch value {
	case SessionStatusWaiting, SessionStatusRunning, SessionStatusHuman, SessionStatusCancelled, SessionStatusCompleted:
		return true
	default:
		return false
	}
}

func validEpochStatus(value string) bool {
	switch value {
	case EpochStatusQueued, EpochStatusActive, EpochStatusFenced, EpochStatusCompleted:
		return true
	default:
		return false
	}
}

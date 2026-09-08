// Package application 编排 Ticket 09 的 Execute 用例。
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	executiondomain "github.com/disturb-yy/keystone/internal/execution/domain"
)

var (
	ErrIdempotencyConflict  = errors.New("execution idempotency key conflicts")
	ErrExecutionInProgress  = errors.New("execution is already in progress")
	ErrExecutionUnavailable = errors.New("execution persistence is unavailable")
)

// ExecuteRequest 是已经完成边界参数校验的 Execute 输入。
type ExecuteRequest struct {
	ProjectID       string
	ChangeID        string
	ExpectedVersion int64
	RepositoryRoot  string
	BaseRevision    string
	WorkspacePath   string
	Branch          string
	RequestKey      string
	RequestDigest   string
	Instruction     string
}

// ProvisioningRequest 是 SourceControl 看到的最小、结构化请求。
type ProvisioningRequest struct {
	RepositoryRoot string
	WorkspacePath  string
	Branch         string
	BaseRevision   string
}

// ProvisionedWorkspace 是 SourceControl 返回的内部身份；不得直接编码到公开 DTO。
type ProvisionedWorkspace struct {
	WorkspacePath string
	PhysicalPath  string
	Branch        string
	HeadRevision  string
}

// ExecutionSession 是公开安全的会话摘要。
type ExecutionSession struct {
	ID            string
	ChangeID      string
	Status        string
	Branch        string
	BaseRevision  string
	InputRevision string
	WorkspaceID   string
	Tickets       []TicketState
}

// TicketState 是 Canonical Ticket 的执行状态摘要。
type TicketState struct {
	TicketID string
	Ordinal  int
	Title    string
	State    string
}

// Persistence 是 Execute 的权威状态 Port。
type Persistence interface {
	BeginExecution(context.Context, ExecuteRequest) (BeginResult, error)
	FinalizeExecution(context.Context, FinalizeRequest) (ExecutionSession, error)
	FindExecution(context.Context, string) (ExecutionSession, error)
}

// SourceControl 是受约束的 Worktree Port。
type SourceControl interface {
	Provision(context.Context, ProvisioningRequest) (ProvisionedWorkspace, error)
}

// BeginResult 描述 provisioning intent 的耐久化结果。
type BeginResult struct {
	IntentID       string
	Session        ExecutionSession
	NeedsProvision bool
}

// FinalizeRequest 将 SourceControl 已验证的身份收敛到 Session。
type FinalizeRequest struct {
	ExecuteRequest
	IntentID      string
	WorkspaceID   string
	PhysicalPath  string
	HeadRevision  string
	WorkspacePath string
}

// Service 实现“先 intent、后 Git、再 finalization”的 Execute 顺序。
type Service struct {
	Persistence Persistence
	Source      SourceControl
	Now         func() time.Time
}

// Execute 创建或恢复一个 Change Worktree 会话，不启动 Runtime。
func (s Service) Execute(ctx context.Context, request ExecuteRequest) (ExecutionSession, error) {
	if ctx == nil || s.Persistence == nil || s.Source == nil {
		return ExecutionSession{}, fmt.Errorf("execute change: %w", ErrExecutionUnavailable)
	}
	if err := validateRequest(request); err != nil {
		return ExecutionSession{}, err
	}
	begin, err := s.Persistence.BeginExecution(ctx, request)
	if err != nil {
		return ExecutionSession{}, err
	}
	if !begin.NeedsProvision {
		return begin.Session, nil
	}
	workspace, err := s.Source.Provision(ctx, ProvisioningRequest{RepositoryRoot: request.RepositoryRoot, WorkspacePath: request.WorkspacePath, Branch: request.Branch, BaseRevision: request.BaseRevision})
	if err != nil {
		return ExecutionSession{}, err
	}
	if workspace.Branch != request.Branch || workspace.HeadRevision != request.BaseRevision || workspace.WorkspacePath != request.WorkspacePath || workspace.PhysicalPath == "" {
		return ExecutionSession{}, fmt.Errorf("execute change: %w", executiondomain.ErrConflict)
	}
	return s.Persistence.FinalizeExecution(ctx, FinalizeRequest{ExecuteRequest: request, IntentID: begin.IntentID, WorkspaceID: begin.Session.WorkspaceID, PhysicalPath: workspace.PhysicalPath, HeadRevision: workspace.HeadRevision, WorkspacePath: workspace.WorkspacePath})
}

func validateRequest(request ExecuteRequest) error {
	if strings.TrimSpace(request.ProjectID) == "" || strings.TrimSpace(request.ChangeID) == "" || strings.TrimSpace(request.RequestKey) == "" || request.RequestDigest == "" {
		return fmt.Errorf("execute change: %w", executiondomain.ErrInvalid)
	}
	if request.ExpectedVersion < 1 {
		return fmt.Errorf("execute change: %w", executiondomain.ErrInvalid)
	}
	if request.RepositoryRoot == "" || request.WorkspacePath == "" || request.BaseRevision == "" {
		return fmt.Errorf("execute change: %w", executiondomain.ErrInvalid)
	}
	if err := executiondomain.ValidateBranch(request.Branch); err != nil {
		return err
	}
	if len([]byte(request.Instruction)) == 0 || len([]byte(request.Instruction)) > executiondomain.MaxInstructionBytes {
		return fmt.Errorf("execute change: %w", executiondomain.ErrInvalid)
	}
	return nil
}

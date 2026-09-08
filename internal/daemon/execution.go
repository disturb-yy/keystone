package daemon

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	executionapp "github.com/disturb-yy/keystone/internal/execution/application"
	executiondomain "github.com/disturb-yy/keystone/internal/execution/domain"
	"github.com/disturb-yy/keystone/internal/infrastructure/sourcecontrol"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

type daemonSourceControl struct {
	adapter sourcecontrol.Adapter
}

func (s daemonSourceControl) Provision(ctx context.Context, request executionapp.ProvisioningRequest) (executionapp.ProvisionedWorkspace, error) {
	result, err := s.adapter.Provision(ctx, sourcecontrol.ProvisionRequest{RepositoryRoot: request.RepositoryRoot, WorkspacePath: request.WorkspacePath, Branch: request.Branch, BaseRevision: request.BaseRevision})
	if err != nil {
		return executionapp.ProvisionedWorkspace{}, err
	}
	return executionapp.ProvisionedWorkspace{WorkspacePath: result.WorkspacePath, PhysicalPath: result.PhysicalPath, Branch: result.Branch, HeadRevision: result.HeadRevision}, nil
}

func (s *Server) handleChangeExecute(w http.ResponseWriter, r *http.Request, service *work.ChangeService, changeID domain.ChangeID) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	request, err := decodeStrictJSON(r, controlplane.ChangeExecuteRequest{})
	if err != nil || request.ExpectedVersion < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "execute request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "change service is unavailable")
		return
	}
	change, err := service.Show(r.Context(), changeID)
	if err != nil {
		writeChangeError(w, err)
		return
	}
	branch := strings.TrimSpace(request.WorkspaceBranch)
	if branch == "" {
		branch = executiondomain.DefaultBranch(string(change.ID))
	}
	if err := executiondomain.ValidateBranch(branch); err != nil {
		writeExecutionError(w, err)
		return
	}
	s.mu.RLock()
	executionService, paths := s.execution, s.paths
	s.mu.RUnlock()
	if executionService == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "execution service is unavailable")
		return
	}
	workspacePath := filepath.Join(paths.WorkspacesDir, string(change.ProjectID), string(change.ID))
	workspacePath = filepath.Clean(workspacePath)
	requestDigest := executiondomain.RequestDigest(string(change.ID), int64(request.ExpectedVersion), key.String(), branch)
	session, err := executionService.Execute(r.Context(), executionapp.ExecuteRequest{
		ProjectID: string(change.ProjectID), ChangeID: string(change.ID), ExpectedVersion: int64(request.ExpectedVersion),
		RepositoryRoot: change.RepositoryRoot, BaseRevision: change.BaseRevision, WorkspacePath: workspacePath,
		Branch: branch, RequestKey: key.String(), RequestDigest: requestDigest, Instruction: change.Intent.Summary,
	})
	if err != nil {
		writeExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, controlplane.ChangeExecuteResponse{Change: changeDTO(change), Execution: executionReadModelDTO(session)})
}

func (s *Server) handleChangeExecution(w http.ResponseWriter, r *http.Request, changeID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	s.mu.RLock()
	service := s.execution
	s.mu.RUnlock()
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "execution service is unavailable")
		return
	}
	session, err := service.Persistence.FindExecution(r.Context(), changeID)
	if err != nil {
		writeExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionReadModelDTO(session))
}

func executionReadModelDTO(session executionapp.ExecutionSession) controlplane.ExecutionReadModel {
	tickets := make([]controlplane.ExecutionTicketDTO, 0, len(session.Tickets))
	for _, ticket := range session.Tickets {
		tickets = append(tickets, controlplane.ExecutionTicketDTO{TicketID: ticket.TicketID, Ordinal: ticket.Ordinal, Title: ticket.Title, State: ticket.State})
	}
	return controlplane.ExecutionReadModel{ExecutionSessionID: session.ID, ChangeID: session.ChangeID, Status: session.Status, WorkspaceBranch: session.Branch, BaseRevision: session.BaseRevision, InputRevision: session.InputRevision, Tickets: tickets}
}

func writeExecutionError(w http.ResponseWriter, err error) {
	code, status, message := "unavailable", http.StatusServiceUnavailable, "execution operation is temporarily unavailable"
	switch {
	case errors.Is(err, executiondomain.ErrInvalid):
		code, status, message = "invalid_request", http.StatusBadRequest, "execution request is invalid"
	case errors.Is(err, executionapp.ErrExecutionInProgress):
		code, status, message = "execution_in_progress", http.StatusConflict, "an execution session is already active"
	case errors.Is(err, executiondomain.ErrConflict), errors.Is(err, executionapp.ErrIdempotencyConflict), errors.Is(err, domain.ErrChangeVersionConflict):
		code, status, message = "execution_conflict", http.StatusConflict, "execution request conflicts with current authority"
	case errors.Is(err, domain.ErrChangeNotFound):
		code, status, message = "change_not_found", http.StatusNotFound, "change was not found"
	case errors.Is(err, domain.ErrTicketGraphNotFound):
		code, status, message = "ticket_graph_not_found", http.StatusNotFound, "ticket graph was not found"
	case errors.Is(err, sourcecontrol.ErrRepositoryDirty):
		code, status, message = "repository_dirty", http.StatusConflict, "repository has uncommitted changes"
	case errors.Is(err, sourcecontrol.ErrRevisionMismatch), errors.Is(err, sourcecontrol.ErrBranchConflict), errors.Is(err, sourcecontrol.ErrWorkspaceConflict), errors.Is(err, executiondomain.ErrFenced):
		code, status, message = "workspace_conflict", http.StatusConflict, "workspace identity conflicts with current authority"
	case errors.Is(err, sourcecontrol.ErrUnsupportedRepository):
		code, status, message = "repository_unsupported", http.StatusConflict, "repository topology is unsupported"
	}
	writeError(w, status, code, message)
}

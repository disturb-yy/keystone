package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/sourcecontrol"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func (s *Server) handleChangeTicketVerify(w http.ResponseWriter, r *http.Request, service *work.ChangeService, changeID, ticketID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	request, err := decodeStrictJSON(r, controlplane.ChangeVerifyRequest{})
	if err != nil || request.ExpectedVersion < 1 || strings.TrimSpace(ticketID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "verify request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	change, err := service.Show(r.Context(), domain.ChangeID(changeID))
	if err != nil {
		writeChangeError(w, err)
		return
	}
	_, candidate, err := s.candidateForTicket(r.Context(), change, ticketID)
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	s.mu.RLock()
	state := s.workerStore
	s.mu.RUnlock()
	if state == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "verification authority is unavailable")
		return
	}
	diffDigest := sha256.Sum256(candidate.Diff)
	result, err := state.BeginVerification(r.Context(), workstore.VerificationIntentRequest{ProjectID: string(change.ProjectID), ChangeID: string(change.ID), TicketID: ticketID, ExpectedVersion: request.ExpectedVersion, RequestKey: key.String(), RequestDigest: governanceRequestDigest("verify", string(change.ID), ticketID, request.ExpectedVersion, key.String()), InputRevision: candidate.HeadRevision, CandidateTreeIdentity: candidate.TreeIdentity, SnapshotBranch: candidate.Branch, SnapshotChangedFiles: candidate.ChangedFiles, SnapshotDiffSHA256: hex.EncodeToString(diffDigest[:]), SnapshotDiffBytes: int64(len(candidate.Diff)), SnapshotHasUntracked: candidate.HasUntracked})
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	readModel, err := state.ReadExecution(r.Context(), string(change.ID))
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	updated, err := service.Show(r.Context(), domain.ChangeID(changeID))
	if err != nil {
		writeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, controlplane.ChangeVerifyResponse{Change: changeDTO(updated), Execution: executionReadModelStoreDTO(readModel), IntentID: result.IntentID, VerificationStatus: result.Status})
}

func (s *Server) handleChangeTicketCommit(w http.ResponseWriter, r *http.Request, service *work.ChangeService, changeID, ticketID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	request, err := decodeStrictJSON(r, controlplane.ChangeCommitRequest{})
	if err != nil || request.ExpectedVersion < 1 || strings.TrimSpace(ticketID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "commit request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	change, err := service.Show(r.Context(), domain.ChangeID(changeID))
	if err != nil {
		writeChangeError(w, err)
		return
	}
	_, candidate, err := s.candidateForTicket(r.Context(), change, ticketID)
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	if candidate.HasUntracked {
		writeGovernanceError(w, sourcecontrol.ErrWorkspaceConflict)
		return
	}
	s.mu.RLock()
	state, adapter := s.workerStore, s.sourceControl
	s.mu.RUnlock()
	if state == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "commit authority is unavailable")
		return
	}
	precondition, err := state.PrepareCommitIntent(r.Context(), workstore.CommitIntentRequest{ProjectID: string(change.ProjectID), ChangeID: string(change.ID), TicketID: ticketID, ExpectedVersion: request.ExpectedVersion, RequestKey: key.String(), RequestDigest: governanceRequestDigest("commit", string(change.ID), ticketID, request.ExpectedVersion, key.String()), InputRevision: candidate.HeadRevision, CandidateTreeIdentity: candidate.TreeIdentity})
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	commitRequest := sourcecontrol.CommitRequest{WorkspacePath: precondition.WorkspacePath, ExpectedBranch: precondition.Branch, ExpectedParent: precondition.ExpectedParent, ExpectedTree: precondition.CandidateTreeIdentity, Message: precondition.Message}
	var result sourcecontrol.CommitResult
	if precondition.Status == "committed" {
		result = sourcecontrol.CommitResult{GitOID: precondition.GitOID, ParentRevision: precondition.ExpectedParent, AfterRevision: precondition.GitOID, TreeIdentity: precondition.CandidateTreeIdentity, Message: precondition.Message}
	} else {
		var reconcileErr error
		result, reconcileErr = adapter.ReconcileCommit(r.Context(), commitRequest)
		if errors.Is(reconcileErr, sourcecontrol.ErrCommitNotFound) {
			result, reconcileErr = adapter.Commit(r.Context(), commitRequest)
		}
		if reconcileErr != nil {
			writeGovernanceError(w, reconcileErr)
			return
		}
	}
	if err := state.FinalizeCommit(r.Context(), workstore.FinalizeCommitRequest{IntentID: precondition.IntentID, GitOID: result.GitOID, ParentRevision: result.ParentRevision, AfterRevision: result.AfterRevision, TreeIdentity: result.TreeIdentity, Message: result.Message}); err != nil {
		writeGovernanceError(w, err)
		return
	}
	readModel, err := state.ReadExecution(r.Context(), string(change.ID))
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	updated, err := service.Show(r.Context(), domain.ChangeID(changeID))
	if err != nil {
		writeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, controlplane.ChangeCommitResponse{Change: changeDTO(updated), Execution: executionReadModelStoreDTO(readModel), KeystoneCommitID: precondition.KeystoneCommitID, GitOID: result.GitOID, ParentRevision: result.ParentRevision, AfterRevision: result.AfterRevision})
}

func (s *Server) handleChangeFinalVerify(w http.ResponseWriter, r *http.Request, service *work.ChangeService, changeID domain.ChangeID) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	request, err := decodeStrictJSON(r, controlplane.ChangeFinalVerifyRequest{})
	if err != nil || request.ExpectedVersion < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "final verify request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	change, err := service.Show(r.Context(), changeID)
	if err != nil {
		writeChangeError(w, err)
		return
	}
	s.mu.RLock()
	state, paths, adapter := s.workerStore, s.paths, s.sourceControl
	s.mu.RUnlock()
	if state == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "verification authority is unavailable")
		return
	}
	model, err := state.ReadExecution(r.Context(), string(change.ID))
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	workspace := filepath.Join(paths.WorkspacesDir, string(change.ProjectID), string(change.ID))
	candidate, err := adapter.Candidate(r.Context(), workspace, model.InputRevision)
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	if candidate.Branch != model.WorkspaceBranch || candidate.HasUntracked || len(candidate.ChangedFiles) != 0 || len(candidate.Diff) != 0 {
		writeGovernanceError(w, sourcecontrol.ErrWorkspaceConflict)
		return
	}
	diffDigest := sha256.Sum256(candidate.Diff)
	result, err := state.BeginVerification(r.Context(), workstore.VerificationIntentRequest{ProjectID: string(change.ProjectID), ChangeID: string(change.ID), ExpectedVersion: request.ExpectedVersion, RequestKey: key.String(), RequestDigest: governanceRequestDigest("final_verify", string(change.ID), "", request.ExpectedVersion, key.String()), InputRevision: candidate.HeadRevision, CandidateTreeIdentity: candidate.TreeIdentity, Final: true, SnapshotBranch: candidate.Branch, SnapshotChangedFiles: candidate.ChangedFiles, SnapshotDiffSHA256: hex.EncodeToString(diffDigest[:]), SnapshotDiffBytes: int64(len(candidate.Diff)), SnapshotHasUntracked: candidate.HasUntracked})
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	model, err = state.ReadExecution(r.Context(), string(change.ID))
	if err != nil {
		writeGovernanceError(w, err)
		return
	}
	updated, err := service.Show(r.Context(), changeID)
	if err != nil {
		writeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, controlplane.ChangeFinalVerifyResponse{Change: changeDTO(updated), Execution: executionReadModelStoreDTO(model), IntentID: result.IntentID, VerificationStatus: result.Status})
}

func (s *Server) candidateForTicket(ctx context.Context, change domain.Change, ticketID string) (workstore.ExecutionReadModel, sourcecontrol.CandidateIdentity, error) {
	s.mu.RLock()
	state, paths, adapter := s.workerStore, s.paths, s.sourceControl
	s.mu.RUnlock()
	if state == nil {
		return workstore.ExecutionReadModel{}, sourcecontrol.CandidateIdentity{}, workstore.ErrVerificationUnavailable
	}
	model, err := state.ReadExecution(ctx, string(change.ID))
	if err != nil {
		return workstore.ExecutionReadModel{}, sourcecontrol.CandidateIdentity{}, err
	}
	inputRevision := model.InputRevision
	for _, ticket := range model.Tickets {
		if ticket.TicketID == ticketID {
			if ticket.InputRevision != "" {
				inputRevision = ticket.InputRevision
			}
			break
		}
	}
	workspace := filepath.Join(paths.WorkspacesDir, string(change.ProjectID), string(change.ID))
	candidate, err := adapter.Candidate(ctx, workspace, inputRevision)
	return model, candidate, err
}

func executionReadModelStoreDTO(model workstore.ExecutionReadModel) controlplane.ExecutionReadModel {
	tickets := make([]controlplane.ExecutionTicketDTO, 0, len(model.Tickets))
	for _, ticket := range model.Tickets {
		tickets = append(tickets, controlplane.ExecutionTicketDTO{TicketID: ticket.TicketID, Ordinal: ticket.Ordinal, Title: ticket.Title, State: ticket.State, GateStatus: ticket.GateStatus, VerificationStatus: ticket.VerificationStatus, CommitStatus: ticket.CommitStatus, VerificationEvidenceID: ticket.VerificationEvidenceID, VerificationCommandStatuses: ticket.VerificationCommandStatuses, VerificationCriterionOutcomes: ticket.VerificationCriterionOutcomes, InputRevision: ticket.InputRevision, CommitBeforeRevision: ticket.CommitBeforeRevision, CommitAfterRevision: ticket.CommitAfterRevision})
	}
	return controlplane.ExecutionReadModel{ExecutionSessionID: model.ExecutionSessionID, ChangeID: model.ChangeID, Status: model.Status, WorkspaceBranch: model.WorkspaceBranch, BaseRevision: model.BaseRevision, InputRevision: model.InputRevision, Tickets: tickets, Stage: model.Stage, ChangeStatus: model.ChangeStatus, Version: model.Version, PendingIntentIDs: model.PendingIntentIDs, CandidateRevision: model.CandidateRevision, FinalVerification: model.FinalVerification, FinalEvidenceID: model.FinalEvidenceID}
}

func governanceRequestDigest(operation, changeID, ticketID string, version int, key string) string {
	digest := sha256.Sum256([]byte(operation + "\x00" + changeID + "\x00" + ticketID + "\x00" + strconv.Itoa(version) + "\x00" + key))
	return hex.EncodeToString(digest[:])
}

func writeGovernanceError(w http.ResponseWriter, err error) {
	code, status, message := "unavailable", http.StatusServiceUnavailable, "governance operation is temporarily unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalidRequest):
		code, status, message = "invalid_request", http.StatusBadRequest, "governance request is invalid"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, status, message = "idempotency_conflict", http.StatusConflict, "Idempotency-Key conflicts with an existing governance request"
	case errors.Is(err, workstore.ErrVerificationConflict), errors.Is(err, workstore.ErrCommitUnavailable), errors.Is(err, domain.ErrChangeVersionConflict), errors.Is(err, sourcecontrol.ErrRevisionMismatch), errors.Is(err, sourcecontrol.ErrBranchConflict), errors.Is(err, sourcecontrol.ErrWorkspaceConflict):
		code, status, message = "governance_conflict", http.StatusConflict, "governance request conflicts with current authority"
	case errors.Is(err, domain.ErrChangeNotFound):
		code, status, message = "change_not_found", http.StatusNotFound, "change was not found"
	}
	writeError(w, status, code, message)
}

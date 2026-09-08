package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	executiondomain "github.com/disturb-yy/keystone/internal/execution/domain"
	"github.com/disturb-yy/keystone/internal/infrastructure/sourcecontrol"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
)

// WorkerProtocolHandler 是 Daemon 到本机 Worker 的窄 loopback HTTP 边界。
// Handler 只做鉴权、严格 JSON 解码、错误映射和 Authority 调用，不写 SQL。
type WorkerProtocolHandler struct {
	Authority   *workstore.Store
	Artifacts   workstore.WorkerArtifactStore
	Wake        func()
	Snapshotter sourcecontrol.Adapter
}

func (s *Server) handleWorkerRoute(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	authority, artifacts, snapshotter := s.workerStore, s.artifacts, s.sourceControl
	s.mu.RUnlock()
	handler := NewWorkerProtocolHandler(authority, artifacts)
	handler.Wake = s.wakePlanning
	handler.Snapshotter = snapshotter
	handler.ServeHTTP(w, r)
}

// NewWorkerProtocolHandler 创建 Worker Protocol Handler。
func NewWorkerProtocolHandler(authority *workstore.Store, artifacts workstore.WorkerArtifactStore) *WorkerProtocolHandler {
	return &WorkerProtocolHandler{Authority: authority, Artifacts: artifacts}
}

// ServeHTTP 分发固定的 Register、Heartbeat、Pull 和 Report 路由。
func (h *WorkerProtocolHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Authority == nil || !isLoopbackWorkerRequest(r) {
		writeWorkerError(w, http.StatusForbidden, "worker_unauthorized", "worker request is not allowed")
		return
	}
	switch r.URL.Path {
	case "/worker/v1/register":
		h.handleRegister(w, r)
	case "/worker/v1/heartbeat":
		h.handleHeartbeat(w, r)
	case "/worker/v1/pull":
		h.handlePull(w, r)
	case "/worker/v1/claim":
		h.handleClaim(w, r)
	case "/worker/v1/report":
		h.handleReport(w, r)
	default:
		writeWorkerError(w, http.StatusNotFound, "not_found", "worker route is not found")
	}
}

func (h *WorkerProtocolHandler) handleClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkerError(w, http.StatusMethodNotAllowed, "protocol_invalid", "worker method is not allowed")
		return
	}
	var request workercontract.ClaimRequest
	if err := decodeWorkerRequest(r, &request); err != nil || request.Validate() != nil {
		writeWorkerError(w, http.StatusBadRequest, "protocol_invalid", "worker claim request is invalid")
		return
	}
	secret, ok := readBearerSecret(r)
	if !ok {
		writeWorkerError(w, http.StatusUnauthorized, "worker_unauthorized", "worker credentials are invalid")
		return
	}
	workerID, err := h.Authority.WorkerIDForSecret(r.Context(), secret)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	response, err := h.Authority.ClaimExecution(r.Context(), workerID, request)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	info, err := h.Authority.ExecutionAssignmentInfoFor(r.Context(), workerID, request.AgentRunID, request.LeaseToken)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	snapshot, err := h.Snapshotter.Observe(r.Context(), info.WorkspacePath, info.InputRevision)
	if err != nil {
		if shouldFenceExecutionObservation(err) {
			if fenceErr := h.fenceExecutionObservation(r.Context(), request.AgentRunID, "pre_snapshot"); fenceErr != nil {
				writeWorkerAuthorityError(w, fenceErr)
				return
			}
			writeWorkerAuthorityError(w, workstore.ErrWorkerReportInvalid)
			return
		}
		writeWorkerAuthorityError(w, workstore.ErrWorkerUnavailable)
		return
	}
	digest := sha256.Sum256(snapshot.Diff)
	if err := h.Authority.RecordExecutionSnapshot(r.Context(), request.AgentRunID, workstore.WorkspaceSnapshotInput{Phase: "pre", HeadRevision: snapshot.HeadRevision, Branch: snapshot.Branch, ChangedFiles: snapshot.ChangedFiles, DiffSHA256: hex.EncodeToString(digest[:]), DiffBytes: int64(len(snapshot.Diff)), HasUntracked: snapshot.HasUntracked}); err != nil {
		if shouldFenceExecutionObservation(err) {
			if fenceErr := h.fenceExecutionObservation(r.Context(), request.AgentRunID, "pre_snapshot"); fenceErr != nil {
				writeWorkerAuthorityError(w, fenceErr)
				return
			}
			writeWorkerAuthorityError(w, workstore.ErrWorkerReportInvalid)
			return
		}
		writeWorkerAuthorityError(w, err)
		return
	}
	writeWorkerJSON(w, http.StatusOK, response)
}

func (h *WorkerProtocolHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkerError(w, http.StatusMethodNotAllowed, "protocol_invalid", "worker method is not allowed")
		return
	}
	var request workercontract.Register
	if err := decodeWorkerRequest(r, &request); err != nil || request.Validate() != nil {
		writeWorkerError(w, http.StatusBadRequest, "protocol_invalid", "worker register request is invalid")
		return
	}
	secret, ok := readBearerSecret(r)
	if !ok || h.Authority.AuthenticateWorker(r.Context(), request.WorkerID, secret) != nil {
		writeWorkerError(w, http.StatusUnauthorized, "worker_unauthorized", "worker credentials are invalid")
		return
	}
	response, err := h.Authority.RegisterWorker(r.Context(), request)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	h.wakePlanning()
	writeWorkerJSON(w, http.StatusOK, response)
}

func (h *WorkerProtocolHandler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkerError(w, http.StatusMethodNotAllowed, "protocol_invalid", "worker method is not allowed")
		return
	}
	var request workercontract.Heartbeat
	if err := decodeWorkerRequest(r, &request); err != nil || request.Validate() != nil {
		writeWorkerError(w, http.StatusBadRequest, "protocol_invalid", "worker heartbeat request is invalid")
		return
	}
	secret, ok := readBearerSecret(r)
	if !ok || h.Authority.AuthenticateWorker(r.Context(), request.WorkerID, secret) != nil {
		writeWorkerError(w, http.StatusUnauthorized, "worker_unauthorized", "worker credentials are invalid")
		return
	}
	response, err := h.Authority.HeartbeatWorker(r.Context(), request)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	if request.AgentRunID == "" {
		h.wakePlanning()
	}
	writeWorkerJSON(w, http.StatusOK, response)
}

func (h *WorkerProtocolHandler) handlePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkerError(w, http.StatusMethodNotAllowed, "protocol_invalid", "worker method is not allowed")
		return
	}
	var request workercontract.PullRequest
	if err := decodeWorkerRequest(r, &request); err != nil || request.Validate() != nil {
		writeWorkerError(w, http.StatusBadRequest, "protocol_invalid", "worker pull request is invalid")
		return
	}
	secret, ok := readBearerSecret(r)
	if !ok || h.Authority.AuthenticateWorker(r.Context(), request.WorkerID, secret) != nil {
		writeWorkerError(w, http.StatusUnauthorized, "worker_unauthorized", "worker credentials are invalid")
		return
	}
	assignment, err := h.Authority.PullAssignment(r.Context(), request.WorkerID)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	writeWorkerJSON(w, http.StatusOK, workercontract.PullResponse{Assignment: assignment})
}

func (h *WorkerProtocolHandler) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkerError(w, http.StatusMethodNotAllowed, "protocol_invalid", "worker method is not allowed")
		return
	}
	var request workercontract.Report
	if err := decodeWorkerRequest(r, &request); err != nil || request.Validate() != nil {
		writeWorkerError(w, http.StatusBadRequest, "protocol_invalid", "worker report request is invalid")
		return
	}
	secret, ok := readBearerSecret(r)
	if !ok {
		writeWorkerError(w, http.StatusUnauthorized, "worker_unauthorized", "worker credentials are invalid")
		return
	}
	workerID, err := h.Authority.WorkerIDForSecret(r.Context(), secret)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	if info, infoErr := h.Authority.ExecutionAssignmentInfoFor(r.Context(), workerID, request.AgentRunID, request.LeaseToken); infoErr == nil {
		snapshot, snapshotErr := h.Snapshotter.Observe(r.Context(), info.WorkspacePath, info.InputRevision)
		if snapshotErr != nil && !errors.Is(snapshotErr, workstore.ErrWorkerLeaseInvalid) {
			if shouldFenceExecutionObservation(snapshotErr) {
				if fenceErr := h.fenceExecutionObservation(r.Context(), request.AgentRunID, "post_snapshot"); fenceErr != nil {
					writeWorkerAuthorityError(w, fenceErr)
					return
				}
				writeWorkerAuthorityError(w, workstore.ErrWorkerReportInvalid)
				return
			}
			writeWorkerAuthorityError(w, workstore.ErrWorkerUnavailable)
			return
		}
		if snapshotErr == nil {
			digest := sha256.Sum256(snapshot.Diff)
			if snapshotErr := h.Authority.RecordExecutionSnapshot(r.Context(), request.AgentRunID, workstore.WorkspaceSnapshotInput{Phase: "post", HeadRevision: snapshot.HeadRevision, Branch: snapshot.Branch, ChangedFiles: snapshot.ChangedFiles, DiffSHA256: hex.EncodeToString(digest[:]), DiffBytes: int64(len(snapshot.Diff)), HasUntracked: snapshot.HasUntracked}); snapshotErr != nil && !errors.Is(snapshotErr, workstore.ErrWorkerLeaseInvalid) {
				if shouldFenceExecutionObservation(snapshotErr) {
					if fenceErr := h.fenceExecutionObservation(r.Context(), request.AgentRunID, "post_snapshot"); fenceErr != nil {
						writeWorkerAuthorityError(w, fenceErr)
						return
					}
					writeWorkerAuthorityError(w, workstore.ErrWorkerReportInvalid)
					return
				}
				writeWorkerAuthorityError(w, snapshotErr)
				return
			}
		}
	} else if !errors.Is(infoErr, workstore.ErrWorkerLeaseInvalid) {
		writeWorkerAuthorityError(w, infoErr)
		return
	}
	response, err := h.Authority.ReportWorkerFor(r.Context(), workerID, request, h.Artifacts)
	if err != nil {
		writeWorkerAuthorityError(w, err)
		return
	}
	status := http.StatusOK
	if response.Disposition == "terminal_conflict" {
		status = http.StatusConflict
	}
	if response.Disposition == "candidate_received" || response.Disposition == "late" || response.Disposition == "duplicate" {
		h.wakePlanning()
	}
	writeWorkerJSON(w, status, response)
}

func shouldFenceExecutionObservation(err error) bool {
	return errors.Is(err, sourcecontrol.ErrInvalidRequest) ||
		errors.Is(err, sourcecontrol.ErrRevisionMismatch) ||
		errors.Is(err, sourcecontrol.ErrBranchConflict) ||
		errors.Is(err, sourcecontrol.ErrWorkspaceConflict) ||
		errors.Is(err, sourcecontrol.ErrUnsupportedRepository) ||
		errors.Is(err, executiondomain.ErrConflict)
}

func (h *WorkerProtocolHandler) fenceExecutionObservation(ctx context.Context, agentRunID, reason string) error {
	if err := h.Authority.FenceExecutionAssignment(ctx, agentRunID, reason); err != nil && !errors.Is(err, workstore.ErrWorkerLeaseInvalid) {
		return err
	}
	return nil
}

func (h *WorkerProtocolHandler) wakePlanning() {
	if h != nil && h.Wake != nil {
		h.Wake()
	}
}

func decodeWorkerRequest(r *http.Request, destination any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, workercontract.MaxBodyBytes+1))
	if err != nil {
		return err
	}
	return workercontract.DecodeStrict(body, destination)
}

func readBearerSecret(r *http.Request) (string, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func isLoopbackWorkerRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func writeWorkerAuthorityError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusServiceUnavailable, "unavailable", "worker authority is unavailable"
	switch {
	case errors.Is(err, workstore.ErrWorkerUnauthorized), errors.Is(err, workstore.ErrWorkerNotRegistered):
		status, code, message = http.StatusUnauthorized, "worker_unauthorized", "worker credentials are invalid"
	case errors.Is(err, workstore.ErrWorkerLeaseInvalid):
		status, code, message = http.StatusConflict, "lease_invalid", "worker lease is invalid"
	case errors.Is(err, workstore.ErrWorkerAssignmentConflict):
		status, code, message = http.StatusConflict, "assignment_conflict", "worker assignment is unavailable"
	case errors.Is(err, workstore.ErrWorkerCapabilityUnavailable):
		status, code, message = http.StatusConflict, "capability_unavailable", "worker capability is unavailable"
	case errors.Is(err, workstore.ErrWorkerReportInvalid):
		status, code, message = http.StatusBadRequest, "protocol_invalid", "worker report is invalid"
	case errors.Is(err, workstore.ErrWorkerUnavailable):
		status, code, message = http.StatusServiceUnavailable, "unavailable", "worker authority is temporarily unavailable"
	}
	writeWorkerError(w, status, code, message)
}

func writeWorkerError(w http.ResponseWriter, status int, code, message string) {
	writeWorkerJSON(w, status, workercontract.ErrorResponse{Code: code, Message: message})
}

func writeWorkerJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

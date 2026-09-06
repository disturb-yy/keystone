package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const maxChangeRequestBytes = 1 << 20

func (s *Server) handleChangesRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleChangeCreate(w, r)
	case http.MethodGet:
		s.handleChangeList(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
	}
}

func (s *Server) handleChangeCreate(w http.ResponseWriter, r *http.Request) {
	request, err := decodeStrictJSON(r, controlplane.ChangeCreateRequest{})
	if err != nil || request.RepositoryPath == "" || request.Intent == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "change creation request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	service := s.changeService()
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "change service is unavailable")
		return
	}
	change, err := service.Create(r.Context(), work.ChangeCreateRequest{RepositoryPath: request.RepositoryPath, Intent: request.Intent, IdempotencyKey: key.String(), Actor: actorFromRequest(r)})
	if err != nil {
		writeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, controlplane.ChangeCreateResponse{Change: changeDTO(change)})
}

func (s *Server) handleChangeList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("repository_path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "repository_path is required")
		return
	}
	service := s.changeService()
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "change service is unavailable")
		return
	}
	changes, err := service.List(r.Context(), path)
	if err != nil {
		writeChangeError(w, err)
		return
	}
	response := controlplane.ChangeListResponse{Changes: make([]controlplane.ChangeDTO, 0, len(changes))}
	for _, change := range changes {
		response.Changes = append(response.Changes, changeDTO(change))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleChangeRoute(w http.ResponseWriter, r *http.Request) {
	parts := splitChangePath(r.URL.Path)
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "change_not_found", "change was not found")
		return
	}
	changeIDText, err := url.PathUnescape(parts[0])
	if err != nil || controlplane.ValidateChangeID(changeIDText) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "change_id is invalid")
		return
	}
	changeID := domain.ChangeID(changeIDText)
	service := s.changeService()
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "change service is unavailable")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
			return
		}
		change, err := service.Show(r.Context(), changeID)
		if err != nil {
			writeChangeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, controlplane.ChangeCommandResponse{Change: changeDTO(change)})
		return
	}
	if parts[1] == "commands" || parts[1] == "command" {
		if r.Method != http.MethodPost || len(parts) != 2 {
			writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
			return
		}
		s.handleChangeCommand(w, r, service, changeID)
		return
	}
	if parts[1] == "decisions" || parts[1] == "decision" {
		if len(parts) != 2 {
			writeError(w, http.StatusNotFound, "change_not_found", "change was not found")
			return
		}
		if r.Method == http.MethodPost {
			s.handleChangeDecision(w, r, service, changeID)
			return
		}
		if r.Method == http.MethodGet {
			decisions, err := service.Decisions(r.Context(), changeID)
			if err != nil {
				writeChangeError(w, err)
				return
			}
			response := controlplane.ChangeDecisionsResponse{Decisions: make([]controlplane.HumanDecisionDTO, 0, len(decisions))}
			for _, decision := range decisions {
				response.Decisions = append(response.Decisions, humanDecisionDTO(decision))
			}
			writeJSON(w, http.StatusOK, response)
			return
		}
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	switch parts[1] {
	case "events":
		if len(parts) != 2 {
			writeError(w, http.StatusNotFound, "change_not_found", "change was not found")
			return
		}
		events, err := service.Events(r.Context(), changeID)
		if err != nil {
			writeChangeError(w, err)
			return
		}
		response := controlplane.ChangeEventsResponse{Events: make([]controlplane.ChangeEventDTO, 0, len(events))}
		for _, event := range events {
			response.Events = append(response.Events, changeEventDTO(event))
		}
		writeJSON(w, http.StatusOK, response)
	case "runs":
		if len(parts) != 2 {
			writeError(w, http.StatusNotFound, "change_not_found", "change was not found")
			return
		}
		runs, err := service.Runs(r.Context(), changeID)
		if err != nil {
			writeChangeError(w, err)
			return
		}
		response := controlplane.ChangeRunsResponse{Runs: make([]controlplane.AgentRunDTO, 0, len(runs))}
		for _, run := range runs {
			response.Runs = append(response.Runs, agentRunDTO(run))
		}
		writeJSON(w, http.StatusOK, response)
	case "artifacts":
		s.handleArtifactRoute(w, r, service, changeID, parts[2:])
	default:
		writeError(w, http.StatusNotFound, "change_not_found", "change was not found")
	}
}

func (s *Server) handleChangeCommand(w http.ResponseWriter, r *http.Request, service *work.ChangeService, changeID domain.ChangeID) {
	request, err := decodeStrictJSON(r, controlplane.ChangeCommandRequest{})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "change command request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	change, err := service.Command(r.Context(), work.ChangeCommandRequest{ChangeID: changeID, Command: request.Command, ExpectedVersion: domain.ChangeVersion(request.ExpectedVersion), IdempotencyKey: key.String(), Actor: actorFromRequest(r)})
	if err != nil {
		writeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, controlplane.ChangeCommandResponse{Change: changeDTO(change)})
}

func (s *Server) handleChangeDecision(w http.ResponseWriter, r *http.Request, service *work.ChangeService, changeID domain.ChangeID) {
	request, err := decodeStrictJSON(r, controlplane.HumanDecisionRequest{})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "human decision request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	change, err := service.Decide(r.Context(), work.ChangeDecisionRequest{ChangeID: changeID, Decision: request.Decision, ExpectedVersion: domain.ChangeVersion(request.ExpectedVersion), IdempotencyKey: key.String(), Actor: actorFromRequest(r), Reason: request.Reason})
	if err != nil {
		writeChangeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, controlplane.HumanDecisionResponse{Change: changeDTO(change)})
}

func (s *Server) handleArtifactRoute(w http.ResponseWriter, r *http.Request, service *work.ChangeService, changeID domain.ChangeID, parts []string) {
	if len(parts) == 0 {
		refs, err := service.Artifacts(r.Context(), changeID)
		if err != nil {
			writeChangeError(w, err)
			return
		}
		response := controlplane.ChangeArtifactsResponse{Artifacts: make([]controlplane.ArtifactRefDTO, 0, len(refs))}
		for _, ref := range refs {
			response.Artifacts = append(response.Artifacts, artifactRefDTO(ref))
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if len(parts) != 2 || parts[1] != "content" || r.Method != http.MethodGet {
		writeError(w, http.StatusNotFound, "artifact_not_found", "artifact was not found")
		return
	}
	refIDText, err := url.PathUnescape(parts[0])
	if err != nil || controlplane.ValidateArtifactRefID(refIDText) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "artifact_ref_id is invalid")
		return
	}
	artifact, content, err := service.ArtifactContent(r.Context(), changeID, domain.ArtifactRefID(refIDText))
	if err != nil {
		writeChangeError(w, err)
		return
	}
	w.Header().Set("Content-Type", artifact.MediaType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("ETag", strconv.Quote(artifact.Identity.SHA256))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) changeService() *work.ChangeService {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.changes
}

func decodeStrictJSON[T any](r *http.Request, target T) (T, error) {
	if r == nil || r.Body == nil {
		return target, errors.New("request body is nil")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxChangeRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil {
		return target, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return target, errors.New("multiple JSON values")
		}
		return target, err
	}
	return target, nil
}

func actorFromRequest(r *http.Request) string {
	if actor := strings.TrimSpace(r.Header.Get("X-Actor")); actor != "" {
		return actor
	}
	return "client"
}

func splitChangePath(path string) []string {
	path = strings.TrimPrefix(path, "/v1/changes/")
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func changeDTO(change domain.Change) controlplane.ChangeDTO {
	return controlplane.ChangeDTO{
		ChangeID: string(change.ID), ProjectID: string(change.ProjectID), RepositoryRoot: change.RepositoryRoot,
		Stage: string(change.Stage), Status: string(change.Status), Version: int(change.Version), BaseRevision: change.BaseRevision,
		IntentArtifact: artifactRefDTO(change.Intent), LatestAgentRun: agentRunPointerDTO(change.LatestAgentRun),
		CreatedAt: change.CreatedAt.UTC().Format(timeRFC3339Nano), UpdatedAt: change.UpdatedAt.UTC().Format(timeRFC3339Nano),
	}
}

func artifactRefDTO(ref domain.ArtifactRef) controlplane.ArtifactRefDTO {
	return controlplane.ArtifactRefDTO{ArtifactRefID: string(ref.ID), ArtifactID: string(ref.ArtifactID), Role: ref.Role, Ordinal: ref.Ordinal}
}

func agentRunPointerDTO(run *domain.AgentRun) *controlplane.AgentRunDTO {
	if run == nil {
		return nil
	}
	dto := agentRunDTO(*run)
	return &dto
}

func agentRunDTO(run domain.AgentRun) controlplane.AgentRunDTO {
	var completedAt *string
	if run.CompletedAt != nil {
		value := run.CompletedAt.UTC().Format(timeRFC3339Nano)
		completedAt = &value
	}
	artifacts := make([]controlplane.AgentRunArtifactDTO, 0, len(run.Artifacts))
	for _, artifact := range run.Artifacts {
		artifacts = append(artifacts, controlplane.AgentRunArtifactDTO{ArtifactRefID: string(artifact.ArtifactRefID), Role: artifact.Role, Ordinal: artifact.Ordinal})
	}
	return controlplane.AgentRunDTO{AgentRunID: string(run.ID), ChangeID: string(run.ChangeID), Stage: string(run.Stage), Attempt: run.Attempt, Status: run.Status, Outcome: run.Outcome, Artifacts: artifacts, StartedAt: run.StartedAt.UTC().Format(timeRFC3339Nano), CompletedAt: completedAt}
}

func humanDecisionDTO(decision domain.HumanDecision) controlplane.HumanDecisionDTO {
	return controlplane.HumanDecisionDTO{DecisionID: string(decision.ID), ChangeID: string(decision.ChangeID), Decision: decision.Kind, Actor: decision.Actor, Reason: decision.Reason, CreatedAt: decision.CreatedAt.UTC().Format(timeRFC3339Nano)}
}

func changeEventDTO(event domain.ChangeEvent) controlplane.ChangeEventDTO {
	refs := make([]string, 0, len(event.ArtifactRefIDs))
	for _, ref := range event.ArtifactRefIDs {
		refs = append(refs, string(ref))
	}
	var runID, decisionID *string
	if event.AgentRunID != nil {
		value := string(*event.AgentRunID)
		runID = &value
	}
	if event.DecisionID != nil {
		value := string(*event.DecisionID)
		decisionID = &value
	}
	return controlplane.ChangeEventDTO{EventID: event.EventID, ChangeID: string(event.ChangeID), Sequence: event.Sequence, Type: event.Type, OccurredAt: event.OccurredAt.UTC().Format(timeRFC3339Nano), Actor: event.Actor, ArtifactRefIDs: refs, AgentRunID: runID, DecisionID: decisionID}
}

func writeChangeError(w http.ResponseWriter, err error) {
	code := work.ChangeErrorCode(err)
	status := changeStatus(code)
	writeError(w, status, code, changeMessage(code))
}

func changeStatus(code string) int {
	switch code {
	case "invalid_request":
		return http.StatusBadRequest
	case "repository_dirty", "base_revision_unavailable", "source_snapshot_unstable", "lifecycle_transition_invalid", "human_decision_required":
		return http.StatusConflict
	case "project_not_found", "change_not_found", "artifact_not_found":
		return http.StatusNotFound
	case "idempotency_conflict", "change_version_conflict":
		return http.StatusConflict
	case "unavailable":
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func changeMessage(code string) string {
	switch code {
	case "repository_dirty":
		return "repository has uncommitted changes"
	case "base_revision_unavailable":
		return "repository base revision is unavailable"
	case "source_snapshot_unstable":
		return "repository source snapshot is unstable"
	case "change_version_conflict":
		return "change version conflicts"
	case "idempotency_conflict":
		return "idempotency key conflicts with change request"
	case "lifecycle_transition_invalid":
		return "change lifecycle transition is invalid"
	case "human_decision_required":
		return "human decision is required"
	case "change_not_found":
		return "change was not found"
	case "artifact_not_found":
		return "artifact was not found"
	case "project_not_found":
		return "project was not found"
	case "unavailable":
		return "change operation is temporarily unavailable"
	case "invalid_request":
		return "change request is invalid"
	default:
		return fmt.Sprintf("change operation failed with code %s", code)
	}
}

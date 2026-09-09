package daemon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	observationSchemaVersion = "dashboard-observation.v1"
	sectionAvailable         = "available"
	sectionNotYetAvailable   = "not_yet_available"
	maxObservationItems      = 100
)

func (s *Server) handleProjectInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	store := s.workStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "dashboard query is unavailable")
		return
	}
	page, err := dashboardPageFromRequest(r)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	result, err := store.ListProjectSummaries(r.Context(), page)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	response := controlplane.ProjectListResponse{Projects: make([]controlplane.ProjectSummaryDTO, 0, len(result.Projects))}
	response.HasMore, response.NextCursor = result.HasMore, result.NextCursor
	for _, project := range result.Projects {
		response.Projects = append(response.Projects, projectSummaryDTO(project))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleProjectChanges(w http.ResponseWriter, r *http.Request, projectID domain.ProjectID) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	store := s.workStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "dashboard query is unavailable")
		return
	}
	page, err := dashboardPageFromRequest(r)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	result, err := store.ListProjectChanges(r.Context(), projectID, page)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	response := controlplane.ProjectChangesResponse{ProjectID: string(projectID), Changes: make([]controlplane.ChangeSummaryDTO, 0, len(result.Changes))}
	response.HasMore, response.NextCursor = result.HasMore, result.NextCursor
	for _, change := range result.Changes {
		response.Changes = append(response.Changes, changeSummaryDTO(change))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleChangeObservation(w http.ResponseWriter, r *http.Request, changeID domain.ChangeID) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	s.mu.RLock()
	service, store, db := s.changes, s.workerStore, s.db
	ready := s.readiness
	s.mu.RUnlock()
	if service == nil || store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "dashboard query is unavailable")
		return
	}
	change, err := service.Show(r.Context(), changeID)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	project, err := store.FindProject(r.Context(), change.ProjectID)
	if err != nil {
		writeDashboardError(w, err)
		return
	}

	graphSection, err := s.observationTicketGraph(r, service, changeID)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	executionSection, err := s.observationExecution(r, store, changeID)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	traceSection, err := s.observationTrace(r, service, changeID)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	artifactSection, err := s.observationArtifacts(r, store, changeID)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	healthSection, err := s.observationHealth(r, store, db, ready)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	observation := controlplane.ChangeObservationReadModel{
		SchemaVersion: observationSchemaVersion,
		ObservedAt:    time.Now().UTC().Format(timeRFC3339Nano),
		Project:       controlplane.ObservationProjectDTO{ProjectID: string(project.Identity.ProjectID), CreatedAt: project.CreatedAt.UTC().Format(timeRFC3339Nano)},
		Change:        observationChangeDTO(change),
		Lifecycle: controlplane.LifecycleObservation{
			ObservationSection: availableSection(), Stage: string(change.Stage), Status: string(change.Status), Version: int(change.Version),
		},
		TicketGraph:      graphSection,
		Execution:        executionSection,
		Trace:            traceSection,
		Artifacts:        artifactSection,
		Health:           healthSection,
		AvailableActions: availableChangeActions(change),
	}
	writeJSON(w, http.StatusOK, observation)
}

func (s *Server) handleNeedsHuman(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	store := s.workStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "dashboard query is unavailable")
		return
	}
	page, err := dashboardPageFromRequest(r)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	projectID, changeID, err := dashboardFilters(r)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	result, err := store.ListNeedsHuman(r.Context(), projectID, changeID, page)
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	response := controlplane.NeedsHumanResponse{Items: make([]controlplane.NeedsHumanItemDTO, 0, len(result.Items))}
	response.HasMore, response.NextCursor = result.HasMore, result.NextCursor
	for _, item := range result.Items {
		response.Items = append(response.Items, controlplane.NeedsHumanItemDTO{
			ProjectID: string(item.Change.ProjectID), ChangeID: string(item.Change.ID), ReasonCode: item.ReasonCode,
			ReasonSummary: item.ReasonSummary, AvailableActions: []string{"retry", "cancel"}, ChangeVersion: int(item.Change.Version),
			RequiredAt: item.RequiredAt.UTC().Format(timeRFC3339Nano), EvidenceArtifactRefIDs: stringsFromArtifactRefs(item.EvidenceArtifactRefs),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) workStore() *workstore.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workerStore
}

func (s *Server) observationTicketGraph(r *http.Request, service interface {
	TicketGraph(context.Context, domain.ChangeID) (domain.CanonicalTicketGraph, error)
}, changeID domain.ChangeID) (controlplane.TicketGraphObservation, error) {
	graph, err := service.TicketGraph(r.Context(), changeID)
	if errors.Is(err, domain.ErrTicketGraphNotFound) {
		return controlplane.TicketGraphObservation{ObservationSection: unavailableSection("ticket_graph_not_yet_formed", "Canonical Ticket Graph has not been formed.")}, nil
	}
	if err != nil {
		return controlplane.TicketGraphObservation{}, err
	}
	value := ticketGraphReadModelDTO(graph)
	return controlplane.TicketGraphObservation{ObservationSection: availableSection(), Graph: &value}, nil
}

func (s *Server) observationExecution(r *http.Request, store *workstore.Store, changeID domain.ChangeID) (controlplane.ExecutionObservation, error) {
	execution, err := store.ReadExecution(r.Context(), string(changeID))
	if errors.Is(err, domain.ErrChangeNotFound) {
		return controlplane.ExecutionObservation{ObservationSection: unavailableSection("execution_not_started", "Execution ReadModel has not been formed.")}, nil
	}
	if err != nil {
		return controlplane.ExecutionObservation{}, err
	}
	value := executionReadModelStoreDTO(execution)
	return controlplane.ExecutionObservation{ObservationSection: availableSection(), Execution: &value}, nil
}

func (s *Server) observationTrace(r *http.Request, service *work.ChangeService, changeID domain.ChangeID) (controlplane.TraceObservation, error) {
	events, err := service.Events(r.Context(), changeID)
	if err != nil {
		return controlplane.TraceObservation{}, err
	}
	runs, err := service.Runs(r.Context(), changeID)
	if err != nil {
		return controlplane.TraceObservation{}, err
	}
	decisions, err := service.Decisions(r.Context(), changeID)
	if err != nil {
		return controlplane.TraceObservation{}, err
	}
	result := controlplane.TraceObservation{ObservationSection: availableSection(), Events: make([]controlplane.ChangeEventDTO, 0), Runs: make([]controlplane.AgentRunDTO, 0), Decisions: make([]controlplane.HumanDecisionDTO, 0)}
	for _, event := range boundedChangeEvents(events) {
		result.Events = append(result.Events, changeEventDTO(event))
	}
	for _, run := range boundedAgentRuns(runs) {
		result.Runs = append(result.Runs, agentRunDTO(run))
	}
	for _, decision := range boundedHumanDecisions(decisions) {
		result.Decisions = append(result.Decisions, humanDecisionDTO(decision))
	}
	return result, nil
}

func (s *Server) observationArtifacts(r *http.Request, store *workstore.Store, changeID domain.ChangeID) (controlplane.ArtifactsObservation, error) {
	artifacts, err := store.ListArtifactObservations(r.Context(), changeID, maxObservationItems)
	if err != nil {
		return controlplane.ArtifactsObservation{}, err
	}
	result := controlplane.ArtifactsObservation{ObservationSection: availableSection(), Artifacts: make([]controlplane.ArtifactObservationDTO, 0, len(artifacts))}
	for _, artifact := range artifacts {
		result.Artifacts = append(result.Artifacts, controlplane.ArtifactObservationDTO{
			ArtifactRefID: string(artifact.Ref.ID), ArtifactID: string(artifact.Ref.ArtifactID), Role: artifact.Ref.Role, Ordinal: artifact.Ref.Ordinal,
			Kind: artifact.Ref.Kind, SchemaVersion: artifact.Ref.SchemaVersion, Summary: artifact.Ref.Summary, SourceRevision: artifact.Ref.SourceRevision,
			ByteLength: artifact.ByteLength, MediaType: artifact.MediaType, InputArtifactRefIDs: stringsFromArtifactRefs(artifact.Ref.InputArtifactRefIDs), RawLogArtifactRefIDs: stringsFromArtifactRefs(artifact.Ref.RawLogArtifactRefIDs),
		})
	}
	return result, nil
}

func (s *Server) observationHealth(r *http.Request, store *workstore.Store, db *sql.DB, ready bool) (controlplane.HealthObservation, error) {
	workers, err := store.ListWorkerHealth(r.Context())
	if err != nil {
		return controlplane.HealthObservation{}, err
	}
	if db != nil {
		if err := db.PingContext(r.Context()); err != nil {
			return controlplane.HealthObservation{}, err
		}
	}
	result := controlplane.HealthObservation{ObservationSection: availableSection(), DaemonReady: ready && db != nil, Workers: make([]controlplane.WorkerHealthDTO, 0, len(workers))}
	for _, worker := range workers {
		value := controlplane.WorkerHealthDTO{WorkerID: worker.WorkerID, ProtocolVersion: worker.ProtocolVersion, Status: worker.Status, Capabilities: append([]string(nil), worker.Capabilities...)}
		if worker.LastHeartbeatAt != nil {
			formatted := worker.LastHeartbeatAt.UTC().Format(timeRFC3339Nano)
			value.LastHeartbeatAt = formatted
		}
		result.Workers = append(result.Workers, value)
	}
	return result, nil
}

func dashboardPageFromRequest(r *http.Request) (workstore.DashboardPage, error) {
	limit := workstore.DefaultDashboardPageLimit
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return workstore.DashboardPage{}, fmt.Errorf("limit must be an integer: %w", domain.ErrInvalidRequest)
		}
		limit = parsed
	}
	return workstore.DashboardPage{Limit: limit, Cursor: r.URL.Query().Get("cursor")}, nil
}

func dashboardFilters(r *http.Request) (domain.ProjectID, domain.ChangeID, error) {
	projectIDText := strings.TrimSpace(r.URL.Query().Get("project_id"))
	changeIDText := strings.TrimSpace(r.URL.Query().Get("change_id"))
	var projectID domain.ProjectID
	if projectIDText != "" {
		value, err := url.PathUnescape(projectIDText)
		if err != nil || controlplane.ValidateProjectID(value) != nil {
			return "", "", fmt.Errorf("project_id is invalid: %w", domain.ErrInvalidRequest)
		}
		projectID = domain.ProjectID(value)
	}
	var changeID domain.ChangeID
	if changeIDText != "" {
		value, err := url.PathUnescape(changeIDText)
		if err != nil || controlplane.ValidateChangeID(value) != nil {
			return "", "", fmt.Errorf("change_id is invalid: %w", domain.ErrInvalidRequest)
		}
		changeID = domain.ChangeID(value)
	}
	return projectID, changeID, nil
}

func projectSummaryDTO(project workstore.ProjectSummary) controlplane.ProjectSummaryDTO {
	return controlplane.ProjectSummaryDTO{ProjectID: string(project.ProjectID), RepositoryRoot: project.RepositoryRoot, CreatedAt: project.CreatedAt.UTC().Format(timeRFC3339Nano), ChangeCount: project.ChangeCount, ActiveChangeCount: project.ActiveChangeCount}
}

func changeSummaryDTO(change domain.Change) controlplane.ChangeSummaryDTO {
	return controlplane.ChangeSummaryDTO{ChangeID: string(change.ID), ProjectID: string(change.ProjectID), Stage: string(change.Stage), Status: string(change.Status), Version: int(change.Version), BaseRevision: change.BaseRevision, LatestAgentRun: agentRunPointerDTO(change.LatestAgentRun), CreatedAt: change.CreatedAt.UTC().Format(timeRFC3339Nano), UpdatedAt: change.UpdatedAt.UTC().Format(timeRFC3339Nano)}
}

func observationChangeDTO(change domain.Change) controlplane.ObservationChangeDTO {
	return controlplane.ObservationChangeDTO{ChangeID: string(change.ID), ProjectID: string(change.ProjectID), Stage: string(change.Stage), Status: string(change.Status), Version: int(change.Version), BaseRevision: change.BaseRevision, IntentArtifact: artifactRefDTO(change.Intent), LatestAgentRun: agentRunPointerDTO(change.LatestAgentRun), CreatedAt: change.CreatedAt.UTC().Format(timeRFC3339Nano), UpdatedAt: change.UpdatedAt.UTC().Format(timeRFC3339Nano)}
}

func availableSection() controlplane.ObservationSection {
	return controlplane.ObservationSection{Availability: sectionAvailable}
}

func unavailableSection(reasonCode, summary string) controlplane.ObservationSection {
	return controlplane.ObservationSection{Availability: sectionNotYetAvailable, ReasonCode: reasonCode, ReasonSummary: summary}
}

func availableChangeActions(change domain.Change) []string {
	switch change.Status {
	case domain.ChangeStatusActive:
		return []string{"pause", "cancel"}
	case domain.ChangeStatusPaused:
		return []string{"resume", "cancel"}
	case domain.ChangeStatusHumanRequired:
		return []string{"retry", "cancel"}
	default:
		return []string{}
	}
}

func stringsFromArtifactRefs(refs []domain.ArtifactRefID) []string {
	if len(refs) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		result = append(result, string(ref))
	}
	return result
}

func boundedChangeEvents(events []domain.ChangeEvent) []domain.ChangeEvent {
	if len(events) > maxObservationItems {
		return events[:maxObservationItems]
	}
	return events
}

func boundedAgentRuns(runs []domain.AgentRun) []domain.AgentRun {
	if len(runs) > maxObservationItems {
		return runs[:maxObservationItems]
	}
	return runs
}

func boundedHumanDecisions(decisions []domain.HumanDecision) []domain.HumanDecision {
	if len(decisions) > maxObservationItems {
		return decisions[:maxObservationItems]
	}
	return decisions
}

func writeDashboardError(w http.ResponseWriter, err error) {
	code := "internal_error"
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, domain.ErrInvalidRequest):
		code, status = "invalid_request", http.StatusBadRequest
	case errors.Is(err, domain.ErrProjectNotFound), errors.Is(err, domain.ErrChangeNotFound):
		code, status = "not_found", http.StatusNotFound
	case errors.Is(err, domain.ErrUnavailable), errors.Is(err, workstore.ErrVerificationUnavailable):
		code, status = "unavailable", http.StatusServiceUnavailable
	}
	writeError(w, status, code, "dashboard query failed")
}

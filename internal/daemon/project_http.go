package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const maxProjectRequestBytes = 1 << 20

func (s *Server) handleProjectInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	request, err := decodeProjectInitRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "project initialization request is invalid")
		return
	}
	key, err := controlplane.ParseIdempotencyKey(r.Header.Get(controlplane.IdempotencyKeyHeader))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is required")
		return
	}
	s.mu.RLock()
	service := s.projects
	s.mu.RUnlock()
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "project service is unavailable")
		return
	}
	project, err := service.Initialize(r.Context(), work.InitializeRequest{RepositoryPath: request.RepositoryPath, IdempotencyKey: key.String()})
	if err != nil {
		writeProjectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, controlplane.ProjectInitResponse{Project: projectDTO(project)})
}

func (s *Server) handleProjectRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) != 1 && !(len(parts) == 2 && parts[1] == "events") {
		writeError(w, http.StatusNotFound, "project_not_found", "project was not found")
		return
	}
	projectIDText, err := url.PathUnescape(parts[0])
	if err != nil || controlplane.ValidateProjectID(projectIDText) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "project_id is invalid")
		return
	}
	projectID := domain.ProjectID(projectIDText)
	s.mu.RLock()
	service := s.projects
	s.mu.RUnlock()
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "project service is unavailable")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
			return
		}
		project, err := service.GetProject(r.Context(), projectID)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, controlplane.ProjectQueryResponse{Project: projectDTO(project)})
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	events, err := service.ListEvents(r.Context(), projectID)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	response := controlplane.ProjectEventsResponse{Events: make([]controlplane.ProjectEventDTO, 0, len(events))}
	for _, event := range events {
		response.Events = append(response.Events, controlplane.ProjectEventDTO{EventID: event.EventID, ProjectID: string(event.ProjectID), Type: domain.ProjectInitializedType, OccurredAt: event.OccurredAt.UTC().Format(timeRFC3339Nano)})
	}
	writeJSON(w, http.StatusOK, response)
}

func decodeProjectInitRequest(r *http.Request) (controlplane.ProjectInitRequest, error) {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxProjectRequestBytes))
	decoder.DisallowUnknownFields()
	var request controlplane.ProjectInitRequest
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return request, errors.New("multiple JSON values")
		}
		return request, err
	}
	if strings.TrimSpace(request.RepositoryPath) == "" || !filepath.IsAbs(request.RepositoryPath) || filepath.Clean(request.RepositoryPath) != request.RepositoryPath {
		return request, errors.New("repository_path must be a normalized absolute path")
	}
	return request, nil
}

func projectDTO(project domain.Project) controlplane.ProjectDTO {
	return controlplane.ProjectDTO{ProjectID: string(project.Identity.ProjectID), RepositoryRoot: project.Binding.Root, ManifestPath: project.Binding.ManifestPath, CreatedAt: project.CreatedAt.UTC().Format(timeRFC3339Nano)}
}

const timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func writeProjectError(w http.ResponseWriter, err error) {
	code := work.ErrorCode(err)
	status := projectStatus(code)
	writeError(w, status, code, projectMessage(code))
}

func projectStatus(code string) int {
	switch code {
	case "invalid_request":
		return http.StatusBadRequest
	case "repository_unsupported", "manifest_invalid":
		return http.StatusUnprocessableEntity
	case "idempotency_conflict", "project_identity_conflict":
		return http.StatusConflict
	case "project_not_found":
		return http.StatusNotFound
	case "manifest_unavailable", "unavailable":
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func projectMessage(code string) string {
	switch code {
	case "invalid_request":
		return "project initialization request is invalid"
	case "repository_unsupported":
		return "repository topology is unsupported"
	case "manifest_invalid":
		return "project manifest is invalid"
	case "idempotency_conflict":
		return "idempotency key conflicts with repository"
	case "project_identity_conflict":
		return "project identity conflicts with repository binding"
	case "project_not_found":
		return "project was not found"
	case "manifest_unavailable":
		return "project manifest is temporarily unavailable"
	default:
		return fmt.Sprintf("project operation failed with code %s", code)
	}
}

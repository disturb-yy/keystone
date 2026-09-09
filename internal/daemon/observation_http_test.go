package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
)

func TestDashboardQueriesExposeBoundedObservationAndSPAFallback(t *testing.T) {
	server, root := newChangeHTTPTestServer(t)
	server.updates = newRefreshHub()
	handler := server.routes()

	projectInit := httptest.NewRequest(http.MethodPost, "/v1/projects/init", bytes.NewBufferString(`{"repository_path":"`+root+`"}`))
	projectInit.Header.Set(controlplane.IdempotencyKeyHeader, "dashboard-project")
	projectInitResponse := httptest.NewRecorder()
	handler.ServeHTTP(projectInitResponse, projectInit)
	if projectInitResponse.Code != http.StatusOK {
		t.Fatalf("project init status = %d, body = %s", projectInitResponse.Code, projectInitResponse.Body.String())
	}
	var initialized controlplane.ProjectInitResponse
	decodeJSON(t, projectInitResponse, &initialized)
	commitGit(t, root, "dashboard project")

	create := httptest.NewRequest(http.MethodPost, "/v1/changes", bytes.NewBufferString(`{"repository_path":"`+root+`","intent":"observe dashboard"}`))
	create.Header.Set(controlplane.IdempotencyKeyHeader, "dashboard-change")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("change create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	var created controlplane.ChangeCreateResponse
	decodeJSON(t, createResponse, &created)

	projectsResponse := httptest.NewRecorder()
	handler.ServeHTTP(projectsResponse, httptest.NewRequest(http.MethodGet, "/v1/projects?limit=50", nil))
	if projectsResponse.Code != http.StatusOK {
		t.Fatalf("projects status = %d, body = %s", projectsResponse.Code, projectsResponse.Body.String())
	}
	var projects controlplane.ProjectListResponse
	decodeJSON(t, projectsResponse, &projects)
	if len(projects.Projects) != 1 || projects.Projects[0].ProjectID != initialized.Project.ProjectID || projects.Projects[0].ChangeCount != 1 {
		t.Fatalf("projects = %+v", projects)
	}

	changesResponse := httptest.NewRecorder()
	handler.ServeHTTP(changesResponse, httptest.NewRequest(http.MethodGet, "/v1/projects/"+initialized.Project.ProjectID+"/changes", nil))
	if changesResponse.Code != http.StatusOK {
		t.Fatalf("project changes status = %d, body = %s", changesResponse.Code, changesResponse.Body.String())
	}
	var changes controlplane.ProjectChangesResponse
	decodeJSON(t, changesResponse, &changes)
	if len(changes.Changes) != 1 || changes.Changes[0].ChangeID != created.Change.ChangeID {
		t.Fatalf("project changes = %+v", changes)
	}

	observationResponse := httptest.NewRecorder()
	handler.ServeHTTP(observationResponse, httptest.NewRequest(http.MethodGet, "/v1/changes/"+created.Change.ChangeID+"/observation", nil))
	if observationResponse.Code != http.StatusOK {
		t.Fatalf("observation status = %d, body = %s", observationResponse.Code, observationResponse.Body.String())
	}
	var observation controlplane.ChangeObservationReadModel
	decodeJSON(t, observationResponse, &observation)
	if observation.SchemaVersion != "dashboard-observation.v1" || observation.Change.ChangeID != created.Change.ChangeID || observation.TicketGraph.Availability != "not_yet_available" || observation.Execution.Availability != "not_yet_available" || observation.Trace.Availability != "available" || len(observation.AvailableActions) != 2 {
		t.Fatalf("observation = %+v", observation)
	}
	encodedObservation, _ := json.Marshal(observation)
	if strings.Contains(string(encodedObservation), "repository_root") || strings.Contains(string(encodedObservation), "workspace_path") {
		t.Fatalf("observation leaked restricted path fields: %s", encodedObservation)
	}

	needsHumanResponse := httptest.NewRecorder()
	handler.ServeHTTP(needsHumanResponse, httptest.NewRequest(http.MethodGet, "/v1/needs-human", nil))
	if needsHumanResponse.Code != http.StatusOK {
		t.Fatalf("needs human status = %d, body = %s", needsHumanResponse.Code, needsHumanResponse.Body.String())
	}
	var needsHuman controlplane.NeedsHumanResponse
	decodeJSON(t, needsHumanResponse, &needsHuman)
	if len(needsHuman.Items) != 0 || needsHuman.HasMore {
		t.Fatalf("needs human = %+v", needsHuman)
	}

	dashboardDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dashboardDir, "index.html"), []byte("dashboard-shell"), 0644); err != nil {
		t.Fatal(err)
	}
	server.options.DashboardDir = dashboardDir
	deepLink := httptest.NewRecorder()
	handler.ServeHTTP(deepLink, httptest.NewRequest(http.MethodGet, "/projects/"+initialized.Project.ProjectID, nil))
	if deepLink.Code != http.StatusOK || deepLink.Body.String() != "dashboard-shell" {
		t.Fatalf("deep link = (%d, %q)", deepLink.Code, deepLink.Body.String())
	}
	unknownAPI := httptest.NewRecorder()
	handler.ServeHTTP(unknownAPI, httptest.NewRequest(http.MethodGet, "/v1/not-a-route", nil))
	if unknownAPI.Code != http.StatusNotFound || strings.Contains(unknownAPI.Body.String(), "dashboard-shell") {
		t.Fatalf("unknown API = (%d, %q)", unknownAPI.Code, unknownAPI.Body.String())
	}
}

func TestUpdatesSendsFilteredRefreshHintOnly(t *testing.T) {
	server := &Server{stopCh: make(chan struct{}), updates: newRefreshHub()}
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	projectID := "0190a000-0000-7000-8000-000000000001"
	changeID := "0190a000-0000-7000-8000-000000000002"
	request := httptest.NewRequest(http.MethodGet, "/v1/updates?project_id="+projectID, nil).WithContext(requestContext)
	writer := newSSETestWriter()
	done := make(chan struct{})
	go func() {
		server.routes().ServeHTTP(writer, request)
		close(done)
	}()
	if !writer.waitForContains(": connected", time.Second) {
		t.Fatalf("SSE did not send connection comment: %q", writer.String())
	}
	server.publishRefresh(controlplane.RefreshHint{ResourceType: "change", ProjectID: "0190a000-0000-7000-8000-000000000099", ChangeID: changeID})
	server.publishRefresh(controlplane.RefreshHint{ResourceType: "change", ProjectID: projectID, ChangeID: changeID})
	if !writer.waitForContains("event: refresh", time.Second) {
		t.Fatalf("SSE did not send refresh event: %q", writer.String())
	}
	event := writer.String()
	if !strings.Contains(event, `"resource_type":"change"`) || !strings.Contains(event, `"project_id":"`+projectID+`"`) || !strings.Contains(event, `"change_id":"`+changeID+`"`) {
		t.Fatalf("SSE event = %q", event)
	}
	if strings.Contains(event, "snapshot") || strings.Contains(event, "cursor") || strings.Contains(event, "decision") {
		t.Fatalf("SSE event leaked non-refresh fields: %q", event)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop after request cancellation")
	}
}

type sseTestWriter struct {
	header http.Header
	mu     sync.Mutex
	body   bytes.Buffer
	notify chan struct{}
}

func newSSETestWriter() *sseTestWriter {
	return &sseTestWriter{header: make(http.Header), notify: make(chan struct{}, 8)}
}

func (w *sseTestWriter) Header() http.Header { return w.header }

func (w *sseTestWriter) WriteHeader(status int) { w.header.Set("X-Test-Status", strconv.Itoa(status)) }

func (w *sseTestWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	count, err := w.body.Write(value)
	select {
	case w.notify <- struct{}{}:
	default:
	}
	return count, err
}

func (w *sseTestWriter) Flush() {}

func (w *sseTestWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func (w *sseTestWriter) waitForContains(value string, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if strings.Contains(w.String(), value) {
			return true
		}
		select {
		case <-w.notify:
		case <-deadline.C:
			return false
		}
	}
}

package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/manifest"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/infrastructure/repository"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestProjectHTTPInitAndQueries(t *testing.T) {
	server, db, root := newProjectHTTPTestServer(t)
	handler := server.routes()

	for _, test := range []struct {
		name string
		body string
		key  string
	}{
		{name: "missing repository path", body: `{}`, key: "request-1"},
		{name: "relative repository path", body: `{"repository_path":"."}`, key: "request-2"},
		{name: "missing idempotency key", body: `{"repository_path":"` + root + `"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/projects/init", bytes.NewBufferString(test.body))
			if test.key != "" {
				request.Header.Set(controlplane.IdempotencyKeyHeader, test.key)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
			assertErrorCode(t, response, "invalid_request")
		})
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/projects/init", bytes.NewBufferString(`{"repository_path":"`+root+`"}`))
	request.Header.Set(controlplane.IdempotencyKeyHeader, "request-success")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("init status = %d, want 200", response.Code)
	}
	var initResponse controlplane.ProjectInitResponse
	decodeJSON(t, response, &initResponse)
	if err := controlplane.ValidateProjectID(initResponse.Project.ProjectID); err != nil {
		t.Fatalf("ProjectID validation error = %v", err)
	}
	if initResponse.Project.RepositoryRoot != root || initResponse.Project.ManifestPath != filepath.Join(root, ".keystone", "project.yaml") || initResponse.Project.CreatedAt == "" {
		t.Fatalf("project DTO = %+v", initResponse.Project)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/v1/projects/"+initResponse.Project.ProjectID, nil)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("project query status = %d, want 200", getResponse.Code)
	}
	var projectResponse controlplane.ProjectQueryResponse
	decodeJSON(t, getResponse, &projectResponse)
	if projectResponse.Project != initResponse.Project {
		t.Fatalf("queried project = %+v, want %+v", projectResponse.Project, initResponse.Project)
	}

	eventsRequest := httptest.NewRequest(http.MethodGet, "/v1/projects/"+initResponse.Project.ProjectID+"/events", nil)
	eventsResponse := httptest.NewRecorder()
	handler.ServeHTTP(eventsResponse, eventsRequest)
	if eventsResponse.Code != http.StatusOK {
		t.Fatalf("events query status = %d, want 200", eventsResponse.Code)
	}
	var events controlplane.ProjectEventsResponse
	decodeJSON(t, eventsResponse, &events)
	if len(events.Events) != 1 || events.Events[0].ProjectID != initResponse.Project.ProjectID || events.Events[0].Type != domain.ProjectInitializedType {
		t.Fatalf("events = %+v, want one ProjectInitialized event", events.Events)
	}

	emptyID := domain.NewProjectID()
	created := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO t_projects (project_id, repository_root, manifest_path, created_at) VALUES (?, ?, ?, ?)`, emptyID, "/tmp/empty-project", "/tmp/empty-project/.keystone/project.yaml", created); err != nil {
		t.Fatal(err)
	}
	emptyRequest := httptest.NewRequest(http.MethodGet, "/v1/projects/"+string(emptyID)+"/events", nil)
	emptyResponse := httptest.NewRecorder()
	handler.ServeHTTP(emptyResponse, emptyRequest)
	if emptyResponse.Code != http.StatusOK {
		t.Fatalf("empty events status = %d, want 200", emptyResponse.Code)
	}
	var emptyEvents controlplane.ProjectEventsResponse
	decodeJSON(t, emptyResponse, &emptyEvents)
	if emptyEvents.Events == nil || len(emptyEvents.Events) != 0 {
		t.Fatalf("empty events = %#v, want non-nil empty slice", emptyEvents.Events)
	}
}

func TestProjectHTTPErrorStatusMapping(t *testing.T) {
	server, _, _ := newProjectHTTPTestServer(t)
	handler := server.routes()
	request := httptest.NewRequest(http.MethodPost, "/v1/projects/init", bytes.NewBufferString(`{"repository_path":"`+t.TempDir()+`"}`))
	request.Header.Set(controlplane.IdempotencyKeyHeader, "unsupported-repository")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported repository status = %d, want 422", response.Code)
	}
	assertErrorCode(t, response, "repository_unsupported")
}

func newProjectHTTPTestServer(t *testing.T) (*Server, *sql.DB, string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:project-http-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migration.NewRunner(append(migration.DefaultMigrations(), workstore.Migrations()...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	state, err := workstore.New(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := work.NewService(repository.Git{}, manifest.Store{}, state)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	runProjectTestGit(t, root, "init")
	return &Server{projects: service}, db, root
}

func runProjectTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

func assertErrorCode(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	var envelope controlplane.ErrorEnvelope
	decodeJSON(t, response, &envelope)
	if envelope.Code != want {
		t.Fatalf("error code = %q, want %q", envelope.Code, want)
	}
}

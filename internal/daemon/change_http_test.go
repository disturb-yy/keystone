package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/artifact"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/infrastructure/manifest"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/infrastructure/repository"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestChangeHTTPCreateControlTraceAndArtifact(t *testing.T) {
	server, root := newChangeHTTPTestServer(t)
	planningWake := &recordingPlanningLifecycle{}
	server.planningManager = planningWake
	handler := server.routes()
	initRequest := httptest.NewRequest(http.MethodPost, "/v1/projects/init", bytes.NewBufferString(`{"repository_path":"`+root+`"}`))
	initRequest.Header.Set(controlplane.IdempotencyKeyHeader, "project-key")
	initResponse := httptest.NewRecorder()
	handler.ServeHTTP(initResponse, initRequest)
	if initResponse.Code != http.StatusOK {
		t.Fatalf("project init status = %d, body = %s", initResponse.Code, initResponse.Body.String())
	}
	var initialized controlplane.ProjectInitResponse
	decodeJSON(t, initResponse, &initialized)
	commitGit(t, root, "add project manifest")

	body := `{"repository_path":"` + root + `","intent":"  add change lifecycle  "}`
	createRequest := httptest.NewRequest(http.MethodPost, "/v1/changes", bytes.NewBufferString(body))
	createRequest.Header.Set(controlplane.IdempotencyKeyHeader, "change-key")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("change create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	var created controlplane.ChangeCreateResponse
	decodeJSON(t, createResponse, &created)
	if created.Change.Status != "active" || created.Change.Stage != "Intent" || created.Change.Version != 1 || created.Change.IntentArtifact.ArtifactRefID == "" {
		t.Fatalf("created change = %+v", created.Change)
	}
	if planningWake.wakeCount() != 1 {
		t.Fatalf("planning wake count after create = %d, want 1", planningWake.wakeCount())
	}

	replayRequest := httptest.NewRequest(http.MethodPost, "/v1/changes", bytes.NewBufferString(body))
	replayRequest.Header.Set(controlplane.IdempotencyKeyHeader, "change-key")
	replayResponse := httptest.NewRecorder()
	handler.ServeHTTP(replayResponse, replayRequest)
	var replay controlplane.ChangeCreateResponse
	decodeJSON(t, replayResponse, &replay)
	if replay.Change.ChangeID != created.Change.ChangeID {
		t.Fatalf("replay change id = %q, want %q", replay.Change.ChangeID, created.Change.ChangeID)
	}
	if err := os.WriteFile(filepath.Join(root, "late-untracked.txt"), []byte("dirty after success"), 0644); err != nil {
		t.Fatal(err)
	}
	dirtyReplayRequest := httptest.NewRequest(http.MethodPost, "/v1/changes", bytes.NewBufferString(body))
	dirtyReplayRequest.Header.Set(controlplane.IdempotencyKeyHeader, "change-key")
	dirtyReplayResponse := httptest.NewRecorder()
	handler.ServeHTTP(dirtyReplayResponse, dirtyReplayRequest)
	if dirtyReplayResponse.Code != http.StatusCreated {
		t.Fatalf("dirty replay status = %d, body = %s", dirtyReplayResponse.Code, dirtyReplayResponse.Body.String())
	}
	var dirtyReplay controlplane.ChangeCreateResponse
	decodeJSON(t, dirtyReplayResponse, &dirtyReplay)
	if dirtyReplay.Change.ChangeID != created.Change.ChangeID {
		t.Fatalf("dirty replay change id = %q, want %q", dirtyReplay.Change.ChangeID, created.Change.ChangeID)
	}
	if planningWake.wakeCount() != 3 {
		t.Fatalf("planning wake count after create replays = %d, want 3", planningWake.wakeCount())
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/v1/changes?repository_path="+url.QueryEscape(root), nil)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("change list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var listed controlplane.ChangeListResponse
	decodeJSON(t, listResponse, &listed)
	if len(listed.Changes) != 1 || listed.Changes[0].ChangeID != created.Change.ChangeID {
		t.Fatalf("listed changes = %+v", listed.Changes)
	}

	commandRequest := httptest.NewRequest(http.MethodPost, "/v1/changes/"+created.Change.ChangeID+"/commands", bytes.NewBufferString(`{"command":"pause","expected_version":1}`))
	commandRequest.Header.Set(controlplane.IdempotencyKeyHeader, "pause-key")
	commandResponse := httptest.NewRecorder()
	handler.ServeHTTP(commandResponse, commandRequest)
	if commandResponse.Code != http.StatusOK {
		t.Fatalf("pause status = %d, body = %s", commandResponse.Code, commandResponse.Body.String())
	}
	pauseBody := commandResponse.Body.String()
	var paused controlplane.ChangeCommandResponse
	decodeJSON(t, commandResponse, &paused)
	if paused.Change.Status != "paused" || paused.Change.Version != 2 {
		t.Fatalf("paused change = %+v", paused.Change)
	}
	if planningWake.wakeCount() != 3 {
		t.Fatalf("pause unexpectedly woke planning: %d", planningWake.wakeCount())
	}
	staleRequest := httptest.NewRequest(http.MethodPost, "/v1/changes/"+created.Change.ChangeID+"/commands", bytes.NewBufferString(`{"command":"resume","expected_version":1}`))
	staleRequest.Header.Set(controlplane.IdempotencyKeyHeader, "stale-key")
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, staleRequest)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale command status = %d, want 409", staleResponse.Code)
	}
	assertErrorCode(t, staleResponse, "change_version_conflict")

	resumeRequest := httptest.NewRequest(http.MethodPost, "/v1/changes/"+created.Change.ChangeID+"/commands", bytes.NewBufferString(`{"command":"resume","expected_version":2}`))
	resumeRequest.Header.Set(controlplane.IdempotencyKeyHeader, "resume-key")
	resumeResponse := httptest.NewRecorder()
	handler.ServeHTTP(resumeResponse, resumeRequest)
	if resumeResponse.Code != http.StatusOK {
		t.Fatalf("resume status = %d, body = %s", resumeResponse.Code, resumeResponse.Body.String())
	}
	var resumed controlplane.ChangeCommandResponse
	decodeJSON(t, resumeResponse, &resumed)
	if resumed.Change.Status != "active" || resumed.Change.Version != 3 {
		t.Fatalf("resumed change = %+v", resumed.Change)
	}
	if planningWake.wakeCount() != 4 {
		t.Fatalf("planning wake count after resume = %d, want 4", planningWake.wakeCount())
	}
	replayPauseRequest := httptest.NewRequest(http.MethodPost, "/v1/changes/"+created.Change.ChangeID+"/commands", bytes.NewBufferString(`{"command":"pause","expected_version":1}`))
	replayPauseRequest.Header.Set(controlplane.IdempotencyKeyHeader, "pause-key")
	replayPauseResponse := httptest.NewRecorder()
	handler.ServeHTTP(replayPauseResponse, replayPauseRequest)
	if replayPauseResponse.Code != http.StatusOK || replayPauseResponse.Body.String() != pauseBody {
		t.Fatalf("pause replay = (%d, %q), want (%d, %q)", replayPauseResponse.Code, replayPauseResponse.Body.String(), http.StatusOK, pauseBody)
	}

	artifactRequest := httptest.NewRequest(http.MethodGet, "/v1/changes/"+created.Change.ChangeID+"/artifacts", nil)
	artifactResponse := httptest.NewRecorder()
	handler.ServeHTTP(artifactResponse, artifactRequest)
	if artifactResponse.Code != http.StatusOK {
		t.Fatalf("artifact trace status = %d", artifactResponse.Code)
	}
	var artifacts controlplane.ChangeArtifactsResponse
	decodeJSON(t, artifactResponse, &artifacts)
	if len(artifacts.Artifacts) != 1 {
		t.Fatalf("artifact refs = %+v", artifacts.Artifacts)
	}

	contentRequest := httptest.NewRequest(http.MethodGet, "/v1/changes/"+created.Change.ChangeID+"/artifacts/"+artifacts.Artifacts[0].ArtifactRefID+"/content", nil)
	contentResponse := httptest.NewRecorder()
	handler.ServeHTTP(contentResponse, contentRequest)
	if contentResponse.Code != http.StatusOK || contentResponse.Body.String() != "  add change lifecycle  " {
		t.Fatalf("artifact content = (%d, %q)", contentResponse.Code, contentResponse.Body.String())
	}
	if contentResponse.Header().Get("ETag") == "" || contentResponse.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("artifact headers = %+v", contentResponse.Header())
	}

	eventsRequest := httptest.NewRequest(http.MethodGet, "/v1/changes/"+created.Change.ChangeID+"/events", nil)
	eventsResponse := httptest.NewRecorder()
	handler.ServeHTTP(eventsResponse, eventsRequest)
	var events controlplane.ChangeEventsResponse
	decodeJSON(t, eventsResponse, &events)
	if len(events.Events) != 3 || events.Events[0].Sequence != 1 || events.Events[1].Type != "ChangePaused" || events.Events[2].Type != "ChangeResumed" {
		t.Fatalf("events = %+v", events.Events)
	}
	projectEventsRequest := httptest.NewRequest(http.MethodGet, "/v1/projects/"+initialized.Project.ProjectID+"/events", nil)
	projectEventsResponse := httptest.NewRecorder()
	handler.ServeHTTP(projectEventsResponse, projectEventsRequest)
	if projectEventsResponse.Code != http.StatusOK {
		t.Fatalf("project events status = %d, body = %s", projectEventsResponse.Code, projectEventsResponse.Body.String())
	}
	var projectEvents controlplane.ProjectEventsResponse
	decodeJSON(t, projectEventsResponse, &projectEvents)
	if len(projectEvents.Events) != 1 || projectEvents.Events[0].Type != "ProjectInitialized" {
		t.Fatalf("project events = %+v, want only project initialization", projectEvents.Events)
	}
}

func TestTicketGraphHTTPErrorBoundaries(t *testing.T) {
	server, root := newChangeHTTPTestServer(t)
	handler := server.routes()

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/v1/changes/not-a-uuid/ticket-graph", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid ticket graph status = %d, want 400", invalid.Code)
	}
	assertErrorCode(t, invalid, "invalid_request")

	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, httptest.NewRequest(http.MethodGet, "/v1/changes/"+id.New()+"/ticket-graph", nil))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown ticket graph status = %d, want 404", unknown.Code)
	}
	assertErrorCode(t, unknown, "change_not_found")

	if _, err := server.projects.Initialize(context.Background(), work.InitializeRequest{RepositoryPath: root, IdempotencyKey: "ticket-graph-http-project"}); err != nil {
		t.Fatal(err)
	}
	commitGit(t, root, "add ticket graph http project manifest")
	change, err := server.changes.Create(context.Background(), work.ChangeCreateRequest{RepositoryPath: root, Intent: "ticket graph http boundary", IdempotencyKey: "ticket-graph-http-change", Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/changes/"+string(change.ID)+"/ticket-graph", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing ticket graph status = %d, want 404", missing.Code)
	}
	assertErrorCode(t, missing, "ticket_graph_not_found")

	method := httptest.NewRecorder()
	handler.ServeHTTP(method, httptest.NewRequest(http.MethodPost, "/v1/changes/"+string(change.ID)+"/ticket-graph", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ticket graph method status = %d, want 405", method.Code)
	}
}

func newChangeHTTPTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dsn := "file:change-http-" + filepath.Base(t.TempDir()) + "?mode=memory&cache=shared"
	db, err := sql.Open("sqlite", dsn)
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
	projects, err := work.NewService(repository.Git{}, manifest.Store{}, state)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	runProjectTestGit(t, root, "init")
	artifactStore, err := artifact.New(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	changes, err := work.NewChangeService(state, repository.Git{}, artifactStore, state)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{projects: projects, changes: changes}, root
}

func TestPlanningTraceMetadataDTOs(t *testing.T) {
	ref := domain.ArtifactRef{
		ID: domain.ArtifactRefID("ref-output"), ArtifactID: domain.ArtifactID("artifact-output"),
		Role: domain.ArtifactRoleOutput, Ordinal: 2, Kind: "understanding", SchemaVersion: "Understanding.v1",
		Summary: "需求理解", SourceRevision: "revision-1",
		InputArtifactRefIDs: []domain.ArtifactRefID{"ref-intent", "ref-context"}, RawLogArtifactRefIDs: []domain.ArtifactRefID{"ref-stdout"},
	}
	dto := artifactRefDTO(ref)
	if dto.Kind != ref.Kind || dto.SchemaVersion != ref.SchemaVersion || dto.Summary != ref.Summary || dto.SourceRevision != ref.SourceRevision {
		t.Fatalf("planning artifact metadata = %+v", dto)
	}
	if len(dto.InputArtifactRefIDs) != 2 || dto.InputArtifactRefIDs[0] != "ref-intent" || dto.InputArtifactRefIDs[1] != "ref-context" || len(dto.RawLogArtifactRefIDs) != 1 || dto.RawLogArtifactRefIDs[0] != "ref-stdout" {
		t.Fatalf("planning artifact links = %+v", dto)
	}

	runDTO := agentRunDTO(domain.AgentRun{
		ID: domain.AgentRunID("run-1"), ChangeID: domain.ChangeID("change-1"), Stage: domain.LifecycleStageUnderstand,
		Attempt: 1, RunKind: domain.AgentRunKindPlanning, SourceRevision: "revision-1", Status: domain.AgentRunStatusRunning, StartedAt: time.Unix(1, 0).UTC(),
	})
	if runDTO.RunKind != domain.AgentRunKindPlanning || runDTO.SourceRevision != "revision-1" {
		t.Fatalf("planning run metadata = %+v", runDTO)
	}
}

func commitGit(t *testing.T, root, message string) {
	t.Helper()
	runProjectTestGit(t, root, "add", ".keystone/project.yaml")
	runProjectTestGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", message)
}

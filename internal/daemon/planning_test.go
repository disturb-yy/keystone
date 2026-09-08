package daemon

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
	"github.com/disturb-yy/keystone/internal/planning"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestPlanningManagerRecoversAtStartupAndOnWake(t *testing.T) {
	recoverer := newRecordingPlanningRecoverer()
	manager, err := newPlanningManager(recoverer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForPlanningRecovery(t, recoverer.calls, 1)

	manager.Wake()
	waitForPlanningRecovery(t, recoverer.calls, 2)
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager.Wake()
	select {
	case call := <-recoverer.calls:
		t.Fatalf("stop 后仍执行第 %d 次 Planning 恢复", call)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestPlanningManagerPeriodicallyRecovers(t *testing.T) {
	recoverer := newRecordingPlanningRecoverer()
	manager, err := newPlanningManager(recoverer, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForPlanningRecovery(t, recoverer.calls, 1)
	waitForPlanningRecovery(t, recoverer.calls, 2)
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPlanningManagerRecordsAndClearsRecoverError(t *testing.T) {
	boom := errors.New("planning unavailable")
	recoverer := newRecordingPlanningRecoverer(nil, boom, nil)
	manager, err := newPlanningManager(recoverer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForPlanningRecovery(t, recoverer.calls, 1)
	manager.Wake()
	waitForPlanningRecovery(t, recoverer.calls, 2)
	waitForPlanningManagerError(t, manager, boom)
	manager.Wake()
	waitForPlanningRecovery(t, recoverer.calls, 3)
	waitForPlanningManagerError(t, manager, nil)
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPlanningManagerFailsStartupRecovery(t *testing.T) {
	boom := errors.New("durable scan failed")
	recoverer := newRecordingPlanningRecoverer(boom)
	manager, err := newPlanningManager(recoverer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Start() error = %v, want %v", err, boom)
	}
	if !errors.Is(manager.Err(), boom) {
		t.Fatalf("Err() = %v, want %v", manager.Err(), boom)
	}
}

func TestPlanningDispatcherMapsCapabilityAndAssignment(t *testing.T) {
	authority := &recordingPlanningAuthority{workerID: "worker-1", available: true}
	dispatcher := planningDispatcher{authority: authority}
	target, available, err := dispatcher.Select(context.Background(), planning.RuntimeCapabilityPlanningReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if target != "worker-1" || !available || authority.capability != "runtime:codex" {
		t.Fatalf("Select() = (%q, %v, %q)", target, available, authority.capability)
	}

	runID := domain.AgentRunID("run-1")
	err = dispatcher.Dispatch(context.Background(), planning.DispatchRequest{
		Target: "worker-1", AgentRunID: runID, Attempt: 3, WorkspacePath: "/tmp/snapshot",
		Instruction: "produce a candidate", BeforeRevision: "abc123",
		Inputs: []planning.DispatchArtifact{{Kind: "intent", SHA256: "digest", SizeBytes: 7, MediaType: "text/plain"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if authority.runID != runID || authority.workerIDArg != "worker-1" || authority.runtime != "codex" || authority.workspaceID != "planning-run-1" {
		t.Fatalf("assignment identity = %+v", authority)
	}
	if authority.workspacePath != "/tmp/snapshot" || authority.instruction != "produce a candidate" || authority.beforeRevision != "abc123" {
		t.Fatalf("assignment execution input = %+v", authority)
	}
	if len(authority.inputs) != 1 || authority.inputs[0].Kind != "intent" || authority.inputs[0].SHA256 != "digest" || authority.inputs[0].SizeBytes != 7 {
		t.Fatalf("assignment artifacts = %+v", authority.inputs)
	}
}

func TestPlanningDispatcherRejectsUnsupportedCapability(t *testing.T) {
	authority := &recordingPlanningAuthority{}
	_, _, err := (planningDispatcher{authority: authority}).Select(context.Background(), planning.RuntimeCapability("planning.write.v1"))
	if err == nil || authority.capability != "" {
		t.Fatalf("Select() error = %v, capability = %q", err, authority.capability)
	}
}

func TestRoutesDoNotExposePlanningStart(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/planning/start", nil)
	(&Server{}).routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("POST /v1/planning/start status = %d, want 404", recorder.Code)
	}
}

func TestServerComposesAndStartsPlanningRecovery(t *testing.T) {
	server := New(t.TempDir(), Options{})
	paths, err := localstate.Resolve(server.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Initialize(); err != nil {
		t.Fatal(err)
	}
	server.setPaths(paths)
	if err := server.openAndMigrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if server.planning == nil {
		t.Fatal("planning coordinator was not composed")
	}
	version, err := readMigrationVersion(context.Background(), server.db)
	if err != nil || version != 6 {
		t.Fatalf("schema migration version = %d, err = %v, want 6", version, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := server.startPlanningManager(ctx); err != nil {
		t.Fatal(err)
	}
	server.setReadiness(true)
	manager := server.planningManager
	if manager == nil {
		t.Fatal("planning manager was not started")
	}
	if err := manager.Err(); err != nil {
		t.Fatalf("planning manager error = %v", err)
	}
	cancel()
	if err := server.shutdownResources(); err != nil {
		t.Fatal(err)
	}
}

func TestServerShutdownStopsPlanningBeforeDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", "file:planning-shutdown?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	planningLifecycle := &recordingPlanningLifecycle{
		stop: func(ctx context.Context) error {
			return db.PingContext(ctx)
		},
	}
	server := New(t.TempDir(), Options{})
	server.db = db
	server.planningManager = planningLifecycle
	if err := server.shutdownResources(); err != nil {
		t.Fatal(err)
	}
	if planningLifecycle.stopCount() != 1 {
		t.Fatalf("planning stop count = %d, want 1", planningLifecycle.stopCount())
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("database remained open after shutdown")
	}
}

func TestPlanningSnapshotCleanupDefersWhenManagerDidNotStop(t *testing.T) {
	closer := &recordingPlanningCloser{}
	err := closePlanningCoordinatorAfterStops(errors.New("stop timeout"), closer)
	if !errors.Is(err, errPlanningSnapshotCleanupDeferred) {
		t.Fatalf("closePlanningCoordinatorAfterStops() error = %v", err)
	}
	if closer.calls != 0 {
		t.Fatalf("planning snapshot close calls = %d, want 0", closer.calls)
	}
	if err := closePlanningCoordinatorAfterStops(nil, closer); err != nil {
		t.Fatal(err)
	}
	if closer.calls != 1 {
		t.Fatalf("planning snapshot close calls = %d, want 1", closer.calls)
	}
}

type recordingPlanningCloser struct {
	calls int
}

func (c *recordingPlanningCloser) Close() error {
	c.calls++
	return nil
}

type recordingPlanningRecoverer struct {
	mu        sync.Mutex
	responses []error
	count     int
	calls     chan int
}

func newRecordingPlanningRecoverer(responses ...error) *recordingPlanningRecoverer {
	return &recordingPlanningRecoverer{responses: responses, calls: make(chan int, 16)}
}

func (r *recordingPlanningRecoverer) Recover(context.Context) error {
	r.mu.Lock()
	r.count++
	call := r.count
	var err error
	if call <= len(r.responses) {
		err = r.responses[call-1]
	}
	r.mu.Unlock()
	r.calls <- call
	return err
}

func waitForPlanningRecovery(t *testing.T, calls <-chan int, want int) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("Planning recovery call = %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("Planning recovery call %d did not happen", want)
	}
}

func waitForPlanningManagerError(t *testing.T, manager *planningManager, want error) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(manager.Err(), want) && (want != nil || manager.Err() == nil) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Planning manager error = %v, want %v", manager.Err(), want)
}

type recordingPlanningAuthority struct {
	capability string
	workerID   string
	available  bool

	runID          domain.AgentRunID
	workerIDArg    string
	workspacePath  string
	runtime        string
	instruction    string
	beforeRevision string
	workspaceID    string
	inputs         []workercontract.ArtifactSummary
}

func (a *recordingPlanningAuthority) PlanningRunAssigned(_ context.Context, runID domain.AgentRunID) (bool, error) {
	return a.runID == runID && runID != "", nil
}

func (a *recordingPlanningAuthority) AvailableWorker(_ context.Context, capability string) (string, bool, error) {
	a.capability = capability
	return a.workerID, a.available, nil
}

func (a *recordingPlanningAuthority) IssuePlanningAssignment(_ context.Context, runID domain.AgentRunID, workerID, workspacePath, runtime, instruction, beforeRevision, workspaceID string, inputs []workercontract.ArtifactSummary) (workercontract.Assignment, error) {
	a.runID = runID
	a.workerIDArg = workerID
	a.workspacePath = workspacePath
	a.runtime = runtime
	a.instruction = instruction
	a.beforeRevision = beforeRevision
	a.workspaceID = workspaceID
	a.inputs = append([]workercontract.ArtifactSummary(nil), inputs...)
	return workercontract.Assignment{}, nil
}

type recordingPlanningLifecycle struct {
	mu    sync.Mutex
	wakes int
	err   error
	stop  func(context.Context) error
	stops int
}

func (l *recordingPlanningLifecycle) Wake() {
	l.mu.Lock()
	l.wakes++
	l.mu.Unlock()
}

func (l *recordingPlanningLifecycle) Stop(ctx context.Context) error {
	l.mu.Lock()
	l.stops++
	stop := l.stop
	l.mu.Unlock()
	if stop != nil {
		return stop(ctx)
	}
	return nil
}

func (l *recordingPlanningLifecycle) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *recordingPlanningLifecycle) wakeCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.wakes
}

func (l *recordingPlanningLifecycle) stopCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stops
}

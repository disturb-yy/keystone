package daemon

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWorkerSupervisorStopReportsMissingCompletion(t *testing.T) {
	supervisor := &WorkerSupervisor{
		options: WorkerSupervisorOptions{ShutdownTimeout: time.Millisecond},
		cancel:  func() {},
		done:    make(chan struct{}),
	}
	if err := supervisor.Stop(context.Background()); err == nil {
		t.Fatal("Stop() hid an unresponsive supervisor")
	}
	if err := supervisor.Stop(context.Background()); err == nil {
		t.Fatal("repeated Stop() lost the terminal stop error")
	}
}

func TestWorkerSupervisorStopObservesCompletion(t *testing.T) {
	done := make(chan struct{})
	supervisor := &WorkerSupervisor{
		options: WorkerSupervisorOptions{ShutdownTimeout: time.Second},
		cancel:  func() { close(done) },
		done:    done,
	}
	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPlanningSnapshotCleanupDefersAfterWorkerStopFailure(t *testing.T) {
	closer := &recordingPlanningCloser{}
	err := closePlanningCoordinatorAfterStops(errors.New("worker did not stop"), closer)
	if !errors.Is(err, errPlanningSnapshotCleanupDeferred) || closer.calls != 0 {
		t.Fatalf("snapshot cleanup error=%v calls=%d", err, closer.calls)
	}
}

func TestWorkerSupervisorRetriesLostReconciliationBeforeContinuing(t *testing.T) {
	authority := &recordingSupervisorAuthority{reconcileErrors: []error{errors.New("temporary database failure"), nil}}
	supervisor := NewWorkerSupervisor(WorkerSupervisorOptions{
		Store: authority, ShutdownTimeout: time.Second,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	if err := supervisor.reconcileWorkerLost(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}
	if authority.reconcileCalls != 2 {
		t.Fatalf("reconcile calls = %d, want 2", authority.reconcileCalls)
	}
	if err := supervisor.Err(); err != nil {
		t.Fatalf("supervisor retained recovered error: %v", err)
	}
}

func TestWorkerSupervisorLostReconciliationUsesCancellableContext(t *testing.T) {
	authority := &recordingSupervisorAuthority{blockUntilCancelled: true}
	supervisor := NewWorkerSupervisor(WorkerSupervisorOptions{
		Store: authority, ShutdownTimeout: 10 * time.Millisecond,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := supervisor.reconcileWorkerLost(ctx, "worker-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("reconcileWorkerLost() error = %v, want context cancellation", err)
	}
}

type recordingSupervisorAuthority struct {
	mu                  sync.Mutex
	reconcileErrors     []error
	reconcileCalls      int
	blockUntilCancelled bool
}

func (*recordingSupervisorAuthority) PrepareWorker(context.Context, string, string) error { return nil }
func (*recordingSupervisorAuthority) WorkerProcessLost(context.Context, string) (bool, error) {
	return false, nil
}

func (a *recordingSupervisorAuthority) ReconcileWorkerLost(ctx context.Context, _ string) error {
	a.mu.Lock()
	a.reconcileCalls++
	block := a.blockUntilCancelled
	var err error
	if len(a.reconcileErrors) != 0 {
		err = a.reconcileErrors[0]
		a.reconcileErrors = a.reconcileErrors[1:]
	}
	a.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}

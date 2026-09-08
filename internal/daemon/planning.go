package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/repository"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
	"github.com/disturb-yy/keystone/internal/planning"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	planningRecoveryInterval = 500 * time.Millisecond
	planningRuntimeCodex     = "codex"
)

type planningRecoverer interface {
	Recover(context.Context) error
}

type planningLifecycle interface {
	Wake()
	Stop(context.Context) error
	Err() error
}

// planningManager 将进程内唤醒视为提示，并始终让 Coordinator 从耐久状态重新判断。
type planningManager struct {
	recoverer planningRecoverer
	interval  time.Duration
	wake      chan struct{}

	mu      sync.RWMutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
	lastErr error
}

func newPlanningManager(recoverer planningRecoverer, interval time.Duration) (*planningManager, error) {
	if recoverer == nil || interval <= 0 {
		return nil, errors.New("create planning manager: recoverer and positive interval are required")
	}
	return &planningManager{recoverer: recoverer, interval: interval, wake: make(chan struct{}, 1)}, nil
}

// Start 先同步完成一次启动恢复，再启动周期与事件驱动的耐久扫描。
func (m *planningManager) Start(ctx context.Context) error {
	if m == nil || ctx == nil {
		return errors.New("start planning manager: manager and context are required")
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return errors.New("start planning manager: already started")
	}
	runContext, cancel := context.WithCancel(ctx)
	m.started = true
	m.cancel = cancel
	m.done = make(chan struct{})
	done := m.done
	m.mu.Unlock()

	if err := m.recoverer.Recover(runContext); err != nil {
		m.setErr(err)
		cancel()
		close(done)
		return fmt.Errorf("recover planning at startup: %w", err)
	}
	m.setErr(nil)
	go m.run(runContext, done)
	return nil
}

// Wake 合并高频事件；事件只缩短下一次耐久扫描的等待时间。
func (m *planningManager) Wake() {
	if m == nil {
		return
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Err 返回最近一次尚未被成功扫描清除的恢复错误。
func (m *planningManager) Err() error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastErr
}

// Stop 停止新的调度判断，并等待当前扫描观察到取消。
func (m *planningManager) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	started, cancel, done := m.started, m.cancel, m.done
	m.mu.RUnlock()
	if !started || cancel == nil || done == nil {
		return nil
	}
	cancel()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop planning manager: %w", ctx.Err())
	}
}

func (m *planningManager) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
		err := m.recoverer.Recover(ctx)
		if ctx.Err() != nil {
			return
		}
		m.setErr(err)
	}
}

func (m *planningManager) setErr(err error) {
	m.mu.Lock()
	m.lastErr = err
	m.mu.Unlock()
}

type planningSnapshotMaterializer struct {
	git repository.Git
}

func (m planningSnapshotMaterializer) Materialize(ctx context.Context, repositoryRoot, baseRevision string) (planning.Snapshot, error) {
	snapshot, err := m.git.MaterializeSnapshot(ctx, repositoryRoot, baseRevision)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

type planningWorkerAuthority interface {
	AvailableWorker(context.Context, string) (string, bool, error)
	PlanningRunAssigned(context.Context, domain.AgentRunID) (bool, error)
	IssuePlanningAssignment(context.Context, domain.AgentRunID, string, string, string, string, string, string, []workercontract.ArtifactSummary) (workercontract.Assignment, error)
}

type planningDispatcher struct {
	authority planningWorkerAuthority
}

func (d planningDispatcher) Select(ctx context.Context, capability planning.RuntimeCapability) (string, bool, error) {
	if d.authority == nil {
		return "", false, errors.New("select planning worker: authority is required")
	}
	runtime, err := runtimeForPlanningCapability(capability)
	if err != nil {
		return "", false, err
	}
	workerID, available, err := d.authority.AvailableWorker(ctx, "runtime:"+runtime)
	if err != nil {
		return "", false, fmt.Errorf("select planning worker: %w", err)
	}
	return workerID, available, nil
}

func (d planningDispatcher) Dispatch(ctx context.Context, request planning.DispatchRequest) error {
	if d.authority == nil || strings.TrimSpace(request.Target) == "" {
		return errors.New("dispatch planning assignment: authority and target are required")
	}
	inputs := make([]workercontract.ArtifactSummary, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		inputs = append(inputs, workercontract.ArtifactSummary{
			Kind: input.Kind, SHA256: input.SHA256, SizeBytes: input.SizeBytes, MediaType: input.MediaType,
		})
	}
	_, err := d.authority.IssuePlanningAssignment(
		ctx,
		request.AgentRunID,
		request.Target,
		request.WorkspacePath,
		planningRuntimeCodex,
		request.Instruction,
		request.BeforeRevision,
		"planning-"+string(request.AgentRunID),
		inputs,
	)
	if err != nil {
		if errors.Is(err, workstore.ErrWorkerAssignmentConflict) {
			return fmt.Errorf("dispatch planning assignment: %w", planning.ErrDispatchFenced)
		}
		return fmt.Errorf("dispatch planning assignment: %w", err)
	}
	return nil
}

func (d planningDispatcher) Assigned(ctx context.Context, runID domain.AgentRunID) (bool, error) {
	if d.authority == nil {
		return false, errors.New("inspect planning assignment: authority is required")
	}
	return d.authority.PlanningRunAssigned(ctx, runID)
}

func runtimeForPlanningCapability(capability planning.RuntimeCapability) (string, error) {
	if capability != planning.RuntimeCapabilityPlanningReadOnly {
		return "", fmt.Errorf("select planning runtime: unsupported capability %q", capability)
	}
	return planningRuntimeCodex, nil
}

package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/disturb-yy/keystone/internal/execution"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
)

const (
	workerRestartInitialBackoff = time.Second
	workerRestartMaxBackoff     = 30 * time.Second
	workerShutdownTimeout       = 4 * time.Second
)

var errWorkerExecutableUnavailable = errors.New("keystone-worker executable is unavailable")
var errWorkerHeartbeatExpired = errors.New("worker heartbeat or lease expired")

// WorkerSupervisorStore 是子进程身份准备与退出收敛所需的最小 authority。
type WorkerSupervisorStore interface {
	PrepareWorker(context.Context, string, string) error
	WorkerProcessLost(context.Context, string) (bool, error)
	ReconcileWorkerLost(context.Context, string) error
}

// WorkerCommandFactory 是 Worker 子进程启动 seam；secret 只通过 stdin 管道传递。
type WorkerCommandFactory func(context.Context, string, ...string) *exec.Cmd

// WorkerSupervisorOptions 配置 Daemon readiness 后的单 Worker 监管。
type WorkerSupervisorOptions struct {
	// Disable 禁止自动发现并启动 keystone-worker，主要用于显式关闭本机副作用进程。
	Disable bool
	// Endpoint 是 Worker Protocol 的 loopback endpoint。
	Endpoint string
	// Store 是持有 WorkerInstance、Lease 和 AgentRun authority 的同一 Work Store。
	Store WorkerSupervisorStore
	// Executable 为空时按 Daemon 同目录、PATH 顺序发现 keystone-worker。
	Executable string
	// Command 覆盖真实 exec.Cmd 构造，用于监管测试。
	Command WorkerCommandFactory
	// Sleep 覆盖崩溃重启退避等待。
	Sleep func(context.Context, time.Duration) error
	// ShutdownTimeout 是停止 Worker 的有界宽限期。
	ShutdownTimeout time.Duration
	// WatchInterval 是 Supervisor 复检耐久 heartbeat/Lease 的周期。
	WatchInterval time.Duration
}

// WorkerSupervisor 保持最多一个独立 Worker 子进程，并为每次启动生成新身份。
type WorkerSupervisor struct {
	options WorkerSupervisorOptions

	mu       sync.Mutex
	cancel   context.CancelFunc
	current  *execution.ProcessTree
	workerID string
	done     chan struct{}
	stopOnce sync.Once
	stopErr  error
	lastErr  error
}

// NewWorkerSupervisor 创建尚未启动的 Supervisor。
func NewWorkerSupervisor(options WorkerSupervisorOptions) *WorkerSupervisor {
	if options.ShutdownTimeout <= 0 {
		// Worker 收到取消后还要等待 Runtime 自己清扫 Codex 子树；宽限期
		// 必须覆盖这一层，Supervisor 才能在释放 Snapshot 前观察到 Worker 退出。
		options.ShutdownTimeout = workerShutdownTimeout
	}
	if options.WatchInterval <= 0 {
		options.WatchInterval = workstore.WorkerHeartbeatInterval()
	}
	if options.Sleep == nil {
		options.Sleep = sleepSupervisor
	}
	if options.Command == nil {
		options.Command = exec.CommandContext
	}
	return &WorkerSupervisor{options: options}
}

// Start 启动非阻塞 Supervisor；缺少 sibling/PATH executable 时返回可分类错误。
func (s *WorkerSupervisor) Start(ctx context.Context) error {
	if s == nil || s.options.Store == nil || strings.TrimSpace(s.options.Endpoint) == "" {
		return errors.New("start worker supervisor: authority and endpoint are required")
	}
	if s.options.Disable {
		return nil
	}
	executable, err := discoverWorkerExecutable(s.options.Executable)
	if err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("start worker supervisor: nil context")
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return errors.New("start worker supervisor: already started")
	}
	workerContext, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()
	go s.run(workerContext, executable)
	return nil
}

// Stop 先请求子进程退出，再在宽限期后终止；调用方随后才关闭 Daemon 资源。
func (s *WorkerSupervisor) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		s.stopErr = s.stop(ctx)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopErr
}

// Err 返回阻止安全启动下一 Worker 的最近一次监管错误。
func (s *WorkerSupervisor) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *WorkerSupervisor) stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	if done == nil {
		return errors.New("stop worker supervisor: completion signal is unavailable")
	}
	waitParent := ctx
	if waitParent == nil {
		waitParent = context.Background()
	}
	wait, stopWaiting := context.WithTimeout(waitParent, s.options.ShutdownTimeout)
	defer stopWaiting()
	select {
	case <-done:
		return nil
	case <-wait.Done():
	}
	return fmt.Errorf("stop worker supervisor: %w", wait.Err())
}

func (s *WorkerSupervisor) run(ctx context.Context, executable string) {
	defer close(s.done)
	backoff := workerRestartInitialBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		workerID := id.New()
		secret, err := workstore.NewWorkerSecret()
		prepared := false
		if err == nil {
			err = s.options.Store.PrepareWorker(ctx, workerID, secret)
			prepared = err == nil
		}
		if err == nil {
			s.setErr(nil)
			err = s.runProcess(ctx, executable, workerID, secret)
		}
		if ctx.Err() != nil {
			return
		}
		if prepared {
			if reconcileErr := s.reconcileWorkerLost(ctx, workerID); reconcileErr != nil {
				if ctx.Err() != nil {
					return
				}
				s.setErr(reconcileErr)
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			backoff = workerRestartInitialBackoff
		} else {
			s.setErr(err)
			if backoff < workerRestartMaxBackoff {
				backoff *= 2
				if backoff > workerRestartMaxBackoff {
					backoff = workerRestartMaxBackoff
				}
			}
		}
		if err := s.options.Sleep(ctx, backoff); err != nil {
			return
		}
	}
}

func (s *WorkerSupervisor) reconcileWorkerLost(ctx context.Context, workerID string) error {
	backoff := workerRestartInitialBackoff
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, s.options.ShutdownTimeout)
		err := s.options.Store.ReconcileWorkerLost(attemptCtx, workerID)
		cancel()
		if err == nil {
			s.setErr(nil)
			return nil
		}
		wrapped := fmt.Errorf("reconcile lost worker: %w", err)
		s.setErr(wrapped)
		if ctx.Err() != nil {
			return errors.Join(wrapped, ctx.Err())
		}
		if err := s.options.Sleep(ctx, backoff); err != nil {
			return errors.Join(wrapped, err)
		}
		if backoff < workerRestartMaxBackoff {
			backoff *= 2
			if backoff > workerRestartMaxBackoff {
				backoff = workerRestartMaxBackoff
			}
		}
	}
}

func (s *WorkerSupervisor) setErr(err error) {
	s.mu.Lock()
	s.lastErr = err
	s.mu.Unlock()
}

func (s *WorkerSupervisor) runProcess(ctx context.Context, executable, workerID, secret string) error {
	// Supervisor 自己管理整棵进程树；不能让 CommandContext 在 Daemon cancel 时
	// 先于 Worker 的优雅取消路径单杀父进程。
	command := s.options.Command(context.WithoutCancel(ctx), executable, "--daemon-endpoint", s.options.Endpoint, "--worker-id", workerID)
	if command == nil {
		return errors.New("create worker process: nil command")
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("create worker startup pipe: %w", err)
	}
	tree, err := execution.StartProcessTree(command)
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start worker process: %w", err)
	}
	s.mu.Lock()
	s.current = tree
	s.workerID = workerID
	s.mu.Unlock()
	defer func() {
		_ = tree.Wait()
		s.mu.Lock()
		if s.current == tree {
			s.current = nil
			s.workerID = ""
		}
		s.mu.Unlock()
	}()
	_, writeErr := io.WriteString(stdin, secret+"\n")
	closeErr := stdin.Close()
	if writeErr != nil {
		return errors.Join(fmt.Errorf("write worker startup pipe: %w", writeErr), execution.TerminateProcessTree(tree, s.processStopBudget()))
	}
	if closeErr != nil {
		return errors.Join(fmt.Errorf("close worker startup pipe: %w", closeErr), execution.TerminateProcessTree(tree, s.processStopBudget()))
	}
	ticker := time.NewTicker(s.options.WatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-tree.Done():
			if waitErr := tree.Wait(); waitErr != nil {
				return fmt.Errorf("worker process exited: %w", waitErr)
			}
			return nil
		case <-ctx.Done():
			return errors.Join(ctx.Err(), execution.TerminateProcessTree(tree, s.processStopBudget()))
		case <-ticker.C:
			probeCtx, cancel := context.WithTimeout(ctx, s.options.WatchInterval)
			lost, probeErr := s.options.Store.WorkerProcessLost(probeCtx, workerID)
			cancel()
			if probeErr != nil {
				s.setErr(fmt.Errorf("inspect worker liveness: %w", probeErr))
				continue
			}
			s.setErr(nil)
			if !lost {
				continue
			}
			return errors.Join(errWorkerHeartbeatExpired, execution.TerminateProcessTree(tree, s.processStopBudget()))
		}
	}
}

func (s *WorkerSupervisor) processStopBudget() time.Duration {
	budget := s.options.ShutdownTimeout
	if budget <= 0 {
		return time.Second
	}
	return budget
}

func discoverWorkerExecutable(explicit string) (string, error) {
	if explicit != "" {
		if filepath.IsAbs(explicit) {
			if _, err := os.Stat(explicit); err != nil {
				return "", fmt.Errorf("discover worker executable: %w", errWorkerExecutableUnavailable)
			}
			return explicit, nil
		}
		path, err := exec.LookPath(explicit)
		if err != nil {
			return "", fmt.Errorf("discover worker executable: %w", errWorkerExecutableUnavailable)
		}
		return path, nil
	}
	if executable, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(executable), workerExecutableName())
		if _, statErr := os.Stat(sibling); statErr == nil {
			return sibling, nil
		}
	}
	path, err := exec.LookPath(workerExecutableName())
	if err != nil {
		return "", fmt.Errorf("discover worker executable: %w", errWorkerExecutableUnavailable)
	}
	return path, nil
}

func workerExecutableName() string {
	if os.PathSeparator == '\\' {
		return "keystone-worker.exe"
	}
	return "keystone-worker"
}

func sleepSupervisor(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

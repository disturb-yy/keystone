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

	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
)

const (
	workerRestartInitialBackoff = time.Second
	workerRestartMaxBackoff     = 30 * time.Second
)

var errWorkerExecutableUnavailable = errors.New("keystone-worker executable is unavailable")

// WorkerCommandFactory 是 Worker 子进程启动 seam；secret 只通过 stdin 管道传递。
type WorkerCommandFactory func(context.Context, string, ...string) *exec.Cmd

// WorkerSupervisorOptions 配置 Daemon readiness 后的单 Worker 监管。
type WorkerSupervisorOptions struct {
	// Disable 禁止自动发现并启动 keystone-worker，主要用于显式关闭本机副作用进程。
	Disable bool
	// Endpoint 是 Worker Protocol 的 loopback endpoint。
	Endpoint string
	// Store 是持有 WorkerInstance、Lease 和 AgentRun authority 的同一 Work Store。
	Store *workstore.Store
	// Executable 为空时按 Daemon 同目录、PATH 顺序发现 keystone-worker。
	Executable string
	// Command 覆盖真实 exec.Cmd 构造，用于监管测试。
	Command WorkerCommandFactory
	// Sleep 覆盖崩溃重启退避等待。
	Sleep func(context.Context, time.Duration) error
	// ShutdownTimeout 是停止 Worker 的有界宽限期。
	ShutdownTimeout time.Duration
}

// WorkerSupervisor 保持最多一个独立 Worker 子进程，并为每次启动生成新身份。
type WorkerSupervisor struct {
	options WorkerSupervisorOptions

	mu       sync.Mutex
	cancel   context.CancelFunc
	current  *exec.Cmd
	workerID string
	done     chan struct{}
	stopOnce sync.Once
}

// NewWorkerSupervisor 创建尚未启动的 Supervisor。
func NewWorkerSupervisor(options WorkerSupervisorOptions) *WorkerSupervisor {
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = 2 * time.Second
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
		s.mu.Lock()
		cancel, current, done := s.cancel, s.current, s.done
		s.mu.Unlock()
		if cancel == nil {
			return
		}
		cancel()
		if current != nil && current.Process != nil {
			_ = current.Process.Signal(os.Interrupt)
		}
		if done == nil {
			return
		}
		wait := ctx
		if wait == nil {
			wait = context.Background()
		}
		select {
		case <-done:
		case <-wait.Done():
			if current != nil && current.Process != nil {
				_ = current.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	})
	return nil
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
		if err == nil {
			err = s.options.Store.PrepareWorker(ctx, workerID, secret)
		}
		if err == nil {
			err = s.runProcess(ctx, executable, workerID, secret)
		}
		if ctx.Err() != nil {
			return
		}
		_ = s.options.Store.ReconcileWorkerLost(context.Background(), workerID)
		if err == nil {
			backoff = workerRestartInitialBackoff
		} else if backoff < workerRestartMaxBackoff {
			backoff *= 2
			if backoff > workerRestartMaxBackoff {
				backoff = workerRestartMaxBackoff
			}
		}
		if err := s.options.Sleep(ctx, backoff); err != nil {
			return
		}
	}
}

func (s *WorkerSupervisor) runProcess(ctx context.Context, executable, workerID, secret string) error {
	command := s.options.Command(ctx, executable, "--daemon-endpoint", s.options.Endpoint, "--worker-id", workerID)
	if command == nil {
		return errors.New("create worker process: nil command")
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("create worker startup pipe: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start worker process: %w", err)
	}
	s.mu.Lock()
	s.current = command
	s.workerID = workerID
	s.mu.Unlock()
	_, writeErr := io.WriteString(stdin, secret+"\n")
	closeErr := stdin.Close()
	waitErr := command.Wait()
	s.mu.Lock()
	s.current = nil
	s.workerID = ""
	s.mu.Unlock()
	if writeErr != nil {
		return fmt.Errorf("write worker startup pipe: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close worker startup pipe: %w", closeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("worker process exited: %w", waitErr)
	}
	return nil
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

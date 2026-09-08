package execution

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// ProcessTree 持有一个独立 OS 进程树；Done 关闭前，父进程已 Wait 且残留子进程已被清扫。
type ProcessTree struct {
	command *exec.Cmd
	control processTreeControl
	done    chan struct{}

	mu      sync.RWMutex
	waitErr error
}

type processTreeControl interface {
	Bind(*exec.Cmd) error
	Interrupt() error
	Kill() error
	Close() error
}

// StartProcessTree 启动 command，并在 command 可以派生子进程前建立平台级进程树边界。
func StartProcessTree(command *exec.Cmd) (*ProcessTree, error) {
	if command == nil {
		return nil, errors.New("start process tree: command is required")
	}
	control, err := prepareProcessTree(command)
	if err != nil {
		return nil, fmt.Errorf("prepare process tree: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, errors.Join(fmt.Errorf("start process tree: %w", err), control.Close())
	}
	if err := control.Bind(command); err != nil {
		killErr := command.Process.Kill()
		waitErr := command.Wait()
		return nil, errors.Join(fmt.Errorf("bind process tree: %w", err), killErr, waitErr, control.Close())
	}
	tree := &ProcessTree{command: command, control: control, done: make(chan struct{})}
	go tree.wait()
	return tree, nil
}

// Done 在父进程已回收且平台进程树清扫完成后关闭。
func (p *ProcessTree) Done() <-chan struct{} {
	if p == nil || p.done == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return p.done
}

// Wait 等待进程树完全停止，并返回父进程或清扫错误。
func (p *ProcessTree) Wait() error {
	if p == nil {
		return nil
	}
	<-p.Done()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.waitErr
}

// Interrupt 请求整棵进程树优雅停止。
func (p *ProcessTree) Interrupt() error {
	if p == nil || p.control == nil || processTreeDone(p.Done()) {
		return nil
	}
	return p.control.Interrupt()
}

// Kill 强制停止整棵进程树。
func (p *ProcessTree) Kill() error {
	if p == nil || p.control == nil || processTreeDone(p.Done()) {
		return nil
	}
	return p.control.Kill()
}

// TerminateProcessTree 先请求优雅退出，再在总预算的后半段强制终止并等待清扫。
func TerminateProcessTree(tree *ProcessTree, timeout time.Duration) error {
	if tree == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	interruptErr := tree.Interrupt()
	grace := timeout / 2
	if grace <= 0 {
		grace = timeout
	}
	timer := time.NewTimer(grace)
	select {
	case <-tree.Done():
		timer.Stop()
		return tree.Wait()
	case <-timer.C:
	}
	killErr := tree.Kill()
	remaining := timeout - grace
	if remaining <= 0 {
		remaining = grace
	}
	timer = time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-tree.Done():
		return errors.Join(tree.Wait(), interruptErr, killErr)
	case <-timer.C:
		return errors.Join(errors.New("terminate process tree: timed out"), interruptErr, killErr)
	}
}

func (p *ProcessTree) wait() {
	waitErr := p.command.Wait()
	cleanupErr := p.control.Close()
	p.mu.Lock()
	p.waitErr = errors.Join(waitErr, cleanupErr)
	p.mu.Unlock()
	close(p.done)
}

func processTreeDone(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

//go:build !windows

package execution

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
)

type unixProcessTreeControl struct {
	mu   sync.Mutex
	pgid int
}

func prepareProcessTree(command *exec.Cmd) (processTreeControl, error) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
	command.SysProcAttr.Pgid = 0
	return &unixProcessTreeControl{}, nil
}

func (c *unixProcessTreeControl) Bind(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return errors.New("process is unavailable")
	}
	c.mu.Lock()
	c.pgid = command.Process.Pid
	c.mu.Unlock()
	return nil
}

func (c *unixProcessTreeControl) Interrupt() error { return c.signal(syscall.SIGTERM) }
func (c *unixProcessTreeControl) Kill() error      { return c.signal(syscall.SIGKILL) }

func (c *unixProcessTreeControl) Close() error {
	// 父进程正常退出也不能留下仍访问 Workspace 的后代。
	return c.Kill()
}

func (c *unixProcessTreeControl) signal(signal syscall.Signal) error {
	c.mu.Lock()
	pgid := c.pgid
	c.mu.Unlock()
	if pgid <= 0 {
		return nil
	}
	err := syscall.Kill(-pgid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

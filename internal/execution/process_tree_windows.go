//go:build windows

package execution

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsProcessTreeControl struct {
	mu     sync.Mutex
	job    windows.Handle
	pid    uint32
	bound  bool
	closed bool
}

func prepareProcessTree(command *exec.Cmd) (processTreeControl, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	// CREATE_SUSPENDED 保证 Bind 能在目标执行任何用户代码、派生任何子进程前
	// 原子建立 Job 边界；绑定完成后只恢复该进程的初始线程。
	command.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED
	return &windowsProcessTreeControl{job: job}, nil
}

func (c *windowsProcessTreeControl) Bind(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return errors.New("process is unavailable")
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(c.job, process); err != nil {
		return err
	}
	thread, err := suspendedProcessThread(uint32(command.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(thread)
	if _, err := windows.ResumeThread(thread); err != nil {
		return err
	}
	c.mu.Lock()
	c.pid = uint32(command.Process.Pid)
	c.bound = true
	c.mu.Unlock()
	return nil
}

func suspendedProcessThread(pid uint32) (windows.Handle, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return 0, err
	}
	for {
		if entry.OwnerProcessID == pid {
			return windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			return 0, err
		}
	}
}

func (c *windowsProcessTreeControl) Interrupt() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.bound {
		return nil
	}
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, c.pid)
}

func (c *windowsProcessTreeControl) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.bound {
		return nil
	}
	return windows.TerminateJobObject(c.job, 1)
}

func (c *windowsProcessTreeControl) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if !c.bound {
		return windows.CloseHandle(c.job)
	}
	terminateErr := windows.TerminateJobObject(c.job, 1)
	_, waitErr := windows.WaitForSingleObject(c.job, windows.INFINITE)
	closeErr := windows.CloseHandle(c.job)
	return errors.Join(terminateErr, waitErr, closeErr)
}

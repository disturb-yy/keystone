package localstate

import (
	"errors"
	"fmt"
	"os"
)

// ErrAlreadyLocked 表示数据根已被另一个进程持有。
var ErrAlreadyLocked = errors.New("local state is already locked")

// InstanceLock 在整个实例生命周期内保持锁文件打开。
type InstanceLock struct {
	file *os.File
}

// Acquire 获取数据根的非阻塞独占锁。调用者应先调用 Paths.Initialize。
func Acquire(p Paths) (*InstanceLock, error) {
	file, err := os.OpenFile(p.LockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open local state lock: %w", err)
	}
	if err := lockFile(file); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("acquire local state lock: %w; close lock file: %v", err, closeErr)
		}
		return nil, fmt.Errorf("acquire local state lock: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		unlockErr := unlockFile(file)
		closeErr := file.Close()
		return nil, errors.Join(
			fmt.Errorf("set local state lock mode: %w", err),
			wrapLockCleanupError("unlock local state lock after mode failure", unlockErr),
			wrapLockCleanupError("close local state lock after mode failure", closeErr),
		)
	}
	return &InstanceLock{file: file}, nil
}

// Release 释放独占锁并关闭锁文件。
func (l *InstanceLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil || closeErr != nil {
		return errors.Join(
			wrapLockCleanupError("release local state lock", unlockErr),
			wrapLockCleanupError("close local state lock", closeErr),
		)
	}
	return nil
}

func wrapLockCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

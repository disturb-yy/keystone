package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
)

func defaultCommandRunner(ctx context.Context, executable string, args ...string) (DaemonProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := exec.Command(executable, args...)
	command.Stdout = io.Discard
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &commandProcess{command: command}, nil
}

type commandProcess struct {
	command *exec.Cmd
}

func (p *commandProcess) Wait() error {
	return p.command.Wait()
}

func discoverDaemonExecutable(deps Dependencies) (string, error) {
	if strings.TrimSpace(deps.DaemonExecutablePath) != "" {
		return deps.DaemonExecutablePath, nil
	}
	if executable, err := deps.CurrentExecutable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), "keystone-daemon")
		if resolved, err := deps.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	if resolved, err := deps.LookPath("keystone-daemon"); err == nil {
		return resolved, nil
	}
	return "", newCLIError(ErrorDaemonExecutable, "无法发现同目录或 PATH 中的 keystone-daemon", nil)
}

func (c *cli) start(ctx context.Context, out io.Writer) error {
	metadata, err := c.ensureDaemon(ctx)
	if err != nil {
		return err
	}
	status, err := c.verifyReady(ctx, metadata)
	if err != nil {
		return err
	}
	return writeJSONOutput(out, status)
}

func (c *cli) ensureDaemon(ctx context.Context) (localstate.Metadata, error) {
	metadata, initialErr := readRuntimeMetadata(c.paths)
	if initialErr == nil {
		if _, err := c.verifyReady(ctx, metadata); err == nil {
			return metadata, nil
		} else {
			initialErr = err
		}
	}
	executable, err := discoverDaemonExecutable(c.deps)
	if err != nil {
		return localstate.Metadata{}, err
	}
	process, err := c.deps.CommandRunner(ctx, executable, "--data-dir", c.paths.Root)
	if err != nil {
		return localstate.Metadata{}, newCLIError(ErrorDaemonStartFailed, "启动 keystone-daemon 子进程失败", err)
	}
	if process == nil {
		return localstate.Metadata{}, newCLIError(ErrorDaemonStartFailed, "启动 keystone-daemon 未返回进程句柄", nil)
	}
	readyContext, cancelReady := context.WithTimeout(ctx, c.deps.StartTimeout)
	defer cancelReady()
	if _, err := c.waitUntilReady(readyContext, process, initialErr); err != nil {
		return localstate.Metadata{}, err
	}
	metadata, err = readRuntimeMetadata(c.paths)
	if err != nil {
		return localstate.Metadata{}, err
	}
	if _, err := c.verifyReady(ctx, metadata); err != nil {
		return localstate.Metadata{}, err
	}
	return metadata, nil
}

func (c *cli) waitUntilReady(ctx context.Context, process DaemonProcess, initialErr error) (controlplane.DaemonStatusResponse, error) {
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- process.Wait()
	}()
	lastErr := initialErr
	startedAt := c.deps.Clock()
	ticker := time.NewTicker(c.deps.PollInterval)
	defer ticker.Stop()
	for {
		if status, err := c.tryReady(ctx); err == nil {
			return status, nil
		} else {
			lastErr = err
		}
		select {
		case err := <-waitResult:
			return controlplane.DaemonStatusResponse{}, processReadinessError(err, lastErr)
		case <-ctx.Done():
			return controlplane.DaemonStatusResponse{}, readinessTimeoutError(ctx, startedAt, c.deps.Clock(), lastErr)
		case <-ticker.C:
		}
	}
}

func (c *cli) tryReady(ctx context.Context) (controlplane.DaemonStatusResponse, error) {
	metadata, err := readRuntimeMetadata(c.paths)
	if err != nil {
		return controlplane.DaemonStatusResponse{}, err
	}
	return c.verifyReady(ctx, metadata)
}

func processReadinessError(processErr, lastErr error) error {
	if processErr == nil {
		return newCLIError(ErrorDaemonStartFailed, readinessFailureMessage(lastErr, "keystone-daemon 子进程在 ready 前退出"), nil)
	}
	return newCLIError(ErrorDaemonStartFailed, readinessFailureMessage(lastErr, "keystone-daemon 子进程在 ready 前失败"), processErr)
}

func readinessFailureMessage(lastErr error, fallback string) string {
	if lastErr == nil {
		return fallback
	}
	return fmt.Sprintf("%s；最近一次 readiness 检查：%v", fallback, lastErr)
}

func readinessTimeoutError(ctx context.Context, startedAt, now time.Time, lastErr error) error {
	duration := now.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}
	message := fmt.Sprintf("Daemon 在 %s 内未进入 ready", duration)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && lastErr != nil {
		message += fmt.Sprintf("；最近一次 readiness 检查：%v", lastErr)
	}
	return newCLIError(ErrorDaemonStartTimeout, message, ctx.Err())
}

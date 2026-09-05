package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
)

const (
	defaultHTTPTimeout  = 2 * time.Second
	defaultStartTimeout = 15 * time.Second
	defaultPollInterval = 50 * time.Millisecond
)

// HTTPDoer 是 CLI HTTP client 的最小依赖边界。
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// DaemonProcess 是已启动的 keystone-daemon 进程句柄。
type DaemonProcess interface {
	Wait() error
}

// CommandRunner 启动一个独立的 keystone-daemon，不通过 shell 解释参数。
type CommandRunner func(context.Context, string, ...string) (DaemonProcess, error)

// Dependencies 是 CLI 的可测试外部依赖。
type Dependencies struct {
	ResolveDataDir       func(string) (localstate.Paths, error)
	HTTPClient           HTTPDoer
	CommandRunner        CommandRunner
	DaemonExecutablePath string
	CurrentExecutable    func() (string, error)
	LookPath             func(string) (string, error)
	Clock                func() time.Time
	HTTPTimeout          time.Duration
	StartTimeout         time.Duration
	PollInterval         time.Duration
}

// DefaultDependencies 返回生产 CLI 使用的标准库实现。
func DefaultDependencies() Dependencies {
	return Dependencies{
		ResolveDataDir:    localstate.Resolve,
		HTTPClient:        &http.Client{Timeout: defaultHTTPTimeout},
		CommandRunner:     defaultCommandRunner,
		CurrentExecutable: os.Executable,
		LookPath:          exec.LookPath,
		Clock:             time.Now,
		HTTPTimeout:       defaultHTTPTimeout,
		StartTimeout:      defaultStartTimeout,
		PollInterval:      defaultPollInterval,
	}
}

func normalizeDependencies(deps Dependencies) Dependencies {
	defaults := DefaultDependencies()
	if deps.ResolveDataDir == nil {
		deps.ResolveDataDir = defaults.ResolveDataDir
	}
	if deps.HTTPClient == nil {
		deps.HTTPClient = defaults.HTTPClient
	}
	if deps.CommandRunner == nil {
		deps.CommandRunner = defaults.CommandRunner
	}
	if deps.CurrentExecutable == nil {
		deps.CurrentExecutable = defaults.CurrentExecutable
	}
	if deps.LookPath == nil {
		deps.LookPath = defaults.LookPath
	}
	if deps.Clock == nil {
		deps.Clock = defaults.Clock
	}
	if deps.HTTPTimeout <= 0 {
		deps.HTTPTimeout = defaults.HTTPTimeout
	}
	if deps.StartTimeout <= 0 {
		deps.StartTimeout = defaults.StartTimeout
	}
	if deps.PollInterval <= 0 {
		deps.PollInterval = defaults.PollInterval
	}
	return deps
}

type cli struct {
	deps Dependencies

	dataDir  string
	paths    localstate.Paths
	resolved bool
}

// NewRootCommand 创建完整的 keystone CLI 命令树。
func NewRootCommand(deps Dependencies) *cobra.Command {
	app := &cli{deps: normalizeDependencies(deps)}
	root := &cobra.Command{
		Use:           "keystone",
		Short:         "Keystone local control-plane CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	daemon := app.newDaemonCommand()
	root.AddCommand(daemon)
	return root
}

func (c *cli) newDaemonCommand() *cobra.Command {
	daemon := &cobra.Command{
		Use:   "daemon",
		Short: "管理本机 Keystone Daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	daemon.PersistentFlags().StringVar(&c.dataDir, "data-dir", "", "Keystone local state root")
	daemon.PersistentPreRunE = c.prepare
	daemon.AddCommand(
		&cobra.Command{
			Use:   "start",
			Short: "启动或复用已就绪的 Daemon",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return c.start(cmd.Context(), cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "查询既有 Daemon 状态",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return c.status(cmd.Context(), cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "stop",
			Short: "请求既有 Daemon 优雅停止",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return c.stop(cmd.Context(), cmd.OutOrStdout())
			},
		},
	)
	return daemon
}

func (c *cli) prepare(_ *cobra.Command, _ []string) error {
	if c.resolved {
		return nil
	}
	paths, err := c.deps.ResolveDataDir(c.dataDir)
	if err != nil {
		return newCLIError(ErrorMetadataInvalid, "解析 LocalStateRoot 失败", err)
	}
	if strings.TrimSpace(paths.Root) == "" || !filepath.IsAbs(paths.Root) {
		return newCLIError(ErrorMetadataInvalid, "LocalStateRoot 必须是归一化绝对路径", nil)
	}
	c.paths = paths
	c.resolved = true
	return nil
}

func (c *cli) client() *daemonHTTPClient {
	return &daemonHTTPClient{
		httpClient: c.deps.HTTPClient,
		timeout:    c.deps.HTTPTimeout,
	}
}

func writeJSONOutput(out io.Writer, payload any) error {
	if out == nil {
		return errors.New("CLI output writer is nil")
	}
	if err := jsonEncode(out, payload); err != nil {
		return newCLIError(ErrorInvalidResponse, "写入 CLI JSON 输出失败", err)
	}
	return nil
}

func jsonEncode(out io.Writer, payload any) error {
	return json.NewEncoder(out).Encode(payload)
}

func (c *cli) status(ctx context.Context, out io.Writer) error {
	metadata, err := readRuntimeMetadata(c.paths)
	if err != nil {
		return err
	}
	status, err := c.verifyReady(ctx, metadata)
	if err != nil {
		return err
	}
	return writeJSONOutput(out, status)
}

func (c *cli) stop(ctx context.Context, out io.Writer) error {
	metadata, err := readRuntimeMetadata(c.paths)
	if err != nil {
		return err
	}
	response, err := c.client().stop(ctx, metadata.Endpoint, controlplane.DaemonStopRequest{
		DaemonInstanceID: metadata.InstanceID,
	})
	if err != nil {
		return err
	}
	if !response.Accepted {
		return newCLIError(ErrorStopRejected, "Daemon 未接受停止请求", nil)
	}
	if response.DaemonInstanceID != metadata.InstanceID {
		return newCLIError(ErrorInstanceMismatch, "停止响应的 DaemonInstanceID 与 metadata 不一致", nil)
	}
	return writeJSONOutput(out, response)
}

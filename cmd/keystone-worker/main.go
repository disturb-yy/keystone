// Package main 是 Keystone 独立本机 Worker 入口。
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/execution"
	"github.com/disturb-yy/keystone/internal/execution/adapters/codex"
	worker "github.com/disturb-yy/keystone/internal/worker"
)

func main() {
	endpoint := flag.String("daemon-endpoint", "", "Daemon loopback endpoint")
	workerID := flag.String("worker-id", "", "WorkerInstance identity")
	codexBinary := flag.String("codex-binary", "codex", "Codex executable discovered by PATH")
	flag.Parse()
	secret, err := readStartupSecret(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read worker startup secret:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*endpoint) == "" || strings.TrimSpace(*workerID) == "" {
		fmt.Fprintln(os.Stderr, "worker endpoint and worker id are required")
		os.Exit(2)
	}
	client := worker.NewClient(*endpoint, secret, nil)
	runner, err := worker.NewRunner(worker.RunnerConfig{
		Client:       client,
		WorkerID:     *workerID,
		Capabilities: []string{"runtime:" + execution.RuntimeCodex, workercontract.VerificationCapability},
		Runtimes: map[string]execution.RuntimeAdapter{
			execution.RuntimeCodex: codex.New(*codexBinary),
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "create worker:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), workerShutdownSignals()...)
	defer stop()
	if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "worker stopped:", err)
		os.Exit(1)
	}
}

func readStartupSecret(input io.Reader) (string, error) {
	if input == nil {
		return "", errors.New("startup pipe is unavailable")
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("startup pipe read failed")
	}
	secret := strings.TrimSpace(line)
	if len(secret) < 32 {
		return "", errors.New("startup secret is invalid")
	}
	return secret, nil
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/disturb-yy/keystone/internal/daemon"
)

func main() {
	dataDir := flag.String("data-dir", "", "Keystone local state root")
	dashboardDir := flag.String("dashboard-dir", "", "Dashboard production asset directory")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := daemon.Run(ctx, *dataDir, daemon.Options{DashboardDir: *dashboardDir}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

//go:build !windows

package main

import (
	"os"
	"syscall"
)

func workerShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

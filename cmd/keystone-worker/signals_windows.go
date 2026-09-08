//go:build windows

package main

import (
	"os"
)

func workerShutdownSignals() []os.Signal {
	// Windows 将 Ctrl+C 与 Ctrl+Break 都映射为 os.Interrupt；这是该平台
	// 唯一可供 signal.Notify 使用的控制台信号。
	return []os.Signal{os.Interrupt}
}

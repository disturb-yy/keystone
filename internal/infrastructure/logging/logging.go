// Package logging 为 Keystone 提供显式的结构化日志能力。
package logging

import (
	"io"
	"log/slog"
)

// New 返回写入 writer 的 JSON logger，并过滤低于 level 的日志。
func New(writer io.Writer, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

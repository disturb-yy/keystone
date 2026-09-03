// Package config 为 Keystone 基础设施提供精简的环境变量配置能力。
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
)

const (
	// LogLevelEnv 是用于选择日志等级的环境变量。
	LogLevelEnv = "KEYSTONE_LOG_LEVEL"
)

// ErrInvalidLogLevel 表示无法解析为 slog 日志等级的值。
var ErrInvalidLogLevel = errors.New("invalid log level")

// Config 保存本包提供的配置值。
type Config struct {
	// LogLevel 控制日志处理器接收的最低等级。
	LogLevel slog.Level
}

// Load 读取支持的环境变量，并在未设置时应用默认值。
func Load() (Config, error) {
	value, ok := os.LookupEnv(LogLevelEnv)
	if !ok {
		return Config{LogLevel: slog.LevelInfo}, nil
	}

	level, err := ParseLevel(value)
	if err != nil {
		return Config{}, fmt.Errorf("load %s: %w", LogLevelEnv, err)
	}

	return Config{LogLevel: level}, nil
}

// ParseLevel 使用标准 slog 文本表示解析日志等级。
func ParseLevel(value string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return slog.LevelInfo, fmt.Errorf("%w %q: %v", ErrInvalidLogLevel, value, err)
	}

	return level, nil
}

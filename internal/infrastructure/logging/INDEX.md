# Logging 项目索引

## 当前职责

`internal/infrastructure/logging` 当前提供一个显式配置的 JSON logger factory：

| 文件 | 内容 |
| --- | --- |
| `logging.go` | `New` 函数 |
| `logging_test.go` | JSON 字段、等级过滤和全局 logger 稳定性测试 |

## 入口与行为

- `New(writer, level)` 使用 `slog.NewJSONHandler` 创建 logger。
- `writer` 是调用方提供的输出目标，`level` 是 handler 的最低等级。
- 创建 logger 不修改 `slog.Default()`。

## 验证

```text
go test ./internal/infrastructure/logging
```

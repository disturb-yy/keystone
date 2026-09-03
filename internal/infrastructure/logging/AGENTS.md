# Logging 局部规约

## 职责

`logging` 只创建标准库 `log/slog` 的 JSON handler 和 logger。调用方必须显式传入 `io.Writer` 与最低 `slog.Level`。

## 边界

- `New` 返回使用调用方输出目标的 `*slog.Logger`。
- 不调用 `slog.SetDefault`，不替换进程全局默认 logger。
- 不负责日志文件生命周期、轮转、配置读取或外部日志服务。
- 只使用 Go 标准库，不依赖其他 Keystone package。

## 修改与验证

保持结构化字段、JSON 输出和等级过滤由标准库 handler 提供。测试应验证可解析 JSON、调用方字段、等级过滤和全局 logger 未变化，并运行 `go test ./internal/infrastructure/logging`。

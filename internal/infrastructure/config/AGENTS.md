# Config 局部规约

## 职责

`config` 只读取 `KEYSTONE_LOG_LEVEL` 环境变量，并将其解析为标准库 `slog.Level`。环境变量未设置时，`Load` 返回 `slog.LevelInfo`；设置了非法值时返回可通过 `errors.Is` 判断的 `ErrInvalidLogLevel`。

## 边界

- 不读取配置文件。
- 不处理 `--data-dir`、用户目录、运行时重载或持久化。
- 只使用 Go 标准库，不依赖其他 Keystone package。

## 修改与验证

保持 `Load` 和 `ParseLevel` 的错误显式返回。修改日志等级解析规则时，使用表驱动测试覆盖默认、合法和非法输入，并运行 `go test ./internal/infrastructure/config`。

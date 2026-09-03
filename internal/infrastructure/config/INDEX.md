# Config 项目索引

## 当前职责

`internal/infrastructure/config` 当前只提供日志等级环境变量解析：

| 文件 | 内容 |
| --- | --- |
| `config.go` | `KEYSTONE_LOG_LEVEL` 常量、`Config`、`Load`、`ParseLevel` 和错误分类 |
| `config_test.go` | 默认值、标准等级和非法值的行为测试 |

## 入口与行为

- `Load()` 使用 `KEYSTONE_LOG_LEVEL`；未设置返回 `slog.LevelInfo`。
- `ParseLevel(value)` 使用 `slog.Level.UnmarshalText` 的标准文本语义。
- 非法值返回 `ErrInvalidLogLevel` 分类错误。

## 验证

```text
go test ./internal/infrastructure/config
```

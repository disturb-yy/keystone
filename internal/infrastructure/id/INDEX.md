# ID 项目索引

## 当前职责

`internal/infrastructure/id` 当前只生成由 Go 1.27 标准库产生的时间有序 UUIDv7 字符串：

| 文件 | 内容 |
| --- | --- |
| `id.go` | `New` 函数，调用标准库 `uuid.NewV7().String()` |
| `id_test.go` | 格式、version 7、variant、字典序和唯一性测试 |

## 入口与行为

- `New()` 调用 Go 1.27 标准库 `uuid.NewV7().String()`。
- UUID 的最高有效 48 位包含 Unix 毫秒时间戳；同一进程内连续生成序列具有标准库提供的时间有序语义，但系统时钟回拨时不保证字典序递增。跨进程、跨主机只提供基于时间戳的近似排序，不构成严格全局顺序或因果顺序。
- 业务时间和事件顺序必须使用 `created_at` 或显式序列表达，不能从 UUID 字典序推导。
- 第 7 个字节的高四位为 UUID version 7。
- 第 9 个字节符合 RFC 9562 variant 10。
- 返回值使用小写十六进制和标准 UUID 连字符布局。

## 验证

```text
go test ./internal/infrastructure/id
```

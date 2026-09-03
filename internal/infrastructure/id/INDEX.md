# ID 项目索引

## 当前职责

`internal/infrastructure/id` 当前只生成随机 UUIDv4 字符串：

| 文件 | 内容 |
| --- | --- |
| `id.go` | `New` 函数与 UUID 字节格式化 |
| `id_test.go` | 格式、version、variant 和唯一性测试 |

## 入口与行为

- `New()` 从 `crypto/rand.Reader` 读取 16 个字节。
- 第 7 个字节的高四位为 UUID version 4。
- 第 9 个字节符合 RFC 4122 variant 10。
- 返回值使用小写十六进制和标准 UUID 连字符布局。

## 验证

```text
go test ./internal/infrastructure/id
```

# ID 局部规约

## 职责

`id` 只使用 `crypto/rand` 生成标准小写 UUIDv4 字符串，并正确设置 UUID version 与 variant 位。

## 边界

- `New` 返回随机生成的字符串或明确的随机源错误。
- 不提供排序语义、持久化、解析、领域 ID 类型或数据库访问。
- 只使用 Go 标准库，不依赖其他 Keystone package。

## 修改与验证

保持输出为 `8-4-4-4-12` 的小写十六进制 UUID 格式。修改生成逻辑时，测试应验证格式、version、variant 和多次调用不重复，并运行 `go test ./internal/infrastructure/id`。

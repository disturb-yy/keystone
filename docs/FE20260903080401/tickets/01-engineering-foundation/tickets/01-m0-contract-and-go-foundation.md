# 01: M0 合约对齐与 Go 基础能力

**What to build:** 让 Keystone 首次拥有可测试、可编译的跨领域基础能力，同时让 M0 与 Ticket 02 的 Migration ownership 清晰分离。开发者可以读取日志等级配置、创建 JSON 结构化 logger 并生成随机 UUIDv4；后续 Ticket 可以在这些窄 package 上扩展，而不会获得数据目录、数据库或生命周期语义。

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] M0 Ticket 和里程碑计划不再承诺 Migration 调用、runner、版本表或占位目录；这些责任唯一归属 Ticket 02。
- [ ] 配置能力只处理 `KEYSTONE_LOG_LEVEL`：未设置时使用 `info`，合法等级被确定性解析，非法值返回错误。
- [ ] 日志能力以 Go 标准库的 JSON 结构化日志输出为调用者提供显式等级和输出目标，且不修改全局默认 logger。
- [ ] 标识符能力使用安全随机源生成小写 UUIDv4 字符串，不引入排序、持久化或领域 ID 语义。
- [ ] 配置、日志与 ID 都是职责明确的独立 Go package；父级基础设施边界及每个 Go package 均有局部规则和导航文档。
- [ ] Go 基础能力不引入第三方依赖，并以外部可观察行为覆盖默认值、非法配置、JSON 输出、日志等级与 UUIDv4 格式。
- [ ] `go test ./...` 可执行并通过，不再因仓库没有 Go package 而无法验证。

# 01: M0 合约对齐与 Go 基础能力

**What to build:** 让 Keystone 首次拥有可测试、可编译的跨领域基础能力，同时让 M0 与 Ticket 02 的 Migration ownership 清晰分离。开发者可以读取日志等级配置、创建 JSON 结构化 logger 并生成时间有序 UUIDv7；后续 Ticket 可以在这些窄 package 上扩展，而不会获得数据目录、数据库或生命周期语义。

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] M0 Ticket 和里程碑计划不再承诺 Migration 调用、runner、版本表或占位目录；这些责任唯一归属 Ticket 02。
- [ ] 配置能力只处理 `KEYSTONE_LOG_LEVEL`：未设置时使用 `info`，合法等级被确定性解析，非法值返回错误。
- [ ] 日志能力以 Go 标准库的 JSON 结构化日志输出为调用者提供显式等级和输出目标，且不修改全局默认 logger。
- [ ] 标识符能力直接使用 Go 1.27 标准库 `uuid.NewV7().String()` 生成小写 UUIDv7 字符串。同一进程内连续生成序列具有标准库提供的时间有序语义，但系统时钟回拨时不保证字典序递增；跨进程、跨主机只提供基于时间戳的近似排序，不构成严格全局顺序或因果顺序。业务时间和事件顺序必须使用 `created_at` 或显式序列表达，不引入业务排序、持久化或领域 ID 语义。
- [ ] 配置、日志与 ID 都是职责明确的独立 Go package；父级基础设施边界及每个 Go package 均有局部规则和导航文档。
- [ ] Go 基础能力不引入第三方依赖，并以外部可观察行为覆盖默认值、非法配置、JSON 输出、日志等级、UUIDv7 格式、同一进程内生成顺序与唯一性。
- [ ] `go test ./...` 可执行并通过，不再因仓库没有 Go package 而无法验证。

# 02: SQLite Migration 基线

**What to build:** 让持有本机状态锁的 Keystone 进程能够在新空数据库上建立可查询的 Migration 基线；重复运行保持不变，已应用 Migration 的内容漂移则明确失败，不会偷偷创建业务状态。

**Blocked by:** 01: 本地数据根与跨平台单实例状态。

**Status:** ready-for-agent

- [ ] SQLite 使用纯 Go driver，Windows、Linux 与 WSL 的构建不以 CGO 为前提，且依赖版本经实际 Go 1.27 验证后锁定。
- [ ] 首个递增 Migration 仅建立 `t_schema_migrations` 元数据，并记录版本、名称、SHA-256 校验和与应用时间；不创建 Project、Change、Ticket 或其他业务表。
- [ ] 新空数据库可在持锁前提下完成 Migration；重复运行不会重复记录或执行已应用版本。
- [ ] 每个 Migration 在事务中应用；失败不会留下半应用状态，已记录版本与嵌入资产的名称或校验和不一致时停止继续启动。
- [ ] 不提供 down migration、隐式修复或绕过单实例锁的数据库访问。

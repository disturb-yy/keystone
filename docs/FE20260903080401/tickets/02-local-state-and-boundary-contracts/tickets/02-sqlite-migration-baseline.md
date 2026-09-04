# 02: SQLite Migration 基线

**What to build:** 让持有本机状态锁的 Keystone 进程能够在新空数据库上建立可查询的 Migration 基线；重复运行保持不变，已应用 Migration 的内容漂移则明确失败，不会偷偷创建业务状态。

**Blocked by:** 01: 本地数据根与跨平台单实例状态。

**Status:** implemented

- [x] SQLite 使用纯 Go driver，Windows、Linux 与 WSL 的构建不以 CGO 为前提，且依赖版本经 Go 1.27 测试和 Linux/Windows 目标构建验证后锁定。
- [x] 首个递增 Migration 仅建立 `t_schema_migrations` 元数据，并记录版本、名称、SHA-256 校验和与应用时间；不创建 Project、Change、Ticket 或其他业务表。
- [x] 新空数据库可在调用方持有本机状态锁的前提下完成 Migration；重复运行不会重复记录或执行已应用版本。
- [x] 每个 Migration 在事务中应用；失败不会留下半应用状态，已记录版本的名称或校验和不一致时停止继续启动。
- [x] 不提供 down migration、隐式修复或绕过单实例锁的数据库访问；runner 明确要求调用方负责锁生命周期。

验证记录：真实 SQLite 测试覆盖空库、重复、增量、回滚、漂移、未知版本和非法输入；`go test ./...`、`go vet ./...`、Linux 与 Windows 无 CGO 目标构建已通过。`go mod tidy` 尚未在当前受限 file proxy 中完成，原因是缺少 `modernc.org/gc/v3` 的测试依赖缓存，并非代码测试失败。

# Work 拥有业务持久化，Daemon 负责组合

M3 的设计规定：`internal/work/domain` 保持纯领域不变量，`internal/work` 承载 Application 用例和 Port，`internal/work/sqlite` 拥有 Work 的 SQLite Repository 与业务 Migration，`internal/work/artifactstore` 与 `internal/work/git` 分别实现内容存储和只读 RepositorySnapshot。Daemon 只组合这些实现、管理数据库与 readiness，并将 HTTP DTO 转换为 Application 调用；CLI 只经 Control Plane Contract 访问 Daemon。

通用 `migration.DefaultMigrations()` 继续只提供版本 1 的迁移元数据；Daemon 在持锁启动中合并它与 `work/sqlite.Migrations()` 后交给同一 Runner。Ticket 04 的 Work Migration 先建立统一 `t_events` 账本，M3 在其后的下一个正整数版本中扩展该账本，并一次创建 `t_changes`、`t_artifacts`、`t_artifact_refs`、`t_agent_runs`、`t_decisions`、`t_change_command_receipts`、`t_event_artifact_refs` 与 `t_agent_run_artifact_refs` 及其索引；规划不预占尚未到来的数值版本。这样业务 SQL 不泄漏进通用 Migration 或 Daemon，且仍由现有 Daemon 生命周期统一应用。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

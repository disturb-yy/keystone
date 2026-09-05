# SQLite 执行 Work 的关键不变量

M3 的设计规定：Daemon 为每个 SQLite 连接启用 `PRAGMA foreign_keys=ON`，并让 Schema 约束而非单独依赖 Go 分支保护关键事实。Event、Decision、ArtifactRef、两类 Artifact 关联和成功 ChangeCommandReceipt 只能插入，不能更新或删除；Artifact 身份行不能更新，M3 不提供删除路径；AgentRun 只允许从 running 一次性变为带 outcome 与 completed_at 的 completed；Change 使用带旧 Version 的条件更新，成功时递增 Version。

这些约束与状态、Event、Decision、Receipt 的同一 transaction 共同构成权威边界，Repository 或并发 Handler 的回归不能静默改写历史。未来若需要孤儿回收或新的维护路径，必须通过显式 Migration 与独立规则引入，不能绕过现有触发器。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

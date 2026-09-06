# `workstore` 局部规约

该 package 是 Work 领域的 SQLite adapter 和业务 Migration owner。

- 只接收 Daemon 已打开的 `*sql.DB`，不拥有连接、锁或 HTTP 生命周期。
- Project、ProjectInitialized、Intent 和 Receipt 的 finalization 必须在一个事务中完成；Change、ArtifactRef、Change Event、HumanDecision、AgentRun 和 Change Receipt 的成功写入也必须在各自事务中原子完成。
- 确定性失败须在同一事务为原始和当前幂等 key 写入 failed receipt；可恢复失败保留 pending intent。Rebind 只能按事务内的预期旧 root 条件更新。
- 查询和写入使用参数化 SQL；完整性错误不得自动修复或删除既有事实。
- SQLite 连接必须启用 `foreign_keys`；Change 历史只能追加，AgentRun 只允许一次 `running` 到 `completed` 更新。
- WorkerInstance、Lease 和 Report receipt 继续使用本 Work DB；Lease secret 只保存 SHA-256，
  stdout/stderr/diff/changed_files 内容先通过 `WorkerArtifactStore` 原子落盘，再在同一事务中关联
  AgentRun、ArtifactRef 和统一 Event ledger。
- `AgentRunReportLate` 是统一 `t_project_events` 的追加事件；不得创建 Worker 私有事件账本，
  不得通过 Report 重放覆盖既有终态。

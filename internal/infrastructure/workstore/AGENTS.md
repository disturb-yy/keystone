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
- Schema v5 只做 additive Planning 扩展；Schema v6 只追加 Canonical Ticket Graph、Ticket、Dependency 和 Event 关联；旧 ArtifactRef、AgentRun、Lease、Event 和 LateReport 数据必须保持可读。
- 同一 Change 最多一个 running Planning AgentRun。Planning start 使用显式目标 stage、attempt、ChangeVersion 和 source revision 围栏，不提前改写 checkpoint。
- `planning_candidate` Lease 的首个有效 Report 只追加 candidate/raw-log 事实并消费 Lease；不完成 AgentRun、不追加 stage 事件、不推进 Change。
- Planning completion 在单一 SQLite 事务内完成 ArtifactRef/link、AgentRun、统一 Event 和条件 checkpoint 推进；非当前 attempt 只能记录 fenced 终态。
- 恢复查询包含 active 的 Intent/Understand/Design checkpoint，也包含任意 Change status 下尚未收敛的 running Planning run；后者允许 candidate 或待持久化失败在围栏下完成裁决，Plan checkpoint 不再启动 Planning。
- Planning Lease 过期本身不推断 Runtime 已停止；late Report、空闲 Heartbeat、Supervisor
  WorkerProcessLost 或 Daemon restart 才能形成 durable system failure candidate。Report 首次
 终态按 digest 幂等，冲突 Report 不新增 receipt。
- Assignment 与 completion 都在事务内重新检查 Change status/version；Pause、Cancel 或
  dispatch fence 不能创建新的 Lease 或推进 Change。调度前失败和 Artifact store 故障也必须
  形成可恢复的 durable candidate，而不是依赖进程内错误缓存。
- Graph、Ticket、Acceptance Criterion 和 Dependency 只允许在 Ticketize 专用事务中创建，
  创建后由数据库 trigger 拒绝 update/delete；Ticket/Dependency 通过复合外键保持同图，
  递归 CTE 拒绝依赖环，`TicketGraphCreated` 通过 Graph 复合外键关联且不形成循环外键。
- Ticketize completion 必须同时满足 active/Ticketize、当前 AgentRun、同一 source revision、
  成功 Plan output 和 Ticketize Draft output 来源约束；成功时在同一事务内写 Graph、事件、
  AgentRun 终态、StageAdvanced 和 Execute checkpoint。

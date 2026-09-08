# ADR-0057：执行 Schema 与原子迁移

## 状态

已接受。

## 决策

Ticket 09 在 `workstore.Migrations()` 中追加实际已注册链的下一条 Migration，不预占或硬编码某个版本号。该 Migration 建立执行专属事实表：

- `t_execution_sessions`
- `t_workspace_provisioning_intents`
- `t_workspaces`
- `t_execution_dispatch_epochs`
- `t_execution_authorizations`
- `t_ticket_execution_states`
- `t_workspace_snapshots`
- `t_ticket_execution_evidence`

既有 `t_agent_runs` 增加 CanonicalTicket/Authorization 关联；`t_execution_authorizations`、`t_workspace_snapshots` 与 `t_ticket_execution_evidence` 保存不可变 WorkspaceInputRevision，Ticket 09 中它等于 BaseRevision，后续 Ticket 10 只能通过已记录 KeystoneCommit 推进它；既有 `t_worker_leases` 增加不可变 Envelope 摘要、timeout、交付及 RuntimeClaim 状态，不创建平行 Assignment 表。所有关联使用 Project/Change/Graph 复合身份，防止跨 Change 绑定。

SQLite 必须以 CHECK、partial unique index 与受限 transition trigger 强制：每个 Change 最多一个 Workspace 与未终结 Session、每个 Ticket 最多一个活跃 Authorization、每个 Project 最多一个活跃写 Workspace Lease、每个 AgentRun/Lease 只有一个 Claim、每个 Ticket 只有一份成功证据。应用层负责先行校验；SQLite 是最终并发防线。

WorkspaceProvisioningIntent 在 Git 操作前单独持久化；Git 成功后 Workspace、ExecutionSession 与 Execute 回执在同一 transaction 收敛。Ticket 成功证据和关联状态继续在 Q27 定义的同一 transaction 写入。

## 理由与边界

执行协调既要保留 Git/SQLite 中断恢复，又要原子更新 Ticket、AgentRun、Lease 和 Evidence；把状态拆为无约束的 JSON 或平行 Assignment 表会破坏唯一性和审计关联。按实际 migration 链追加，避免 Ticket 08 或并行工作改变版本号时产生冲突。

实现必须以真实 SQLite 覆盖 predecessor 升级、重复 Execute、并发授权、重复 Claim、Lease 围栏、跨 Change 外键、rollback 及 trigger 拒绝。该 ADR 不证明任何表或迁移已创建。

# `workstore` 项目索引

## 当前状态

该 package 提供 Ticket 04/05/07 业务 SQLite Migration、Project/Change 状态、ArtifactRef、
AgentRun、Planning candidate/completion、HumanDecision、Event、WorkerInstance、Lease、Report receipt 和 Artifact authority
查询写入，供 Daemon/Application seam 使用；Worker 不直接连接 SQLite。

| 文件 | 职责 |
| --- | --- |
| `store.go` | Schema v2、intent/receipt 事务 finalization、确定性失败回执、条件 rebind 和状态查询 |
| `change_store.go` | Schema v3、统一 Event 账本迁移、Change 状态命令、Trace、Artifact 归属、AgentRun/Decision 和 Receipt |
| `worker_schema.go` | Schema v4、LateReport 事件扩展、WorkerInstance/Lease/Report 表与追加约束 |
| `planning_schema.go` | Schema v5、Planning metadata/link、candidate、completion、result mode 及串行/追加约束 |
| `worker_store.go` | Worker 注册鉴权、可用 Worker 查询、Heartbeat/Pull、普通/Planning Assignment、Report classification、Artifact transaction、liveness watchdog 和重启/丢失收敛 |
| `planning_store.go` | Planning run 启动围栏、candidate/系统失败 candidate 耐久化、late/Artifact failure、幂等原子 completion 和恢复查询 |
| `store_test.go` | 真实 SQLite 的幂等、失败回放、并发 rebind、事件唯一性和 rollback 测试 |
| `change_store_test.go` | Change 创建、生命周期、Retry、Cancel、迁移兼容、并发版本和 SQLite 归属/追加约束测试 |
| `worker_store_test.go` | Assignment/Lease、首次 Report、duplicate/conflict、late trace、liveness 和 daemon restart 收敛测试 |
| `planning_store_test.go` | Planning 串行/围栏/幂等、Pause/Cancel fence、Artifact metadata/link、candidate/result mode、失败恢复、原子回滚与 v5 兼容测试 |

# `workstore` 项目索引

## 当前状态

该 package 提供 Ticket 04/05 业务 SQLite Migration、Project/Change 状态、ArtifactRef、
AgentRun、HumanDecision、Event 和 Receipt 查询写入，供 `internal/work` Application 使用。

| 文件 | 职责 |
| --- | --- |
| `store.go` | Schema v2、intent/receipt 事务 finalization、确定性失败回执、条件 rebind 和状态查询 |
| `change_store.go` | Schema v3、统一 Event 账本迁移、Change 状态命令、Trace、Artifact 归属、AgentRun/Decision 和 Receipt |
| `store_test.go` | 真实 SQLite 的幂等、失败回放、并发 rebind、事件唯一性和 rollback 测试 |
| `change_store_test.go` | Change 创建、生命周期、Retry、Cancel、迁移兼容、并发版本和 SQLite 归属/追加约束测试 |

# `workstore` 项目索引

## 当前状态

该 package 提供 Ticket 04 业务 SQLite Migration、intent/receipt 持久化以及
Project/Event 查询，供 `internal/work` Application 使用。

| 文件 | 职责 |
| --- | --- |
| `store.go` | Schema v2、intent/receipt 事务 finalization、确定性失败回执、条件 rebind 和状态查询 |
| `store_test.go` | 真实 SQLite 的幂等、失败回放、并发 rebind、事件唯一性和 rollback 测试 |

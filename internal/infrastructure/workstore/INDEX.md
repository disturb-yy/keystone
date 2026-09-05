# `workstore` 项目索引

## 当前状态

该 package 提供 Ticket 04 业务 SQLite Migration、intent/receipt 持久化以及
Project/Event 查询，供 `internal/work` Application 使用。

| 文件 | 职责 |
| --- | --- |
| `store.go` | Schema v2、事务 finalization 和状态查询 |
| `store_test.go` | 真实 SQLite 的幂等、事件唯一性和 rollback 测试 |

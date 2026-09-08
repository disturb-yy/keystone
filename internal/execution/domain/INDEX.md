# `execution/domain` 项目索引

| 文件 | 职责 |
| --- | --- |
| `model.go` | Workspace、ProvisioningIntent、ExecutionSession、DispatchEpoch、Authorization、Snapshot、TicketDelta 和 Evidence 值对象 |

该 package 不访问 Git、SQLite、HTTP 或 Worker；持久化映射由 application/infrastructure 负责。

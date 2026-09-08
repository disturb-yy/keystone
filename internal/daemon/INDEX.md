# Daemon 包索引

| 文件 | 职责 |
| --- | --- |
| `server.go` | Daemon 生命周期、Planning composition、HTTP Server、SQLite 与单实例资源的公开 API |
| `planning.go` | Planning 启动恢复、周期/事件唤醒、固定 Runtime 映射、Repository Snapshot 与 Workstore Assignment 适配 |
| `project_http.go` | Project Init、Project Query 和 Project Event Query HTTP Handler |
| `change_http.go` | Change 创建、查询、生命周期命令、HumanDecision、Trace 和 Artifact 内容 Handler；创建、Resume、retry 后唤醒 Planning |
| `worker_http.go` | loopback Worker Protocol 的 Register、Heartbeat、Pull、Report Handler；耐久 Worker/候选事实成功后唤醒 Planning |
| `supervisor.go` | sibling/PATH Worker 发现、stdin secret、进程树 watchdog、崩溃退避和有界关闭 |
| `supervisor_test.go`、`supervisor_unix_test.go` | Worker 有界关闭、终止失败、Snapshot 清理延后、watchdog 和子树清理测试 |
| `server_test.go` | Daemon 启动、Migration readiness 与 HTTP 边界测试 |
| `planning_test.go` | Planning Manager 恢复生命周期、Dispatcher 映射与无公开启动路由测试 |
| `change_http_test.go` | Change HTTP 创建、幂等重放、控制命令、Trace 和 Artifact 内容验收 |
| `worker_http_test.go` | Worker loopback/auth/strict JSON/空 Pull Handler 验收 |

Worker 的 Lease、Report transaction 和 Artifact authority 位于
`internal/infrastructure/workstore`；Daemon 只组合该 adapter，不创建 Worker 私有数据库。

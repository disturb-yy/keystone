# Daemon 包索引

| 文件 | 职责 |
| --- | --- |
| `server.go` | Daemon 生命周期、HTTP Server、SQLite 与单实例资源的公开 API |
| `project_http.go` | Project Init、Project Query 和 Project Event Query HTTP Handler |
| `change_http.go` | Change 创建、查询、生命周期命令、HumanDecision、Trace 和 Artifact 内容 HTTP Handler |
| `worker_http.go` | loopback Worker Protocol 的 Register、Heartbeat、Pull、Report Handler |
| `supervisor.go` | sibling/PATH Worker 发现、stdin secret、崩溃退避和有界关闭 |
| `server_test.go` | Daemon 启动、Migration readiness 与 HTTP 边界测试 |
| `change_http_test.go` | Change HTTP 创建、幂等重放、控制命令、Trace 和 Artifact 内容验收 |
| `worker_http_test.go` | Worker loopback/auth/strict JSON/空 Pull Handler 验收 |

Worker 的 Lease、Report transaction 和 Artifact authority 位于
`internal/infrastructure/workstore`；Daemon 只组合该 adapter，不创建 Worker 私有数据库。

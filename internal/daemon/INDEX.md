# Daemon 包索引

| 文件 | 职责 |
| --- | --- |
| `server.go` | Daemon 生命周期、HTTP Server、SQLite 与单实例资源的公开 API |
| `project_http.go` | Project Init、Project Query 和 Project Event Query HTTP Handler |
| `change_http.go` | Change 创建、查询、生命周期命令、HumanDecision、Trace 和 Artifact 内容 HTTP Handler |
| `server_test.go` | Daemon 启动与 HTTP 边界测试 |
| `change_http_test.go` | Change HTTP 创建、幂等重放、控制命令、Trace 和 Artifact 内容验收 |

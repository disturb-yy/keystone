# Worker 包索引

| 文件 | 职责 |
| --- | --- |
| `client.go` | loopback Worker Protocol HTTP Client、稳定错误分类和请求 deadline |
| `runner.go` | Register、Heartbeat、Pull、edit Assignment Claim、verify 路由、Runtime 取消等待、candidate/执行证据 Report 顺序、Planning Snapshot 修改/路径泄漏防线及同一 Report 重试 |
| `verification.go` | 固定 argv/cwd、环境白名单、timeout/fail-fast/not_run 和 typed output evidence |
| `runner_test.go` | Worker loop、固定 Report、Planning 只读边界、deadline 和 unavailable 重试测试 |
| `verification_test.go` | VerificationExecutor fail-fast、not_run 和环境白名单测试 |

Worker 不持有 SQLite 连接，也不推进 Change 或 Event。

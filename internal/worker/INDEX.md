# Worker 包索引

| 文件 | 职责 |
| --- | --- |
| `client.go` | loopback Worker Protocol HTTP Client 和稳定错误分类 |
| `runner.go` | Register、Heartbeat、Pull、Runtime、Report 顺序及同一 Report 重试 |
| `runner_test.go` | Worker loop、固定 Report 和 unavailable 重试测试 |

Worker 不持有 SQLite 连接，也不推进 Change 或 Event。

# `cmd/keystone-worker` 项目索引

| 文件 | 职责 |
| --- | --- |
| `main.go` | 读取 endpoint、Worker ID 和 stdin secret，启动独立 Worker loop |
| `signals_unix.go`、`signals_windows.go` | 平台原生 Worker shutdown signal 列表 |

本入口不拥有 SQLite、Daemon 生命周期或 Worker authority。

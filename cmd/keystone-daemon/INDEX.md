# `cmd/keystone-daemon` 项目索引

| 文件 | 职责 |
| --- | --- |
| `main.go` | 解析 `--data-dir`、绑定信号上下文并调用 Daemon 生命周期 |

运行实现位于 `internal/daemon`；本入口不直接访问数据库或 HTTP。

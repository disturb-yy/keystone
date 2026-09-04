# `cmd/keystone-daemon` 工程规约

该入口只负责解析 `--data-dir`、接收进程信号并调用 `internal/daemon`。

- 不写 SQL。
- 不实现 HTTP Handler 或路由。
- 不拥有 LocalState、SQLite 或锁资源。
- 启动与关闭错误输出到标准错误并使用非零退出码报告。

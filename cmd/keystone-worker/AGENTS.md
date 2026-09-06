# `cmd/keystone-worker` 工程规约

该入口只负责读取非敏感启动参数、从 stdin 启动管道读取一次性 Worker secret，并调用
`internal/worker`。secret 不得出现在 argv、环境变量、日志或错误输出中。

- 不连接 Keystone SQLite。
- 不实现 Change、Ticket、Lease、Event 或 HTTP Handler。
- Runtime binary 只通过 `internal/execution/adapters/codex` 进入 Worker loop。
- 启动/关闭错误输出到标准错误，但必须使用稳定摘要，不能输出请求 body、secret 或绝对 Workspace 路径。

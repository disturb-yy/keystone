# Worker 包规约

`internal/worker` 只实现独立 `keystone-worker` 的主动协议 Client、运行循环和 Report 转换。

- Worker 不连接 Keystone SQLite，不拥有 Change、Ticket、Lease 或 Event 权威状态。
- secret 只从调用方提供的内存值进入 Authorization header，不写日志、命令行或 Runtime 环境。
- Register 成功后才能 Pull；一次只执行一个 Assignment；Report `unavailable` 只能重发同一 Report，不能重跑 Runtime。
- HTTP、RuntimeAdapter 和时钟通过字段或接口注入，测试不得依赖真实 Codex 认证。
- 注释使用中文；新增行为必须覆盖协议顺序、重试和 secret 不泄漏边界。

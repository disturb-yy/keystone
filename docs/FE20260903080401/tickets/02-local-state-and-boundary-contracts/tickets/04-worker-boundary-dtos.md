# 04: Worker 基础边界 DTO

**What to build:** 让独立 Worker 能以最小传输模型注册、心跳、接收 Assignment 并提交 Report，使 Daemon 未来能够接收执行事实，而不会把 Worker 变成 Lifecycle、Gate 或数据库事实的拥有者。

**Blocked by:** Ticket 01 Engineering Foundation 验收。

**Status:** implemented

- [x] Worker Contract 以独立 package 编译，并定义 Register、Heartbeat、Assignment 与 Report 的强类型传输模型。
- [x] Register 表达 worker 标识、协议版本与能力；Heartbeat 表达 worker 标识；Assignment 表达 AgentRun 标识、不透明 lease token、workspace 路径与 runtime；Report 表达 AgentRun 标识、lease token 与 outcome。
- [x] DTO 标识均是传输标识，不复用或导出 Domain Entity，也不包含 Project、Change、Ticket、Gate、Decision、SQLite 或 Lifecycle 字段。
- [x] JSON 编解码与独立编译测试证明 Worker Contract 不需要 HTTP Handler、Daemon 进程或真实 Runtime 即可验证。
- [x] Pull、Execute、鉴权、租约校验、Report 幂等、Worker 监管、Artifact 采集与状态推进均不在本 Ticket 实现。

验证记录：`go test ./contracts/worker` 与根级 `go test ./...` 已通过；package 只依赖标准库。

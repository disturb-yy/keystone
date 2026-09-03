# 06 — Local Worker and Codex Runtime

- 里程碑：M4
- `BLOCKED_BY`：05
- 交付类型：Worker 与 Runtime 纵切

## 目标

实现 Daemon 监管的独立本机 Worker 和首个真实 Codex Runtime Adapter，使一次受授权 Assignment 可完成可复盘 smoke run。

## 范围

- Daemon 启动、监管和重启一个本机 Worker；Worker 通过受鉴权的 loopback HTTP/JSON 执行 Register、Heartbeat、Pull、Execute、Report。
- Assignment 绑定不可复用的 `AgentRun` 与 lease token；Daemon 只接受有效租约的首次终态 Report，重复 Report 幂等，晚到 Report 仅进入 Trace。
- 定义 Runtime Adapter；实现 Codex Adapter 并仅为 OpenCode 保留接口位置。
- Worker 固定 Runtime cwd 为 Assigned Workspace，独立采集 stdout/stderr、exit code、diff 和 changed files 为 Artifact。
- Execution Guard 强制协议与 Worker 包装层限制：禁止 Runtime commit、push、merge、推进 Lifecycle、写 Keystone DB 或批准 Verify。

## 不包含

- Remote Worker、Worker Pool、消息队列或多 Runtime 智能路由。
- Ticket Worktree 调度和业务代码修改的 Golden Path。
- 对任意本机 shell 行为作不可兑现的完全沙箱承诺。

## 验收条件

- Daemon、独立 Worker、Codex CLI、Worker Report 的 smoke run 可在本机形成完整 Trace。
- 未持有有效 lease 的 Report 不能改变权威状态；相同终态 Report 可安全重试。
- Codex 运行日志和退出状态由 Worker 采集，而不是只信任 Runtime 自报。
- Worker、Runtime 均不能直接写 SQLite 或调用 Lifecycle 变更接口。

## 验证

```bash
go test ./...
go vet ./...
make build
```

另执行一次显式的本机 Codex smoke run，保存 AgentRun、日志 Artifact 和 Report 证据。

## 实现边界

此 Ticket 建立副作用执行通道，不授予 Worker 任何 Change、Ticket、Gate 或 Recovery Decision 的权威性。

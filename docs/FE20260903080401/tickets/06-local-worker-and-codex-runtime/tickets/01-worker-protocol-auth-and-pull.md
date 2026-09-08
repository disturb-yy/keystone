# 06-01：Worker Protocol、鉴权与 Pull

**What to build：** 在既有 `contracts/worker` DTO 和 Daemon loopback listener 之上建立窄的 Worker Protocol，使一个独立本机 Worker 能安全 Register、Heartbeat、Pull 和提交后续 Report，而不把生命周期或持久化职责放进 Contract。

**Blocked by：** 顶层 Ticket 05：Change Lifecycle、Artifact 与 Event；既有 Ticket 02 的 `contracts/worker` 和 Daemon readiness seam。

**Status：** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 05` 未解除）

- [ ] 固定 loopback 路由 `/worker/v1/register`、`/worker/v1/heartbeat`、`/worker/v1/pull`、`/worker/v1/report`；`Execute` 保持 Worker 内部动作，不新增 Client 控制面 endpoint。
- [ ] 为每个 WorkerInstance 生成一次性身份和临时 Bearer secret；通过 Daemon 启动管道交给 `keystone-worker`，secret 不进入 JSON、命令行、Runtime 环境、日志或持久化明文。
- [ ] 所有 Worker 路由执行严格 JSON object、未知字段拒绝、单值检查、body 上限和 loopback/auth 校验；错误分类不泄漏 SQL、绝对路径、堆栈或 secret。
- [ ] Register 校验 `worker_id`、`protocol_version`、`capabilities`，记录注册状态并返回心跳间隔、Lease TTL 和 capability 结果，不返回 secret。
- [ ] Heartbeat 默认每 5 秒发送，允许携带当前 AgentRun 的有限摘要；Lease 续租和 Worker/run 绑定交给 Daemon Application Port，不在 Handler 中写 SQL。
- [ ] Pull 以 `worker_id` 请求；无任务时明确返回 `assignment: null`，有任务时只返回已授权的 AgentRun、opaque `lease_token`、过期时间、Workspace、Runtime、instruction、before revision 和输入摘要。
- [ ] Report Handler 只负责鉴权、严格解码和调用 Report Authority Port；Lease 分类、Artifact 落盘、AgentRun 终态和 Event transaction 由本地 Ticket 02 负责。
- [ ] `contracts/worker` 继续只依赖标准库 JSON 和传输类型，不导入 Domain、Daemon、SQLite、HTTP server 或 Runtime 实现。
- [ ] 测试覆盖未知字段、多个 JSON 值、空 Assignment、错误 Bearer、旧 Worker 身份、loopback 外请求、secret 不回显和有效请求转发；不以 fake handler 绕过协议。

## Implementation boundary

### Contract

沿用 Ticket 02 已冻结的 Register、Heartbeat、Assignment、Report 基础字段。为 Pull、Lease 过期、Report disposition 和四类 Artifact 增加字段时，保持面向边界的 DTO：

- `Assignment` 增加 `lease_expires_at`、`instruction`、`before_revision` 和有序输入摘要；不增加 Artifact 物理路径、Change 内部状态或可执行命令数组。
- `Report` 增加 exit/time/revision、强类型 Artifact payload 和 capture failure；`outcome` 仍是可扩展传输字符串，Contract 不解释 `human_required`。
- Pull response 的无任务值是 `null`，不能把空 object 当成兼容形式。
- Report response 只返回有限 `disposition`、稳定 error code、AgentRun/Lease 摘要和是否需要同一 Report 重试，不返回内部事务或文件路径。

### Authentication

Daemon 为每次 Worker 进程启动生成 secret，使用启动管道传递；Worker 以进程内内存保存并在 HTTP Authorization header 中使用。Daemon 只保存 hash，进程退出或重启后旧 secret 立即失效。比较 secret 必须使用常量时间比较，且错误日志只记录 WorkerInstance 摘要。

认证成功不等于 Lease 有效：Worker 身份、AgentRun、attempt、Lease token 和过期时间仍由 Ticket 02 的 authority service 校验。未知身份或完全无效 secret 不产生业务 Trace；已鉴权但旧 Lease 的 Report 是否保存 LateReport 由 02 的规则决定。

### Route ownership

HTTP Handler 只完成参数边界、鉴权上下文、DTO 转换、调用 Application 和响应状态码。它不能直接查询 `t_*` 表、更新 AgentRun、生成 Event、写 Artifact 或启动 Runtime。Daemon 组合 Worker protocol handler 与 Ticket 02 的 Application Port；Worker 进程只作为主动 HTTP Client。

## Acceptance

- Register/Heartbeat/Pull/Report 路由在 loopback 上可以被独立 Worker 调用，错误请求被稳定拒绝。
- Pull 没有待执行 Assignment 时返回 JSON `null`，有 Assignment 时不泄漏内部路径或 secret。
- 旧 Worker secret、错误 Bearer、非 loopback 请求和未知字段不能改变任何 AgentRun、Change、Lease 或 Event 权威状态。
- Contract package 的测试不需要启动 SQLite、Codex 或真实 Worker；Handler 测试通过 Port seam 验证转发边界。
- `contracts/worker/AGENTS.md`、`contracts/worker/INDEX.md` 和受影响根级导航仍准确描述当前已实现行为；规划字段不能被写成已经落地的运行事实。

## Verification

执行 Contract/Handler 单测、`go test ./...`、必要的 `go vet ./...`、`make build` 和 `git diff --check`。测试日志不能把 mock Worker 注册或 fake Report 写成真实 Codex smoke；真实端到端证据由 06-05 生成。

## Out of scope

- Lease 终态、Report 幂等、Artifact 原子落盘和 AgentRun/Change/Event transaction（06-02）。
- Worker 子进程启动、重启、Heartbeat/Pull loop 和关闭（06-03）。
- RuntimeAdapter、Codex 命令、stdout/stderr/diff 采集和 ExecutionGuard（06-04）。
- Worktree 创建、Ticket 调度、生命周期自动推进、Remote Worker、TLS 和完整 OS sandbox。

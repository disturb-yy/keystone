# ADR-0034：Execute 请求身份与执行观察边界

## 状态

已接受。

## 决策

`ExecuteCommand` 使用包含规范化 WorkspaceBranch 的 ExecuteRequestIdentity。相同 IdempotencyKey 与相同身份只重放首次 accepted 结果；当同一 Change 已有活跃 ExecutionSession 时，不同幂等键的再次 Execute 返回 `execution_in_progress`，不得创建第二个 Workspace、Authorization 或 Assignment。默认 branch 由 Daemon 确定；自定义 branch 只能由带该参数的显式 ExecuteCommand 作为一次受限本机人工批准提出并持久化，自动 Scheduler 不得自行选择它。

Client 通过 ExecutionReadModel 观察会话状态、branch、Ticket 状态和 Artifact 身份摘要。绝对 Workspace 路径、Lease token、原始 Prompt、Runtime 环境与凭据不进入 Control Plane response、错误或日志；完整不可变 Evidence 只沿 Artifact 读取边界取得，V1 不提供 Workspace 文件浏览。

## 理由与边界

规范化请求身份使断线重试可安全重放，同时让不同调用者不能借由新幂等键隐式重启已执行的 Change。将自定义 branch 的批准限制在显式本机 Command，保留 Ticket 09 的自定义能力而不引入通用 Governance approve API。观察模型只公开可复盘的业务状态，避免把 Worker 执行权、宿主机布局或秘密泄漏为 Client Contract。该 ADR 只定义 Ticket 09 的目标 API 与信息边界，不证明端点、CLI 或查询模型已实现。

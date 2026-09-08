# 11-04：Change Detail、Needs Human 与安全操作

> 状态：`ready-for-agent`（规划已对齐，尚未实现）。父 Ticket：[11 Dashboard Observation](../../11-dashboard-observation.md)；规格：[11-dashboard-observation-spec.md](../spec/11-dashboard-observation-spec.md)。
>
> 依赖：11-01 Query Contract、11-02 Daemon 静态托管/SSE、11-03 Dashboard shell；Ticket 10 的 Verify/Commit/FinalVerify 真实 ReadModel 是本子票的内容前提。

## 目标

完成 Change Detail 和 Needs Human 两个页面，以统一 `ChangeObservationReadModel` 呈现生命周期、Ticket Graph、当前运行、Artifact、Trace、Health 和允许操作，并以 Dialog/Drawer 安全提交有限的 Command/Decision。

## 实现范围

- Change Detail 路由为 `/changes/:change_id`，首屏只请求 `GET /v1/changes/{change_id}/observation`；必要的独立 Trace/Artifact 内容查询仍由 typed client 按有界范围发起。
- Lifecycle 使用 TDesign Steps 展示 `LifecycleStage`，使用 Tag 独立展示 `ChangeStatus`；`human_required` 不渲染为 Lifecycle Stage。
- Ticket Graph 使用 Canonical order Table 展示 Ticket、状态、`BLOCKED_BY`、当前 Gate 和服务端摘要；不在 Client 推导依赖完成、frontier 或 runnable。服务端提供合适层级数据时才辅助使用 Tree。
- Execution 展示 Ticket Gate、Verification、Keystone Commit、CandidateRevision 等有界摘要；禁止 raw command output、Workspace path、Lease、Prompt 和凭据。
- Current Run 只展示 Daemon 标记的活动 AgentRun；Completed AgentRun 进入 Trace；Worker/Lease health 作为独立 Health section。
- Trace 以 `EventSequence` 为顺序使用 Timeline，独立显示 Event、Run、ArtifactRef 和 HumanDecision；Artifact 只显示身份、摘要、size 和有界 content preview。
- Needs Human 路由为 `/needs-human`，表格来自 `GET /v1/needs-human`；仅展示 Daemon 标记的 `human_required`/`HUMAN_REQUIRED` 项，不做客户端推断。
- Decision Drawer 显示 reason、evidence references、当前 version 和 Daemon 提供的 `available_actions`；HumanDecision 只能提交 `retry` 或 `cancel`。
- Change Command 通过 Dialog/Popconfirm 提交 `pause`、`resume`、`cancel`；不提供 Execute、Verify、Commit、FinalVerify 或任何 Git/Workspace 控制。
- 每次用户动作生成一个 `Idempotency-Key`；请求重试复用该键；成功后重新 Query，不做 optimistic update；按钮在请求期间 disabled/loading。
- `409 change_version_conflict` 显示陈旧提示并要求重新 Query，不自动重送不同 version；API 错误、断线、section `not_yet_available` 和 stale snapshot 均可见。

## 验收条件

- 真实 Daemon 生产构建中，Change Detail 能按 Query 展示 Lifecycle、Ticket Graph、Current Run、Execution、Artifact、Trace、Health 和 section 状态。
- 至少一个真实 `human_required` Change 可在 Needs Human 定位、打开证据、提交合法 retry/cancel 并在响应后重新 Query。
- Pause/Resume/Cancel 的成功响应显示 Daemon 返回结果；重复网络提交不会产生第二个副作用；陈旧 version 得到 409 且不自动重试。
- EventSequence、Ticket canonical order、ChangeStatus/LifecycleStage 分离和 available_actions 均由服务端数据驱动。
- 断线/刷新后页面重新 Query；最后成功 snapshot 可标记 stale，但不能遮蔽 Query error 或伪造状态。
- `npm run lint`、`npm run build`、`make dashboard-build` 及真实浏览器验收通过。

## 不包含

- 任何新的 Control Plane 业务规则、Verification/Commit 实现、Worker 操作或数据库访问。
- 无限滚动、完整 Trace 导出、完整日志查看、Artifact 下载绕过摘要校验。
- 客户端状态机、乐观状态变更、自动重试命令、自动恢复 Workspace 或自动提交 Decision。

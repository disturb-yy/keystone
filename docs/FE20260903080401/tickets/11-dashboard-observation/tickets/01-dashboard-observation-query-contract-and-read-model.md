# 11-01：Dashboard Observation Query Contract 与 ReadModel

> 状态：代码已实现；Contract/Workstore/Daemon 局部测试已覆盖。父 Ticket：[11 Dashboard Observation](../../11-dashboard-observation.md)；规格：[11-dashboard-observation-spec.md](../spec/11-dashboard-observation-spec.md)。
>
> 上游边界：M7/M8 真实 acceptance 按用户授权跳过；Ticket Graph/Execution 尚未形成时由 section 返回 `not_yet_available`，不伪造上游事实。

## 目标

为 Dashboard 建立由 Daemon 形成的有界 Query Contract 和 `ChangeObservationReadModel`，使四个页面不需要直接访问 SQLite、Domain Entity 或 Worker Protocol，也不需要在 Client 中推导生命周期与治理结论。

## 实现范围

- 在 `contracts/controlplane` 增加面向 Dashboard 的 Project inventory、Project-scoped Change list、Needs Human、Observation envelope、section availability、pagination 和安全 Health DTO。
- 在 Daemon/Application/Read Store 适配层增加对应只读 Query；Handler 只负责路径、参数、DTO 和错误映射，不直接写 SQL。
- 复用已有 `ChangeReadModel`、Trace 和 Artifact 边界；在 `ChangeObservationReadModel` 中以 section 组合，不复制完整历史或原始 Artifact。
- 提供目标端点：
  - `GET /v1/projects`
  - `GET /v1/projects/{project_id}/changes`
  - `GET /v1/changes/{change_id}/observation`
  - `GET /v1/needs-human`
  - 继续支持独立的 Ticket Graph、Execution、Trace、Artifact 和 Daemon status Query。
- 所有列表使用默认 `limit=50`、最大 `limit=200`、服务端游标、`has_more` 和 `next_cursor`。
- 固定稳定排序：Projects 为 `project_id` 升序；Changes 为 `created_at` 降序再 `change_id` 降序；Needs Human 为 `required_at` 升序再 `change_id` 升序。
- 每个 section 显式返回 `available` 或 `not_yet_available`；真实空集合返回 available 加空数组。依赖的数据库或权威 ReadModel 不可用时，整个 Query 返回结构化服务错误，不返回混合快照。
- Observation 的 `available_actions` 由 Daemon 形成；至少覆盖允许的 Pause、Resume、Cancel 与 HumanDecision retry/cancel，具体集合服从当前权威状态和版本。
- 通过测试固定安全字段、有界性、Project/Change 关联、排序、游标、section 状态、404/409/503 错误映射和不泄露规则。

## 权威与安全边界

- `LifecycleStage`、`ChangeStatus`、Ticket execution state、Gate、VerificationOutcome、KeystoneCommit、CandidateRevision、Human Required 和 Worker health 均来自 Daemon 权威事实。
- Client 不根据 `AgentRun` outcome、heartbeat 时间、空数组、错误文案或 SSE 事件推导状态。
- DTO 不包含绝对 Repository/Workspace/database path、Lease token、Prompt、环境变量、凭据、Worker secret、完整日志或未限制 Artifact 内容。
- `repository_root` 仅在 Project Detail 作为本机 Project identity 的显示字段；不得扩展为内部数据库或 Workspace 位置。
- 所有时间为 UTC RFC3339Nano，ID 使用边界格式；任何 unbounded 字段必须拒绝或投影为受限摘要。

## 验收条件

- Contract 测试覆盖 JSON 字段、section availability、分页元数据和不允许的字段不会被序列化。
- Daemon 集成测试能从真实权威读模型返回 Project、Change、Needs Human 和 Observation；不存在的 Project/Change 得到稳定错误。
- Ticket Graph、Execution 尚未形成时响应为 `not_yet_available`，而不是伪造空图、伪造 Gate 或伪造 `integrate_ready`。
- 数据库/读模型不可用时没有部分成功的观察 envelope。
- `go test ./...`、`go vet ./...` 与适用的 Contract/Daemon 测试通过。

## 不包含

- Dashboard React 页面、TDesign 组件、浏览器 hooks、SSE、静态资源托管。
- Ticket Graph、Execution、Verification 或 Commit 业务规则的实现；只消费 Ticket 08/10 形成的权威 Query 能力。
- Client 侧排序、分页、Lifecycle、frontier、Human Required 或 Worker health 推导。

# 11：Dashboard Observation 实施规格

> 状态：规划规格已对齐，尚未实现。`BLOCKED_BY: 10` 是实现硬阻塞；本文件及子票的 `ready-for-agent` 只表示契约已可实施，不表示当前 checkout 已有 Dashboard、Query、SSE 或 M9 行为。
>
> 关联：[Ticket 11](../../11-dashboard-observation.md)、[Dashboard DESIGN.md](../../../../../dashboard/DESIGN.md)。

## Problem Statement

Ticket 10 完成后，Daemon 才能拥有可观察的 Ticket Verify、Keystone Commit、Final Verify 和 `integrate_ready` 事实。Dashboard 需要把这些权威事实以本机、可复盘的方式呈现给操作者，同时允许操作者提交有限且显式的暂停、恢复、取消和 Human Decision。

当前 `dashboard/` 仍是 React/TypeScript/Vite 骨架；现有 `/v1` 只覆盖已落地的 Daemon、Project、Change 与 Trace 边界，Ticket Graph、Execution ReadModel、Needs Human 查询、刷新流和生产静态托管尚未形成当前运行证据。Ticket 08、Ticket 10 的规划文字不能当作这些端点已经存在的证据。

本 Ticket 的问题不是在浏览器复制一份生命周期状态，而是建立一条窄的观察链：Daemon 从权威状态形成有界 Query，Dashboard 只重建并显示 Query，SSE 只提示重新查询，所有副作用仍由 Daemon 的 Command/Decision 边界收口。

## Solution

M9 交付一个由同一 Daemon 托管的生产 Dashboard，提供四个页面：Projects、Project Detail、Change Detail 和 Needs Human。页面通过同源 `/v1` Query 获取快照，通过一个可选过滤的 `/v1/updates` SSE 连接接收刷新提示；断线、重连或浏览器刷新都回到 Query 作为唯一状态来源。

```text
Browser
  ├── GET /                         → Projects
  ├── GET /projects/:project_id     → Project Detail
  ├── GET /changes/:change_id       → Change Detail
  ├── GET /needs-human              → Needs Human
  ├── GET /v1/...                   → bounded authoritative Query
  ├── GET /v1/updates               → event: refresh only
  └── POST /v1/...                  → explicit Command / HumanDecision
                                      ↓
                                  Control Plane Daemon
                                      ↓
                              authoritative state and Trace
```

Dashboard 不是 Control Plane，也不拥有 Lifecycle、Ticket Graph、Execution、Gate、Verification、Commit、Worker Lease 或 Human Decision 的权威状态。任何页面刷新后都必须能够仅凭公开 Query 重建可见状态。

## User Stories

1. 作为本机 Project 操作者，我希望从 Projects 看到已由 Daemon 注册的 Project，并能进入该 Project 的 Change 列表。
2. 作为 Change 操作者，我希望在一个页面按事实顺序看到 Lifecycle、Canonical Ticket Graph、当前运行、Execution、Artifact、Trace 与 Daemon/Worker Health。
3. 作为恢复操作者，我希望直接定位 Daemon 标记为 `human_required` 的 Change，查看有界证据并提交受允许的 retry 或 cancel Decision。
4. 作为审计者，我希望 SSE 丢失、浏览器刷新或 Query 失败时，不会把客户端缓存或事件顺序误认为新的业务状态。
5. 作为系统维护者，我希望 Dashboard 不接触 SQLite、Workspace、绝对路径、Prompt、Lease、凭据或 Runtime 命令输出。

## Scope and Non-goals

### Scope

- Daemon 托管 Dashboard 生产构建，并提供四个明确路由。
- 增加或扩展 `contracts/controlplane` 的 Dashboard Observation Query DTO，以及 `internal/daemon` 的只读 Query adapter。
- 提供 Project inventory、Project-scoped Change list、Change observation、Needs Human 和 bounded Health 数据。
- 保留 Ticket Graph、Execution、Trace、Artifact 和 Health 的独立 Query 能力，并由 Daemon 组合 `ChangeObservationReadModel`。
- 提供 `/v1/updates` 的单一 SSE 刷新流；事件仅携带资源类型及可选 Project/Change 关联。
- 以 TDesign React 组件实现页面、状态反馈和有限 Command/Decision 交互；前端样式固化在 [`dashboard/DESIGN.md`](../../../../../dashboard/DESIGN.md)。
- 以真实 Daemon、生产构建和浏览器完成 Projects 到 Change Trace 的验收。

### Non-goals

- 浏览器直连 SQLite、文件系统、Git、Workspace、Worker Protocol 或 Runtime。
- 在 Client 中计算 Lifecycle Truth、Ticket status、Runnable frontier、Gate、Verification verdict 或 Worker freshness。
- 将 SSE 作为状态快照、事件回放、离线队列、可靠消息或 Command 通道。
- 暴露完整 Event history、完整日志、原始 Artifact、Prompt、环境变量、凭据、Lease token、Workspace path 或 database path。
- 在 Dashboard 中启动 Execute、Verify、Commit、FinalVerify、merge、push、deploy 或任意 Git/Workspace 副作用。
- 团队、RBAC、远程访问、Plugin Marketplace 或完整运维控制台。

## Implementation Decisions

### 事实边界与上游条件

- 实现开始前必须重新确认 Ticket 10 已有真实 acceptance evidence；Ticket 10 的规划文档、`ready-for-agent` 状态或 fake seam 不能解除 `BLOCKED_BY`。
- Ticket 08 的 Canonical Ticket Graph 与 Ticket 10 的 bounded Execution ReadModel 是 M9 的上游事实。M9 不自行重建它们，也不复制它们的 Domain 规则。
- `ChangeObservationReadModel` 是 Daemon 侧的薄组合模型，不是新的业务 Aggregate，不改变现有 Change、Execution 或 Trace 的权威持久化边界。
- Dashboard 使用 `tdesign-react` 和 `tdesign-icons-react`。实现前通过 `tdesign-mcp-server` 的组件列表、组件文档、DOM 结构与图标检索确认 API；组件不满足需求时才用 `dashboard/DESIGN.md` 中的组合 CSS。

### 权责与依赖

| 区域 | M9 责任 | 明确不负责 |
| --- | --- | --- |
| `dashboard/` | 请求 Query、重建视图、展示状态、提交允许的 Command/Decision | 业务状态、SQLite、Lifecycle 推进、SSE 状态重放 |
| `contracts/controlplane/` | 定义版本化、面向边界的 Query/Command/Decision DTO 与安全错误 | Domain Entity、SQL、HTTP Handler、客户端缓存 |
| `internal/daemon/` | 从 Application/Repository 组合有界 Query，托管静态资源，发出刷新提示 | 在 Handler 中写 SQL、从 Worker 自报推导业务状态 |
| `internal/work`、`internal/governance`、`internal/execution` | 继续拥有 Project、Change、Ticket、Gate、Execution、Decision 的业务权威 | Dashboard 组件、浏览器路由 |
| `internal/infrastructure/workstore` | 实现所需 Query projection 与有界读取 | 为 Dashboard 创建第二套状态账本 |
| Worker | 提供 Daemon 已授权的运行事实及能力/心跳输入 | Dashboard API、Lifecycle、Gate、Decision、DB |

### Query API

M9 使用下列端点作为目标边界。已存在端点尽量保持兼容；新增 DTO 必须在 `contracts/controlplane` 中显式建模，不能把 Domain Entity 直接 JSON 化。

| 方法与路径 | 用途 | 结果边界 |
| --- | --- | --- |
| `GET /v1/projects` | Projects inventory | Project identity、规范化 repository root、状态摘要和有界分页 |
| `GET /v1/projects/{project_id}` | Project Detail 的 Project 摘要 | 单一 Project 快照，不包含无界 Change/事件历史 |
| `GET /v1/projects/{project_id}/changes` | Project-scoped Change list | 只返回该 Project 的 Change 摘要和分页元数据 |
| `GET /v1/changes/{change_id}` | 兼容现有 Change 快照 | 继续返回 bounded `ChangeReadModel` |
| `GET /v1/changes/{change_id}/observation` | Change Detail 主 Query | 返回统一的 `ChangeObservationReadModel` envelope |
| `GET /v1/changes/{change_id}/ticket-graph` | 独立 Ticket Graph Query | 返回 Canonical order、`BLOCKED_BY` 和服务端形成的状态/前沿摘要 |
| `GET /v1/changes/{change_id}/execution` | 独立 Execution Query | 返回 Ticket 10 定义的 gates、Verification、Commit、CandidateRevision 等安全摘要 |
| `GET /v1/changes/{change_id}/events`、`runs`、`artifacts`、`decisions` | 独立 Trace Query | 每项固定上限、稳定排序和有界摘要；不返回完整历史或原始内容 |
| `GET /v1/daemon/status` | Daemon Health | 复用现有 Daemon readiness/status 语义；不把它当作 Project 或 Worker 业务状态 |
| `GET /v1/needs-human` | Needs Human inventory | Daemon 形成的有界待人工项目，支持 Project/Change 过滤 |
| `GET /v1/updates` | SSE 刷新提示 | 只返回 `event: refresh`，不返回快照、Artifact、Command 或 Decision |

列表查询统一使用 `limit` 与 `cursor`：默认 `50`，最大 `200`，超限返回结构化 `invalid_request`。每个列表必须给出 `has_more` 和存在更多结果时的 `next_cursor`，游标不可由 Client 自行解释。排序必须在服务端固定并包含稳定的 ID tie-breaker：Projects 按 `project_id` 升序，Changes 按 `created_at` 降序再按 `change_id` 降序，Needs Human 按 `required_at` 升序再按 `change_id` 升序。不得用墙钟当前时间或 Worker 到达顺序决定结果。

### ChangeObservationReadModel

Change Detail 的主响应为有界 envelope。示意字段如下，具体 DTO 以 `contracts/controlplane` 的实现为准：

```text
ChangeObservationReadModel
├── schema_version
├── observed_at
├── project
├── change
├── lifecycle
├── ticket_graph
├── execution
├── trace
├── artifacts
├── health
└── available_actions
```

规范如下：

- 所有时间为 UTC RFC3339Nano；所有关联以 ID 和安全摘要表达。
- `lifecycle` 同时显示 `LifecycleStage` 与 `ChangeStatus`；二者不能合并成一个字段。`human_required` 是 `ChangeStatus`/治理结果，不是新的 LifecycleStage。
- `ticket_graph` 只显示 Daemon 已形成的 Canonical order、Ticket 摘要、`BLOCKED_BY` 和服务端状态；Client 不计算依赖完成、frontier 或 runnable。
- `execution` 只显示 Ticket 10 的 bounded gate、Verification、Commit、CandidateRevision 和当前执行摘要；不显示 Workspace path、Lease token、Prompt、环境或凭据。
- `trace` 以 `EventSequence` 排序展示 Event；`AgentRun`、`ArtifactRef`、`HumanDecision` 保持独立关联，不把它们压成一条可变日志文本。
- `artifacts` 只展示 kind、identity、size、摘要、关联和受限 content preview；原始内容必须继续通过受控 Artifact API 按摘要校验读取。
- `health` 由 Daemon 形成，包含 Daemon readiness 及 Worker availability/capabilities/last heartbeat 等已授权摘要；Client 不自行计算 freshness 或健康结论。
- `available_actions` 是 Daemon 根据当前权威状态和版本给出的允许操作集合。Dashboard 只渲染集合，不从 status 推导按钮。

每个组合 section 都显式标记其可用性：

- `available`：该 section 已由 Daemon 形成；集合为空表示真实 empty，不等于缺失。
- `not_yet_available`：上游生命周期尚未形成该 section，返回安全的 reason code，不用空数组伪装已完成。
- Query 所依赖的数据库或权威读模型不可用时，整个 Query 返回结构化服务错误；不得返回混合了成功与失败来源的伪快照。

### Needs Human

`GET /v1/needs-human` 只返回 Daemon 已标记的待人工项目。每一项至少包含：`project_id`、`change_id`、可选的 Ticket/Execution scope、规范化 `reason_code`、受限 reason summary、允许的 `available_actions`、当前 `change_version`、`required_at` 和有界 evidence references。

Needs Human 页面只根据这些项目定位 Change；它不通过 `AgentRun` outcome、错误文案、超时、Worker heartbeat 或空 section 推导 `human_required`。Decision Drawer 显示 Daemon 返回的 evidence identity，并只提交当前 Query 明确允许的 `retry` 或 `cancel` HumanDecision。

### SSE RefreshHint

`GET /v1/updates` 是同一 Dashboard 的单一 EventSource 连接，可选 `project_id`、`change_id` 过滤。服务器发送：

```text
event: refresh
data: {"resource_type":"change","project_id":"...","change_id":"..."}
```

`resource_type` 使用受控值；关联 ID 可省略但不能伪造。SSE 不携带状态、内容、命令、Decision、cursor 或 replay token，不保证事件不丢失，也不作为可靠队列。客户端收到事件后按关联范围重新发起 Query；断线和重连执行同样的 re-query，并保留最近一次成功 snapshot 作为标记为 stale 的显示数据，不能乐观修改字段。

### Command 与 HumanDecision

Dashboard 仅暴露现有/已批准的显式边界：

- `POST /v1/changes/{change_id}/commands`，`command` 只能是 `pause`、`resume` 或 `cancel`。
- `POST /v1/changes/{change_id}/decisions`，`decision` 只能是 `retry` 或 `cancel`，并可携带受限 `reason`。
- 请求带 Query 观察到的 `expected_version`，并通过 `Idempotency-Key` 提交幂等键。
- 每次用户动作在提交前生成一个幂等键；网络重试复用同一个键，不能为同一用户动作创建新键。
- 提交期间按钮进入 loading/disabled；成功后重新 Query，不做 optimistic update。
- `409 change_version_conflict` 只提示快照已陈旧并要求重新 Query；Dashboard 不自动改写 version、不自动重送不同请求，也不连续重试。
- Dashboard 不显示或调用 Execute、Verify、Commit、FinalVerify 等 M8/M9 之外的命令。

### Dashboard 页面与组件

页面结构固定如下：

| 路由 | 主内容 | 关键状态 |
| --- | --- | --- |
| `/` | Projects 表格 | loading、empty、error、stale |
| `/projects/:project_id` | Project 摘要与 Change 表格 | loading、empty、error、分页 |
| `/changes/:change_id` | Lifecycle、Ticket Graph、Execution、Trace、Artifact、Health | section `not_yet_available`、Human Required、断线 |
| `/needs-human` | 待人工表格与 Decision Drawer | evidence、retry/cancel、409 |

页面使用 TDesign `Layout`、`Menu`、`Breadcrumb`、`Card`、`Table`、`Tree`、`Timeline`、`Tag`、`Alert`、`Dialog`、`Drawer`、`Skeleton`、`Empty`、`Result`、`Button`、`Popconfirm`、`Statistic`、`Space`、`Typography` 等组件。Canonical Ticket 主视图使用 Table；Tree 仅在服务端已有适合的层级数据时作为辅助，不在浏览器构造依赖图。Lifecycle 使用 Steps，Status 使用 Tag，Trace 使用 Timeline。

详细色彩、字号、间距、响应式和可访问性规则以 [`dashboard/DESIGN.md`](../../../../../dashboard/DESIGN.md) 为唯一 UI 样式基线；页面实现不得在组件中重新选择一套颜色或间距。

### Client 请求与状态模型

Dashboard 使用 typed native `fetch` client 和页面级 local hooks，不引入全局业务状态库。每个 Query 使用 `AbortController` 和 request sequence，旧响应不能覆盖新响应；SSE 连接由应用 shell 管理但只触发页面 Query。

页面至少区分：loading、available empty、available data、error、stale snapshot、SSE disconnected/reconnecting、`human_required` 和 section `not_yet_available`。错误和断线时保留最近一次成功数据，但必须显示 stale/disconnected 标记；首次 Query 失败没有可展示的伪数据。Query 错误不会被空数组吞掉。

## Security and Privacy

所有 Browser 请求均为同源 `/v1`；Daemon 继续是唯一 Control Plane 入口。Query/DTO 的字段审查必须拒绝暴露：绝对 Repository/Workspace/database path、Lease token、Prompt、环境变量、凭据、Worker secret、原始命令输出、未限制的 Event payload 和未校验 Artifact 内容。

Project Detail 可显示用于本机身份确认的 `repository_root` 摘要/路径，但不得扩展为 `database_path`、Workspace path 或运行时内部位置。所有 content preview 必须有字节/长度上限，并沿用 Artifact 摘要校验边界。

## Verification and Acceptance

### 自动验证

```bash
go test ./...
go vet ./...
make build
make dashboard-build
git diff --check
```

`make dashboard-build` 必须通过 `dashboard/package-lock.json` 安装依赖并构建生产资产；不能只在 Vite dev server 中证明页面可访问。文档-only 阶段只运行适用的 `git diff --check`，不把未实现代码的验证写成已通过。

### 浏览器验收

使用真实 Daemon 和生产构建完成一次可复核路径：

1. 打开 Projects，确认 Query 数据、loading/empty/error 至少有一条真实可观察路径。
2. 进入 Project Detail，再进入 Change Detail，观察 Lifecycle、Ticket Graph、Current Run、Artifact、Trace 和 Health。
3. 让 SSE 连接断开并恢复，确认页面重新 Query，且结果与 Daemon 权威响应一致；浏览器刷新后同样成立。
4. 对一个 Daemon 标记为 `human_required` 的 Change 打开证据 Drawer，提交一个合法 Decision，确认返回结果后页面重新 Query；验证陈旧 `expected_version` 得到 409 并不自动重送。
5. 通过 SPA fallback 直接访问四个深链接，确认生产静态托管能返回页面；API 404 不能被静默回退为 `index.html`。

验收记录必须区分：源码/构建结果、Daemon runtime 事实、浏览器观察事实和独立评审结果。规划文档、mock data 和 dev server 不构成 M9 运行验收。

## Vertical Slices

| 子票 | 结果 | 依赖 |
| --- | --- | --- |
| [11-01](../tickets/01-dashboard-observation-query-contract-and-read-model.md) | Control Plane Observation Query Contract、Project inventory、Needs Human、Health | Ticket 08/10 真实事实；Ticket 10 是顶层阻塞 |
| [11-02](../tickets/02-sse-refresh-and-daemon-static-hosting.md) | SSE RefreshHint、Daemon 生产静态托管、SPA fallback | 11-01 的资源类型与 Query 边界 |
| [11-03](../tickets/03-dashboard-shell-projects-and-project-detail.md) | TDesign Dashboard shell、Projects、Project Detail | 11-01、11-02 |
| [11-04](../tickets/04-change-detail-needs-human-and-actions.md) | Change Detail、Needs Human、Command/Decision UX | 11-01、11-02、11-03 |
| [11-05](../tickets/05-ticket11-integration-verification-and-navigation.md) | 真实 Daemon 浏览器验收、构建验证、导航文档 | 11-01 至 11-04、Ticket 10 acceptance |

## Risks and Open Boundaries

- Ticket 10 若没有真实 Verify/Commit/FinalVerify ReadModel，M9 只能继续保持 planning，不能用静态 JSON 或客户端占位状态解除阻塞。
- 当前 Daemon 尚无 Dashboard 静态资源路由与 SSE 路由；实现时需保证 API 路由优先、SPA fallback 只作用于非 API 页面路径。
- 当前 Dashboard 没有 TDesign 依赖；依赖安装、版本锁定和组件 API 必须通过 `tdesign-mcp-server` 资料与 lockfile 共同核对。
- SSE 断线状态不等于业务失败；UI 只能标记连接质量并重新查询，不能修改 ChangeStatus。
- 任何新的 Query section 都必须保持 bounded、可用性显式且不引入第二套业务真相；若字段无法安全投影，返回 `not_yet_available` 或安全错误，而不是泄露内部对象。

## Decision Log

| 决策 | 结论 | 原因 |
| --- | --- | --- |
| Query 与 SSE 的关系 | Query 是唯一状态源，SSE 只发刷新提示 | 断线/重连可恢复，避免事件丢失改变状态 |
| Observation 形态 | Daemon 组合统一 envelope，同时保留独立 Query | 页面读取简单，Trace/Execution 仍保持边界和有界性 |
| Human Required | 只认 Daemon 标记和 `available_actions` | 客户端不推导治理结论 |
| Command 安全 | 版本前置条件、显式幂等键、成功后重查、409 不自动重送 | 防止陈旧页面和重复副作用 |
| 组件系统 | TDesign React 优先，样式 token 固化于 `dashboard/DESIGN.md` | 保持组件语义一致、减少局部漂移 |
| 页面形态 | desktop-first，四个固定路由，同源 Daemon 托管 | 适配本机 Control Plane 的观察工作流 |

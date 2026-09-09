# Keystone Dashboard 低保真线框

## Page Goal

为本机单操作者提供深色、高密度且分层的 Control Plane 界面。用户能够浏览已注册
Project、核查 Change 当前权威状态与证据、处置 Daemon 明确允许的人工操作，并从已注册
Project 创建新的 Change。

## Users and Primary Tasks

| 用户 | 首要任务 | 成功结果 |
| --- | --- | --- |
| 工程人员 | 浏览 Project 与 Change 队列 | 在不读取 SQLite 的前提下定位目标 Change |
| 工程人员 | 核查单个 Change 的阶段、证据和下一步 | 以 Daemon Observation 确认当前权威事实 |
| 工程人员 | 创建 Change | 选择已注册 Project、提交原始 Intent，并进入新 Change 详情 |
| 工程人员 | 处理 Human Required | 依据证据执行 Daemon 允许的 Retry 或 Cancel |

## Information Architecture

```text
Keystone Dashboard
├── Projects (/)
│   └── Project Detail (/projects/:project_id)
│       └── Change Detail (/changes/:change_id)
├── Create Change (/changes/new)
└── Needs Human (/needs-human)
```

- Projects 与 Needs Human 是高密度队列页面：摘要和筛选位于表格之前，行进入详情。
- Change Detail 是核查与处置页面：主工作区展示当前生命周期、Ticket、Execution、Trace 和
  Artifact；右侧固定显示 Change 权威状态、可用操作和 Daemon/Worker 健康摘要。
- Create Change 是聚焦输入页面：不混入 Change 观察内容，也不提供 Project 初始化写操作。

## ASCII Wireframe

### 全局 Shell

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Keystone · Local Control Plane                         Daemon ● Ready │
├───────────────┬──────────────────────────────────────────────────────┤
│ Projects      │ 页面标题 · 上下文                         [刷新]      │
│ Create Change │ ──────────────────────────────────────────────────── │
│ Needs Human   │ 页面主体                                               │
│               │                                                        │
└───────────────┴──────────────────────────────────────────────────────┘
```

### 队列页：Projects 与 Needs Human

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Projects                                      [刷新]                  │
│ 已注册仓库与其当前 Change 概览                                      │
├──────────────────────────────────────────────────────────────────────┤
│ [Project 总数] [活跃 Change] [需要人工处理]                           │
├──────────────────────────────────────────────────────────────────────┤
│ Project / Change        状态          阶段        更新时间       →   │
│ ──────────────────────────────────────────────────────────────────── │
│ repository root          active        Execute     刚刚           详情│
│ …                                                                    │
└──────────────────────────────────────────────────────────────────────┘
```

Needs Human 将首列替换为 Change，突出 `reason_summary`、当前版本、所需人工动作与
证据引用；Review 进入 Drawer，详情链接进入 Change Detail。

### Change Detail

```text
┌───────────────────────────────────────────────┬──────────────────────┐
│ Change ID · Project · 当前阶段                 │ 权威状态              │
│ 说明 / 最后更新时间                            │ active · Execute      │
│                                               │ version / revision   │
├───────────────────────────────────────────────┤ ──────────────────── │
│ [生命周期 Steps]                               │ 可用操作              │
│ 当前阶段、下一步、最近 Agent Run                │ [Pause] [Cancel]     │
├───────────────────────────────────────────────┤ ──────────────────── │
│ Canonical Ticket Graph                         │ Daemon / Worker 健康 │
│ 高密度表格，显示依赖与结构 frontier             │ ready / last heartbeat│
├───────────────────────────────────────────────┤                      │
│ Execution · Trace · Artifacts                  │                      │
│ 依层级展开的表格和时间线                        │                      │
└───────────────────────────────────────────────┴──────────────────────┘
```

### Create Change

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Create Change                                                        │
│ 将原始需求交给已注册 Project 的 Daemon 生命周期。                    │
├──────────────────────────────────────────────────────────────────────┤
│ Target Project *                                                     │
│ [选择已注册 Project：repository root · project id                ▾] │
│                                                                      │
│ Intent *                                                             │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ 描述希望 AI 完成的需求                                           │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│ ▸ Advanced: Idempotency Key                                          │
│                                                                      │
│ [取消]                                             [创建 Change]     │
└──────────────────────────────────────────────────────────────────────┘
```

没有已注册 Project 时，以 Empty State 替换表单控件，显示可复制的 `keystone init`
引导并保持创建按钮禁用。

## Component Tree

```text
DashboardShell
├── Header
│   ├── ProductIdentity
│   └── DaemonConnectionStatus
├── SidebarNavigation
│   ├── ProjectsLink
│   ├── CreateChangeLink
│   └── NeedsHumanLink
└── RouteOutlet
    ├── QueuePage
    │   ├── PageHeader
    │   ├── SummaryStrip
    │   ├── QueryStateNotice
    │   └── AuthorityTable
    ├── ChangeDetailPage
    │   ├── ChangeHeader
    │   ├── MainObservationColumn
    │   └── StickyAuthorityRail
    └── CreateChangePage
        ├── PageHeader
        ├── RegisteredProjectSelect
        ├── IntentTextarea
        ├── IdempotencyKeyDisclosure
        └── CreateChangeActions
```

## Interaction Flow

```text
Projects → Project Detail → Change Detail
                                   │
                                   ├─ Daemon available_actions → Command / Decision → re-query
                                   └─ Needs Human Review → Decision Drawer → re-query

Create Change → load registered Projects
              → select Project + enter Intent
              → generate or confirm idempotency key
              → POST /v1/changes
              → success: /changes/:change_id
              → failure: preserve draft and show recovery guidance
```

- 自动生成的幂等键在表单未变时用于重试；Project 或 Intent 改变后生成新的自动键。
- 高级区中的手动幂等键不由界面改写。
- 草稿保存在当前浏览器会话，关闭浏览器后清除。
- Query、SSE、Command 和 Decision 一律以 Daemon 响应为准，不做 optimistic update。

## Page States

| 页面 / 场景 | 初次加载 | 可用 | 空 | 错误或冲突 | 提交中 |
| --- | --- | --- | --- | --- | --- |
| Projects | 表格骨架 | Project 表 | 说明尚未注册 Project | Alert + 重试 | 不适用 |
| Project Detail | 摘要与表格骨架 | Project 与 Change 表 | 说明暂无 Change | Alert + 保留旧快照 | 不适用 |
| Change Detail | 主列与右栏骨架 | 当前阶段优先的 Observation | section 标记 `not_yet_available` | stale Alert；409 提示重新读取 | 操作按钮 loading |
| Needs Human | 队列表骨架 | 人工处置项 | 说明暂无待人工事项 | Alert + 重试 | Drawer 操作 loading |
| Create Change | Project 下拉骨架 | 可编辑表单 | 无 Project 时的 CLI 引导 | 保留输入、中文恢复建议与错误码 | 表单与按钮禁用 |

## Data Fields and API Mapping

| 页面 | 读取或写入 | 可见字段 | 操作边界 |
| --- | --- | --- | --- |
| Projects | `GET /v1/projects` | `project_id`、`repository_root`、Change 计数 | 行进入 Project Detail |
| Project Detail | `GET /v1/projects/:project_id`、`GET /v1/projects/:project_id/changes` | Project 身份、Change ID、阶段、状态、版本、更新时间 | 行进入 Change Detail |
| Change Detail | `GET /v1/changes/:change_id/observation` | lifecycle、ticket graph、execution、trace、artifacts、health、`available_actions` | Command/Decision 使用当前 `change.version` 与 `Idempotency-Key` |
| Needs Human | `GET /v1/needs-human` | Change、原因、版本、证据引用、可用操作 | 只提交 Daemon 指定的 Retry 或 Cancel |
| Create Change | `GET /v1/projects`、`POST /v1/changes` | 选中 Project 的 `repository_root` 与 `project_id`、原始 Intent、高级幂等键 | 请求体仅含 `repository_path` 与 `intent`；幂等键位于请求头 |

## Responsive Behavior

- `>=1200px`：保留完整侧栏；Change Detail 使用主工作区和粘性右栏；高密度表格展示完整列。
- `768px–1199px`：侧栏收窄；摘要换行；右栏移至主工作区下方；表格保留状态和操作列。
- `<768px`：侧栏收起为菜单；所有内容单列；表格允许横向滚动；Create Change 保持全部输入与提交能力。

## Accessibility Notes

- Shell 使用 `header`、`nav`、`main`、`aside` 和 `section` landmark；每页只有一个主标题。
- 深色 token 与状态文字共同表达状态，不依赖红绿颜色区分。
- Project Select、Intent 与高级幂等键区均有可访问名称、必填与错误说明关联。
- 创建失败、Query 失败、stale、断线和 Human Required 使用合适的 `role="alert"` 或 `role="status"`。
- Command、Decision 和 Create Change 的 loading 状态阻止重复操作，并保持键盘焦点可预期。

## Acceptance Criteria

- 五条路由共用一个深色、高密度 Shell，且导航语义与 URL 一致。
- Create Change 只能从 Daemon 返回的已注册 Project 中选择，不能输入任意路径。
- Create Change 保留原始 Intent、正确使用幂等键，并在成功后进入新 Change 详情。
- 无 Project、loading、empty、error、stale、409、断线和提交中状态均有可理解的界面反馈。
- Change Detail 首屏突出当前生命周期、下一步和可用操作，Evidence 与 Trace 在同页可达。
- 窄屏保留读取、创建与人工处置能力，不建立第二套信息架构。

## Open Decisions

当前线框没有未决产品或交互决策。

## Approval Record

| 日期 | 范围 | 批准来源 | 状态 |
| --- | --- | --- | --- |
| 2026-09-10 | 深色开发者控制台、五页覆盖、高密度结构、窄屏行为、Create Change 交互 | 用户在本会话逐项确认推荐方案 | 已确认，可进入视觉原型审批；不授权生产代码修改 |

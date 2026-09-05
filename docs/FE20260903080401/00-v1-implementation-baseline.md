# Keystone V1 Implementation Baseline

> 状态：本轮 V1 开工前对齐完成
>
> 语言：中文
>
> 约束：本文件不替代 Architecture v0.1，而是其 V1 实施收敛层。Architecture v0.1 的冻结决策不得被本文件静默推翻。

## 1. V1 目标

Keystone V1 必须真实跑通：

```text
Repository
→ keystone init
→ Project
→ Change
→ Intent
→ Understand
→ Design
→ Plan
→ Ticketize
→ Ticket Graph
→ Workspace
→ Worker
→ Codex / OpenCode
→ Verify
→ Commit
→ Integrate Ready
→ Dashboard 可观察
```

V1 的成功标准不是“每个架构概念都实现完整”，而是：

> 一个真实 Change 能完整穿过 Keystone，同时 Control Plane、Worker、Runtime、Git、Repository、Dashboard 的 Ownership 与 Extension Boundary 不被破坏。

---

## 2. 已确认的 15 项实施决策

### 2.1 技术栈

- Go 1.27
- 后端架构：DDD + Modular Monolith
- CLI：Go + Cobra
- Daemon：Go
- Worker：Go
- Dashboard：React + TypeScript + Vite
- Control Plane API：HTTP + JSON
- 实时观察：SSE
- 数据库：SQLite
- Git：系统 Git CLI
- Workspace：Git Worktree
- Runtime：Codex CLI / OpenCode CLI，经 Runtime Adapter 接入

DDD 是工程架构与领域组织方式，不把 V1 拆成微服务。

### 2.2 Monorepo 一级结构

```text
keystone/
├── AGENTS.md
├── INDEX.md
├── README.md
├── cmd/
│   ├── keystone/
│   ├── keystone-daemon/
│   └── keystone-worker/
├── internal/
│   ├── work/
│   ├── planning/
│   ├── governance/
│   ├── execution/
│   ├── traceability/
│   └── infrastructure/
├── contracts/
│   ├── controlplane/
│   └── worker/
├── dashboard/
├── migrations/
├── configs/
├── docs/
├── scripts/
├── go.mod
├── go.sum
└── Makefile
```

每个 Go package 必须包含：

```text
AGENTS.md  # 规则 / 边界 / 禁止事项 / 编码约束
INDEX.md   # 包导航 / 关键类型 / 入口 / 依赖关系
```

### 2.3 Daemon / CLI / Dashboard / Worker 运行关系

```text
CLI ─────────┐
             │ HTTP/JSON
Dashboard ───┼────────► Daemon
             │            │
             │            │ Worker Protocol
             │            ▼
             │          Worker
             │            │
             │            ▼
             │        Workspace
             │            │
             │            ▼
             │      Codex / OpenCode
```

- 一个用户级 Local Daemon 管理多个 Project。
- CLI 与 Dashboard 都是 Client。
- CLI 可以自动 ensure daemon。
- Dashboard 生产构建由 Daemon 托管静态资源。
- Worker 是独立进程，由 Daemon 启动与监管。
- Worker 主动连接 Daemon。
- Lifecycle Durable Truth 只属于 Daemon / Control Plane。
- Side Effect Execution 属于 Worker。

### 2.4 V1 最小 Domain Core

Durable Core：

```text
Project
Change
Lifecycle
Ticket
TicketDependency
Workspace
AgentRun
Event
```

轻量 Supporting Model：

```text
Actor
ArtifactRef
EvidenceRef
DecisionRef
```

其余概念如 Gate、Policy、Risk、Capability、TicketGenerator、Execution DAG，在 V1 保持明确边界，但允许先以 Port、Value Object 或单一默认实现表达。

原则：

> Architecture Concept Exists ≠ V1 Must Fully Implement It.

### 2.5 Control Plane API 与 Worker Protocol

Control Plane API：

- 表达用户意图与 Command / Query。
- 创建和查询 Project / Change / Ticket。
- 提交 Pause / Resume / Cancel / Retry / Decision。
- 展示 Lifecycle、Run、Artifact、Trace。
- Client 不允许直接设置权威状态。

Worker Protocol：

```text
Register
Heartbeat
Execute
Report
```

Worker 只执行已授权副作用并回传事实，不得：

```text
CreateChange
AdvanceLifecycle
CompleteTicket
ApproveGate
MarkIntegrateReady
Write Keystone DB
```

### 2.6 数据持久化

三层边界：

```text
Repository
→ 版本化项目知识与 Project Manifest

SQLite
→ Control Plane 权威状态、可查询元数据、Domain Event

Local Artifact Store
→ 大文本、原始日志、Diff、测试输出、阶段产物
```

所有 Keystone 自有表统一使用 `t_` 前缀，例如：

```text
t_projects
t_changes
t_tickets
t_ticket_dependencies
t_workspaces
t_agent_runs
t_workers
t_artifacts
t_decisions
t_events
```

V1 不采用完整 Event Sourcing。

### 2.7 `keystone init`

`keystone init` 只做最小 Project Bootstrap：

```text
Normalize invocation directory
→ ensure daemon
→ Daemon resolves Git Root
→ 检查 Manifest
→ register/reconcile Project
→ 写 .keystone/project.yaml
→ 验证 Manifest ↔ Local DB
```

不做：

- Repository 全量分析
- INDEX 生成
- AGENTS 生成
- CodeMap
- Conceptual Space
- Change 创建
- Workspace 创建
- AI 执行

### 2.8 Change 创建与生命周期推进

采用 `Create = Start`：

```text
keystone change create "<intent>"
  ↓
Intent
→ Understand
→ Design
→ Plan
→ Ticketize
```

默认自动推进，只有明确 Failure / Human Required / Pause / Cancel 才停。

各 Stage 通过 Artifact 交接。
Lifecycle Coordinator 只协调，不承担分析、设计、拆票或写代码工作。

### 2.9 Ticketize

```text
Plan Artifact
→ TicketGenerator Port
→ ToTicketsInspiredGenerator
→ Runtime
→ Structured Ticket Draft
→ Keystone Deterministic Validation
→ Canonical Ticket Graph
```

V1 真正执行的 Dependency 只需要：

```text
BLOCKED_BY
```

最小校验：

- 至少一个 Ticket
- title 非空
- generation key 唯一
- dependency 引用存在
- 无 self dependency
- blocking graph 无 cycle
- acceptance criteria 存在

### 2.10 Workspace / Git Worktree

V1：

> 一个 Change 一个 Git Worktree；Change 内 Ticket 串行复用该 Workspace。

- Change 创建时固定 `base_revision`。
- 创建 Change 时要求原 Repository working tree clean。
- 进入 Execute 时才创建 Worktree。
- Worktree 位于 `~/.keystone/workspaces/<project>/<change>/`。
- 一个 Agent Run 独占 Workspace。
- 默认分支：`keystone/change/<change-id>`。
- 支持自定义 branch name。
- Ticket Verify PASS 后由 Keystone 创建 commit。
- 支持自定义 commit log / commit template。
- Runtime 不拥有 `git commit / push / merge` 权限。

### 2.11 Runtime Adapter

Codex / OpenCode 都作为 Local CLI Runtime：

```text
Worker
→ RuntimeAdapter
  ├── CodexAdapter
  └── OpenCodeAdapter
→ CLI subprocess
```

Runtime：

- cwd 固定为 Assigned Workspace。
- 可以读写代码、运行测试 / build / lint / repository-local shell。
- 不允许 commit、push、推进 Lifecycle、批准 Verify、修改 Canonical Ticket Graph、直接写 DB。

Runtime structured output 只用于摘要和交接；执行事实由 Worker 独立采集。

### 2.12 Verify

V1 Verify 至少包含：

```text
1. Workspace / Git Checks
2. Deterministic Project Verify Commands
3. Independent Acceptance Criteria Review
4. Change-level Final Verify
```

Verify 结果：

```text
PASS
FAIL
HUMAN_REQUIRED
```

Implementer 与 Verifier 必须是不同 Agent Run，即便使用同一个 Local Worker 和同一个 Runtime。

Verifier 只读，不直接修复代码。

### 2.13 Dashboard

V1 只做 4 个主要页面：

```text
Projects
Project Detail
Change Detail
Needs Human
```

Change Detail 展示：

```text
Lifecycle
Tickets / BLOCKED_BY
Current Run
Artifacts
Trace
```

Dashboard 只能：

- 展示 Control Plane State
- 提交 Command / Decision

Dashboard 不能推导或拥有 Lifecycle Truth。

### 2.14 第一个真实 Golden Path

使用独立 Go HTTP Demo Repository。

初始能力：

```text
GET /
→ 200
→ hello
```

第一条 Change：

> 为 HTTP 服务增加 `/healthz` 健康检查接口，返回 JSON 状态，并添加自动化测试。

建议 Acceptance Criteria：

1. `GET /healthz` 返回 HTTP 200。
2. `Content-Type` 为 `application/json`。
3. Response 为 `{"status":"ok"}`。
4. 原有 `GET /` 行为不改变。
5. 必须新增自动化测试。
6. `go test ./...` 必须通过。

### 2.15 正式实施顺序

```text
M0 Engineering Foundation
 ↓
M1 CLI → Daemon → SQLite
 ↓
M2 Repository → Init → Project
 ↓
M3 Change → Lifecycle → Artifact/Event
 ↓
M4 Daemon → Worker → RuntimeAdapter → Codex
 ↓
M5 Understand → Design → Plan
 ↓
M6 Ticketize → Canonical Ticket Graph
 ↓
M7 Worktree → Execute → Diff
 ↓
M8 Verify → Commit → Integrate Ready
 ↓
M9 Dashboard → Full Golden Path
```

---

## 3. V1 明确不深挖

继续留到专题会话：

- 完整 Policy DSL
- 完整 Risk Model
- 复杂 Gate 类型
- 高级 Evidence 模型
- Remote Worker
- Worker Pool
- Team / RBAC
- 多 Runtime 智能路由
- 完整 Eval / Learning
- 高级跨 Change 并行
- Graph Database
- Plugin Marketplace
- Deploy / Operate / Maintain

---

## 4. V1 最终工程原则

> V1 可以粗糙，但必须真实运行。
>
> 先做 Thin Vertical Slice，再增加深度。
>
> 架构边界稳定，实现尽量简单。
>
> 所有实现都应优先回答：它是否让 Golden Path 再向前真实走了一步？

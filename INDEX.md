# Keystone 项目索引

## Purpose

本文件是 Keystone 的项目地图，不是项目规约。

- `AGENTS.md` 定义修改前置条件、分层职责、依赖方向、命名、测试和变更纪律。
- `INDEX.md` 记录当前路径、目标架构位置、上下游关系、导航入口和事实状态。
- `CONTEXT.md` 定义稳定术语；`docs/adr/` 记录已接受的难逆架构决策，二者不作为运行实现证据。
- `docs/FE20260903080401/` 记录版本化 Ticket 与实施规格；当前状态栏只描述 checkout 中已经存在的内容。

## 当前仓库状态

| 区域 | 当前内容 | 状态 |
| --- | --- | --- |
| `go.mod` | 模块 `github.com/disturb-yy/keystone`，Go `1.27`，依赖 `golang.org/x/sys` 与 `modernc.org/sqlite` | 已声明，支持 Ticket 02 的跨平台锁和纯 Go SQLite 基线 |
| `Makefile` | `test`、`build`、`lint`、`dashboard-build` 根级验证入口 | 已存在；Dashboard 目标使用 `package-lock.json` 执行 npm 校验/构建 |
| `docs/FE20260903080401/` | V1 基线、里程碑、验收清单和版本化 Ticket/规格文档 | 已存在；Ticket 02 的 spec 与 01-05 验收记录已在当前树 |
| `CONTEXT.md`、`docs/adr/` | 项目术语与已接受的架构决策 | 已存在；记录 LocalStateRoot、DaemonReadiness 等语义及本机 Daemon 控制边界，不表示 M1 已实现 |
| `cmd/`、`configs/` | `cmd/keystone`、`cmd/keystone-daemon`、`cmd/keystone-worker` 与 `configs` 的 `.gitkeep` | `keystone init`、Daemon 入口已实现；Worker 仍无运行实现 |
| `internal/infrastructure/` | 基础能力及 `manifest`、`repository`、`workstore` 三个 Ticket 04 adapter | 已有本机状态、Migration、Git/Manifest 和 Project SQLite 持久化能力 |
| `contracts/controlplane/` | `/v1` 版本前缀、错误 envelope、Daemon/Project DTO、`Idempotency-Key` 表达 | 已落地 JSON Contract package；HTTP Handler 位于 `internal/daemon` |
| `contracts/worker/` | `Register`、`Heartbeat`、`Assignment`、`Report` 传输 DTO | 已落地独立 JSON Contract package，无 Worker runtime |
| `docs/architecture-baseline/` | 架构参考目录 | 当前工作树不存在；目标架构文字不作为运行行为证据 |
| `dashboard/` | React、TypeScript、Vite 源码、`package.json` 与 `package-lock.json` | 已有可构建骨架，无业务页面 |
| `migrations/`、`scripts/` | 根级路径尚不存在 | Migration runner 位于 `internal/infrastructure/migration/`；Ticket 04 业务 Migration 由 `workstore` 提供 |

当前工作树已有基础 `.go`、Ticket 02 基础设施、Ticket 04 Project Bootstrap、Contract 测试以及 Dashboard 前端源码；Worker runtime 和后续业务领域仍未实现。`.agents/`、`.codex/` 和 `.idea/` 属于工作区或 IDE 工具目录，不纳入项目架构导航。

## Architecture Map

### L0 — 运行时关系

```text
Human
├── CLI / Dashboard
│       ↓
│   Control Plane API Contract
│       ↓
└── Control Plane Daemon
        ├── Application
        │      ↓
        │    Domain
        │      ↓
        │  Infrastructure Adapters
        │
        ↓
    Worker Protocol
        ↓
    Independent Worker
        ↓
    Execution Guard → Assigned Workspace → Runtime / Tools
```

这是目标运行时关系图，不是当前可执行调用链。架构参考将 V1 定位为 Local-first、Monorepo、一个本机 Daemon 管理多个 Project、一个独立 Worker 执行副作用，并默认使用 Git Worktree 作为 Workspace。

### L1 — Interface / Entry

| 路径 | 地图位置 | 当前状态 |
| --- | --- | --- |
| `cmd/` | CLI、Daemon、Worker 入口边界 | `cmd/keystone` 已提供 `init` 和 Daemon 生命周期命令；Worker 仍无运行实现 |
| `dashboard/` | Local Web UI Client | 已有 React/TypeScript/Vite 骨架与锁文件 |

### L2 — Contract

| 路径 | 连接对象 | 当前状态 |
| --- | --- | --- |
| `contracts/controlplane/` | CLI / Dashboard ↔ Daemon 的 Control Plane API | 已落地 `/v1` Daemon 与 Project DTO；路由位于 `internal/daemon` |
| `contracts/worker/` | Daemon ↔ Worker 的 Narrow Worker Protocol | 已落地四组 DTO；Worker 进程和协议处理尚未实现 |

### L3 — Application

Application 位于 Control Plane Daemon 内部；Ticket 04 的 Project Bootstrap Application 位于 `internal/work/`。架构参考中的协调节点包括：

- Lifecycle Coordinator：推进 Change 的宏观 Lifecycle Stage。
- Scheduler：从 Effective Ticket Graph 的 READY Frontier 选择 Runnable Work。
- Governance 协调：在敏感副作用前取得 Policy、Gate、Evidence 或 Human Decision。
- Recovery 协调：统一处理语义恢复和 Human Escalation。

详细的 Application 职责和依赖规则见 `AGENTS.md` 的 Application Rules 与 Dependency Rules。

### L4 — Domain

业务领域以 DDD Lite 组织，`domain/` 是业务强边界。当前只有 Ticket 04 所需的
`internal/work/` 与 `internal/work/domain/` 已创建；其他目标领域目录仍未创建：

| 目标路径 | 逻辑子系统 | 地图中的核心对象 |
| --- | --- | --- |
| `internal/work/`、`internal/work/domain/` | Work & Lifecycle | 当前已创建；Ticket 04 的 Project Bootstrap Domain 与 Application |
| `internal/planning/` | Intelligence & Planning | 尚未创建；Understanding、Context、Design、Plan、Ticket Generation |
| `internal/governance/` | Governance | 尚未创建；Policy、Risk、Gate、Evidence、Decision、Escalation |
| `internal/execution/` | Orchestration & Execution | 尚未创建；Frontier、Scheduler、Execution DAG、Assignment、Recovery Coordination |
| `internal/traceability/` | Traceability & Learning | 尚未创建；Artifact Lineage、Domain Event、Execution Trace、Eval、Incident |

五个逻辑子系统是横向职责边界，不拆成五个微服务；Lifecycle 是纵向主流程：

```text
Intent → Understand → Design → Plan → Ticketize → Execute → Verify → Integrate → Learn
```

### L5 — Infrastructure

| 目标路径 | 地图位置 | 当前状态 |
| --- | --- | --- |
| `internal/infrastructure/` | `config`、`logging`、`id`、`localstate`、`migration`、`manifest`、`repository`、`workstore` | 基础能力、Git/Manifest adapter 和 Project Bootstrap SQLite adapter |
| `internal/infrastructure/localstate/` | 数据根、目录初始化、跨平台单实例锁和运行元数据 | 已落地；不拥有 Daemon 生命周期或业务状态 |
| `internal/infrastructure/migration/` | 纯 Go SQLite `t_schema_migrations` runner | 已落地；只增量、事务应用、重复跳过和漂移失败 |
| `migrations/` | 根级数据库 Schema / Migration 文件 | 当前未创建；Ticket 04 业务 Migration 由 `internal/infrastructure/workstore` 提供 |
| `configs/` | 运行配置和默认配置 | 只有 `.gitkeep` |

Infrastructure 是 Control Plane 的基础设施适配区域。具体依赖和实现规则见 `AGENTS.md` 的 Infrastructure Rules。

### L6 — Worker / Side Effects

`cmd/` 是 CLI、Daemon 和 Worker 进程入口的目标边界。Worker 位于 Worker Protocol 下游，连接 Assigned Workspace、Runtime 和 Tools；当前没有已实现的 Worker 入口。

## Relationship Map

```text
Interface Adapter → Application → Domain
Infrastructure → Domain

cmd/keystone、cmd/keystone-daemon → Control Plane 具体实现
cmd/keystone-worker → Worker Protocol 与 Worker 具体实现

CLI / Dashboard → contracts/controlplane → Daemon
Daemon → contracts/worker → Worker
```

上图是目标架构关系；依赖方向和跨边界约束以 `AGENTS.md` 为唯一规约来源。

## Target Truth Map

以下 ownership 是目标架构约束，不是当前 checkout 的运行时事实。

| Owner | 当前地图中的权威内容 |
| --- | --- |
| Repository / Git | 源码、版本化项目知识、代码版本 |
| Control Plane Daemon | Project、Change、Ticket、Gate、Decision、Evidence、Execution、Artifact Lineage、Domain Event |
| Worker | Side-effect 执行、运行句柄、心跳、Workspace、日志 |
| Runtime / Tools | Runtime-specific 行为和工具结果 |

## Navigation

| 任务 | 地图入口 | 相关地图 |
| --- | --- | --- |
| CLI 入口 | `cmd/keystone/` | L2 `contracts/controlplane/`、L3 Application |
| Daemon 入口 | `cmd/keystone-daemon/` | L2 Contract、L3 Application、L5 Infrastructure |
| Worker 入口 | `cmd/keystone-worker/` | L2 `contracts/worker/`、Workspace、Runtime |
| 领域对象 | `internal/work/domain/` | Ticket 04 Project Bootstrap Domain |
| 用例编排 | `internal/work/` | Ticket 04 Project Bootstrap Application |
| 持久化或外部适配 | `internal/infrastructure/` | L5 Infrastructure、`migrations/` |
| Ticket 02 实现 | `docs/FE20260903080401/tickets/02-local-state-and-boundary-contracts/` | spec、子 Ticket、localstate/migration/Contract 实现与验收记录 |
| 运行术语与长期决策 | `CONTEXT.md`、`docs/adr/` | 术语消歧与已接受的本机 Daemon 控制边界 |

规则、修改前阅读顺序和验证要求见 `AGENTS.md`；本表只提供定位关系。

## Evidence

- 当前树与路径：`find . -path './.git' -prune -o -print`、`rg --files -uu`。
- 模块声明：`go.mod`。
- 当前规约来源：`AGENTS.md`。
- 运行术语与已接受决策：`CONTEXT.md`、`docs/adr/0001-local-daemon-control-plane.md`。
- Ticket 02 规格：`docs/FE20260903080401/tickets/02-local-state-and-boundary-contracts/spec/02-local-state-and-boundary-contracts-spec.md` 及其父 Ticket/子 Ticket。
- Graphify 输出、CodeMap 输出和 MCP 代码地图：当前未发现。
- 当前已有 Ticket 03 基础 package，以及 Ticket 04 的 `internal/work`、`internal/work/domain`、`internal/infrastructure/manifest`、`repository`、`workstore` 和 Project HTTP/CLI 链路源码。

## Freshness

- 生成日期：`2026-09-04`。
- Graphify 输出、CodeMap 输出和 MCP 代码地图：当前未发现。
- `CONTEXT.md` 与 `docs/adr/0001-local-daemon-control-plane.md` 已记录 Ticket 03 对齐结论；它们不改变 Daemon、HTTP API 或 CLI 尚未实现的事实。
- `Makefile` 提供根级验证入口；`dashboard/`、`contracts/{controlplane,worker}/`、Daemon、Project HTTP/CLI 链路与 `internal/infrastructure/{config,logging,id,localstate,migration,manifest,repository,workstore}/` 已落地。`migrations/`、`scripts/` 和 Worker runtime 尚未创建。
- 新增实现、创建目标目录或刷新架构 / 代码地图后，本索引需要重新对齐。

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
| `docs/FE20260903080401/` | V1 基线、里程碑、验收清单和版本化 Ticket/规格文档 | 已存在；Ticket 02 的 spec 与验收记录、Ticket 04 的规格和本地子 Ticket 已在当前树；Ticket 04 仍受 Ticket 03 阻塞且未实现 |
| `CONTEXT.md`、`docs/adr/` | 项目术语与已接受的架构决策 | 已存在；记录 LocalStateRoot、DaemonReadiness 等语义及本机 Daemon 控制边界，不表示 M1 已实现 |
| `cmd/`、`configs/` | `cmd/keystone`、`cmd/keystone-daemon`、`cmd/keystone-worker` 与 `configs` 的 `.gitkeep` | 预留入口，无运行实现 |
| `internal/infrastructure/` | `config`、`logging`、`id`、`localstate`、`migration` 五个 Go package，各自含源码、测试和局部文档 | 已有基础能力；`localstate` 与 `migration` 落地 Ticket 02 的本机状态和 SQLite 元数据基线 |
| `contracts/controlplane/` | `/v1` 版本前缀、错误 envelope、健康 DTO、`Idempotency-Key` 表达 | 已落地独立 JSON Contract package，无 HTTP Handler |
| `contracts/worker/` | `Register`、`Heartbeat`、`Assignment`、`Report` 传输 DTO | 已落地独立 JSON Contract package，无 Worker runtime |
| `docs/architecture-baseline/` | 架构参考目录 | 当前工作树不存在；目标架构文字不作为运行行为证据 |
| `dashboard/` | React、TypeScript、Vite 源码、`package.json` 与 `package-lock.json` | 已有可构建骨架，无业务页面 |
| `migrations/`、`scripts/` | 根级路径尚不存在 | Ticket 02 的 Migration runner 位于 `internal/infrastructure/migration/`；未创建根级迁移资产或脚本 |

当前工作树已有基础 `.go`、Ticket 02 基础设施与 Contract 测试以及 Dashboard 前端源码，但没有 Daemon、Worker、HTTP API、业务数据库 Schema 或 Worker runtime。`.agents/`、`.codex/` 和 `.idea/` 属于工作区或 IDE 工具目录，不纳入项目架构导航。

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
| `cmd/` | CLI、Daemon、Worker 入口边界 | 仅含预留子目录和 `.gitkeep`，没有已实现入口 |
| `dashboard/` | Local Web UI Client | 已有 React/TypeScript/Vite 骨架与锁文件 |

### L2 — Contract

| 路径 | 连接对象 | 当前状态 |
| --- | --- | --- |
| `contracts/controlplane/` | CLI / Dashboard ↔ Daemon 的 Control Plane API | 已落地 `/v1` DTO；HTTP 路由和 Daemon 尚未实现 |
| `contracts/worker/` | Daemon ↔ Worker 的 Narrow Worker Protocol | 已落地四组 DTO；Worker 进程和协议处理尚未实现 |

### L3 — Application

Application 位于 Control Plane Daemon 内部，当前没有源码。架构参考中的协调节点包括：

- Lifecycle Coordinator：推进 Change 的宏观 Lifecycle Stage。
- Scheduler：从 Effective Ticket Graph 的 READY Frontier 选择 Runnable Work。
- Governance 协调：在敏感副作用前取得 Policy、Gate、Evidence 或 Human Decision。
- Recovery 协调：统一处理语义恢复和 Human Escalation。

详细的 Application 职责和依赖规则见 `AGENTS.md` 的 Application Rules 与 Dependency Rules。

### L4 — Domain

业务领域以 DDD Lite 组织，`domain/` 是业务强边界。目标领域目录如下，当前均尚未创建：

| 目标路径 | 逻辑子系统 | 地图中的核心对象 |
| --- | --- | --- |
| `internal/work/` | Work & Lifecycle | Project、Change、Ticket、Lifecycle |
| `internal/planning/` | Intelligence & Planning | Understanding、Context、Design、Plan、Ticket Generation |
| `internal/governance/` | Governance | Policy、Risk、Gate、Evidence、Decision、Escalation |
| `internal/execution/` | Orchestration & Execution | Frontier、Scheduler、Execution DAG、Assignment、Recovery Coordination |
| `internal/traceability/` | Traceability & Learning | Artifact Lineage、Domain Event、Execution Trace、Eval、Incident |

五个逻辑子系统是横向职责边界，不拆成五个微服务；Lifecycle 是纵向主流程：

```text
Intent → Understand → Design → Plan → Ticketize → Execute → Verify → Integrate → Learn
```

### L5 — Infrastructure

| 目标路径 | 地图位置 | 当前状态 |
| --- | --- | --- |
| `internal/infrastructure/` | `config`、`logging`、`id`、`localstate`、`migration` 基础 package | 已有窄职责基础能力；`localstate` 管理本机状态边界，`migration` 管理 SQLite 元数据 Migration，尚无业务 Repository |
| `internal/infrastructure/localstate/` | 数据根、目录初始化、跨平台单实例锁和运行元数据 | 已落地；不拥有 Daemon 生命周期或业务状态 |
| `internal/infrastructure/migration/` | 纯 Go SQLite `t_schema_migrations` runner | 已落地；只增量、事务应用、重复跳过和漂移失败 |
| `migrations/` | 根级数据库 Schema / Migration 文件 | 当前未创建；业务 Schema 不属于 Ticket 02 |
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
| 领域对象 | `internal/<area>/domain/` | L4 Domain Map |
| 用例编排 | 对应 `internal/<area>/` Application 区域 | L3 Application |
| 持久化或外部适配 | `internal/infrastructure/` | L5 Infrastructure、`migrations/` |
| Ticket 02 实现 | `docs/FE20260903080401/tickets/02-local-state-and-boundary-contracts/` | spec、子 Ticket、localstate/migration/Contract 实现与验收记录 |
| Ticket 04 规划 | `docs/FE20260903080401/tickets/04-repository-init-and-project/` | spec、四张本地子 Ticket；规划已对齐，仍受 Ticket 03 阻塞且未实现 |
| 运行术语与长期决策 | `CONTEXT.md`、`docs/adr/` | 术语消歧与已接受的本机 Daemon 控制边界 |

规则、修改前阅读顺序和验证要求见 `AGENTS.md`；本表只提供定位关系。

## Evidence

- 当前树与路径：`find . -path './.git' -prune -o -print`、`rg --files -uu`。
- 模块声明：`go.mod`。
- 当前规约来源：`AGENTS.md`。
- 运行术语与已接受决策：`CONTEXT.md`、`docs/adr/0001-local-daemon-control-plane.md`。
- Ticket 02 规格：`docs/FE20260903080401/tickets/02-local-state-and-boundary-contracts/spec/02-local-state-and-boundary-contracts-spec.md` 及其父 Ticket/子 Ticket。
- Ticket 04 规格：`docs/FE20260903080401/tickets/04-repository-init-and-project/spec/04-repository-init-and-project-spec.md` 及其四张本地子 Ticket；它们是规划工件，不是实现证据。
- Graphify 输出、CodeMap 输出和 MCP 代码地图：当前未发现。
- 当前已有七个可编译、可测试的 Go package：`contracts/controlplane`、`contracts/worker`、`internal/infrastructure/config`、`internal/infrastructure/logging`、`internal/infrastructure/id`、`internal/infrastructure/localstate` 和 `internal/infrastructure/migration`。

## Freshness

- 生成日期：`2026-09-05`。
- Graphify 输出、CodeMap 输出和 MCP 代码地图：当前未发现。
- `CONTEXT.md` 与 `docs/adr/0001-local-daemon-control-plane.md` 至 `0004-project-init-cross-resource-recovery.md` 已记录 Ticket 03 / Ticket 04 的对齐结论；它们不改变 Daemon、HTTP API、CLI、Project Schema 或 Manifest 尚未实现的事实。
- `Makefile` 提供根级验证入口；`dashboard/`、`contracts/{controlplane,worker}/` 与 `internal/infrastructure/{config,logging,id,localstate,migration}/` 已落地。`migrations/`、`scripts/`、Daemon、Worker runtime 和 HTTP API 尚未创建。
- 新增实现、创建目标目录或刷新架构 / 代码地图后，本索引需要重新对齐。

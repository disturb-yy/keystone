# Keystone 项目索引

## Purpose

本文件是 Keystone 的项目地图，不是项目规约。

- `AGENTS.md` 定义修改前置条件、分层职责、依赖方向、命名、测试和变更纪律。
- `INDEX.md` 记录当前路径、目标架构位置、上下游关系、导航入口和事实状态。
- `docs/architecture-baseline/` 记录设计输入；当前状态栏只描述 checkout 中已经存在的内容。

## 当前仓库状态

| 区域 | 当前内容 | 状态 |
| --- | --- | --- |
| `go.mod` | 模块 `github.com/disturb-yy/keystone`，Go `1.27` | 已声明，已有三个可编译、可测试的基础 Go package |
| `Makefile` | `test`、`build`、`lint`、`dashboard-build` 根级验证入口 | 已存在；Dashboard 目标使用 `package-lock.json` 执行 npm 校验/构建 |
| `docs/FE20260903080401/` | V1 基线、里程碑、验收清单和版本化 Ticket/规格文档 | 已存在，属于项目局部文档 |
| `cmd/`、`configs/` | `cmd/keystone`、`cmd/keystone-daemon`、`cmd/keystone-worker` 与 `configs` 的 `.gitkeep` | 预留入口，无运行实现 |
| `internal/infrastructure/` | `config`、`logging`、`id` 三个 Go package，各自含源码、测试和局部文档；`id` 使用标准库 UUIDv7 | 已有基础能力，无业务或持久化实现 |
| `docs/architecture-baseline/` | 12 个 Markdown 和 1 个 JSON 架构参考文件 | 静态设计输入 |
| `dashboard/` | React、TypeScript、Vite 源码、`package.json` 与 `package-lock.json` | 已有可构建骨架，无业务页面 |
| `contracts/`、`migrations/`、`scripts/` | 路径尚不存在 | 目标边界尚未落地 |

当前工作树已有基础 `.go`、Go 测试和 Dashboard 前端源码，但没有 Daemon、Worker、API、数据库 Schema 或迁移实现。`.agents/`、`.codex/` 和 `.idea/` 属于工作区或 IDE 工具目录，不纳入项目架构导航。

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
| `contracts/controlplane/` | CLI / Dashboard ↔ Daemon 的 Control Plane API | 尚未创建 |
| `contracts/worker/` | Daemon ↔ Worker 的 Narrow Worker Protocol | 尚未创建 |

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
| `internal/infrastructure/` | 当前为 config、logging、id 三个标准库基础 package；未来可扩展 Adapter | 已有窄职责基础能力，尚无 Repository/DB 实现 |
| `migrations/` | 数据库 Schema / Migration 文件 | 尚未创建 |
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
| 总体架构 | `docs/architecture-baseline/README.md` | `00` 至 `10` 文档和 `architecture-summary.json` |

规则、修改前阅读顺序和验证要求见 `AGENTS.md`；本表只提供定位关系。

## Evidence

- 当前树与路径：`find . -path './.git' -prune -o -print`、`rg --files -uu`。
- 模块声明：`go.mod`。
- 当前规约来源：`AGENTS.md`。
- 架构参考：`docs/architecture-baseline/README.md`、`00-architecture-overview.md`、`03-domain-model.md`、`04-subsystems.md`、`05-runtime-topology.md`、`06-governance-and-execution.md`、`08-v1-scope.md`、`09-decision-log.md`、`architecture-summary.json`。
- Graphify 输出、CodeMap 输出和 MCP 代码地图：当前未发现。
- 当前已有三个可编译、可测试的基础 Go package：`internal/infrastructure/config`、`internal/infrastructure/logging` 和 `internal/infrastructure/id`。

## Freshness

- 生成日期：`2026-09-03`。
- Graphify 输出、CodeMap 输出和 MCP 代码地图：当前未发现。
- `Makefile` 已创建根级验证入口；`dashboard/` 与 `internal/infrastructure/{config,logging,id}/` 已落地，`contracts/`、`migrations/`、`scripts/` 尚未创建，具体实现边界仍待源码落地后确认。
- 新增实现、创建目标目录或刷新架构 / 代码地图后，本索引需要重新对齐。

# Keystone 项目索引

## Purpose

本文件是 Keystone 的事实性项目地图，不是项目规约。

- `AGENTS.md` 定义修改前置条件、分层职责、依赖方向、命名、测试和变更纪律。
- `INDEX.md` 记录当前 checkout 中的路径、入口、上下游关系和实现状态。
- `CONTEXT.md` 定义稳定运行术语；`docs/adr/` 记录已接受的架构决策，但二者都不替代源码证据。
- `docs/FE20260903080401/` 保存版本化 Ticket 与实施规格；spec 是规范输入，不等同于已落地运行能力。

## 当前仓库状态

| 区域 | 当前内容 | 状态 |
| --- | --- | --- |
| `go.mod` | 模块 `github.com/disturb-yy/keystone`，Go `1.27`，包含 Cobra、`golang.org/x/sys` 和纯 Go `modernc.org/sqlite` | 已声明；支持当前 CLI、跨平台锁和 SQLite readiness 实现 |
| `Makefile` | `test`、`build`、`lint`、`dashboard-build` 根级入口 | 已存在；Dashboard 目标从 `dashboard/package-lock.json` 安装依赖 |
| `cmd/keystone/` | `keystone daemon start|status|stop` CLI | 已实现；通过 Control Plane HTTP/JSON 查询，不直接访问 SQLite |
| `cmd/keystone-daemon/` | `--data-dir`、进程信号和 Daemon 生命周期入口 | 已实现；具体运行行为位于 `internal/daemon/` |
| `internal/daemon/` | loopback HTTP、InstanceLock、SQLite、Migration、RuntimeMetadata 和 readiness | 已实现；只建立 `t_schema_migrations`，不创建业务表 |
| `internal/infrastructure/` | `config`、`logging`、`id`、`localstate`、`migration` 五个基础 package | 已实现；尚无业务 Repository |
| `contracts/controlplane/` | `/v1`、健康、错误、status、stop 和幂等键 DTO | 已实现独立 JSON Contract；不拥有 HTTP Handler 或状态 |
| `contracts/worker/` | `Register`、`Heartbeat`、`Assignment`、`Report` DTO | 仅有传输契约；Worker 进程和 runtime 未实现 |
| `dashboard/` | React、TypeScript、Vite 骨架和锁文件 | 已有可构建骨架；没有业务 API/页面实现 |
| `internal/work/`、`internal/planning/`、`internal/governance/`、`internal/execution/`、`internal/traceability/` | 目标业务领域边界 | 当前未创建；Project、Change、Ticket 等业务行为未实现 |
| `cmd/keystone-worker/`、`migrations/`、`scripts/`、`configs/` | 目标入口或根级资源路径 | 当前不存在；不把目标路径当作运行实现 |
| `docs/FE20260903080401/tickets/03-cli-daemon-sqlite-readiness/spec/` | Ticket 03 实施规格 | 已存在的规范输入；不作为实现证据，也未在本次同步中修改 |

当前已落地的真实运行链是 CLI、独立 Daemon、loopback HTTP 和 SQLite 元数据
readiness。Worker、Project、Change、Ticket、业务 Domain/Application、业务
SQLite Schema/Repository/Migration 仍未实现。

## 已实现运行边界

### CLI 与 Daemon

```text
keystone daemon start|status|stop
        │
        ├── contracts/controlplane（JSON DTO）
        │        │
        │        └── loopback HTTP
        │                    │
        │             internal/daemon
        │                    │
        │        localstate + migration + state/keystone.db
        │
        └── start 只在需要时启动同目录或 PATH 中的 keystone-daemon
```

- `cmd/keystone` 在 `daemon` 父命令边界解析一次 `--data-dir`，使用归一化的
  `LocalStateRoot`；`status` 和 `stop` 不启动子进程。
- `start` 可复用已 ready 的实例；否则启动独立 `keystone-daemon`，在有界等待内
  通过 health 和 status 确认同一 `DaemonInstanceID` 已 ready。
- `cmd/keystone-daemon` 只解析 `--data-dir`、绑定进程信号并调用
  `internal/daemon.Run`；不直接写 SQL 或实现 Handler。
- `internal/daemon` 依次初始化 local state、获取 `runtime/keystone.lock`、监听
  `127.0.0.1:0`、打开 `state/keystone.db`、应用 Migration、查询版本、发布
  `runtime/instance.json`，然后才进入 ready。
- `runtime/keystone.lock` 是实例排他性的锁权威；`runtime/instance.json` 只供
  endpoint、PID、实例 ID 和启动时间发现/诊断，不替代锁。

### 当前 HTTP 路由

| 方法与路径 | 成功/失败边界 | 当前实现位置 |
| --- | --- | --- |
| `GET /healthz` | Booting 或数据库不可用时 `503 {"ready":false}`；ready 时 `200 {"ready":true}` | `internal/daemon/server.go` |
| `GET /v1/daemon/status` | ready 时返回 `DatabasePath`、`SchemaMigrationVersion`、`DaemonReadiness`、`DaemonInstanceID`；不可用时 `503 ErrorEnvelope` | `internal/daemon/server.go` + `contracts/controlplane` |
| `POST /v1/daemon/stop` | 实例 ID 缺失时 `400`，不匹配时 `409`，匹配时 `200` 接受优雅停止；不按 PID 强杀 | `internal/daemon/server.go` + `contracts/controlplane` |

所有当前 Daemon HTTP 路由只绑定 IPv4 loopback。`contracts/controlplane` 只定义
传输字段；Handler、状态判定、SQLite 连接和生命周期由 `internal/daemon` 拥有。

### SQLite readiness 边界

Daemon 使用 `internal/infrastructure/migration` 的纯 Go SQLite driver 打开
LocalStateRoot 下的 `state/keystone.db`。当前默认 Migration 只创建并记录
`t_schema_migrations`；`SchemaMigrationVersion` 是已提交的最高 Migration 版本，
不是业务版本。当前 checkout 没有根级 `migrations/`，也没有业务表、业务 Repository
或业务持久化行为。

## Architecture Map

### L0 — 当前可验证调用链

```text
keystone CLI
    ↓ HTTP/JSON（contracts/controlplane）
internal/daemon HTTP Server
    ├── localstate：LocalStateRoot、目录、锁、RuntimeMetadata
    └── migration：t_schema_migrations → state/keystone.db

keystone-daemon ──→ internal/daemon.Run
```

这是当前源码和测试能证明的链路。Dashboard 当前只是前端骨架；Worker Protocol
和下游 Worker 仍是未实现的目标边界。

### L1 — Interface / Entry

| 路径 | 地图位置 | 当前状态 |
| --- | --- | --- |
| `cmd/keystone/` | 本机 CLI | 已实现 `daemon start|status|stop` |
| `cmd/keystone-daemon/` | 独立 Daemon 进程入口 | 已实现参数/信号适配 |
| `dashboard/` | Local Web UI Client | 仅有 React/TypeScript/Vite 骨架，无业务 API 调用 |
| `cmd/keystone-worker/` | Worker 进程入口 | 当前未创建 |

### L2 — Contract

| 路径 | 连接对象 | 当前状态 |
| --- | --- | --- |
| `contracts/controlplane/` | CLI / 后续 Dashboard ↔ Daemon | 已落地 `/v1`、Health、Error、Daemon status/stop DTO；Handler 在 `internal/daemon` |
| `contracts/worker/` | Daemon ↔ Worker | 已落地四组 DTO；没有 Worker 进程、HTTP 协议处理或状态推进 |

### L3 — Daemon / Application seam

`internal/daemon/` 是当前已落地的运行组合层，负责编排本机状态、HTTP、SQLite
和 readiness。当前没有 `internal/<area>/` 业务 Application 或 Domain package，
因此没有 Project、Change、Ticket、Lifecycle、Gate 或业务 Query/Command 实现。

### L4 — Domain

以下是目标业务边界，当前均未创建：

| 目标路径 | 逻辑子系统 | 当前状态 |
| --- | --- | --- |
| `internal/work/` | Work & Lifecycle | 未实现；无 Project、Change、Ticket |
| `internal/planning/` | Intelligence & Planning | 未实现 |
| `internal/governance/` | Governance | 未实现 |
| `internal/execution/` | Orchestration & Execution | 未实现；无 Scheduler、Execution DAG 或 Worker 协调 |
| `internal/traceability/` | Traceability & Learning | 未实现；无 Artifact/Event/Trace |

这些目录来自目标架构和 Ticket 规划，不构成当前可执行调用链。

### L5 — Infrastructure

| 路径 | 地图位置 | 当前状态 |
| --- | --- | --- |
| `internal/infrastructure/config/` | 日志等级配置解析 | 已实现 |
| `internal/infrastructure/logging/` | JSON 结构化日志 | 已实现 |
| `internal/infrastructure/id/` | UUIDv7 生成 | 已实现 |
| `internal/infrastructure/localstate/` | 数据根、目录、跨平台锁、运行元数据 | 已实现；不承载 Daemon 或业务状态 |
| `internal/infrastructure/migration/` | 纯 Go SQLite `t_schema_migrations` runner | 已实现；不创建业务 Schema |

### L6 — Worker / Side Effects

`contracts/worker/` 只有传输 DTO。`cmd/keystone-worker/`、Worker runtime、
Workspace 执行、Codex/OpenCode 调用和副作用报告当前均未实现。

## Relationship Map

当前已实现关系：

```text
cmd/keystone → contracts/controlplane → internal/daemon
cmd/keystone-daemon → internal/daemon
internal/daemon → localstate + migration → SQLite metadata
```

目标但当前未落地的下游关系：

```text
Dashboard → contracts/controlplane → Daemon
Daemon → contracts/worker → Worker → Runtime / Tools
```

依赖方向和跨边界约束以 `AGENTS.md` 为唯一规约来源；本图只报告当前代码和
明确的未实现目标边界。

## Current Truth Map

| Owner | 当前可验证事实 |
| --- | --- |
| `cmd/keystone` | 解析 LocalStateRoot，发现 metadata endpoint，经 HTTP 查询 health/status/stop；不直接访问 SQLite |
| `cmd/keystone-daemon` | 解析参数和信号，调用 Daemon 运行层 |
| `internal/daemon` | 拥有本次 Daemon 的 InstanceLock、HTTP Server、SQLite 连接、Migration、readiness 和关闭顺序 |
| `contracts/controlplane` | 只拥有边界 DTO，不拥有 Daemon 状态、HTTP Handler 或数据库 |
| `contracts/worker` | 只拥有 Worker Protocol DTO；没有 Worker runtime |
| 业务领域与业务 Schema | 当前不存在；Project、Change、Ticket 和业务表未实现 |

## Navigation

| 任务 | 地图入口 | 相关证据/边界 |
| --- | --- | --- |
| CLI 命令与生命周期 | `cmd/keystone/command.go`、`process.go`、`readiness.go` | `cmd/keystone/command_test.go`、`contracts/controlplane/` |
| Daemon 进程入口 | `cmd/keystone-daemon/main.go` | `internal/daemon/server.go`、`server_test.go` |
| HTTP 路由与 readiness | `internal/daemon/server.go` | `contracts/controlplane/contract.go`、Daemon 测试 |
| 本机状态和实例边界 | `internal/infrastructure/localstate/` | `paths.go`、`lock*.go`、`metadata*.go` |
| SQLite Migration readiness | `internal/infrastructure/migration/runner.go` | `runner_test.go`；当前只有 `t_schema_migrations` |
| Control Plane DTO | `contracts/controlplane/` | `contract.go`、`contract_test.go` |
| Worker 边界 | `contracts/worker/` | 仅 DTO；Worker 入口和 runtime 未创建 |
| Project / Change / Ticket | `internal/work/` | 目标路径，当前未创建 |
| Ticket 03 规范输入 | `docs/FE20260903080401/tickets/03-cli-daemon-sqlite-readiness/` | spec 仅描述契约，不替代当前源码事实 |
| 术语与架构决策 | `CONTEXT.md`、`docs/adr/0001-local-daemon-control-plane.md` | 运行语义和难逆边界 |

## Evidence

- 当前文件树与路径：`rg --files -uu -g '!.git/**'`、目录存在性检查。
- CLI：`cmd/keystone/*.go`，尤其是 `command.go`、`http_client.go`、`process.go`、
  `readiness.go`、`metadata.go` 与 `command_test.go`。
- Daemon：`cmd/keystone-daemon/main.go`、`internal/daemon/server.go`、
  `internal/daemon/server_test.go`。
- Control Plane 边界：`contracts/controlplane/contract.go`、`contract_test.go`。
- Local state 与 SQLite：`internal/infrastructure/localstate/`、
  `internal/infrastructure/migration/runner.go` 及各自局部 `AGENTS.md`/`INDEX.md`。
- 规则、术语和规范输入：根 `AGENTS.md`、`CONTEXT.md`、ADR 0001、Ticket 03 父文档和 spec。
- Graphify 输出、CodeMap 输出和 MCP 代码地图：当前未发现；本次没有据此推断架构或调用链。

## Freshness

- 同步日期：`2026-09-05`。
- 本索引在 `cmd/keystone/`、`cmd/keystone-daemon/`、`internal/daemon/`、
  `contracts/controlplane/`、localstate/migration 路径、HTTP 路由、Migration 默认表或
  业务领域目录变化后需要重新核对。
- 若新增 `graphify-out/`、`.codemap/` 或 MCP 代码地图，应补充相应证据并重新检查
  当前调用链与风险边界。

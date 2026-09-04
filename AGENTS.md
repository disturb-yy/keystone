# Keystone 工程规约

## Purpose

本文件定义整个项目的 AI 编程规约。

所有 Agent 在修改任何代码之前，必须先阅读：

1. 本文件。
2. 根目录 `INDEX.md`。
3. 目标 package 最近一级的 `AGENTS.md`（存在时）。
4. 目标 package 的 `INDEX.md`（存在时）。
5. 与变更直接相关的源码和测试。

不得仅凭文件名或猜测修改代码。

## 当前仓库事实

- `go.mod` 声明模块 `github.com/disturb-yy/keystone`，Go 版本为 `1.27`。
- `Makefile` 已提供根级 `test`、`build`、`lint` 和 `dashboard-build` 入口；其中 Dashboard 目标预期从 `dashboard/package-lock.json` 安装依赖后运行 npm 命令。
- `docs/FE20260903080401/` 保存 V1 实施基线、里程碑、验收清单和版本化 Ticket/规格文档。
- `docs/FE20260903080401/` 下的 Ticket 02 目录包含本地状态、Migration 和边界 Contract 的实施规格与验收记录；`docs/architecture-baseline/` 不在本工作树中，若由其他 checkout 提供也只属于静态设计输入。
- `internal/infrastructure/config`、`internal/infrastructure/logging` 和 `internal/infrastructure/id` 已是三个标准库 Go package，各自有源码、聚焦测试以及局部 `AGENTS.md`/`INDEX.md`；它们只提供配置解析、结构化日志和 UUIDv7 生成，不实现业务或持久化行为。
- `internal/infrastructure/localstate` 提供数据根路径、目录初始化、跨平台单实例锁和诊断元数据；`internal/infrastructure/migration` 提供基于纯 Go SQLite driver 的 `t_schema_migrations` runner。两者均不实现 Daemon 或业务 Schema。
- `contracts/controlplane` 与 `contracts/worker` 已提供 `/v1` Control Plane 和 Worker 的最小 JSON 边界 DTO，各自有局部 `AGENTS.md`/`INDEX.md`；它们不引用 Domain、不访问 SQLite。
- `dashboard/` 已有 React、TypeScript、Vite 工程、`package-lock.json` 和构建/检查脚本；`cmd/keystone`、`cmd/keystone-daemon`、`cmd/keystone-worker` 与 `configs` 保留当前的 `.gitkeep` 预留事实，`migrations/` 和 `scripts/` 当前尚未创建。
- 当前工作树已有 Go 基础源码、Ticket 02 基础设施与边界 Contract 测试、Dashboard 前端源码及构建产物，但没有 Daemon、Worker、HTTP API、业务数据库 Schema 或运行时执行实现。根 Make 入口验证的是当前已落地的 Go package 与 Dashboard 工程；目标架构和静态设计输入不是服务行为证据。

## Architecture

### 已确认的目标架构

Keystone 采用 Go + DDD Lite，物理形态是 Local-first 的 Modular Monolith Control Plane 加独立 Worker，使用 Monorepo 组织 CLI、Dashboard、Daemon、Worker、Core、Ports 和 Adapters。

运行时边界：

```text
Human
├── Keystone CLI
└── Keystone Dashboard
          │
          ↓
   Control Plane API Contract
          │
          ↓
   Control Plane Daemon
          │
          ↓
      Worker Protocol
          │
          ↓
   Independent Worker
          │
          ↓
 Execution Guard → Assigned Workspace → Codex / OpenCode / Tools
```

核心生命周期为：

```text
Intent → Understand → Design → Plan → Ticketize → Execute → Verify → Integrate → Learn
```

核心工作模型为：

```text
Project → Change → Ticket Dependency Graph → Ticket → Execution DAG → Agent Run
```

五个稳定的逻辑子系统是：

- `Work & Lifecycle`：Project、Change、Ticket 和生命周期状态。
- `Intelligence & Planning`：理解、上下文、设计、计划和 Ticket 生成。
- `Governance`：Policy、Risk、Gate、Evidence、Decision、Escalation 和 Recovery Boundary。
- `Orchestration & Execution`：Frontier、Scheduler、Execution DAG、Runtime、Worker 和 Workspace 协调。
- `Traceability & Learning`：Artifact Lineage、Domain Event、Execution Trace、Eval、Incident 和改进反馈。

五个子系统是逻辑职责边界，不拆成五个微服务。V1 保持一个本机 Daemon 管理多个 Project，一个独立 Worker 执行副作用，默认使用 Git Worktree 隔离 Workspace。

### 目标状态与事实边界

以下是目标架构的状态与事实边界；当前 checkout 尚无对应服务实现。

- Control Plane Daemon 持有 Project、Change、Ticket、Gate、Decision、Evidence、Execution 和派生追踪信息的权威持久状态。
- Repository 持有源码和版本化项目知识；Git 持有代码版本事实。
- Worker 只持有执行句柄、心跳、Workspace、运行时会话和日志等短期运行信息，不拥有 Change、Ticket、Gate、依赖图或 Recovery Decision 的权威状态。
- Client 只能通过 Control Plane API Contract 访问 Daemon；Worker 只能通过 Narrow Worker Protocol 与 Daemon 交互。Client 和 Worker 都不能直接修改 Keystone DB。
- Governance 必须在敏感副作用前生效；Agent 只能在 Assigned Workspace 中执行，不能直接修改 Project 原始 Repository Workspace。

### 代码组织

业务领域 package 使用 DDD Lite：

```text
业务领域 package
└── domain/       # 业务强边界
```

目标领域边界为 `internal/work`、`internal/planning`、`internal/governance`、`internal/execution` 和 `internal/traceability`；`internal/infrastructure` 当前承载 config、logging、id、localstate 和 migration 五个窄职责基础 package。上述业务领域目录当前尚未实现，新增代码时以实际目标领域和最近一级文档为准。

复杂度较低时：

- Application 留在领域根 package。
- Interface 留在领域根 package。
- Infrastructure 留在领域根 package。
- 用文件命名和接口控制职责。

复杂度上升后才允许进一步拆包。

## Dependency Rules

允许：

```text
Interface Adapter → Application
Application → Domain
Infrastructure → Domain
cmd/keystone、cmd/keystone-daemon → Control Plane 具体实现
cmd/keystone-worker → Worker Protocol 与 Worker 具体实现
```

跨进程边界固定为：

```text
CLI / Dashboard → contracts/controlplane → Daemon
Daemon → contracts/worker → Worker
```

`contracts/controlplane` 和 `contracts/worker` 是传输边界；Contract 不直接复用 Domain Entity，应使用面向边界的请求、响应和事件模型。

禁止：

```text
Domain → Application
Domain → Infrastructure
Domain → Interface
Domain → HTTP / SQL / 数据库或第三方 SDK
Application → Infrastructure 具体实现
Interface → Repository / Provider 具体实现
领域 A → 领域 B 的具体基础设施实现
Client / Worker → Keystone DB
```

跨领域调用优先通过公开接口或 Application Service。

## Domain Rules

`internal/*/domain`：

- 只放业务概念。
- 可以包含 Entity、Value Object、Aggregate、Domain Service、Repository Interface。
- 不处理 HTTP。
- 不写 SQL。
- 不读配置。
- 不直接调用外部 API。
- 不依赖数据库实现。
- 不依赖日志框架，除非确有跨领域统一抽象且已被批准。

Domain 代码应该能在不启动数据库、HTTP Server、Redis 的情况下进行单元测试。

## Application Rules

Application 负责：

- 用例编排。
- 调用领域能力。
- 调用 Repository / Provider 抽象。
- 事务边界。
- 权限或流程级协调。

Application 不负责：

- SQL 细节。
- HTTP 参数解析。
- 数据库表结构细节。
- 把业务规则全部写成 if/else 而绕开 Domain。

## Interface Rules

Handler、CLI 和其他 Interface Adapter 负责：

- 参数解析。
- 输入校验。
- DTO 转换。
- 调用 Application。
- 协议状态码与响应输出。

不得在 Interface Adapter 中直接写 SQL、调用 Repository 实现、调用第三方 API 或编写核心业务规则。

## Infrastructure Rules

Infrastructure 负责实现：

- Repository。
- Provider。
- Cache。
- MQ。
- External API Adapter。
- Storage Adapter。

Infrastructure 可以依赖 Domain 中定义的接口和模型，但不能把基础设施细节泄漏到 Domain 或 Application 的业务模型中。

## Worker Rules

Worker 是可替换的副作用执行进程，负责：

- 接收受授权的 Worker Protocol 请求。
- 在指定 Workspace 中调用 Runtime 和 Tools。
- 返回执行结果、Evidence 和运行日志。

Worker 不负责生命周期推进、Gate 决策、权威状态持久化、Canonical Dependency Graph 或 Semantic Recovery。

## Package Rules

每个 Go package 必须有：

- `AGENTS.md`。
- `INDEX.md`。

新增 package 时，这两个文件必须同步创建。删除、重命名、迁移重要文件时，必须同步更新对应 `INDEX.md` 以及受影响的根级导航文档。

## File Naming

推荐：

```text
service.go
sync.go
handler.go
repository.go
repository_mysql.go
provider.go
provider_tushare.go
model.go
mapper.go
```

避免：

```text
utils.go
common.go
helper.go
manager.go
misc.go
```

除非职责非常明确。

## Error Handling

- 错误必须向上返回或明确处理。
- 不吞错误。
- 使用 `%w` 保留错误链。
- Domain 错误应表达业务语义。
- Infrastructure 错误可包装技术上下文，但不得泄露敏感数据。

## Context

所有可能阻塞或涉及 I/O 的 Application / Infrastructure 方法应优先接收：

```go
context.Context
```

不得将 Context 存储在 struct 中长期持有。

## Testing

优先级：

1. Domain 单元测试。
2. Application 用例测试。
3. Repository / Provider 集成测试。
4. Handler / Interface Adapter 测试。

修改业务规则时必须优先补充 Domain 测试。完成代码变更后执行 `go test ./...`；必要时执行 `go vet ./...`。

## Change Discipline

修改前：

- 根据 `INDEX.md` 定位代码。
- 检查依赖方向。
- 确认变更属于哪个领域和层级。
- 确认目标 package 的局部 `AGENTS.md` 和 `INDEX.md` 已阅读。

修改后：

- 更新测试。
- 更新受影响的 `INDEX.md`。
- 架构规则改变时更新本文件。
- 结构变化时同步更新根 `README.md` 和 `INDEX.md`。
- 执行 `go test ./...`，必要时执行 `go vet ./...`。
- 文档变更后执行 `git diff --check`，并检查描述是否有当前文件树或源码无法验证的方案性表述。

## Prohibited

禁止为了完成任务：

- 将所有逻辑塞进 `service.go`。
- 从 Domain 直接调用数据库、缓存、消息队列或外部 API。
- Handler / Interface Adapter 直接访问数据库。
- 创建无业务意义的 package。
- 未阅读局部 `AGENTS.md` 就跨 package 修改。
- 复制父级 `AGENTS.md` 全文到子 package。
- 把 `docs/architecture-baseline/` 中的设计输入描述当作已实现的服务行为、接口或 Schema。

## Coding Style

- 文档和注释使用中文编写。
- 函数添加必要的注释。
- 注释说明业务原因、约束、不变量、恢复顺序或设计取舍，不逐行复述实现。
- 单函数避免过长，复杂函数按业务职责拆分，不进行无意义的碎片化抽象。
- 导出标识符的注释说明调用方可见的语义、前置条件或副作用。
- 注释只能描述当前可验证的行为；不保留 TODO、FIXME、XXX、未实现需求或未来设计草案。
- 根因修复用正确逻辑直接覆盖，不写否定的否定，直接写肯定性的描述。

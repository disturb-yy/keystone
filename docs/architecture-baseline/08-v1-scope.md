# 08 — V1 Thin Vertical Slice

## 目标

证明 Keystone 能把一个真实 Repository 和真实代码变更请求跑完整个 Control Plane 闭环。

目标路径：

```text
Repository
    ↓
keystone init
    ↓
Project
    ↓
Change Intake
    ↓
Change
    ↓
Intent
    ↓
Understand
    ↓
Design
    ↓
Plan
    ↓
Ticketize
    ↓
Ticket Graph
    ↓
Scheduler
    ↓
Workspace
    ↓
Worker
    ↓
Runtime
    ↓
Code Artifact
    ↓
Verify
    ↓
Integrate Ready
    ↓
Trace / Dashboard
```

## V1 约束

### 保持简单

- 一个 Local Daemon
- 一个 Local Worker
- 初期一个主要 Runtime 即可
- Git Worktree Workspace
- 一个 Database
- 一套默认 Stage Strategy
- 一个默认 Ticket Generator
- 简单 Gate
- 简单 Evidence Model
- 简单 Dashboard
- 简单 Event Log
- 保守并行

### 保持架构

即使实现简单，也要守住：

- CLI / Dashboard → API → Daemon
- Daemon → Worker Protocol → Worker
- Control Plane owns truth
- Worker owns side effects
- Repository owns source and project knowledge
- Git owns code revisions
- Capability 不直接绑定 Runtime
- Workspace 隔离可变执行
- Governance 在敏感 Side Effect 前生效

## V1 明确不要求

- Microservices
- Message Broker
- Cloud Control Plane
- Multi-user Auth
- RBAC
- Remote Worker
- Worker Pool
- Complex Policy DSL
- Graph Database
- Plugin Marketplace
- Multi-runtime Intelligent Routing
- Full Event Sourcing
- Autonomous Self-modifying Policy / Skill Loop
- Complex Cross-project Orchestration
- Full Deploy / Operate / Maintain Lifecycle

## V1 Dashboard

最小有用界面：

```text
Projects
  ↓
Project
  ↓
Changes
  ↓
Change Detail
   ├── Lifecycle
   ├── Ticket Graph
   ├── Current Runs
   ├── Human-required Actions
   └── Trace / Artifacts
```

## V1 成功标准

不是：

> 每个架构概念都做得非常完整。

而是：

> 一个真实 Change 能完整穿过 Keystone，同时关键 Ownership 与 Extension Boundary 没被破坏。

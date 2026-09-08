# 03 — 高层领域模型

## 1. Project

Project 是长期存在的工程 Scope。

概念上包含：

```text
Project
├── Repository Identity
├── Project Knowledge Scope
├── Policy Scope
├── Capability / Runtime Configuration Scope
└── Changes
```

V1 可以假设：

```text
1 Project = 1 Repository
```

但领域模型不要永久绑定二者，以便未来支持多 Repository Project。

## 2. Change

Change 表示一次完整的软件变更。

它承载：

- Intent
- Understanding
- Design
- Plan
- Ticket Graph
- Decisions
- Result
- Change-level Validation

它回答：

> 用户真正想完成的完整变化是什么？

## 3. Ticket

Ticket 是从 Change Plan 中产生的调度 / 执行单元。

目标特征：

- 足够窄，适合 fresh context；
- 可独立理解；
- 可独立验证；
- 优先 vertical tracer-bullet slice；
- 明确 blocking relationship。

它回答：

> 这次 Change 中，哪一个独立有价值的切片可以被执行和验证？

## 4. Ticket Dependency Graph

Ticket Dependency 是一等对象。

当前对齐方向允许：

```text
BLOCKED_BY
REQUIRES_ARTIFACT
ORDERED_AFTER
CONFLICTS_WITH
RELATED_TO
```

`to-tickets` 风格的 blocking edge 属于 canonical planning graph。

Planner / CodeMap / Runtime Analysis 可以发现新的 dependency / conflict evidence，但不能静默改写 Canonical Graph。

概念上：

```text
Canonical Ticket Graph
        +
Validated Discovered Constraints
        +
Execution Constraints
        ↓
Effective Graph
        ↓
Scheduler
```

## 5. Execution DAG

每个 Ticket 内部可以拥有独立 Execution DAG：

```text
Ticket
  ↓
Execution DAG
  ↓
Task
  ↓
Agent Run
```

Ticket 调度与 Agent Run 执行不是同一层。

## 6. Workspace

Workspace 是 Keystone 正式管理的执行资源。

```text
Project Repository
       ↓
Workspace
       ↓
Agent Run
```

V1 默认实现：

```text
Git Worktree
```

但领域模型不绑定 Git Worktree。

未来可支持：

- Local Directory
- Container
- Sandbox
- Remote Workspace

## 7. Agent Run

Agent Run 是短生命周期执行实例。

Change / Ticket 的持久状态不放在 Agent Session 中。

## 8. Actor

Actor 是统一身份抽象：

```text
HUMAN
AGENT
SYSTEM
EXTERNAL
```

用于 Event、Decision、Artifact、Evidence、Run 的 Traceability。

V1 仍保持 Local-first / Single-user，不需要复杂 RBAC。

## 9. Artifact

Artifact 不等于文件。

可包括：

- Intent
- Spec
- Plan
- Ticket Graph
- Code Revision
- Diff
- Test Result
- Review
- Decision
- Evidence
- Context Snapshot
- Policy Snapshot
- Eval Result

Artifact 可以通过 typed lineage 形成 Artifact Graph。

## 10. Event

Keystone 区分：

```text
Current State
Domain Event Log
Raw Runtime Logs
```

当前方向：

> Append-only Domain Event + Materialized Current State

V1 不要求完整 Event Sourcing。

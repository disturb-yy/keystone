# 06 — Governance 与 Execution 边界

## 1. Lifecycle Coordinator

Lifecycle Coordinator 是 Application Layer 中一个很薄的协调者。

职责：

```text
观察当前 Change Lifecycle
决定下一 Stage
请求 Governance 判断是否允许推进
触发对应 Stage Work
```

它不负责：

- 分析代码
- 设计
- 规划
- 拆票
- 写代码
- Review
- 跑测试

## 2. Governance

Governance 回答：

```text
是否允许继续？
需要什么 Evidence？
是否需要 Human Decision？
```

当前已对齐核心概念：

- Policy
- Risk
- Gate
- Evidence
- Decision
- Escalation
- Recovery Boundary

## 3. Scheduler

Scheduler 负责 Runnable Work。

概念上：

```text
Project-level Coordination
        ↓
Effective Ticket Graph
        ↓
READY Frontier
        ↓
Scheduler
        ↓
Ticket / Task / Agent Run
```

Scheduler 不等于 Lifecycle Coordinator。

Lifecycle Coordinator：

> Change 下一阶段应该是什么？

Scheduler：

> 当前哪一项具体工作应该执行？

## 4. Project-level Coordination

一个 Project 可以有多个 Active Change。

所以 Execution Eligibility 需要考虑：

- Cross-Change Dependency
- Write Conflict
- Shared Artifact Dependency
- Project Policy
- Workspace Availability

V1 可以保守：

```text
明确安全 → 并行
无法确认 → 串行
```

## 5. Execution Guard

Execution Guard 是正式横向架构边界：

```text
Governance
   ↓
Execution Authorization
   ↓
Execution Guard
   ↓
Worker
```

Governance 决定：

> 允许做什么？

Execution Guard 保证：

> 实际最多只能做这些。

可能 Enforcement Surface：

- Workspace Scope
- Tool Access
- Source Control Actions
- Runtime Permissions
- Hook / Sandbox

V1 可以只做粗粒度 Enforcement。

## 6. Workspace Boundary

```text
Agent Run
   ↓
Assigned Workspace
```

Agent 不直接修改 Project 原始 Repository Workspace。

Workspace 用于：

- Isolation
- Parallelism
- Rollback / Disposal
- Side-effect Scope

## 7. Runtime Independence

Role、Capability、Runtime、Tool 保持相互独立。

例如：

```text
Capability: code-review
Role: Reviewer
Runtime: Codex
Tools: Git + CodeMap
```

另一次可以：

```text
Capability: code-review
Role: Reviewer
Runtime: OpenCode
```

而不改变 Core Model。

## 8. Recovery

Recovery 由 Control Plane 统一治理，而不是散落在各 Skill 内。

已对齐高层 Failure Class：

- Transient
- Execution
- Planning
- Context
- Policy / Governance

Skill 可以做 Mechanical Retry。

Semantic Recovery 属于 Control Plane。

## 9. Human Escalation

自动执行必须存在明确自治边界。

当系统无法安全继续时，进入 Human Required，并向 Human 提交最小 Decision Package。

具体 Threshold / Schema 后续单独对齐。

# 04 — 五大逻辑子系统

Keystone 使用五个稳定逻辑边界。

它们是**逻辑架构边界**，不是五个微服务。

## 1. Work & Lifecycle

拥有工作模型：

```text
Project
Change
Ticket
Ticket Dependency
Lifecycle
```

回答：

> 我们正在做什么？现在做到哪一步？

## 2. Intelligence & Planning

负责 AI Coding 的理解与规划能力。

高层职责：

```text
Project Understanding
Context Resolution
Conceptual Space
Design
Plan
Ticket Generation
Knowledge Provider Integration
Capability Knowledge
```

可能使用：

```text
AGENTS.md
INDEX.md
Architecture Docs
CodeMap
Git
Search
Conceptual Space
```

回答：

> 这件事应该怎么理解、怎么改、怎么拆？

它可以提出 Plan / Ticket / Dependency / Risk，但不能直接修改权威 Lifecycle State。

## 3. Governance

负责整个生命周期中的控制规则。

高层职责：

```text
Policy
Risk
Gate
Evidence
Human Decision
Escalation
Recovery Boundary
Permission / Authorization Intent
```

回答：

> 现在允许继续吗？需要什么证据？什么时候必须停下来找人？

## 4. Orchestration & Execution

负责调度和执行协调。

高层职责：

```text
Effective Graph
Frontier
Scheduler
Execution DAG
Agent Run
Runtime Selection
Worker Assignment
Workspace Assignment
Recovery Coordination
```

回答：

> 现在什么工作可以跑、由谁跑、在哪跑、用什么能力跑？

## 5. Traceability & Learning

负责历史解释与改进反馈。

高层职责：

```text
Artifact Lineage
Domain Events
Execution Trace
Eval
Metrics
Incident
Regression
Improvement Proposal
```

回答：

> 为什么系统最终得到这个结果？

以及：

> 下一次应该如何更可靠？

## Lifecycle 与 Subsystem 的关系

Lifecycle 是纵向主流程：

```text
Intent → Understand → Design → Plan → Ticketize → Execute → Verify → Integrate → Learn
```

五大 Subsystem 是横向职责边界。

不要按 Lifecycle 每一步拆一个 Service。

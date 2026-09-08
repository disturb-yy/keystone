# Keystone Architecture v0.1（中文）

> 状态：**总体架构方向已对齐**
>
> 范围：仅包含高层架构。具体流程语义、Schema、Gate 规则、Ticket 字段、Scheduler 算法和实现细节，刻意留到后续专题会话中继续对齐。

## Keystone 是什么？

Keystone 是一个**独立实现的 AI Coding Control Plane（AI 编程控制平面）**。

它的目标不是替代 Codex、OpenCode、Git、CodeMap、CI 或其他工程工具，而是围绕一个稳定的软件变更生命周期，对这些能力进行协调，并将工作状态、治理、执行、可追溯性和 Human-in-the-loop 纳入统一控制平面。

LoopX、Anthropic AI-native SDLC、`to-tickets`、CodeMap、Cognitive Control Plane、现有 AI Coding Workflow 等，均作为 **Reference Architecture / Design Input（参考架构 / 设计输入）**，而不是 Keystone 的运行时依赖。

## 核心生命周期

```text
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
Execute
  ↓
Verify
  ↓
Integrate
  ↓
Learn
```

当前产品边界主要止于：

```text
Integrate / Change Ready
```

Deploy / Operate / Maintain 暂时作为外部集成点。

## 核心工作层级

```text
Project
  ↓
Change
  ↓
Ticket Dependency Graph
  ↓
Ticket
  ↓
Execution DAG
  ↓
Agent Run
```

## V1 产品形态

```text
Human
 ├─ Keystone CLI
 └─ Keystone Dashboard
          │
          ↓
   Control Plane API
          ↓
 Keystone Control Plane Daemon
          │
          ↓
     Worker Protocol
          ↓
    Keystone Worker
          │
          ↓
 Workspace / Runtime / Tools
          │
          ├─ Codex
          └─ OpenCode
```

## V1 实现策略

- Local-first
- 一个本机 Daemon 管理多个 Project
- Modular Monolith
- Monorepo
- Worker 独立进程
- Local Web Dashboard
- Thin Vertical Slice
- Stable Core + Explicit Extension Ports
- 第一版 Workspace 默认用 Git Worktree
- Git 是代码事实来源
- Repository / Git 是版本化项目知识的事实来源
- Keystone DB 是 Work / Decision / Execution / Derived Knowledge 的事实来源

## 压缩包内容

- `00-architecture-overview.md` —— 总体架构图
- `01-architecture-principles.md` —— 架构原则
- `02-lifecycle.md` —— 生命周期主干
- `03-domain-model.md` —— 顶层领域模型
- `04-subsystems.md` —— 五大逻辑子系统
- `05-runtime-topology.md` —— CLI / Dashboard / Daemon / Worker / Workspace
- `06-governance-and-execution.md` —— 治理、生命周期协调、调度、执行边界
- `07-extensibility.md` —— Capability 与扩展策略
- `08-v1-scope.md` —— V1 Thin Vertical Slice 范围
- `09-decision-log.md` —— 已对齐的大框架决策
- `10-deferred-topics.md` —— 后续单独对齐的细节专题
- `architecture-summary.json` —— 机器可读架构摘要

## 总体设计原则

> **先把方向和边界做对，V1 可以粗糙简单。可扩展性来自稳定边界与明确扩展口，而不是第一版就做复杂插件平台。**

# 09 — 高层决策记录

本文件压缩记录当前会话中已经确认的总体架构决策。

具体 Schema、算法和实现细节刻意省略。

## D01 — 产品目标

构建一个跨 Project、跨 Runtime 的通用 AI Coding Control Plane，并为未来扩展到更完整 AI-native SDLC 留边界。

## D02 — 持久工作对象

Change 是长期存在的 Lifecycle Subject。

Agent / Session / Runtime 是可替换执行机制。

## D03 — 状态权威

Authoritative State 由 Control Plane 持有。

Agent 只能提交 Result / Transition Request。

## D04 — Gate

Gate 是一等治理概念，与 Workflow / Skill / Agent 分离。

## D05 — Evidence

Evidence 是一等对象。

重要 Transition 不能只依赖 Agent Self-report。

## D06 — Workflow 模型

采用固定 Lifecycle + Stage 内 Dynamic Execution Plan。

## D07 — Plan Gate

Planner Output 不自动执行。

是否需要 Human Approval 由 Risk / Policy 决定。

## D08 — Risk / Policy

Static Policy + Semantic Review。

Hard Policy 不能被 Agent 静默降低。

## D09 — Policy Source

Governance Policy 概念上集中、版本化，而不是散落在 Prompt / AGENTS.md 中。

## D10 — Conceptual Space

Conceptual Space 是版本化 Context Contract，不是 Enforcement Mechanism。

## D11 — Context

采用 Context Resolver + Progressive Disclosure。

Agent Run 获得 Role/Stage-specific Minimum Sufficient Context。

## D12 — Skill

Skill 逐步升级为 Versioned Executable Capability Contract。

## D13 — Agent

采用 Stable Agent Role Template + Ephemeral Agent Run。

Durable State 不放 Agent Session。

## D14 — Runtime

Role 不绑定 Runtime。

Runtime 是可替换 Execution Infrastructure。

## D15 — LoopX 关系（修订）

LoopX 仅作为 Reference Architecture。

Keystone 不依赖 LoopX 运行。

## D16 — Recovery

Recovery 由 Control Plane 统一治理。

Skill 只允许 Mechanical Retry。

## D17 — Escalation

自治受到 Recovery Budget、Hard Stop、Risk、Conflict 和 Human Escalation 约束。

## D18 — Human Decision

Human Decision 是版本化、可审计的 Decision Artifact，并绑定具体被批准对象。

## D19 — Artifact Lineage

Artifact 是一等对象，可形成 Typed Artifact Graph。

V1 不需要 Graph Database。

## D20 — Event

采用 Append-only Domain Event + Materialized Current State。

Raw Runtime Logs 单独保存。

## D21 — Parallel Run

允许并行 Analysis / Review。

无法自动消解的冲突必须显式 Resolution。

不允许多个 Agent 同时修改同一 Workspace。

## D22 — Ticket Dependency

Ticket Dependency 是一等关系，并直接约束 Scheduler。

## D23 — Change vs Ticket

Change 表示完整用户 / 产品变化。

Ticket 是从 Change Plan 拆出的执行切片。

## D24 — Canonical vs Discovered Dependency

Human-approved / Planned Dependency Graph 是 Canonical。

Agent / Tool 只能提出 Dependency Proposal / Evidence，不能静默改写权威图。

## D25 — Ticket Lifecycle

Ticket 有独立执行生命周期。

Change 保持更高层聚合生命周期。

## D26 — Independent Implementation

Keystone 独立实现。

LoopX、Anthropic SDLC、`to-tickets`、CodeMap 等只做参考输入。

## D27 — Lifecycle

```text
Intent → Understand → Design → Plan → Ticketize → Execute → Verify → Integrate → Learn
```

## D28 — 五大顶层子系统

```text
Work & Lifecycle
Intelligence & Planning
Governance
Orchestration & Execution
Traceability & Learning
```

## D29 — 扩展策略

Stable Domain Core + Explicit Extension Ports。

V1 不做复杂插件平台。

## D30 — 物理架构

Control Plane 使用 Modular Monolith。

Worker 作为 Side-effect Execution Process 独立。

## D31 — Project Bootstrap

Repository 通过：

```text
keystone init
```

进入 Keystone。

## D32 — Local-first

V1 采用 Local Control Plane Daemon + CLI + Worker。

## D33 — Dashboard

Dashboard 是一等 Human Interaction Surface。

V1 为 Local Web UI。

## D34 — Multi-project Daemon

一个 Local Daemon 管多个 Project。

## D35 — Change Intake

CLI、Dashboard、未来 GitHub/Jira/API 等都是 Change Source Adapter。

## D36 — Workspace

Workspace 是正式执行资源。

V1 用 Git Worktree，但抽象不绑定 Git Worktree。

## D37 — Git Boundary

Git 是代码事实来源。

Keystone 管 Intent、Verification 和 Integration Decision。

## D38 — Knowledge Boundary

Versioned Project Knowledge 主要属于 Repository。

Keystone 保存 Index / Projection / Derived Runtime Knowledge。

## D39 — Lifecycle vs Workflow

Keystone 不做任意 Workflow Engine。

Lifecycle 稳定，Stage Strategy / Capability 可扩展。

## D40 — 内部组织

Modular Monolith + Domain/Application + Ports/Adapters。

V1 不引入 Message Broker。

## D41 — Lifecycle Coordinator

Lifecycle Coordinator 推进 Change 宏观阶段。

Governance 决定是否允许推进。

Scheduler 调度 Runnable Work。

## D42 — Multiple Active Changes

一个 Project 可以有多个 Active Change。

Scheduler 需要 Project-level Coordination View。

## D43 — Actor

统一 Actor 抽象用于 Human / Agent / System / External。

V1 保持 Single-user / Local。

## D44 — Worker Authority

Worker 可替换，不拥有 Control Plane Durable Truth。

## D45 — V1 Strategy

采用 Thin Vertical Slice。

先跑完整闭环，再加深单个子系统。

## D46 — Repository Structure

V1 使用 Monorepo 管 CLI / Dashboard / Daemon / Worker / Core / Ports / Adapters。

## D47 — 默认运行模式

Autonomous by Default, Governed by Gates。

Human 主要负责 Intent、Observation 和必要 Decision。

## D48 — Capability

Capability 是统一执行抽象。

Skill / Role / Runtime / Tool 保持独立维度。

## D49 — API Boundary

Daemon 是唯一权威 Control Plane API Surface。

Worker 使用 Narrow Worker Protocol。

Client / Worker 不能直接修改 DB。

## D50 — Execution Guard

Governance 产生 Authorization。

Execution Guard 在 Side Effect 发生前执行 Enforcement。

## D51 — Learning Loop

Learning 产生 Evidence / Eval / Improvement Proposal。

Keystone 自身改进遵循：

```text
Proposal → Verification → Gate → Versioned Update
```

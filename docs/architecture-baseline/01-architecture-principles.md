# 01 — 架构原则

## P1. 独立 Control Plane

Keystone 独立实现。

LoopX、Anthropic AI-native SDLC、`to-tickets`、CodeMap、Cognitive Control Plane 等仅作为参考。

## P2. Stable Core + Explicit Extension Ports

核心模型保持稳定，外围实现可以替换。

高层稳定核心包括：

- Project
- Change
- Ticket
- Lifecycle
- Dependency
- Gate
- Evidence
- Artifact
- Agent Run
- Event
- Decision

主要扩展点包括：

- Runtime Adapter
- Skill / Capability
- Ticket Generator
- Context Provider
- Policy Provider
- Evidence Provider
- Knowledge Provider
- Source Control Adapter
- Storage Adapter
- External Tool Adapter
- Workspace Implementation

V1 不做复杂动态插件平台。

## P3. 先保证架构方向，再保证实现简单

第一版实现可以粗糙。

边界不能粗糙。

## P4. Lifecycle 稳定，Stage 内部策略可扩展

Keystone 保持固定 AI Coding Lifecycle Spine。

不做任意 Workflow Engine。

Stage 内部可以使用不同 Strategy / Skill / Capability。

## P5. Autonomous by Default, Governed by Gates

当 Keystone 拥有足够权限和证据时，Change 默认自动推进。

Human 主要处理：

- Approval
- Ambiguity
- Escalation
- Exception
- Hard Stop
- Conflict

## P6. Control Plane 持有权威状态

Agent、Runtime、Worker、Skill、CLI、Dashboard 都不能拥有 Lifecycle 权威状态。

## P7. Worker 管副作用，不管工作真相

Worker 是可替换执行节点。

## P8. Git 管代码真相

Keystone 引用 Git Revision / Diff，不自己再造版本控制系统。

## P9. Repository 管版本化项目知识

AGENTS.md、INDEX.md、架构文档、Conceptual Space 等长期工程知识原则上跟随 Git Repository。

Keystone 可以保存 Index、Projection、Cache、Execution Discovery 与 Derived Knowledge。

## P10. Evidence 优先于 Self-report

重要 Gate 不能只依赖 Agent 自述。

## P11. Executor 不应自我批准

执行与独立 Verification / Review 要在概念上分离。

## P12. Learning 产生 Proposal，不静默自我修改

Learning 应产生 Evidence / Eval / Improvement Proposal。

Skill、Policy、Context、Capability、Runtime Routing 的变更需要版本化并经过治理。

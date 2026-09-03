# Keystone V1 M0 → M9 实施顺序

## 原则

不要按五大逻辑子系统横向“做完”，而是每个 Milestone 让 Golden Path 向前多真实走一段。

## M0 — Engineering Foundation

实现：

- Go 1.27 Monorepo
- DDD + Modular Monolith 目录
- AGENTS.md / INDEX.md 规范
- Build / Test / Lint 基础
- Config / Logging / ID 基础
- React + TypeScript + Vite Dashboard 骨架

M0 不实现或预占 Migration 调用、Migration runner、版本表、SQLite、数据目录或对应占位目录；Migration、SQLite 和数据目录由 Ticket 02「Local State and Boundary Contracts」负责。

Done：

```text
make test
make build
make lint
make dashboard-build
```

## M1 — CLI → Daemon → SQLite

实现：

- daemon start / stop / status
- `/healthz`
- SQLite connection
- migration
- localhost HTTP

Done：CLI 可以确认 Daemon 与 DB ready。

## M2 — Repository → Init → Project

实现：

- Git root detection
- Project Domain / Application
- Project Repository Port + SQLite Adapter
- Control Plane Project API
- `.keystone/project.yaml`
- local reconciliation
- ProjectInitialized Event

Done：真实仓库可以 `keystone init`。

## M3 — Change → Lifecycle → Artifact/Event

实现：

- Change / Lifecycle
- ArtifactRef / Actor / Event
- create / show / list Change
- Lifecycle Coordinator
- 最薄 Governance 接口

Done：Change 状态机可通过测试 Strategy 推进。

## M4 — Daemon → Worker → RuntimeAdapter → Codex

实现：

- Worker Register / Heartbeat / Execute / Report
- Worker supervision
- RuntimeAdapter
- CodexAdapter
- OpenCodeAdapter 接口位置

Done：Daemon → Worker → real Codex → Daemon smoke run。

## M5 — Understand → Design → Plan

实现真实 Stage Strategy：

- Understand
- Design
- Plan
- Artifact persistence

这些 Run 默认 read-only，不修改 Project 原始 Repository。

Done：Change 自动产生 Understanding / Design / Plan Artifacts。

## M6 — Ticketize → Canonical Ticket Graph

实现：

- TicketGenerator Port
- ToTicketsInspiredGenerator
- Structured Draft
- Deterministic Validator
- Ticket / TicketDependency
- BLOCKED_BY

Done：Plan 可以形成可持久化 Canonical Ticket Graph。

## M7 — Worktree → Execute → Diff

实现：

- Workspace Port
- SourceControl Port
- GitWorktree Adapter
- Scheduler
- Execution Authorization / Guard
- custom branch name
- Codex 实际修改代码
- Worker 采集 diff / changed files / logs

Done：Ticket 能在真实 Change Worktree 中形成实际 Diff。

## M8 — Verify → Commit → Integrate Ready

实现：

- Workspace / Git Checks
- configured verify commands
- independent Acceptance Verifier Run
- Keystone-controlled commit
- custom commit template
- Change-level final verify
- candidate revision

Done：真实 Change 到达 Integrate Ready。

## M9 — Dashboard → Full Golden Path

实现：

- Projects
- Project Detail
- Change Detail
- Needs Human
- Lifecycle
- Tickets
- Runs
- Artifacts
- Trace
- SSE
- Daemon / Worker health

Done：浏览器能完整观察 Golden Path。

## Milestone Done 规则

> Done 不是“代码写完”，而是对应链路能真实运行。

| Milestone | 必须真实跑通 |
|---|---|
| M1 | CLI → Daemon → SQLite |
| M2 | Repository → Init → Project |
| M3 | Change → Lifecycle → Event |
| M4 | Daemon → Worker → Codex → Daemon |
| M5 | Change → Understand → Design → Plan |
| M6 | Plan → Ticket Graph |
| M7 | Ticket → Worktree → Codex → Diff |
| M8 | Verify → Commit → Integrate Ready |
| M9 | Dashboard 完整观察 Golden Path |

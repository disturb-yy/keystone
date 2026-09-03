# Keystone V1 正式 Ticket 索引

## 使用方式

本目录是 Keystone V1 的版本化执行计划。每次只认领一张处于 READY frontier 的 Ticket；实现前必须重新阅读根 `AGENTS.md`、根 `INDEX.md`、目标 package 的局部 `AGENTS.md` 与 `INDEX.md`，并以当时的源码树校验本 Ticket 的前置条件。

Ticket 描述的是已确认的实现契约，不替代 `docs/architecture-baseline/`。若发现两者冲突，停止实现并先对齐；不得通过修改 Ticket 静默改变冻结架构。

## 阻塞图

```text
01 Engineering Foundation
  ↓
02 Local State and Boundary Contracts
  ↓
03 CLI, Daemon and SQLite Readiness
  ↓
04 Repository Init and Project
  ↓
05 Change Lifecycle, Artifact and Event
  ↓
06 Local Worker and Codex Runtime
  ↓
07 Understand, Design and Plan
  ↓
08 Ticketize and Canonical Graph
  ↓
09 Worktree Execute and Diff
  ↓
10 Verify, Commit and Integrate Ready
  ↓
11 Dashboard Observation
  ↓
12 Golden Path E2E Evidence
```

## Ticket 一览

| Ticket | 里程碑 | `BLOCKED_BY` | 交付结果 |
| --- | --- | --- | --- |
| [01](01-engineering-foundation.md) | M0 | 无 | 可构建工程基础与 Dashboard 骨架 |
| [02](02-local-state-and-boundary-contracts.md) | M0 | 01 | 数据目录、Migration 与边界 DTO 基线 |
| [03](03-cli-daemon-sqlite-readiness.md) | M1 | 01、02 | CLI 可确认 Daemon 与 SQLite 就绪 |
| [04](04-repository-init-and-project.md) | M2 | 03 | 真实仓库可 `keystone init` 并形成 Project |
| [05](05-change-lifecycle-artifact-event.md) | M3 | 04 | Change 状态、Artifact 与 Event 由 Daemon 持久化 |
| [06](06-local-worker-and-codex-runtime.md) | M4 | 05 | Daemon 经本机 Worker 完成真实 Codex smoke run |
| [07](07-understand-design-plan.md) | M5 | 06 | 自动产生可验证的 Understanding、Design、Plan Artifact |
| [08](08-ticketize-canonical-graph.md) | M6 | 07 | Plan 形成持久化、无环的 Canonical Ticket Graph |
| [09](09-worktree-execute-diff.md) | M7 | 06、08 | Ticket 在 Change Worktree 内形成独立采集的 Diff |
| [10](10-verify-commit-integrate-ready.md) | M8 | 09 | 独立 Verify、Keystone Commit 与 Integrate Ready |
| [11](11-dashboard-observation.md) | M9 | 10 | Dashboard 观察权威状态与 Trace |
| [12](12-golden-path-e2e.md) | M9 | 11 | 真实 Golden Path 的可复盘验收证据 |

## 统一完成标准

- 仅实现当前 Ticket 的范围；不可提前实现下游 Ticket 的业务行为。
- `BLOCKED_BY` Ticket 的验收证据必须存在，才可开始实现。
- Domain、Application、Interface、Infrastructure、Worker 和 Runtime 的权责必须符合根 `AGENTS.md`。
- 每个 Ticket 都要更新受影响 Go package 的 `INDEX.md`；新建 Go package 时同步创建 `AGENTS.md` 与 `INDEX.md`。
- 运行本 Ticket 声明的验证命令；代码改动完成后运行 `go test ./...`，并在适用时运行 `go vet ./...`。
- 不把 Runtime 自报当作权威状态或 PASS Evidence；权威状态只能由 Daemon 在确认事实后推进。

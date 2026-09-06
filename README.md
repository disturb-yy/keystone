# Keystone

## 当前 checkout

- `cmd/keystone` 已提供 `keystone init`、`keystone change ...` 和 `keystone daemon start|status|stop`；CLI 通过 `contracts/controlplane` 的 HTTP/JSON 边界工作，不直接访问 SQLite。
- `cmd/keystone-daemon` 与 `internal/daemon` 已实现 loopback HTTP、单实例锁、SQLite readiness、Project Bootstrap 和 Change Lifecycle 路由。
- Change API 覆盖 `/v1/changes` 的创建/列表，以及 Change 的查询、生命周期命令、HumanDecision、Events、AgentRuns、ArtifactRefs 和 Artifact 内容读取。
- SQLite Migration 已到 v3：Project 与 Change 共用 `t_project_events` 追加式账本；Change Intent、AgentRun 证据和其他 Artifact 内容由 LocalStateRoot 下的 Artifact store 原子保存并按 SHA-256/长度校验。
- `internal/infrastructure/config`、`logging`、`id`、`localstate`、`migration`、`manifest`、`repository`、`artifact` 和 `workstore` 提供基础能力及 Project/Change 持久化适配；`dashboard/` 仍是无业务 API 的 React/TypeScript/Vite 骨架。

当前明确未实现：Worker 进程/runtime、Ticket Graph、真实 Strategy、Worktree 编排、Execute/Verify 的副作用执行与 Git 写入、Integrate 写入、Dashboard 业务功能。

详细的当前路径、边界、证据和刷新条件见根 [`INDEX.md`](INDEX.md)；版本化 Ticket/spec 保持为只读规范输入。

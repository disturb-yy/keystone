# Keystone

## 当前 checkout

- `cmd/keystone` 已提供 `keystone init`、`keystone change ...` 和 `keystone daemon start|status|stop`；CLI 通过 `contracts/controlplane` 的 HTTP/JSON 边界工作，不直接访问 SQLite。
- `cmd/keystone-daemon` 与 `internal/daemon` 已实现 loopback HTTP、单实例锁、SQLite readiness、Project Bootstrap 和 Change Lifecycle 路由。
- Change API 覆盖 `/v1/changes` 的创建/列表，以及 Change 的查询、生命周期命令、HumanDecision、Events、AgentRuns、ArtifactRefs 和 Artifact 内容读取。
- `scripts/golden-path-e2e.sh` 与 `testdata/golden-path-demo/` 提供 Ticket 12 的 revision-bound Golden Path Runner 和冻结 Demo Fixture；Runner 只保留安全 Review Packet，不自行发布成功 Evidence。
- Dashboard 的 Playwright devDependency 已固定为 `1.55.0`，用于生产构建后的 Ticket 12 深链接、SSE 重连和 reload Query 观察。
- SQLite Migration 已到 v3：Project 与 Change 共用 `t_project_events` 追加式账本；Change Intent、AgentRun 证据和其他 Artifact 内容由 LocalStateRoot 下的 Artifact store 原子保存并按 SHA-256/长度校验。
- `internal/infrastructure/config`、`logging`、`id`、`localstate`、`migration`、`manifest`、`repository`、`artifact` 和 `workstore` 提供基础能力及 Project/Change 持久化适配；`dashboard/` 仍是无业务 API 的 React/TypeScript/Vite 骨架。

当前明确未完成：Ticket 12 的真实 Codex、Linux/WSL 双平台独立 Run、独立只读 Review 和 Evidence 发布；这些运行时证据不能由 fixture、fake、构建或脚本静态检查替代。

详细的当前路径、边界、证据和刷新条件见根 [`INDEX.md`](INDEX.md)；版本化 Ticket/spec 保持为只读规范输入。M8 的目标契约、实施顺序和子票入口见[Ticket 10](docs/FE20260903080401/tickets/10-verify-commit-integrate-ready.md)；它不表示 Verify、Commit 或 Integrate Ready 已实现。

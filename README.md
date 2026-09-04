# Keystone

## 当前 checkout

- `cmd/keystone` 已提供 `keystone daemon start|status|stop`。`start` 可启动或复用独立 `keystone-daemon`，`status` 和 `stop` 只操作既有实例；CLI 通过 `contracts/controlplane` 的 HTTP/JSON 边界工作，不直接访问 SQLite。
- `cmd/keystone-daemon` 已提供独立进程入口，`internal/daemon` 已实现 loopback HTTP、单实例锁、SQLite 打开、Migration、RuntimeMetadata 和 readiness 生命周期。
- 当前 Daemon 路由是 `GET /healthz`、`GET /v1/daemon/status` 和 `POST /v1/daemon/stop`。ready 前 health 返回 `503`/`{"ready":false}`，ready 后返回 `200`/`{"ready":true}`；status 通过 DTO 返回 DatabasePath、SchemaMigrationVersion、DaemonReadiness 和 DaemonInstanceID。
- SQLite 仅使用 LocalStateRoot 下的 `state/keystone.db` 和 `t_schema_migrations` 元数据表；当前没有业务 Schema、业务 Repository 或根级 `migrations/`。
- `internal/infrastructure/config`、`logging`、`id`、`localstate`、`migration` 提供基础能力；`contracts/controlplane` 和 `contracts/worker` 只提供传输 DTO；`dashboard/` 仍是无业务 API 的 React/TypeScript/Vite 骨架。

当前明确未实现：Worker 进程/runtime、Project、Change、Ticket、业务 Domain/Application、业务状态持久化、业务 SQLite 表及 Dashboard 业务功能。目标架构和 Ticket 03 spec 只提供设计/规范输入，不构成额外运行行为证据。

详细的当前路径、边界、证据和刷新条件见根 [`INDEX.md`](INDEX.md)；Ticket 03 spec 保持为只读规范输入。

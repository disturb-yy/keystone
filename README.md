# Keystone

## 当前 checkout

- `go.mod`：声明模块 `github.com/disturb-yy/keystone` 和 Go `1.27`，并锁定 Ticket 02 所需的 `golang.org/x/sys` 与 `modernc.org/sqlite`；当前 Go package 均有聚焦测试。
- `Makefile`：提供根级 `test`、`build`、`lint` 和 `dashboard-build` 验证入口；Dashboard 目标会在 `dashboard/` 中按锁文件执行 `npm ci`。
- `docs/FE20260903080401/`：保存 V1 实施基线、里程碑、验收清单和版本化 Ticket/规格文档。
- `CONTEXT.md` 与 `docs/adr/`：保存已确认的领域/运行术语和难以逆转的架构决策；它们不表示对应运行能力已经实现。
- `docs/FE20260903080401/tickets/02-local-state-and-boundary-contracts/`：Ticket 02 的实施规格、子 Ticket 和验收记录。
- `internal/infrastructure/config`、`logging`、`id`：提供日志配置解析、JSON logger 和 UUIDv7 生成；`localstate` 提供本机数据根、目录、跨平台锁和诊断元数据；`migration` 提供纯 Go SQLite `t_schema_migrations` runner。
- `contracts/controlplane`、`contracts/worker`：提供 `/v1` Control Plane 与 Worker 的最小 JSON 边界 DTO，不访问 Domain 或 SQLite。
- `dashboard/`：已有 React、TypeScript、Vite 工程和 `package-lock.json`，仅为可构建骨架；`cmd/keystone`、`cmd/keystone-daemon`、`cmd/keystone-worker` 与 `configs` 保留 `.gitkeep` 预留事实。
- 当前文件树已有 `.go`、Ticket 02 基础设施/Contract 源码及 Dashboard 前端源码，但没有 Daemon、Worker runtime、HTTP API、业务数据库 Schema 或业务迁移。根 Make 入口可验证当前 Go 工程与 Dashboard 工程。

目标架构和 Ticket 规格中的流程、名称和拓扑不构成当前服务行为或接口实现的证据；当前实现边界以源码、测试和 `INDEX.md` 的事实记录为准。

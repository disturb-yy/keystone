# Keystone

## 当前 checkout

- `go.mod`：声明模块 `github.com/disturb-yy/keystone` 和 Go `1.27`；当前已有基础 Go package 和测试。
- `Makefile`：提供根级 `test`、`build`、`lint` 和 `dashboard-build` 验证入口；Dashboard 目标会在 `dashboard/` 中按锁文件执行 `npm ci`。
- `docs/FE20260903080401/`：保存 V1 实施基线、里程碑、验收清单和版本化 Ticket/规格文档。
- `docs/architecture-baseline/`：12 个 Markdown 架构参考文件和 1 个 JSON 摘要文件。
- `internal/infrastructure/config`、`logging`、`id`：已有三个可编译、可测试的标准库 Go package、聚焦测试和局部文档，分别提供日志配置解析、JSON logger 和 UUIDv4 生成。
- `dashboard/`：已有 React、TypeScript、Vite 工程和 `package-lock.json`，仅为可构建骨架；`cmd/keystone`、`cmd/keystone-daemon`、`cmd/keystone-worker` 与 `configs` 保留 `.gitkeep` 预留事实。
- 当前文件树已有 `.go` 和 Dashboard 前端源码，但没有 Daemon、Worker、API、数据库 Schema 或迁移实现；`contracts/`、`migrations/`、`scripts/` 尚未创建。根 Make 入口可验证现有 Go 基础能力与 Dashboard 工程。

`docs/architecture-baseline/` 中的文件是静态架构参考文本；其中的架构图、名称和流程文字不构成当前服务行为或接口实现的证据。

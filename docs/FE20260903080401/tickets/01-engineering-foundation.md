# 01 — Engineering Foundation

- 里程碑：M0
- `BLOCKED_BY`：无
- 交付类型：工程基础

## 目标

建立最小但真实可构建的 Monorepo 基础，使后续 Ticket 可以在不预建空业务层的前提下新增 Go package、Dashboard 和验证入口。

## 范围

- 建立实际需要的工程目录和根级构建入口，不创建没有行为的领域 package。
- 提供跨领域基础能力的最小实现位置：日志等级配置、结构化日志和标识符生成。
- 建立 React、TypeScript、Vite Dashboard 骨架及 npm 锁文件；它只需可构建，不提前实现业务页面。
- 提供根 `Makefile` 的 `test`、`build`、`lint`、`dashboard-build` 入口，并使根说明与索引反映真实文件树。
- 为本 Ticket 创建的每个 Go package 同步创建局部 `AGENTS.md` 与 `INDEX.md`。

## Migration ownership

M0 不实现或预占 Migration 调用、Migration runner、版本表、SQLite、数据目录或对应占位目录。Migration、SQLite 和数据目录由 Ticket 02「Local State and Boundary Contracts」唯一负责，具体实现与验收留在该 Ticket 的边界内；本节只修订 M0 ownership，不静默修改后续 Ticket 契约。

## 不包含

- Daemon、CLI、SQLite Schema、HTTP API 或 Worker 行为。
- SQLite driver、Migration 文件、Migration runner、版本表、数据目录与单实例锁。
- Dashboard 的 Project、Change、Trace 页面。
- 为未来领域预创建 `application`、`ports`、`adapters` 等空目录。

## 验收条件

- `go test ./...` 可执行并通过，且不再因仓库没有 Go package 失败。
- `make build`、`make test`、`make lint` 以及 `make dashboard-build` 均可运行。
- Dashboard 的 `npm run build` 产生生产构建产物。
- 新增目录、README 与根 `INDEX.md` 只描述当前可验证的文件树。

## 验证

```bash
go test ./...
go vet ./...
make build
make dashboard-build
git diff --check
```

## 实现边界

只建立后续实现所需的工程承载面。Migration、SQLite、数据目录和相关版本状态留给 Ticket 02；任何其他 Domain 规则、持久化表或 Client/Worker 协议均留给后续 Ticket。

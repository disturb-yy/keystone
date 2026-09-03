# 02 — Local State and Boundary Contracts

- 里程碑：M0
- `BLOCKED_BY`：01
- 交付类型：本机运行约定与边界契约

## 目标

固定本机数据归属、SQLite Migration 规则和最小传输 DTO，使后续 Daemon、Client 与 Worker 在同一可验证边界上实现。

## 范围

- 实现 `--data-dir` 覆盖与默认 `~/.keystone/` 布局：`state/keystone.db`、`artifacts/`、`workspaces/`、`runtime/`。
- 定义单实例锁、PID 与 Daemon endpoint 元数据的归属，禁止将用户级状态写入 Repository。
- 建立只增量 Migration 机制和版本表；Daemon 在持有单实例锁时执行 Migration。
- 建立 `contracts/controlplane` 的 `/v1` 通用 DTO、错误 envelope、健康检查 DTO 与 idempotency key 表达。
- 建立 `contracts/worker` 的 Register、Heartbeat、Assignment、Report DTO；它们不得复用 Domain Entity。

## 不包含

- HTTP Handler、SQLite 业务表、Project 或 Change 的具体 API。
- Worker 的进程监管、租约执行或 Runtime Adapter。
- Artifact 的业务类型和生命周期推进。

## 验收条件

- 默认数据目录与测试覆盖目录可被确定性解析，Repository Manifest 不被用作用户级状态目录。
- 新空数据库可完成 Migration；重复启动不会重复应用已记录 Migration。
- Control Plane 与 Worker Contract 能由独立 package 编译和单元测试。
- DTO 只包含边界字段，不导出或引用 Domain Entity。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

Artifact 原子落盘、状态事务和具体表由使用它们的 Ticket 实现；本 Ticket 只固定可复用的运行与契约骨架。

# 03 — CLI, Daemon and SQLite Readiness

- 里程碑：M1
- `BLOCKED_BY`：01、02
- 交付类型：首条真实运行链路

## 目标

实现 `keystone` CLI、用户级 Daemon、loopback HTTP 和 SQLite readiness，使 CLI 能确认 Daemon 与数据库均已就绪。

## 范围

- 实现 `keystone daemon start|stop|status`，包含单实例锁、endpoint 发现、优雅停止与明确错误。
- Daemon 仅监听 loopback 地址，完成 Migration 后才对外报告 ready。
- 实现 `GET /healthz` 和 `/v1` 的最小错误边界；Client 只能经 HTTP/JSON 访问 Daemon。
- 通过 CLI 的 ensure 流程启动或复用 Daemon，并对健康检查和 SQLite readiness 给出可判定结果。
- 创建实际 Go package 所需的局部 `AGENTS.md` 与 `INDEX.md`。

## 不包含

- Project 注册、Repository 检查或 `.keystone/project.yaml`。
- Change、Ticket、Worker 或 Dashboard 业务功能。
- 将 HTTP Handler 直接连接到 SQLite Repository 实现。

## 验收条件

- 在指定临时 `--data-dir` 中，CLI 可以启动 Daemon、查询 status 并停止该实例。
- Daemon ready 前不会把 `/healthz` 作为成功状态返回；ready 后返回 HTTP 200 与 JSON 响应。
- 数据库文件和 Migration 版本可从配置的数据目录查询。
- 第二个同数据目录实例被单实例机制拒绝，且不会破坏既有实例。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

Daemon 是唯一 API Surface 和 SQLite Owner；此 Ticket 不引入任何绕过 Daemon 的 Client 或 Worker DB 访问。

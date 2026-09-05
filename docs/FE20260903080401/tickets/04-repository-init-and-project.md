# 04 — Repository Init and Project

- 里程碑：M2
- `BLOCKED_BY`：03
- 交付类型：Project 纵切
- 当前状态：规格与本地子 Ticket 已对齐；仍受顶层 Ticket 03 验收阻塞，尚未实现

## 目标

让真实 Git Repository 可以通过 `keystone init` 被 Daemon 注册为 Project，并在 Repository Manifest、SQLite 权威状态和可查询 Event 之间完成 reconcile。

## 范围

- 在 `internal/work` 中实现 Project 的 Domain 语义、Repository Port 和 Application 用例；业务强边界放在 `internal/work/domain`。
- 实现 Git root 解析、规范化的 Repository 身份识别和 `.keystone/project.yaml` 的创建或 reconcile。
- 实现 Project SQLite Adapter、Control Plane Project Command/Query DTO、HTTP Adapter 和 CLI `init`。
- 将 Project 写入 SQLite，并在同一事务中记录 `ProjectInitialized` Domain Event。
- 使用 idempotency key 避免重复 init 产生重复权威 Project。

## 不包含

- Change 创建、Worktree、Runtime 或 Repository 全量分析。
- 让 Client 直接写 ProjectManifest 或任何 Control Plane 状态。
- 将 SQLite、Git CLI 或 HTTP 细节放入 Domain。

## 验收条件

- 在真实临时 Git Repository 中，`keystone init` 成功创建或 reconcile `.keystone/project.yaml`。
- 同一 Repository 重复 init 产生同一 Project，而非重复记录。
- Project Query 可返回权威 Project 状态，`ProjectInitialized` Event 可被查询。
- 非 Git 目录和不可 reconcile 的 Manifest 返回稳定的边界错误。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

Repository 保有版本化 Manifest；Daemon 保有 Project 权威状态。任何 Lifecycle 推进都留给 Ticket 05。

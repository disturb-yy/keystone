# 08 — Ticketize and Canonical Graph

- 里程碑：M6
- `BLOCKED_BY`：07
- 交付类型：Ticketize 纵切

## 目标

把 Plan Artifact 转换为经 Keystone 确定性校验、持久化并由 Daemon 权威持有的 Canonical Ticket Graph。

## 范围

- 在 `internal/planning` 定义 TicketGenerator Port 和首个 ToTickets-inspired Generator。
- 定义 Structured Ticket Draft Schema，至少包含 generation key、标题、范围、验收条件和 `BLOCKED_BY` 引用。
- 实现确定性校验：至少一张 Ticket、非空标题、唯一 generation key、依赖存在、无自依赖、无环、每张均有 Acceptance Criteria。
- 在 `internal/work` 持久化 Ticket 与 TicketDependency，并以事务记录 Graph 创建 Event。
- 提供 Ticket 和 Dependency Query，Client 不得直接写 Canonical Graph。

## 不包含

- 高级依赖类型、执行 DAG 优化、跨 Change 并行或图数据库。
- Worker 领取 Ticket、创建 Worktree 或运行 Codex 修改代码。
- 让生成器输出未经校验就成为权威 Ticket。

## 验收条件

- 合法 Plan 可形成可查询的 Canonical Ticket Graph。
- 每种最小非法 Draft 都被确定性拒绝，并留下失败 Artifact 或错误记录。
- `BLOCKED_BY` 图无环，READY frontier 可由权威 Graph 推导。
- 生成器输出与 Canonical Graph 的身份、来源 Artifact 和创建 Event 可追溯。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

V1 只实现 `BLOCKED_BY`。Ticket Generator 是建议来源，Keystone Validator 和持久化 Graph 才是权威来源。

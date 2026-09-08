# 08 — Ticketize and Canonical Graph

- 里程碑：M6
- `BLOCKED_BY`：07
- 交付类型：Ticketize 纵切

## 实施文档

- [共同规格](08-ticketize-canonical-graph/spec/08-ticketize-canonical-graph-spec.md)
- [实施子票](08-ticketize-canonical-graph/tickets/)

## 目标

把 Plan Artifact 转换为经 Keystone 确定性校验、持久化并由 Daemon 权威持有的 Canonical Ticket Graph。

## 范围

- 在 `internal/planning` 定义 TicketGenerator Port、首个 ToTickets-inspired Generator、Structured Ticket Draft Schema 和确定性候选校验；Generator 只通过 Ticketize AgentRun 提供候选。
- Structured Ticket Draft 至少包含 generation key、标题、范围、验收条件和 `BLOCKED_BY` 引用。
- 实现确定性校验：至少一张 Ticket、非空标题、唯一 generation key、依赖存在、无自依赖、无环、每张均有 Acceptance Criteria。
- 由 `internal/work/domain` 定义 Ticket 与 TicketDependency 不变量，`internal/work` 编排权威提交，`internal/infrastructure/workstore` 以事务持久化 Graph、Ticket 与 Dependency，并记录 Graph 创建 Event。
- 提供 `GET /v1/changes/{change_id}/ticket-graph` 只读查询；Client 不得直接写 Canonical Graph。

## 不包含

- 高级依赖类型、执行 DAG 优化、跨 Change 并行或图数据库。
- Worker 领取 Ticket、创建 Worktree 或运行 Codex 修改代码。
- 让生成器输出未经校验就成为权威 Ticket。

## 验收条件

- 合法 Plan 可形成可查询的 Canonical Ticket Graph；`TicketGraphCreated` 直接关联 Graph 身份，并通过 Graph 可追溯 Plan 与 Draft 来源。
- 每种最小非法 Draft 都被确定性拒绝，并留下有界 Failure Artifact；基础设施不可用不伪造失败事实，允许同一 Report 重送。
- `BLOCKED_BY` 图无环，StructuralFrontier 可由权威 Graph 推导。
- 生成器输出与 Canonical Graph 的身份、来源 Artifact 和创建 Event 可追溯。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

V1 只实现 `BLOCKED_BY`。Ticket Generator 是建议来源，Keystone Validator 和持久化 Graph 才是权威来源；成功 Graph 提交、`TicketGraphCreated`、当前 Ticketize AgentRun 成功和 Change 从 Ticketize 到 Execute 的推进是同一权威事务。被 Pause、Cancel 或失效执行权围栏的候选只保留为 Trace；Resume 创建新的 TicketGeneration，不能复用围栏候选。

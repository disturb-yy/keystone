# 05 — Change Lifecycle, Artifact and Event

- 里程碑：M3
- `BLOCKED_BY`：04
- 交付类型：Change 生命周期纵切

## 目标

实现由 Daemon 权威持有的 Change 生命周期、不可变 Artifact 与 Domain Event，并提供明确的 Pause、Resume、Cancel、Retry 和 Human Decision 语义。

## 范围

- 实现 `stage + status` 分离的 Change 模型：Stage 依既定链路推进，Status 为 `active`、`paused`、`human_required`、`cancelled`、`integrate_ready`。
- 创建 Change 即 Start，固定 `base_revision`，保存 Intent Artifact，并由 Lifecycle Coordinator 编排后续 Stage。
- 将状态变更和对应 Domain Event 在同一 SQLite 事务提交；Artifact 使用原子落盘、摘要和引用记录。
- 失败记录在 AgentRun/Artifact 层；Retry 创建新 AgentRun 并保留历史。Pause 阻止新 Assignment，Cancel 阻止后续推进，Human Decision 追加记录恢复动作。
- 提供 Change 的 create、show、list 与 Command API；Client 不得直接设置 Stage 或 Status。

## 不包含

- Worker、Runtime、真实 Understand/Design/Plan Strategy。
- Ticket Graph、Worktree、Verify 或 Git Commit。
- 完整 Policy DSL、Risk Model 或复杂 Gate 类型。

## 验收条件

- Change 创建后有持久化 Intent、固定 base revision 和可查询 Event。
- 合法 Stage/Status 变迁通过 Domain 单元测试；非法变迁被拒绝。
- 状态更新与 Event 不会半提交；Artifact 引用可定位到内容和摘要。
- Pause、Cancel、Retry、Human Decision 的历史可查询，且不会覆盖既有事实。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

Lifecycle Coordinator 只协调。它不分析需求、不生成计划、不执行工具，也不将 Worker Report 当作无条件状态推进依据。

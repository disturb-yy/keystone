# 07-03：Planning Coordinator、AgentRun 输入输出与 Artifact 持久化

**What to build：** 将 07-01/07-02 的纯 Contract/Strategy 接入 Work Application 和 Artifact/Workstore port，形成单个 Change 的严格串行 Planning Coordinator，并以事务保存 AgentRun、ArtifactRef、Artifact link、Event 和阶段推进。

**Blocked by：** 07-01、07-02、顶层 Ticket 06；必须以 Ticket 05 已验证的 Change/AgentRun/Event/fencing 能力为基础。

**Status：** implemented

## Coordinator rules

- Coordinator 读取 Change 的当前 stage/status、不可变 `base_revision`、Intent、ProjectContext 和最近成功的上游 Artifact。
- 只在 active 且没有 running Planning AgentRun 时启动当前 stage；同一 Change 同时只能有一个阶段尝试。
- 每次 Stage 使用一个 AgentRun 和递增 attempt。失败后保持当前 stage，进入 `human_required`，不自动 retry。
- 成功的 Understand/Design 只推进到下一个 Planning stage；Plan 成功推进到 `Ticketize`/`active` 后停止。
- Coordinator 不接受 Runtime 自报的 lifecycle、done、Ticket 或 Verify 结果。

## Persistence transaction

成功或失败提交必须保证：

1. 候选输出经过 decoder/validator 和固定 revision 校验。
2. Artifact content 通过有界和摘要校验后写入现有 Artifact store。
3. Workstore 在一个事务内登记/更新 ArtifactRef、AgentRun input/output/failure links、AgentRun 终态和 Event。
4. 只有当前 stage、当前 attempt、active 状态和成功条件同时满足时，才追加 StageAdvanced/推进 Change。

ArtifactRef 只做向后兼容的最小 metadata 扩展：`kind`、schema version、summary、source revision 和必要输入关联。旧引用不迁移、不重写；现有统一 Event ledger 继续作为唯一事件账本。

Migration 使用实现时下一个可用版本，由 `internal/infrastructure/workstore/` 持有，采用 additive schema change。checksum 漂移或应用失败必须阻止 ready，不自动修复。

## Recovery and fencing

- 重复 completion 返回已有终态，不产生第二个 Event 或 ArtifactLink。
- 旧 AgentRun、旧 attempt、暂停/取消后的 fenced result 只能按当前 Work 规则保存可审计事实，不能推进 Change。
- 失败、schema invalid、Artifact write failure、Runtime timeout 和 revision mismatch 进入 `human_required`，保留 Failure/raw log Artifact。
- 人工 retry 使用新的 AgentRun/attempt，复用已成功的不可变上游输入；不修改旧 AgentRun。

## Acceptance

- Coordinator 单测覆盖串行阶段、重复调用、失败停机、Plan 完成停在 Ticketize、当前 AgentRun 判断和人审 retry 输入复用。
- Workstore 集成测试覆盖 Artifact/AgentRun/Event/Change 的原子提交、迁移兼容和旧引用读取。
- 没有在 Planning package 中直接写 SQL、调用 SQLite 或引入 HTTP Handler/Worker concrete type。

## Out of scope

- 临时 Snapshot materialization、Daemon startup scan 和 Worker Supervisor wiring（07-04）。
- Ticket Draft、Canonical Graph、Ticketize Assignment 和真实 Worker/Runtime 实现。

## Verification

```bash
go test ./internal/work/... ./internal/infrastructure/workstore/... ./internal/planning/...
go test ./...
go vet ./...
git diff --check
```

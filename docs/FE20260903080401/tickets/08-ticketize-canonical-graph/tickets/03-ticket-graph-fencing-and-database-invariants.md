# 08-03：Ticket Graph 围栏与数据库不变量

**What to build：** 为 08-01/08-02 的 Ticketize 路径补齐不可变 Graph、跨图隔离、依赖无环、来源一致性、重放/并发以及 Pause/Cancel/Resume/失效执行权的纵深防线，使重启和晚到结果不能制造或改写权威图。

**Blocked by：** 08-02、顶层 Ticket 07。实现前必须以当前 Workstore Migration 序列、既有 Event/AgentRun/Artifact trigger 和实际 Pause/Cancel/Lease fencing 行为为准，不得假定规划文档已经落地。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 为已提交的 Graph、Ticket、Acceptance Criterion 与 Dependency 实现数据库不可变性：拒绝 update/delete，并确保 Change 至多一个成功 Graph。
- 使用复合外键约束 Ticket/Dependency 必须在同一 Graph；拒绝跨图 dependency、重复 dependency、自依赖和损坏的来源 Artifact/AgentRun/Change 组合。
- 用递归 CTE 或等价的数据库防线拒绝闭环，保留应用层 validator 作为第一道、数据库作为最后一道防线。
- 对 Graph 的最终提交状态施加可验证约束：1 至 64 张 Ticket、每张 1 至 32 条 Acceptance Criteria、Plan 来源为同 Change 成功 Plan output、Draft 来源为当前 Ticketize run output。
- 在 Event ledger 中保持 TicketGraphCreated 与 Graph 的单向关联：Event 通过 graph_id/change_id/project_id 复合外键关联 Graph；Graph 不保存 created_event_id，从而避免循环外键。
- 将 TicketGraphCommitIdentity 收敛为 TicketizeAgentRunID 加 Candidate Artifact 内容身份的内部重放身份；同一身份返回既有图，不同身份不能覆盖或替换既有图。
- 覆盖 Pause、Cancel、Resume、Lease/attempt 失效、旧 Worker、重复/晚到 Report、Daemon 重启和并发 completion。若围栏事实先于 Graph 提交，Candidate 只保留 Trace；Cancel 永不恢复，Resume 必须创建新的 TicketGeneration。
- 若为补强约束需要新增 Migration，使用当时下一个可用 additive version，并以 Migration/Store 集成测试证明旧 Event、Artifact 和 AgentRunReportLate 数据完整保留。

## Acceptance

- 直接 SQL 或错误 Application 路径无法更新、删除、替换或为同一 Change 再次插入成功 Graph。
- 数据库拒绝跨图 dependency、自依赖、重复边、环、缺少 Acceptance Criteria、空图、来源 Change/Plan/Draft/AgentRun 不一致及其他会破坏已提交事实的写入。
- 同一 Candidate completion 的重放返回首个 Graph；不同 Candidate、旧 run、旧 attempt 或已有 Graph 的写入被稳定地分类为冲突或 fenced，且不产生第二个 StageAdvanced。
- Pause/Cancel/Lease 失效/晚到 Report 与成功提交竞争时，只有提交前仍持有有效执行权的一方可形成图；围栏 Candidate 在 Resume 后不可复用。
- 重启后从持久化状态恢复时不会重新生成、删除或替换已成功图，也不会把旧 Candidate 解释为当前可提交结果。
- Event ledger schema 迁移保留旧数据、Artifact 关联、索引和 AgentRunReportLate 触发语义；TicketGraphCreated 仅在有效 Graph 创建时存在并正确关联。

## Out of scope

- 改变 08-01 已定义的公开 GET/CLI ReadModel 字段，或把内部 fencing/commit identity 暴露给 Client。
- 新增 Ticket status、RunnableTicket、Scheduler、ExecutionAuthorization、Assignment、Worktree 或执行 DAG。
- 将暂停后的候选自动重试、将取消后的 Change 恢复、编辑/版本化 Graph 或跨 Change 合并 Graph。

## Verification

~~~bash
go test ./internal/infrastructure/workstore/...
go test ./internal/work/...
go test ./internal/daemon/...
go test ./...
go vet ./...
make build
git diff --check
~~~

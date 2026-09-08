# 08-04：Ticket 08 集成验收与导航

**What to build：** 在前序子票完成后，以可控 fixture/fake Generator 贯通 Plan 输入、Ticketize、Graph 事务、Event、恢复围栏、HTTP、CLI 和 SQLite upgrade，并把导航文档刷新为当前 checkout 可验证的事实。

**Blocked by：** 08-01、08-02、08-03、顶层 Ticket 07。顶层 Ticket 07 必须有实际实现与验收证据；本子票不能用 stub、规格或交叉编译替代其运行前置条件。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 创建高层集成 fixture：已验证 Plan、固定 base_revision/ProjectContext、受控 fake TicketGenerator、Ticketize AgentRun、SQLite Workstore 和 Daemon API/CLI client。
- 覆盖一条合法成功链：Plan 来源验证 → Candidate → Graph/Tickets/Dependencies → TicketGraphCreated → AgentRun success/StageAdvanced → Change Execute → GET/CLI 查询。
- 覆盖无图查询、未知 Change、非法 ID、generator_failed、draft_invalid、ticketize_unavailable 后同 Report 重送、人工 retry、新 Generation、Pause/Cancel/Resume、失效 Lease、晚到/重复 Report、并发 completion 与重启恢复。
- 验证迁移升级路径保留历史 Event、Artifact 关联与 AgentRunReportLate；验证不可变图、单 Change 唯一图、同图依赖、无环和无部分提交。
- 将受影响的根 INDEX、Ticket 索引、Planning/Work/Workstore/Daemon/Contract/CLI 局部 INDEX 和必要的 README/规约更新为实际文件树与已验证行为；不把目标架构或未实现下游功能写成现状。
- 运行根级测试、vet、构建和 diff hygiene；若真实 Ticket 07/06 前置验收缺失，记录精确证据并保持未实现范围诚实。

## Acceptance

- 一个测试 Change 从已验证 Plan 形成一次且仅一次可查询 Canonical Ticket Graph；Query/CLI 不泄露 GenerationKey 或执行授权状态。
- 所有成功事实在同一事务可观察，所有失败/不可用/围栏情形按规格分离，且没有部分 Graph、错误 stage 推进、自动 retry 或围栏 Candidate 复用。
- 重启、重复 Report 和并发 completion 后仍保持同一 Graph/来源/Event 链，历史 Event ledger 可读且 AgentRunReportLate 不回归。
- 文档索引只描述 checkout 中实际实现、实际测试和实际 Migration，不把 Ticket 08 的规划规格或下游 Ticket 09 行为说成已落地。
- 根级 go test ./...、go vet ./...、make build 与 git diff --check 通过；任何环境限制、前置阻塞或未能执行的真实平台验证均有明确、可复查的记录。

## Out of scope

- 新功能开发、Graph 编辑/版本化、外部 Issue Tracker 发布或 Dashboard Graph 页面。
- Ticket 09 及后续的调度、执行、Worktree、Diff、Verify、Commit、Integrate Ready 和 Golden Path。
- 以真实 Codex 代替受控 seam 测试；真实 Worker/Runtime smoke 仍按 Ticket 06 的专属验收边界处理。

## Verification

~~~bash
go test ./...
go vet ./...
make build
git diff --check
~~~

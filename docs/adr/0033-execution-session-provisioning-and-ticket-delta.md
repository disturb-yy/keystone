# ADR-0033：执行会话、可恢复 Workspace Provisioning 与 Ticket 增量证据

## 状态

已接受。

## 决策

被接受的 `ExecuteCommand` 建立可恢复的 ExecutionSession：Daemon 在 Workspace 与首个 ExecutionAuthorization 持久化后返回 accepted，不等待 Worker 或 Codex；Worker 暂不可用只使会话待执行，不撤销显式执行意图。会话内每张 Ticket 的 TicketExecutionSuccess 先进入 Ticket 10 定义的 TicketVerifyCommitGate；只有当前 Ticket 已 Verify PASS 并形成 KeystoneCommit 后，Daemon 才继续串行调度下一张 RunnableTicket，直到全部 Ticket 都已成功收口或进入 human_required。

Daemon 先持久化 WorkspaceProvisioningIntent，再创建或核验 Git Worktree，最后完成 Workspace 权威记录。首次创建恢复只能接受与预期 Change、BaseRevision 和 WorkspaceBranch 完全一致的结果；已有 KeystoneCommit 后的恢复必须改核验记录的 WorkspaceInputRevision。未知路径、分支冲突或不明 Worktree 不自动删除，而是进入 human_required。每个 Ticket-bound AgentRun 同时保存执行前后的 WorkspaceSnapshot、TicketDelta 与 CompleteChangeDiff：前者归因单张 Ticket 的增量，后者证明相对于该 Ticket WorkspaceInputRevision 的完整未提交状态。Pause、Cancel 或授权围栏释放 ProjectExecutionSlot，晚到 Report 只进入 Trace；人工 retry 只在原 Workspace 为同一 Ticket 建立新的 AgentRun，不重置或清理已有 Diff。

## 理由与边界

将接受请求与 Worker 可用性解耦，既避免客户端等待 Runtime，也不让 Worker 重启丢失已经明确授权的本机工作。持久化 provisioning intent 避免 Git 与 SQLite 中断后通过删除或猜测取得表面一致。累计 Workspace Diff 无法说明多 Ticket 串行执行中每张 Ticket 的贡献，因此必须同时保存全量状态与增量归因。ADR-0051 仅细化本 ADR 的下一张 Ticket 调度时机；该 ADR 不证明实现存在，也不增加多 Worker、跨 Change 并行、自动 Git 清理或 Ticket 10 的具体 Commit/Verify 细节。

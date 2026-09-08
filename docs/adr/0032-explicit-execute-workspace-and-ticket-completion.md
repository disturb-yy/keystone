# ADR-0032：显式 Execute、Change Workspace 与 Ticket 级完成

## 状态

已接受。

## 决策

V1 只能由带 `ChangeVersion` 与 `IdempotencyKey` 的显式 `ExecuteCommand` 启动 Change 执行准备。Daemon 在首次创建 Workspace 前复核源 Repository 仍干净且 `HEAD == BaseRevision`，固定默认或一次性校验通过的自定义 branch；同一 Project 同时至多一个写 Workspace 的 Assignment。Daemon 的 Scheduler 只从 RunnableTicket 选择对象，并在形成可审计的 ExecutionAuthorization 后创建绑定该 CanonicalTicket 的 AgentRun 与 Assignment。

一次成功 Report 只完成其绑定的 CanonicalTicket，不能直接推进整个 Change；所有 CanonicalTicket 均满足 TicketExecutionSuccess 后，Change 才从 Execute 进入 Verify。TicketExecutionSuccess 要求独立观察到成功退出、无 Guard 或采集失败，以及完整、非空、未截断的 Diff/changed-files 证据；允许 `git add`，但 Runtime 仍不得 commit、push 或 merge。源快照漂移、Workspace 身份不符、失败或不完整证据均进入 human_required，不自动 retry 或跳过 Ticket。

## 理由与边界

自动触发会使 Ticketize、查询或 Worker 回调意外引入 Git/Codex 副作用；按 Change 维度持久化 Workspace、branch 和授权事实才能在重启后恢复同一执行上下文。将 Ticket 完成与 Change Stage 分开，避免第一张 Ticket 的成功跳过剩余 READY frontier。该 ADR 只定义 Ticket 09 的目标契约，不证明 06、08 或 09 已实现，也不扩展到多 Worker、跨 Change 并行、Verify、Commit 或 Worktree 清理。

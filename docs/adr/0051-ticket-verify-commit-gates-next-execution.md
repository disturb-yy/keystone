# ADR-0051：Ticket Verify/Commit 串行收口后才执行下一张 Ticket

## 状态

已接受。

## 决策

V1 在每个 TicketExecutionSuccess 后保持 Change 的 LifecycleStage 为 Execute，并打开该 CanonicalTicket 的 TicketVerifyCommitGate。Client 必须先以 VerifyCommand 形成 PASS VerificationEvidence，再以 CommitCommand 形成 KeystoneCommit；在该门完成前，Daemon 不得为同一 ExecutionSession 的任何下一张 Ticket 创建 ExecutionAuthorization 或 Assignment。

所有 CanonicalTicket 都已通过这一门后，Daemon 才将 Change 从 Execute 原子推进到 Verify。FinalVerifyCommand 仅在 Verify 阶段、所有 Ticket 都已有 KeystoneCommit 时接受；其 PASS 形成 CandidateRevision、FinalVerify 检查点与 integrate_ready。

## 理由与边界

逐 Ticket 收口使每次 Git Commit 正好覆盖刚完成 Ticket 的 Workspace 增量，避免在所有 Ticket 已修改同一 Workspace 后从历史 TicketDelta 重建和应用补丁。它也允许未 Commit Ticket 的 Verify 失败安全回到同一 Execute 尝试，而不回写已提交 Git 历史。该 ADR 细化 ADR-0033 的“继续下一张 Ticket”时机，不改变 Ticket 09 的“所有 TicketExecutionSuccess 后才离开 Execute”检查点，也不证明任何执行、验证或提交实现已经存在。

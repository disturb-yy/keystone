# 09-03：原子完成 Ticket 并记录双重 Diff Evidence

**What to build：** 一张已 Claim 的 Ticket 在 Assigned Workspace 中完成一次受控 Runtime 后，Daemon 以执行前后 WorkspaceSnapshot、Worker 原始 Artifact 和完整 Git 状态共同判定成功，原子写入 TicketExecutionEvidence 与 AgentRun 终态，并只完成当前 Ticket。

**Blocked by：** 顶层 Ticket 06、08 的真实实现与验收证据；本地 Ticket 09-02：确定性调度与唯一 Runtime Claim。

**Status：** ready-for-agent（仅文档成熟度；顶层 Ticket 06、08 未解除）

- [ ] Runtime 启动前采集不可变 WorkspaceSnapshot(pre)，退出后在同一 Lease/Claim 仍有效时采集 Snapshot(post)，两者绑定当前 WorkspaceInputRevision。
- [ ] Worker 只上报 stdout、stderr、diff、changed_files 四类原始 Artifact 及退出/采集事实；Daemon 验证摘要、大小、截断标记、相对路径和完整性，Runtime 文本不能成为生命周期依据。
- [ ] 成功必须同时满足零退出码、无 Guard/Capture failure、HEAD/branch 未改变、无 residual untracked、Worker diff/changed_files 与 Daemon 完整未提交状态一致、CompleteChangeDiff 与 TicketDelta 均非空。
- [ ] staged、unstaged、新建 tracked 文件、空 diff、截断 diff、未跟踪残留、HEAD 漂移、Artifact 摘要不符和证据不一致均形成明确失败或 `human_required`，不能伪造 `succeeded`。
- [ ] Snapshot、Worker 原始 Artifact、CompleteChangeDiff、TicketDelta、AgentRun 终态、Ticket `succeeded` 和审计事件在一个 authority transaction 中落盘；一次成功只能完成绑定的 Canonical Ticket。
- [ ] 同一 Report 的临时存储失败可按相同 digest 重试且不重复执行、不重复推进；失效 Claim 的晚到 Report 不能改变权威状态。
- [ ] 使用真实临时 Git fixture、fake Runtime 和 Worker 报告测试覆盖完整双重 Diff 比对、原子回滚、重放与脱敏 read model；不实现 Verify、Commit 或 Integrate。

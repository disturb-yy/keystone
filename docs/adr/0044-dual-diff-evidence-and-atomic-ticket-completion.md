# ADR-0044：双重 Diff 证据与原子 Ticket 完成

## 状态

已接受。

## 决策

Daemon 仅在 RuntimeClaim 成功后、子进程启动前采集 WorkspaceSnapshot(pre)。Runtime 退出且 Worker 提交 Report 后，Daemon 在同一 Lease/Claim 围栏仍有效时采集 WorkspaceSnapshot(post)，从中形成规范化、非空的 CompleteChangeDiff 和非空 TicketDelta。

Ticket 成功要求退出码为零、无 Guard 或 Capture 失败、HEAD 保持该 Ticket 的 WorkspaceInputRevision、WorkspaceBranch 未变、没有 residual untracked 文件，并且 Worker 原始 `diff`、`changed_files` 未截断且与 Daemon 相对于同一 WorkspaceInputRevision 的规范化完整未提交状态一致。`stdout`、`stderr` 可保留其截断元数据，但不能替代 Diff 证据。

Git 采集先完成；随后 Worker 原始 Artifact、两个 Snapshot、CompleteChangeDiff、TicketDelta、AgentRun 终态、Ticket `succeeded` 与审计事件必须在同一权威事务提交。临时采集、Artifact 或 SQLite 不可用时，只允许同一 Report 重试而不推进状态；证据不一致或不变量失败进入 `human_required`。

## 理由与边界

Worker 的独立观察满足执行证据要求，Daemon 的规范化快照提供完整状态和 Ticket 归因；两者共同避免由 Runtime 自报、局部 Diff 或累计 Diff 单独决定成功。提交前的 Git I/O 不能与 SQLite 构成单一物理事务，因此只将已完成采集的结果与权威状态原子关联。

该 ADR 证明受围栏窗口内的可观察 Git 状态，不声称完整 OS 隔离，也不将外部进程造成的文件变化识别为特定主体。

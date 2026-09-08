# ADR-0035：Ticket 执行证据与执行状态机

## 状态

已接受。

## 决策

Ticket 09 保持 Worker Protocol 的四种 WorkerArtifactKind：stdout、stderr、diff、changed_files；不把 CanonicalTicket、WorkspaceSnapshot 或 TicketDelta 加入 Worker Contract。Worker 独立采集并上报完整 Workspace 状态的原始观察，Daemon 的 SourceControl 在 Execute AgentRun 前后形成 WorkspaceSnapshot、派生 TicketDelta。Daemon 必须在同一 Lease 围栏的 Ticket 执行完成事务中持久化 Worker 原始观察、CompleteChangeDiff、Snapshot 与 Delta，并以专用的 TicketExecutionEvidence 关联它们；不能借用通用 Change 级 ArtifactRef 的 role/ordinal，也不能在成功 Report 后另开补写事务。

每张 CanonicalTicket 独立持有 `pending`、`assigned`、`succeeded` 或 `human_required` 的 TicketExecutionState。只有 pending、所有 blocker succeeded、active ExecutionSession 与有效 Workspace 同时成立时才是 RunnableTicket；授权和 AgentRun 创建原子置为 assigned，成功置为 succeeded，失败置为 human_required 并停止整个 Change 的调度。人工 retry 只为同一 Ticket 创建新的 AgentRun 后回到 pending；不存在自动 retry、skip 或由 Worker 决定状态的路径。

## 理由与边界

扩展 Worker Contract 会把 Ticket 权威与 Workspace 归因泄漏给执行进程，并违背已冻结的四类传输边界；把两类 Diff 塞进一个 `diff` payload 则无法查询或归因。专用证据事务同时保留 Worker 的独立观察与 Daemon 的权威状态，避免多 Ticket 串行时复用 Change 级 ArtifactRef 造成冲突。该 ADR 只定义 Ticket 09 的目标持久化和状态语义，不证明 Schema、SourceControl、Scheduler 或 Report 事务已实现。

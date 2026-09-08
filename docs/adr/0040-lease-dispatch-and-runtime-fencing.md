# ADR-0040：Lease 发放、等待与 Runtime 围栏

## 状态

已接受。

## 决策

ExecutionSession 与 ExecutionAuthorization 可以在没有可用 Worker 时保持有效，并保留其持久化 FIFO 调度位置；此时不发放 Lease、不启动 Runtime，也不因暂时无 Worker 将 Change 置为 `human_required`。FIFO 队首不得被后续 Change 跳过。

Daemon 仅可在已注册 Worker 可用时，以同一权威事务创建唯一 Assignment、发放 Lease 并取得 ProjectExecutionSlot。Lease 一经发放，过期、Worker 丢失、Daemon 重启或 Heartbeat 续约失败都必须形成 ExecutionFence：Worker 取消 Runtime context，并在有界宽限期后终止其子进程；Daemon 将关联 AgentRun 置为失败、CanonicalTicket 置为 `human_required`，并释放执行槽。任何晚到 Report 只可保留 Trace，不能自动重派、重试或推进状态。

## 理由与边界

未交付前的临时 Worker 不可用不是执行失败；已交付后的 Lease 失效则意味着 Daemon 无法再证明唯一 Runtime 仍拥有 Workspace 写入资格，必须优先围栏。该决定不引入通用自动重试、运行中抢占或跨 Change 并行，也不声称 Daemon 重启后能直接终止已失联的旧进程；旧 Worker 必须通过续约失败自行停止。

该 ADR 冻结 Ticket 09 的目标恢复契约，不证明 Lease 状态机、Heartbeat 响应语义或进程终止机制已实现。

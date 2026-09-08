# ADR-0048：Execute 围栏与重新入队恢复

## 状态

已接受。

## 决策

Ticket 09 的 Execute 专用规则覆盖早期通用 Worker 的 Pause 语义。Pause 或 Cancel 必须围栏待发放及活动的 Authorization/Lease，取消 Runtime 并释放 ProjectExecutionSlot。Pause 将未成功的 assigned Ticket 回到 pending，保留 Workspace、Diff 与已成功 Ticket；Resume 不复用旧 Authorization、Lease 或 AgentRun，而是以新的 ExecutionDispatchEpoch 重新排到 Project FIFO 队尾。Worker 暂不可用不会改变当前 epoch。

Cancel 同样围栏并保留 Workspace 与证据，但终止 ExecutionSession，永不重新排队、重试或自动清理。human_required 的 retry 只将失败的同一 Ticket 回到 pending，并在新的调度 epoch 中创建新的 AgentRun；不 reset、checkout、clean 或丢弃失败时的 Diff。

Report 与 Pause/Cancel 的竞态以 SQLite 中首先提交的权威事务为准：Report 先提交则保留该结果，控制命令只作用于后续步骤；控制命令先提交则该 Report 仅作为 LateReport 保存。

## 理由与边界

Pause 必须真正撤销 Workspace 写入资格，不能让已暂停 Change 继续由 Runtime 修改；重新入队而非恢复旧位置避免主动让出执行槽的 Session 插队。保留 Workspace 与 Diff 使人工能检查或继续失败尝试，但不把保留误认为自动恢复。

该 ADR 只细化 Ticket 09 的 Execute 行为，不改变后续 Ticket 的 Verify/Commit 规则，也不引入抢占、自动 retry 或 Worktree 清理。

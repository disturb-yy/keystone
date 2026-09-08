# 09-04：执行 Fence、恢复与人工重试边界

**What to build：** 执行过程中发生 Lease/Heartbeat/Worker/Daemon 故障或显式控制命令时，Daemon 以持久化 Fence 和 DispatchEpoch 收敛运行、Ticket、Workspace、Diff 与 Trace，拒绝过期回报，并仅按明确的人类决策重试，不做猜测性 Git 修复。

**Blocked by：** 顶层 Ticket 06、08 的真实实现与验收证据；本地 Ticket 09-03：原子完成 Ticket 并记录双重 Diff Evidence。

**Status：** ready-for-agent（仅文档成熟度；顶层 Ticket 06、08 未解除）

- [ ] Lease 过期、Worker 丢失、Heartbeat 失败或 Daemon 重启会取消 Runtime context；宽限期后终止子进程，AgentRun 失败、Ticket 进入 `human_required`、释放 Project slot，晚到 Report 只保留 Trace/受控 Artifact。
- [ ] Pause 形成 ExecutionControlFence，保留 Workspace、Diff 和 Trace；未成功 Ticket 回到 `pending`，Resume 创建新的 DispatchEpoch、Authorization 和 AgentRun 并重新进入 FIFO。
- [ ] Cancel 围栏并保留可审计事实后终止 Session，绝不自动重试、重新排队、reset、checkout、clean 或清理 Worktree。
- [ ] HumanDecision retry 只让失败的同一 Ticket 回到 `pending`，保留已成功 Ticket、失败 Diff 和证据；不能跳过依赖或重置 WorkspaceInputRevision。
- [ ] Report 与 Pause/Cancel 的并发以 SQLite 首次提交为准：先提交的 Report 保留结果，先提交的控制命令使后续 Report 成为 LateReport；重复控制命令具备幂等语义。
- [ ] 旧 Assignment、旧 Lease、旧 Claim、不同 Worker 或重复 Runtime 均不能越过 Fence 改变 Ticket、Change 或 Evidence 权威状态；每个恢复 epoch 至多产生一个有效启动。
- [ ] 使用可控时钟、进程中断和 SQLite 并发测试覆盖 restart、heartbeat race、pause/cancel/retry、late report、重复命令和 Worktree 身份恢复拒绝，并保存可复核 Trace。

# 09-02：确定性调度与唯一 Runtime Claim

**What to build：** 已建立的 Execute 会话从 Canonical Ticket Graph 的 READY frontier 确定性地选出一张 Ticket，形成结构性授权、唯一 Assignment 和 Lease；Worker 可以重放 Pull 但只能由首个有效 Claim 启动一次 Runtime，并在 Assigned Workspace 中进入可观测的运行状态。

**Blocked by：** 顶层 Ticket 06、08 的真实实现与验收证据；本地 Ticket 09-01：显式 Execute 创建并可恢复 Change Worktree。

**Status：** ready-for-agent（仅文档成熟度；顶层 Ticket 06、08 未解除）

- [ ] Ticketize 固化的 CanonicalTicketOrder、Project FIFO 和单 Project 写入槽决定调度；Worker、UUID、墙钟时间、到达顺序和内存队列不能改变选择，队首无 Worker 时不跳过或发放 Lease。
- [ ] DefaultExecutionAuthorizationPolicy.v1 只在 Change/Session/epoch、Workspace 身份、当前 Runnable Ticket、`edit`/`codex` 模式、输入投影、timeout 和 branch 追溯均满足时授权；不满足时按稳定冲突或 `human_required` 分类且不创建 Assignment。
- [ ] Assignment、Lease、AgentRun 和 DispatchEpoch 在权威存储中建立关联；同一 Project 同时至多一个会写 Workspace 的活跃 Assignment，Ticket 只能有一个活跃授权。
- [ ] Worker Pull 可重放并返回同一 Assignment，但不能隐式启动 Runtime；同一 RuntimeClaim 重放同一结果，第二个 Claim、其他 Worker、失效或已围栏 Lease 一律不能启动。
- [ ] 每个 Worker 同时至多持有一个已 Claim 或运行中的 Assignment；Claim 成功后才允许在指定 Workspace 启动 Runtime，Workspace 边界和输入摘要在启动前再次校验。
- [ ] 每份 ExecutionEnvelope 固定 30 分钟 timeout，计时从 Runtime 实际启动开始；输入 Artifact 由 Daemon 有界投影为 UTF-8 指令，Worker 不获得 Artifact 下载权限、物理路径或 DB 连接。
- [ ] 使用 fake Runtime/Worker seam 和 SQLite 并发测试覆盖 FIFO、授权合取、Pull 重放、Claim 竞争、超时起算和脱敏输入，并能查询 `pending`、`assigned` 状态。

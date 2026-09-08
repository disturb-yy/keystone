# 12-03：公开 Planning 与 Canonical Ticket Graph 驱动

> 状态：`ready-for-agent`（仅表示规划成熟度，尚未实现）。父 Ticket：[12 Golden Path E2E Evidence](../../12-golden-path-e2e.md)；规格：[12-golden-path-e2e-spec.md](../spec/12-golden-path-e2e-spec.md)。
>
> 顶层硬阻塞：Ticket 11 的真实 acceptance evidence 未形成前不得开始实现；本子票还依赖 12-02 的受控 Run bootstrap。

**What to build：** 让一个已通过外层预检的 GoldenPathRun 只经公开 Control Plane API/CLI 创建 Project 和固定 ChangeIntent，以真实 Codex 驱动 Understand、Design、Plan、Ticketize，并读取实际形成的 Canonical Ticket Graph 与 Trace。

**Blocked by：** Ticket 11 的真实 acceptance evidence；12-02 受控 GoldenPathRun Bootstrap 与外部预检。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 用预先保存的 Idempotency-Key 调用公开 Project init API，随后只通过公开 Query 读取 Project 与 ProjectInitialized Event。
- 原样提交父 Ticket 冻结的 ChangeIntent；不得为平台、Run、Runtime 或 Graph 增加上下文、改写措辞或创建第二个 Intent。
- 等待 Daemon 真实驱动 Understand、Design、Plan 和 Ticketize；所有 runtime-backed Planning AgentRun 使用真实 Codex，Planning candidate 继续遵守 read-only sandbox。
- 查询公开 Ticket Graph，读取 Daemon 形成的 canonical order、依赖与状态；不得预设 Ticket 数量、Ticket ID、frontier 或 RunnableTicket。
- 在每次公开写入前原子记录 `GoldenPathCommandLedger`，固定 scope、规范请求 digest、expected version、UUIDv7 Idempotency-Key 与 key fingerprint。
- 按固定预算轮询公开 Query；仅在传输中断或明确 temporary unavailable 时原样重送同一 Ledger 条目。

## Authority and Safety Boundaries

- Daemon 是 Project、Change、Artifact、Event、Canonical Ticket Graph 与阶段推进的唯一权威；Runner 不能调用内部 Service、写 SQLite 或从 Worker 自报推导状态。
- `202 Accepted` 仅允许继续轮询。冲突、idempotency conflict、非法状态、`human_required`、真实 AgentRun 失败或预算耗尽立即终止，不产生新 key、新 version 或新逻辑 Command。
- `PlanningReadiness`、Ticket 文档和 graph 创建成功不能被误写为 Execute 授权、Diff、Verify PASS 或 GoldenPathEvidence。

## Acceptance Criteria

- [ ] Project init、Change create、Planning/Ticketize 等公开写入都有完整 Ledger 条目；重送保持请求、expected version 与 Idempotency-Key 完全相同。
- [ ] 固定 ChangeIntent 的 Artifact、Planning 产物、实际 AgentRun/Event 与 Ticket Graph 都可通过公开 Query 关联和观察。
- [ ] 所有 Planning runtime-backed AgentRun 实际使用 Codex；版本探针、fake Runtime、OpenCode 或静态计划均不能作为该 slice 的成功替代。
- [ ] Runner 按阶段预算观察到真实 Canonical Ticket Graph，且不依赖硬编码 Ticket 数量、ID、依赖图或客户端 frontier 推导。
- [ ] 任一不可恢复结果保留 Run 现场与安全失败摘要，不越界调用 Execute、Verify、Commit 或 Final Verify。

## Out of Scope

- Ticket 的 Worktree 创建、ExecutionAuthorization、Execute、Diff、Verify、Commit、Final Verify、CandidateRevision、服务检查、Dashboard 浏览器观察或成功 Evidence 发布。
- 修改 Ticket 08 的图模型、Ticket 07 的 Planning 业务规则、Daemon 权威边界或 Worker Protocol。

## Verification

- 使用真实临时 Git demo、公开 API/CLI 与真实 Codex Planning 路径验证 Project→Change→Ticketize→Ticket Graph 的可观察因果链。
- 覆盖同 key 重送、`202` 轮询、冲突/失败停止及敏感字段不进入安全摘要的外部行为。


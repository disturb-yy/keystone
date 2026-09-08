# 07 — Understand, Design and Plan

> **状态：** 代码已实现并进入验证；顶层 `BLOCKED_BY: 06` 仍保留，Ticket 06 的真实 Codex 与原生 Windows 证据缺口尚未由本 Ticket 解除。

- 里程碑：M5
- `BLOCKED_BY`：06
- 交付类型：Planning 生命周期纵切

## 文档入口

- [共同实施规格](07-understand-design-plan/spec/07-understand-design-plan-spec.md)
- [实施子票目录](07-understand-design-plan/tickets/)

## 目标

实现 Understand、Design、Plan 三个真实 Stage Strategy，使 active Change 从 Intent 自动生成受 schema 校验、可复盘的阶段 Artifact。阶段严格串行，固定使用 Change 的 `base_revision` 和有界 `ProjectContext.v1`；Plan 成功后停在 `Ticketize`/`active`，由 Ticket 08 继续负责 Ticketize。

## 已冻结边界

- 不新增公开 `planning start` endpoint；Daemon 创建 Change 后触发 eligibility，启动时从耐久状态扫描可恢复 Change。
- Worker/Runtime 只回传候选 payload 和执行事实；Daemon/Coordinator 验证 Contract 后才决定 AgentRun、Artifact、Event 和 Change 状态。
- 每个 Stage 使用一个 AgentRun，失败、schema invalid、timeout、revision mismatch、Pause、Cancel 或 Human Required 都不能触发隐式下游 Assignment；V1 不自动 retry。
- Run 使用固定 revision 的隔离临时 Snapshot，不修改原始 Repository，不创建生产 Change Worktree。
- `internal/planning` 负责 Planning Contract/Context/Validator、Strategy、Decoder 和 Coordinator port；Work authority 仍在 `internal/work`，SQLite/Migration 仍在 `workstore`，Snapshot adapter 复用现有 `repository`。

## 不包含

- Ticket Draft 生成、Canonical Ticket Graph 和 Ticketize Assignment。
- Repository 全量分析、CodeMap 自动生成、概念空间构建或完整 Risk/Policy/Gate 模型。
- 让 Runtime/Worker 直接修改 Lifecycle、Decision、Event、Database、原始 Repository 或生产 Worktree。
- 重复实现 Ticket 06 的运行边界；本 Ticket 只增加隔离只读 Planning 所需的 `planning_candidate` 采集和权威隔离 seam。

## 当前 checkout 边界

当前 `internal/planning` 已实现 Contract、strict decoder/validator、三阶段 Strategy 和 Coordinator；`workstore` schema v5 保存 Planning metadata、candidate、link 与阶段提交，Daemon 在进入 readiness 前完成首次耐久恢复扫描，并通过固定 revision Snapshot 下发只读 Assignment。当前实现不新增公开 planning start API，也不实现 Ticketize Graph。Ticket 06 的真实 Codex smoke 与原生 Windows 运行证据仍需单独补齐。

## 验收条件

- 新 Change 可依次产生 Understanding、Design、Plan Artifact，且每份 Artifact 有可查询摘要、输入 revision、AgentRun、输入关联和原始日志引用。
- 结构化输出不满足 schema、大小、路径或 revision 约束时，Change 不推进并保留失败证据。
- 阶段失败、Pause、Cancel、Human Required、旧 attempt 和晚到结果遵循 fencing，不覆盖或复活权威状态。
- 策略、Decoder 和 Validator 在不启动 HTTP、SQLite 或真实 Runtime 的情况下拥有核心单元测试。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

Plan 只是 Artifact，不是可执行事实。是否执行 Ticket 和如何调度由后续 Ticket 的 Canonical Graph 与授权机制决定。

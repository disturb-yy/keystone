# 07 — Understand, Design and Plan

- 里程碑：M5
- `BLOCKED_BY`：06
- 交付类型：Planning 生命周期纵切

## 目标

实现 Understand、Design、Plan 三个真实 Stage Strategy，使 Change 从 Intent 自动生成受 schema 校验、可复盘的阶段 Artifact。

## 范围

- 在 `internal/planning` 实现 Stage Strategy、输入/输出 Artifact Contract 和 Runtime 调用编排。
- 每个 Stage 以固定的上游 Artifact、Project 上下文与 base revision 为输入，生成不可变的结构化输出与原始 Runtime 日志。
- 默认低风险 demo Change 在 Ticketize 前自动推进；出现失败、暂停、取消或人工要求时停止推进。
- Worker/Runtime 只回传执行事实；Lifecycle Coordinator 在验证 Artifact Contract 后决定状态推进。
- Run 默认只读，不修改原始 Repository 或创建 Worktree。

## 不包含

- Ticket Draft 生成与 Canonical Ticket Graph。
- Repository 全量分析、CodeMap 自动生成或概念空间构建。
- 让 Runtime 直接修改 Lifecycle、Decision 或 Database。

## 验收条件

- 新 Change 可依次产生 Understanding、Design、Plan Artifact，且每份 Artifact 有可查询摘要、输入 revision、AgentRun 与原始日志引用。
- 结构化输出不满足 schema 时，Change 不会推进并保留失败证据。
- Stage 的失败、Pause、Cancel、Human Required 不会触发隐式下游 Assignment。
- 策略在不启动 HTTP、SQLite 或真实 Runtime 的情况下拥有核心单元测试。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

## 实现边界

Plan 只是 Artifact，不是可执行事实。是否执行 Ticket 和如何调度由后续 Ticket 的 Canonical Graph 与授权机制决定。

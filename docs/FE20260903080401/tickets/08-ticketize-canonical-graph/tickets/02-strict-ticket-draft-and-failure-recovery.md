# 08-02：严格 Ticket Draft 与失败恢复

**What to build：** 在 08-01 的成功闭环上收紧 StructuredTicketDraft decoder/validator，并将 generator_failed、draft_invalid 与 ticketize_unavailable 区分为可恢复、可审计且不伪造权威图的结果。

**Blocked by：** 08-01、顶层 Ticket 07。必须以 08-01 已存在的成功提交/查询 seam 为基础；不得为实现失败路径另建第二套 Graph、Event 或 Artifact ledger。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 实现严格单对象 UTF-8 JSON decoder，拒绝未知字段、重复字段、缺失字段、多个 JSON 值、尾随内容、无效编码和超限 body；不得依赖默认 decoder 对重复键的静默覆盖。
- 实现 Draft 的完整确定性规则：schema version、1 至 64 张 Ticket、GenerationKey 格式和唯一性、非空 title/scope、1 至 32 条去重 Acceptance Criteria、同 Draft BLOCKED_BY 引用、禁止自依赖和无环。
- 让 Validator 返回稳定的领域分类和字段上下文；不得泄露 JSON parser 实现细节、SQL 错误、绝对路径、secret 或未界定原始日志。
- 将 raw Candidate 作为 Ticketize AgentRun output ArtifactRef 保存；当 Draft 无效时额外保存有界 Failure Artifact，完成该 run 为失败并让 Change 进入 human_required。
- 将 generator_failed 作为可审计失败处理：保存有界 Failure Artifact，完成相关 AgentRun 为失败，Change 进入 human_required，不产生 Graph。
- 将 ticketize_unavailable 限定为 Artifact/SQLite 在任何权威成功或失败事实写入前的暂时不可用；它不完成 AgentRun、不创建 Failure Artifact、不推进 Change，允许同一 Report 重送。
- 接入既有 HumanDecision/retry 语义：人工明确恢复后创建新的 TicketGeneration，复用固定的已验证输入；旧失败 Candidate 与 Failure Artifact 只作 Trace，不能成为成功图。
- 保持 08-01 的合法成功、查询排序和原子提交语义不变。

## Frozen bounds

| 项目 | 规则 |
| --- | --- |
| Candidate body | 单一 UTF-8 JSON object，最大 1 MiB |
| tickets | 1 至 64 项 |
| generation_key | 小写 ASCII，匹配 [a-z0-9][a-z0-9._-]{0,63}，同 Draft 唯一 |
| title | trim 后非空，最多 256 个 Unicode rune |
| scope | trim 后非空，最多 8 KiB |
| acceptance_criteria | 每 Ticket 1 至 32 条；每项 trim 后非空、最多 8 KiB、同 Ticket 不重复 |
| blocked_by | 每 Ticket 0 至 63 项；仅引用同 Draft key、不可重复、不可自引用、整体无环 |

所有边界都是 validator 的可测试行为；Prompt、Generator 说明或数据库错误不能替代前置校验。

## Acceptance

- 表驱动单元测试覆盖合法 Draft，以及所有字段/关系边界、未知字段、重复字段、尾随值、无效 UTF-8、超限、空值、缺失依赖、自依赖、重复依赖和环。
- draft_invalid 保留 Candidate output 与有界 Failure Artifact，AgentRun 为失败，Change 为 human_required，且无 Canonical Graph、TicketGraphCreated 或 StageAdvanced。
- generator_failed 同样有稳定失败分类和 Failure Artifact，但不伪造 Draft 解析结果或部分图。
- ticketize_unavailable 不写入业务失败或成功事实；在依赖恢复后重送同一 Report 能走 08-01 的正常成功路径。
- 人工 retry 创建新的 TicketGeneration/AgentRun；旧失败产物可查询但不能被直接提交、覆盖或解释为成功。
- 合法 Candidate 的成功路径仍在 08-01 的单一原子 completion 中，不能因为增加失败处理而发生两次 stage 推进或两张图。

## Out of scope

- 暂停/取消/失效 Lease 的 fenced candidate、重启和并发 completion 围栏（08-03）。
- 修改已成功 Canonical Graph、图版本、自动 retry、Graph 编辑 UI 或外部 Tracker。
- Scheduler、执行状态、Worker Assignment、Worktree、真实 Runtime/Codex smoke。

## Verification

~~~bash
go test ./internal/planning/...
go test ./internal/work/...
go test ./internal/infrastructure/workstore/...
go test ./internal/daemon/...
go test ./...
go vet ./...
make build
git diff --check
~~~

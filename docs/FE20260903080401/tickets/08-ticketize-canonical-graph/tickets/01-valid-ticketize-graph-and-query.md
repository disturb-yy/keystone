# 08-01：合法 Ticketize Graph 与只读查询

**What to build：** 建立从已验证 Plan 到合法 StructuredTicketDraft、Canonical Ticket Graph、TicketGraphCreated Event、Change 进入 Execute，以及 HTTP/CLI 只读查询的首个纵向闭环。该闭环使用受控 fake TicketGenerator 证明权责和事务，不依赖真实 Codex。

**Blocked by：** 顶层 Ticket 07 的真实实现与验收证据。开始前还必须核对当前 Change、Plan Artifact、AgentRun、Event、Worker/Runtime 和 Workstore 的实际 API；Ticket 07 的规格或本子票的 ready-for-agent 不解除该阻塞。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 在 internal/planning 定义最小 TicketGenerator port、TicketGeneration 输入、合法 StructuredTicketDraft 类型和只产生 candidate 的 fake seam。
- 使用同一 Change 已成功 Plan AgentRun 的原始 output ArtifactRef、固定 base_revision 和有界 ProjectContext.v1 构造 Ticketize 输入；Ticketize AgentRun 的 input 必须是 role 为 input 的同内容 ArtifactRef，不复用 Plan output 的 role。
- 在 internal/work/domain 定义 CanonicalTicket、TicketDependency、Graph 的最小有效不变量，并让 Daemon 以 UUIDv7 分配 Graph/Ticket 身份。
- 在 internal/work 提供专用成功 completion 编排，不使用会单独推进 stage 的通用 AgentRun completion。
- 在 Workstore 的实现时下一个可用 additive Migration 中建立成功路径所需的 Graph、Ticket、Acceptance Criteria、Dependency 与来源关系，并加入 Change 单图唯一性、同图来源和最小外键完整性。
- 扩展现有 Event ledger 以表达 TicketGraphCreated，遵循既有 SQLite child-first 迁移范式，保留历史 Event、Artifact 关联、触发器、索引和 AgentRunReportLate 行为。
- 在同一 authority transaction 内完成：当前 Ticketize 条件确认、Graph/Tickets/Dependencies 写入、TicketGraphCreated、当前 AgentRun 成功、StageAdvanced、Change 从 Ticketize 到 Execute。
- 提供 GET /v1/changes/{change_id}/ticket-graph，返回稳定排序的 Graph、Tickets、Acceptance Criteria、Dependencies 和 StructuralFrontier。
- 提供只读 CLI：

~~~text
keystone change ticket-graph CHANGE_ID
~~~

  它只能查询已运行 Daemon；不得隐式启动 Daemon、Worker、Generator 或任何写入流程。

## Acceptance

- 给定一个处于 Ticketize/active 的 Change、同 Change 的已验证 Plan 和合法 fake Generator Candidate，Daemon 能形成且只能形成一个可查询 Canonical Ticket Graph。
- 查询响应包含 Graph ID、Change/Project ID、base revision、原始 Plan 与 Draft ArtifactRef、Ticketize AgentRun、generator metadata、按 ordinal 稳定排序的 Ticket/criteria/依赖，以及由无 blocker Ticket 派生的 StructuralFrontier。
- Candidate 中的 GenerationKey 只用于 Draft 内解析，不出现在公开 ReadModel、CLI 输出或 Canonical Ticket identity 中。
- 成功后 Graph、TicketGraphCreated、当前 AgentRun success、StageAdvanced 和 Change Execute 在一次事务中可观察；任一写入失败不能留下“已 Execute 但无图”或“有图但未完成”的半终态。
- 不存在 Change 返回 404 change_not_found；存在 Change 但没有图返回 404 ticket_graph_not_found；非法 ID 返回 400 invalid_request。
- CLI 参数错误、连接失败和查询错误保持安全错误边界，且测试证明它没有启动进程或写入 SQLite。
- Migration 升级后已有 Event ledger 数据仍可读，AgentRunReportLate 的既有语义和关联不丢失。

## Out of scope

- 完整严格 JSON decoder、重复字段识别、所有非法 Draft 分类、Failure Artifact 和人工恢复语义（08-02）。
- Pause/Cancel/Resume、Lease/attempt 失效、重启/并发 completion 的完整围栏，以及不可变 trigger、递归 CTE 环检测等数据库纵深防线（08-03）。
- Scheduler、RunnableTicket、Ticket status、ExecutionAuthorization、Assignment、Worktree、Diff 和真实 Codex。

## Verification

~~~bash
go test ./internal/planning/...
go test ./internal/work/...
go test ./internal/infrastructure/workstore/...
go test ./internal/daemon/...
go test ./cmd/keystone/...
go test ./...
go vet ./...
make build
git diff --check
~~~

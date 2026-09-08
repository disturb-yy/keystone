# 08：Ticketize 与 Canonical Ticket Graph 实施规格

> 状态：规划规格已对齐，尚未实现。顶层 Ticket 08 的 BLOCKED_BY: 07 仍然有效；本规格和子 Ticket 的 ready-for-agent 只表示实施契约已足够明确，不表示当前 checkout 已具备 Ticketize、Canonical Ticket Graph、查询接口或 SQLite Schema。
>
> 用途：把已验证的 Plan Artifact 转换为可审计、不可替换、由 Daemon 权威持有的 Canonical Ticket Graph，并为后续 Ticket 09 的执行调度提供结构事实，不提前实现执行授权或 Scheduler。

## Problem Statement

Ticket 07 的目标是让一个 Change 形成可验证的 Plan Artifact，并把 Change 停留在 Ticketize/active。当前工作树没有 Ticketize 的 Generator、Draft 校验、Canonical Ticket Graph、相关持久化或只读查询。若把 Generator 的自然语言或 JSON 输出直接作为 Ticket，或让 Worker、CLI、HTTP Handler 直接写数据库，将无法在重复 Report、并发恢复、暂停、取消、持久化故障和 Daemon 重启后回答以下问题：

- 哪一份已验证 Plan 是当前图的唯一来源；
- 哪一次 Ticketize AgentRun 和哪份候选 Draft 形成了图；
- 某个依赖是否跨图、重复、指向不存在 Ticket 或形成环；
- 图、创建事件、AgentRun 成功与 Change 进入 Execute 是否是同一个权威事实；
- 失败、暂时不可用与被围栏候选应分别如何恢复，且不会误生成部分图。

Ticket 08 必须建立一个明确的信任边界：TicketGenerator 只给出有界候选；Planning 校验候选；Work/Daemon 在事务中确认事实；Workstore 持久化不可变图；Client 只查询结果。成功生成的图是后续执行的结构输入，而不是执行授权、READY 状态或 Worker Assignment。

## Solution

为每个处于 Ticketize/active 的 Change 引入一次受控的 TicketGeneration。它固定绑定该 Change 的已验证 Plan、base_revision 和有界 ProjectContext.v1，并通过既有 Worker/Runtime seam 执行受限的 TicketGenerator。Generator 输出严格的 StructuredTicketDraft；它只有在 Deterministic Validator 通过、当前执行权仍有效且 Work authority 原子提交成功后，才会成为 Canonical Ticket Graph。

成功提交在同一个 SQLite authority transaction 内完成：

1. 确认 Change 仍为 active 且 stage 为 Ticketize，Ticketize AgentRun、Plan、base_revision 和 ProjectContext 均与本次 Generation 匹配，且 Change 尚无图。
2. 校验 Candidate Draft 的 JSON、边界、依赖引用和无环性。
3. 持久化来源 Plan、Candidate Draft、Graph、CanonicalTicket、Acceptance Criteria 与 BLOCKED_BY 关系。
4. 写入直接关联 Graph 的 TicketGraphCreated Event，完成当前 AgentRun，写入 StageAdvanced，并把 Change 推进到 Execute。

任何一步失败都不能留下可被读取为成功的半完成图。Generator 失败和 Draft 无效形成可审计的失败证据并进入 human_required；在写入任何权威成功或失败事实前遇到 Artifact/SQLite 暂时不可用时，返回可重送的 unavailable；暂停、取消、过期 Lease 或旧 Report 造成的 fenced candidate 只保留 Trace，永远不能在 Resume 后复用。

Graph 是每个 Change 仅有一次的不可变成功事实。若业务需要改变已成功图，V1 创建新的 Change；不编辑、替换、版本化或重新生成现有图。

## User Stories

1. 作为 Change 的发起者，我希望一个已验证 Plan 在受控 Ticketize 后形成可查询的 Canonical Ticket Graph，以便后续执行有稳定的结构依据，而不是依赖 Runtime 的自由文本。

2. 作为 Daemon，我希望为每次 TicketGeneration 固定 Change、Plan、base_revision 和 ProjectContext.v1，以便候选不能在运行中偷换输入或从新的仓库状态获得未审计内容。

3. 作为 Generator 的调用方，我希望 Generator 只返回 StructuredTicketDraft candidate，以便它不能自称成功、分配 Ticket ID、推进 Change 或直接写入权威数据库。

4. 作为实施者，我希望 Draft Schema 具有严格、可测试的大小和关系边界，以便非法 JSON、未知字段、重复字段、空标题、失效依赖和环在进入持久化前被确定性拒绝。

5. 作为审计者，我希望每张 CanonicalTicket 都能追溯到形成它的 Graph、原始 Plan output、Ticketize Candidate Draft 和 Ticketize AgentRun，以便可重放事实链而不把候选误认为权威状态。

6. 作为后续 Scheduler 的消费者，我希望能从 Canonical Graph 派生没有 blocker 的 StructuralFrontier，以便获得结构性起点，同时不会把它误用为执行授权或 RunnableTicket。

7. 作为 Change 的发起者，我希望重复提交同一 Candidate Report 时得到与首次成功一致的结果，以便网络重试不会生成第二张图或覆盖历史。

8. 作为并发恢复流程，我希望两个 Ticketize completion 竞争同一 Change 时最多一个能成功，以便 Graph、Event、AgentRun 与 Change stage 始终一致。

9. 作为人工恢复者，我希望 Generator 运行失败或 Draft 无效时能查看有界 Failure Artifact 和稳定失败分类，以便决定是否以新 TicketGeneration 重试，而不是猜测数据库状态。

10. 作为 Report 调用方，我希望在 Artifact 或 SQLite 尚未写入任何权威成功或失败事实时收到暂时 unavailable，以便安全重送同一个 Report，而不会被错误地标记为业务失败。

11. 作为暂停 Change 的操作者，我希望暂停、取消、Lease 失效或晚到 Report 先于提交时会围栏该 Candidate，以便旧运行不能在稍后偷偷生成 Graph。

12. 作为恢复 Change 的操作者，我希望 Resume 创建新的 TicketGeneration，而不复用已围栏 Candidate，以便新的权威图只来自仍有效的 AgentRun 和控制状态。

13. 作为 API Client，我希望能通过一个只读端点获取 Graph、Ticket、Acceptance Criteria、依赖和 StructuralFrontier 的稳定排序快照，以便展示或检查结构，而不需要访问 Keystone SQLite。

14. 作为 CLI 用户，我希望通过只读命令查询指定 Change 的 Graph，以便本地诊断不会启动 Daemon、提交写操作或调用 Worker。

15. 作为数据库维护者，我希望数据库本身拒绝跨图依赖、自依赖、重复 Graph、更新/删除已提交图和依赖闭环，以便应用层 bug 或恢复竞态不能破坏已提交事实。

16. 作为迁移维护者，我希望新增 Schema 使用下一个可用的 additive Migration，并完整保留既有 Event ledger、Artifact 关联、触发器和 AgentRunReportLate 语义，以便升级不会改写历史或触发 checksum drift。

17. 作为 Ticket 09 的实现者，我希望 Ticket 08 明确不产出 Ticket 状态、Scheduler、ExecutionAuthorization、Assignment 或 Worktree，以便结构图与执行图的职责不会混淆。

18. 作为项目维护者，我希望所有完成证据同时覆盖纯校验、SQLite 约束、Daemon/API/CLI seam、恢复竞态和根级构建，以便规划文档不被误当作运行行为证据。

## Implementation Decisions

### 事实边界与术语

- 本规格定义 Ticket 08 的目标实施契约，不解除顶层 Ticket 07 的阻塞。Ticket 07 的规格、局部导航文件或 fake Runtime 不能替代其真实实现和验收证据。
- 文档目录中的 ImplementationTicket 与 Change 内的 CanonicalTicket 是不同概念。ImplementationTicket 组织 Keystone 自身开发；CanonicalTicket 是 Daemon 为一个 Change 持有的后续执行切片。
- StructuredTicketDraft、TicketGenerationCandidate、TicketGenerationFailure、TicketizeUnavailable、TicketizeFenced、StructuralFrontier、TicketGraphCommitIdentity 的准确语义以 CONTEXT.md 为准；不可在实现中把它们收敛为同一个“失败”或“Ticket”类型。
- ADR-0029、ADR-0030 与 ADR-0031 分别冻结了单 Change 图的不可替换性、成功提交的原子边界和围栏候选不得复用的恢复规则。它们是实现约束，不是现有代码证据。

### 权责与调用边界

| 区域 | Ticket 08 的责任 | 明确不负责 |
| --- | --- | --- |
| internal/planning | TicketGenerator port、StructuredTicketDraft、严格 decoder/validator、candidate 分类 | Canonical Graph、Change 推进、SQL、HTTP Handler |
| internal/work/domain | CanonicalTicket、TicketDependency、Graph 不变量和稳定领域错误 | JSON parser、SQLite、Worker/Runtime |
| internal/work | Ticketize orchestration、提交前条件确认、专用 atomic completion port | SQL 细节、HTTP 参数解析、Runtime 具体实现 |
| internal/infrastructure/workstore | additive migration、Graph/Event/AgentRun 原子持久化、数据库不变量 | Domain 规则替代、HTTP/CLI |
| internal/daemon | 依赖装配、受控 Ticketize completion 接线、只读 HTTP Handler | 直接写 SQL、直接信任 Generator |
| contracts/controlplane | Graph 查询 DTO、稳定 JSON 和错误边界 | Domain Entity、业务校验、数据库访问 |
| cmd/keystone | 只读 Graph 查询命令与输出 | 启动 Daemon、触发 Ticketize、写入数据库 |
| Worker/Runtime | 受授权运行 Generator 并回传候选/日志 | Change、Graph、Gate、依赖图或恢复决策权威 |

TicketGenerator 的运行输入是已验证 Plan、固定 base_revision、有界 ProjectContext.v1 和必要的 Generator 配置。若 Runtime 必须读取文件，使用 Ticket 07 定义的固定 revision、隔离且只读的临时 Snapshot；不得读取或修改原始 Repository 工作树，也不得创建 Ticket 09 的生产 Change Worktree。Generator 的具体 prompt 或 ToTickets-inspired 实现可替换，但不能改变该边界。

### 实施子票图

~~~text
顶层 Ticket 07 的真实实现与验收
                 ↓
08-01 合法 Graph、原子成功提交与只读查询
                 ↓
08-02 严格 Draft、失败证据与人工恢复
                 ↓
08-03 围栏、重放、重启与数据库纵深不变量
                 ↓
08-04 集成验收与导航
~~~

每张子票都是前一张可观察行为的纵向扩展，而不是按 Domain、HTTP、SQLite 或 CLI 横向分层。08-01 已包含最小端到端成功事务和查询；后续子票只收紧失败、恢复和防御性边界，不得引入第二条成功提交链。

### StructuredTicketDraft Schema 与确定性校验

Candidate 必须是单一 UTF-8 JSON object，语义形状如下：

~~~json
{
  "schema_version": "keystone.ticket-draft.v1",
  "tickets": [
    {
      "generation_key": "validate-draft",
      "title": "校验候选 Draft",
      "scope": "说明本 Ticket 的受控实现范围。",
      "acceptance_criteria": [
        "非法 Draft 被确定性拒绝。"
      ],
      "blocked_by": []
    }
  ]
}
~~~

Decoder 必须拒绝未知字段、重复字段、缺失必需字段、多个 JSON 值、尾随内容、无效 UTF-8、超出 body 上限和不匹配的 schema_version。不得依赖默认 JSON decoder 静默覆盖重复键。

| 字段或关系 | 规则 |
| --- | --- |
| Draft body | 最大 1 MiB；必须是单一 object |
| tickets | 1 至 64 项；按 Draft 顺序形成稳定 ordinal |
| generation_key | 仅在同一 Draft 中解析；小写 ASCII，匹配 [a-z0-9][a-z0-9._-]{0,63}，全 Draft 唯一 |
| title | trim 后非空，最多 256 个 Unicode rune |
| scope | trim 后非空，最多 8 KiB |
| acceptance_criteria | 每张 1 至 32 条；每条 trim 后非空、最多 8 KiB，同一 Ticket 文本不得重复 |
| blocked_by | 每张 0 至 63 项；每项为同 Draft 的 generation_key，不能重复、不能指向自身 |
| dependency graph | 只允许 BLOCKED_BY；必须无环，不接受跨 Draft、跨 Change 或反向关系 |

Validator 必须在内存中先生成面向领域的 Canonical graph candidate，并返回稳定分类与字段上下文。它不分配 Canonical Graph/Ticket ID，不持久化，也不把 GenerationKey 暴露为公共 Canonical identity。Daemon 以 UUIDv7 分配 Graph 和 CanonicalTicket ID；Draft ordinal 仅用于稳定排序与审计。

### Artifact、AgentRun 与失败分类

- Graph 的 Plan 来源必须是该 Change 成功 Plan AgentRun 的原始 output ArtifactRef。
- 因既有 ArtifactRef role 约束，Ticketize AgentRun 的 input 使用一个内容相同但 role 为 input 的 ArtifactRef；Graph 仍关联原始 Plan output，不能为了复用而把 Plan output 直接作为 Ticketize input。
- TicketGenerationCandidate 以 Ticketize AgentRun 的 output ArtifactRef 保存；成功 Graph 与该 Candidate、原始 Plan、AgentRun 建立可查询关联。
- generator_failed 与 draft_invalid 均保存有界 Failure Artifact，并完成该 AgentRun 为失败、使 Change 进入 human_required。draft_invalid 同时保留原始 Candidate output 与 Failure Artifact，便于审计。
- ticketize_unavailable 表示 Artifact 或 SQLite 在任何权威成功或失败事实写入前暂时不可用；不得完成 AgentRun、不得推进 Change、不得写入 Failure Artifact 或人工恢复记录。同一 Report 可重送。
- ticketize_fenced 表示 Pause、Cancel、Lease/attempt 失效或晚到结果已使 Candidate 失去提交资格；它可保留 Trace，但不创建 Graph，不改写 Change，也不被解释为 Draft 无效。

失败 Artifact 不应包含密钥、绝对宿主机路径、完整未界定日志或原始 SQLite 错误。它至少表达稳定分类、面向人的安全摘要、关联 AgentRun/Candidate 摘要和可复盘原因。

### 成功提交、幂等与围栏

不得复用通用 AgentRun completion 作为 Ticketize 成功路径，因为通用路径无法同时保证 Graph、来源、Event、Change stage 和特殊 Artifact 角色的一致性。Work application 提供专用 Ticketize completion port，其唯一成功事务按以下顺序确认和写入：

1. 锁定或以等价条件确认 Change 仍为 active/Ticketize，当前 Ticketize AgentRun 尚有效，Plan/Revision/ProjectContext 与本次 Generation 一致，且尚不存在 Graph。
2. 完成严格 Draft 校验，生成 Daemon-owned UUIDv7 图与 Ticket 身份，并确认 Candidate/Plan Artifact 来源满足同一 Change/Run 约束。
3. 写入 Graph、Tickets、Acceptance Criteria 和同图 BLOCKED_BY 边；写入直接关联 Graph 的 TicketGraphCreated Event。
4. 在同一事务中完成当前 AgentRun、写入 StageAdvanced，并将 Change 从 Ticketize 推进到 Execute。

提交身份是 TicketizeAgentRunID 与 Candidate Artifact 内容身份的内部组合，不新增 Client Idempotency-Key、通用 receipt 或可编辑 Graph version。重放请求先按 Change 查询既有 Graph：若既有 Graph 的 AgentRun 和 Candidate identity 均相同，返回既有权威结果；不相同则按冲突或 fenced 分类处理，绝不覆盖首次成功。

若 Pause、Cancel、Lease 失效、旧 attempt、旧 Worker 或晚到 Report 在图提交前已成为权威事实，Candidate 不可提交。Cancel 不恢复；Resume 重新满足条件后创建新的 TicketGeneration，不能复用旧 Candidate。若 Graph 已成功提交，后续暂停、取消、重复或晚到报告不能删除、替换或重建它。

### 持久化与数据库不变量

Workstore 继续拥有业务 SQLite Migration。Migration 使用实现时下一个可用版本并且只追加；不得修改已应用 Migration 的 SQL 或 checksum。建议的规范化表和不可变量如下：

| 持久化事实 | 最小字段与约束 |
| --- | --- |
| t_ticket_graphs | graph_id、project_id、change_id、base_revision、ticketize_agent_run_id、原始 Plan ArtifactRef、Draft ArtifactRef、generator name/version、created_at；change_id 唯一；另有 graph_id/change_id/project_id 复合唯一键供 Event 参照 |
| t_tickets | ticket_id、graph_id、Draft ordinal、title、scope、generation_key；同图 generation_key 唯一、同图 ordinal 唯一、ticket_id/graph_id 复合唯一 |
| t_ticket_acceptance_criteria | ticket_id、ordinal、text；同 Ticket ordinal 唯一，同 Ticket 文本唯一 |
| t_ticket_dependencies | graph_id、dependent_ticket_id、blocker_ticket_id；两个 ticket_id/graph_id 复合外键确保两端同图；CHECK 禁止自身依赖 |

Graph、Ticket、Acceptance Criterion 和 Dependency 一经提交均禁止 update/delete。StructuralFrontier 由“同图中没有 blocker 的 Ticket”在查询时稳定派生，不建立会漂移的权威状态表。Graph 的子项外键可在一个提交事务中 deferred，使 Graph 写入 trigger 能在最终状态验证至少 1 至 64 张 Ticket、每张 1 至 32 条 Acceptance Criteria、Plan/Draft/Run 来源一致；应用层 validator 仍是第一道错误边界。

数据库必须再用递归 CTE 或等价机制拒绝环，防止并发或应用 bug 绕过内存校验。Event ledger 需要添加 TicketGraphCreated 枚举时，遵循当前 SQLite 的 child-first rename/recreate/复制/rebuild 迁移方式：先处理 Event Artifact 子表，再重建 Event 父表、触发器和索引，保留既有 Event、Artifact 关联及 AgentRunReportLate 语义。为避免 Graph/Event 循环外键，使用单向 t_project_events.ticket_graph_id 指向 t_ticket_graphs 的 graph_id/change_id/project_id 复合键；Graph 不保存 created_event_id。

### 查询与 CLI Contract

Control Plane 仅提供只读：

~~~text
GET /v1/changes/{change_id}/ticket-graph
~~~

成功响应是 TicketGraphReadModel，至少包含：

| 层级 | 字段 |
| --- | --- |
| graph | graph_id、change_id、project_id、base_revision、原始 Plan ArtifactRef、Draft ArtifactRef、ticketize_agent_run_id、generator name/version、created_at |
| tickets | ticket_id、ordinal、title、scope、acceptance_criteria |
| dependencies | dependent_ticket_id、blocker_ticket_id |
| structural_frontier | CanonicalTicket ID 列表 |

Graph、Ticket、criteria、dependency 与 StructuralFrontier 均按稳定 canonical 顺序返回；GenerationKey、内部 commit identity、scheduler 状态、Ticket execution 状态、Worker lease、Assignment、风险或授权字段不得出现。无效 Change ID 返回 400 invalid_request；不存在 Change 返回 404 change_not_found；存在 Change 但未形成图返回 404 ticket_graph_not_found。错误继续使用既有安全 ErrorEnvelope。

CLI 命令固定为：

~~~text
keystone change ticket-graph CHANGE_ID
~~~

它只能发现并查询已运行的 Daemon，不得启动 Daemon、触发 Ticketize、调用 Worker 或写入任何数据库记录。输出应稳定表达与 HTTP ReadModel 等价的结构或安全的文本摘要。

### 明确不提前引入的执行语义

StructuralFrontier 只是无直接 blocker 的结构节点。Ticket 08 不引入 RunnableTicket、Ticket status、ExecutionAuthorization、Scheduler、Execution DAG、ProjectExecutionSlot、Worktree、Assignment、Diff 或任何改变 Repository 的 Runtime 权限。这些属于 Ticket 09 及后续 Ticket；TicketizeCompletion 自动将 Change 放入 Execute 只表示图已存在，不表示任何 Ticket 可执行。

## Testing Decisions

| 测试层级 | 必须证明的事实 |
| --- | --- |
| Planning/domain 单元测试 | Strict JSON 解码、UTF-8/尾随值/未知与重复字段、边界、GenerationKey、标题/范围/AC、缺失依赖、自依赖和环的确定性拒绝；合法 Draft 的稳定 ordinal 和 StructuralFrontier 推导 |
| Work application 测试 | 受控 fake TicketGenerator 只能产生 Candidate；成功路径只调用专用 atomic completion；同 Candidate 重放返回既有 Graph；不同 Candidate、旧 run、旧 attempt 和已有图不能覆盖首个事实 |
| Workstore 集成测试 | additive Migration 升级、旧 Event/Artifact 数据保留、AgentRunReportLate 保留、所有外键/唯一/CHECK/不可变 trigger、跨图边、自依赖、递归环和部分写入均被拒绝；deferred 约束最终仍正确 |
| Daemon/Contract 测试 | GET 的 DTO shape、稳定排序、400/404 错误分类、无 Graph/未知 Change 区别和无 GenerationKey/执行状态泄漏 |
| CLI 测试 | keystone change ticket-graph CHANGE_ID 只读、参数校验、连接既有 Daemon、不隐式启动 Daemon 或 Worker |
| 恢复与竞态测试 | Pause、Cancel、Resume、Lease/attempt 失效、重复/晚到 Report、并发 completion、重启恢复、generator_failed、draft_invalid、ticketize_unavailable 和重送的差异化行为 |

实现完成后至少运行：

~~~bash
go test ./...
go vet ./...
make build
git diff --check
~~~

受限 fake Runtime/Generator 是 Ticket 08 的正确 seam 测试；真实 Codex、Worker 平台 smoke、跨平台 Runtime 行为和完整 Golden Path 不由本 Ticket 取代。若 Ticket 07 或 Ticket 06 的实际验收尚缺失，必须报告确切阻塞证据。

## Out of Scope

- 高级依赖类型、跨 Change 依赖、Effective Graph、图数据库、图编辑、Graph versioning 或已成功图的重新生成。
- Ticket status、RunnableTicket、Scheduler、ExecutionAuthorization、Worker Assignment、ProjectExecutionSlot、Worktree、执行 DAG、Runtime 修改代码、Diff、Verify、Commit 或 Integrate Ready。
- Dashboard 的 Graph 可视化、外部 Issue Tracker 同步、外部 tracker receipt 或面向第三方的 Ticket 发布。
- 真实 Codex/OpenCode smoke、远程 Worker、Worker pool、消息队列、完整 OS sandbox、自动 retry 和完整 Risk/Policy/Gate 模型。
- 把 Repository 全量源码、绝对路径、凭据、未授权日志或任意运行对象带入 ProjectContext.v1、Candidate、Failure Artifact 或公开查询。

## Further Notes

- 当前仓库没有已配置的外部 Issue Tracker，因此本规格与实施子票写入既有的版本化本地路径。若需要同步到外部 tracker，应先执行 /setup-matt-pocock-skills，而不是在 Ticket 08 内临时发明发布流程。
- 顶层 Ticket 08 仍硬阻塞于 Ticket 07。开始任一实施子票前，必须重新检查当前 checkout 中 Ticket 07 的真实实现和验收证据，以及各目标 package 的 AGENTS.md、INDEX.md、源码和测试。
- 规格中的表名、字段、端点和错误码是待实现的受控契约；不应将 docs/architecture-baseline 或本规格本身叙述为已经存在的运行行为。
- 实现时保留工作树中与 Ticket 08 无关的改动，不修改历史 Migration，不把数据库故障降格为业务失败，也不通过 CLI/Worker 绕过 Daemon authority。

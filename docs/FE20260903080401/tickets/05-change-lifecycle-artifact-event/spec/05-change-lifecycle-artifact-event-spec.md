# 05: Change Lifecycle、Artifact 与 Event 本地交付规格

> **状态：** 规划规格。它记录 Ticket 05 已对齐的 M3 设计，不表示实现已完成，也不解除顶层 Ticket 05 对 Ticket 04 的 `BLOCKED_BY`。
>
> **用途：** 作为后续本地实施子票的共同契约；不对应外部 issue tracker，也不附加外部 triage label。

## Problem Statement

Project 已由 Daemon 权威注册后，本机操作者仍不能创建一条可恢复、可审计且可并发保护的 Change。现有边界缺少一条完整路径：在干净且稳定的 RepositoryBinding 上固定 BaseRevision，保存不可变 ChangeIntent，维护生命周期检查点和运行控制状态，并把状态变化、Artifact、AgentRun、HumanDecision、Event 与幂等结果作为可解释的权威事实保存。

如果 Client 直接选择 Project、读取 Manifest、调用 Git、写 SQLite，或直接设置 Stage/Status，重试、并发命令、晚到执行结果和磁盘失败都会造成重复、覆盖或无法解释的状态。操作者还无法辨别可安全重放的成功、需要人工决定的失败和暂时不可用的本机依赖。

## Solution

M3 让 Daemon 成为 Change、Artifact、AgentRun、HumanDecision、DomainEvent 与 ChangeCommandReceipt 的唯一权威。Client 仅通过窄的 Control Plane HTTP/JSON Contract 创建、查询和控制 Change；所有写操作带 Idempotency-Key，状态变化额外带 ChangeVersion 前置条件。

创建 Change 即开始生命周期：Daemon 从既有 Project 的权威 RepositoryBinding 解析 Project，以只读双检取得稳定的 ChangeSourceSnapshot，固定 BaseRevision，把原始 ChangeIntent 存成不可变 ChangeIntentArtifact，并在同一权威事务内创建 Change、ChangeCreated Event 和首次成功 Receipt。LifecycleStage 与 ChangeStatus 分离；M3 只协调已确认事实，不运行真实 StageStrategy、Worker、Runtime 或 Git 写入。

Artifact 内容先以摘要可校验、原子可见的方式写入 LocalStateRoot，之后 SQLite 才记录其业务引用。Event、Decision、ArtifactRef、AgentRun 与 Receipt 保持可追加的历史；当前 Change 快照和完整 Trace 分开读取，避免暴露宿主机文件路径、任意日志文本或内部持久化结构。

## User Stories

1. 作为本机操作者，我希望在已注册且干净的 RepositoryBinding 创建 Change，以便由 Daemon 管理一条可追溯的生命周期。
2. 作为本机操作者，我希望只提交 repository_path 与 ChangeIntent，而不必读取 Manifest 或指定 ProjectID，以便 Client 留在 Control Plane 边界内。
3. 作为本机操作者，我希望创建时固定 BaseRevision，以便所有后续阶段有明确的源版本起点。
4. 作为本机操作者，我希望已暂存、未暂存、未跟踪和子模块变化阻止创建，以便 Intent 不建立在含糊的源状态上。
5. 作为本机操作者，我希望被 Git 忽略的文件不阻止创建，以便本机缓存不会妨碍正常流程。
6. 作为使用 detached HEAD 的操作者，我希望 HEAD 能解析为 commit 时仍可创建 Change，以便受控的临时版本也可以追溯。
7. 作为本机操作者，我希望快照确认期间 HEAD 发生变化时得到明确冲突，以便不会误以为得到稳定源快照。
8. 作为本机操作者，我希望 unborn 或无法解析的 HEAD 被拒绝，以便 BaseRevision 永远是可验证的提交标识。
9. 作为本机操作者，我希望原始 ChangeIntent 被不可变保存，以便审计和后续阶段能使用我实际提交的输入。
10. 作为本机操作者，我希望 Intent 摘要有界而原文可经 ArtifactRef 单独读取，以便普通列表和 Event 保持轻量。
11. 作为本机操作者，我希望以相同 Idempotency-Key 重试成功的创建时得到首次成功响应，以便网络中断不制造第二个 Change。
12. 作为本机操作者，我希望使用新 Idempotency-Key 创建新的 Change，即使 Intent 与 BaseRevision 相同，以便一个 Project 可以并存独立意图。
13. 作为本机操作者，我希望查看 Change 的 Stage、Status、Version、BaseRevision、Intent Artifact 和最新 AgentRun，以便知道 Daemon 已确认到哪里。
14. 作为本机操作者，我希望分别查看 Event、AgentRun、ArtifactRef 和 HumanDecision Trace，以便复盘时不把无界历史塞进快照响应。
15. 作为本机操作者，我希望暂停 active Change，以便阻止新的协调和 Assignment，而不伪造对已经开始工作的强制终止。
16. 作为本机操作者，我希望恢复 paused Change，以便 Daemon 在允许时继续协调已经保存的事实。
17. 作为本机操作者，我希望取消 active、paused 或 human_required Change，以便禁止后续生命周期推进，同时保留已发生的事实。
18. 作为本机操作者，我希望取消后的晚到 AgentRun 结果仍可追溯但不会推进 Change，以便取消不抹去真实执行历史。
19. 作为本机操作者，我希望可重试的阶段失败进入 human_required，而不是自动重试，以便恢复意图始终由人明确决定。
20. 作为本机操作者，我希望只在 human_required 时提交 retry 或 cancel 的 HumanDecision，以便 Client 不能直接覆盖 Stage 或 Status。
21. 作为本机操作者，我希望 retry 创建新的 AgentRun attempt，而不改写失败尝试，以便输入、输出和失败证据可以区分。
22. 作为本机操作者，我希望命令基于我观察到的 ChangeVersion 执行，以便陈旧 Client 无法覆盖更新后的生命周期状态。
23. 作为本机操作者，我希望相同幂等键配合不同规范请求时收到明确冲突，以便脚本错误不会被静默合并。
24. 作为本机操作者，我希望 Event 能指出关联的 AgentRun、HumanDecision 和 ArtifactRef，以便因果关系不依赖时间戳猜测。
25. 作为本机操作者，我希望 Artifact 内容在读取时重新校验摘要，以便缺失或损坏的磁盘内容不会被当作可信证据交付。
26. 作为本机操作者，我希望 Artifact 的物理路径永不出现在 API 响应中，以便 Client 不依赖宿主机布局。
27. 作为本机操作者，我希望错误响应区分无效输入、未找到、可处理冲突和暂时不可用，以便采取正确恢复动作。
28. 作为 CLI 使用者，我希望 Change 写命令确保 Daemon 已就绪，而纯查询不会启动新的 Daemon，以便命令副作用可预测。
29. 作为 CLI 使用者，我希望成功的 Change 命令输出 JSON，以便自动化脚本能消费稳定结果。
30. 作为 Control Plane Client，我希望严格 JSON Contract 拒绝未知字段和多个 JSON 值，以便协议演进不静默丢失语义。
31. 作为领域维护者，我希望 Domain 能在没有 SQLite、HTTP、Git 或 Worker 的环境中验证生命周期不变量，以便核心规则快速且确定地测试。
32. 作为基础设施维护者，我希望 SQLite 强制 Event、Decision、ArtifactRef、AgentRun 和 Receipt 的归属与不可变性，以便 Repository 回归不会静默改写历史。
33. 作为 Project 功能维护者，我希望 Project 与 Change 使用一个内部 Event 账本，以便 V1 不形成需要跨表拼接的双账本。
34. 作为后续 Worker 实现者，我希望 M3 已保存 AgentRun 的不可变输入、输出和失败 Artifact 关联，以便不必重写历史模型。
35. 作为后续 Worktree 实现者，我希望能够在创建 Workspace 前复核 BaseRevision，而不重新定义 Change 创建时的源起点。
36. 作为安全审查者，我希望 Client、Worker 和 Dashboard 都不能直接访问 Keystone SQLite，以便 Daemon 始终是生命周期权威。

## Implementation Decisions

- Ticket 05 是 M3 的 Change 生命周期纵切。它依赖 Ticket 04 已提供的 Project、RepositoryBinding、Project 查询、统一内部 Event 账本与 Work-owned 业务 Migration；规划成熟度、Daemon Readiness 或既有 M1 代码都不构成依赖已解除的证据。

- Change 是绑定一个 Project 和不可变 BaseRevision 的长期事实。相同 Idempotency-Key、相同操作、相同聚合范围和相同规范请求才重放首次成功结果；不得按 Intent、BaseRevision 或 RepositoryBinding 做业务语义去重。

- LifecycleStage 是最近一个已确认的耐久检查点，顺序固定为 Intent、Understand、Design、Plan、Ticketize、Execute、Verify、FinalVerify。ChangeStatus 是运行控制状态，固定为 active、paused、human_required、cancelled、integrate_ready。创建后的状态为 Intent/active；M3 不得使 Change 进入 integrate_ready。

- 控制转换固定为：active 可 Pause 至 paused；paused 可 Resume 至 active；active、paused、human_required 可 Cancel 至 cancelled；active 的可重试阶段失败进入 human_required；human_required 只能以 HumanDecision retry 回到 active 并创建新的 AgentRun，或以 HumanDecision cancel 进入 cancelled。普通 Command 只允许 Pause、Resume、Cancel；Decision 只允许 retry、cancel。

- Pause 与 Cancel 是前瞻性协调围栏，不要求杀死已启动的进程。paused 阻止晚到结果推进检查点；cancelled 的晚到结果只追加 Trace，绝不推进 Change。

- 生产组合在 M3 不运行真实 StageStrategy、Worker 或 Runtime。创建只保存 Intent/active，不伪造 AgentRun 或阶段产物；Application 测试可以注入确定性的 Strategy，验证成功、失败和 human_required 分支。

- Change 创建仅接受绝对 repository_path 与 Intent。Daemon 解析既有 Project 的权威 RepositoryBinding；找不到 Project 返回 project_not_found，不隐式初始化 Project，Client 不提交 ProjectID。

- ChangeSourceSnapshot 在同一只读边界连续取得干净状态、HEAD、干净状态、HEAD。干净由 Git porcelain 的空结果定义，包含已暂存、未暂存、未跟踪和子模块变化，排除被忽略文件。两次 HEAD 必须解析为同一个完整 Git commit；unborn 或不可解析 HEAD 为 base_revision_unavailable，变化为 source_snapshot_unstable。该流程不写 Git、不加 Git 锁，也不承诺抵御恶意本机 TOCTOU。

- BaseRevision 是完整小写 Git OID，允许与对象格式相符的 40 或 64 位值。资源标识使用小写 canonical UUIDv7；时间使用 UTC RFC3339Nano；SHA-256 使用小写 64 位十六进制；数值字段使用非负 JSON 整数。Idempotency-Key 保持原样、非空且不透明，不在 M3 增加格式、长度或 trim 规则。

- ChangeIntent 必须为有效 UTF-8，去除首尾空白后非空，原始长度最多 64 KiB。校验后原始字节以 text/plain; charset=utf-8 的 change_intent Artifact 保存。摘要只使用空白归一化后的前 256 个 Unicode rune，不能改写或替代原文。

- Artifact 身份由 SHA-256 与字节长度表示，ArtifactRef 由独立 UUID 表示业务关联。内容写入顺序为同目录临时文件、计算摘要、文件同步、原子 rename、再开始 SQLite transaction；事务失败可留下无引用内容，但正常失败不得形成指向未成功写入内容的权威引用。已存在内容只能在摘要与长度复核后复用。

- M3 不把跨平台断电持久性表述为更强保证。读取 Artifact 必须重新计算摘要；内容缺失或不匹配返回 unavailable，不交付损坏字节，也不自动修复。内容端点只返回原始字节、媒体类型、长度和摘要派生 ETag，不暴露物理路径，不提供 Range 或条件读取。

- AgentRun 固定身份、Stage、attempt、输入和关联 ArtifactRef。读取状态只有 running、completed；completed 必须具备 succeeded、failed 或 human_required outcome 及完成时间。retry 永远创建相同 Change/Stage 的新 attempt；输入、输出和失败证据以带 role 与 ordinal 的专用关联保存，不用自由 JSON 数组或通用 owner 表。

- DomainEvent 是追加式审计事实，不采用完整 Event Sourcing。Project 与 Change 共用一个内部 UnifiedEventLedger，在聚合内以 EventSequence 排序。M2 只对外公开 ProjectInitialized 的窄查询；M3 在同一账本扩展 Change Event，不能建立第二个权威业务 Event 表。若前序物理结构不足，M3 必须通过单个受控 Migration 演进该账本，而不能并行保留两套权威数据。

- Change Event 固定返回 event_id、change_id、sequence、type、occurred_at、Actor、有序 artifact_ref_ids，以及可空 agent_run_id、decision_id。M3 EventType 固定为 ChangeCreated、AgentRunStarted、AgentRunCompleted、StageAdvanced、ChangePaused、ChangeResumed、ChangeHumanRequired、HumanDecisionRecorded、ChangeCancelled；不含自由 payload、details map、日志文本或原始 Artifact。

- 创建只追加关联 Intent Artifact 的 ChangeCreated。AgentRun 终态先追加 AgentRunCompleted，再在同一事务与后续序号追加 StageAdvanced 或 ChangeHumanRequired。retry 依次追加 HumanDecisionRecorded 和新的 AgentRunStarted；Decision cancel 依次追加 HumanDecisionRecorded 和 ChangeCancelled；普通取消只追加 ChangeCancelled。

- ChangeVersion 是 Change 权威状态的单调版本。命令和 Decision 都携带 expected_version；先查 Receipt，再检查 Version。成功状态改变、Event、Decision 与 Receipt 在同一 SQLite transaction 提交，Version 只经带旧值条件的更新递增；陈旧版本返回 change_version_conflict。

- 每个 Change 写操作都携带原样、非空且不透明的 Idempotency-Key。成功 Receipt 保存操作、聚合范围、幂等键、规范请求比较信息、首次成功 HTTP 状态和有界规范响应体；同键同请求重放原成功响应而非之后变化的 ChangeView，同键不同请求为 idempotency_conflict。无效输入、业务冲突和 unavailable 不写 Receipt。

- ChangeView 固定包含 change_id、project_id、repository_root、stage、status、version、base_revision、intent_artifact、latest_agent_run、created_at、updated_at；latest_agent_run 缺失时为 null。ArtifactRef、AgentRun 和 Decision 的响应只暴露各自稳定摘要字段，不暴露内部存储信息。

- M3 提供 Change 创建、按 repository_path 列表、单个 Change、Event/Run/Artifact/Decision Trace、Artifact 内容、Command 和 Decision 的固定端点。空集合为 []，不分页；Change 以 created_at、change_id 倒序，Event 以 EventSequence 升序，其余历史以 created_at、标识升序。Project 下 Change 聚合查询和 Dashboard 聚合查询留给后续 Ticket。

- 写入请求只接受单个严格 JSON object，拒绝未知字段和多个 JSON 值。创建成功为 201，Command 与 Decision 成功为 200。失败使用既有 ErrorEnvelope，并采用 invalid_request、project_not_found、change_not_found、artifact_not_found、repository_dirty、base_revision_unavailable、source_snapshot_unstable、idempotency_conflict、change_version_conflict、lifecycle_transition_invalid、human_decision_required、unavailable、internal_error 的稳定分类；不得泄漏 SQL、Git 原始错误、绝对路径或内部存储错误。

- CLI 提供 Change 命令组：create、list、show、pause、resume、cancel、decide retry、decide cancel。写操作显式要求 Idempotency-Key；Command 与 Decision 额外要求 expected-version。Change 父命令只解析一次 data-dir；写命令共用 Daemon ensure seam，纯查询只发现既有 Daemon。CLI 只将当前目录绝对化后交给 Daemon，不读取 Git、Manifest 或 SQLite。

- Work 组织为纯 Domain、Application 与 Port；SQLite Repository/业务 Migration、ArtifactStore、只读 Git Snapshot 为独立基础设施实现。Daemon 负责组合、连接、Migration、readiness 与 HTTP DTO 转换；Control Plane Contract 不导入 Domain、SQLite 或 Daemon 实现。

- 通用 Migration runner 保持递增、漂移检查和应用编排的窄职责。业务 Migration 由 Work 拥有，并在 Daemon 持锁启动时与通用元数据 Migration 合并应用。M3 使用 Ticket 04 后下一个可用版本，不在规划中预留固定版本；它建立 Change、Artifact、ArtifactRef、AgentRun、Decision、Receipt 与两类专用 Artifact 关联，并演进既有 UnifiedEventLedger。

- 每个 SQLite 连接启用 foreign_keys。Event、Decision、ArtifactRef、成功 Receipt 与专用关联只能追加；Artifact 身份不可更新；M3 不提供删除路径；AgentRun 只允许 running 到 completed 的一次合法更新。未来孤儿回收或维护能力须以显式 Migration 与规则引入。

- 建议的本地交付顺序是：先建立纯领域模型与 Contract，再建立 Work 持久化、Git Snapshot 与 ArtifactStore，再接通 Daemon/HTTP/CLI 的纵切，最后做端到端验收和导航更新。每一项实施子票仍继承顶层 Ticket 04 的阻塞条件，不因本规格存在而自动 ready。

## Testing Decisions

- 最高验收 seam 是既有 CLI 到 loopback Daemon 的真实 HTTP/JSON 路径：在临时 LocalStateRoot 和真实临时 Git Repository 中执行 Change 命令，观察 Contract 响应、SQLite 权威状态、Artifact 内容和 Trace。它复用既有 CLI 的 Daemon ensure/发现能力，不新建并行测试协议。

- Domain 单元测试只验证 LifecycleStage、ChangeStatus、ChangeVersion、合法与非法转换、AgentRun 一次性终态、retry 的新 attempt、HumanDecision 与晚到结果围栏；不启动 Git、SQLite、HTTP、Daemon 或 Worker。

- Application 测试经 Port failure seam 验证创建顺序、双读 ChangeSourceSnapshot、Receipt 先于 Version 检查、成功重放原响应、同键请求冲突、同 Project 多 Change、状态/Event/Decision/Receipt 原子性，以及确定性 Strategy 的成功、失败与 human_required 分支。

- Git Snapshot 集成测试使用真实临时 Git Repository，覆盖 root 与子目录、dirty、staged、untracked、ignored、submodule、detached HEAD、unborn HEAD 和普通并发 HEAD 改变，并证明 M3 不执行 Git 写入。

- ArtifactStore 集成测试覆盖原文保存、摘要和长度、同内容复用、临时写入失败、rename 前后失败、SQLite transaction 回滚留下孤儿、内容缺失、摘要失配与内容读取响应。测试观察可见内容和权威引用，不耦合临时文件命名。

- SQLite 集成测试覆盖 Migration 顺序、统一 Event 账本演进、外键启用、复合归属约束、EventSequence、不可变触发器、AgentRun 终态约束、ChangeVersion 条件更新、Receipt 原响应保存和事务回滚。

- Control Plane Contract 与 Handler 测试覆盖严格 JSON、必需 Idempotency-Key、UUID 与时间格式、固定 DTO 字段、空列表、排序、成功状态码、ErrorEnvelope 分类、Artifact 原始内容边界和 readiness 未完成时的 unavailable；它们不从 Contract 导入 Domain 或 SQLite 实现。

- CLI 测试沿用既有依赖注入 seam，验证 data-dir 只解析一次、写命令确保 Daemon、读命令不启动 Daemon、正确传递当前目录/幂等键/Version、成功 JSON 输出和 ErrorEnvelope 的用户可诊断转换；还需补充真实 CLI/Daemon 集成验证。

- 功能测试优先验证操作者可观察的状态、响应、Artifact 完整性和 Trace，而不是私有 SQL 文本、临时文件名或内部调用次数。既有 Daemon、Migration、Control Plane Contract 与 CLI 生命周期测试是直接先例。

- 实施完成后执行完整 Go 测试、静态检查、构建和差异卫生检查。Linux、WSL 与原生 Windows 的真实 Git、文件系统、CLI/Daemon 行为均为验收目标；交叉编译不能替代原生 Windows 证据，且 Ticket 04 的原生 InstanceLock 前置验收必须先满足。

## Out of Scope

- Worker 进程、Runtime Adapter、真实 Understand/Design/Plan Strategy、Codex/OpenCode 调用和任何自动 Agent 执行。
- Ticket Graph、Ticketize 图持久化、Workspace、Git Worktree、Execute、Verify、Git add/commit/push/merge 与 integrate_ready 的实际进入逻辑。
- Dashboard 页面、SSE、Project 下 Change 聚合列表、分页、通用 Event stream、完整 Trace 导出和完整 Event Sourcing。
- Policy DSL、Risk Model、复杂 Gate、RBAC、认证、远程 Control Plane、TLS、团队协作和 macOS 目标。
- 对恶意本机 TOCTOU 的文件系统安全保证、跨平台断电级持久性承诺、自动 Artifact 孤儿回收、down Migration 和自动数据修复。
- Client 或 Worker 直接访问 Git、ProjectManifest、Keystone SQLite 或 ArtifactStore 内部路径的能力。

## Further Notes

- 本规格使用 Change、ChangeSourceSnapshot、LifecycleStage、ChangeStatus、ArtifactRef、EventSequence、AgentRun、HumanDecision、ChangeCommandReceipt 与 Actor 的项目术语，并遵循已经接受的相关 ADR 取舍。

- 统一 Event 账本的最终物理列形态不在本规格中预先锁死。实施前应以 Ticket 04 实际已落地的账本 Schema 为依据，使用一个受控 Migration 补足 M3 的强类型关联、归属约束与序列约束，持续保持唯一权威账本。

- 本文件按用户指定目录保存，仅是本地规划工件；没有外部 issue tracker 发布动作，也不声称代码、Schema、HTTP 路由、CLI 命令或原生平台测试已经落地。

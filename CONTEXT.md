# Keystone 运行语义上下文

本上下文收敛 Keystone 本机 Control Plane 的稳定运行术语，避免将 Daemon 就绪、Ticket 可执行性和规划文档状态混为一谈。

## 本机 Control Plane

**LocalStateRoot**：
由本机操作者选择的数据根，用于承载一个 Keystone 本机实例的运行状态。每个 LocalStateRoot 同时至多对应一个活跃的 DaemonInstance。
_避免_：用户全局状态、Repository 状态目录、项目状态目录

**DaemonInstance**：
在一个 LocalStateRoot 上运行并拥有该根 Control Plane 权威状态的独立 Daemon 进程。
_避免_：CLI 进程、Worker、数据库连接

**DaemonInstanceID**：
标识一个 DaemonInstance 的唯一关联标识，用于让 Client 确认控制请求仍指向观察到的实例。它只防止陈旧目标，不是安全凭据。
_避免_：进程 PID、访问令牌、锁所有权

**InstanceLock**：
对一个 LocalStateRoot 排他绑定一个 DaemonInstance 的本机互斥事实。它是实例排他性的唯一权威，不能由 PID 或 RuntimeMetadata 替代。
_避免_：PID 所有权、metadata 所有权、端点所有权

**RuntimeMetadata**：
供本机 Client 发现和诊断 DaemonInstance 的运行记录。它不是实例锁、身份权威或就绪状态的判定依据。
_避免_：锁记录、Ready Metadata、进程真相

**DaemonEndpoint**：
DaemonInstance 接收本机 Control Plane 请求的 loopback 地址。Client 使用它建立请求连接，但不以它替代实例权威性判断。
_避免_：固定服务端口、远程 API 地址

**DaemonReadiness**：
DaemonInstance 能够安全服务本机 Control Plane 请求的运行状态。它只描述 DaemonInstance，不描述 Ticket 或实施文档。
_避免_：Ticket ready、ready-for-agent、规划完成

**SchemaMigrationVersion**：
一个 LocalStateRoot 所属持久状态中，已成功提交的最高 Schema Migration 版本。
_避免_：业务版本、应用版本、Ticket 版本

## Project 初始化

**Project**：
由 Daemon 权威持有的长期工程范围。V1 中一个 Project 对应一个 RepositoryIdentity，并且同时至多有一个活动的 RepositoryBinding。
_避免_：当前工作目录、DaemonInstance、单次 Change

**ProjectID**：
标识 Project 的稳定关联标识。V1 用它表达 RepositoryIdentity，而不从本机路径、远程地址或运行实例派生。
_避免_：RepositoryBinding、DaemonInstanceID、LocalStateRoot

**RepositoryIdentity**：
将 Project 与其版本化项目知识稳定关联的身份。V1 中它由 ProjectID 表达，可跨本机目录移动保持一致，但不能同时关联两个活动的仓库根。
_避免_：任意文件路径、远程地址、LocalStateRoot

**RepositoryBinding**：
一个 Project 当前关联的规范化、非 bare Git 主工作树根。子目录属于同一 Binding；子模块可作为独立 Project 的 Binding。
_避免_：linked worktree、Workspace、RepositoryIdentity

**ProjectManifest**：
Repository 持有的版本化 ProjectIdentity 与确定性验证配置表达；V1 只表达 ProjectID，V2 的可解释字段为 `version`、`project_id`、必填 `verify.commands` 与可选 `commit.template`。它不拥有 Project 当前权威状态，也不承载本机运行状态。
_避免_：Keystone DB、RuntimeMetadata、用户级状态目录

**ProjectManifestVersion**：
ProjectManifest 可被当前 Control Plane 解释的版本边界。V1 可继续用于 Project 身份协调，但不能形成确定性 Verify 配置；只有由 Repository 手工、版本化地显式升级为 V2 的 Project 才可进入自动 Verify。V2 拒绝未知字段与重复键；Daemon 不得静默改写 V1 或 V2，也不得丢弃未知字段取得协调成功。
_避免_：SchemaMigrationVersion、应用版本、自动修复标记

**VerificationConfigurationDigest**：
严格 V2 parser 对已接受的 `verify.commands` 形成的规范化语义摘要；它保留命令声明顺序但不把 YAML 空白或注释当作配置变化，并固定为 VerificationEvidence 与 Verify/FinalVerify CommandReceipt 的输入身份。
_避免_：原始 Manifest 字节、Client 临时参数、可变环境变量

**CommitTemplateDigest**：
严格 V2 parser 对已接受 `commit.template` 形成的规范化语义摘要；它只固定 CommitIntent 与 CommitCommand 的消息输入，不会使已形成的 VerificationEvidence 失效。
_避免_：VerificationConfigurationDigest、Client 自由提交消息、YAML 格式差异

**VerificationPolicySnapshot**：
Daemon 在接受 ExecuteCommand 时从 BaseRevision 读取并严格解析的 ProjectManifest V2 验证策略快照，固定 VerificationCommand、VerificationConfigurationDigest、CommitTemplate 与 CommitTemplateDigest。当前候选 Workspace 对 Manifest 的修改不影响该 Change；V1 或无效 BaseRevision Manifest 不能形成此快照，Verify 时进入 human_required。
_避免_：候选工作树当前 Manifest、Client 参数、可在 Verify 时替换的配置

**ProjectInitialization**：
将 RepositoryBinding 注册或协调为 Project 的 Control Plane Command。语义相同的重复请求必须收敛到同一 Project，不能创建重复权威记录。
_避免_：Change 创建、Repository 全量分析、Client 直写 Control Plane 状态

**ProjectInitializationIntent**：
Daemon 持有的可恢复初始化候选，保存尚未成为权威 Project 的 ProjectID 与 RepositoryBinding。它不是 Project，也不产生 ProjectInitialized。
_避免_：Project、已完成回执、运行时日志

**ProjectInitializationReceipt**：
与一次带幂等键的 ProjectInitialization 关联的持久结果。成功回执可重放其结果；未完成意图继续协调，不能被当作成功回执。
_避免_：ProjectInitialized、Client 缓存、DaemonInstanceID

**ProjectInitialized**：
Project 首次成为一个 LocalStateRoot 权威记录时追加的不可变领域事实。重试或只修复 ProjectManifest 的协调不产生新的 ProjectInitialized。
_避免_：启动日志、Manifest 写入记录、Lifecycle 推进

## Change 生命周期与审计

**Change**：
由 Daemon 权威持有、绑定一个 Project 与不可变 BaseRevision 的长期变更意图及其生命周期事实。一个 Project 可同时拥有多个 Change；除同一 IdempotencyKey 的同一规范请求外，M3 不按 Intent、BaseRevision 或 RepositoryBinding 进行业务去重。
_避免_：单次 AgentRun、Git Worktree、Client 本地草稿

**ChangeCreation**：
以绝对 RepositoryBinding 路径和经过严格边界验证的 ChangeIntent 提交的 Control Plane Command；Daemon 解析既有 Project 后才创建 Change，Client 不读取 ProjectManifest 或直接指定 ProjectID。
_避免_：ProjectInitialization、Client Manifest 读取、隐式 init

**ChangeIntent**：
创建 Change 的原始文本意图。M3 只接受去除首尾空白后非空、有效 UTF-8 且原始长度不超过 64 KiB 的值；通过校验后原样保存为 ChangeIntentArtifact，不能以摘要或规范化文本替代原始内容。
_避免_：可变的 Change 描述、预先规范化的摘要、任意 JSON payload

**BaseRevision**：
在 Change 创建时确认并固定的源 Repository 版本快照；它是 `git rev-parse --verify HEAD^{commit}` 返回的完整小写 Git OID（依 Repository 的对象格式为 40 或 64 位），后续生命周期不得用当前 HEAD 覆盖它。首次创建 Workspace 前必须再次确认源 HEAD 仍等于 BaseRevision。
_避免_：运行时 HEAD、候选提交、用户输入的 revision

**ChangeSourceSnapshot**：
创建 Change 时对干净 RepositoryBinding 作出的只读版本确认，由 BaseRevision 表达其固定版本。M3 连续执行“干净状态、HEAD、干净状态、HEAD”两轮确认，且两次 HEAD 必须相同；M3 以 `git status --porcelain=v1 --untracked-files=all --ignore-submodules=none` 的空输出定义干净。已暂存、未暂存、未跟踪和子模块变化均阻止创建，被忽略文件不阻止；HEAD 变化为 source_snapshot_unstable，unborn HEAD 不能形成快照。该边界不加 Git 锁，也不声称消除本机 TOCTOU。
_避免_：Git Worktree、运行时工作目录、Client 传入的 revision

**ChangeIntentArtifact**：
首次创建 Change 时保存的不可变意图 Artifact，用于后续 Stage 的输入；它不是 ProjectInitializationIntent。
_避免_：ProjectInitializationIntent、可编辑描述、运行日志

**LifecycleStage**：
Change 最近一个已确认、已持久化的生命周期检查点，按 Intent、Understand、Design、Plan、Ticketize、Execute、Verify、FinalVerify 的既定顺序前进。
_避免_：当前进程状态、Client 可直接设置的字段、ChangeStatus

**LifecycleCoordinator**：
由 Daemon 持有的生命周期协调者，依据已确认事实决定是否允许下一步；它不拥有 Stage 的业务产出，也不直接执行工具。
_避免_：StageStrategy、Worker、Client 状态修改器

**StageStrategy**：
为一个 LifecycleStage 提供输入到候选输出的受限能力；其结果须由 LifecycleCoordinator 验证后才可能形成权威检查点。
_避免_：LifecycleCoordinator、AgentRun、状态权威

**ChangeStatus**：
决定 Change 是否允许继续协调的运行控制状态；V1 使用 active、paused、human_required、cancelled 与 integrate_ready，且可重试 Stage 失败进入 human_required，integrate_ready 只在 FinalVerification PASS 且 CandidateRevision 已持久化后出现。paused 和 cancelled 不允许晚到结果推进检查点。
_避免_：LifecycleStage、AgentRun outcome、Verify 结果

**Artifact**：
与 Change 生命周期有关、内容不可变且可完整性校验的持久化输入、输出或证据；同一内容可以被多个 ArtifactRef 关联。
_避免_：可编辑文档、临时内存值、Event payload

**ArtifactRef**：
将一个业务事实独立关联到 Artifact 的可查询引用，保留其定位、摘要与完整性标识而不复制内容。
_避免_：Artifact 内容副本、可变文件路径、运行时日志流

**ArtifactSummary**：
用于在本机查询中识别 Artifact 的受长度限制预览；它不能替代原始内容，也不承载任意 Runtime 输出摘录。
_避免_：完整 Artifact、Event payload、错误文本

**ArtifactKind**：
Artifact 在生命周期中的语义类别；M3 仅产生 change_intent，后续类别由拥有相应 Stage 的 Ticket 增加。它与 WorkerArtifactKind 是不同层次的概念。
_避免_：文件扩展名、MIME type、WorkerArtifactKind

**WorkerArtifactKind**：
Worker Protocol 中原始观察 Artifact 的固定传输类别：stdout、stderr、diff 与 changed_files。它不表达 CanonicalTicket、WorkspaceSnapshot、TicketDelta 或生命周期语义。
_避免_：ArtifactKind、TicketExecutionEvidence、DomainEvent

**ArtifactContent**：
由 ArtifactRef 定位的原始不可变字节。M3 在 LocalStateRoot 的 SHA-256 分层目录中先完成同目录临时写入、文件 Sync 与原子 rename，才允许 SQLite 引用它；普通进程失败不能留下指向未成功写入内容的权威引用。断电后的缺失或摘要不符由每次读取时的 SHA-256 校验发现并作为 unavailable 返回，M3 不伪造跨平台断电持久性或自动修复。
_避免_：ArtifactSummary、数据库 BLOB 暴露、未校验的本机文件读取

**ChangeReadModel**：
供 Client 观察的有界 Change 快照，由 ChangeView 表达当前字段并以 repository_root 返回规范化 RepositoryBinding，所有时间字段使用 UTC RFC3339Nano；Event、AgentRun、ArtifactRef 和 HumanDecision 保留在独立 Trace 列表中。M3 不在一个 Change 查询中嵌入完整历史、原始 Artifact 内容或无界分页结果。
_避免_：持久化行模型、完整审计导出、可变 Client 缓存

**ChangeObservationReadModel**：
由 Daemon 为 Dashboard 组合的有界 Change 观察快照，按独立 section 包含 Lifecycle、CanonicalTicketGraph、Execution、Trace、Artifact、Health 与 available_actions；它只投影已有权威事实，不建立第二套业务账本或状态机。每个 section 必须标记 available 或 not_yet_available；权威读模型不可用时整个观察 Query 失败。
_避免_：Dashboard 本地真相、Client 推导的 Lifecycle、无界聚合响应

**ObservationSection**：
ChangeObservationReadModel 中带有明确可用性标记的独立观察片段。available 加空集合表达真实 empty；not_yet_available 表达上游尚未形成事实，不能用空数组伪装已完成，也不能由 Client 推导原因。
_避免_：缺失字段、伪造的空状态、部分成功快照

**DomainEvent**：
在权威业务事实发生时追加的不可变审计记录；它解释状态如何到达当前值，但不以重放替代权威状态。它的公开表达只包含固定的边界字段和 ArtifactRef，不承载无约束内容。
_避免_：应用日志、Worker 自报、完整 Event Sourcing

**UnifiedEventLedger**：
同一物理 `t_events` 账本对 Project 与 Change 保存追加式 DomainEvent。M2 只写入并公开窄的 ProjectInitialized 查询；M3 在同一账本上扩展 Change 追溯，不能另建第二个业务 Event 账本。
_避免_：通用 Event stream、Project/Change 双账本、完整 Event Sourcing

**ChangeEvent**：
归属于一个 Change 并具有 EventSequence 的 DomainEvent；它以受控 EventType、固定来源信息、有序 ArtifactRef 关联及可空的 AgentRun 或 HumanDecision 标识表达事实。这样 Trace 可准确归因而不使用自由 payload。
_避免_：通用 JSON details、应用日志、Change 当前状态快照

**EventArtifactLink**：
一个 ChangeEvent 与同一 Change 的 ArtifactRef 之间的有序、强类型关联；它独立保存以维持外键完整性和响应中的 artifact_ref_ids 顺序，不是通用多态 owner 关系。
_避免_：t_events 中的 UUID JSON 数组、无归属校验的引用、Artifact 内容副本

**EventType**：
DomainEvent 的受控语义名称，用于表达已发生的业务事实，而不是可自由扩展的日志级别或错误分类。
_避免_：日志级别、HTTP 错误码、Runtime 输出类型

**EventSequence**：
同一业务聚合内 DomainEvent 的单调发生顺序，用于可靠复盘而不依赖 UUID 或时间戳排序。
_避免_：UUIDv7 顺序、全局因果顺序、日志行号

**CommandReceipt**：
与一个幂等键及其规范化 Command 绑定的持久化成功结果，保存首次提交的有界 HTTP 状态码与规范响应体，使相同请求在 Change 后续变化后仍可重放原结果。只有成功提交的变更操作创建 Receipt；未产生权威副作用的输入、冲突或 unavailable 结果不占用该键。
_避免_：Client 缓存、Event、一次性 HTTP 响应

**IdempotencyKey**：
由调用者为一次可重试变更操作显式提供的不透明键。M3 的 ChangeCreation、ChangeCommand 与 HumanDecision 均须携带它；CLI 不为每次调用静默生成新键，以免掩盖调用者的重试边界。
_避免_：ChangeVersion、EventSequence、由 CLI 隐式生成的一次性重试标识

**ChangeVersion**：
Change 权威状态的单调版本，用于拒绝基于陈旧观察提交的不同 Command；它不表达 EventSequence 或 BaseRevision。
_避免_：EventSequence、Git revision、Idempotency-Key

**AgentRun**：
为一个 LifecycleStage 创建的单次执行尝试，其身份、输入、关联 ArtifactRef 与 attempt 不可变，且只可记录一次终态。Execute 阶段的 AgentRun 必须且只绑定一张 CanonicalTicket，并由 SourceControl 形成执行前后的 WorkspaceSnapshot 与 TicketDelta；Retry 必须创建新的 AgentRun，不能改写既有尝试或 Change 的已确认检查点。paused 或 cancelled 后到达的结果仍可追溯，但不能自行推进检查点。
_避免_：Change、Worker、可复用任务槽

**AgentRunArtifactLink**：
一个 AgentRun 与同一 Change 的 ArtifactRef 之间的有序、强类型关联；M3 只使用 input、output 与 failure 角色，以固定某次 attempt 实际消费和产生的证据，不使用 JSON 数组或通用 owner 表。
_避免_：从当前 Change 倒推的可变输入、EventArtifactLink、Worker 临时目录

**AgentRunOutcome**：
AgentRun 一次性完成时记录的实际结果；V1 使用 succeeded、failed 或 human_required。Change 的 Cancel 是控制面状态，不是 AgentRunOutcome。
_避免_：ChangeStatus、Worker lease、执行进程退出信号

**AgentRunStatus**：
AgentRun 在读取模型中的执行状态；M3 只使用 running 与 completed，且 completed 必有 AgentRunOutcome 与 completed_at，running 的 outcome 与 completed_at 均为空。
_避免_：ChangeStatus、AgentRunOutcome、Worker 进程存活探针

**WorkerInstance**：
由 Daemon 监管的本机副作用执行进程实例；它在一次进程生命周期内拥有独立的 Worker 身份、协议凭据和可用性事实，不拥有 Change 或 Ticket 权威状态。
_避免_：DaemonInstance、AgentRun、WorkerPool

**WorkerHealthReadModel**：
由 Daemon 从 Worker 注册、能力和心跳事实形成的有界健康观察摘要；它只供 Client 观察可用性、能力与最近心跳，不允许 Client 自行计算 freshness，也不表达 Change、Ticket 或执行成功。
_避免_：Worker 自报业务状态、Worker Protocol、Dashboard 本地健康真相

**WorkerProtocol**：
Daemon 与 Worker 之间用于注册、心跳、领取 Assignment 和提交 Report 的窄执行边界；它只传递执行所需的授权与结果，不承载 Control Plane 的生命周期命令。
_避免_：Control Plane API、应用日志、数据库接口

**WorkerCapability**：
Worker 在 `worker/v1` Register 中声明、由 Daemon 选择 Assignment 时核验的传输能力名称。`verification-v1` 表示该 Worker 能严格解码并执行 V1 的 VerificationAssignment；缺失该能力的旧 Worker 只可继续接收既有 edit Assignment。
_避免_：Change 权限、Runtime 自报成功、协议版本升级

**VerificationAssignment**：
`worker/v1` 中 capability-gated 的 Assignment 变体，使用显式 `kind: verify`、Daemon 派生的不可变 VerificationPolicySnapshot 摘要、有序命令计划与受限审查输入；它不接收 Client 命令、Change/Ticket/Gate 状态命令或数据库凭据，并继续受 Lease 与 RuntimeClaim 围栏。
_避免_：通用 shell 代理、Client Prompt、Worker 自选验证范围

**VerificationReport**：
`worker/v1` 中 capability-gated 的 Report 扩展，保存有序 VerificationCommandResult 与严格结构化的 AcceptanceCriterionResult；通用 Report.Artifacts 仍只保留 stdout、stderr、diff、changed_files 四类。Report.Outcome 只表达该 VerifierAgentRun 的传输终态，不替代 Daemon 形成的 VerificationOutcome。
_避免_：新的通用 WorkerArtifactKind、AgentRunOutcome、Worker 权威 verdict

**Assignment**：
Daemon 将一个已授权的执行机会交给 WorkerInstance 的业务关联，绑定一个 AgentRun 和一次执行权；Assignment 不是可被多个 Worker 复用的任务槽。
_避免_：Ticket、Change、可复用任务槽

**Lease**：
Assignment 在有限时间内有效且只允许一个执行者提交首次终态的执行授权；Lease 失效后，结果最多进入 Trace，不能恢复已被 Daemon 形成的权威事实。
_避免_：InstanceLock、ChangeVersion、lease token

**RuntimeAdapter**：
将 Assignment 的受限运行输入转换为具体 Runtime 执行结果的边界能力，使 Worker 不依赖某一种 Runtime 的命令细节；V1 首先实现 Codex Adapter，并保留 OpenCode 的接口位置。
_避免_：WorkerInstance、RuntimeMetadata、LifecycleCoordinator

**ExecutionGuard**：
限制 Runtime 执行边界并检查执行前后不变量的治理事实；V1 通过可验证的命令、环境和 Git 状态约束降低越界风险，但不等同于完整的操作系统沙箱。
_避免_：Prompt 禁令、InstanceLock、完整沙箱

**LateReport**：
Worker 已通过协议鉴权但不再具备改变权威 AgentRun 或 Change 资格的执行结果；它可以连同 Artifact 保留为 Trace，但不能推进生命周期、重新取得 ProjectExecutionSlot 或恢复已取消的 Change。
_避免_：Worker 自报状态、重试命令、ChangeStatus

**HumanDecision**：
对 human_required Change 追加的人工恢复事实；V1 只允许 retry 或 cancel，Daemon 解释其合法动作，Client 不直接指定 LifecycleStage 或 ChangeStatus。Execute 阶段的 retry 只为同一 CanonicalTicket 在既有 Workspace 创建新的 AgentRun，不重置或清理已有 Diff；Verify 与 FinalVerify 的恢复语义由 VerificationRecovery 限定。
_避免_：状态字段覆盖、可编辑备注、Worker Report

**NeedsHumanQuery**：
由 Daemon 根据已形成的 human_required/HUMAN_REQUIRED 事实返回的有界待人工列表；每项包含 scope、reason_code、当前 ChangeVersion、允许的 available_actions 与有界 evidence references。它不通过 AgentRun、heartbeat、错误文案或空 section 推导人工需求。
_避免_：Client 推导的 Human Required、失败日志列表、自动恢复队列

**ChangeCommand**：
由 Client 提交、以 ChangeVersion 为前置条件的状态控制请求；M3 只允许 pause、resume 与 cancel，不承载 HumanDecision 或目标状态字段。
_避免_：HumanDecision、Event、直接状态更新

**ChangeCommandReceipt**：
对 ChangeCreation、ChangeCommand、ExecuteCommand、VerifyCommand、CommitCommand、FinalVerifyCommand 或 HumanDecision 的持久化成功幂等结果；它与对应状态、Event、Decision 和首次响应在同一 SQLite transaction 提交。VerifyCommand 与 FinalVerifyCommand 的首次成功响应是关联 VerificationIntent 的 `202 Accepted`，不是 verdict；同键重送可先收敛该 intent 后重放该初始响应。它与 ProjectInitializationReceipt 共享重放语义，但不共享恢复职责或物理记录。
_避免_：ProjectInitializationReceipt、Client 缓存、DomainEvent

**Actor**：
Command、Decision 或 DomainEvent 的来源归属。M3 仅用 human:local 与 daemon:local 表示本机来源，不能将它解释为已验证身份或授权主体。
_避免_：访问令牌、RBAC Principal、状态权威

**ControlPlaneError**：
Control Plane 对非成功请求返回的安全 ErrorEnvelope。稳定 code 用于区分输入、未找到、可由调用者处理的冲突、暂时不可用和内部错误；message 只供人阅读，不能包含 SQL、Git 原始错误或本机绝对路径。
_避免_：DomainEvent、任意 details JSON、基础设施异常直出

## Ticketize 与 Canonical Graph

**ImplementationTicket**：
`docs/FE20260903080401/tickets/` 中用于组织 Keystone 自身实施工作的版本化 Ticket。它不属于任何 Change 的 Canonical Ticket Graph。
_避免_：CanonicalTicket、Change 内执行单元

**StructuredTicketDraft**：
TicketGenerator 为一个 Change 的已验证 Plan 提供的候选 Ticket 集合。它在通过确定性校验并持久化前不具有权威性。
_避免_：CanonicalTicket、可直接执行的 Ticket

**TicketGeneration**：
将同一 Change、固定 BaseRevision 的已验证 Plan 和有界 ProjectContext.v1 转换为一份 StructuredTicketDraft 的单次候选尝试。它由一个 Ticketize AgentRun 产生候选，失败尝试可被追溯，但不能改变既有 Canonical Ticket Graph。
_避免_：CanonicalTicketGraph、Ticket 执行

**TicketGenerationCandidate**：
TicketGenerator 为一次 TicketGeneration 产生的有界原始候选内容。它可作为审计证据保存，但在通过 StructuredTicketDraft 校验前不表达 Ticket 或 Graph 事实。
_避免_：StructuredTicketDraft、CanonicalTicketGraph、Generator 成功声明

**TicketGenerationFailure**：
描述一次因 `generator_failed` 或 `draft_invalid` 未能形成 CanonicalTicketGraph 的有界失败证据。它保留稳定失败分类和候选摘要，不替代 AgentRun、HumanDecision 或权威图。
_避免_：自由错误日志、自动 retry、部分 Graph

**TicketizeUnavailable**：
Ticketize 在任何权威失败或成功事实提交前遇到 Artifact 或持久化不可用的暂时结果。它允许同一 Report 重送，不推进 Change 或占用人工恢复。
_避免_：TicketGenerationFailure、HumanDecision、已提交的 Graph

**TicketizeFenced**：
Ticketize 候选因 Pause、Cancel 或失效执行权而失去形成权威 Graph 的资格的结果。它可以保留为 Trace，但不表示 Draft 无效或基础设施不可用。
_避免_：TicketGenerationFailure、TicketizeUnavailable、Graph 提交

**FencedTicketGeneration**：
在 Pause、Cancel 或失效执行权已先成为权威事实后完成的 TicketGeneration。它的候选和 AgentRun 只保留为 Trace，不能形成 CanonicalTicketGraph；Resume 必须开始新的 TicketGeneration。
_避免_：可复用候选、自动 retry、取消后的 Graph

**GenerationKey**：
StructuredTicketDraft 内唯一、仅供该次 TicketGeneration 的 Ticket 相互引用的稳定键。它不是 CanonicalTicket 的身份，也不跨 Generation 解析。
_避免_：TicketID、跨 Change 引用、文档 Ticket 编号

**CanonicalTicket**：
由 Daemon 为一个 Change 权威持有、可独立理解和验证的执行切片，并独立持有不可变的 CanonicalTicketOrder 与 TicketExecutionState。它与生成候选、AgentRun 和版本化实施 Ticket 是不同概念。
_避免_：StructuredTicketDraft、AgentRun、ImplementationTicket

**CanonicalTicketOrder**：
Ticketize 成功时与 CanonicalTicketGraph 一同持久化的、仅在该 Change 内比较的不可变顺序；Scheduler 在同一 ExecutionSession 内以最小顺序选择 RunnableTicket，不能以 UUID、墙钟时间或 Worker 到达顺序替代它。
_避免_：TicketID 排序、StructuralFrontier、Worker 选择

**CanonicalTicketGraph**：
同一 Change 基于其已验证 Plan 形成的一次成功、不可替换的 CanonicalTicket 与 TicketDependency 集合。V1 不为既有成功图引入重生成、编辑或版本化。
_避免_：TicketGeneration、Effective Graph、版本化实施 Ticket 图

**TicketDependency**：
同一 CanonicalTicketGraph 内两个 CanonicalTicket 的受控前置关系。V1 仅表达 `BLOCKED_BY`。
_避免_：Execution DAG、跨 Change 依赖、任意关系类型

**BLOCKED_BY**：
CanonicalTicket 声明其必须等待的直接前置 CanonicalTicket；若 A 的 `BLOCKED_BY` 包含 B，则 B 是 A 的 blocker。
_避免_：反向依赖、文档 Ticket 的 `BLOCKED_BY`、执行授权

**StructuralFrontier**：
CanonicalTicketGraph 中没有 `BLOCKED_BY` blocker 的初始结构性节点集合。它不表示执行前置条件或授权已满足，不能替代 RunnableTicket。
_避免_：RunnableTicket、ready-for-agent、DaemonReadiness

**TicketizeCompletion**：
当前 Ticketize AgentRun 的候选 Draft 通过校验并与 CanonicalTicketGraph 一同成为权威事实的确认检查点。只有该检查点才将 Change 从 Ticketize 推进到 Execute。
_避免_：Generator 返回、仅完成的 AgentRun、Ticket 执行完成

**TicketGraphCreated**：
记录 CanonicalTicketGraph 首次成为一个 Change 权威事实的不可变 DomainEvent。它以 ticket graph 身份直接关联形成该图的 Plan、Draft 与 TicketizeCompletion，不表示任何 Ticket 已获执行授权。
_避免_：Generator 日志、StageAdvanced、Ticket 已完成

**TicketGraphReadModel**：
供 Client 只读观察 CanonicalTicketGraph、CanonicalTicket、TicketDependency 与 StructuralFrontier 的有界快照。它只使用 CanonicalTicket 身份，不公开只属于候选 Draft 的 GenerationKey，也不提供 Graph 生成、编辑或执行授权。
_避免_：CanonicalTicketGraph 的写接口、GenerationKey 查询键、Scheduler、Worker Assignment

**TicketGraphCommitIdentity**：
由 TicketizeAgentRunID 与 TicketGenerationCandidate 的内容身份组成的内部提交关联，用于收敛同一候选的重复 Graph 提交。它不是 Client 幂等键，也不由 GenerationKey 表达。
_避免_：GenerationKey、IdempotencyKey、可编辑 Graph 版本

## Execute 与 Change Workspace

**ExecuteCommand**：
由 Client 显式提交到 `POST /v1/changes/{change_id}/execute`、带有 ChangeVersion、IdempotencyKey 与可选 WorkspaceBranch 的 Change 执行准备请求；只有 Daemon 可以据此协调 Workspace 和后续执行，TicketizeCompletion、查询或 Worker 回调都不能隐式触发它。成功及同请求重放均以 `202 Accepted` 返回 ExecuteCommandResponse。
_避免_：自动调度、Worker Pull、Pause/Resume/Cancel ChangeCommand、自由 command 字符串

**ExecuteRequestIdentity**：
同一 Change 的 ExecuteCommand 经规范化后的幂等请求身份，包含省略时采用默认值的 WorkspaceBranch；相同 IdempotencyKey 只可重放同一身份的首次 accepted 结果，活跃 ExecutionSession 遇到不同键的重复请求以 execution_in_progress 拒绝。
_避免_：原始 HTTP body、每次 CLI 调用、可变 Workspace 路径

**ExecutionReadModel**：
供 Client 通过 `GET /v1/changes/{change_id}/execution` 观察 ExecutionSession、WorkspaceBranch、按 CanonicalTicketOrder 排列的 Ticket 状态、VerificationIntent/Outcome/Evidence 身份摘要、KeystoneCommit 前后 revision 与 CandidateRevision/FinalVerification 摘要的有界快照；CLI 以 `keystone change execution show CHANGE_ID` 读取。它不公开绝对 Workspace 路径、Lease token、原始 Prompt、命令输出或凭据，Evidence 内容仍通过 Artifact 读取边界获取。
_避免_：Workspace 文件浏览器、Worker 配置、Runtime 环境

**ExecuteCommandResponse**：
ExecuteCommand 被接受或同身份重放时返回的不可变公开回执，包含 Change、ExecutionSessionID、Session 状态、规范化 WorkspaceBranch、Ticket 状态和允许公开的 Artifact 身份摘要。相同 IdempotencyKey 但不同规范化请求为 idempotency_key_conflict；活跃 ExecutionSession 使用不同键为 execution_in_progress。
_避免_：Runtime 完成通知、WorkspacePath、Lease token、Prompt

**ExecutionSession**：
一次被接受的 ExecuteCommand 形成的、由 Daemon 持久化的 Change 级执行协调事实；它在 Workspace 与首个 ExecutionAuthorization 可恢复后成立，不等待 Worker、Lease 或 Runtime 完成。尚未发放 Lease 时，它保留其 FIFO 调度位置且不启动 Runtime，Worker 暂不可用不会撤销它。Pause 会释放其当前调度位置；Resume 以新的 ExecutionDispatchEpoch 重新入队，Cancel 则终止该 Session。
_避免_：HTTP 请求生命周期、DaemonReadiness、AgentRun

**ExecutionDispatchEpoch**：
ExecutionSession 可参与一次 Project 调度的持久化顺序身份。首次 Execute 以 Session 创建顺序形成初始 epoch；Pause 后 Resume 或 Execute 阶段 HumanDecision retry 必须形成新的 epoch 并排到 FIFO 队尾，不能恢复旧位置或插队。Worker 暂不可用时不创建新 epoch。
_避免_：UUID 排序、内存队列位置、暂停后插队

**WorkspaceProvisioningIntent**：
Daemon 在创建 Git Worktree 前持久化的 Workspace 候选，表达预期的 Change、BaseRevision、WorkspaceBranch 与规范化物理 WorkspacePath 身份；首次创建恢复必须核对 Git worktree list、物理顶层路径、branch 与 HEAD/BaseRevision，已形成 KeystoneCommit 的 Workspace 恢复则核对记录的当前 WorkspaceInputRevision。重试只能协调完全匹配的结果，未知或冲突的 Git 状态进入 human_required，不能自动删除、remove 或 prune。
_避免_：Workspace、成功回执、Git 垃圾回收

**Workspace**：
Daemon 为一个 Change 权威关联的、基于固定 BaseRevision、WorkspaceBranch 与 WorkspacePath 的 Git Worktree 执行空间；首次创建受 WorkspaceSourceRecheck 保护，同一 Change 串行复用它，Runtime 只能使用已授权的 Workspace。
_避免_：RepositoryBinding、每张 Ticket 的临时目录、Runtime 自选 cwd

**WorkspacePath**：
Daemon 私有的 Workspace 物理位置，由 LocalStateRoot、ProjectID 与 ChangeID 经 `filepath` 派生为 `<LocalStateRoot>/workspaces/<project-id>/<change-id>`，并在存在时按 EvalSymlinks 取得规范化物理身份；它不得与 RepositoryBinding.Root 重叠或互为子目录。默认 LocalStateRoot 可解析为 `~/.keystone`，但实现和 Client 都不得硬编码该用户目录，也不得通过 ExecutionReadModel 公开绝对路径。
_避免_：Client 输入路径、RepositoryBinding、公开文件浏览器

**WorkspaceSourceRecheck**：
首次创建 Workspace 前由既有 repository.Git 对 RepositoryBinding 作出的双读源快照复核：它必须是非 bare 主工作树，保持干净（包含 staged、unstaged 与 untracked，忽略文件除外）且 HEAD 等于 BaseRevision；不一致不创建 Workspace，并由 Daemon 进入 human_required。既有 Workspace 的恢复只核验其持久化身份，不能用新的源 HEAD 改写它。
_避免_：新的 BaseRevision、运行时 HEAD、Git 锁

**WorkspaceBranch**：
首次 ExecuteCommand 为一个 Change Workspace 确定的、经 `git check-ref-format --branch` 校验且首次创建时必须尚不存在的分支身份；默认使用 `keystone/change/<change-id>`，也可一次选择自定义值，创建后不可替换。首次创建以参数数组等价于 `git worktree add -b <branch> <path> <BaseRevision>` 执行。自定义值只能由显式 ExecuteCommand 作为本机人工批准提出，Daemon 不会自动选择它。
_避免_：当前 HEAD、可在重试时改写的 branch、Runtime 参数

**SourceControl**：
由 `internal/execution/application` 持有、以 `internal/infrastructure/sourcecontrol` 实现的受类型约束 Git Port。它复用 repository.Git 的主工作树发现和源快照复核，并以无 shell 的参数数组在执行边界中依据 RepositoryBinding、BaseRevision、WorkspaceBranch 与当前 WorkspaceInputRevision 创建、核验、快照和恢复 Workspace；Daemon 只组合它。它不接受 Client、Worker 或 Runtime 提供的任意 Git 命令文本，也不拥有 Ticket Graph、授权或生命周期状态。
_避免_：RepositoryBinding、Worker、通用 Git 代理

**TicketExecutionAuthority**：
Execution Application 面向 Work & Lifecycle 的窄 Port，用于在不暴露 Work Repository、Entity 或 SQL 的前提下核验 RunnableTicket，并在合法条件下完成 CanonicalTicket 与 Change 的状态推进。它只传递受控身份与结果，不让 Scheduler 直接改写 Work 权威。
_避免_：WorkStore、跨领域 Entity、Worker Report

**ExecutionPersistencePort**：
由 `internal/infrastructure/workstore` 实现、供 Execution Application 使用的窄复合持久化 Port。它的下一条实际 Schema Migration 建立 ExecutionSchema，并在一个 SQLite transaction 内原子关联执行状态、TicketExecutionEvidence、AgentRun、CanonicalTicket、Change 与审计事件；不向 Daemon 暴露跨表事务，也不使执行领域依赖具体 Store。
_避免_：Daemon SQL、分布式事务、WorkStore 具体类型

**ExecutionSchema**：
由实际已注册 Migration 链的下一条版本建立的执行持久化事实集合：ExecutionSession、WorkspaceProvisioningIntent、Workspace、ExecutionDispatchEpoch、ExecutionAuthorization、TicketExecutionState、WorkspaceSnapshot 与 TicketExecutionEvidence；它扩展既有 AgentRun 与 WorkerLease 的 Ticket/Authorization/Envelope/timeout/Claim 关联，而不建立平行 Assignment 表。SQLite 以复合外键、CHECK、partial unique index 与受限 transition trigger 强制单 Workspace、单未终结 Session、单活跃 Ticket Authorization、单 Project 写 Workspace Lease、单 Claim 与单成功证据等不变量。
_避免_：预占 Migration 版本、平行 Worker Assignment、应用层唯一性约定

**WorkspaceBoundary**：
对 Assigned Workspace 的可验证根与 Git 身份约束，拒绝路径逃逸或失效 Git root；它不从 Ticket 的自由文本 scope 推断文件 allowlist，也不等同完整操作系统沙箱。
_避免_：文本路径沙箱、RepositoryBinding、完整 OS 隔离

**ExecutionAuthorization**：
Daemon 依 DefaultExecutionAuthorizationPolicy.v1 对一个 RunnableTicket 形成的、可审计的一次执行授权事实；它持久化 policy_version、逐项判定结果与关联 ExecuteRequestIdentity。只有满足默认执行条件的授权才能创建关联的 AgentRun 与 Assignment，它可以在 Worker 暂不可用时保持待执行。自定义 WorkspaceBranch 只能由显式 ExecuteCommand 的受限本机人工批准形成此事实，Worker、Runtime 与自动 Scheduler 不能授予、扩大或撤销它。
_避免_：Lease、Worker 自报、通用风险标签

**DefaultExecutionAuthorizationPolicy.v1**：
不使用自由文本、LLM 评分或通用 Governance Aggregate 的结构性授权合取。它要求 active/Execute Change、有效且位于 Project FIFO 队首的 ExecutionSession/DispatchEpoch、身份匹配的 Workspace、当前 RunnableTicket、无该 Ticket 的活跃或已围栏 Authorization/AgentRun、固定 `edit`/`codex`、有效 InstructionInputProjection/timeout/输入摘要/WorkspaceBoundary，以及由初始 ExecuteCommand 记录的 branch 选择。Worker 暂不可用或未到队首保持等待；Artifact 或存储暂时不可用允许重试；Workspace、branch 或证据身份不变量失败进入 human_required；其余前置条件不满足以稳定冲突拒绝且不创建 Assignment。
_避免_：自然语言风险分类、Worker 自行授权、隐藏的管理员例外

**ExecutionEnvelope**：
ExecutionAuthorization 绑定的不可变实际执行输入：Change、ExecutionSession、CanonicalTicket、AgentRun/attempt、Workspace、BaseRevision、WorkspaceInputRevision、WorkspaceBranch、ExecutionMode、runtime、ExecutionInstruction、输入 Artifact 摘要与 timeout；任一字段变化都只能创建新的 Authorization 与 AgentRun。
_避免_：可变 Worker 参数、Runtime 自选输入、复用 Lease

**WorkspaceInputRevision**：
一次 Ticket Execute、Ticket Verify 或 FinalVerify 开始时 Daemon 固定的 Workspace HEAD commit identity。首张 Ticket 为 Change 的 BaseRevision；后续 Ticket 为前一张 KeystoneCommit 的 after revision；FinalVerify 为最后一张 KeystoneCommit 的 after revision。它是每个 ExecutionEnvelope 的 Git 不变量，不由 Client、Worker 或 Runtime 指定，也不替换 Change 的 BaseRevision。
_避免_：Change BaseRevision、任意当前 HEAD、Client revision 参数

**ExecutionMode**：
ExecutionEnvelope 声明的不可隐式升级执行模式。`inspect` 使用只读 Runtime 并只允许受限 Git 读取；`edit` 是 Ticket 09 默认模式，使用可写的已授权 Workspace 并额外允许 `git add`；`verify` 是 Ticket 10 的只读验证模式，只可执行 Manifest 固定的参数数组与独立审查，不被授予修改候选或运行 `git add` 的权限，任何观测到的候选变化都不能形成 PASS；`source_control` 只属于 Daemon 的 SourceControl，不可作为 Worker 或 Codex Runtime 参数。M8 不提供泛化的高权限 Runtime 模式。
_避免_：自由 sandbox 参数、Worker capability、管理员模式

**RuntimeGitPolicy**：
ExecutionMode 对经 Guard 中介 Git 调用施加的命令约束。`inspect` 与 `verify` 仅允许 `status`、`diff`、`rev-parse`、`show`、`log`、`ls-files` 与 `check-ignore`；`edit` 在此基础上允许 `add`；其余 Git 子命令全部拒绝。该策略不是完整 OS 沙箱，绝对路径 Git 或其他进程逃逸只能由 WorkspaceSnapshot、身份核验和证据完整性阻止成为成功。
_避免_：完整安全边界、任意 Git CLI、SourceControl

**ExecutionTimeout**：
Daemon 为一次 ExecutionEnvelope 固定的 `30m` Runtime 时长；从 Worker 实际启动 Runtime 时开始计时，必须同时传入 Assignment 和 Runtime context。Lease 续约、Client 参数或 Worker 本地配置都不能延长它；到期形成 `runtime_timeout`。
_避免_：Authorization 等待时长、可续租 deadline、Worker 默认值

**ExecutionInstruction**：
Daemon 从 CanonicalTicket 与已引用 Artifact 派生的有界执行指令，以摘要成为 ExecutionEnvelope 的组成部分；Client 不注入任意 Prompt，Worker 和 Runtime 不能改写其语义。M7 在授权前校验 Artifact 内容，并按引用顺序投影 UTF-8 文本；渲染结果总量不得超过 64 KiB。
_避免_：CLI 自由文本 Prompt、Runtime 自报计划、无界上下文

**InstructionInputProjection**：
Daemon 对 ExecutionInstruction 引用的 ArtifactContent 所作的、摘要已校验且按固定顺序的有界文本投影；它使 Runtime 消费的内容可由输入 Artifact 摘要与指令摘要复盘，但不向 Worker 授予 Artifact 拉取能力或本地挂载路径。临时读取失败保持可重试；缺失、摘要不符、非 UTF-8、媒体类型不支持或超出 64 KiB 时进入 human_required，且不创建 Assignment。
_避免_：Worker Artifact 下载、未校验 Prompt 拼接、二进制 Prompt

**ProjectExecutionSlot**：
一个 Project 在 V1 中同时至多拥有一个已发放 Lease、会写入 Workspace 的 Assignment；ExecutionSession 按 FIFO 排队且队首不因暂时无 Worker 被跳过，但在 Lease 发放前不占用该槽。Pause、Cancel 或授权被围栏时释放该执行槽，已保留的其他 Workspace 不占用它。
_避免_：InstanceLock、WorkerInstance、Workspace 的存在性

**ExecutionDispatch**：
Daemon 将 FIFO 队首的已授权 Ticket 交付给已注册 Worker 的唯一过程；它在同一权威事务创建 Assignment 与 Lease 并取得 ProjectExecutionSlot，但 Pull 仅可重放交付，不能启动 Runtime。只有成功 RuntimeClaim 后 Runtime 才可启动；未发放 Lease 的等待不产生运行中的 Runtime。
_避免_：Worker Pull 自行授权、预先启动的 Runtime、多个活跃 Assignment

**RuntimeClaim**：
Worker 为一份已交付 Assignment 生成的一次性启动请求，带有 Lease 与 RuntimeClaimID。Daemon 只将第一个有效 Claim 原子绑定为该 Lease 的启动资格；相同 Claim 可重放相同结果，其他 Claim、其他 Worker、失效 Lease 或被围栏 Assignment 都不能启动 Runtime。每个 Worker 同时至多持有一个已 Claim 或运行中的 Assignment。
_避免_：Pull 即执行、可复用 Lease、重复 Worker 进程

**ExecutionFence**：
对已发放 Lease 的 Assignment 或已取得 RuntimeClaim 的执行失去继续写入 Workspace 资格的权威事实。Lease 过期、Worker 丢失、Daemon 重启或 Heartbeat 续约失败都触发它：Worker 必须取消 Runtime context，并在有界宽限期后终止子进程；Daemon 将绑定 AgentRun 置为失败、Ticket 置为 human_required、释放 ProjectExecutionSlot。晚到 Report 仅保留 Trace，不能自动重派、重试或推进状态。
_避免_：Lease 自然过期、静默继续执行、自动恢复

**ExecutionControlFence**：
Pause 或 Cancel 对 Execute 阶段未发放或活动 Authorization/Lease 作出的显式围栏。Pause 取消 Runtime、释放 ProjectExecutionSlot，并将未成功的 assigned Ticket 回到 pending，保留 Workspace 和全部 Diff；Resume 只能以新的 ExecutionDispatchEpoch、Authorization 与 AgentRun 继续。Cancel 同样围栏和保留证据，但终止 Session 且永不重新排队、重试或自动清理。
_避免_：Pause 后继续运行、恢复旧 Lease、Cancel 自动清理

**Scheduler**：
Daemon 持有的执行协调能力。它以持久化的活跃 ExecutionDispatchEpoch FIFO 竞争 ProjectExecutionSlot：首次 epoch 使用 ExecutionSession 创建顺序，Pause 后 Resume 或 Execute retry 的新 epoch 置于队尾；取得槽后只从该会话所属 Change 的 RunnableTicket 中选择最小 CanonicalTicketOrder；授权与 AgentRun 创建原子地将该 Ticket 置为 assigned。每次 TicketExecutionSuccess 后，Scheduler 必须先收敛该 Ticket 的 TicketVerifyCommitGate，完成前不得授权任何下一张 Ticket；它不抢占已经取得的槽，也不替代 ExecutionAuthorization 或让 Worker、UUID、墙钟时间、Worker 到达顺序决定顺序。
_避免_：StructuralFrontier、Worker Poll、跨 Change 并行器、机会式公平

**WorkspaceSnapshot**：
SourceControl 在一次 AgentRun 的成功 RuntimeClaim 后、子进程启动前，或 Runtime 退出后、接受 Report 前，以私有临时状态观察到的 Workspace 不可变内容身份。它覆盖 HEAD、index 与 staged/unstaged tracked 内容，并用临时 index 形成 CandidateTreeIdentity；该内容必须以 WorkspaceInputRevision 为直接 HEAD 基线，用于确认该次执行没有复用先前 Ticket 的未提交变化，并作为生成 TicketDelta 的两端事实。新建文件必须在 edit ExecutionMode 内先由 `git add` 纳入 index；ignored 文件不阻塞，残留 untracked 文件使快照不能形成完整执行证据；快照不得修改实际 Workspace 或 index。
_避免_：当前 Workspace 路径、Git HEAD、Runtime 自报快照

**CandidateTreeIdentity**：
SourceControl 从完整 WorkspaceSnapshot 的 tracked 候选内容以私有临时 index 形成的 Git tree identity；它不改变实际 index，并且只有没有 residual untracked 文件时才有效。Commit 前由 `git add --all` 形成的 CommitTreeIdentity 必须与已验证的 CandidateTreeIdentity 相同。
_避免_：Git HEAD、未暂存 Diff、Worker 自报 tree

**TicketDelta**：
SourceControl 从一张 CanonicalTicket 的 AgentRun 前后两个 WorkspaceSnapshot 派生的有界、非空增量 Diff 与规范化 changed files；它用于归因该 Ticket 的实际贡献，不能替代完整 Workspace 状态证据。
_避免_：累计 Workspace Diff、Runtime 自报文件列表、Change 总结

**CompleteChangeDiff**：
Daemon 从 WorkspaceSnapshot(post) 相对于该 Ticket 的 WorkspaceInputRevision 形成的一次 Ticket 执行后当前完整未提交 Workspace 状态的规范化、非空变更证据，由覆盖已暂存和未暂存 tracked 修改的 Diff 与 changed files 共同表达。Worker 的原始 `diff` 与 `changed_files` 必须未截断、无采集失败并与该规范化状态一致；先前 KeystoneCommit 与当前 TicketDelta 共同表达 Change 的累计候选，残留 untracked 文件或任一采集缺失都会使证据不完整。
_避免_：Runtime 自报文件列表、局部 Diff、零 Diff

**TicketExecutionEvidence**：
Daemon 为同一受 Lease/RuntimeClaim 围栏的 Ticket 执行形成的强类型证据集合，关联 Execute AgentRun、CanonicalTicket、WorkspaceInputRevision、Worker 原始观察、WorkspaceSnapshot、CompleteChangeDiff 与 TicketDelta。Git 采集发生在提交事务前；其结果、原始 Artifact、AgentRun 终态、Ticket 状态与审计事件必须在同一权威事务落盘。暂时不可用时只允许同一 Report 重试，不推进状态；不变量失败进入 human_required。它不复用通用 Change 级 ArtifactRef 的角色/序号表达多次 Ticket 执行。
_避免_：Worker Report 本身、通用 ArtifactRef、执行后补写

**TicketExecutionState**：
CanonicalTicket 的权威执行状态；V1 只使用 pending、assigned、succeeded 与 human_required。pending 且所有 BLOCKED_BY 已 succeeded 的 Ticket 才可能 Runnable；执行失败进入 human_required，Pause 围栏的未成功 assigned Ticket 回到 pending，人工 retry 为同一 Ticket 建立新 AgentRun 后才回到 pending。
_避免_：AgentRunStatus、ChangeStatus、自动 skip

**TicketExecutionSuccess**：
Daemon 对一张 CanonicalTicket 的执行成功确认，要求其绑定 AgentRun 的退出码为零、ExecutionGuard 无发现、HEAD 始终保持该 Ticket 的 WorkspaceInputRevision、WorkspaceBranch 未变、无 residual untracked 文件，且完整的 TicketExecutionEvidence、非空 CompleteChangeDiff 与非空 TicketDelta 都成立；它只完成该 Ticket，只有同一 CanonicalTicketGraph 全部 Ticket 成功才使 Change 离开 Execute 进入 Verify。
_避免_：Runtime 成功文本、单个 AgentRun 推进整个 Change、Verify PASS

**TicketVerifyCommitGate**：
在 TicketExecutionSuccess 后、下一张 Ticket 获得 ExecutionAuthorization 前由 Daemon 收敛的串行门。它要求当前 CanonicalTicket 先通过 VerifyCommand、形成 PASS VerificationEvidence 并完成 CommitCommand；它不单独推进 Change LifecycleStage。所有 Ticket 均已完成该门后，Change 才从 Execute 原子进入 Verify。
_避免_：Change 级 Verify Stage、Worker 自主调度、历史 TicketDelta 补丁重建

## Verify、Commit 与 Integrate Ready

**VerificationCommand**：
ProjectManifest V2 中按稳定顺序声明、由 VerificationPolicySnapshot 固定的确定性验证调用；一个 Project 必须声明 1 至 32 条名称唯一的命令，每条具有规范名称、非空参数数组与 1 至 1800 秒的 timeout。它在 Workspace root 以 `verify` 模式、经清理的环境运行，不接受隐式 shell、自由 cwd、自由环境变量或忽略失败；完整声明及其 VerificationConfigurationDigest 是 VerificationEvidence 的输入，而不是 Client 临时提交的 shell 文本。
_避免_：Runtime 自由指令、未记录的 shell 展开、Implementer 自报测试结果

**VerifierAgentRun**：
与 Implementer 不同的 Verify 或 FinalVerify AgentRun；它可复用同一 Worker 或 Runtime，但只审查固定 WorkspaceSnapshot、按顺序的 CanonicalTicket Acceptance Criteria 与已采集证据，并以结构化结果逐项给出 PASS、FAIL 或 HUMAN_REQUIRED，不拥有修复候选或创建 Git Commit 的权限。Daemon 必须确认其运行前后候选 WorkspaceSnapshot 相同；V1 只保证这一可观察边界，不把它称为完整 OS 隔离。
_避免_：Implementer AgentRun、Runtime 自报批准、完整沙箱

**AcceptanceCriterionRef**：
对 CanonicalTicket 内一条不可变 Acceptance Criterion 的稳定引用，由 TicketID、1 起始 ordinal 和规范化文本的 SHA-256 组成；它复用 Ticket 08 已持久化的 ordinal/text，不引入第二套 Criterion ID。
_避免_：自由文本匹配、GenerationKey、可变验收条件

**AcceptanceCriterionResult**：
VerifierAgentRun 对一个 AcceptanceCriterionRef 给出的结构化 PASS、FAIL 或 HUMAN_REQUIRED 观察及其 Evidence 引用。一个 VerificationEvidence 必须按 ordinal 恰好覆盖该 Ticket 的每条 Criterion；重复、遗漏、乱序或摘要不符使结果为 HUMAN_REQUIRED。
_避免_：无归属审查摘要、全 Ticket 单一自由文本 verdict、AgentRunOutcome

**VerificationOutcome**：
对 CanonicalTicket 或整个 Change 的权威验证结论，取值为 PASS、FAIL 或 HUMAN_REQUIRED，且不同于 AgentRunOutcome。PASS 要求每条已声明 VerificationCommand 成功、Workspace/Git Checks 与独立验收审查逐项 PASS；任一命令非零退出或 timeout 为 FAIL，并将后续命令记录为 `not_run`；缺失、损坏或无法判定的审查证据为 HUMAN_REQUIRED。FAIL 与 HUMAN_REQUIRED 均不得创建 KeystoneCommit 或推进 Change。
_避免_：AgentRunOutcome、进程 exit code、ChangeStatus

**VerificationCommandResult**：
一条已声明 VerificationCommand 的不可变执行观察，按 Manifest 顺序记录 `succeeded`、`failed`、`timed_out` 或 `not_run`、可用时的 exit code、受限输出 Artifact 与采集 Worker/AgentRun。它不自行构成 VerificationOutcome。
_避免_：未经声明的临时检查、Verifier 自由文本、ChangeStatus

**VerificationEvidence**：
形成 VerificationOutcome 的不可变证据集合，关联 VerificationPolicySnapshot、验证前后 WorkspaceSnapshot、输入 revision、有序 VerificationCommandResult、完整的 AcceptanceCriterionResult 与采集 Worker/AgentRun；Implementer 的摘要只能作为审查输入，不能单独构成 PASS Evidence。
_避免_：Runtime 成功文本、无归属日志、可变命令配置

**VerificationUnavailable**：
在任何 PASS、FAIL 或 HUMAN_REQUIRED 成为权威事实前发生的暂时 Artifact 或持久化不可用。它不是 VerificationOutcome，不推进 Change；同一幂等 VerifyCommand 或 FinalVerifyCommand 可重新驱动对应 VerificationIntent 并重放初始 `202 Accepted`。未发放 Lease 时的 Worker 不可用保持 intent pending；已 Claim 后发生的 Lease/Runtime fence 仍遵循既有 human_required 围栏。
_避免_：FAIL、自动补救、绕过 LeaseFence 的重试

**VerificationIntent**：
由已接受 VerifyCommand 或 FinalVerifyCommand 持久化的、尚未形成权威 verdict 的不可变验证候选；它固定请求身份、VerificationPolicySnapshot、WorkspaceInputRevision、WorkspaceSnapshot 与 Criterion 引用。它在无可用 Worker 时保持 pending，只有 Assignment/Lease/RuntimeClaim 就绪后才创建 VerifierAgentRun；同一请求在 unavailable 后只能重新驱动同一 intent，不创建并发第二个活跃尝试。
_避免_：VerificationOutcome、可变 Client 请求、自动重试队列

**VerificationRecovery**：
VerificationOutcome 后唯一允许的恢复边界。Ticket Verify 的 FAIL 经 HumanDecision retry 后，只能为同一未 Commit 的 CanonicalTicket 创建新的 Execute AgentRun；其执行前 WorkspaceSnapshot 必须仍等于失败 VerificationEvidence 的候选 Snapshot，允许同一 Ticket 在已知候选上增量修复但不混入其他 Ticket 的未提交变化。仅由验证证据不足形成的 Ticket HUMAN_REQUIRED 可在同一 WorkspaceInputRevision 和 CandidateTreeIdentity 下创建新的 VerifierAgentRun；由 Fence、候选变化或未知 Workspace 状态形成的 HUMAN_REQUIRED 必须先由人将 Workspace 恢复到相应耐久 Snapshot，否则保持 human_required。Daemon 不自动 reset、clean、amend 或删除候选。FinalVerification 的 FAIL 不得重开、amend 或重置已 Commit 的 Ticket，修复只能由以 CandidateRevision 为起点的新 Change 完成。FinalVerification 的 HUMAN_REQUIRED 只可在 CandidateRevision 和 WorkspaceSnapshot 未变时重新执行 FinalVerify。
_避免_：自动 retry、已提交 Ticket 修订、绕过 Verifier 的人工 PASS

**VerifyCommand**：
Client 通过 `POST /v1/changes/{change_id}/tickets/{ticket_id}/verify`、请求体 `expected_version` 与 `Idempotency-Key` 提交的 Ticket Verify 请求。Daemon 只在指定 CanonicalTicket 已达到 TicketExecutionSuccess 且正占有 TicketVerifyCommitGate 时接受它，持久化或收敛 VerificationIntent 并返回 `202 Accepted`；Worker 就绪后才创建 VerifierAgentRun。Client 不传入命令、Prompt、Workspace 或 verdict。
_避免_：Worker Report、Runtime 触发、自由验证脚本

**CommitCommand**：
Client 通过 `POST /v1/changes/{change_id}/tickets/{ticket_id}/commit`、请求体 `expected_version` 与 `Idempotency-Key` 提交的 KeystoneCommit 请求。Daemon 只在指定 CanonicalTicket 的当前 VerificationEvidence 已形成 PASS 后接受它，并以 CommitIntent 协调唯一 Git Commit；只有 KeystoneCommit 已记录后才返回成功回执。
_避免_：Runtime git commit、PASS 自报、无版本前置条件的重试

**FinalVerifyCommand**：
Client 通过 `POST /v1/changes/{change_id}/final-verify`、请求体 `expected_version` 与 `Idempotency-Key` 提交的 Change 级 FinalVerification 请求。Daemon 只在所有 CanonicalTicket 都已有 KeystoneCommit 时接受它，持久化或收敛 VerificationIntent 并返回 `202 Accepted`；Worker 就绪后才针对最终 HEAD 形成 CandidateRevision 或 VerificationOutcome。
_避免_：任一 Ticket PASS、自动 merge、Client 指定 revision

**CommitTemplate**：
ProjectManifest V2 可选的受校验提交消息模板，只能引用 `{change_id}`、`{ticket_id}` 与 `{ticket_title}` 受控占位符；缺省时 Daemon 基于 CanonicalTicket title 生成消息。CommitTemplateDigest 固定其输入；每个 KeystoneCommit 都必须附加受控的 Change、Ticket 与 KeystoneCommitID trailer，模板不能移除或伪造它们。
_避免_：Client 自由 commit message、Git hook 输出、无关联提交

**KeystoneCommit**：
Daemon 在 CanonicalTicket Verify PASS 后、响应 CommitCommand 创建并记录的 Git Commit 事实，关联该 Ticket 的 WorkspaceInputRevision 作为 before revision、after revision、CommitTreeIdentity 与 VerificationEvidence。它使用 Workspace 已配置的 Git author/committer；身份缺失进入 HUMAN_REQUIRED，并以 `--no-verify` 排除未记录的 hook 副作用。其受控 trailers 固定为 `Keystone-Change-ID`、`Keystone-Ticket-ID` 与 `Keystone-Commit-ID`。它不表示 TicketGraph 提交、SQLite transaction 或 Runtime 执行。
_避免_：TicketGraphCommitIdentity、数据库提交、Runtime git commit

**CommitTreeIdentity**：
Daemon 在重验已验证 WorkspaceSnapshot 后以 `git add --all` 形成的完整 index tree 身份；它必须等于该 Snapshot 的 CandidateTreeIdentity，不能由验证后新出现的文件或变化扩大。
_避免_：未暂存 Diff、Git reflog、Client 文件列表

**KeystoneCommitID**：
CommitIntent 生成并写入 `Keystone-Commit-ID` trailer 的不可变关联标识，用于在 Git 与 SQLite 之间唯一对账，不替代 CanonicalTicketID 或 Change ID。
_避免_：TicketGraphCommitIdentity、Git OID、IdempotencyKey

**CommitIntent**：
Daemon 在调用 Git 前持久化的 KeystoneCommit 候选，固定 CanonicalTicket、预期 parent revision、CommitTreeIdentity、VerificationEvidence、CommitTemplateDigest、KeystoneCommitID 与请求身份。重试或重启必须以该意图、直接 parent/tree 和受控 Git trailer 对账；任何缺失、歧义或冲突进入 human_required，不得盲目创建第二个 Commit。
_避免_：已完成 KeystoneCommit、Client 幂等键、Git reflog 推测

**CandidateRevision**：
所有 CanonicalTicket 均已有 KeystoneCommit 后，Change 在 FinalVerification 中被验证的最终 Git HEAD。它不同于创建 Change 时固定的 BaseRevision；只有 FinalVerification PASS 才使其成为 integrate_ready 的可查询事实。
_避免_：BaseRevision、未提交 Diff、任意 Workspace HEAD

**FinalVerification**：
在所有 CanonicalTicket 已 Commit 后、针对干净最终 HEAD 进行的 Change 级独立验证。它使用 ExecuteCommand 时固定的 VerificationPolicySnapshot，按 Manifest 声明顺序执行 VerificationCommand；若首个命令失败或 timeout，后续命令记录为 `not_run`，否则全部执行，并由独立于所有 Implementer 的 VerifierAgentRun 审查 ChangeIntent、全部 CanonicalTicket Acceptance Criteria 与已有 Evidence；PASS 后该 HEAD 成为 CandidateRevision。它不是任一单 Ticket Verify 的重命名，也不执行 merge、push、PR、deploy 或自动回滚。
_避免_：Ticket Verify、Git 集成、部署检查

## 工作流与端点

**RunnableTicket**：
处于 pending、所有 BLOCKED_BY 已 succeeded、所属 Change 有 active ExecutionSession 与有效 Workspace，且同一会话不存在未完成 TicketVerifyCommitGate 的 CanonicalTicket；它可由 Scheduler 考虑，但尚未获得 ExecutionAuthorization，不能仅凭 Runnable 身份生成 Assignment。
_避免_：StructuralFrontier、ready-for-agent、规划就绪

**TicketScope**：
CanonicalTicket 中解释预期变更意图的语义范围；V1 的自由文本 Scope 不是机器可执行的文件 allowlist，若需要强制路径范围必须由后续 Ticket Draft 引入显式结构化字段。
_避免_：路径沙箱、Git ignore 规则、Runtime 命令白名单

**PlanningReadiness**：
Ticket 或规格的描述已足以被实现者认领的文档状态。它不会解除 BLOCKED_BY，也不会证明实现已完成。
_避免_：RunnableTicket、DaemonReadiness

**DaemonReadinessEndpoint**：
专门报告 DaemonReadiness 的本机 HTTP 端点。它不表示任何被 Keystone 管理的 Repository 服务状态。
_避免_：Demo Service Health Endpoint、业务健康检查

**RefreshHint**：
Daemon 在权威事实写入后向 Dashboard 发送的有界刷新提示；它只包含受控 resource_type 及可选 Project/Change 关联，不携带状态、Artifact、Command、Decision 或 replay 信息。Client 必须通过 Query 获取新快照，不能将 RefreshHint 当作业务事件或状态来源。
_避免_：SSE 状态同步、可靠事件队列、Event Replay

**DemoServiceHealthEndpoint**：
Golden Path 中被 Keystone 管理的示例服务自身的健康检查端点。它与 DaemonReadinessEndpoint 属于不同系统主体。
_避免_：DaemonReadinessEndpoint

**DemoAcceptanceCriteria**：
Golden Path demo candidate 必须同时满足的六条固定验收约束，覆盖原 `GET /` 行为、DemoServiceHealthEndpoint 语义和确定性自动化测试。它属于 Golden Path E2E 契约，不是任一 CanonicalTicket 的 AcceptanceCriterion。
_避免_：CanonicalTicket AcceptanceCriterion、DaemonReadiness 检查、仅格式化测试

## Golden Path 验收

**GoldenPathFixture**：
仓库内保存的不含 `.git` 的 Go HTTP demo 初始源，可包含已版本化的 ProjectManifest V2；每次 E2E 将其复制到临时目录并执行 `git init`，不把嵌套 Git Repository 纳入 Keystone 主仓库。
_避免_：共享临时 Repository、携带历史的 fixture、主仓库子模块

**GoldenPathKeystoneRevision**：
一次 GoldenPathRun 用于构建 Keystone 二进制的完整、不可变 Git OID，由操作者显式指定。它不是 demo 的 BaseRevision、CandidateRevision、分支名或调用者工作树的当前 HEAD。
_避免_：BaseRevision、CandidateRevision、分支名、当前 HEAD

**GoldenPathRunID**：
Runner 在任何 Codex 预检、Daemon 启动或公开写入前，为一个新建受控临时根生成的 UUIDv7 标识。它只标识一次验收尝试，不表示 Change、AgentRun 或审阅结论。
_避免_：ChangeID、AgentRunID、GoldenPathReviewID、临时目录绝对路径

**GoldenPathEvidenceSet**：
以规范化输入 manifest 及其 SHA-256（GoldenPathEvidenceSetID）标识的一组 Golden Path 不可变共同输入，至少绑定 Keystone revision、fixture tree、ChangeIntent、Runner script 和 Dashboard lockfile。只有具有相同 GoldenPathEvidenceSetID 的 Linux 与 WSL PASS 记录可共同支持 V1 结论。
_避免_：平台运行时版本、单次 GoldenPathRun、可变工作树

**GoldenPathRun**：
从 `init` 到 Dashboard Trace 的一次完整验收尝试，在独立且复核前保留、具有 GoldenPathRunID 的临时 Repository 与 LocalStateRoot 中由公开 CLI 或 Control Plane API 驱动并记录实际结果；所有 runtime-backed AgentRun 使用真实 Codex，直接写 SQLite、调用内部 Service 或用 debug 接口伪造状态不属于 GoldenPathRun。
_避免_：单元测试、局部 smoke、mock Runtime 代替的完整链路

**GoldenPathRunner**：
一个显式调用、可重复且有界的本机验收编排器；它从 GoldenPathKeystoneRevision 的 archive 内脚本驱动，为每次具有 GoldenPathRunID 与 GoldenPathEvidenceSetID 的 Run 隔离并在复核前保留现场，只提交公开 Command、读取公开 Query 并采集证据。它只为发现自身 DaemonEndpoint 而受限读取其 LocalStateRoot 的 RuntimeMetadata，不被普通测试自动触发，也不拥有业务状态或恢复决策权；它不自行发布成功 Evidence 或动态取得未预先固定的浏览器工具。
_避免_：测试 fixture、内部 Application 驱动器、数据库脚本

**GoldenPathCommandLedger**：
一个 GoldenPathRun 在受控临时根中保存的非权威命令重放记录，将一次逻辑公开写入固定关联到其规范请求、ChangeVersion 和 IdempotencyKey。它只限制 Runner 的重送，不替代 Daemon 的 CommandReceipt 或权威状态。
_避免_：CommandReceipt、Daemon 账本、可变重试草稿

**GoldenPathCodexPreflight**：
GoldenPathRunner 在启动 Daemon、创建 Project 或写入 GoldenPathCommandLedger 前，对指定 Codex executable 的有界、非交互外层可用性检查。它只固定可执行文件、版本、认证状态和受控 PATH 的安全摘要；通过不构成 RealCodexAcceptance，失败也不伪造 Worker 或 AgentRun 事实。
_避免_：Worker capability 自报、DaemonReadiness、RealCodexAcceptance

**GoldenPathBrowserObservation**：
一个 GoldenPathRun 在 Daemon 托管的生产 Dashboard 上形成的可复核浏览器观察，包含真实 Query、EventSource 断线重连和页面刷新后的状态重建。它只使用由 Dashboard lockfile 固定、在 canonical Run 前显式准备并由 Runner 验证的 Playwright/Chromium；它不是 dev server、mock payload、浏览器缓存或仅有截图。
_避免_：前端状态模拟、静态截图、Dashboard 单元测试

**GoldenPathBrowserProvision**：
canonical GoldenPathRun 开始前，操作者使用已由 Dashboard lockfile 固定的本地 Playwright CLI 准备匹配 Chromium 的显式前置步骤。Runner 只接受显式传入并验证的浏览器目录，不通过 `npx` 或隐式下载取得 package 或浏览器；该前置步骤通过不构成 GoldenPathBrowserObservation。
_避免_：Run 中动态下载、Vite dev server、浏览器观察成功

**RealCodexAcceptance**：
Worker 在一次 runtime-backed AgentRun 中实际启动 Codex CLI 并形成可复核候选或审查结果的验收事实；Execute 阶段必须形成源码修改，Planning/Verify 阶段可分别形成 candidate 或只读审查结果，且必须记录 Codex 版本和真实进程结果。Fake Runtime、版本探针和交叉编译不能替代它。
_避免_：Runtime 自报成功、fake Codex、交叉编译运行证据

**GoldenPathPlatformProvenance**：
GoldenPathRunner 为声明的 `linux` 或 `wsl` 平台形成的脱敏本机来源声明，包含受限系统探针、WSL/container marker、host/VM 声明和安全命令投影。它供独立审阅核对而不声称密码学地证明宿主环境；不匹配或不确定的声明不能支持 PASS。
_避免_：跨编译产物、Docker 内的 WSL 替代证据、密码学远程证明

**GoldenPathReviewPacket**：
每个终态 GoldenPathRun 在受控临时根中形成的脱敏、完整性可核对输入包。其规范 manifest 逐项列出材料 role、media type、byte length 和 SHA-256，manifest 的 SHA-256 是 packet digest；失败包还固定终态阶段、最后 checkpoint、失败类别和退出码。
_避免_：成功 Evidence、原始敏感日志、可变人工摘要

**GoldenPathReviewID**：
独立审阅者为一次固定 GoldenPathReviewPacket 生成的 UUIDv7 标识。它将审阅角色、独立性声明和 PASS、FAIL 或 UNVERIFIED 结论绑定到特定 GoldenPathRunID、GoldenPathEvidenceSetID 与 packet digest，不表示 AgentRun 或发布成功。
_避免_：GoldenPathRunID、AgentRunID、GoldenPathEvidenceSetID、成功 Evidence

**GoldenPathReview**：
非该 Run 执行者或 Runner 的独立审阅者，针对一个固定 GoldenPathReviewPacket 作出的只读结论。它以 GoldenPathReviewID 绑定 GoldenPathRunID、GoldenPathEvidenceSetID、packet digest、非敏感角色和独立性声明，结论只能为 PASS、FAIL 或 UNVERIFIED；只有 PASS 允许由人工将该平台 Run 追加到 GoldenPathEvidence。
_避免_：Runner 自检、Runtime outcome、未复核的成功摘要

**GoldenPathEvidence**：
按平台追加保存且已获 GoldenPathReview PASS 的成功 GoldenPathRun 脱敏复盘记录，分别覆盖 Keystone Source、Demo Candidate 和 GoldenPath Trace 三条不能互相替代的证据链；每条记录绑定 GoldenPathRunID、GoldenPathEvidenceSetID、GoldenPathReviewID 与 packet digest。它必须标识实际运行平台，使 Linux 与 WSL 不可复用同一 Run，也不得包含 secret、token 或本机绝对路径，且未完成真实 Codex 验收时不得伪造成功记录。
_避免_：设计计划、仅有测试日志的记录、未验证的运行摘要

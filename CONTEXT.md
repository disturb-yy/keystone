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
Repository 持有的版本化 ProjectIdentity 表达；它不拥有 Project 当前权威状态，也不承载本机运行状态。
_避免_：Keystone DB、RuntimeMetadata、用户级状态目录

**ProjectManifestVersion**：
ProjectManifest 可被当前 Control Plane 解释的版本边界。不兼容或含义不明确的 Manifest 必须被拒绝，不能通过静默丢弃字段取得协调成功。
_避免_：SchemaMigrationVersion、应用版本、自动修复标记

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
由 Daemon 权威持有、绑定一个 Project 与不可变 BaseRevision 的长期变更意图及其生命周期事实。
_避免_：单次 AgentRun、Git Worktree、Client 本地草稿

**BaseRevision**：
在 Change 创建时确认并固定的源 Repository 版本快照；后续生命周期不得用当前 HEAD 覆盖它。
_避免_：运行时 HEAD、候选提交、用户输入的 revision

**ChangeSourceSnapshot**：
创建 Change 时对干净 RepositoryBinding 作出的只读版本确认，由 BaseRevision 表达其固定版本。
_避免_：Git Worktree、运行时工作目录、Client 传入的 revision

**ChangeIntentArtifact**：
首次创建 Change 时保存的不可变意图 Artifact，用于后续 Stage 的输入；它不是 ProjectInitializationIntent。
_避免_：ProjectInitializationIntent、可编辑描述、运行日志

**LifecycleStage**：
Change 最近一个已确认、已持久化的生命周期检查点，按 Intent、Understand、Design、Plan、Ticketize、Execute、Verify、FinalVerify 的既定顺序前进。
_避免_：当前进程状态、Client 可直接设置的字段、ChangeStatus

**ChangeStatus**：
决定 Change 是否允许继续协调的运行控制状态；V1 使用 active、paused、human_required、cancelled 与 integrate_ready，且 integrate_ready 只在 FinalVerify 成功后出现。
_避免_：LifecycleStage、AgentRun outcome、Verify 结果

**Artifact**：
与 Change 生命周期有关、内容不可变且可完整性校验的持久化输入、输出或证据。
_避免_：可编辑文档、临时内存值、Event payload

**ArtifactRef**：
将业务事实关联到一个 Artifact 的可查询引用，保留其定位、摘要与完整性标识而不复制内容。
_避免_：Artifact 内容副本、可变文件路径、运行时日志流

**DomainEvent**：
在权威业务事实发生时追加的不可变审计记录；它解释状态如何到达当前值，但不以重放替代权威状态。
_避免_：应用日志、Worker 自报、完整 Event Sourcing

**EventSequence**：
同一业务聚合内 DomainEvent 的单调发生顺序，用于可靠复盘而不依赖 UUID 或时间戳排序。
_避免_：UUIDv7 顺序、全局因果顺序、日志行号

**CommandReceipt**：
与一个幂等键及其规范化 Command 绑定的持久化结果，使相同请求可重放而不同请求不能复用同一键。
_避免_：Client 缓存、Event、一次性 HTTP 响应

## 工作流与端点

**RunnableTicket**：
依赖与执行前置条件均已满足、可进入执行 frontier 的 Ticket。它不等同于文档已准备好或 Daemon 已就绪。
_避免_：ready-for-agent、规划就绪

**PlanningReadiness**：
Ticket 或规格的描述已足以被实现者认领的文档状态。它不会解除 BLOCKED_BY，也不会证明实现已完成。
_避免_：RunnableTicket、DaemonReadiness

**DaemonReadinessEndpoint**：
专门报告 DaemonReadiness 的本机 HTTP 端点。它不表示任何被 Keystone 管理的 Repository 服务状态。
_避免_：Demo Service Health Endpoint、业务健康检查

**DemoServiceHealthEndpoint**：
Golden Path 中被 Keystone 管理的示例服务自身的健康检查端点。它与 DaemonReadinessEndpoint 属于不同系统主体。
_避免_：DaemonReadinessEndpoint

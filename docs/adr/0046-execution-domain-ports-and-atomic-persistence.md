# ADR-0046：执行领域 Port 与原子持久化边界

## 状态

已接受。

## 决策

现有 `internal/execution` 根 package 保持 RuntimeAdapter、Guard、输入输出和证据采集的基础职责，不直接加入 Scheduler、Workspace 或 SQLite 业务状态。Ticket 09 新增的执行模型置于 `internal/execution/domain`，用例置于 `internal/execution/application`；两者只依赖自身领域模型和窄 Port。

Execution Application 通过 TicketExecutionAuthority 与 Work & Lifecycle 协作，而不取得 Work Repository、Entity 或 SQL 访问。受类型约束的 Git SourceControl 由 `internal/infrastructure/sourcecontrol` 实现。现有 `internal/infrastructure/workstore` 实现 ExecutionPersistencePort，在同一 SQLite transaction 中关联执行状态、证据、AgentRun、CanonicalTicket、Change 与审计事件；Daemon 仅组合 adapter 与 Handler，不公开跨表事务。

新增 Go package 时，必须同步创建其局部 `AGENTS.md` 和 `INDEX.md`。

## 理由与边界

执行成功需要同时满足 Ticket、AgentRun、Evidence 与 Change 的条件，拆成多个独立 SQLite 写入会失去原子性；将事务收口为一个基础设施 Port 能保持领域方向，同时避免 Daemon 成为 SQL 编排层。保留现有 Runtime package 则避免把已有 Worker 基础能力与新的业务协调耦合。

该 ADR 是 Ticket 09 的目标代码组织，不创建目录、不迁移现有 Runtime，也不证明任何 Port、Migration 或 Git adapter 已实现。

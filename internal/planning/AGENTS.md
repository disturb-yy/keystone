# `internal/planning` 局部规约

## 边界

该 package 是 Intelligence & Planning 的窄 Application/Contract 边界，负责
Understand、Design、Plan、Ticketize 的 Context、结构化 Contract、schema validator、Stage
Strategy、prompt/decoder 和 Coordinator port；Ticketize 只产生候选，不直接创建权威 Graph。

## 权威关系

- `internal/work` 持有 Change、AgentRun、Lifecycle 和人工恢复的权威模型；Planning
  只能通过公开 Application/State port 协调，不复制或绕过这些状态。
- `internal/infrastructure/workstore` 持有 SQLite、Migration、ArtifactRef、Event 和
  AgentRun 的具体持久化；Planning 不写 SQL。
- `internal/infrastructure/repository` 提供固定 revision 的隔离 Snapshot adapter；
  Planning 不修改原始 Repository，也不把宿主机绝对路径写入 ProjectContext 或
  Runtime prompt。
- Ticket 06 提供 Worker/Runtime 的窄执行边界；Planning 不依赖 Worker 进程、Codex
  命令行或 Runtime 的具体实现。

## 依赖和不变量

- 阶段顺序固定为 `Understand -> Design -> Plan -> Ticketize`；Ticket 07 只生成前三类
  已验证候选，Ticket 08 的 Draft 仍不是 Canonical Graph，Graph 由 Work authority 提交。
- 所有阶段使用不可变 `base_revision`、单一上游 Artifact 和有界
  `ProjectContext.v1`；上游 Artifact 必须经过严格 decoder/validator 才能进入下游。
- Runtime 只提供候选 payload 和执行观察事实；只有 Coordinator/Daemon 在验证后才能
  形成权威 Artifact、AgentRun 终态和 Change 推进；Ticketize 成功还必须经过专用
  Work completion port。
- 失败、暂停、取消、旧 attempt、失效 Lease 和晚到结果遵循 Work/Worker fencing，
  不得复活或覆盖权威生命周期。
- 所有可能阻塞或执行 I/O 的 port 接收 `context.Context`；不得把 Context 存入长生命
  周期 struct。

## 实现纪律

- 不写 HTTP Handler、SQL、SQLite 连接管理、Worker Protocol 路由或 Runtime concrete
  调用。
- Prompt 只携带经过校验的项目元数据、固定 revision 和当前阶段授权输入，不携带协议
  secret、数据库连接、宿主机绝对路径或运行环境对象。
- `Prepare` 与 `Decode` 必须可被异步 Coordinator 分开调用；同步 `Execute` 只作为
  fake Runtime 可验证的便利 seam，不能成为生产调度的强制形态。
- 新增 Go 文件时同步更新本目录 `INDEX.md`；新增子 package 前必须先重新对齐目录所有权
  并同步创建局部 `AGENTS.md`/`INDEX.md`。
- 文档和注释使用中文；技术标识符、schema version、API path 和命令保持原文。

## 验证

Planning 核心实现必须具备不启动 HTTP、SQLite、Worker 或真实 Runtime 的纯单元测试。
按变更范围执行聚焦测试，再执行根级 `go test ./...`、`go vet ./...`、`make build` 和
`git diff --check`。

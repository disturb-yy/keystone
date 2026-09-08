# `internal/planning` 局部规约

## 边界

该 package 是 Intelligence & Planning 的窄 Application/Contract 边界，负责 Understand、Design、Plan 的 Context、结构化 Contract、schema validator、Stage Strategy、prompt/decoder 和 Coordinator port；Ticketize 只在此定义 TicketGenerator、StructuredTicketDraft 和候选校验。当前 checkout 只有本规约与 `INDEX.md`，没有已实现的 Go 行为。

## 权威关系

- `internal/work` 持有 Change、AgentRun、Lifecycle 和人工恢复的权威模型；Planning 通过公开 Application/State port 协调，不复制或绕过这些状态。
- `internal/infrastructure/workstore` 持有 SQLite、Migration、ArtifactRef、Event 和 AgentRun 的具体持久化；Planning 不写 SQL。
- `internal/infrastructure/repository` 提供固定 revision 的 Snapshot adapter；Planning 不修改原始 Repository，不把宿主机绝对路径泄漏到边界。
- Ticket 06 提供 Worker/Runtime 的窄执行边界；TicketGenerator 通过受限 Ticketize AgentRun 获取候选，Planning 不依赖 `cmd/keystone-worker`、Codex 命令行或 Runtime 的具体实现。

## 依赖和不变量

- 阶段顺序固定为 `Understand → Design → Plan → Ticketize`，一个 Change 同时最多一个当前阶段的 Planning AgentRun；Ticketize 只消费已验证 Plan。
- 所有阶段使用不可变 `base_revision` 和有界 `ProjectContext.v1`；上游 Artifact 必须经过 validator 才能进入下游。
- Runtime 只提供候选 payload 和执行事实；只有 Coordinator/Daemon 在验证后才能形成 Artifact、AgentRun 终态和 Change 推进。
- 失败、暂停、取消、旧 attempt、失效 Lease 和晚到结果遵循 Work/Worker fencing，不得复活或覆盖权威生命周期。
- 所有可能阻塞或执行 I/O 的 port 优先接收 `context.Context`；不得把 Context 存入长生命周期 struct。

## 实现纪律

- 不写 HTTP Handler、SQL、SQLite 连接管理、Worker Protocol 路由或 Codex/其他 Runtime concrete 调用。
- Ticketize 在本 package 中只产出并校验 StructuredTicketDraft；CanonicalTicketGraph 的领域不变量、权威提交、查询、SQLite 和 Migration 分别归 Work Domain、Work Application、Daemon/Contract 与 Workstore。
- 以候选 Contract 为边界实现 TicketGenerator；生产 Worktree、Scheduler、Verify 和完整 Risk/Policy/Gate 仍由后续专题拥有。
- 新增 Go 文件时同步更新本目录 `INDEX.md`；新增子 package 前必须先重新对齐目录所有权并同步创建局部 `AGENTS.md`/`INDEX.md`。
- 修改共享 Work/Infrastructure 边界时，先阅读目标 package 的局部文档和源码；Migration 采用 additive 方式，checksum/完整性失败不自动修复。
- 文档和注释使用中文；技术标识符、schema version、API path 和命令保持原文。

## 验证

Planning 实现至少补充纯单元测试；跨越 Workstore、Daemon 或 Snapshot 时增加对应集成测试。按变更范围执行 `go test ./...`、`go vet ./...`、`make build` 和 `git diff --check`，不以规划文档替代运行证据。

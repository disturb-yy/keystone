# 01: 创建可追溯的 Change

**What to build:** 本机操作者能在一个已注册、干净且稳定的 RepositoryBinding 中，经 CLI、Control Plane 与 Daemon 创建 Change。成功结果以固定 BaseRevision、不可变 ChangeIntentArtifact、初始 Intent/active 生命周期事实和 ChangeCreated Event 为依据，并可立即通过基础 Change 查询观察。

**Blocked by:** 顶层 Ticket 04：Repository Init and Project；无本地前驱。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 04` 未解除）

- [ ] `change create` 只接受绝对 repository_path、有效 ChangeIntent 和原样非空 Idempotency-Key；Client 不提交 ProjectID、不读取 Git、Manifest 或 Keystone SQLite，Daemon 从权威 RepositoryBinding 解析既有 Project，缺失时返回 `project_not_found`，不隐式初始化 Project。
- [ ] 创建在只读边界连续确认干净状态、HEAD、干净状态、HEAD；已暂存、未暂存、未跟踪或子模块变化返回 `repository_dirty`，ignored 文件不阻断，detached HEAD 可用，unborn 或不可解析 HEAD 返回 `base_revision_unavailable`，不稳定 HEAD 返回 `source_snapshot_unstable`，整个流程不写 Git。
- [ ] 成功创建以完整小写 Git OID 固定 BaseRevision，建立小写 canonical UUIDv7 的 Change，初始状态为 Intent/active，并将原始 Intent 作为 text/plain; charset=utf-8 的不可变 change_intent Artifact 保存；Intent 摘要有界但不改写原文。
- [ ] Artifact 内容以摘要与长度可校验、原子可见的方式先落盘，再在同一权威 SQLite transaction 中建立 Change、Intent ArtifactRef、ChangeCreated 与首次成功 Receipt；事务失败不留下悬挂的权威引用，可留下未来显式维护的无引用内容。
- [ ] 相同操作、聚合范围、Idempotency-Key 和规范请求重放首次成功响应；同键不同请求返回 `idempotency_conflict`；不同 key 的相同 Intent 与 BaseRevision 允许形成独立 Change，不做语义去重。
- [ ] 操作者可通过 `change show` 和按 repository_path 的 `change list` 观察稳定 ChangeView；列表按创建时间和标识倒序、空集合返回 []，ChangeView 包含生命周期、Version、BaseRevision、Intent Artifact、最新 Run 及时间字段，不泄漏存储路径。
- [ ] 从真实 CLI 到 loopback Daemon 的临时 Git Repository、LocalStateRoot 与 SQLite 验收创建、重放、各类源快照失败、Artifact 引用和基础读取；Domain 与 Application 测试同时覆盖输入不变量、事务边界与无 Git 写入。

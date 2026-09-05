# 01: 首次 Project 注册与查询

**What to build:** 本机操作者能够在一个正常 Git Repository root 执行 `keystone init`，经既有 Daemon ensure 与 Control Plane API 创建严格 V1 ProjectManifest、权威 Project 和该 LocalStateRoot 内首次且唯一的 ProjectInitialized；操作者随后能经 Project Query 与 Project Event Query 观察同一 Project 和该 Event。

**Blocked by:** 顶层 Ticket 03 的 CLI、Daemon、SQLite Readiness 与原生 Windows InstanceLock 验收；无本地前驱。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 03` 未解除）

- [ ] 在真实 Git Repository root 上，`keystone init` 通过 loopback Control Plane 完成首次注册；Init JSON 只接受绝对 `repository_path` 与必需 Idempotency-Key，成功为 HTTP 200 且顶层 `project` 返回强类型 Project DTO。CLI、Handler、Application、Domain、Migration、SQLite、Git/Manifest 适配和 Daemon composition 构成一条可运行路径。
- [ ] 首次注册在同一 SQLite transaction 中创建 Project 与一次 ProjectInitialized；Project Query 成功为 HTTP 200 且返回同一顶层 `project`，Project Event Query 成功为 HTTP 200 且顶层 `events` 返回按 `occurred_at`、`event_id` 升序的 Event，空结果使用 `[]`。
- [ ] Project DTO 的 JSON 字段固定为 `project_id`、`repository_root`、`manifest_path`、`created_at`；Project Event DTO 固定为 `event_id`、`project_id`、`type`、`occurred_at`，时间均使用 UTC RFC3339Nano。
- [ ] 普通 dirty worktree 可初始化，且过程不执行 Git add、commit 或其他工作区内容修改。
- [ ] 非 Git、空请求、缺失 Idempotency-Key 和不合法 ProjectID 返回既有 ErrorEnvelope 的稳定边界错误；Client 不直接访问 Keystone SQLite。
- [ ] 真实 CLI、Daemon、Git、Manifest 和 SQLite 端到端测试覆盖首次成功、查询、一次 Event 与失败原子性。

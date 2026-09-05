# 02: 可并发保护的 Change 控制命令

**What to build:** 已创建 Change 的本机操作者能以显式 Version 和幂等键执行 Pause、Resume、Cancel。每个接受的控制动作都形成可查询的当前状态和不可改写的审计事实，不会因为并发或网络重试而覆盖另一个操作者已确认的结果。

**Blocked by:** 顶层 Ticket 04：Repository Init and Project；本地 Ticket 01：创建可追溯的 Change。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 04` 未解除）

- [ ] Control Plane 和 CLI 提供 Pause、Resume、Cancel；写命令显式要求 Idempotency-Key 与 expected_version，并只让写命令确保 Daemon 已就绪，纯读取继续只发现已有 Daemon。
- [ ] 状态转换严格遵守 active 到 paused、paused 到 active，以及 active、paused、human_required 到 cancelled；Client 不能直接设置 Stage 或 Status，非法状态或错误命令类别返回 `lifecycle_transition_invalid` 或 `human_decision_required`。
- [ ] 每次成功控制操作以旧 ChangeVersion 为条件更新，单调递增 Version，并与对应 ChangePaused、ChangeResumed 或 ChangeCancelled Event、ChangeCommandReceipt 在同一 SQLite transaction 提交；陈旧 Version 返回 `change_version_conflict`，不得半提交状态或 Event。
- [ ] Receipt 先于 Version 校验命中：同键同规范请求始终重放保存的首次成功 HTTP 状态与响应体，同键不同请求返回 `idempotency_conflict`；无效输入、冲突和 unavailable 不写 Receipt。
- [ ] Pause 与 Cancel 作为前瞻性协调围栏：Pause 阻止新的协调和 Assignment，Cancel 阻止后续生命周期推进；实现不伪造对已启动进程的强制终止，也不删除既有事实。
- [ ] 操作者从真实 CLI/Daemon 请求中获得 JSON 成功响应和既有 ErrorEnvelope 分类；Domain、Application、SQLite 与端到端测试覆盖所有合法/非法转换、重复命令、竞争 Version、事务回滚和读命令不启动 Daemon。

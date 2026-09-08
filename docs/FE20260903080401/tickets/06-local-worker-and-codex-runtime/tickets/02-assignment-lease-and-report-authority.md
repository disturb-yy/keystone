# 06-02：Assignment、Lease 与 Report Authority

**What to build：** 将 Ticket 05 的 AgentRun、Artifact、Event 和 Pause/Cancel 围栏接入 Worker 执行机会，建立 Assignment/Lease 的唯一权威、Report 分类和可重放的证据提交事务。

**Blocked by：** 顶层 Ticket 05：Change Lifecycle、Artifact 与 Event；本地 Ticket 01：Worker Protocol、鉴权与 Pull。

**Status：** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 05` 未解除）

- [ ] Assignment 绑定不可变 AgentRun、attempt、WorkerInstance 和一次性 Lease；同一 AgentRun 同时最多一个 active Lease。
- [ ] Lease 默认 TTL 30 秒、Heartbeat 默认每 5 秒续租，状态为 `active`、`expired`、`revoked`、`consumed`；时间和 token hash 通过 Port 注入/保存，Domain 不依赖 wall clock 或 SQLite。
- [ ] Lease token 只在传输中作为 opaque 值存在，持久化只保存 hash、绑定关系、状态、过期时间和首次终态摘要。
- [ ] Report 只有在 Worker 身份、AgentRun/attempt、Lease 和 `running` 状态均匹配时才可以形成首次终态；exit code 0 映射 succeeded，其余映射 failed，`human_required` 由 Daemon/Coordinator 产生。
- [ ] 同一终态 Report 重放返回 `duplicate`；已完成 AgentRun 收到不同终态返回 `terminal_conflict`，不得改写状态、Event、ArtifactLink 或 ChangeVersion。
- [ ] 有效 Lease 在 paused Change 到达时仍完成实际 AgentRun 和 Artifact，但返回 `accepted_fenced`，不追加 `StageAdvanced`；Resume 由 Coordinator 重新评估。
- [ ] cancelled Change 的实际结果可以保存为已发生事实，但永远不恢复、不推进 Change；Lease 过期、撤销、Daemon 重启、旧 Worker 或旧 attempt 的结果只进入 LateReport Trace。
- [ ] Artifact 先校验并原子写入 Local Artifact Store，再与 AgentRun 终态、专用 ArtifactLink、Lease 结果、固定 Event 和必要 Change 更新在一个 SQLite transaction 中提交。
- [ ] Artifact 捕获/编码/摘要/落盘失败返回 `unavailable`，允许使用相同 Report digest 重试，不重复运行 Runtime；transaction 失败留下的内容只能是可按 digest 识别的 orphan。
- [ ] Daemon 重启撤销旧活动 Lease，把仍 running 的 AgentRun 收敛为 failed/`daemon_restarted`；active Change 进入 human_required，不自动 retry；已完成 Run 不变。
- [ ] 为 LateReport 增加固定 `AgentRunReportLate` EventType 到 Ticket 05 的统一 Event ledger，不建立第二个 Trace 账本，不保存自由日志 payload。

## Authority model

Assignment 不是 Ticket，也不是可复用任务槽。它是 Daemon 向一个 WorkerInstance 发放一次 AgentRun 执行授权的关联；Lease 失效后不能复活，retry 必须经 Ticket 05 的 HumanDecision 并创建新的 AgentRun attempt。

Report Authority Service 负责把协议 DTO 转换为领域结果和持久化命令。它不能信任 Runtime 自报的 outcome、文件列表、完成文本或 Worker 的“已提交”描述；exit code、执行时间、before/after revision、独立 Artifact capture 和 Guard 结果才是可验证输入。

## Report decision matrix

| 条件 | disposition | 权威效果 |
| --- | --- | --- |
| active / running / 当前 Worker / active Lease | `accepted` | 完成 AgentRun；按 Ticket 05 规则推进或进入 human_required |
| paused / running / 当前 Worker / active Lease | `accepted_fenced` | 完成 AgentRun 和 Artifact；不推进 Stage，Resume 时重评估 |
| cancelled / running / 当前 Worker / active Lease | `accepted_fenced` | 保存实际终态和 Artifact；不修改取消事实、不恢复 Change |
| completed / 相同终态摘要 | `duplicate` | 返回首次结果；不新增终态或推进 |
| completed / 不同终态 | `terminal_conflict` | 返回冲突；原事实保持不变 |
| expired/revoked/旧 Worker/旧 attempt | `late` | 可安全保存时追加 LateReport 和 Artifact；不改 AgentRun/Change 权威 |
| secret/token 完全无法鉴权 | 拒绝 | 不写业务状态或不可信 Trace |

Report、Pause、Cancel 和 Lease expiry 的结果以数据库 transaction commit 顺序为准，不使用进程内锁作为唯一保证。终态写入要有唯一约束/条件更新；ChangeVersion 仍按 Ticket 05 的前置条件递增。

## Persistence boundary

- 业务记录归 Ticket 05 所属 Work persistence，由既有统一 Migration runner 追加受控业务 Migration；不创建 Worker 私有 SQLite 或重复 Event ledger。
- 逻辑上需要 WorkerInstance、Lease 绑定、首次 Report digest、Artifact metadata、AgentRunArtifactLink 和 LateReport 引用；实际表名/列形态必须以 Ticket 05 已落地 Schema 为准，不在本票凭空锁死物理设计。
- Worker/Runtime 的 stdout、stderr、diff 和 changed files 原文放 Local Artifact Store；SQLite 只保存摘要、长度、媒体/种类、业务归属和可审计关联。
- Event 顺序固定：有效 AgentRun 终态先 `AgentRunCompleted`，随后按当前状态追加 `StageAdvanced` 或 `ChangeHumanRequired`；LateReport 追加 `AgentRunReportLate`，不追加生命周期推进 Event。
- 所有 Event、AgentRun 终态、Decision、ArtifactLink 和 Receipt 继续遵守 Ticket 05 的不可变/追加约束，不能通过 Report endpoint 提供删除或覆盖。

## Acceptance

- 伪造 worker/run/attempt/token 的 Report 不能改变权威 AgentRun、ChangeVersion、ChangeStatus、ArtifactLink 或 Event。
- 有效 Report 在 active、paused、cancelled 三种状态下分别得到准确的终态/推进效果；Cancel 后不会因 Report 恢复 Change。
- 相同终态 Report 在 Lease consumed、Daemon 重启或网络重试后仍安全返回 duplicate；不同内容明确 conflict。
- Lease expiry、worker_lost、daemon_restarted、旧 attempt 和旧 Worker 结果可查询为 Trace，但不能形成第二次 AgentRun 终态。
- Artifact 内容、摘要、长度和关联在 transaction 失败/重试后保持一致；没有指向未写入或摘要失配内容的权威引用。
- SQLite 并发测试证明 Report 与 Pause/Cancel/Lease expiry 的结果由 commit 顺序决定，历史不被覆盖。

## Verification

补充 Domain/Application/SQLite/Artifact 集成测试，使用可注入时钟和 fake ArtifactStore/Report。执行 `go test ./...`、必要的 `go vet ./...`、`make build` 和差异卫生检查。不要通过 HTTP 调试接口伪造生命周期成功；真实 Codex 证据由 06-05 提供。

## Out of scope

- Worker HTTP route、Bearer secret 生成与 Pull DTO（06-01 已定义，必要时由其提供 Adapter）。
- Worker 子进程监管、Heartbeat/Pull loop 和 Runtime 调用（06-03/06-04）。
- Ticket 09 的 Worktree 创建、生产 Assignment scheduler、Execution DAG 和真实 Ticket 分配。
- 自动 retry、Remote Worker、Gate/Verify approval、完整 OS sandbox 和任意 shell 安全保证。

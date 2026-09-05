# 03: AgentRun、HumanDecision 与 Retry 恢复

**What to build:** 当已创建 Change 的一个阶段产生可重试失败时，Daemon 能留下不可改写的 AgentRun 与证据，进入 human_required，并让本机操作者以审计化的 HumanDecision retry 或 cancel 恢复控制；每次 retry 都形成新的尝试，绝不覆盖历史。

**Blocked by:** 顶层 Ticket 04：Repository Init and Project；本地 Ticket 01：创建可追溯的 Change；本地 Ticket 02：可并发保护的 Change 控制命令。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 04` 未解除）

- [ ] AgentRun 固定 Change、Stage、attempt、输入和 Artifact 关联；状态只允许 running 到 completed 的一次合法终态更新，completed 必须有 succeeded、failed 或 human_required outcome 及完成时间，输入、输出和失败证据以带 role、ordinal 的专用关联保存。
- [ ] M3 生产组合不引入真实 Worker、Runtime 或 StageStrategy；Application 可注入确定性 Strategy 在测试中产生成功、失败与 human_required 事实，且不暴露调试专用 HTTP 接口来伪造执行。
- [ ] 可重试阶段失败与 AgentRunCompleted、ChangeHumanRequired 等因果 Event 在同一权威 transaction 中提交；Event 保持聚合内 EventSequence，能关联 AgentRun、HumanDecision 与有序 ArtifactRef，而不保存自由 payload 或日志文本。
- [ ] 只有 human_required Change 接受 retry 或 cancel 的 HumanDecision，且二者都要求 expected_version 与 Idempotency-Key。retry 先追加 HumanDecisionRecorded，回到 active 并建立同一 Change/Stage 的新 attempt；cancel 先追加 HumanDecisionRecorded，再追加 ChangeCancelled。
- [ ] paused 的晚到结果只能保留为 Trace，不能推进检查点；cancelled 的晚到结果同样可追溯但永不推进 Change。既有 Run、Decision、Event 与 Artifact 关联不得被 retry 或取消改写。
- [ ] 操作者可经 Change Run 与 Decision Trace 读取稳定摘要和有序历史，CLI 的 `decide retry`、`decide cancel` 输出 JSON 并保持既有错误边界。测试覆盖终态约束、attempt 递增、Decision 幂等重放、晚到结果围栏、因果 Event 顺序和 SQLite 外键归属。

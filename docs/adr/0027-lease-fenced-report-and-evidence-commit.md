# ADR-0027：Lease 围栏、Report 幂等与证据提交

## 状态

已接受。

## 决策

Daemon 是 `AgentRun` 终态与 Change 推进的唯一权威。每个 `Assignment` 使用有限且不可复用的 `Lease`，只有有效 Lease 的首次终态 `Report` 能形成权威结果；相同终态 Report 可安全重放，失效 Lease 的结果只能作为 `LateReport` 进入 Trace，不得覆盖 AgentRun、Change 或检查点。

Report 的 Artifact 必须先完成摘要、大小、路径和内容约束校验，再原子写入 Local Artifact Store；随后 Artifact 引用、`AgentRun` 终态、固定类型 Event 和 `AgentRunArtifactLink` 在同一 SQLite transaction 中提交。SQLite 的唯一约束和事务顺序负责处理重复提交及 Report、Pause、Cancel、Lease 失效之间的并发。

## 理由与边界

该顺序避免 Worker 崩溃、网络重试、Daemon 重启或并发 Report 把 Runtime 自报结果、半完成证据或过期授权变成权威状态。Artifact 写入成功而事务失败时，内容可作为待回收的 orphan，后续使用摘要重试关联；不能通过删除已经存在的证据来制造新的终态。

Pause 不强制终止已开始的 AgentRun：有效 Report 仍可完成该 AgentRun 并保存 Artifact，但暂停状态下不产生 `StageAdvanced`，Resume 重新评估。Cancel 后到达的实际结果也可保留为审计事实，但永远不能推进或恢复 Change。Lease 过期、被撤销或 Daemon 重启后到达的 Report 则只记录 `AgentRunReportLate` Trace Event。

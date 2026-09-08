# ADR-0049：结构性默认执行授权策略

## 状态

已接受。

## 决策

M7 的自动授权固定为 `DefaultExecutionAuthorizationPolicy.v1`，不引入自然语言风险评分、LLM 判断或通用 Governance Aggregate。它仅在以下结构性条件同时成立时形成 ExecutionAuthorization：

- Change 为 `active/Execute`，Session 与当前 DispatchEpoch 有效且处于 Project FIFO 队首；
- Workspace 的物理身份、BaseRevision、当前 WorkspaceInputRevision、branch 与持久化记录一致；Ticket 09 中 WorkspaceInputRevision 等于 BaseRevision，Ticket 10 的受控 KeystoneCommit 才可推进它；
- Ticket 是当前 RunnableTicket，且无该 Ticket 的活跃或已围栏 Authorization/AgentRun；
- ExecutionMode 固定为 `edit`，Runtime 固定为 `codex`；
- InstructionInputProjection、timeout、输入摘要及 WorkspaceBoundary 均已验证；
- 自定义 branch 已由初始显式 ExecuteCommand 记录为本机人工选择。

Authorization 必须持久化 policy_version、逐项判定结果和关联 ExecuteRequestIdentity，但不将内部判定细节暴露给 Client。Worker 暂不可用或未到队首时保持等待；Artifact 或存储暂时不可用时可重试；Workspace、branch 或证据身份不变量失败进入 `human_required`；其他前置条件不满足返回稳定冲突且不创建 Assignment。

## 理由与边界

可枚举的结构条件比风险标签更容易审计、重放和测试，并避免 Scheduler 或 Worker 将 Ticket 文本解释为授权范围。将人工 branch 选择固定在 ExecuteCommand 也避免后续 Ticket 静默扩大执行上下文。

该 ADR 不创建通用审批工作流、Risk Aggregate 或自动例外机制；未来增加新的执行模式或授权维度必须以新的显式策略版本处理。

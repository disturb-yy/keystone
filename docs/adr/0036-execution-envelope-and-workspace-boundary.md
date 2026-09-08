# ADR-0036：不可变执行信封与 Workspace 边界

## 状态

已接受。

## 决策

每个 ExecutionAuthorization 绑定不可变的 ExecutionEnvelope：Change、ExecutionSession、CanonicalTicket、AgentRun/attempt、Workspace、不可变 Change BaseRevision、每张 Ticket 的 WorkspaceInputRevision、WorkspaceBranch、`codex` runtime、Daemon 派生的 ExecutionInstruction 摘要、有序输入 Artifact 摘要和 timeout。首张 Ticket 的 WorkspaceInputRevision 等于 BaseRevision；每张已经由 KeystoneCommit 收口的 Ticket 把 after revision 作为下一张 Ticket 的 WorkspaceInputRevision。Client 不提供任意 Prompt 或 runtime 覆盖；任一输入变化只能创建新的 Authorization 与 AgentRun。

SourceControl 使用不修改实际 Workspace/index 的私有临时状态形成 WorkspaceSnapshot，覆盖 HEAD、index、staged/unstaged tracked 内容；ignored 文件不阻塞，残留 untracked 文件使证据不完整。WorkspaceBoundary 只验证 Assigned Workspace 的根和 Git 身份、拒绝路径逃逸或失效 Git root。V1 不从 CanonicalTicket 的自由文本 TicketScope 推断 allowlist，也不把该边界描述为完整操作系统沙箱。

## 理由与边界

不可变信封使 Daemon 授权的工作与 Worker 实际运行的输入可逐项复盘，避免 Runtime 在授权后替换目标。非侵入式快照避免为了观测而污染要验证的 Git 状态。自由文本无法可靠表达机器可执行路径规则；在没有结构化字段前，伪造 allowlist 会制造安全错觉。该 ADR 只定义 Ticket 09 的目标输入与 Workspace 约束，不证明 Git snapshot、ExecutionGuard 或 Runtime Adapter 已实现这些规则。

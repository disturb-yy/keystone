# Change 创建只按幂等操作去重

M3 的设计规定：同一 Project 可以同时拥有多个 Change，创建不会按 Intent、BaseRevision 或 RepositoryBinding 做业务去重。只有相同操作、相同目标、相同 IdempotencyKey 和相同规范请求才重放既有 Change；相同内容配合新键仍创建新的 Change。

M3 尚未执行 Git 写入或创建 Worktree，因此不应借由“单一活动 Change”提前约束未来的 Workspace 并发策略；该策略属于 Ticket 09 的边界。代价是 Client 必须显式选择复用幂等键还是表达一次新的变更意图。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

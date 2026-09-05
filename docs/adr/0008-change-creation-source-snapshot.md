# Change 创建时固定干净源快照

Daemon 在创建 Change 时通过窄的只读 RepositorySnapshot 边界确认 RepositoryBinding 干净并解析 HEAD，将结果作为不可变 BaseRevision 持久化；Client 不能指定 revision。这样 M3 可提供真实的 Change 创建事实而不提前创建 Worktree，Ticket 09 在创建 Worktree 前复核该快照而不重新定义 Change 的起点。

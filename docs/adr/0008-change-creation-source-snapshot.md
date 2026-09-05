# Change 创建时固定干净源快照

Daemon 在创建 Change 时通过窄的只读 RepositorySnapshot 边界连续确认“干净状态、HEAD、干净状态、HEAD”，只有两次 HEAD 都能解析为相同 commit 才将完整小写 Git OID 作为不可变 BaseRevision 持久化；Client 不能指定 revision。M3 以 `git status --porcelain=v1 --untracked-files=all --ignore-submodules=none` 的空输出定义干净：已暂存、未暂存、未跟踪和子模块变化均阻止创建，被忽略文件不阻止。Detached HEAD 在可解析到提交时允许，unborn HEAD 以 `base_revision_unavailable` 拒绝，两次 HEAD 不同以 `source_snapshot_unstable` 409 拒绝。该过程不加 Git 锁、不写 Git，也不声称抵御本机恶意 TOCTOU；这样 M3 可提供尽力稳定的 Change 创建事实而不提前创建 Worktree，Ticket 09 在创建 Worktree 前复核该快照而不重新定义 Change 的起点。

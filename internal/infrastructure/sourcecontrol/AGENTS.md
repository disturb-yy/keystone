# `sourcecontrol` 局部规约

该 package 是 Ticket 09 的受约束 Git Worktree 适配器。

- 只接收结构化 provisioning、workspace identity 和 snapshot 输入，不接受自由 Git 命令。
- 所有 Git 命令使用参数数组和 `exec.CommandContext`，统一关闭 optional locks。
- 源 Repository 只读校验；创建 Worktree 是 Execute 阶段的写操作，受控 Commit 只允许在已固定 parent/tree/message/trailer 的专用 seam 中发生。
- Commit 必须先校验 branch、parent、CandidateTreeIdentity、author/committer、无 residual untracked，写入后再校验 direct parent/tree/trailer/clean；恢复只对账 HEAD 的唯一匹配结果。
- 不执行 `worktree remove/prune`、reset、checkout、clean、merge、push、rebase、amend 或自动修复。
- 错误只返回可分类的适配器错误，不向 Control Plane 暴露 raw Git 输出或绝对路径。

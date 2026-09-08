# `sourcecontrol` 局部规约

该 package 是 Ticket 09 的受约束 Git Worktree 适配器。

- 只接收结构化 provisioning、workspace identity 和 snapshot 输入，不接受自由 Git 命令。
- 所有 Git 命令使用参数数组和 `exec.CommandContext`，统一关闭 optional locks。
- 源 Repository 只读校验；创建 Worktree 是唯一允许的 Git 写操作。
- 不执行 `worktree remove/prune`、reset、checkout、clean、commit、merge、push 或自动修复。
- 错误只返回可分类的适配器错误，不向 Control Plane 暴露 raw Git 输出或绝对路径。

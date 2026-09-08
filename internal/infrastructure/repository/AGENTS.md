# `repository` 局部规约

该 package 是 Project Bootstrap 与 Planning Snapshot 的 Git 适配器。

- 对源 Repository 只使用只读 Git 命令，识别物理 root、bare、主工作树、linked worktree 和固定 commit。
- Planning materialization 只能写入权限受限的临时 clone；不得修改源 Repository，也不得创建生产 Change Worktree。
- Isolated Snapshot 必须固定到完整 `base_revision`，只能通过受约束的相对路径访问，并由调用方幂等清理。
- Snapshot 的临时绝对路径只允许进入内部短期 Lease 和 Assignment；不得进入长期业务事实、Artifact、Event、日志或 Control Plane response。
- 错误向 Application 返回可分类错误；不暴露 raw Git output、源 Repository 绝对路径或临时绝对路径。

# `repository` 局部规约

该 package 是 Project Bootstrap 的 Git 适配器。

- 只使用只读 Git 命令识别物理 root、bare、主工作树和 linked worktree。
- 不执行 add、commit、checkout 或其他会改变 Repository 的命令。
- 错误向 Application 返回领域错误；不把原始 Git 输出暴露给 HTTP。

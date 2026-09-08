# ADR-0047：SourceControl 拓扑、恢复与原生验证

## 状态

已接受。

## 决策

SourceControl 复用 `repository.Git` 的物理主工作树发现和双读源快照复核，不复制其只读逻辑。源必须为规范化、非 bare 的主工作树；首次创建前必须无 staged、unstaged 或 untracked 变化，忽略文件不阻塞，且 HEAD 必须等于 BaseRevision。

WorkspacePath 以 `filepath` 派生，并在存在时经 EvalSymlinks 取得物理身份；它不得与 RepositoryBinding.Root 重叠或互为子目录。首次 Worktree 创建只通过无 shell 的参数数组，语义等价于 `git worktree add -b <validated-branch> <workspace-path> <BaseRevision>`。branch 必须通过 `git check-ref-format --branch`，且首次创建时为新 ref。

首次创建中的恢复必须同时核对 Git `worktree list --porcelain`、物理顶层路径、branch、HEAD/BaseRevision 和持久化 Workspace 身份。已经形成 KeystoneCommit 的 Workspace 恢复改为核对同一集合与记录的当前 WorkspaceInputRevision；任何未知目录、ref 冲突、路径或身份不一致均进入 `human_required`，不得自动 remove、prune、删除或猜测修复。

SourceControl 必须使用真实 Git fixture 覆盖 Linux/WSL 与原生 Windows 的默认/自定义 branch、符号链接或路径重叠、脏源、恢复匹配和冲突拒绝。Windows 证据必须来自原生执行，不能只交叉编译。

## 理由与边界

复用现有只读适配器让 Change 创建与 Execute 的源清洁定义一致；物理路径和 worktree 元数据双重核验避免由符号链接、分支残留或中断创建把错误目录当作已授权 Workspace。参数数组避免 shell 解释和平台路径拼接差异。

该 ADR 不证明 Git 写 adapter、恢复流程或 Windows 测试已存在，也不把 Git 自身的跨进程锁当作 Keystone 的业务执行授权。

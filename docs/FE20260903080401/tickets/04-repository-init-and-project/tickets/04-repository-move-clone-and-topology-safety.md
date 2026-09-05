# 04: Repository 移动、Clone 与拓扑安全

**What to build:** 已完成安全 Init 的操作者可以在旧 RepositoryBinding 明确不存在时移动 Repository 并重新绑定；当 clone、不可验证旧 root 或身份冲突仍存在时获得明确拒绝。同时，子目录、目录 symlink、独立 submodule、bare Repository 与 linked worktree 都具有稳定、可观察且跨平台验证的 Project 初始化行为。

**Blocked by:** 顶层 Ticket 03 的 CLI、Daemon、SQLite Readiness 与原生 Windows InstanceLock 验收；本地 Ticket 02：安全的重复 Init 与严格 ProjectManifest。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 03` 未解除；完成 02 后可与本地 Ticket 03 并行）

- [ ] 仅当旧物理 root 明确不存在时，移动后的合法 RepositoryBinding 可以 rebind 到同一 Project；该操作不追加第二个 ProjectInitialized。
- [ ] 旧 root 仍存在的 clone、旧 root 不可访问或无法验证、同一 Binding 关联不同 ProjectID，均返回 `project_identity_conflict`，不猜测、覆盖或自动合并。
- [ ] 从子目录或指向 Repository 的目录 symlink 执行 init 收敛到物理 root；独立 submodule 可以独立初始化；bare Repository 与 linked worktree 返回 `repository_unsupported`。
- [ ] 在 Linux、WSL 与原生 Windows 上以真实 Git Repository、CLI、Daemon、Manifest 和 Query 验证移动、clone、symlink、submodule、bare 与 linked worktree 的外部结果；交叉编译不替代原生行为证据。
- [ ] 该 Ticket 不改变中断恢复与数据根切换的语义；它只扩展安全的 RepositoryBinding 拓扑与 rebind 可观察行为。

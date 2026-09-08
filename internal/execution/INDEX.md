# execution 项目索引

| 路径 | 职责 |
| --- | --- |
| `types.go` | RuntimeAdapter、ExecutionInput、RuntimeResult、普通执行证据与 Planning candidate Artifact 类型 |
| `capture.go` | 有界输出、摘要和 changed files 规范化 |
| `guard.go` | Workspace/Git root、HEAD 与 Git 拒绝 wrapper 边界 |
| `process_tree.go`、`process_tree_unix.go`、`process_tree_windows.go` | 跨平台外部进程树生命周期、优雅停止和强制清理 |
| `adapters/codex/` | Codex CLI RuntimeAdapter |
| `domain/` | Workspace、ProvisioningIntent、ExecutionSession、DispatchEpoch、Authorization、Snapshot 和 Evidence 纯对象 |
| `application/` | 先 intent、后 Worktree、再 Session finalization 的 Execute 用例与 SourceControl/Persistence Port |

该 package 的 Domain/Application 不实现 Worker Protocol、Lease、Report authority 或 SQLite 持久化；Ticket 09 的 Workstore authority 和 `sourcecontrol` adapter 位于 `internal/infrastructure/`。

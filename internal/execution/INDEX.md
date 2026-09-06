# execution 项目索引

| 路径 | 职责 |
| --- | --- |
| `types.go` | RuntimeAdapter、ExecutionInput、RuntimeResult 与 Artifact 类型 |
| `capture.go` | 有界输出、摘要和 changed files 规范化 |
| `guard.go` | Workspace/Git root、HEAD 与 Git 拒绝 wrapper 边界 |
| `adapters/codex/` | Codex CLI RuntimeAdapter |

该 package 不实现 Worker Protocol、Lease、Report authority 或 SQLite 持久化。

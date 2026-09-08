# `sourcecontrol` 项目索引

| 文件 | 职责 |
| --- | --- |
| `git.go` | 复用 `repository.Git` 连续 clean/HEAD 校验，并负责受约束 Worktree provisioning、身份恢复、branch 校验和完整 Git Snapshot |
| `git_test.go` | 真实临时 Git Repository 的 clean/dirty/branch/path/provision/snapshot 测试 |

该 package 不拥有 Change、Ticket、Scheduler、Worker Lease 或 SQLite 权威状态。

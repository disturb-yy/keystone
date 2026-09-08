# `internal/work` 项目索引

## 当前状态

该 package 编排 Project Bootstrap、Change Lifecycle、Ticketize Graph completion，并定义
Planning/Ticketize 权威状态窄端口，不承载 HTTP Handler 或 SQL 实现。

| 文件 | 职责 |
| --- | --- |
| `service.go` | Project 初始化、可恢复/确定性失败回放、旧 root 检查、rebind 和查询用例 |
| `change_service.go` | Change 创建、稳定源快照、Intent Artifact、生命周期命令、HumanDecision、Trace 和 AgentRun port 编排 |
| `planning_state.go` | 显式 Planning stage attempt、非权威 candidate 查询、幂等/围栏 completion 与耐久恢复查询端口 |
| `ticketize_service.go` | Canonical Ticket Graph 只读查询 Application 用例 |
| `ticketize_state.go` | Ticketize run、专用原子 Graph completion、Graph 查询和 fenced run 窄端口 |
| `AGENTS.md` | Application 层局部规约 |
| `INDEX.md` | 当前文件和依赖地图 |

依赖关系为 `internal/work -> internal/work/domain`，外部系统通过窄端口注入。

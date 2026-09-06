# `internal/work` 项目索引

## 当前状态

该 package 编排 Project Bootstrap 和 Change Lifecycle，不承载 HTTP Handler 或 SQL 实现。

| 文件 | 职责 |
| --- | --- |
| `service.go` | Project 初始化、可恢复/确定性失败回放、旧 root 检查、rebind 和查询用例 |
| `change_service.go` | Change 创建、稳定源快照、Intent Artifact、生命周期命令、HumanDecision、Trace 和 AgentRun port 编排 |
| `AGENTS.md` | Application 层局部规约 |
| `INDEX.md` | 当前文件和依赖地图 |

依赖关系为 `internal/work -> internal/work/domain`，外部系统通过窄端口注入。

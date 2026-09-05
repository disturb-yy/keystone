# `internal/work` 项目索引

## 当前状态

该 package 编排 Project Bootstrap，不承载 HTTP Handler 或 SQL 实现。

| 文件 | 职责 |
| --- | --- |
| `service.go` | Project 初始化、恢复、rebind 和查询用例 |
| `AGENTS.md` | Application 层局部规约 |
| `INDEX.md` | 当前文件和依赖地图 |

依赖关系为 `internal/work -> internal/work/domain`，外部系统通过窄端口注入。

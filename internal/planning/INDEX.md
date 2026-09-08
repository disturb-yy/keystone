# `internal/planning` 项目索引

## 当前状态

该目录已建立 Planning 的 package 边界文档，但当前只有 `AGENTS.md` 与本索引，没有 Go 源码、Runtime 调用、HTTP Handler 或 SQLite 行为。实现前必须满足 Ticket 07 的 `BLOCKED_BY: 06` 和 Ticket 06 的真实验收前置条件。

## 目标职责

| 责任 | 说明 |
| --- | --- |
| Context | 构造有界、不可变的 `ProjectContext.v1` 和固定 `base_revision` 输入 |
| Contract/Validator | 定义 Understand、Design、Plan 的结构化输入输出和严格 schema 校验 |
| Stage Strategy | 将阶段输入转换为 Runtime 候选请求，并保持阶段顺序 |
| Decoder | 将 Runtime 候选字节解析为可验证 payload；不把 Runtime 自报当作 authority |
| Coordinator | 通过窄 port 协调 AgentRun、Artifact、Event 和 Change 推进，不直接持久化 |
| Ticketize Candidate | 定义 TicketGenerator、StructuredTicketDraft 与确定性候选校验；不持有 Canonical Ticket Graph |

## 依赖地图

```text
internal/planning
    ├── Planning Contract / Context / Validator
    ├── Stage Strategy / Prompt / Decoder
    ├── TicketGenerator / StructuredTicketDraft / Validator
    └── Coordinator ports
          ↓
internal/work Application / Domain authority
          ↓
internal/infrastructure/{artifact,repository,workstore}
          ↓
Ticket 06 Worker / Runtime boundary
```

上图表示目标依赖方向，不表示当前运行链已实现。`internal/planning` 不依赖具体 SQL、HTTP、Worker 进程或 Codex CLI；`repository` 的临时 Snapshot 扩展若改变局部职责，必须同步更新其局部规约和索引。

## Ticket 07 文档入口

- 共同规格：`docs/FE20260903080401/tickets/07-understand-design-plan/spec/07-understand-design-plan-spec.md`
- 实施子票：`docs/FE20260903080401/tickets/07-understand-design-plan/tickets/`

## Ticket 08 文档入口

- 顶层契约：`docs/FE20260903080401/tickets/08-ticketize-canonical-graph.md`
- 共同规格：`docs/FE20260903080401/tickets/08-ticketize-canonical-graph/spec/08-ticketize-canonical-graph-spec.md`
- 实施子票：`docs/FE20260903080401/tickets/08-ticketize-canonical-graph/tickets/`
- Ticketize 只拥有候选生成与校验；CanonicalTicketGraph 的领域和持久化权威仍在 Work/Workstore。

## 验证入口

Planning 实现完成后，以受影响 package 的聚焦测试为起点，再执行根级 `go test ./...`、必要的 `go vet ./...`、`make build` 和 `git diff --check`。文档和索引只能说明实际存在的文件及已验证行为。

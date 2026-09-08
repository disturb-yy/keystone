# 07-01：Planning Contract、Context 与 Validator

**What to build：** 建立 Understand、Design、Plan 共用的输入/输出 Contract、`ProjectContext.v1`、严格 JSON decoder/schema validator 和有界规则，使策略可以在不依赖 HTTP、SQLite 或真实 Runtime 的情况下得到确定性校验。

**Blocked by：** 顶层 Ticket 06；实现还必须以当前 `internal/work/domain` 的 ArtifactRef、Change、AgentRun 事实为输入，不提前改变 Ticket 08 的 Ticket Graph 契约。

**Status：** implemented（Ticket 06 外部验收缺口仍由顶层阻塞关系跟踪）

## Scope

- 在 `internal/planning` 定义阶段名称、Planning input、`ProjectContext.v1`、结构化候选 payload、Artifact envelope 和 validator port。
- 定义 Understand、Design、Plan 三个 schema 的最小必需字段，保持阶段输入单向、固定 `base_revision`、上游 Artifact 不可静默替换。
- 实现严格的单对象 JSON 解码：UTF-8、未知/重复字段拒绝、缺失字段拒绝、尾随内容拒绝、有界 body 和字段/数组/路径限制。
- 产生面向调用方的稳定领域错误；不得把 JSON parser、SQL 或 HTTP 错误原样暴露为 Control Plane Contract。
- 为合法、超限、未知字段、重复字段、非法路径、revision mismatch、空 payload 和尾随内容补充表驱动单测。

## Frozen bounds

| 项目 | 上限/规则 |
| --- | --- |
| `ProjectContext.v1` | 64 KiB |
| 结构化 Artifact | 1 MiB |
| `summary` | 256 Unicode rune |
| 普通数组 | 64 项 |
| Plan steps | 32 项 |
| 文本字段 | 8 KiB |
| 路径 | 相对路径；禁止绝对路径、`..` 和 OS 分隔符 |

所有上限都必须是可测试的 validator 行为，不以 Prompt 约束替代。

## Acceptance

- 三个阶段的输入/输出类型能表达固定 revision、输入 Artifact 引用和 schema version。
- 不合法或超过上限的候选 payload 在进入 Coordinator 前被拒绝；错误包含稳定分类和字段上下文，不泄露 secret、绝对路径或原始运行环境。
- 旧 `ArtifactRef` 可以继续读取；规划字段为空时按 legacy 语义处理，不能被 validator 当成新规划 Artifact。
- 单元测试不启动 HTTP、SQLite、Worker 或真实 Codex。

## Out of scope

- Stage Strategy、Runtime 调用、Coordinator、Artifact store 和 Daemon recovery。
- Ticket Draft、Canonical Graph、Repository 全量分析和 CodeMap。

## Verification

```bash
go test ./internal/planning/...
go test ./...
git diff --check
```

# `internal/planning` 项目索引

## 当前状态

该目录已实现 Ticket 07 的 Planning Contract、Context、严格 decoder/validator、Stage
Strategy、prompt、fake-friendly Runtime/clock seam 和 Coordinator。Coordinator 只依赖
Work authority、Artifact、Snapshot 与 Dispatcher 端口，不包含 HTTP、SQL、SQLite 或
Codex concrete 调用。

## 文件地图

| 文件 | 职责 |
| --- | --- |
| `contract.go` | 阶段、schema、ProjectContext、Artifact envelope、候选 payload 和 validator port |
| `errors.go` | 面向调用方的稳定 Planning 错误分类与字段上下文 |
| `decoder.go` | 有界 UTF-8 单对象 JSON、未知/重复字段与尾随值拒绝 |
| `validator.go` | ProjectContext、三阶段 payload、revision、数组、文本和路径不变量 |
| `strategy.go` | Stage 定义、纯 Prepare/Decode、fake Runtime/clock seam 和执行观察事实 |
| `prompt.go` | 只含授权输入的确定性 Runtime prompt 构造 |
| `coordinator.go` | 耐久恢复、阶段串行、ProjectContext 复用、candidate 校验、失败收敛和 Snapshot 生命周期 |
| `integration_test.go` | 真实 SQLite Workstore 与 Artifact Store 上的三阶段串行、Worker candidate 和 Coordinator 重启恢复纵切测试 |
| `*_test.go` | Contract/decoder/validator/strategy 的纯单元测试，以及 Coordinator 恢复与 fencing 测试 |
| `AGENTS.md` | Planning package 的局部修改规约 |
| `INDEX.md` | 当前文件、职责和验证入口 |

## 依赖地图

```text
Coordinator
    -> Strategy.Prepare / Strategy.Decode
    -> Planning Contract / Validator
    -> Work Application / State ports

Strategy.Execute（纯测试便利 seam）
    -> injected Runtime interface
```

本 package 不依赖具体 SQL、HTTP、Worker 进程、Repository adapter 或 Codex CLI。

## Ticket 文档入口

- 共同规格：`docs/FE20260903080401/tickets/07-understand-design-plan/spec/07-understand-design-plan-spec.md`
- 07-01：`docs/FE20260903080401/tickets/07-understand-design-plan/tickets/01-planning-contract-context-and-validator.md`
- 07-02：`docs/FE20260903080401/tickets/07-understand-design-plan/tickets/02-stage-strategies-prompts-and-decoder.md`
- 07-03：`docs/FE20260903080401/tickets/07-understand-design-plan/tickets/03-planning-coordinator-and-artifact-persistence.md`
- 07-04：`docs/FE20260903080401/tickets/07-understand-design-plan/tickets/04-isolated-snapshot-and-daemon-recovery.md`
- 07-05：`docs/FE20260903080401/tickets/07-understand-design-plan/tickets/05-ticket07-integration-verification-and-navigation.md`

## 验证入口

从 `go test ./internal/planning/... -count=1` 开始，再按集成范围执行根级 Go 验证和
`git diff --check`。

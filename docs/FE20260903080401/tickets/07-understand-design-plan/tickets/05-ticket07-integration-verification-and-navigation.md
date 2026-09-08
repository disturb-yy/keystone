# 07-05：Ticket 07 集成验证与导航

**What to build：** 在 07-01 至 07-04 和 Ticket 06 前置实现/证据满足后，验证 Understand → Design → Plan 的完整纵切，并把实际 checkout 中的实现边界同步到导航文档。

**Blocked by：** 顶层 Ticket 06、07-01、07-02、07-03、07-04。`ready-for-agent` 不解除任何前置条件。

**Status：** implemented-with-ticket06-native-evidence-gap

## Integration checks

- 创建一个临时 Project/Change fixture，固定 `base_revision` 和有界 Intent/ProjectContext；不使用当前 checkout 的工作树或真实项目数据库作为破坏性 fixture。
- 通过内部 Application/测试 seam 触发 Planning；不新增公开 `planning start` 或“伪造成功” HTTP API。
- 验证三个阶段严格串行，输入 Artifact 和 source revision 连续，Plan 成功后 Change 只到 `Ticketize`/`active`。
- 验证 schema invalid、Runtime failure、timeout、revision mismatch、Artifact size/path violation 都保留失败证据并进入 `human_required`，不触发隐式下游 Assignment。
- 验证 pause/cancel/resume、重复 completion、旧 attempt、失效/晚到结果的 fencing；权威 Change/AgentRun/Event/Artifact 不被覆盖或重复推进。
- 验证重启后 Coordinator 从耐久状态恢复，已完成阶段不重跑，未完成阶段按当前规则继续或停在人审边界。

## Navigation updates

完成实现后，依据真实文件树更新：

- 根 `INDEX.md`：`internal/planning`、受影响 `internal/work`、`internal/daemon` 和 `internal/infrastructure` 的实际职责/状态。
- `docs/FE20260903080401/tickets/INDEX.md`：Ticket 07 的规格入口、实现状态和验证链接；保留 `BLOCKED_BY` 事实。
- `internal/planning/AGENTS.md`、`internal/planning/INDEX.md`：新增文件、依赖方向、测试入口和当前行为。
- 若 Snapshot 所有权扩大，更新 `internal/infrastructure/repository/AGENTS.md`、`INDEX.md`；若 Workstore schema/职责改变，更新其局部文档。
- 根 `README.md` 只在用户入口或实际结构确实变化时更新；不把规格文字写成运行行为。

## Acceptance

- `go test ./...`、必要的 `go vet ./...`、`make build` 和 `git diff --check` 通过，或报告可复现的具体阻塞。
- 集成结果包含成功链、失败停机、重复/晚到结果和重启恢复的权威查询证据。
- 文档只描述当前 checkout 已验证的能力；Ticket 08 及之后的业务行为仍未提前落地。
- 无关工作树变更（包括现有 `.gitignore`、Ticket 06 未跟踪目录、ADR 和 architecture-baseline）保持不变。

## Out of scope

- Ticketize/Candidate Ticket、Canonical Graph、生产 Execute、Verify、Commit、Dashboard 和 Golden Path。
- 清理或重置无关用户文件，或把未完成的 Ticket 06 smoke 写成成功证据。

## Verification

```bash
go test ./...
go vet ./...
make build
git diff --check
```

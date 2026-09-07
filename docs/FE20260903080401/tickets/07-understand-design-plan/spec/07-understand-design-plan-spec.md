# 07：Understand、Design、Plan 实施规格

> **状态：** Planning 规格已对齐，尚未实现。顶层 Ticket 07 的 `BLOCKED_BY: 06` 仍然有效；本文件和子 Ticket 的 `ready-for-agent` 只表示契约成熟度，不表示当前 checkout 已具备 Ticket 07 的 Planning Strategy、Coordinator 或 Planning Artifact 业务行为。
>
> **用途：** 作为 Ticket 07 五个实施子票的共同契约，约束 Planning 的输入、阶段策略、Artifact 验证、Daemon 权威提交和恢复边界。

## 1. Problem Statement

Ticket 05 已为 Change、Lifecycle、Artifact、Event、AgentRun 和人工恢复建立了 Work 领域基础，Ticket 06 负责提供受授权的本机 Worker/Runtime 执行通道。当前还没有一条能把 Change 的 Intent 变成可复盘 Understanding、Design、Plan Artifact 的 Planning 纵切。

如果 Runtime 直接决定阶段完成、写入 Change 状态，或把自由文本当作 Plan，Daemon 重启、重复 Report、schema 错误和人工恢复都会破坏权威历史。Ticket 07 必须把 Runtime 视为候选结果提供者，把阶段验证、AgentRun 终态、Artifact 关联、Change 推进和恢复保留在 Daemon/Work 的权威边界内。

## 2. Current Checkout Fact Boundary

以下是当前 checkout 的事实，不是本 Ticket 的已实现行为：

- 顶层 `docs/FE20260903080401/tickets/07-understand-design-plan.md` 已存在；本目录下的规格和五个子 Ticket 是本次补齐的实施文档。
- `internal/planning/` 当前只有本 Ticket 新增的局部规约和索引，没有 Go 源码、Runtime 调用或持久化行为。
- `internal/work/` 与 `internal/work/domain/` 已承载 Change 生命周期、AgentRun 和 Artifact/Event 的 Work 领域模型；AgentRun/Change 的权威写入由 State port 和 `internal/infrastructure/workstore/` 负责。
- `internal/infrastructure/repository/` 当前是只读 Git root/topology/snapshot 适配器；它尚未提供 Ticket 07 的临时隔离 Snapshot materialization。
- `internal/infrastructure/workstore/` 当前持有业务 SQLite Migration 和 Work 状态；Ticket 07 的 Migration 必须继续由它拥有。
- `internal/execution/`、`internal/worker/`、`cmd/keystone-worker/` 以及 Daemon/Workstore 的 Worker Protocol seam 已存在，提供 Ticket 07 可依赖的 Runtime/Worker 边界；Planning Strategy、Coordinator 和 Planning Artifact 业务行为仍未实现。Ticket 06 的完整验收和平台证据仍必须在当前 checkout 中核验，不能由本规格替代。

## 3. Goal

在 Ticket 06 完成后，提供一个耐久、可恢复、严格串行的 Planning Coordinator，使符合条件的 active Change 依次经历：

```text
Intent → Understand → Design → Plan → Ticketize/active
```

每个阶段都生成有版本的结构化 Artifact，并将输入 Artifact、输出 Artifact、AgentRun、source revision 和阶段事件关联起来。Plan 是后续 Ticketize 的输入材料，不是 Ticket、Canonical Graph 或可执行事实。

## 4. Scope

### 4.1 Coordinator 触发与串行规则

- Change 创建后由 Daemon/Application 触发 Planning eligibility；V1 不新增公开的 `planning start` HTTP endpoint。
- 进程启动时扫描 active 且可恢复的 Change，恢复必须来自耐久状态；不得只依赖内存队列。
- V1 将新建的 active Change 视为可进行只读 Planning 的低风险 demo Change。这里的“低风险”只描述当前 V1 的无源码副作用策略，不引入完整 Risk Model 或安全分类。
- 一个 Change 同时最多有一个 Planning AgentRun；Understand、Design、Plan 严格串行，每个阶段一次执行尝试。
- 每次尝试固定使用 Change 创建时记录的 `base_revision`。上游成功 Artifact 和 Project Context 在后续阶段不可被静默替换。
- Plan 成功后，Coordinator 原子完成 Plan AgentRun、保存输出和输入/输出 Artifact 关联、追加阶段事件，并把 Change 推进到 `Ticketize`/`active`，随后停止。Ticketize Assignment 和 Canonical Graph 由 Ticket 08 负责。

### 4.2 阶段输入

| 阶段 | 必需输入 | 固定共同输入 |
| --- | --- | --- |
| Understand | Intent Artifact | `ProjectContext.v1`、`base_revision` |
| Design | Understanding Artifact | `ProjectContext.v1`、`base_revision` |
| Plan | Design Artifact | `ProjectContext.v1`、`base_revision` |

`ProjectContext.v1` 是有界、不可变的项目元数据快照，不是 Repository 全量分析。它不得包含绝对路径、凭据、完整源码、CodeMap、概念空间或无法复现的运行时对象。默认最大尺寸为 64 KiB。

### 4.3 阶段输出与权威信任边界

Runtime 只返回候选 payload、运行结果和原始日志。Coordinator/Daemon 负责构造和验证权威 Artifact envelope，生成业务 Artifact 身份、时间、输入关联和状态事件。Runtime 自报的 `done`、阶段状态、文件列表或“已完成”文本都不能单独推动 Change。

每个结构化输出必须是严格的单一 UTF-8 JSON object，并满足：

- schema version 与 stage/kind 精确匹配；拒绝未知字段、重复字段、缺失必需字段、多个 JSON 值和尾随内容。
- 结构化 Artifact 默认最大 1 MiB；单个文本字段最大 8 KiB；summary 最大 256 个 Unicode rune；普通数组最大 64 项。
- Plan steps 最大 32 项；步骤中的路径只能是相对路径，不得包含绝对路径、`..` 或平台分隔符。
- 验证失败、解码失败、超限、编码失败或 base revision 不匹配都视为本阶段失败，保留失败 Artifact/raw log，不推进下游阶段。

Artifact envelope 至少表达以下语义：

```json
{
  "kind": "understanding|design|plan",
  "schema_version": "...",
  "stage": "Understand|Design|Plan",
  "summary": "...",
  "source_revision": "...",
  "input_artifact_ids": ["..."],
  "payload": {}
}
```

以上字段是语义示意；最终 Go 类型和 JSON 字段必须由 Contract 子票定义。`artifact_id`、`change_id`、`agent_run_id`、created time 和持久化归属由 Daemon/Work authority 生成或确认，不接受 Runtime 覆盖。

### 4.4 失败、暂停、取消和人工恢复

- Runtime 非零退出、启动失败、timeout、Guard failure、revision 不一致、schema invalid 或 Artifact 持久化失败，都完成当前 AgentRun 为失败事实，保留有界 raw log/Failure Artifact，并使当前 Change 进入 `human_required`。
- V1 不自动 retry。人工 `retry` 创建新的 attempt，复用不可变的 Intent/Project Context 和已经成功的上游 Artifact；旧失败 Artifact 只用于 Trace。
- Pause/Cancel/Resume 遵循 Ticket 05/06 的提交顺序。暂停期间完成但被围栏的成功结果可以保存为事实，不能推进 Change；Resume 时由 Coordinator 重新检查当前 stage、attempt、base revision 和上游输入。
- Cancelled 或晚到结果不能复活 Change。失效 Lease、旧 Worker、旧 attempt 或已取消执行的结果最多形成可安全保存的 Late/Failure Trace，不得改写权威生命周期。

## 5. Runtime、Snapshot 与权限边界

Planning 通过 Ticket 06 提供的 Runtime/Worker port 调用策略，不依赖 Worker 或 Codex 的具体实现。策略核心测试可以使用 fake Runtime，不启动 HTTP、SQLite 或真实 Codex。

每次 Planning run 使用固定 `base_revision` 的隔离临时 source Snapshot：

- 原始 Repository 不被修改，不创建生产 Change Worktree，不向生产工作树写入 Artifact 或中间文件。
- Snapshot 的实现细节（archive、临时 checkout 或等价机制）不是本规格的公开 API；必须保证固定 revision、路径边界、清理和失败可观察。
- Snapshot port 属于 Planning 的窄边界；具体 Git adapter 复用现有 `internal/infrastructure/repository/`。如果 materialization 需要扩大该 package 的职责，必须在同一实现变更中先更新其局部 `AGENTS.md`/`INDEX.md`，明确临时 Snapshot 的不变量；不得另建宽泛的 `utils` 或提前创建新的基础设施目录。

Planning 不拥有 Change、AgentRun、Event、Decision 或 SQLite 的权威写入；Coordinator 通过 `internal/work` 的公开 Application/State port 完成协调，具体 SQL 和 Migration 仍在 `workstore`。

## 6. Artifact、AgentRun 与事务边界

- `ArtifactRef` 只做向后兼容的最小扩展，新增 `kind`、schema version、summary 和 source revision 等规划 Artifact 元数据；既有旧引用的 legacy/空字段保持可读。
- 不建立第二个 Event/Trace ledger。Planning 事件继续进入现有 Work Event ledger，并关联 AgentRun 与 ArtifactRef。
- Workstore 使用实现时的下一个可用 Migration version，采用 additive migration；不得重写既有 Artifact、Event 或 AgentRun。Migration checksum 漂移或应用失败必须阻止 ready，不自动修复。
- Stage completion 的最小原子边界是：验证输入/输出 → 写入 Artifact 内容和引用 → 完成 AgentRun → 追加关联事件 → 按当前 stage/attempt 条件推进 Change。任何一步失败都不能留下可被误判为成功的半完成终态。
- 重复 completion、旧 AgentRun、旧 attempt 和重复/冲突 Report 必须由已有 fencing/幂等规则分类处理；不得通过第二次写入覆盖首个权威终态。

## 7. Package Ownership

| 路径 | Ticket 07 责任 | 当前状态 |
| --- | --- | --- |
| `internal/planning/` | Context、Contract、schema validator、Stage Strategy、prompt/decoder、Coordinator port | 本次只创建局部文档；Go 实现待 Ticket 06 后开始 |
| `internal/work/` | Change/AgentRun/Lifecycle authority 和窄 Application/State port | 已存在；只按实现所需扩展 |
| `internal/infrastructure/workstore/` | Planning Artifact 元数据/AgentRun 事务和 Migration | 已存在；继续作为 SQLite owner |
| `internal/infrastructure/repository/` | 固定 revision 的只读/隔离 Snapshot adapter | 已存在；具体扩展须先对齐局部职责 |
| `internal/daemon/` | Coordinator composition、启动恢复和生命周期接线 | 已存在；不在 Planning package 中写 HTTP Handler |
| `internal/execution/`、`internal/worker/`、`cmd/keystone-worker/` | Ticket 06 已有的 Runtime/Worker 运行边界 | Ticket 07 只消费稳定 port，不重复实现或扩展为新的执行领域 |

## 8. Implementation Order and Sub-Ticket Graph

```text
07-01 Contract/Context/Validator
        ↓
07-02 Stage Strategy/Prompt/Decoder/Fake Runtime
        ↓
07-03 Coordinator/AgentRun/Artifact Persistence
        ├──────────────┐
        ↓              ↓
07-04 Isolated Snapshot/Daemon Recovery
        └──────┬───────┘
               ↓
07-05 Integration Verification/Navigation
```

所有子票仍受顶层 Ticket 06 阻塞。07-04 依赖 07-02、07-03；07-05 依赖全部前置子票和 Ticket 06 的真实实现/验收证据。

推荐实施顺序：Contract/Context/Validator → Artifact metadata 与 ports → Strategy/Decoder/fake runtime → isolated Snapshot → Coordinator/Daemon wiring/recovery → integration tests/navigation。

## 9. Acceptance

- 一个新 active Change 能在固定 `base_revision` 上依次生成 Understanding、Design、Plan 三类结构化 Artifact；每份 Artifact 都能查询 summary、source revision、输入关联、AgentRun 和 raw log 引用。
- 三个阶段严格串行，运行中不能启动同一 Change 的第二个 Planning AgentRun；Plan 成功后停在 `Ticketize`/`active`，不生成 Ticket Draft 或 Canonical Graph。
- schema、大小、字段、路径、revision 或 Artifact 完整性失败时，Change 不推进，失败证据可查询，且不触发隐式下游 Assignment。
- Pause、Cancel、Resume、旧 attempt、失效 Lease 和重复/晚到结果遵循 fencing；保存的事实不能复活或覆盖 Change 权威状态。
- Coordinator、Strategy、Decoder、Validator 可以在不启动 HTTP、SQLite 或真实 Runtime 的情况下进行核心单元测试。
- 运行时、Worker、Snapshot、Artifact store、Workstore 和 Daemon 的边界符合根 `AGENTS.md`；没有把目标架构或静态设计输入写成当前已实现行为。

## 10. Verification

实现阶段至少执行：

```bash
go test ./...
go vet ./...
make build
git diff --check
```

若 Ticket 06 的 Worker/Runtime 或平台证据尚未满足，必须记录实际阻塞事实；fake Runtime、交叉编译或静态文档不能替代真实前置验收。

## 11. Out of Scope

- Ticket Draft、Canonical Ticket Graph、Ticketize Assignment 和无环图持久化（Ticket 08）。
- 生产 Worktree、Ticket scheduler、Execution DAG、实际 Ticket 执行和 Diff（Ticket 09）。
- Verify approval、Commit、Integrate Ready、Dashboard 和 Golden Path E2E（Ticket 10–12）。
- Repository 全量分析、CodeMap 自动生成、概念空间构建和完整 Risk/Policy/Gate 模型。
- Remote Worker、Worker pool、消息队列、远程认证、OpenCode 实现、自动 retry 和完整 OS sandbox。

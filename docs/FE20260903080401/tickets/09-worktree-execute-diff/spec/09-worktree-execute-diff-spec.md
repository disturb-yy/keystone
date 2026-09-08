# 09：Change Worktree 执行与 Diff 证据实施规格

> 状态：规划规格已对齐，尚未实现。顶层 Ticket 09 的 `BLOCKED_BY: 06、08` 仍然有效；本规格、ADR 和术语只冻结目标实施契约，不表示当前 checkout 已具备 Canonical Ticket 执行、Scheduler、Change Worktree、ExecutionAuthorization、SourceControl 或 M7 SQLite Schema。
>
> 用途：让 Daemon 在显式 Execute 后，为一个 Change 创建并持续复用受控 Git Worktree，串行执行 RunnableTicket，并以 Worker 的独立观察和 Daemon 的完整 WorkspaceSnapshot 形成可审计的 Ticket 级 Diff 证据。它不提前实现 Ticket 10 的 Verify、Commit、FinalVerify 或 Integrate Ready。

## Problem Statement

Ticket 06 提供 Worker/Runtime 的基础链路，Ticket 08 定义 Canonical Ticket Graph，但两者本身都不能安全回答以下问题：

- 哪个显式人类动作允许创建 Change Worktree，且同一重试不会创建第二个目录或分支；
- 哪张有依赖的 CanonicalTicket 可以在何时取得唯一 Workspace 写入权；
- Worker 重复 Pull、网络响应丢失、Lease 过期、Daemon 重启或重复进程时，如何保证 Runtime 至多取得一次启动权；
- Runtime 修改、`git add`、Worker 的 Diff 观察和 Daemon 的完整 Git 状态如何共同构成一张 Ticket 的成功，而不是把累计 Diff 或 Runtime 文本误当作事实；
- 暂时无 Worker、证据读取失败、源仓库漂移、路径/branch 冲突、Pause、Cancel 与 Retry 如何分别恢复；
- Client 如何观察进度而不取得 Workspace 路径、Lease token、Prompt、运行环境或 SQLite 写入权。

Ticket 09 必须将这些事实收敛到 Daemon 和 SQLite，保持 Worker 只执行副作用、Runtime 只修改 Assigned Workspace、SourceControl 只处理受类型约束的 Git 操作。

## Target Result

```text
explicit ExecuteCommand
        │
        ▼
WorkspaceProvisioningIntent ── SourceControl recheck/worktree add ──► Workspace
        │                                                               │
        └──────────────► ExecutionSession / FIFO DispatchEpoch ◄───────┘
                                      │
                                      ▼
                        DefaultExecutionAuthorizationPolicy.v1
                                      │
                                      ▼
                     Assignment delivery ── RuntimeClaim ──► Runtime
                                      │                            │
                         WorkspaceSnapshot(pre)                    │
                                      │                            ▼
                                      └──── Worker Report + WorkspaceSnapshot(post)
                                                               │
                                                               ▼
                one authority transaction: Evidence + AgentRun + Ticket state + Event
```

M7 的成功只完成绑定的 CanonicalTicket。所有 Ticket 成功后，Change 才具备离开 Execute 的条件；本 Ticket 不运行 Verify、不创建 Git Commit、不 merge/push/PR/deploy，也不实现后续 Ticket 的 `TicketVerifyCommitGate` 行为。

## Scope and Non-Goals

### 包含

- 显式、带版本和幂等键的 ExecuteCommand；默认或一次性显式批准的自定义 branch。
- Change 级 Workspace、可恢复的 provisioning intent、确定性 Project FIFO 调度、Ticket 级 Authorization、Assignment、Claim、Lease 围栏和状态机。
- 真实 Git Worktree、真实 Diff/changed-files、Worker 原始 Artifact、Daemon Snapshot/Delta 与 TicketExecutionEvidence。
- `POST /v1/changes/{change_id}/execute`、执行只读模型、CLI、SQLite Migration、受控 Git adapter、Worker Contract 的最小 Claim/timeout 扩展和完整测试矩阵。

### 不包含

- Verify PASS、Git Commit、merge、push、PR、deploy、FinalVerify、Integrate Ready、自动回滚或 Worktree 清理。
- 多 Worker、跨 Change 并行、非 Git Workspace、Runtime 自选 Workspace、自由 Prompt、通用高权限 Git/Runtime 模式或完整 OS sandbox。
- 将 Ticket scope 的自由文本变为文件 allowlist，或由 LLM/自然语言风险评分决定授权。
- 改写已成功 Canonical Graph、自动 retry、skip、reset、checkout、clean、删除 Git worktree 或猜测性恢复。

## Authority and Package Boundaries

| 区域 | Ticket 09 的责任 | 明确不负责 |
| --- | --- | --- |
| `internal/execution` | 继续承载 RuntimeAdapter、ExecutionGuard、采集类型与 Codex adapter | Scheduler、业务状态、SQLite、HTTP |
| `internal/execution/domain` | Workspace、Session、Authorization、DispatchEpoch、WorkspaceInputRevision、Snapshot、Evidence 等纯执行概念 | Git、SQL、Worker Protocol、HTTP |
| `internal/execution/application` | Execute 用例、Scheduler、Authorization Policy、Ticket/证据编排与 Port | SQL、Git 命令细节、HTTP 参数解析 |
| `internal/infrastructure/sourcecontrol` | 受类型约束的 Worktree、branch、物理身份、Snapshot 和恢复 Git adapter | Ticket Graph、生命周期、调度权威 |
| `internal/infrastructure/workstore` | 下一条真实 Migration、ExecutionPersistencePort、Work/Execution 原子状态与 SQLite 纵深约束 | Daemon SQL 编排、Git、Worker 进程 |
| `internal/work` / Work & Lifecycle | Change、CanonicalTicket、Graph、阶段与状态的权威规则；实现 TicketExecutionAuthority | 直接接收 Worker 自报、Git/SQL 细节 |
| `internal/daemon` | 组合 Port、HTTP/Worker Handler、生命周期和错误映射 | 绕过 Application 写 SQL 或信任 Worker 状态 |
| `contracts/controlplane` | Execute/Execution Read DTO 和安全 ErrorEnvelope | Domain Entity、业务校验、数据库访问 |
| `contracts/worker` | Assignment、Claim、Heartbeat、Report 的窄传输字段 | Ticket/Change/Gate/Graph/Workspace 权威 |
| `cmd/keystone` | 显式 Execute 与只读查询入口 | 自动创建幂等键、直接访问 SQLite、选择 Runtime cwd |

新增 Go package 时必须同步创建最近一级 `AGENTS.md` 和 `INDEX.md`。实现前必须重新阅读根规约、根导航、上述目标 package 局部规约、直接相关源码和测试。

## Execute API and Read Model

写入边界固定为：

```text
POST /v1/changes/{change_id}/execute
Idempotency-Key: <non-empty opaque key>

{
  "expected_version": 17,
  "workspace_branch": "optional-validated-branch"
}
```

CLI 固定为：

```text
keystone change execute CHANGE_ID --expected-version N --idempotency-key KEY [--branch NAME]
```

- 成功和同一规范化请求的重放均返回 `202 Accepted` 与不可变 ExecuteCommandResponse。
- Response 仅含 Change、ExecutionSessionID、Session 状态、规范化 branch、按 CanonicalTicketOrder 排列的 Ticket 状态和允许公开的 Artifact 身份摘要。
- 相同 IdempotencyKey 与不同规范化请求返回 `idempotency_key_conflict`；活跃 Session 使用不同 key 返回 `execution_in_progress`。
- 没有可用 Worker 不是 Execute 失败；Session 与授权可以等待，但不得启动 Runtime。
- 无效输入、陈旧 ChangeVersion、非 Execute 状态、无图/无 RunnableTicket、Workspace/branch 身份不变量失败和暂时 Artifact/SQLite 不可用必须映射为稳定、安全且可处理的 ErrorEnvelope；不得泄露绝对路径或原始 Git/SQLite 错误。

只读边界固定为：

```text
GET /v1/changes/{change_id}/execution
keystone change execution show CHANGE_ID
```

ExecutionReadModel 不公开绝对 WorkspacePath、Lease token、原始 Prompt、运行环境、凭据或 Workspace 文件浏览能力。Artifact 内容仍只能经过既有 Artifact Content 读取边界。

## Workspace and SourceControl Contract

### 创建与身份

1. ExecuteCommand 首次被接受时，Daemon 先规范化 branch；省略时固定为 `keystone/change/<change-id>`，自定义值只能来自该显式本机命令。
2. `WorkspaceProvisioningIntent` 在任何 Git 写操作前持久化，固定 Change、Project、BaseRevision、branch 与派生 WorkspacePath。
3. SourceControl 复用 `repository.Git`：源必须是规范化、非 bare 的主工作树，连续双读均 clean，且 HEAD 等于 BaseRevision。clean 包含 staged、unstaged 和 untracked；ignored 文件不阻塞。
4. WorkspacePath 由 `<LocalStateRoot>/workspaces/<project-id>/<change-id>` 用 `filepath` 派生；存在时按 EvalSymlinks 取得物理身份，且不得与 RepositoryBinding.Root 重叠或互为子目录。
5. branch 必须通过 `git check-ref-format --branch`，首次创建时必须不存在。创建使用无 shell 参数数组，语义等价于：

   ```text
   git worktree add -b <validated-branch> <workspace-path> <BaseRevision>
   ```

6. Git 成功后，Workspace、ExecutionSession 和 Execute 回执在同一 SQLite transaction 收敛。Git/SQLite 中断后的重试只可协调完全匹配的 intent 结果。

### 恢复与拒绝

恢复同时核对 Git `worktree list --porcelain`、物理顶层路径、WorkspacePath、branch、HEAD 与持久化的 WorkspaceInputRevision，以及持久化 Workspace 身份。Ticket 09 中该输入 revision 等于 BaseRevision；Ticket 10 只有在已记录 KeystoneCommit 后才可把它推进为该 Commit 的 after revision。未知目录、已存在 ref、路径冲突、拓扑不符、branch/HEAD 不符或无法确定来源时进入 `human_required`；不得执行 `git worktree remove/prune`、删除目录或推测性修复。

SourceControl 只接收受类型约束输入，以参数数组运行 Git；Client、Worker、Runtime 和 Ticket 文本均不能提供任意 Git 命令。

## Scheduling, Authorization and Ticket State

### 顺序

- Ticketize 成功时持久化不可变 CanonicalTicketOrder；同一 Session 从 RunnableTicket 中选择最小 order。
- 初始 ExecutionDispatchEpoch 使用 ExecutionSession 持久化创建顺序。Project 同时至多一个已发放 Lease、会写 Workspace 的 Assignment。
- Worker 暂不可用时，FIFO 队首保持位置且不发放 Lease、不启动 Runtime；后续 Change 不得跳过它。
- Pause 后 Resume、或 Execute 阶段 HumanDecision retry，创建新的 DispatchEpoch 并排入 Project FIFO 队尾；Cancel 永不重新入队。
- Worker、UUID、墙钟时间、Worker 到达顺序和内存队列不得决定调度。

### DefaultExecutionAuthorizationPolicy.v1

自动授权不使用自由文本风险评分或通用 Governance Aggregate。它只在以下条件同时成立时形成 ExecutionAuthorization：

- Change 为 `active/Execute`，Session 与当前 DispatchEpoch 有效且位于 Project FIFO 队首；
- Workspace 的物理身份、BaseRevision、当前 WorkspaceInputRevision、branch 与持久化记录一致；在 Ticket 09 中 WorkspaceInputRevision 必须等于 BaseRevision；
- Ticket 是当前 RunnableTicket，且没有该 Ticket 的活跃或已围栏 Authorization/AgentRun；
- ExecutionMode 是 `edit`，Runtime 是 `codex`；
- InstructionInputProjection、timeout、输入摘要和 WorkspaceBoundary 已验证；
- branch 选择可追溯到初始 ExecuteCommand。

Authorization 持久化 policy version、逐项判定结果和 ExecuteRequestIdentity。Worker 不可用或未到队首时保持等待；Artifact/存储暂时不可用时可重试；Workspace、branch 或证据身份不变量失败进入 `human_required`；其他前置条件不满足以稳定冲突拒绝且不创建 Assignment。

CanonicalTicket 只使用 `pending`、`assigned`、`succeeded`、`human_required`。所有 blocker 成功后才可能 Runnable；一次成功只完成该 Ticket。M7 不实现下游 Verify/Commit gate，不能借 Ticket 09 的 Scheduler 提前实现 Ticket 10 行为。

## Worker, Runtime and Claim Contract

### 执行模式与 Git

| ExecutionMode | Runtime 权限 | 经 Guard 中介允许的 Git 子命令 |
| --- | --- | --- |
| `inspect` | 只读 Runtime | `status`、`diff`、`rev-parse`、`show`、`log`、`ls-files`、`check-ignore` |
| `edit` | Ticket 09 默认；可写 Assigned Workspace | `inspect` 集合加 `add` |
| `source_control` | Daemon 内部能力，不是 Worker/Codex 参数 | 不适用 |

其他经 Guard 中介的 Git 子命令一律拒绝，包括 commit、push、merge、rebase、cherry-pick、reset、checkout、switch、restore、clean、worktree、stash、branch、tag、config、remote、fetch、pull。Guard 不是完整 OS sandbox；绝对路径 Git、其他工具和直接文件写入不能单独被它证明安全，必须再经 WorkspaceSnapshot、身份核验和完整证据拒绝伪造成功。

新建文件必须在 `edit` 模式中由 `git add` 纳入 index；残留 untracked 文件使完整证据不成立。

### Assignment delivery and claim

1. Scheduler 为已注册 Worker 创建唯一 Assignment 和有限 Lease，取得 ProjectExecutionSlot；Pull 只能可重放地交付相同 Assignment，不能启动 Runtime。
2. Worker 为该 Assignment 生成 RuntimeClaimID，并通过窄 Claim DTO/端点请求启动资格。
3. Daemon 只将第一个有效 Claim 原子绑定为该 Lease 的启动资格；相同 Claim 重放相同结果，不同 Claim、不同 Worker、失效 Lease 或已围栏 Assignment 都不能启动 Runtime。
4. 每个 Worker 同时至多持有一个已 Claim 或运行中的 Assignment。Claim 成功后才允许启动 Runtime。

M7 为每份 ExecutionEnvelope 固定 `30m` timeout，计时从 Runtime 实际启动开始；timeout 同时写入 Envelope、Assignment 和 Runtime context。Lease 续约、Client、Worker 本地默认值和 Runtime 都不能延长它。

Daemon 在授权前读取并校验输入 ArtifactContent，按冻结引用顺序渲染 UTF-8 `ExecutionInstruction`，最大 64 KiB。Worker 不获得 Artifact 下载权限、物理路径或本地挂载。临时读取失败可重试；缺失、摘要不符、非 UTF-8、媒体类型不支持或超限进入 `human_required`，不创建 Assignment。

## Evidence and Success Contract

1. 成功 RuntimeClaim 后、子进程启动前，Daemon 验证 WorkspaceBoundary 并采集 WorkspaceSnapshot(pre)。
2. Runtime 退出后，Worker 上报固定四种 WorkerArtifactKind：stdout、stderr、diff、changed_files。Worker 不上报 Ticket、Snapshot、Delta 或生命周期判断。
3. 在同一 Lease/Claim 仍有效时，Daemon 于接受 Report 前采集 WorkspaceSnapshot(post)，生成规范化、非空 CompleteChangeDiff 和非空 TicketDelta。
4. 成功必须同时满足：退出码为零、无 Guard/Capture 失败、HEAD 保持该 Ticket 的 WorkspaceInputRevision、branch 未变、无 residual untracked 文件、Worker 的 `diff`/`changed_files` 未截断且与 Daemon 相对于同一 WorkspaceInputRevision 的完整未提交状态一致、CompleteChangeDiff 与 TicketDelta 非空。Ticket 09 不创建 Commit，因此其 WorkspaceInputRevision 等于 BaseRevision；Ticket 10 才会以受控 Commit 推进下一张 Ticket 的输入 revision。
5. Git 采集在 SQLite transaction 前完成；已采集 Snapshot、Worker 原始 Artifact、完整 Diff、Delta、AgentRun 终态、Ticket `succeeded` 和审计事件必须在同一 authority transaction 落盘。

`stdout` 与 `stderr` 可以保留其长度限制和截断元数据，但不能代替完整 Diff 证据。临时采集、Artifact 或 SQLite 不可用时，只允许同一 Report 重试且不推进状态；证据不一致或任何不变量失败进入 `human_required`。该流程证明受围栏窗口内可观察的 Git 状态，不声称识别外部进程的具体修改主体或提供完整 OS 隔离。

## Fencing and Recovery

| 情形 | 权威结果 |
| --- | --- |
| Worker 尚不可用、Assignment 未发放 | Session/Authorization 保留等待和 FIFO 位置；不启动 Runtime、不进入 human_required |
| Lease 过期、Worker 丢失、Daemon 重启、Heartbeat 续约失败 | ExecutionFence：取消 Runtime context，宽限期后终止子进程；AgentRun 失败、Ticket `human_required`、释放 Slot；晚到 Report 只留 Trace |
| Pause | ExecutionControlFence：取消 Runtime、释放 Slot；未成功 assigned Ticket 回到 pending；Workspace/Diff 保留；Resume 以新 epoch/new Authorization/new AgentRun 入队 |
| Cancel | 围栏并保留 Workspace/Diff/Trace，但终止 Session，绝不重新排队、重试或自动清理 |
| HumanDecision retry | 仅失败的同一 Ticket 回到 pending；保留 Workspace、失败 Diff 与已成功 Ticket；不 reset/checkout/clean |
| Report 与 Pause/Cancel 竞争 | SQLite 首个提交为准；Report 先提交则保留结果，控制命令作用于后续；控制命令先提交则 Report 为 LateReport |

## Persistence and Migration

Ticket 09 在 `workstore.Migrations()` 中追加实际已注册链的下一条 Migration，绝不预占或硬编码版本号。它建立：

```text
t_execution_sessions
t_workspace_provisioning_intents
t_workspaces
t_execution_dispatch_epochs
t_execution_authorizations
t_ticket_execution_states
t_workspace_snapshots
t_ticket_execution_evidence
```

既有 `t_agent_runs` 增加 CanonicalTicket/Authorization 关联；`t_execution_authorizations`、`t_workspace_snapshots` 与 `t_ticket_execution_evidence` 持久化不可变 WorkspaceInputRevision；既有 `t_worker_leases` 增加 Envelope 摘要、timeout、交付和 RuntimeClaim 状态，不创建平行 Assignment 表。

SQLite 以 Project/Change/Graph 复合外键、CHECK、partial unique index 与受限 transition trigger 强制：每个 Change 最多一个 Workspace 和未终结 Session、每个 Ticket 最多一个活跃 Authorization、每个 Project 最多一个活跃写 Workspace Lease、每个 AgentRun/Lease 只有一个 Claim、每个 Ticket 只有一份成功证据。Application 先做可读校验；SQLite 是最终并发防线。

## Implementation Order

实现只能在 Ticket 06、08 的真实实现和验收证据均已重新核验后开始。按下列纵向顺序实施，每一步保持可测试：

1. 建立 `execution/domain`、`execution/application` 与 Port，添加下一条 Schema Migration、核心约束和 Ticket 09 读取模型骨架。
2. 实现 `sourcecontrol`，复用 `repository.Git`，覆盖 provisioning intent、真实 Worktree 创建、物理身份和恢复拒绝。
3. 实现 Session、DispatchEpoch、结构性 Authorization、Ticket state、Project slot 和 Workstore 原子事务。
4. 扩展 Worker Contract/Store/Runner 的 timeout、可重放 delivery、RuntimeClaim、Heartbeat 失效取消和 fenced late-report 行为。
5. 实现完整 Snapshot、Worker/Daemon 双重 Diff 比对、TicketExecutionEvidence、成功/失败/暂时不可用分类。
6. 接入专用 HTTP/CLI、ExecutionReadModel、迁移升级、端到端恢复测试和所有受影响 `INDEX.md`。

不得以空 package、TODO、假成功、直接 Handler SQL 或临时绕过来跨越依赖；下游 Ticket 10 的 Verify/Commit 由其自身 Ticket 实现。

## Testing Decisions

| 测试层级 | 必须证明的事实 |
| --- | --- |
| Execution domain/application | 结构性授权合取、稳定 FIFO/CanonicalTicketOrder、状态转换、epoch 重排、错误分类和不变量 |
| SourceControl 集成 | 真实 Git 的默认/自定义 branch、干净/脏源、物理路径/符号链接重叠、intent 恢复、冲突拒绝与不自动清理 |
| Workstore SQLite | predecessor 升级、复合 FK、unique/CHECK/trigger、重复 Execute、并发授权、重复 Claim、rollback、Lease 围栏和跨 Change 拒绝 |
| Worker/Runtime | Pull 可重放但 Claim 唯一、timeout 从运行开始、Heartbeat 失效取消、Git allowlist、输入投影和脱敏环境 |
| Evidence | pre/post Snapshot、staged/unstaged/new file、untracked 拒绝、完整 Diff/changed-files 一致性、同 Report 重试与 LateReport |
| Daemon/Contract/CLI | `202` 回执、Idempotency-Key、版本冲突、执行读取脱敏、CLI 参数与不隐式启动/授权 |
| 跨平台 | Linux/WSL 与原生 Windows 上真实 Git Worktree fixture；Windows 必须原生运行，不能以交叉编译替代 |
| 人工验收 | 依赖满足后，真实 Codex 在真实临时 Git Repository 的 Assigned Workspace 形成可追溯 Diff；该结果不代替自动化测试 |

自动化可使用确定性测试 Runtime 和 Codex 可执行 stub 验证 argv、Workspace 与 sandbox，不要求 CI 登录或调用真实模型。

完成代码变更后至少执行：

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
git diff --check
```

Ticket 09 不修改 Dashboard 时，不应把 Dashboard build 当作 M7 行为证据。

## Decision References

- `CONTEXT.md`：Execute、Workspace、Authorization、Scheduler、Evidence、SourceControl 与恢复术语。
- ADR-0032 至 ADR-0036：显式 Execute、provisioning、请求身份、Ticket 状态与执行信封。
- ADR-0039 至 ADR-0049：确定性调度、Lease/Claim、Runtime 模式、timeout、双重证据、API、代码边界、拓扑、恢复与授权策略。
- ADR-0055：全局 BaseRevision 与逐 Ticket WorkspaceInputRevision 的后续 Commit 链边界；Ticket 09 只实现其初始相等情形。
- ADR-0057：执行 Schema 与原子迁移。

## Further Notes

- 本规格不解除 Ticket 06、08 的顶层阻塞；开始实现前必须以当前 checkout 的源码、测试和验收记录重新核验它们，而不是依赖规划文档或 ready-for-agent 状态。
- 所有文档中的目标表、端点、DTO、Port 和错误分类均为待实现契约，不能被表述为当前运行行为。
- 保留工作树中与 Ticket 09 无关的改动；不得重置、清理、删除或覆盖并行 Ticket 的文档、Schema 或实现工作。

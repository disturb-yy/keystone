# 10：Verify、Commit 与 Integrate Ready 实施规格

> 状态：规划规格已对齐，尚未实现。顶层 Ticket 09 的真实实现和验收是硬阻塞；本规格及子票的 `ready-for-agent` 仅表示设计已可实施，不表示当前 checkout 已有 M8 行为。
>
> 用途：为每张已执行 CanonicalTicket 建立独立验证、受控 Git 收口和最终候选验证链，使 `integrate_ready` 成为可回放证据的终点，而不是 Worker、Client 或 Git 当前状态的自报。

## Problem Statement

Ticket 09 完成后，一张 CanonicalTicket 只能证明受授权 Runtime 在 Change Workspace 中形成了可观察的执行证据；它不能证明固定验证命令成功、每条 Acceptance Criterion 被独立审查、候选在验证时未改变，也不能让 Runtime 自行创建可信 Git Commit。若把命令文本交给 Client、把 Implementer 的成功摘要当作 PASS、允许任意 Worker 回调推进状态，或在 Git 成功后才试图猜测 SQLite 记录，重送、断电、Worker 升级、Lease 围栏和多张 Ticket 串行执行都会产生不可审计的分叉。

本 Ticket 还必须处理两个 V1 约束：既有 ProjectManifest V1 只表达身份，既有 Worker Protocol 是严格解码的 `worker/v1`；不能通过静默改写 Manifest 或无条件发送未知 JSON 字段来取得表面兼容。每张 Ticket 的 KeystoneCommit 会改变 Workspace HEAD，因此不能继续把每一轮执行都错误地约束为 `HEAD == BaseRevision`。

## Solution

M8 保留 Change 的不可变 BaseRevision，另外为每张 Execute、Ticket Verify 与 FinalVerify 固定 WorkspaceInputRevision。首张 Ticket 从 BaseRevision 开始；每张 Ticket 的 KeystoneCommit after revision 是下一张 Ticket 的输入 revision。每个 Ticket 的执行、验证和提交在同一个 TicketVerifyCommitGate 内收口，未完成前 Scheduler 不得授权下一张 Ticket。

Verify 和 FinalVerify 是 Intent-first 的异步 Command：Daemon 先在权威事务中持久化不可变 VerificationIntent 与 `202 Accepted` Receipt，再在支持 `verification-v1` 的本机 Worker 可用时发放一次受 Lease/RuntimeClaim 围栏的 `kind: verify` Assignment。Verifier 在 Workspace root 以无 shell 的固定参数数组运行命令、审查固定的验收输入，并返回严格结构化结果；Daemon 对 Assignment、Snapshot、Artifact 与 Criterion 引用重新核验后才形成 VerificationOutcome。

Commit 是显式、同步的 Control Plane Command。Daemon 重验已验证候选，用 `git add --all` 形成与 CandidateTreeIdentity 相同的 CommitTreeIdentity，先持久化 CommitIntent，再以受控消息和 `--no-verify` 创建 Git Commit。它只接受预期 parent、tree 和三个 Keystone trailer 全部一致的 Git 结果；中断恢复不能证明唯一结果时保持 `human_required`。

全部 Ticket 已 Commit 后，Change 才从 Execute/active 推进为 Verify/active。FinalVerify 使用 Execute 时已固定的策略和最终干净 HEAD；只有 FinalVerification PASS 才在同一权威事务中记录 CandidateRevision、推进 LifecycleStage 为 FinalVerify 并将 ChangeStatus 置为 `integrate_ready`。

```text
TicketExecutionSuccess
        ↓
TicketVerifyCommitGate ── VerifyCommand ──> VerificationIntent (202 / pending)
        │                                      ↓
        │                              VerifierAgentRun / Evidence
        │                                      ↓
        │                           PASS ──> CommitCommand
        │                                      ↓
        │                                CommitIntent → KeystoneCommit
        │                                      ↓
        └──────────── Scheduler 可授权下一张 Ticket ────┘

全部 Ticket 已 Commit → Change Verify/active → FinalVerifyCommand (202)
                                              ↓
                                      FinalVerification PASS
                                              ↓
                                CandidateRevision + integrate_ready
```

## User Stories

1. 作为 Change 发起者，我希望验证命令来自版本化的 BaseRevision Manifest，以便 Client、Worker 或候选代码不能临时替换验证范围。
2. 作为审计者，我希望能区分 VerifierAgentRun 的传输结果和 Daemon 形成的 PASS、FAIL、HUMAN_REQUIRED，以便不把进程退出码误认为业务批准。
3. 作为已部署旧 Worker 的操作者，我希望其仍可完成既有 edit 工作，而不会收到无法严格解码的验证 Assignment。
4. 作为 Scheduler，我希望当前 Ticket 没有 Verify PASS 和 KeystoneCommit 时不能继续调度，以便每一笔 Commit 都有明确归属和 parent 链。
5. 作为恢复流程，我希望 Git 成功而 SQLite 尚未完成时能够精确对账，而不是重复创建 Commit 或猜测最新 HEAD。
6. 作为人工恢复者，我希望 Verify FAIL、证据不足、暂时不可用和 Lease fence 有不同的处理方式，以便不会用自动清理掩盖未知 Workspace 修改。
7. 作为 Dashboard/CLI 使用者，我希望读取有界的执行进度和证据身份，而不暴露绝对路径、Lease token、原始 Prompt、完整命令输出或凭据。
8. 作为项目维护者，我希望 Final Verify 失败不会改写已提交 Ticket，以便修复仍有新的 Change、新的 BaseRevision 和可审计因果关系。

## Implementation Decisions

### 事实边界、术语与上游条件

- 本规格不解除顶层 Ticket 09 的阻塞。开始任一子票前必须重新核对 Ticket 09 的真实 Workspace、ExecutionEnvelope、RuntimeClaim、Snapshot、TicketExecutionEvidence、Scheduler 和恢复行为；规划文档或 fake seam 不能替代运行证据。
- BaseRevision、WorkspaceInputRevision、CandidateTreeIdentity、VerificationPolicySnapshot、VerificationIntent、VerificationEvidence、CommitIntent、KeystoneCommit、CandidateRevision 和 VerificationRecovery 的准确术语以根 `CONTEXT.md` 为准。
- ADR-0037、ADR-0038、ADR-0050、ADR-0051、ADR-0052、ADR-0053、ADR-0054、ADR-0055 与 ADR-0056 是本 Ticket 的冻结取舍；它们定义目标约束，不证明代码或 Migration 已存在。
- `integrate_ready` 是 ChangeStatus 的 V1 终态，不是 merge、push、PR、deploy 或远程托管的同义词。

### 权责与调用边界

| 区域 | M8 责任 | 明确不负责 |
| --- | --- | --- |
| `internal/governance/domain` | VerificationPolicy、VerificationOutcome、Evidence、Criterion 结果、CommitIntent 与恢复不变量 | SQL、HTTP、Git 命令、Worker Protocol |
| `internal/governance` | Verify/Commit/FinalVerify use case、窄 Port、Gate 收口和状态前置条件 | Handler 参数解析、基础设施具体类型 |
| `internal/work/domain` | Change、CanonicalTicket、TicketVerifyCommitGate、WorkspaceInputRevision 和 Lifecycle 约束 | Verifier 实现、Git/SQLite 细节 |
| `internal/execution` | 受授权的 VerificationIntent 调度、VerifierAgentRun/Lease/RuntimeClaim 组合 | PASS 决定、Commit、Change 生命周期权威 |
| `internal/infrastructure/manifest` | 严格 V2 parser、规范化 digest | 静默升级、Client 配置覆盖 |
| `internal/infrastructure/sourcecontrol` | Snapshot、CandidateTreeIdentity、受类型约束 Commit、Git 对账 | 任意 Git 代理、Worker 权限、自动清理 |
| `internal/infrastructure/workstore` | additive Migration、Intent/Evidence/Commit 的原子持久化和数据库约束 | Domain 规则替代、HTTP/CLI |
| `contracts/worker` | V1 capability-gated DTO 与严格传输校验 | Domain Entity、状态推进、SQLite |
| `internal/worker` | VerificationExecutor、固定命令调用、受限结果采集和同一 Report 重发 | 生命周期批准、Git Commit、数据库访问 |
| `contracts/controlplane` | Command/ReadModel DTO 与安全 ErrorEnvelope | 业务校验、Daemon 逻辑 |
| `internal/daemon`、`cmd/keystone` | 依赖装配、HTTP/CLI adapter、错误映射和安全输出 | 直接 SQL、直接信任 Worker、隐式执行 |

新增 Go package 必须在实际创建时同步添加最近一级 `AGENTS.md` 和 `INDEX.md`。Application 只依赖 Domain 和 Port；Daemon 只组合实现；Worker 与 Contract 不得获得 Change、Gate、Commit 或 SQLite 权威。

### ProjectManifest V2 与策略快照

M8 接受的 V2 语义形状如下；`commit` 可省略：

```yaml
version: 2
project_id: "018f0000-0000-7000-8000-000000000000"
verify:
  commands:
    - name: go-test
      argv: ["go", "test", "./..."]
      timeout_seconds: 900
commit:
  template: "{ticket_title}"
```

| 字段 | 规则 |
| --- | --- |
| root | 只允许 `version`、`project_id`、`verify`、`commit`；单一 UTF-8 YAML document，不接受未知字段、重复 key、alias、custom tag 或多 document |
| `version` | 必须为整数 `2` |
| `project_id` | 与既有严格 UUIDv7 Project identity 相同 |
| `verify.commands` | 必填，1 至 32 条，数组顺序不可改变 |
| command `name` | 唯一，匹配 `[a-z][a-z0-9_-]{0,63}` |
| command `argv` | 1 至 64 个非空 UTF-8 元素；单元素最多 4 KiB，整条参数数组最多 16 KiB；不经 shell 解释 |
| command `timeout_seconds` | 1 至 1800，不能由 Client、Worker 配置或 Lease 延长 |
| `commit.template` | 可选，最多 4 KiB；只允许 `{change_id}`、`{ticket_id}`、`{ticket_title}`；render 后最多 8 KiB，禁止 NUL 和三种受控 trailer 名称 |

Parser 以保留数组顺序的规范化值形成 SHA-256 语义 digest：`verify.commands` 产生 VerificationConfigurationDigest，`commit.template` 单独产生 CommitTemplateDigest。YAML 的注释、缩进和空白不改变 digest；字段值、命令顺序、参数、timeout 或模板变化都会改变对应 digest。

Daemon 在接受 ExecuteCommand 时从 BaseRevision 读取 V2，形成并持久化 VerificationPolicySnapshot；后续 Ticket Verify、Commit 和 FinalVerify 只使用该快照，绝不读取候选 Workspace 中的 Manifest。V1、缺失或不合法的 BaseRevision 配置不由 `keystone init` 或 Daemon 静默改写：Ticket 可以保留既有执行事实，但 VerifyCommand 必须形成可审计的配置不足事实并使该 Ticket/Change 进入 `human_required`，不创建 Worker Assignment 或 Commit。

### Revision、Workspace 与 Snapshot 链

Change 的 BaseRevision 始终描述创建 Change 时的干净源。WorkspaceInputRevision 是每张 Ticket 的实际 Git parent：首张为 BaseRevision，后续为前一张 KeystoneCommit 的 after revision；FinalVerify 为最后 Commit 的 after revision。Runtime、Verify 和 Commit 都不得把当前 HEAD 作为自由输入。

SourceControl 通过私有临时 index 观察 Snapshot，记录 HEAD、index、staged/unstaged tracked 内容和 CandidateTreeIdentity，不改变实际 Workspace 或 index。新建源码必须在 `edit` ExecutionMode 中通过 `git add` 纳入 tracked 候选；ignored 文件不阻塞，任何 residual untracked 文件都使 Execute/Verify 不能形成 PASS。一次 Ticket 的 CompleteChangeDiff 相对于其 WorkspaceInputRevision 描述完整未提交状态，Worker 的原始 diff/changed_files 仍须与其一致；已提交前序 Ticket 的累计因果关系由 KeystoneCommit 链和各自 TicketDelta 表达。

Ticket Verify 前、固定命令后、独立审查后，Daemon 都必须核验 WorkspaceInputRevision、WorkspaceBranch 和 CandidateTreeIdentity 不变。Verify 只提供可观察的候选不变性，不宣称完整 OS sandbox。当前 Ticket Verify FAIL 后的修复可在同一、已知候选上增量执行；其下一次 Execute 前 Snapshot 必须与失败证据的候选 Snapshot 相同。Fence、外部变化或未知状态不能被自动 reset/clean，必须先由人恢复到耐久 Snapshot。

### Worker V1 验证扩展

M8 保持 `protocol_version: "v1"` 和既有 `/worker/v1/register`、`heartbeat`、`pull`、`report` 路由。支持验证的 Worker 在 Register 中声明精确 capability `verification-v1`；Daemon 只向该 Worker 发放 `kind: "verify"` Assignment。未声明该 capability 的 Worker 只接收缺省 `kind` 的既有 edit Assignment，旧二进制不会被分配未知字段。

`VerificationAssignment` 是 V1 Assignment 的 capability-gated 变体，至少固定：

| 组 | 必须携带的内容 |
| --- | --- |
| 关联与围栏 | `agent_run_id`、Lease、attempt、WorkspaceID/path、VerificationIntentID、Ticket 或 Final scope |
| 不可变输入 | policy digest、WorkspaceInputRevision、CandidateTreeIdentity、按 ordinal 排列的命令 name/argv/timeout |
| 审查输入 | Change/Ticket 的规范化、受限文本，AcceptanceCriterionRef 与原文、已采集 Evidence identity；最终审查包含全部 CanonicalTicket 的 criterion 输入 |
| 执行限制 | 明确 `kind: verify`、Workspace root、无 shell、清理环境、只读 Git policy 和全量 timeout 边界 |

审查输入必须是 Daemon 派生的 UTF-8 数据，最大 64 KiB；无法在该上限内安全投影时进入 `human_required`，不得把任意 Artifact、绝对路径、凭据或 Client Prompt 交给 Worker。固定命令由 Worker 的 VerificationExecutor 以参数数组直接启动，cwd 固定为 Assigned Workspace root；Manifest 不可指定 cwd、环境变量、shell 文本或忽略失败。运行环境只继承 Daemon/Worker 明确允许的 `PATH`、`HOME`、`TMPDIR`、`LANG`、`LC_ALL` 与必要 Keystone 运行字段，并移除 Client/Manifest 注入和 `GIT_*` 覆盖。`verify` ExecutionMode 不授予 `git add`、commit、push、merge、reset、清理或自由 Prompt 权限。

`VerificationReport` 在 `kind: verify` 时携带按 command ordinal 排列的结果、逐 Criterion 结果和受限审查摘要。每条命令 stdout/stderr 是类型化的 VerificationEvidence Artifact：每 stream 最多 256 KiB，带 SHA-256、原始长度和 `truncated`；因此 32 条命令的最大原始 stream 内容为 16 MiB，保持既有 Worker V1 64 MiB body 上限内。验证 Assignment 的通用 `Report.Artifacts` 必须省略，避免把逐命令输出伪装成四类通用 Artifact；edit Assignment 的 stdout、stderr、diff、changed_files 四类和既有大小限制完全不变。

每条 Criterion 结果必须携带 Ticket ID、1 起始 ordinal、规范化文本 SHA-256、PASS/FAIL/HUMAN_REQUIRED 和 Evidence 引用。Verifier 必须在命令首次失败或 timeout 后把后续命令记录为 `not_run`，但仍须在可完成时提交按 canonical order 精确覆盖的 Criterion 结果；重复、遗漏、乱序、摘要不符或未能形成结构化审查都不能形成 PASS。Worker 的 `Report.Outcome` 只是传输终态；Daemon 比对 Report 与 Assignment 后才产生 VerificationOutcome。

### Verification Intent、Evidence 与恢复

Ticket Verify 仅在当前 Ticket 已有 TicketExecutionSuccess、Gate 仍属于该 Ticket、Change 为 Execute/active 时可接受。FinalVerify 仅在所有 Ticket 已有 KeystoneCommit、Change 为 Verify/active、最终 Workspace 干净时可接受。两个 Command 都在单一 authority transaction 中持久化 VerificationIntent、初始 `202 Accepted` Receipt、请求身份、策略快照、WorkspaceInputRevision、Snapshot、Criterion 引用和审计 Event；不等待 Worker、命令或 Verifier。

| 观察 | 权威处理 |
| --- | --- |
| 无可用 `verification-v1` Worker | Intent 保持 pending；不创建 Lease/AgentRun，不进入 human_required |
| Artifact/SQLite 在任何 verdict 前暂时不可用 | VerificationUnavailable；同一 key 可重驱动同一 Intent，不新增活跃 attempt |
| 已 Claim 后 Lease/Runtime fence | 按 Ticket 09 围栏成为 `human_required`；晚到 Report 只可 Trace |
| 任一命令非零或 timeout | VerificationOutcome 为 FAIL；后续命令为 `not_run`，不创建 Commit |
| 成功命令输出截断、候选 Snapshot 改变、Guard finding、缺少必要 Artifact 或 Criterion 覆盖不完整 | VerificationOutcome 为 HUMAN_REQUIRED |
| 全部命令成功、输出完整、Snapshot 不变且每条 Criterion PASS | VerificationOutcome 为 PASS |

Ticket Verify FAIL 的 HumanDecision `retry` 创建同一未 Commit Ticket 的新 Execute AgentRun，前 Snapshot 必须严格等于失败候选 Snapshot；这允许受审计的增量修复，但不允许隐式丢弃候选。仅因证据不足导致的 Ticket HUMAN_REQUIRED 可在相同 WorkspaceInputRevision/CandidateTreeIdentity 上创建新的 VerifierAgentRun；围栏、未知变化或 Guard 失败必须先由人恢复到相应耐久 Snapshot。Final Verify FAIL 必须从 CandidateRevision 创建新 Change；Final Verify HUMAN_REQUIRED 只可在 revision 和 Snapshot 均不变时重验。

### Ticket Gate、Commit 与恢复对账

每个 TicketExecutionSuccess 打开唯一 TicketVerifyCommitGate。Scheduler 在该 Gate 形成 PASS VerificationEvidence 和 KeystoneCommit 前，不得为任何下一张 Ticket 创建 ExecutionAuthorization、Assignment 或 RuntimeClaim；Ticket Verify FAIL/HUMAN_REQUIRED 使 Change 保持 Execute/human_required。所有 Gate 已 Commit 后，Daemon 在同一事务中把 Change 由 Execute/active 推进至 Verify/active。

CommitCommand 的唯一成功路径为：

1. 确认版本前置条件、当前 Gate、PASS VerificationEvidence、WorkspaceInputRevision、WorkspaceBranch 和已验证 CandidateTreeIdentity 仍匹配。
2. 重新采集 Snapshot；确认没有 residual untracked，然后执行受类型约束的 `git add --all`，取得必须等于 CandidateTreeIdentity 的 CommitTreeIdentity。
3. 从 V2 CommitTemplate 或默认的规范化 Ticket title 渲染消息，附加且只附加 `Keystone-Change-ID`、`Keystone-Ticket-ID`、`Keystone-Commit-ID` 三个 trailer。模板不能伪造、删除或重复这些 trailer。
4. 在 SQLite 持久化 CommitIntent：Ticket、预期 parent=WorkspaceInputRevision、tree、VerificationEvidence、CommitTemplateDigest、KeystoneCommitID 与请求身份。
5. 以无 shell 参数数组执行 Git Commit，使用 Workspace 已配置的 Git author/committer；`git var GIT_AUTHOR_IDENT` 或 committer identity 不可用时进入 `human_required`。Commit 固定使用 `--no-verify`，因为必要验证只能来自已记录的 VerificationEvidence，而非未记录 hook。
6. Git 返回后确认 HEAD 直接承接预期 parent、tree 等于 CommitTreeIdentity、三个 trailer 与 Intent 全等、Workspace 干净；随后在一项 authority transaction 中记录 KeystoneCommit、完成 Gate、更新当前 WorkspaceInputRevision、写入 Event 和同步成功 Receipt。

进程中断或同一 Command 重送时，Daemon 只能以 CommitIntent、预期 direct parent、tree 和唯一 KeystoneCommitID trailer 对账。发现唯一完全匹配 Commit 时只补齐权威记录；不存在、多个候选、HEAD 已被外部推进、tree/trailer 不同或 Workspace 不干净时进入 `human_required`，绝不创建第二个 Commit、amend、reset、rebase、force、clean 或猜测修复。

### Final Verify 与 Integrate Ready

最后一个 Gate 完成后，FinalVerifyCommand 固定最终 WorkspaceInputRevision、干净 WorkspaceSnapshot、CandidateTreeIdentity、策略快照、所有 KeystoneCommit 与全部 Criterion 引用。Verifier 重新运行同一 ordered commands，并独立于所有 Implementer AgentRun 审查 ChangeIntent、每张 Ticket 的 Criterion 及证据链；它不修复候选，也不创建 Commit。

FinalVerification PASS 必须在同一 authority transaction 中写入 Final VerificationEvidence/Outcome、CandidateRevision、StageAdvanced 至 FinalVerify、ChangeStatus `integrate_ready` 和审计 Event。FAIL 使 Change 保持 Verify/human_required，已 Commit Ticket 永不重开；修复只能由以 CandidateRevision 为起点的新 Change 表达。HUMAN_REQUIRED 保持 Verify/human_required，并只允许未变候选重验。M8 不执行 merge、push、PR、deploy、远程调用、回滚或自动恢复。

### Control Plane、CLI 与 ReadModel

固定端点如下：

```text
POST /v1/changes/{change_id}/tickets/{ticket_id}/verify
POST /v1/changes/{change_id}/tickets/{ticket_id}/commit
POST /v1/changes/{change_id}/final-verify
GET  /v1/changes/{change_id}/execution
```

三个写命令的 JSON body 只能包含 `expected_version`，并必须提供 `Idempotency-Key`。同一 key 仅在操作、Change、可选 Ticket 和规范请求身份都相同时重放首次成功响应；同 key 不同请求返回 `idempotency_conflict`。Receipt 命中后才按既有规则检查版本前置条件。Verify/FinalVerify 的初始成功响应是关联 Intent 的 `202 Accepted`，即使尚无 Worker；Commit 的 `200 OK` 只在 KeystoneCommit 已记录后返回。输入错误、状态/Gate 冲突、V1/无效策略和未持久化 Intent 的暂时不可用不创建成功 Receipt。

CLI 固定映射为：

```text
keystone change verify CHANGE_ID TICKET_ID --expected-version N --idempotency-key KEY
keystone change commit CHANGE_ID TICKET_ID --expected-version N --idempotency-key KEY
keystone change final-verify CHANGE_ID --expected-version N --idempotency-key KEY
keystone change execution show CHANGE_ID
```

`GET /v1/changes/{change_id}/execution` 返回有界 ExecutionReadModel：Change stage/status/version、按 canonical order 的 Ticket gate/verification/commit 摘要、pending/running/completed VerificationIntent 身份、命令/criterion outcome 摘要、KeystoneCommit before/after revision、CandidateRevision 和 FinalVerification 摘要。它不公开绝对 Workspace 路径、branch 物理位置、Lease token、原始命令输出、Prompt、环境或凭据；内容仍只通过既有 Artifact 读取边界取得。

### 持久化、幂等与数据库约束

Workstore 在实现时使用下一个可用 additive Migration，并保留既有 Event、Artifact、AgentRun、Ticket Graph、Lease、LateReport 与 Migration checksum。目标持久化事实至少包括：

| 事实 | 最小约束 |
| --- | --- |
| VerificationPolicySnapshot | 每个 ExecuteSession 固定 BaseRevision、commands digest、template digest 和规范值；V1 不伪造快照 |
| VerificationIntent | Change + scope + optional Ticket、请求 identity、输入 revision、Snapshot/tree、状态与 attempt；同一 Gate/FinalVerify 至多一个活跃 Intent |
| VerificationEvidence | Intent、VerifierAgentRun、前后 Snapshot、Outcome、不可变 command/criterion 结果；终态后禁止更新/删除 |
| VerificationCommandResult | Intent + ordinal 唯一；声明身份、状态、exit code、typed stdout/stderr ArtifactRef、截断事实 |
| AcceptanceCriterionResult | Intent + Ticket + ordinal 唯一；Text SHA-256 必须对应同一 Canonical Graph 的 criterion |
| CommitIntent / KeystoneCommit | 每 Ticket 至多一个成功 Commit；parent、tree、Evidence、template digest、KeystoneCommitID、Git OID 与唯一 trailer 对账身份不可替换 |

验证完成、AgentRun 终态、Evidence 引用、Ticket Gate/Change 状态、Commit 记录、Receipt 与 Event 必须各自在适用的单一 SQLite authority transaction 内同时可见。Git 与 SQLite 不能形成跨资源物理事务，因此 CommitIntent 是唯一恢复锚点；不得用 `git log` 的最新提交、reflog 或 Client 重送语义取代精确对账。

### 实施子票图

```text
顶层 Ticket 09 的真实实现与验收
                 ↓
10-01 Manifest V2、策略快照与验证领域骨架
                 ↓
10-02 Ticket Verify、Worker V1 能力与独立证据
                 ↓
10-03 Keystone Commit、输入 revision 链与 Gate 收口
                 ↓
10-04 Final Verify、Control Plane 与受限读取模型
                 ↓
10-05 M8 集成验收、恢复证据与导航
```

每张子票是前一张可观察行为的纵向扩展。不得把 Domain、Migration、HTTP、CLI、Worker 或 Git 拆成彼此不可验证的横向“完成”；也不得在 Ticket 10 内提前实现 Dashboard、Golden Path 或下游远程集成。

## Testing Decisions

| 测试层级 | 必须证明的事实 |
| --- | --- |
| Manifest/domain 单元测试 | V1/V2、严格 YAML、未知/重复字段、digest 稳定性、命令/template 边界、Criterion identity、Outcome 与恢复分类 |
| Governance/work application 测试 | Intent-first/Receipt、Gate 前置条件、PASS/FAIL/HUMAN_REQUIRED、同 key 重放、无 Worker pending、Snapshot checkpoint 和 Final 状态转换 |
| Worker Contract/runner 测试 | V1 旧/新 capability 共存、严格解码、`kind: verify` 仅路由到新 Worker、无 shell argv、timeout/fail-fast/not_run、256 KiB 截断和同一 Report 重送 |
| SourceControl 与 Workstore 集成测试 | 临时真实 Git repo、CandidateTreeIdentity、`git add --all` 一致性、author 缺失、固定 trailers、CommitIntent 中断对账、重复/冲突 Commit、Migration 与不变量 |
| Daemon/CLI 测试 | 四个端点、202/200/错误分类、请求/版本/幂等、无敏感字段的 ReadModel 和 CLI 参数/输出 |
| 串行与恢复测试 | 两张依赖 Ticket 的 BaseRevision→Commit1→WorkspaceInputRevision 链、Verify FAIL 增量修复、evidence-only human retry、fence/外部变化拒绝、Final FAIL 新 Change、Final HUMAN 未变重验 |
| 跨平台真实 Git fixture | Linux、WSL 和原生 Windows 的 Worktree/Snapshot/Commit/恢复路径；Windows 必须原生执行，交叉编译只可作为补充 |

实现完成后至少运行：

```bash
go test ./...
go vet ./...
make build
git diff --check
```

并运行受控 fake Verifier 的端到端真实临时 Git fixture；若依赖可用，再单独记录真实 Codex/Worker smoke。未执行、失败或缺少原生 Windows 证据必须诚实报告为阻塞，不能由文档、交叉编译或模拟成功替代。

## Out of Scope

- `worker/v2`、远程 Worker、Worker pool、消息队列、TLS、远程认证、多租户调度或新的 Worker 权威状态。
- 任意 shell、Client/Manifest 提供的 cwd/env、自由 Prompt、完整 OS sandbox、自动重试、自动 Worktree 清理或 Git 历史改写。
- merge、push、PR、deploy、远程 Git 托管、自动回滚、amend、reset、rebase、force 或清理用户文件。
- Dashboard 页面、SSE、可浏览 Workspace、完整日志导出和 Ticket 11/12 的 Golden Path 交付。
- ProjectManifest V1 的隐式升级、兼容性猜测或通过候选 Manifest 改写当前 Change 的验证/提交策略。

## Further Notes

- 开始实现前必须重读根 `AGENTS.md`、根 `INDEX.md`、目标 package 的局部 `AGENTS.md`/`INDEX.md`、相关源码和测试；本规格不能替代这些当前 checkout 事实。
- 现有 `contracts/worker` 的 V1 body 上限、四类通用 Artifact 和 strict decoder 是兼容边界；新增验证字段只能在 `verification-v1` Worker 的明确 Assignment/Report 变体中出现。
- 所有源码、DTO、Migration 版本、端点字段和错误码在实现前都须根据当时 checkout 验证；本文件定义的是受控目标契约，不是当前运行行为证明。

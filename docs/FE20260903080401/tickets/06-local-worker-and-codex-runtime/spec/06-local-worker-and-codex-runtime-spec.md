# 06：Local Worker、Codex Runtime 与执行证据本地交付规格

> **状态：** 规划规格。它记录 Ticket 06 已对齐的 M4 设计，不表示 Worker、Runtime、Supervisor、业务 Schema 或真实 Codex smoke 已经实现，也不解除顶层 Ticket 06 对 Ticket 05 的 `BLOCKED_BY`。
>
> **用途：** 作为 Ticket 06 五个实施子票的共同契约；本地子票的 `ready-for-agent` 只表示文档成熟度，不表示当前 checkout 已具备实现前置条件。

## Problem Statement

Ticket 05 为 Change、Lifecycle、Artifact、Event、AgentRun、HumanDecision 和恢复围栏建立了 M3 的权威模型，但本机还没有一条真实的副作用执行通道。没有独立 Worker，Daemon 既不能监管执行进程，也不能把一次受授权的 AgentRun 交给具体 Runtime；没有 Runtime Adapter，Codex 的命令细节会泄漏到协议或生命周期；没有独立证据采集，stdout、stderr、exit code、diff 和 changed files 也可能被 Runtime 自报或半完成的日志替代。

如果 Worker 直接写 Keystone SQLite、推进 Change、接受自己的 Verify 结论，或者把 Lease 当成可复用任务槽，进程重启、网络重试、Report 并发和 Daemon 重启就会覆盖权威历史。另一方面，本机 Runtime 需要 repository-local shell、测试命令和文件读写能力；V1 不能把提示词禁令误写成完整操作系统沙箱保证。

## Current Checkout Fact Boundary

以下事实来自当前 checkout，必须与本规格的目标设计分开阅读：

- `contracts/worker` 当前只有 Register、Heartbeat、Assignment、Report 的窄 DTO；它不实现 HTTP、鉴权、Lease、幂等、Supervisor、Runtime 或 Artifact 采集。
- 当前已落地的是 M1 本机 CLI/Daemon、LocalState、单实例锁、SQLite readiness、Migration runner、Control Plane 最小 HTTP Contract，以及 Ticket 04/05 规划输入；当前没有 `cmd/keystone-worker`、`internal/worker`、`internal/execution` 或 Worker Runtime 行为。
- 当前 Daemon 已有 loopback HTTP、SQLite 和资源关闭顺序，但没有 Worker 子进程监管、Worker protocol 路由或 AgentRun 执行分配器。
- Ticket 05 的目标模型提供 AgentRun 一次性终态、Artifact 关联、Event 账本和 Pause/Cancel 晚到结果围栏；这不是本 Ticket 06 可以绕过的实现事实。
- 本机 `codex --version` 当前可见 `codex-cli 0.153.0`。这只是执行环境探针，不是已经完成 Codex smoke 的证据。
- 当前仓库未保存 Ticket 06 的真实 smoke 结果。本规格中的 ID、摘要、退出码、diff 和 Trace 查询都是待真实运行后才能填写的证据字段。

## Goal

在 Ticket 05 完成并满足其持久化前置条件后，由一个本机 Daemon 监管一个独立 Worker 进程。Worker 通过经过鉴权的 loopback JSON `WorkerProtocol` 主动 Pull 一个已授权的 Assignment，在 Assigned Workspace 中通过 `RuntimeAdapter` 调用真实 Codex CLI，独立采集有界执行证据，并向 Daemon 提交一次可验证、可重放、受 Lease 围栏保护的 Report。

M4 的最高验收 seam 是一个不修改当前工作树的临时 Git Repository/LocalStateRoot：Daemon、Worker 和真实本机 Codex 完成一次有实际源码修改的 smoke，Daemon 形成 AgentRun 终态、Artifact、关联和 Trace；重复 Report 不重复推进，失效 Lease 的 Report 不能改变权威状态。

## Scope

### 1. 独立 Worker 与窄协议

- Daemon readiness 完成后启动并监管一个本机 Worker；V1 只支持一个长期 Worker 进程。
- Worker 主动向 Daemon 注册、发送心跳、Pull Assignment 和提交 Report。`Execute` 是 Worker 内部的本地动作，不是 Daemon 暴露给 Client 的控制面路由。
- Worker Protocol 使用 loopback HTTP/JSON，路由固定为 `/worker/v1/register`、`/worker/v1/heartbeat`、`/worker/v1/pull` 和 `/worker/v1/report`。
- 每个 Worker 进程使用一次性 WorkerInstance 身份和临时 Bearer secret。secret 由 Daemon 通过启动管道交付，不进入 JSON、命令行参数、Runtime 环境或日志；Daemon 只保存不可逆摘要。
- 协议只传递执行所需的授权、输入摘要、Assignment 和结果，不承载 Change/Ticket/Gate/Recovery 的状态命令，也不允许 Worker 直接访问 Keystone SQLite。

### 2. Assignment、Lease 与 Report 权威

- 一个 Assignment 绑定一个不可复用的 AgentRun、一个 attempt、一个 WorkerInstance 和一个有限 Lease。
- Lease 默认每 5 秒由心跳续租，默认 TTL 为 30 秒；实现必须使用可注入时钟和配置，不能把时间等待写死在 Domain 中。
- Lease 具有 `active`、`expired`、`revoked`、`consumed` 生命周期。首次合法终态 Report 消费 Lease；同一 Report 的重放仍按保存的指纹安全返回结果。
- Daemon 只接受有效 Worker 身份、匹配 Worker/AgentRun/attempt 的 Lease 和 AgentRun `running` 状态的首次终态 Report 作为权威输入。相同终态重放为 `duplicate`；已完成但内容不同的终态为 `terminal_conflict`，不能改写历史。
- Lease 过期、撤销、Daemon 重启、旧 Worker 身份或非当前 attempt 的结果不能改变权威 AgentRun/Change；经过鉴权且可安全保存的结果作为 `LateReport` 和 Artifact 进入 Trace。
- Daemon 重启时撤销旧 WorkerInstance 的活动 Lease，把仍为 `running` 的 AgentRun 收敛为 `failed`/`daemon_restarted`；若所属 Change 仍为 active，则按 Ticket 05 的规则进入 `human_required`，不自动 retry。已完成 AgentRun 不受影响。

### 3. Runtime Adapter 与 Codex

- `RuntimeAdapter` 是 Worker 使用的执行端口；它把 Assignment 的受限输入转换成 RuntimeResult，不让 Worker 主循环依赖某个 CLI 的参数细节。
- V1 实现 `CodexAdapter`。OpenCode 只定义接口位置和 capability 名称，不实现 OpenCode 调用，也不做 Runtime 自动回退或二次尝试。
- Codex 默认通过 PATH 发现，执行命令固定为：

  ```text
  codex exec --json --ephemeral --sandbox workspace-write --ask-for-approval never -
  ```

  Prompt 通过 stdin 写入；`exec.Cmd.Dir` 固定为 Assignment 的 Assigned Workspace。Runtime 的 JSON、自然语言或自报状态不是权威结果，exit code 和 Worker 独立采集的证据才进入 Report。
- 单次 Runtime 默认最长 30 分钟。超时由 Worker 终止 Runtime、保留可用输出并形成失败结果；不能自动启动第二个 Runtime 或自动 retry。
- Worker 为 Runtime 构造经过筛选的环境，绝不注入 Worker protocol secret、Lease token 或 Keystone DB 连接信息；运行时所需的本机 PATH、用户目录和临时目录按实现/测试 seam 明确传入。

### 4. Evidence 与 Guard

- Worker 独立采集 stdout、stderr、exit code、执行起止时间、执行前后 revision、diff 和 changed files。Runtime 自报的文件列表、退出状态或“已完成”文本不能替代这些字段。
- Report 使用强类型 Artifact 数组。V1 固定 `stdout`、`stderr`、`diff`、`changed_files` 四类；每项包含 kind、内容摘要、字节数、是否截断和可选 capture error，内容以有界字节传输。
- 默认上限为 stdout/stderr/diff 各 16 MiB、changed_files 1 MiB、单个 Report 总计 64 MiB。超限必须显式 `truncated`；无法读取、编码、摘要校验或传输的 Artifact 必须显式 capture failure，Report 返回 `unavailable` 并可用同一摘要重试，不能形成半完成终态。
- changed files 必须是去重、排序后的 Workspace 相对路径；API、Event 和 Trace 不暴露绝对宿主机路径。Artifact 由 Daemon 校验摘要/大小并原子写入 Local Artifact Store，成功后才在同一 SQLite transaction 中关联 AgentRun、Event 和状态。
- `ExecutionGuard` 通过 Worker 与 Runtime 的边界、PATH 中的 Git 拒绝 wrapper、执行前后 HEAD 检查和 Workspace 路径验证，拒绝或发现常见 `git commit`、`git push`、`git merge` 越界。它不承诺阻止恶意同用户进程、任意绝对路径工具或所有 shell 绕过。

### 5. Supervisor 与关闭

- Worker 在 Daemon readiness、Migration 和 Worker Protocol 监听完成后启动；Worker 不影响 Daemon 的 SQLite readiness，Worker 健康是独立的可观测事实。
- 默认只允许一个 Worker 子进程。可执行文件优先使用 Daemon 同目录的 `keystone-worker`，再按 PATH 发现；命令、时钟、进程和 HTTP seam 可注入测试替身。
- Worker 异常退出后使用 1、2、4 秒退避，最大 30 秒；每次重启生成新的 WorkerInstance ID 和 secret，不复用旧 Lease 或旧协议凭据。反复失败不得形成紧循环。
- Daemon 关闭先停止新的 Worker 请求和 Assignment，再请求 Worker 退出，必要时在有界宽限期后终止子进程；之后按既有顺序关闭 HTTP、SQLite、运行元数据和 InstanceLock。

## Non-scope

- Remote Worker、Worker pool、消息队列、TLS、远程认证、multi-runtime routing 和多租户调度。
- Ticket Worktree 创建、Change Scheduler、Execution DAG、实际生产 Ticket 分配和 Golden Path 的代码修改；Ticket 09 负责 Worktree/Execute/Diff 的生产链路。Ticket 06 smoke 使用预先创建的临时 Git Workspace 和内部受控 Assignment fixture，不新增公共伪造执行 API。
- Understand、Design、Plan、Ticketize、Verify、FinalVerify 的真实 Strategy，Verify approval，Gate、Policy、Risk 和 Recovery Decision。
- 由 Worker 或 Runtime 直接推进 Lifecycle、把 Ticket 标为 DONE、创建 ChangeVersion、提交 Event、批准 Verify、写 Keystone DB 或执行 Git commit/push/merge。
- 对任意本机 shell 或恶意同用户进程提供完整 OS sandbox；Prompt 中的“不要 commit”也不能当作安全控制。
- 远程分布式锁、自动重试、自动恢复、自动 Artifact orphan 回收、Dashboard Worker 控制页和完整 Trace 导出。

## Domain and ownership model

### 权威边界

```text
Client / Dashboard
        │ Control Plane Contract
        ▼
Daemon / Work authority
  Change · AgentRun · Lease · ArtifactRef · Event · Decision
        │ WorkerProtocol: Register / Heartbeat / Pull / Report
        ▼
Independent Worker
  Assignment · RuntimeAdapter · ExecutionGuard · raw capture
        ▼
Codex CLI / repository-local tools
```

- Daemon 负责授权 Assignment、保存 Lease、验证 Report、提交 AgentRun/Change/Event/Artifact 权威事实和解释并发结果。
- Worker 负责执行句柄、心跳、Workspace、Runtime 会话和原始采集；Worker 崩溃或重启不拥有恢复 Change 的权力。
- Runtime 只看到 Assigned Workspace 和受限运行输入；它不看到 Control Plane 的生命周期接口、Worker protocol secret 或 Keystone DB。
- Artifact Store 保存原始/大内容，SQLite 保存摘要、业务归属、AgentRunArtifactLink、Event 关联和状态约束。任何一方都不能通过临时目录反推业务权威。

### 术语与状态

本规格使用根 `CONTEXT.md` 中的 `WorkerInstance`、`WorkerProtocol`、`Assignment`、`Lease`、`RuntimeAdapter`、`ExecutionGuard`、`LateReport`、`AgentRun`、`AgentRunOutcome` 和 `AgentRunStatus`。新增的 `AgentRunReportLate` 是 Ticket 06 对统一 Event 账本的 M4 扩展，不建立第二个 Trace 账本。

Report 处理需要区分两个维度：

1. **AgentRun 权威：** 活动且匹配的 Lease 允许首次 Report 把 AgentRun 从 running 收敛为 completed；Pause 下仍可完成实际 AgentRun；Cancel 下也可保留真实终态和 Artifact，但不改变 Change 的取消事实。
2. **Change 推进资格：** 只有 Change 仍为 active、AgentRun 是当前检查点、Report 通过 Guard 和 Artifact 完整性验证时，Daemon 才能追加 `StageAdvanced` 或 `ChangeHumanRequired`。paused、cancelled、旧 attempt、过期/撤销 Lease 的结果不得推进。

因此，暂停期间的有效 Report 是“终止 AgentRun、冻结 Change 推进”，而 Lease 过期、Daemon 重启或旧 attempt 到达的 Report 是严格意义上的 `LateReport`，只能 Trace；二者都不能恢复或绕过 Ticket 05 的生命周期规则。

## Worker Protocol contract

### 通用约束

- 所有 Worker 路由要求 `Authorization: Bearer <process-secret>`，并严格限制在 loopback listener；非 loopback 请求或凭据不匹配直接拒绝。
- 请求和响应都是单个严格 JSON object；拒绝未知字段、缺失必需字段、重复 JSON 值和超过有界 body。协议不得回显 secret、Lease token、绝对路径或 Runtime 原始环境。
- Register、Heartbeat、Pull、Report 均携带 `worker_id` 或从鉴权上下文取得等价身份。Worker ID 必须与当前 WorkerInstance 绑定，不接受 Worker 自选的旧实例复用。
- Contract 只保存稳定的传输字段；Lease 鉴权、Report 分类、Artifact 落盘、生命周期推进属于 Daemon Application/Infrastructure，不放进 `contracts/worker`。

### Register

请求保留 Ticket 02 的 `worker_id`、`protocol_version`、`capabilities`；实现可增加受控的 `pid`/platform 摘要，但不能增加 Runtime 自报权限。Daemon 校验协议版本和 capability，记录 WorkerInstance 注册事实，并返回本次会话可用性、心跳间隔和 Lease TTL；返回值不包含 secret。

Codex capability 缺失时，Worker 不会领取需要 Codex 的 Assignment。已领取后才发现 Codex 不可用时，Worker 形成 `runtime_unavailable` 失败 Report；Daemon 不切换 OpenCode、不启动第二 Worker、不自动 retry。

### Heartbeat

默认每 5 秒发送一次，Lease 默认 30 秒过期。请求可带当前 `agent_run_id` 与不回显的 Lease 标识摘要，Daemon 仅在 Worker、Assignment 和 Lease 匹配且 Lease 仍 active 时续租。Heartbeat 不改变 AgentRun outcome、ChangeVersion 或 LifecycleStage。

Worker 长时间执行时必须持续心跳；进程退出、心跳超时或鉴权失效使 Daemon 撤销/过期活动 Lease，并按 `worker_lost` 规则收敛仍 running 的 AgentRun。不能以最后一次 Heartbeat 伪造执行完成。

### Pull

Worker 以 `worker_id` Pull。没有待执行 Assignment 时响应 `assignment: null`，不能用空对象表达“没有任务”。有任务时只返回已授权的 Assignment：

| 字段 | 语义 |
| --- | --- |
| `agent_run_id` | 不可变 AgentRun 身份 |
| `lease_token` | 不透明的一次执行授权，Worker 不解释其内容 |
| `lease_expires_at` | Lease 的 UTC 过期时间 |
| `workspace_path` | 已存在且已验证的 Assigned Workspace；只在内部传输，不能进入外部 API |
| `runtime` | 例如 `codex` 的 capability 名称 |
| `instruction` | 有界执行输入；不得包含 DB 凭据或协议 secret |
| `before_revision` | Daemon 认可的执行前 revision |
| `input_artifacts` | 有序且强类型的输入摘要/引用，不含宿主机物理路径 |

Worker 不修改 Assignment，不延长 Lease，不把 Pull 响应写入 Keystone DB。`workspace_path` 是 Daemon 已验证的本机内部路径；日志、Report、Control Plane response 和 Trace 只使用 Workspace ID/相对路径。

### Execute（Worker 内部）

Worker 收到 Assignment 后先校验本地 Workspace 存在、是预期的 Git root、可读写且没有把路径解析到 Workspace 之外，再调用匹配的 RuntimeAdapter。Execute 不新增 HTTP route，不接受 Client 参数，不把 Runtime 输出当作生命周期命令。

Worker 必须在执行前记录 HEAD，在执行期间独立采集 stdout/stderr，在执行后采集 exit code、HEAD、diff 和 changed files。Guard 发现 HEAD 被提交/切换、路径越界、禁止 Git 命令或证据不完整时，Report 必须表达失败/不可用事实，不能假装成功。

### Report

Report 的最小形态为：

| 字段 | 语义 |
| --- | --- |
| `agent_run_id` | Assignment 绑定的 AgentRun |
| `lease_token` | 本次执行的原始不透明 Lease 凭据，仅经 HTTPS/loopback body 传输，不进入日志 |
| `outcome` | Worker 只提交 `succeeded` 或 `failed`；`human_required` 由 Daemon/Coordinator 依据失败和生命周期规则产生 |
| `exit_code` | 可空的进程退出码；启动失败/超时等没有进程退出码时保持 null 并带 failure reason |
| `started_at` / `completed_at` | Worker 观察到的 UTC 时间 |
| `after_revision` | 执行后可解析的 HEAD；异常/无 HEAD 时为空并带 failure reason |
| `artifacts` | stdout、stderr、diff、changed_files 四类有界 Artifact payload |
| `capture_failures` | 无法采集的明确分类和阶段；不能用 Runtime 文本替代 |

Daemon 在接收 Report 时按以下顺序处理：鉴权 Worker；解析并校验 AgentRun/attempt/Lease；校验时间、exit code、revision、Artifact 摘要/大小/相对路径；原子写 Artifact；在一个 SQLite transaction 中写 AgentRun 终态、专用 ArtifactLink、Lease 终态和固定 Event；最后返回处理结果。任一步 Artifact 校验或写入失败都返回 `unavailable`，Worker 用同一 Report 摘要重试，不重复运行 Runtime。

推荐的有限响应 disposition 为：

| disposition | 权威效果 |
| --- | --- |
| `accepted` | 首次合法终态已写入，并且当前 active Change 可以按 Ticket 05 规则推进 |
| `accepted_fenced` | AgentRun/Artifact 已保存，但 Change 处于 paused 或 cancelled，不能推进 |
| `duplicate` | 与已提交终态的摘要相同，返回首次处理结果，不产生第二个终态/Event |
| `terminal_conflict` | 同一已完成 AgentRun 收到不同终态，返回冲突并保留原权威事实 |
| `late` | Lease/Worker/attempt 不再具备 AgentRun 权威，只追加 `AgentRunReportLate` Trace（若报告可安全保存） |

完全无效或未知的协议 secret/Worker 身份不得写入业务权威；已鉴权但 Lease token 不匹配的请求返回 `lease_invalid`，不能改变 AgentRun、Change 或 Event。对已知且已过期/撤销的有效会话，Daemon 可在完成同样的内容约束校验后保存 LateReport，不能保存为新的权威终态。

稳定错误分类至少包括 `protocol_invalid`、`worker_unauthorized`、`worker_not_registered`、`capability_unavailable`、`lease_invalid`、`lease_expired`、`assignment_not_found`、`terminal_conflict`、`artifact_invalid`、`unavailable` 和 `internal_error`。错误不暴露 SQL、进程命令行、绝对路径、secret 或内部堆栈。

## Assignment、Lease 与并发规则

### 生命周期默认值

- Worker heartbeat interval：5 秒。
- Lease TTL：30 秒，自 Pull/成功 Heartbeat 起算并由 Daemon 统一计算。
- Runtime timeout：30 分钟，独立于 Heartbeat TTL。
- 同一 Daemon 同时只运行一个 WorkerInstance；一个 AgentRun 同时只允许一个 active Lease。
- Lease token 只作为传输凭据，持久化只保存 hash、worker/run/attempt 绑定、状态、过期时间和首次终态摘要；不保存明文 token。

这些是可测试的默认值，不是 Domain 常量。测试必须注入时钟，覆盖 Pull、Heartbeat、Runtime 超时和 Report 到达顺序，不用真实 sleep 才能证明规则。

### Report 分类矩阵

| Change / AgentRun / Lease | Report | 处理 |
| --- | --- | --- |
| active / running / 匹配且未过期 | 首次合法终态 | 完成 AgentRun；exit 0 为 succeeded，其余为 failed；再按阶段规则推进或进入 human_required |
| paused / running / 匹配且未过期 | 首次合法终态 | 完成 AgentRun 和 Artifact，`accepted_fenced`；不追加 StageAdvanced，Resume 时重新评估 |
| cancelled / running / 匹配且未过期 | 实际终态 | 保存实际 AgentRun/Artifact 为审计事实，`accepted_fenced`；永远不恢复或推进 Change |
| completed / 同一终态摘要 | 重复终态 | `duplicate`，返回首次结果，不增加终态或推进 |
| completed / 不同终态 | 不一致终态 | `terminal_conflict`，原状态和 Event 不变 |
| 任意 / 旧 attempt | 结果 | `late`，只 Trace，不改变当前 attempt 或 Change |
| 任意 / expired、revoked 或旧 WorkerInstance | 结果 | `late`，只 Trace；Lease 不能复活 |
| 任意 / secret 或 token 无法鉴权 | 结果 | 拒绝，无业务权威变更；不得将不可信 payload 当作 Trace |

Pause/Cancel 与 Report 的最终顺序由 SQLite transaction commit 顺序决定。如果 Report 先提交，AgentRun 终态按上表保存，随后 Pause/Cancel 只改变 Change；如果 Pause/Cancel 先提交，仍可保存实际 AgentRun 终态但不能推进 Change。如果 Lease 失效先提交，后到 Report 只能是 LateReport。对同一 AgentRun 的终态写入必须有唯一约束和条件更新，不能依赖进程内 mutex 作为唯一保护。

### Worker 丢失与 Daemon 重启

Worker 进程异常退出、心跳超过 TTL 或协议连接被撤销时，Daemon 先标记 Lease expired/revoked，再把仍 running 的 AgentRun 完成到 `failed`，failure reason 为 `worker_lost`。active Change 进入 `human_required`；paused/cancelled Change 只保留事实，不自动恢复。Worker 重启使用新身份，旧 Worker 不能用旧 secret 或旧 Lease 回写。

Daemon 重启恢复时，SQLite 先恢复已提交的 AgentRun/Artifact/Event/Lease，再撤销所有旧 WorkerInstance 的 active Lease；不把运行元数据中一个“存活”字段当作执行成功。新的 Worker readiness 只允许新 Pull，不自动重放旧 Assignment。重试必须经过 Ticket 05 的 HumanDecision 并产生新的 AgentRun attempt。

## Artifact、Trace 与事务边界

### Report Artifact payload

四种 Report Artifact 使用统一的强类型形态：

```json
{
  "kind": "stdout",
  "content_base64": "...",
  "sha256": "64 位小写十六进制摘要",
  "size_bytes": 123,
  "truncated": false,
  "capture_error": null
}
```

`kind` 只能是 `stdout`、`stderr`、`diff` 或 `changed_files`。`sha256` 和 `size_bytes` 必须对应实际发送的字节；`truncated` 必须明确表示内容只是有界前缀，`capture_error` 必须表达读取/编码/命令失败。changed_files 的内容使用稳定的 UTF-8 结构化序列，路径按 Workspace 相对路径去重排序；禁止发送绝对路径、任意 `..` 逃逸或宿主机分隔符依赖。

默认上限如下：

| 内容 | 上限 |
| --- | ---: |
| stdout | 16 MiB |
| stderr | 16 MiB |
| diff | 16 MiB |
| changed_files | 1 MiB |
| 单个 Report 合计 | 64 MiB |

达到上限的输出可以按契约保存有界前缀，但必须标记 `truncated: true`，smoke 验收不能把截断的 diff 当成完整变更证据。changed_files 结构无法完整采集、任意 Artifact 无法编码或摘要不匹配时，Report 为 `unavailable`，Worker 应重试已采集的同一 Report，而不是重新执行 Codex。

### Daemon 落盘顺序

1. Daemon 对 Report 做 body 大小、字段、Worker/Lease/AgentRun、时间、exit code、revision 和 Artifact 内容校验。
2. 每个 Artifact 在 Local Artifact Store 的同一目录使用临时文件写入、摘要/长度复核、文件同步和原子 rename；内容已存在时只能在摘要和长度复核后复用。
3. 所有 Artifact 都可读且校验通过后，SQLite transaction 写入 Artifact 元数据、专用 `AgentRunArtifactLink`、AgentRun 终态、Lease consumed/rejected 状态、统一 Event 和需要的 ChangeVersion/ChangeStatus 更新。
4. transaction 失败时不回滚已经原子可见的内容文件；该文件只能作为按 digest 识别的 orphan，重试时复用，不能产生已提交的业务引用。
5. 读取 Artifact 时重新计算摘要；缺失或失配返回 `unavailable`，不得把磁盘损坏内容交给 Client，也不得通过改写引用修复历史。

M4 只扩展 Ticket 05 的 Work-owned 业务 Schema 和统一 Event ledger，不创建 Worker 私有数据库。建议以 WorkerInstance、Lease 关联、Report digest 和四类 ArtifactLink 表达逻辑关系；实际列名、Migration 版本和既有 Event ledger 的物理形态必须以 Ticket 05 实际已落地 Schema 为准，通过单个受控业务 Migration 演进，不能在规划阶段凭空锁死第二套表。

LateReport 若内容可安全校验，也使用同一个 Artifact Store 和统一 Event ledger 保存，但不产生 AgentRun 的第二次终态、不修改 ChangeVersion、不追加 StageAdvanced/ChangeHumanRequired。`AgentRunReportLate` Event 只引用固定摘要与 ArtifactRef，不保存自由日志 payload。

## RuntimeAdapter、Codex 与 ExecutionGuard

### Package seam

计划中的职责位置如下，实际创建时仍需阅读目标 package 的局部 `AGENTS.md`/`INDEX.md`：

```text
internal/execution/
├── domain/                 # Assignment/Lease/RuntimeResult 的业务不变量
├── runtime.go              # RuntimeAdapter 与执行输入/结果 Port
├── guard.go                # ExecutionGuard Port 与可验证边界
└── adapters/codex/         # CodexAdapter、命令构造和采集实现

internal/worker/            # Worker 进程、协议客户端、Pull/Heartbeat/Report loop
internal/daemon/            # Supervisor、Worker HTTP handler、Daemon composition
contracts/worker/           # Register/Heartbeat/Assignment/Report transport DTO
```

复杂度不足时可以把 Application/Port 留在领域根 package，但禁止让 Domain 依赖 HTTP、SQL、Codex SDK、命令行或日志框架。`internal/worker` 不得导入 SQLite Repository；`internal/daemon` 通过 Application/Port 组合实际实现。

### Codex invocation

Codex Adapter 接受已验证的 Workspace、有限 instruction 和运行选项，构造：

```text
codex exec --json --ephemeral --sandbox workspace-write --ask-for-approval never -
```

- 使用 `exec.Cmd.Dir` 固定 cwd，不把 Workspace 依赖交给 Prompt，也不把绝对路径返回给外部 Client。
- Prompt 通过 stdin 一次写入并关闭；stdout/stderr 分别由 Worker/Adapter 独立管道采集，避免把一个合并流当作两类证据。
- 默认 PATH 发现 `codex`，测试可以注入命令路径和假的 RuntimeAdapter；不能在测试中把 fake 的输出描述成真实 Codex 证据。
- 只使用 `workspace-write` 和 `ask-for-approval never`，不使用 dangerous/full-access 绕过；Runtime timeout 到期形成失败/不可用分类，不启动第二次执行。
- Runtime 环境不得携带 Worker protocol secret、Lease token、Keystone SQLite DSN 或控制面身份。需要 Codex 本机认证时只使用执行环境已配置的非协议凭据，并在 smoke 证据中记录是否可用而不记录 secret。
- `RuntimeResult` 只提供进程观察事实和原始输出；AgentRun outcome、human_required、StageAdvanced 和 ChangeVersion 由 Daemon 解释并提交。

OpenCode 只实现与 `RuntimeAdapter` 一致的 interface/capability 位置；没有 OpenCode binary、参数或回退语义进入 Ticket 06 的验收路径。

### Guard guarantees

Guard 在执行前验证 Workspace 是指定的既有 Git root，记录可解析的 before HEAD；在执行期间把拒绝 Git wrapper 放在 Runtime 的 PATH 前部，至少拒绝 `commit`、`push`、`merge`，并过滤控制面 secret；执行后重新解析 HEAD、采集 diff/changed files 并检查禁止副作用。

HEAD 改变、Workspace 逃逸、禁止命令可验证失败或证据采集不完整时，Report 不得为可推进的 succeeded。Guard 不是全能 shell sandbox：绝对路径调用、同用户恶意进程和未覆盖的工具副作用需要操作系统级能力另行解决。这个边界必须在测试和用户可见错误中保持诚实。

## Supervisor and process lifecycle

Daemon 的 Worker 监管顺序为：

1. 创建并锁定 LocalStateRoot，完成 Migration、SQLite readiness 和 Worker protocol loopback listener。
2. 生成 WorkerInstance ID 与临时 secret，建立只向子进程交付凭据的启动管道。
3. 启动一个 sibling/PATH `keystone-worker`，等待 Register；Register 成功才允许 Pull，Worker 未注册不阻塞 Daemon readiness。
4. 监管心跳、子进程退出和活动 Lease；Worker 丢失按 `worker_lost` 规则收敛，不自动 retry。
5. 异常退出按 1、2、4 秒退避，最大 30 秒重新启动；每次新进程都使用新 ID/secret，旧结果只能依据旧 Lease 规则处理。
6. 关闭时先停止新的 Pull/Report 和 Assignment 分配，再让 Worker 退出，最后关闭 HTTP、SQLite、运行元数据和 InstanceLock。

Supervisor 测试必须注入进程启动、时钟、sleep/backoff、HTTP 和 Worker 入口 seam，证明不会重复启动、泄漏 secret、紧循环重启或在 Worker 退出后误报 Daemon 未 ready。真实 Linux/WSL smoke 再验证 sibling/PATH 发现、子进程 stdio、退出和实际 Codex。

## Cross-ticket handoff

```text
Ticket 05 Change/Lifecycle/Artifact/Event/AgentRun
                         │
                         ▼
06-01 Worker Protocol、auth、Pull
                         │
                         ▼
06-02 Assignment、Lease、Report authority
                    ┌────┴────┐
                    ▼         ▼
             06-03 Supervisor 06-04 Runtime/Codex/Guard/Evidence
                    └────┬────┘
                         ▼
                 06-05 Smoke、平台验证、导航
```

- 06-01 先冻结 protocol/auth seam，但不实现生命周期规则。
- 06-02 复用 Ticket 05 的 AgentRun、Artifact、Event 和 Pause/Cancel 规则，建立 Lease 围栏与事务，不引入 Runtime。
- 06-03 与 06-04 可并行：前者负责独立进程和 Worker loop，后者负责执行端口、Codex、Guard 和 evidence capture。
- 06-05 在前四项和顶层 Ticket 05 已完成后做真实 smoke、跨平台验证和当前树导航更新。
- Ticket 09 后续负责生产 Worktree 创建、Execute/Diff 协调和实际 Ticket 分配；Ticket 06 的 smoke fixture 不得冒充 Ticket 09 的生产调度实现。

## Verification decisions

### 自动化测试

- `contracts/worker`：严格 JSON、Register/Heartbeat/Assignment/Report 字段、空 Assignment 为 `null`、未知字段、超大 body 和兼容性测试；不导入 Domain/SQLite。
- `internal/execution/domain`/Application：Lease 状态、绑定 Worker/run/attempt、一次性终态、duplicate/conflict/late、Pause/Cancel 围栏、时钟边界和 Daemon restart 收敛；不启动 HTTP 或 Codex。
- Worker protocol Handler：loopback/auth、Register、Heartbeat renew、Pull、Report 分类、错误 envelope、secret 不回显、绝对路径不外泄。
- Artifact/Report 集成：四类 Artifact、摘要/长度、截断、capture failure、原子写、重复 digest 复用、transaction 失败 orphan、统一 Event 关联和内容读校验。
- CodexAdapter：命令参数、stdin、cwd、环境过滤、stdout/stderr 分流、timeout、exit code、fake Runtime seam 和 OpenCode capability 未实现；不能只断言 Runtime JSON 文本。
- Guard：Git wrapper 对 commit/push/merge 的拒绝、before/after HEAD、Workspace root/relative path、检测越界后的 failed 结果；明确不测试为“完整 OS sandbox”。
- Supervisor：readiness 后启动、单 Worker、Register 前禁止 Pull、心跳/退出/Lease 超时、1/2/4/30 秒退避、新 ID/secret、关闭顺序、Worker 健康与 Daemon readiness 分离。

### 平台矩阵

| 平台 | 必须验证 | 证据性质 |
| --- | --- | --- |
| Linux | Daemon/Worker/真实 Codex、loopback、临时 Git、Artifact/Trace、退出和重启 | M4 主 smoke |
| WSL | 与 Linux 相同的真实 Codex 路径和文件/进程行为 | 独立本机 smoke，不能只引用 Linux |
| 原生 Windows | Worker protocol、子进程启动/关闭、路径/环境、Lease/Report、fake Runtime；若机器有 Codex 可加真实运行 | 原生平台证据；交叉编译不能替代 |
| 交叉编译 | 编译目标可生成、依赖不使用错误平台实现 | 补充证据，不是运行时证据 |

代码变更完成后按仓库规约执行 `go test ./...`、必要时 `go vet ./...`、`make build`，并对文档和实现执行差异卫生检查。Ticket 06 的 Linux/WSL real Codex smoke 必须使用临时 Git Repository、临时 LocalStateRoot 和预先授权的内部 AgentRun fixture，不修改当前 checkout、当前工作树分支或真实项目数据库。

### Smoke evidence

真实运行成功后，06-05 才能在本目录新增 `06-smoke-evidence.md`。它只保存可复核的摘要：Daemon/Worker/Codex 版本、临时 Project/Change/AgentRun/WorkerInstance 标识、Assignment/Report disposition、exit code、before/after revision、changed files 摘要、Artifact digest/size、AgentRun/Trace 查询结果和验证命令版本。原始 stdout/stderr/diff 保留在本次 Local Artifact Store，不在提交文档中复制，不写 secret、Lease token、用户目录、绝对路径或未脱敏 Prompt。

证据文档必须明确区分“已运行得到的事实”和“未运行的验收项”。在 smoke 尚未执行时，不得创建空白成功记录、虚构 digest 或把当前 `codex --version` 探针描述为端到端证据。

## Implementation ticket graph

### 06-01 Worker Protocol、鉴权与 Pull

**Blocked by:** 顶层 Ticket 05；本地实现前置为既有 `contracts/worker` DTO 和 Daemon readiness seam。

**Deliverable:** 严格 loopback Worker Protocol、进程级 secret 交付/校验、Register、Heartbeat、Pull 和稳定错误边界。

### 06-02 Assignment、Lease 与 Report Authority

**Blocked by:** 顶层 Ticket 05；本地 Ticket 01。

**Deliverable:** Assignment/Lease 权威、Report 分类、AgentRun/Artifact/Event 事务提交、duplicate/conflict/late、Pause/Cancel/重启围栏。

### 06-03 Worker Supervisor 与 Process Lifecycle

**Blocked by:** 顶层 Ticket 05；本地 Ticket 02。

**Deliverable:** 单本机 Worker 进程、readiness 后启动、Pull/Heartbeat/Report loop、崩溃退避、旧身份撤销和有序关闭。

### 06-04 Runtime Adapter、Codex、Evidence 与 Guard

**Blocked by:** 顶层 Ticket 05；本地 Ticket 02。

**Deliverable:** RuntimeAdapter/CodexAdapter、stdout/stderr/exit/diff/changed files 独立采集、Artifact payload、Workspace/HEAD/Git Guard 和 fake Runtime seam。

### 06-05 Ticket 06 Smoke、平台验证与导航

**Blocked by:** 顶层 Ticket 05；本地 Ticket 01、02、03、04。

**Deliverable:** 临时 Repository/LocalStateRoot 的真实 Codex smoke、失效/重复 Report 验证、Linux/WSL/Windows 证据、`06-smoke-evidence.md` 和基于当前树的索引更新。

本地 Ticket 01、02、03、04、05 的文档状态可以标为 `ready-for-agent`；这只表示本地规格已经足够实施，顶层 Ticket 06 的 `BLOCKED_BY: 05` 仍保留，不能由规划文档自行解除。

## Further Notes

- 本规格沿用 Ticket 02 的 Register/Heartbeat/Assignment/Report DTO 起点，明确把 Pull、Execute、鉴权、Lease、Report 幂等、Supervisor 和 Runtime 放入 Ticket 06，而不把这些目标行为倒灌到 Contract package。
- 本规格沿用 Ticket 05 的 AgentRun 一次性终态、Artifact 强类型关联、统一 Event ledger、Pause/Cancel 晚到结果和 HumanDecision 恢复规则；Worker 只是副作用通道，不是 Change 的第二权威。
- `AgentRunReportLate` 是为了使可追踪但无权威的晚到结果具有固定类型和查询语义；它不允许自由 payload，也不产生第二套事件账本。
- Worker、Execution、Daemon 等新 Go package 在后续实现票中必须先补齐目标目录的 `AGENTS.md` 和 `INDEX.md`，并按照根规约检查依赖方向；本次对齐只新增规划文档，不创建这些实现 package。
- 本文件不修正当前 README/AGENTS 中与已落地 Project Bootstrap 之间的无关描述漂移；相关实现票完成时再按实际 checkout 同步根级导航和事实。

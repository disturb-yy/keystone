# Keystone V1 Ticket 04：Repository Init 与 Project

- 状态：`ready-for-agent`（本地规格；未配置 Issue Tracker）
- 对应里程碑：M2
- 前置依赖：顶层 Ticket 03 的 CLI、Daemon、SQLite Readiness 与原生 Windows InstanceLock 验收证据

## Problem Statement

Keystone 还不能将真实 Git Repository 稳定注册为由 Control Plane 管理的 Project。本机操作者若必须自行判定 Git root、写入 ProjectManifest 或直接触碰 SQLite，重复执行 `keystone init`、切换数据根、移动 Repository、保留 clone、进程中断和文件系统错误都会造成重复权威记录、错误绑定或不安全的自动覆盖。

用户需要在 Repository 或其子目录直接运行 `keystone init`，并始终由 Daemon 协调版本化的 ProjectManifest、本机 LocalStateRoot 内的 Project 权威状态和最小可查询的 ProjectInitialized 事实。该能力只完成 Project Bootstrap，不提前创建 Change、Workspace、Runtime、Artifact 或 Lifecycle。

## Solution

`keystone init [--data-dir PATH]` 成为 M2 唯一的人机入口。CLI 只归一化当前目录、复用既有 Daemon ensure seam，并经 loopback Control Plane API 提交初始化 Command；Daemon 解析物理 Git root、协调严格的 ProjectManifest、持久化可恢复的 ProjectInitializationIntent，并以单一 SQLite transaction 写入新的 Project、首次 ProjectInitialized 与成功回执。

Repository 持有版本化的 RepositoryIdentity 表达，Daemon 持有每个 LocalStateRoot 的 Project、Event 和幂等权威状态。Manifest 文件与 SQLite 不伪装成跨资源事务：可恢复的文件故障保留 intent 供重试；身份、格式和拓扑冲突明确失败且绝不覆盖已有知识。

## User Stories

1. 作为本机操作者，我希望在正常 Git Repository 的 root 或子目录运行 `keystone init`，以便不必手工寻找仓库根或调用 HTTP。
2. 作为本机操作者，我希望 init 复用受控的 Daemon ensure seam，以便不会绕过 LocalStateRoot、InstanceLock 或 SQLite ownership。
3. 作为 Repository 维护者，我希望 ProjectManifest 只保存稳定的 ProjectID，以便本机数据根、endpoint、凭据和运行元数据不会进入版本控制。
4. 作为首次初始化的操作者，我希望得到一个权威 Project 和一次 ProjectInitialized，以便后续 Command/Query 有可靠的起点。
5. 作为重复执行 init 的操作者，我希望同一个 Repository 始终收敛到同一 Project，以便重试不制造重复事实。
6. 作为网络重试调用方，我希望同一 Idempotency-Key 与同一物理 RepositoryBinding 返回稳定结果，以便客户端可以安全重放请求。
7. 作为 API 调用方，我希望同一 Idempotency-Key 被用于不同物理 RepositoryBinding 时收到明确冲突，以便不会误把一次请求重放到错误项目。
8. 作为使用不同 `--data-dir` 的操作者，我希望合法 ProjectManifest 能在新的 LocalStateRoot bootstrap 同一 ProjectID，以便身份不依赖某个数据库文件。
9. 作为移动 Repository 的操作者，我希望在旧绑定明确不存在时重新绑定，以便目录移动不会丢失 Project 身份。
10. 作为同时保有 clone 的操作者，我希望系统拒绝双活动 RepositoryBinding，以便不会猜测两个物理根的关系。
11. 作为安全审查者，我希望损坏、未知版本、重复字段、特殊文件和不安全路径上的 ProjectManifest 被拒绝且原字节不变，以便协调不静默丢失知识。
12. 作为使用 dirty worktree 的开发者，我希望 init 不强制清理、暂存或提交工作区，以便 Bootstrap 不篡改 Repository 内容。
13. 作为从目录 symlink 进入 Repository 的操作者，我希望系统归一到物理 root，以便路径别名不产生第二个 Binding。
14. 作为 submodule 使用者，我希望独立 submodule 可以成为独立 Project，以便嵌套 Repository 保有清晰边界。
15. 作为使用 bare Repository 或 linked worktree 的操作者，我希望得到稳定的拒绝结果，以便 V1 不在不支持的拓扑上创建含糊 Project。
16. 作为 Control Plane Client，我希望通过强类型 Init、Project Query 和 Project Event Query API 观察结果，以便不直接读取 Keystone SQLite。
17. 作为审计者，我希望 Event Query 返回按发生时间和事件标识稳定排序的 ProjectInitialized 列表，以便可可靠检查 M2 的首次事实。
18. 作为测试作者，我希望从真实 CLI 到真实 Daemon、Git、Manifest 和 SQLite 的最高端到端 seam 验收行为，以便 mock 不掩盖跨资源恢复错误。

## Implementation Decisions

- Project、ProjectID、RepositoryIdentity、RepositoryBinding、ProjectManifest、ProjectInitializationIntent、ProjectInitializationReceipt 与 ProjectInitialized 的含义以项目 glossary 为准。V1 用小写 canonical UUIDv7 ProjectID 表达稳定 RepositoryIdentity；UUIDv7 不承担业务事件排序。
- Client 不直接读写 Git、ProjectManifest 或 Keystone SQLite。CLI 只处理参数、当前目录绝对化、shared ensure 与 HTTP；Handler 只处理 DTO、状态码和 ErrorEnvelope；Daemon 经 Domain Port 完成 Git、Manifest、SQLite 与恢复协调。
- `keystone init` 不自行 spawn、持锁、打开 SQLite 或执行 Migration。`status` 和 `stop` 不隐式启动 Daemon。
- Project 只允许一个活动的、物理规范化、非 bare Git 主工作树 RepositoryBinding。子目录归一到同一 Binding；独立 submodule 可初始化；bare Repository 与 linked worktree 被拒绝。调用者目录 symlink 可以归一，Manifest 目录和文件本身不得是 symlink、目录或特殊文件。
- ProjectManifest V1 是单一 YAML document 和单一 mapping，只含整型 `version: 1` 与小写 UUIDv7 `project_id`。空、截断、多文档、非 mapping、非字符串键、重复键、anchor、alias、tag、未知字段、类型错误、未知版本与非法 ProjectID 都映射为 `manifest_invalid`，不得改写既有字节。
- Manifest 缺失时才可创建。新目录与文件使用受 umask 影响的公开版本化权限；不更改已有权限、不承诺 Windows ACL、不自动执行 Git add 或 commit。并发创建使用 create-if-absent、完整写入、同步和重读核验；竞争后只允许相同合法 ProjectID 收敛。
- Reconcile 以当前物理 RepositoryBinding、合法 Manifest ProjectID 和当前 LocalStateRoot 的权威 Project 为依据。Manifest 与数据库均缺失时生成候选 ID；仅 Manifest 存在时按其 ID bootstrap 本机权威状态；仅数据库存在且绑定一致时补建 Manifest；两侧一致时只完成或重放回执；旧绑定明确不存在时才允许 rebind。活动 clone、不可验证旧 root、不同 ID 或不一致完整性返回稳定冲突或内部错误，绝不自动猜测修复。
- ProjectInitializationIntent 是 Manifest 与 SQLite 之间唯一可恢复的中间状态。流程固定为：先持久化 pending intent，再严格创建或核验 Manifest，最后在一个 SQLite transaction 中完成新的 Project、该 Project 唯一的 ProjectInitialized 与成功回执。Project 已存在时只验证其唯一 Event 并完成或重放回执；缺失、重复或不匹配 Event 是 `internal_error`，不得补写或删除。
- 可恢复的 Manifest I/O 故障保留 pending intent 并返回 `manifest_unavailable`。相同 key 恢复相同候选；不同 key 的同 root 附着同一活动候选。确定性冲突把 intent 标记为失败并释放活动 root claim；同 key 返回稳定失败，不同 key 可在外部问题被修复后重新尝试。
- 业务 Migration 由 Work 领域拥有，并在顶层 Ticket 03 已交付的 Daemon 运行链路中接入通用 Migration runner。最小持久状态包括 Project、窄 ProjectInitialized Event 与初始化回执；该 Event 写入后续 M3 也会扩展的统一内部 `t_events` 账本，但 M2 仍只公开 ProjectInitialized 的窄查询，不提供通用 Event stream。回执永久保存调用时的规范化物理 root，用于所有终态的 Idempotency-Key 比较；活动 root claim 只在 pending 时存在。
- ProjectInitialized 的“一次”限定为一个 LocalStateRoot SQLite 内同一 ProjectID 首次成为权威记录时的一次。不同数据根或机器可以以相同 ProjectID bootstrap，并在各自权威库产生首次 Event。
- `POST /v1/projects/init` 的 JSON body 只含绝对、非空的 `repository_path`，并要求非空 Idempotency-Key；成功始终为 HTTP 200，顶层 `project` 字段承载 Project DTO。`GET /v1/projects/{project_id}` 成功为 HTTP 200，使用同一顶层 `project` 字段返回权威 Project；`GET /v1/projects/{project_id}/events` 成功为 HTTP 200，顶层 `events` 字段返回 Project Event 列表，空集合编码为 `[]`，按 `occurred_at`、`event_id` 升序，不分页也不提供通用 payload。
- Project DTO 的 JSON 字段固定为 `project_id`、`repository_root`、`manifest_path`、`created_at`。Project Event DTO 的 JSON 字段固定为 `event_id`、`project_id`、`type`、`occurred_at`。时间采用 UTC RFC3339Nano；M2 只允许 ProjectInitialized。
- 错误使用既有 ErrorEnvelope，且不得暴露 Git、SQLite、操作系统路径或 errno。`invalid_request` 为 400；`repository_unsupported` 与 `manifest_invalid` 为 422；`idempotency_conflict` 与 `project_identity_conflict` 为 409；`project_not_found` 为 404；`manifest_unavailable` 与 `unavailable` 为 503；不可分类完整性失败为 500 `internal_error`。
- V1 保护正常和并发 Keystone init，不宣称抵御同机恶意进程刻意实施的 TOCTOU 替换。该威胁模型需要独立的平台专用 no-follow 或 handle-relative 设计与原生验证。

## Testing Decisions

- 已确认的最高测试 seam 是真实 `keystone init` 经 shared ensure 与 loopback HTTP 到 Daemon，再到真实 Git Repository、ProjectManifest 和 SQLite 的完整链路。端到端测试只断言用户可观察的命令结果、HTTP 响应、Manifest 状态、Project/ Event Query 与可恢复重试结果，不绑定内部函数拆分。
- Domain 单元测试只验证 ProjectID、RepositoryBinding、业务错误和首次 ProjectInitialized 不变量，不启动 Git、SQLite、HTTP 或 Daemon。
- Repository 与 Manifest 行为使用真实临时 Git Repository 验证 root/子目录、dirty worktree、symlink 入口、submodule、bare Repository、linked worktree、严格 YAML、特殊文件、并发创建和无 Git 写入副作用。
- Application 恢复测试通过 Port failure seam 覆盖 pending、Manifest 成功或失败、SQLite finalization、成功重放、稳定失败、Manifest/数据库单侧缺失、数据根切换、移动 Repository 和 clone 冲突。
- SQLite 集成测试使用真实数据库验证 Migration、Project/Event/receipt 原子性、ProjectID 与活动 root 唯一性、deduplication、rollback、receipt 持久性与 Event 完整性错误。
- Contract 测试验证三个强类型 API 的 JSON 编解码、必需字段、空 events、UTC 时间、canonical UUIDv7、排序和 ErrorEnvelope 边界，不导入 Domain 或数据库实现。
- Linux、WSL 与原生 Windows 必须运行真实 Git/Manifest/CLI 行为测试；顶层 Ticket 03 的原生 Windows InstanceLock 证据是实现的硬前置条件。交叉编译仅补充，不替代原生行为证据。
- 实现完成后执行 `go test ./...`、`go vet ./...`、`make build` 与 `git diff --check`。本次只更新规格和 Ticket 规划，只验证文档、路径和差异卫生，不把代码或平台测试报告为已执行。

## Out of Scope

- Change、Stage、Status、Lifecycle Coordinator、Ticket Graph、Workspace、Worktree、Runtime、Worker、Artifact、Decision、Governance、Dashboard 和 Repository 全量分析。
- Git add、commit、push、merge，远程 Control Plane、TLS、RBAC、团队能力、macOS 支持和任何 Client/Worker 直连 Keystone SQLite 的能力。
- 通用 Event stream、Event payload、分页、Traceability、Artifact lineage、Change Event、完整 Event Sourcing、down migration、自动数据修复和跨资源事务。
- 针对恶意本机 TOCTOU 对手的跨平台文件系统安全承诺。

## Further Notes

- `ready-for-agent` 只表示文档成熟度；它不解除顶层 Ticket 03 的 BLOCKED_BY。当前 checkout 已有 M1 的 CLI、loopback Daemon、SQLite readiness 与最小 HTTP 边界，但原生 Windows InstanceLock 行为验收仍缺失；Project、ProjectManifest、业务 Schema、Project API 与业务 Event 仍未实现。
- 当前没有本任务可用的 Issue Tracker 或 triage label 配置。遵从用户指定，本规格只保存为版本化本地工件；若需要外部 issue 发布，应先运行 `/setup-matt-pocock-skills`。
- 本规格综合已确认的 Project 初始化决策，并以真实 CLI 到 Daemon 的端到端 seam 作为后续实施验收重点。

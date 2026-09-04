# Keystone V1 Ticket 02：本地状态与边界契约实施规格

- 状态：`ready-for-agent`（本地规格；未配置 Issue Tracker）
- 对应里程碑：M0
- 前置依赖：Ticket 01 Engineering Foundation 的验收证据

## Problem Statement

Keystone 需要在 Daemon、CLI、Dashboard 和 Worker 产生实际运行行为之前，先固定本机运行状态的归属和跨进程边界。若数据目录、单实例语义、Migration 规则和传输 DTO 各自临时决定，后续实现会在不同数据根上启动多个实例、将用户状态误写入 Repository，或把 Worker 的执行事实误当成 Control Plane 的权威状态。

当前工程已有基础 Go package 与 Dashboard 骨架，但尚未实现 Daemon、Worker、HTTP API、数据库 Schema 或 Migration。本 Ticket 必须建立可复用且可验证的本地状态骨架，同时不提前实现 Ticket 03 的运行链路或 Ticket 06 的副作用执行通道。

## Solution

建立三个明确的边界：本机状态基础设施、SQLite Migration 基础设施、以及 Control Plane / Worker 的传输契约。本机状态以解析后的数据根为唯一身份，使用跨平台独占锁保护同一根目录；SQLite 只先持有 Migration 元数据；两类 Contract 只表达跨进程边界字段，不引用 Domain Entity 或授予任何权威状态写入能力。

V1 支持 Windows、Linux 与 WSL。实现以实际平台锁原语保证单实例，并以原生平台测试证明互斥行为。Daemon 的启动、监听、ready 语义和 HTTP 路由仍由后续 Ticket 实现。

## User Stories

1. 作为本机操作者，我希望未指定配置时使用稳定的用户级 Keystone 数据根，以便不同 Repository 不会各自持有一份 Control Plane 状态。
2. 作为本机操作者，我希望能通过 `--data-dir` 完整覆盖数据根，以便隔离测试、演示和多个本机实例。
3. 作为测试作者，我希望数据根可在不访问真实用户目录的情况下确定性解析，以便测试可重复且不会污染开发环境。
4. 作为 Repository 维护者，我希望用户级数据库、运行元数据和 Artifact 目录不写入 Repository Manifest，以便版本化项目知识与本机运行状态保持分离。
5. 作为 Daemon 操作者，我希望同一解析后数据根一次只能由一个实例持有，以便 SQLite Migration 和权威状态不会并发损坏。
6. 作为第二个启动者，我希望活跃实例不被 PID 文件或旧 endpoint 元数据误删或覆盖，以便已运行的 Daemon 不受损害。
7. 作为本机操作者，我希望可观察 PID、endpoint、实例标识与启动时间，以便诊断 Daemon 的本机运行位置。
8. 作为 Windows 使用者，我希望单实例语义与 Linux/WSL 一致，以便平台差异不会改变 Control Plane 的安全边界。
9. 作为单机使用者，我希望本机状态目录和运行元数据在平台支持时采用 owner-only 访问模式，以便减少本机数据暴露。
10. 作为首次运行 Keystone 的操作者，我希望新空数据库可以完成 Migration，以便系统拥有可判定的 Schema 基线。
11. 作为重复启动 Keystone 的操作者，我希望已记录的 Migration 不会再次应用，以便重复启动不会改变既有状态。
12. 作为运维者，我希望已应用 Migration 的内容发生漂移时启动明确失败，以便不在未知 Schema 上继续运行。
13. 作为领域开发者，我希望本 Ticket 不创建 Project、Change 或其他业务表，以便业务语义继续由对应 Ticket 拥有。
14. 作为 Control Plane Client 开发者，我希望 `/v1` 错误有稳定、机器可读的 envelope，以便能够可靠地区分失败类别。
15. 作为 Client 开发者，我希望健康检查有最小的强类型响应，以便后续 Daemon ready 状态可被一致消费。
16. 作为发起 Command 的 Client，我希望幂等键有统一的 HTTP 表达，以便具体 Command 后续可避免重复执行。
17. 作为 Contract 消费者，我希望成功响应保持强类型，而不是依赖无结构的通用 payload，以便编译期可发现边界变更。
18. 作为 Worker 实现者，我希望能够注册自己的传输身份与能力，以便 Daemon 后续可以识别合法的 Worker 通道。
19. 作为 Worker 实现者，我希望能发送最小心跳，以便后续运行层可以观察短期可用性。
20. 作为 Worker 实现者，我希望 Assignment 只携带受授权执行所需的传输信息，以便不会获得 Lifecycle 或 Gate 的权威性。
21. 作为 Worker 实现者，我希望 Report 只表达执行 outcome 与关联标识，以便 Daemon 后续独立决定权威状态是否推进。
22. 作为后续协议维护者，我希望 Assignment 预留不透明 lease token 与 AgentRun 标识，以便 Ticket 06 可以添加 Pull、Execute 和租约校验而不破坏已发布的模型。
23. 作为架构维护者，我希望本机状态、SQLite 和 Contract 保持窄职责模块，以便现有配置、日志和 ID 基础能力不会演变为万能工具箱。
24. 作为后续实施 Agent，我希望每个新增 Go package 都有局部规约和导航，以便依赖方向、职责和验证入口可被重新核验。
25. 作为验收者，我希望所有可观察行为有跨平台测试和根级验证证据，以便 `ready-for-agent` 不被误解为已完成实现。

## Implementation Decisions

- 本 Ticket 只交付可复用的本机状态原语、SQLite Migration runner 和边界 DTO。实际 Daemon 进程、CLI 命令、loopback HTTP、`/healthz` 路由与 ready 生命周期由 Ticket 03 负责。
- 数据根默认从操作系统用户目录解析为 `~/.keystone`。显式 `--data-dir` 可为相对路径，但必须在进程入口一次性归一为绝对清洁路径；归一后的根完整拥有 `state`、`artifacts`、`workspaces` 与 `runtime` 子目录。
- 路径解析与目录创建分离：解析保持可测试的纯行为，显式初始化才创建目录。Repository Manifest 永不保存用户级 Control Plane 状态。
- 同一解析后数据根只允许一个持锁实例。锁文件是唯一权威，生命周期内保持打开；PID 和 endpoint 元数据从不用于判断或抢占锁所有权。
- Daemon 运行元数据采用单个原子更新的记录，包含 PID、endpoint、实例标识和启动时间。正常退出清理该记录；启动或 Migration 失败时不发布 ready 元数据；遗留记录仅用于诊断。
- V1 的本机状态层必须同时支持 Windows、Linux 和 WSL。Unix 实现使用非阻塞独占 `Flock`，Windows 实现使用非阻塞独占 `LockFileEx`，通过 `golang.org/x/sys` 的平台实现隔离。操作系统在异常进程退出后释放锁；不得根据遗留元数据自动杀进程或删除活锁。
- 本机状态目录在平台支持时按 owner-only 模式创建：目录使用 `0700`，状态与运行元数据文件使用 `0600`。不对 Windows ACL 做未经测试的等价承诺。
- 本机状态、SQLite 与 Migration 资产、Control Plane Contract、Worker Contract 分别保持独立的窄模块。现有配置、日志和 UUIDv7 模块不处理数据根、持久化、锁或进程协议；新增 Go package 必须同步具有局部规约和导航。
- SQLite 使用纯 Go 的 `modernc.org/sqlite`，避免 CGO 成为 Windows、Linux 或 WSL 的构建前提。依赖的精确版本必须在实际实现时以 Go 1.27 构建与测试结果锁定。
- Migration 仅允许按版本递增。首个版本只创建 `t_schema_migrations` 元数据表；该表记录版本、名称、SHA-256 校验和与应用时间。首次执行在同一事务内创建并记录该版本，后续版本走相同的增量记录规则。
- 每个 Migration 在事务中应用。重复运行跳过已记录版本；已记录版本的名称或校验和与嵌入资产不一致时，runner 明确失败。不存在 down migration、隐式修复或业务表预创建。
- Control Plane Contract 固定 `/v1` 版本前缀。成功响应使用各自的强类型 DTO；失败统一表达为包含机器可读 `code`、安全 `message` 和可选 `request_id` 的错误 envelope；不引入通用 `data` wrapper 或无约束 `details` map。
- 健康 DTO 只表达 `ready` 状态。其 HTTP 路由、状态码和 Daemon 生命周期含义仍由 Ticket 03 实现。
- `Idempotency-Key` 是非空、不透明的 HTTP header。Contract 只固定表达；是否为必填项由后续具体 mutating Command 决定。
- Worker Contract 在本 Ticket 仅定义 Register、Heartbeat、Assignment 和 Report 的模型。Register 表达 worker 标识、协议版本与能力；Heartbeat 表达 worker 标识；Assignment 表达 AgentRun 标识、不透明 lease token、workspace 路径与 runtime；Report 表达 AgentRun 标识、lease token 与 outcome。
- Worker DTO 中的标识均是传输标识，不能复用或导出 Domain Entity。Worker Contract 不包含 Project、Change、Ticket、Gate、Decision、SQLite 或 Lifecycle 字段；Pull、Execute、鉴权、租约校验、Report 幂等语义与 Worker 监管属于 Ticket 06。

## Testing Decisions

- 最高测试 seam 是本机状态边界：给定一个数据根，调用者可观察到确定性路径解析、跨进程独占、Migration 结果和可诊断的运行元数据。测试验证外部结果，不验证具体系统调用或内部文件拆分。
- 数据根测试覆盖默认根、显式相对根的归一、显式绝对根、四个派生目录、测试临时目录隔离，以及不依赖 Repository Manifest 的行为。
- 锁测试覆盖同一数据根的第二个获取者被拒绝、不同数据根可并存、持锁者不受第二个获取者影响，以及异常关闭后锁可重新获取。Linux/WSL 与 Windows 均需在原生执行环境运行行为测试；交叉编译只作为补充编译证据。
- 运行元数据测试覆盖原子发布、正常清理、启动失败不发布 ready 元数据，以及遗留 PID/endpoint 记录不会推翻仍被持有的锁。
- Migration 测试使用真实 SQLite 数据库而非 mock：空数据库应用首个版本、重复运行不重复记录、失败事务不留下半应用状态、以及校验和漂移阻止继续启动。测试不创建业务表。
- Control Plane 与 Worker Contract 分别进行 JSON 编解码和独立 package 编译测试，覆盖必需字段、可选 `request_id` 和 Contract 不导入 Domain 的边界。测试不要求 HTTP Handler、Daemon 或 Worker 进程存在。
- 现有配置、结构化日志和 UUIDv7 基础能力的聚焦单元测试是本 Ticket 的测试风格先例：覆盖调用方可观察行为和错误分类，不锁定标准库或第三方库的内部实现。
- 代码实现完成后，按 Ticket 执行 `go test ./...`、`go vet ./...`、`make build` 与 `git diff --check`。本次只新增规格文档，验证以 Markdown 差异卫生为限，不把 Go 或 Dashboard 验证误报为本次已执行。

## Out of Scope

- Daemon 启动、停止、状态查询、endpoint 发现、loopback HTTP、`/healthz` 路由、CLI ensure 流程和 ready 状态机。
- Project、Change、Ticket、Workspace、Artifact、Event、Decision 或任何其他业务 Schema、Repository Adapter 与 Domain 规则。
- Worker 进程监管、Pull、Execute、Runtime Adapter、Codex 调用、lease 校验、Report 幂等、Artifact 采集与 Lifecycle 推进。
- Artifact 原子落盘、业务状态事务、数据清理策略、自动恢复、自动杀进程或对遗留运行元数据的强制修复。
- macOS 或其他未明确的平台支持、Remote Worker、Worker Pool、消息队列、团队/RBAC 与远程 Control Plane。
- 将用户级状态写入 Repository，或让 Client / Worker 直接访问 Keystone SQLite。

## Further Notes

- 本规格综合已确认的 grill 决策；所有描述均为 Ticket 02 的目标契约，不表示当前 checkout 已拥有对应运行行为。
- Ticket 02 的实现开始仍以 Ticket 01 的验收证据为前置条件。规格可以先行 `ready-for-agent`，但不解除 `BLOCKED_BY`。
- 当前未配置可供本任务使用的 Issue Tracker；依照用户指定，本规格作为版本化本地工件保存，不执行外部发布或标签写入。
- 实施时必须保留工作树中无关的既有变更，并仅在新 package、依赖和导航实际落地后更新相关文档的当前事实描述。

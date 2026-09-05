# Keystone V1 Ticket 03：CLI、Daemon 与 SQLite Readiness 实施规格

- 状态：`ready-for-agent`（本地规格；未配置 Issue Tracker）
- 对应里程碑：M1
- 前置依赖：Ticket 01 Engineering Foundation 与 Ticket 02 Local State and Boundary Contracts 的验收证据

## Problem Statement

Keystone 已有 LocalStateRoot、InstanceLock、RuntimeMetadata、SQLite Migration 与最小 Control Plane DTO，但这些能力尚未被真实进程串联。本机操作者不能通过 `keystone` 启动或复用 Daemon，不能可靠判断 DaemonReadiness，也不能在不直接访问 SQLite 的前提下查询 DatabasePath 与 SchemaMigrationVersion。

如果 CLI 依据 PID 强杀进程、依据 RuntimeMetadata 抢占锁、直接打开 SQLite，或在 Migration 未成功时提前报告 ready，就会破坏 DaemonInstance 对 LocalStateRoot 的唯一权威。M1 需要先交付一条可验证的本机运行链路，而不提前引入 Project、Change、Worker 或业务持久化。

## Solution

建立由 `keystone` CLI 启动独立 `keystone-daemon` 的本机 Control Plane。CLI 以归一化后的 LocalStateRoot 为实例边界，只通过 RuntimeMetadata 发现 DaemonEndpoint，并经 HTTP/JSON 确认 DaemonReadiness、DatabasePath 与 SchemaMigrationVersion。

Daemon 持有 InstanceLock、SQLite 连接和 Migration 生命周期，仅监听动态分配的 IPv4 loopback endpoint。它先以 Booting 状态提供 not-ready 健康响应，完成持锁、数据库可用性检查、Migration 与版本查询后才发布 RuntimeMetadata 并进入 DaemonReadiness。状态查询、停止和错误均保持强类型的 Control Plane 边界；CLI、Dashboard 与 Worker 都不直接访问 Keystone SQLite。

## User Stories

1. 作为本机操作者，我希望通过 `keystone daemon start` 启动用户级 Daemon，以便不必手动管理后台进程。
2. 作为本机操作者，我希望为 Daemon 命令提供 `--data-dir`，以便测试、演示和多个隔离实例使用不同 LocalStateRoot。
3. 作为本机操作者，我希望 `start` 仅在目标 DaemonInstance 已进入 DaemonReadiness 后成功返回，以便成功退出码可以作为可靠的自动化信号。
4. 作为本机操作者，我希望重复执行 `start` 复用同一健康实例，以便不会产生重复后台进程或重复 Migration。
5. 作为本机操作者，我希望 `status` 只查询既有实例而不隐式启动 Daemon，以便观察命令不产生副作用。
6. 作为本机操作者，我希望 `stop` 只请求既有实例优雅退出而不隐式启动新实例，以便停止操作可预测。
7. 作为并发启动者，我希望同一 LocalStateRoot 的并发 ensure 收敛到一个 DaemonInstance，以便不会并发写入同一 SQLite 状态。
8. 作为第二个启动者，我希望在已持锁但暂时不可达的实例面前得到明确错误，以便不会误删 RuntimeMetadata、抢锁或杀错进程。
9. 作为使用不同 LocalStateRoot 的操作者，我希望不同根能够独立运行，以便隔离测试和多实例场景互不干扰。
10. 作为诊断者，我希望 RuntimeMetadata 能帮助发现 DaemonEndpoint 与 DaemonInstanceID，以便 Client 可连接到正确的本机实例。
11. 作为诊断者，我希望知道 RuntimeMetadata 只是发现提示而不是锁权威，以便陈旧记录不会被当作进程控制依据。
12. 作为本机 Client，我希望 Daemon 只监听 loopback 地址，以便 M1 不意外暴露远程 Control Plane。
13. 作为健康检查调用方，我希望 Booting 中的 DaemonReadinessEndpoint 返回 `503` 和 `{"ready":false}`，以便连接成功不被误判为服务已就绪。
14. 作为健康检查调用方，我希望 Ready 后的 DaemonReadinessEndpoint 返回 `200` 和 `{"ready":true}`，以便能判定 Daemon 当前可服务。
15. 作为首次启动者，我希望 Daemon 在新空数据库上应用既有 Migration，以便 LocalStateRoot 有可查询的 SchemaMigrationVersion。
16. 作为重复启动者，我希望已应用的 Migration 不会被重复执行，以便重用健康实例或重启实例不会改变既有状态。
17. 作为 CLI 使用者，我希望通过 Daemon status Query 获得 DatabasePath 与 SchemaMigrationVersion，以便无需直接打开 SQLite 就能判定本机状态。
18. 作为 Control Plane Client 开发者，我希望 status 成功响应是强类型 DTO，以便字段演进可在编译期发现。
19. 作为 Control Plane Client 开发者，我希望 `/v1` 失败响应遵循既有 ErrorEnvelope，以便可以安全地区分 unavailable、无效请求和实例不匹配。
20. 作为停止请求发起者，我希望携带 DaemonInstanceID，以便陈旧 RuntimeMetadata 不会把 stop 请求送到错误实例。
21. 作为停止请求发起者，我希望同一实例已接受的重复 stop 保持幂等，以便客户端重试不会产生额外副作用。
22. 作为本机操作者，我希望不存在、不可达或实例不匹配的 stop 明确失败，以便不会把不确定状态伪装成成功。
23. 作为本机操作者，我希望 stop 不依据 PID 强杀进程，以便 PID 复用或陈旧诊断数据不会伤害无关进程。
24. 作为本机操作者，我希望 DaemonReadiness 表达当前可服务性，以便运行中的数据库故障会反映为 not ready，而不是永久保留启动时的成功状态。
25. 作为本机操作者，我希望在数据库不可服务时仍可请求既有实例 stop，以便能以受控方式恢复本机环境。
26. 作为本机信任域内的用户，我希望 M1 不引入未验证的认证机制，以便首条运行链路保持克制；我也希望该限制被明确记录，以便不会把 DaemonInstanceID 误当作安全凭据。
27. 作为 Windows 使用者，我希望 LocalStateRoot 的排他锁在原生 Windows 上经过行为验证，以便 `LockFileEx` 不是只有交叉编译证据。
28. 作为 Linux 与 WSL 使用者，我希望已有本机锁语义在真实 Daemon 生命周期中继续成立，以便平台差异不改变实例边界。
29. 作为测试作者，我希望能在临时 LocalStateRoot 中观察完整 CLI → Daemon → HTTP → SQLite 链路，以便验收不依赖真实用户目录或手工检查。
30. 作为测试作者，我希望能够控制 Daemon 的 Booting 阶段，以便验证 not-ready 健康响应而不依赖计时竞争。
31. 作为架构维护者，我希望 HTTP Handler 不直接连接 SQLite Repository，以便 Interface Adapter 不获得基础设施所有权。
32. 作为后续实施 Agent，我希望每个新增 Go package 都有局部 `AGENTS.md` 与 `INDEX.md`，以便依赖方向、职责和验证入口可重新核验。
33. 作为验收者，我希望 M1 不创建 Project、Change、Ticket、Worker 或业务表，以便后续 Ticket 的职责不被提前占用。

## Implementation Decisions

- 本规格使用 `CONTEXT.md` 的 LocalStateRoot、DaemonInstance、DaemonInstanceID、InstanceLock、RuntimeMetadata、DaemonEndpoint、DaemonReadiness 与 SchemaMigrationVersion 作为规范词汇；不将它们与 Ticket READY frontier 或 `ready-for-agent` 混用。
- 本规格遵循 ADR 0001：`keystone` 启动独立的 `keystone-daemon`，不以 CLI 自重启的隐藏服务模式替代独立 DaemonInstance。CLI 以同目录可执行文件或进程环境中的 `keystone-daemon` 发现目标；无法确定可执行文件时明确失败。
- CLI 命令树采用 Cobra，使 `keystone daemon start|stop|status` 成为首个真实命令组。`--data-dir` 是该命令组的共同输入，父 CLI 必须仅调用一次既有路径解析能力，并将归一化后的 LocalStateRoot 传给 Daemon。
- `start` 是唯一可 ensure DaemonInstance 的命令。它在有界等待内验证目标实例的 health 与 status，只有两者确认同一 DaemonInstanceID 且 DaemonReadiness 为 true 时才成功；健康既有实例被复用。
- `status` 与 `stop` 从不隐式启动 Daemon。它们可读取 RuntimeMetadata 以发现 DaemonEndpoint，但不得以 RuntimeMetadata、PID 或 endpoint 判断 InstanceLock 所有权。
- 同一 LocalStateRoot 的 InstanceLock 是唯一排他性权威。并发 `start` 在失去锁竞争后只可有界等待并尝试发现健康实例；若锁仍被持有而实例不可验证，必须返回明确错误，不能删除或覆盖 RuntimeMetadata、抢锁或按 PID 杀进程。
- Daemon 生成 DaemonInstanceID 用于关联 RuntimeMetadata、status 和 stop。该标识不承担认证、授权或秘密材料职责。
- Daemon 启动顺序固定为：初始化 LocalStateRoot，获取 InstanceLock，绑定 `127.0.0.1:0` 并进入 Booting，打开并探测 SQLite，应用既有 Migration，查询已提交的最高 SchemaMigrationVersion，原子发布 RuntimeMetadata，最后进入 DaemonReadiness。
- Booting 阶段的 `GET /healthz` 返回既有 HealthResponse 的 `{"ready":false}` 和 HTTP `503`；Ready 阶段返回 `{"ready":true}` 和 HTTP `200`。该端点是 DaemonReadinessEndpoint，不是 Golden Path 示例服务的业务健康检查。
- Daemon 以动态 IPv4 loopback endpoint 运行。RuntimeMetadata 仅在成功进入 DaemonReadiness 后发布，并在正常退出时仅清理由相同 DaemonInstanceID 写入的记录。
- SQLite 连接、`PingContext`、Migration 应用和 SchemaMigrationVersion 查询均由 Daemon 生命周期拥有。Migration 沿用既有纯 Go SQLite driver、递增、事务、checksum 漂移失败和不预建业务表的规则。
- `GET /v1/daemon/status` 是 DatabasePath、SchemaMigrationVersion、DaemonReadiness 与 DaemonInstanceID 的唯一权威 Query。成功响应使用新的强类型 DTO；CLI 不为 status 打开 SQLite。
- `POST /v1/daemon/stop` 接受必需的 DaemonInstanceID，并在匹配当前实例时返回已接受的停止结果后异步执行优雅关停。相同实例已接受的重复请求保持幂等；本 Command 不要求 `Idempotency-Key`。
- `/v1` 的无效请求、实例不匹配和服务不可用都使用既有 ErrorEnvelope，不泄露内部错误、锁细节或 SQLite 错误文本。DaemonReadinessEndpoint 的 not-ready 响应继续使用 HealthResponse，而不是 ErrorEnvelope。
- DaemonReadiness 是当前可服务性。Ready 后若数据库探测或权威 status 查询失败，DaemonReadinessEndpoint 必须降为 not ready，status 返回服务不可用；已存在实例的 stop 通道仍可工作。
- 优雅关停先保证已接受的 HTTP 响应可返回，再停止服务、关闭数据库、按 DaemonInstanceID 清理 RuntimeMetadata 并释放 InstanceLock。关闭失败必须向上报告或以可诊断方式处理，不能吞掉错误。
- M1 的信任模型是本机 loopback 信任域；DaemonInstanceID 只避免陈旧目标，不能被描述为跨用户认证。跨用户隔离、control credential、远程访问和 TLS 不在本规格范围内。
- HTTP Handler 只负责协议解析、DTO 转换和响应；Daemon lifecycle/Application seam 负责状态编排；SQLite 与 LocalStateRoot 操作保留在具体基础设施/运行组合层。不得让 Handler 或 CLI 直接承担 SQLite Repository 行为。
- 为 status、stop、RuntimeMetadata 读取与 Daemon lifecycle 新增的 Go package 必须在实际创建时同步具有局部 `AGENTS.md` 与 `INDEX.md`；根 README 与 INDEX 只在文件和运行入口实际落地后陈述对应事实。
- 本 Ticket 不新增业务 Domain Entity、业务 Repository、业务表或 Project Manifest；`t_schema_migrations` 仍是唯一可由 M1 使用的 SQLite 表。

## Testing Decisions

- 最高验收 seam 是真实进程的 CLI lifecycle：在临时 LocalStateRoot 中启动实际 `keystone` 与 `keystone-daemon`，从外部观察 start、status、stop、HTTP 响应、RuntimeMetadata 生命周期和 SQLite 文件，而不从 CLI 测试中直接查询 SQLite。
- CLI lifecycle 测试必须证明首次 start 在 DaemonReadiness 前不成功、Ready 后成功返回、status 可经 HTTP 返回 DatabasePath 和 SchemaMigrationVersion、stop 使同一实例优雅退出，并且 status/stop 不会启动新实例。
- 需要一个受控的 Daemon readiness integration seam 来稳定观察 Booting：测试可阻塞数据库就绪前的启动阶段并向已绑定 endpoint 发送 `GET /healthz`，验证 `503 {"ready":false}`；解除阻塞后验证 `200 {"ready":true}`。该控制只能存在于测试注入边界，不能成为公开运行协议。
- 同根并发 lifecycle 测试覆盖：第一个 DaemonInstance 保持健康时第二个 start 复用或明确拒绝，但不得破坏第一个实例、其 RuntimeMetadata 或 InstanceLock；不同 LocalStateRoot 可并行运行。
- stale、损坏、缺失或不可达 RuntimeMetadata 的测试覆盖明确错误、无 PID 强杀、无元数据自动删除和无锁抢占。测试只观察外部状态和错误分类，不绑定文件读取的内部拆分。
- status/stop 的 HTTP 测试覆盖强类型 JSON、必需 DaemonInstanceID、实例不匹配、重复 stop、服务不可用与既有 ErrorEnvelope。HealthResponse 的 JSON 行为沿用现有 Contract 测试风格。
- SQLite lifecycle 测试使用真实数据库，复用既有 Migration runner 的空库、重复应用、漂移、失败回滚与最高版本查询先例；不得用 mock 代替 Migration 可观察结果。
- Ready 后数据库不可服务的测试验证 health 降级、status 服务不可用以及 stop 仍可完成，不验证具体数据库驱动调用次数或内部重试实现。
- 既有 `localstate` 测试是路径解析、InstanceLock、权限和 RuntimeMetadata 原子更新的先例；既有 `migration` 测试是事务 Migration 与漂移保护的先例；既有 Control Plane Contract 测试是 JSON DTO 与 ErrorEnvelope 的先例。
- 原生 Windows 是 Ticket 03 代码实施前的硬验收门槛：必须在 Windows runner 上运行 `go test ./internal/infrastructure/localstate -count=1`，记录 Windows 版本、`go version`、`go env GOOS GOARCH GOHOSTOS`、命令输出和退出状态。交叉编译不能替代此证据。
- 实现完成后运行 `go test ./...`、`go vet ./...`、`make build` 与 `git diff --check`；必要时补充 Windows 无 CGO 构建和原生 Windows 行为验证。文档阶段只验证 Markdown 路径、内容和差异卫生，不将这些代码验证误报为已执行。

## Out of Scope

- Project 注册、Git root 检查、Repository reconcile、`.keystone/project.yaml` 或任何 Project Domain 行为。
- Change、Ticket、Lifecycle、Artifact、Event、Decision、Governance、Traceability 或业务状态持久化。
- Worker 启动、监管、Register、Heartbeat、Pull、Execute、Report、Runtime Adapter、Codex/OpenCode 调用与 Workspace 行为。
- Dashboard API 调用、静态资源托管、SSE、业务页面或 Dashboard 对权威状态的推导。
- 业务 SQLite Schema、业务表、业务 Repository、业务 Migration、down migration、隐式修复或数据清理策略。
- Client、Dashboard 或 Worker 直接访问 Keystone SQLite，或 HTTP Handler 直接连接 SQLite Repository。
- 固定端口、非 loopback 监听、远程 Control Plane、TLS、跨用户认证、control credential、RBAC 或团队能力。
- PID 强杀、依据 RuntimeMetadata 自动恢复、删除活锁、覆盖活跃实例或自动修复陈旧运行状态。
- macOS、Remote Worker、Worker Pool、消息队列、发布安装器和进程外服务管理。
- 对 Golden Path 示例服务的 `/healthz` 业务接口做任何实现；该接口不是 DaemonReadinessEndpoint。

## Further Notes

- 本规格综合已完成的 Ticket 03 grill 决策，并以 `CONTEXT.md` 与 ADR 0001 为术语和难逆边界来源；它描述目标契约，不表示 Daemon、CLI、HTTP 或 SQLite runtime 已在当前 checkout 中实现。
- `ready-for-agent` 仅表示本地规格已准备好，不解除 `BLOCKED_BY`。Ticket 02 的原生 Windows InstanceLock 行为证据尚未在当前环境获得；在该证据补齐前不得开始 Ticket 03 代码实现。
- 当前未配置可供本任务使用的 Issue Tracker。依照用户指定，本规格保存为版本化本地工件，不执行外部发布或标签写入。
- 实施时必须保留工作树中无关的既有变更，并仅在实际创建 package、入口、Contract 或运行能力后更新当前事实文档。

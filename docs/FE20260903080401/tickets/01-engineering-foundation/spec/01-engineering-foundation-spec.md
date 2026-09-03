# Keystone V1 Ticket 01：Engineering Foundation 实施规格

- 状态：`ready-for-agent`（本地规格；未配置 Issue Tracker）
- 对应里程碑：M0
- 前置依赖：无

## Problem Statement

Keystone 当前只有模块声明、架构参考和实施 Ticket，尚没有可编译的 Go package、Dashboard 工程或统一的根级构建入口。因此 `go test ./...` 没有可执行的 package，后续 Ticket 也缺少能承载跨领域基础能力、可重复构建 Dashboard 与验证入口的最小工程面。

Ticket 01 还曾将 Migration 调用入口列入 M0，而后续 Ticket 已定义数据目录、增量 Migration 机制、版本表与重复执行的验收责任。若 M0 提前实现 Migration，就会重叠并削弱后续 Ticket 的独立验收边界。

## Solution

建立一个真实但克制的 M0 工程基础：以三个窄的跨领域基础能力提供日志配置、结构化日志和随机标识符；建立可生产构建但不包含业务页面的 Dashboard；以根级 Make 目标作为工程验证入口；并让项目文档只陈述代码树中已存在的事实。

Migration 的实现、数据目录与用户级运行状态完整留在 Ticket 02。M0 不创建只为占位的业务层、进程入口、协议、数据库对象或 Migration package。

## User Stories

1. 作为 Keystone 开发者，我希望仓库至少有可测试的 Go package，以便 `go test ./...` 成为有效的工程检查。
2. 作为后续 Ticket 的实现者，我希望跨领域配置能力具有明确且小的职责，以便不会抢占用户级数据目录或 CLI 配置的设计。
3. 作为本机操作者，我希望能通过 `KEYSTONE_LOG_LEVEL` 配置日志等级，以便在不引入配置文件的情况下控制基础日志输出。
4. 作为本机操作者，我希望非法日志等级得到明确错误，以便配置错误不会被静默掩盖。
5. 作为应用开发者，我希望获得 JSON 格式的结构化日志能力，以便后续 Daemon、Worker 和 Adapter 能在相同日志语义上扩展。
6. 作为调用跨领域基础能力的开发者，我希望 logger 不修改进程全局默认 logger，以便不同运行入口可显式控制自己的输出目的地与等级。
7. 作为领域和应用开发者，我希望获得随机且标准格式的标识符，以便后续实体、事件和边界 DTO 可以安全使用唯一 ID。
8. 作为架构维护者，我希望基础能力保持窄 package 边界，以便 `infrastructure` 不演变为无职责的公共工具箱。
9. 作为 Dashboard 开发者，我希望有 React、TypeScript 和 Vite 的可构建骨架，以便后续观察界面可在既定工具链上演进。
10. 作为产品使用者，我希望 M0 Dashboard 不伪装成已具备 Project、Change 或 Trace 行为的产品，以便界面不超前承诺未实现的 Control Plane 能力。
11. 作为贡献者，我希望根级命令统一暴露测试、构建、静态检查和 Dashboard 构建，以便在不记忆多套命令的情况下验证改动。
12. 作为持续集成维护者，我希望 Dashboard 依赖由锁文件确定，以便相同依赖树可以被重复安装和构建。
13. 作为 Agent，我希望每个新增 Go package 有局部规则和导航，以便能够在修改前确认职责、依赖方向与测试入口。
14. 作为项目读者，我希望 README、项目地图和工程规约中的事实段与真实文件树一致，以便不把架构设计输入误认为已实现行为。
15. 作为 Ticket 02 的实现者，我希望 Migration 机制、版本表、数据目录和 SQLite 责任不被 M0 预占，以便其验收仍能覆盖一条完整而独立的运行边界。

## Implementation Decisions

- M0 只创建实际有行为的跨领域基础能力；不预建领域、Application、Port 或 Adapter 空目录。
- 基础设施父边界仅说明跨领域基础能力的 ownership 和依赖约束，不作为万能 Go package。配置、日志和 ID 分为各自独立的窄 package，并各自附带局部规则和导航。
- 配置能力唯一处理 `KEYSTONE_LOG_LEVEL`：未设置时解析为 `info`，合法值遵循标准库日志等级语义，非法值返回错误。它不处理配置文件、运行时重载、`--data-dir`、默认用户目录或任何落盘状态。
- 日志能力使用 Go 标准库 `log/slog` 的 JSON handler。调用者显式提供输出目标和已解析的等级；该能力不改变全局默认 logger，也不自行初始化进程级配置。
- 标识符能力只生成使用 `crypto/rand` 的小写 UUIDv4 字符串。M0 不引入排序语义、持久化策略或领域 ID 类型。
- Go 基础能力不引入第三方依赖；每个能力必须有其可观察行为的单元测试。
- Dashboard 使用 React、TypeScript、Vite、npm 与锁文件。它只渲染静态 Keystone 占位内容，不包含路由、Control Plane 调用、Project、Change、Trace、SSE 或权威状态推导。
- 根 Make 契约固定为：`test` 执行 Go 测试，`build` 编译 Go package，`lint` 执行 Go vet 与 Dashboard lint，`dashboard-build` 以锁文件安装依赖后生成生产构建。Dashboard 相关 Make 目标自行保证依赖已按锁文件安装。
- Migration 调用、SQLite driver、版本表、Migration 文件、数据目录、单实例锁和 Daemon ready 语义均由 Ticket 02 拥有。M0 不定义 Migration 抽象或占位目录。
- 根级工程规约只更新其当前事实段，README 与项目地图只更新实际创建的文件、包和可运行命令；既有架构规约和静态架构基线不因本 Ticket 改写。
- Ticket 01 与 M0 计划会删除对 Migration 基础的实现承诺，使计划边界与 Ticket 02 的唯一 ownership 一致。

## Testing Decisions

- 最高验证 seam 是根 Make 契约：测试、构建、静态检查和 Dashboard 生产构建必须能从根目录独立触发。测试关注命令的可观察退出状态和生成的构建结果，不验证 Makefile 的内部编排。
- 配置测试覆盖未设置日志等级、合法日志等级与非法日志等级返回错误；不测试环境读取的内部实现细节。
- 日志测试覆盖 JSON 输出可被解析、包含调用方提供的结构化字段，并尊重传入等级；不测试 `slog` 内部 handler 实现。
- ID 测试覆盖生成值符合小写 UUIDv4 格式、版本与 variant 位正确，并验证多次调用产生不同值；不依赖随机值的具体内容。
- Dashboard 测试以生产构建成功为主要验收；静态检查验证 TypeScript 与 lint 规则可运行。M0 不因不存在业务行为而引入业务页面测试。
- 文档验证使用 `git diff --check`，并复核根级文档中关于当前文件树、包与构建命令的描述。该检查只证明文档与 diff 卫生，不替代代码行为测试。
- 当前仓库没有可复用的 Go 或 Dashboard 测试先例；本 Ticket 建立的测试应成为后续跨领域基础能力测试的最小先例。

## Out of Scope

- Daemon、CLI、HTTP API、Control Plane Contract、Worker Protocol、Worker 进程监管及 Runtime Adapter。
- SQLite Schema、SQLite driver、Migration 文件、Migration runner、版本表、数据目录、单实例锁与 endpoint 元数据。
- Project、Change、Ticket、Execution、Governance、Traceability 的 Domain 语义、持久化或生命周期推进。
- Dashboard 路由、API 客户端、SSE、Project/Change/Trace 页面、认证与任何权威状态写入。
- 配置文件、用户级状态目录、`--data-dir`、动态配置重载，以及第三方 Go 基础设施依赖。

## Further Notes

- 本规格记录已完成的 grill 对齐结论，并作为 Ticket 01 的本地 `ready-for-agent` 实施契约。
- 根级文档当前含有与真实文件树不一致的 `.gitkeep` 和空目录描述；实施时只按可验证的实际结果修订事实，不将目标架构描述成已运行的服务。
- 现有未提交的文档改动属于既有工作树状态。实施应保留这些改动，只修改本 Ticket 直接需要对齐的内容。

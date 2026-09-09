# 11-05：Ticket 11 集成验收与导航

> 状态：代码与导航已实现；自动构建/静态检查和局部 HTTP 测试已执行，真实浏览器与上游 M7/M8 acceptance 未宣称通过。父 Ticket：[11 Dashboard Observation](../../11-dashboard-observation.md)；规格：[11-dashboard-observation-spec.md](../spec/11-dashboard-observation-spec.md)。
>
> 依赖：11-01、11-02、11-03、11-04，以及 Ticket 10 的真实 acceptance evidence。该子票负责收口，不解除任何上游 Ticket 的阻塞。

## 目标

把 Ticket 11 的前四个纵向切片固定为可重复的构建、运行和浏览器验收，并同步维护版本化 Ticket 导航与当前事实索引，明确区分 planning、hygiene、build 和 runtime evidence。

## 实现范围

- 在真实 Daemon 上运行生产 Dashboard 构建，不以 Vite dev server、静态 mock JSON 或直接 SQLite 作为验收替代品。
- 覆盖 Projects → Project Detail → Change Detail → Trace 的观察路径，以及 Needs Human → evidence → retry/cancel Decision 的人工路径。
- 验证 Query 首次加载、空集合、section `not_yet_available`、服务错误、SSE 断线/重连、浏览器刷新、stale snapshot、409 version conflict、Command/Decision 成功和 SPA 深链接 fallback。
- 执行并记录：
  - `go test ./...`
  - `go vet ./...`
  - `make build`
  - `make dashboard-build`
  - `git diff --check`
- 更新 Ticket 11 顶层链接、版本化 Ticket `INDEX.md`、根 `INDEX.md` 的 Dashboard 事实导航和相关 package/index 文档；只描述当前已验证事实，不把规划文档写成已实现。
- 保留 `dashboard/DESIGN.md` 作为样式选择的单一基线；若验收发现视觉偏差，先更新 DESIGN 决策再改页面，避免局部 token 漂移。

## 浏览器验收证据

验收记录至少包含：

1. Daemon 实例、生产构建版本和访问 URL。
2. Query endpoint 的响应摘要、`schema_version`、`observed_at`、section availability 和分页边界。
3. SSE 连接建立、`refresh` payload、断线和重新 Query 的前后观察。
4. Change Detail 的 LifecycleStage/ChangeStatus、Ticket order/`BLOCKED_BY`、Current Run、Trace EventSequence、Artifact preview 和 Health 观察。
5. Human Required Change 的 evidence view、合法 Decision、CommandReceipt/权威返回、重查结果和 409 stale 行为。
6. 四个深链接直接刷新、API 路径错误和静态 fallback 的结果。

证据中不得保存 secret、token、完整日志、Prompt、绝对路径或凭据。独立评审若未明确给出通过结论，必须标记为未验证，不能以本地 lint/build 代替。

## 验收条件

- 自动验证命令全部通过；若环境阻塞，记录具体环境限制及未完成的 runtime 证据，不修改代码绕过。
- 真实浏览器验收覆盖 Ticket 11 的全部页面和关键失败/恢复状态。
- `dashboard/DESIGN.md`、Ticket 规格、子票和索引链接均指向实际文件；无错误目录副本。
- `git diff --check` 无新增 whitespace error；LF/CRLF 配置提示若出现，单独记录为工作树换行提示，不误报为功能失败。
- 交付报告区分 changed files、artifact/version、validation commands/results 和 residual risks。

## 不包含

- Ticket 12 Golden Path E2E 编排、真实 Codex 全链路或远程发布。
- 上游 Ticket 08/09/10 的行为补齐或验收证据伪造。
- 因浏览器验收方便而新增 debug API、SQLite 写入口、自动恢复或隐藏命令。

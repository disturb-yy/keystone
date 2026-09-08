# 12-05：Candidate 服务与 Production Dashboard 观察

> 状态：`ready-for-agent`（仅表示规划成熟度，尚未实现）。父 Ticket：[12 Golden Path E2E Evidence](../../12-golden-path-e2e.md)；规格：[12-golden-path-e2e-spec.md](../spec/12-golden-path-e2e-spec.md)。
>
> 顶层硬阻塞：Ticket 11 的真实 acceptance evidence 未形成前不得开始实现；本子票还依赖 12-04 已产生的 CandidateRevision。

**What to build：** 让已抵达 CandidateRevision 的 GoldenPathRun 从独立 demo archive 验证全部 HTTP 行为，并在 Daemon 托管的 production Dashboard 中以真实浏览器观察四个深链接、Query、SSE 断线重连和刷新后的状态重建。

**Blocked by：** Ticket 11 的真实 production Dashboard acceptance evidence；12-04 公开 Execute 至 Candidate 驱动。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 分别从 BaseRevision 与 CandidateRevision 的 demo Repository archive 构建独立服务副本；禁止读取 Keystone Workspace 或在副本中修复源码。
- 以 loopback 临时地址和固定 startup 协议启动服务；Base 仅检查既有 `GET /`，Candidate 检查六条 `DemoAcceptanceCriteria`、HTTP 安全摘要、日志和退出状态。
- 服务结束时先发送 SIGTERM，在受限等待后只终止 Runner 自己的服务子进程树，保留安全材料供复核。
- 使用预先准备、lockfile 对应的 headless Playwright/Chromium 访问 Daemon 托管的 production Dashboard。
- 覆盖 Projects、Project Detail、Change Detail、Needs Human 的直接深链接；用真实 EventSource network offline-to-online 制造断线和恢复，观察断线、新 Query、重连、新 Query 与完整刷新后的 Query 重建。
- 将浏览器引擎、版本、headless、production build digest、脱敏 origin、页面/Query/SSE 安全摘要及 Daemon 一致性材料形成待审阅输入，但不自行发布成功 Evidence。

## Authority and Safety Boundaries

- Demo HTTP 成功只证明 archive 中的 Base/Candidate 服务行为，不替代 Daemon 的 CandidateRevision、Verifier PASS 或 `integrate_ready` 权威事实。
- Dashboard 只通过公开 Query 重建状态；SSE 仅是 RefreshHint，不携带或推导业务状态。Vite dev server、mock payload、前端状态伪造、浏览器缓存或静态截图不能通过验收。
- 浏览器观察不得暴露绝对路径、完整 Prompt、环境、token、未投影 API 内容或本机敏感信息。

## Acceptance Criteria

- [ ] Base 与 Candidate 均从独立 archive 构建、以约定 startup 协议监听；Base 保持原 `GET /` 行为，Candidate 同时满足六条固定 DemoAcceptanceCriteria。
- [ ] 服务检查不读取/修复 Keystone Workspace，HTTP、启动、退出和日志均形成安全摘要与 digest，并在失败时保留现场。
- [ ] 生产 Dashboard 的四个深链接可通过真实 Daemon 托管访问，且页面通过公开 Query 显示与 Daemon 权威响应一致的状态。
- [ ] Playwright 观察到真实 EventSource 断线、恢复、断线后新 Query 与完整页面刷新后新 Query；缺失 browser、driver、production build、版本匹配或任一观察均使平台 Run 失败。
- [ ] 本子票只形成待审阅的浏览器/服务材料，不创建或追加 GoldenPathEvidence。

## Out of Scope

- Dashboard Query、SSE、路由、静态托管或 UI 业务能力的实现；这些属于 Ticket 11。
- 独立审阅、成功 Evidence 发布、跨平台结论、自动清理或对 Candidate/Workspace 的人工修复。

## Verification

- 在真实 CandidateRevision、Daemon production build 和固定 headless browser 中执行 Base/Candidate HTTP 与 Dashboard 观察。
- 覆盖服务启动超时、HTTP 语义失败、SSE 无重连、刷新后无 Query、敏感投影失败等终态，并保留可审阅安全材料。


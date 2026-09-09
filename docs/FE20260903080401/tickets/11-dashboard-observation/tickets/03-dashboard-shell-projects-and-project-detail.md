# 11-03：TDesign Dashboard Shell、Projects 与 Project Detail

> 状态：代码已实现；Dashboard production build 与 lint 已通过。父 Ticket：[11 Dashboard Observation](../../11-dashboard-observation.md)；规格：[11-dashboard-observation-spec.md](../spec/11-dashboard-observation-spec.md)。
>
> 依赖边界：11-01 Query Contract、11-02 Daemon 静态托管/SSE 已落地；M7/M8 真实 acceptance 按用户授权跳过，不在本子票中补做。

## 目标

用 TDesign React 建立可访问、可响应的 Dashboard 应用壳，以及 Projects、Project Detail 两个只读观察页面。页面必须使用 Daemon Query 数据，不携带第二套业务状态。

## 实现范围

- 通过 `tdesign-mcp-server` 的组件列表、文档、DOM 结构和图标检索确认使用方式，再锁定 `tdesign-react`、`tdesign-icons-react` 版本并更新 `dashboard/package-lock.json`。
- 建立 `Layout`、Header、Aside、Content、Menu、Breadcrumb 和路由壳；固定路由为 `/` 与 `/projects/:project_id`，为 Change Detail/Needs Human 保留导航入口。
- 使用 `react-router-dom` 实现 `/`、`/projects/:project_id`、`/changes/:change_id`、`/needs-human` 的客户端路由，并配合 11-02 的生产 SPA fallback。
- Projects 页面使用 TDesign Table 展示 Project identity、repository root 的本机确认字段、Change 数量/摘要和可进入 Project Detail 的链接；没有 Project 时显示 Empty。
- Project Detail 展示 Project 摘要和该 Project 的 Change Table，使用 `limit/cursor` 服务端分页，不在 Client 排序或拼接其他 Project 的 Change。
- 使用 typed native `fetch` client 和页面级 hooks；每次 Query 使用 `AbortController`/request sequence，旧响应不得覆盖新响应。
- 覆盖 loading、available empty、error、stale snapshot、SSE disconnected/reconnecting；失败时保留最近成功数据但显式标记 stale。
- 所有色彩、字号、间距、状态 Tag、响应式断点和可访问性遵循 [`dashboard/DESIGN.md`](../../../../../dashboard/DESIGN.md)，不在页面中重新发明 token。

## 组件与交互边界

- 组件优先使用 TDesign `Layout`、`Menu`、`Breadcrumb`、`Card`、`Table`、`Tag`、`Alert`、`Skeleton`、`Empty`、`Result`、`Button`、`Space` 和 `Typography`。
- Projects 与 Project Detail 只读；本子票不新增 Project/Change mutation。
- ID、版本、时间等技术字段按 DESIGN.md 的 mono/UTC 规则显示；不显示 Workspace、数据库或凭据相关字段。
- 连接状态由 shell 显示为提示，不改变页面数据；刷新提示只触发 Query。

## 验收条件

- `npm run lint`、`npm run build` 和 `make dashboard-build` 通过，且依赖锁文件与 package manifest 一致。
- 真实 Daemon 生产构建中能从 Projects 进入 Project Detail；页面数据来自 `/v1` Query。
- 空 Project、Query error、首屏 loading、已有 snapshot 的 stale、SSE 断线和恢复均有可见状态。
- 键盘可完成侧边导航、Breadcrumb、表格行链接；颜色不是唯一状态编码；窄屏不出现不可操作的水平溢出。
- 客户端没有 SQLite import、Worker Protocol 调用、Lifecycle 推导或本地持久化业务真相。

## 不包含

- Change Detail、Needs Human 详情与 Command/Decision 提交。
- Ticket Graph、Execution、Artifact/Trace 组件的业务展示。
- 通过客户端计算 Change 数量、状态、健康、frontier 或 Human Required。

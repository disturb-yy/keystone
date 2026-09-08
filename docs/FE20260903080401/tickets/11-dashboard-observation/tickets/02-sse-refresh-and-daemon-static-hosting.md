# 11-02：SSE RefreshHint 与 Daemon 静态托管

> 状态：`ready-for-agent`（规划已对齐，尚未实现）。父 Ticket：[11 Dashboard Observation](../../11-dashboard-observation.md)；规格：[11-dashboard-observation-spec.md](../spec/11-dashboard-observation-spec.md)。
>
> 依赖：Ticket 10 真实 acceptance evidence、Ticket 11-01 Query Contract。11-02 不得以静态文件或 mock event 代替上游 Query 行为。

## 目标

让 Daemon 在同源下托管 Dashboard 生产构建，并提供一个只发送刷新提示的 `/v1/updates` SSE 流。SSE 丢失、重连和浏览器刷新都必须回到 Query，而不是依赖事件完整性或在浏览器中保存业务事件账本。

## 实现范围

- 将生产构建资产纳入 Daemon 的静态资源托管边界；具体资源嵌入/发布方式以当前 Daemon 装配和构建链验证为准，不能让 Dashboard dev server 成为生产依赖。
- 为非 API 的 Dashboard 页面路径提供 SPA fallback，至少覆盖 `/`、`/projects/:project_id`、`/changes/:change_id` 和 `/needs-human` 的深链接。
- API 路由优先匹配；`/v1/*`、`/healthz`、Worker Protocol 和未知 API 不能被静默返回 `index.html`。
- 实现 `GET /v1/updates`：单一 Dashboard EventSource，可选 `project_id`、`change_id` 过滤。
- 仅发送受控 `event: refresh`，数据字段为 `resource_type` 及可选 `project_id`、`change_id`；不得发送状态、Artifact、命令、Decision、cursor、replay token 或内部错误详情。
- 连接保持、心跳、断开清理、客户端取消和 Daemon 关闭必须有界；事件发布只作为现有权威写入后的唤醒提示，不改变任何业务状态。
- 为事件过滤、字段安全、连接关闭、断线重连后的重新查询和静态 fallback 编写 HTTP 测试。

## 事件契约

```text
event: refresh
data: {"resource_type":"change","project_id":"...","change_id":"..."}
```

`resource_type` 使用固定枚举，关联 ID 必须来自 Daemon 已确认的事实。流不提供 replay、顺序保证或可靠投递；Dashboard 收到任何提示后按关联范围重新 Query，无法识别或过期的提示不直接更新 UI。

## 验收条件

- 生产构建资产可由真实 Daemon 通过同源 URL 访问，四个深链接在刷新后仍返回 SPA 文档。
- `/v1/updates` 的响应头、SSE framing、过滤行为和断开清理稳定；事件 payload 只包含 RefreshHint 字段。
- 断开 EventSource 后重新连接不会恢复旧业务状态；浏览器必须重新 Query 并得到与 Daemon 一致的快照。
- API 404、Health、Worker Protocol 和静态页面路径不会互相 fallback。
- `go test ./...`、`make build`、`make dashboard-build` 和适用的 HTTP 测试通过。

## 不包含

- 浏览器状态管理、TDesign 页面、Command/Decision Drawer。
- SSE 事件回放、离线同步、可靠消息队列或把 SSE 作为 Query 替代。
- 任何通过事件直接推进 Lifecycle、Ticket、Gate、Verification 或 Human Required 的逻辑。

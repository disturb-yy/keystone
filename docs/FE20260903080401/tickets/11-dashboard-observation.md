# 11 — Dashboard Observation

- 里程碑：M9
- `BLOCKED_BY`：10
- 交付类型：观察型 Dashboard 纵切

## 目标

完成由 Daemon 托管的 Dashboard，使用户可以从权威 Query API 观察 V1 Golden Path，并仅通过显式 Command/Decision 改变系统。

## 范围

- 实现并托管 Dashboard 生产构建，提供 Projects、Project Detail、Change Detail、Needs Human 四个页面。
- Change Detail 展示 Lifecycle、Ticket 与 `BLOCKED_BY`、Current Run、Artifact、Trace、Daemon/Worker Health。
- Dashboard 先查询 `/v1` 权威快照；SSE 只发送刷新提示，断线或重连后重新查询。
- Pause、Resume、Cancel、Retry 和 Human Decision 通过显式 Command endpoint 提交，并展示 Daemon 返回的权威结果。
- 为页面的 loading、empty、error、断线重连与 Human Required 状态提供可见反馈。

## 不包含

- 浏览器直连 SQLite、客户端推导 Lifecycle Truth 或将 SSE 当作唯一状态源。
- 团队、RBAC、远程访问、Plugin Marketplace 或完整运维控制台。
- 在 Dashboard 中直接修改 Git、Workspace 或 Runtime。

## 验收条件

- 四个页面可在 Daemon 托管的生产构建中访问并展示 Query API 的数据。
- SSE 丢失或浏览器刷新后，页面重新查询仍与 Daemon 状态一致。
- Dashboard 只能提交定义的 Command/Decision，不能写 DB 或自行推进 Lifecycle。
- Human Required Change 在界面中可定位、查看证据并提交追加式 Decision。

## 验证

```bash
go test ./...
go vet ./...
make build
make dashboard-build
git diff --check
```

另以浏览器完成一次生产构建下的 Projects 到 Change Trace 观察验收。

## 实现边界

Dashboard 是 Client，不是 Control Plane。其可见状态必须可由 API Query 重建。

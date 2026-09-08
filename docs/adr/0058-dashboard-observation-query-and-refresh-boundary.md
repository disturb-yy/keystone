# ADR-0058：Dashboard Observation 的 Query 与 Refresh 边界

## 状态

已接受。

## 决策

Dashboard 是 Daemon 的同源 Client。Daemon 从权威状态形成有界的 `ChangeObservationReadModel` 及其他 Query ReadModel；Dashboard 只能读取这些模型、显示其 section 状态，并提交已经定义的 Command/HumanDecision。Dashboard 不访问 SQLite、Domain Repository、Worker Protocol、Workspace 或 Runtime，也不在浏览器中重建 Lifecycle、Ticket Graph、Gate、Verification、Commit、Health 或 Human Required 真相。

`GET /v1/updates` 采用 SSE，但只发送受控的 `event: refresh` 提示及可选的 Project/Change 关联。它不携带状态、Artifact、命令、Decision、cursor 或 replay token，不承担可靠投递、事件回放或状态同步职责。Dashboard 收到提示、发生断线、重连或浏览器刷新时，必须重新请求 Query；最后一次成功 snapshot 可以标记为 stale，但不能乐观改写或掩盖 Query 错误。

`ChangeObservationReadModel` 是 Daemon 侧薄组合，不建立第二套业务账本。Lifecycle、Execution、Trace、Artifact、Health 和 `available_actions` 各自保持有界 section；尚未形成的上游事实返回 `not_yet_available`，权威读模型不可用则整个 Query 返回安全错误。

## 理由与边界

Dashboard 需要同时观察多个子系统，但把多个 Domain 状态复制到 Client 会产生版本竞争、断线分叉和无法审计的客户端真相。由 Daemon 组合可以集中执行字段脱敏、有界分页、版本前置条件和 section 可用性判断，并保持现有 Domain/Application/Interface 边界。

SSE 适合减少轮询延迟，却不能保证浏览器持续在线、事件不丢或事件顺序可重放。将其限定为刷新提示后，Query 仍是唯一恢复路径，网络中断不会变成业务状态变化。该决定不引入远程访问、团队权限、完整 Trace 导出、自动恢复、Execute/Verify/Commit 控制或 Ticket 12 Golden Path 编排。

Ticket 11 的真实实现仍受 Ticket 10 acceptance evidence 阻塞；本 ADR 记录目标边界，不证明 Query、SSE 或 Dashboard 已在当前 checkout 实现。

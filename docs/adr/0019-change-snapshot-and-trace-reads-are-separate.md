# Change 快照与 Trace 查询分离

M3 的设计规定：`ChangeView` 只返回当前的 `change_id`、Project 标识、规范化 `repository_root`、Stage、Status、Version、BaseRevision、Intent Artifact、最新 AgentRun 摘要及创建/更新时间。Event、AgentRun、ArtifactRef 与 HumanDecision 分别由独立列表端点返回；无 AgentRun 时 `latest_agent_run` 为 `null`。资源标识使用小写 canonical UUIDv7，时间使用 UTC RFC3339Nano，BaseRevision 为完整小写 Git OID；M3 的这些列表不分页且以既定稳定顺序返回。

这样 `show` 可作为轻量当前快照，追溯数据不会造成嵌套、无界或重复的响应；原始 Artifact 内容仍只能按 ArtifactRef 单独读取。代价是 Client 需要在需要完整 Trace 时发起额外请求。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

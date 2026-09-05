# 统一 Event 账本保持窄边界 Contract

Project 与 Change 的 DomainEvent 使用同一追加式账本和聚合内 EventSequence，但不因此暴露通用 payload 或 details map。Project Event Query 保持其已冻结的窄 DTO；Change Event Query 只返回固定的标识、序列、类型、时间、Actor、有序 ArtifactRef 关联，以及可空的 `agent_run_id` 与 `decision_id`。这样 Event 可精确追溯到具体尝试或人工决定，而不会把内部事件演化泄漏为不受约束的 API。

`artifact_ref_ids` 不作为 `t_events` 内的 JSON 数组保存，而由专用 `t_event_artifact_refs` 关系表持有；它带 `ordinal` 并以复合外键保证 Event 与 ArtifactRef 属于同一 Change。该表是 Event 的强类型关联，不是通用多态 owner 表。创建只追加关联 Intent 的 `ChangeCreated`；Run 终态后再追加 `StageAdvanced` 或 `ChangeHumanRequired`；retry 与 decision cancel 分别以 `HumanDecisionRecorded` 后续接 `AgentRunStarted` 或 `ChangeCancelled` 表达同一事务中的不同事实。

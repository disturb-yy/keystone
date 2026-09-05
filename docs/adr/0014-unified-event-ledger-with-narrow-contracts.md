# 统一 Event 账本保持窄边界 Contract

Project 与 Change 的 DomainEvent 使用同一追加式账本和聚合内 EventSequence，但不因此暴露通用 payload 或 details map。Project Event Query 保持其已冻结的窄 DTO；Change Event Query 只返回固定的标识、序列、类型、时间、Actor 与 ArtifactRef 关联，丰富内容通过 Artifact 边界读取。这样既能统一审计排序，也不会把内部事件演化泄漏为不受约束的 API。

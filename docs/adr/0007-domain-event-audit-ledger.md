# 追加式 Domain Event 审计账本

Keystone 以统一、追加式 `t_events` DomainEvent envelope 记录 Project 和 Change 的领域事实，并为同一业务聚合分配 EventSequence；查询依据该序列复盘，而不以 UUIDv7 或时间戳推断因果顺序。Ticket 04 的 M2 Migration 建立该内部账本但只公开窄的 ProjectInitialized 查询；M3 在同一账本扩展 Change 追溯，不另建第二个业务 Event 表或通用 Event stream。权威状态仍由当前持久化模型保存，Event 只提供审计和追溯，V1 不采用完整 Event Sourcing。

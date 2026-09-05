# 追加式 Domain Event 审计账本

Keystone 以统一、追加式 DomainEvent envelope 记录 Project 和 Change 的领域事实，并为同一业务聚合分配 EventSequence；查询依据该序列复盘，而不以 UUIDv7 或时间戳推断因果顺序。权威状态仍由当前持久化模型保存，Event 只提供审计和追溯，V1 不采用完整 Event Sourcing。

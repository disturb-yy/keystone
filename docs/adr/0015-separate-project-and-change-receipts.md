# Project 与 Change 的 Receipt 分离

ProjectInitializationReceipt 处理 Manifest 与 SQLite 之间的可恢复协调，ChangeCommandReceipt 只处理纯 SQLite 状态命令与 HumanDecision 的幂等重放。两者采用相同的 key 与规范请求冲突规则，但保持独立表、独立生命周期和独立恢复语义，避免将跨资源初始化错误抽象为所有 Command 的通用行为。

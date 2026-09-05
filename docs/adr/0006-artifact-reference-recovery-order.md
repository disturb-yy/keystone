# Artifact 与权威引用的恢复顺序

Artifact 内容先在 LocalStateRoot 的 SHA-256 分层目录中同目录临时写入、文件 Sync 与原子 rename，再由同一个 SQLite transaction 写入 ArtifactRef、关联状态与 DomainEvent。已有 digest 的目标会重新校验后复用。文件与 SQLite 不伪装为分布式事务：事务失败可以留下未被引用的 Artifact，但普通进程失败时权威状态永远不能引用未成功写入的内容；孤儿回收不是正确性前提。

跨 Windows/Linux 的 M3 不宣称超出底层文件系统所能提供的断电级目录持久性。断电后若内容缺失或摘要不符，读取边界重新校验 SHA-256 并返回 unavailable，不交付损坏内容，也不在 M3 自动修复；POSIX 上可额外同步父目录，但不能把它表述为跨平台通用保证。

Artifact 内容以摘要和长度表达其可校验身份，可以由多个业务事实复用；每个关联仍拥有独立 ArtifactRef 与归属信息。M3 只生成 change_intent，并可提供有界的本机识别摘要；后续 Runtime 产生的日志、Diff 与测试输出不把任意内容摘录复制到 Event 或普通 Change 列表。

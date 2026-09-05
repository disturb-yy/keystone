# Artifact 与权威引用的恢复顺序

Artifact 内容先以可校验、原子可见的方式持久化，再由同一个 SQLite transaction 写入 ArtifactRef、关联状态与 DomainEvent。文件与 SQLite 不伪装为分布式事务：事务失败可以留下未被引用的 Artifact，但权威状态永远不能引用不存在或未校验的内容；孤儿回收不是正确性前提。

Artifact 内容以摘要和长度表达其可校验身份，可以由多个业务事实复用；每个关联仍拥有独立 ArtifactRef 与归属信息。M3 只生成 change_intent，并可提供有界的本机识别摘要；后续 Runtime 产生的日志、Diff 与测试输出不把任意内容摘录复制到 Event 或普通 Change 列表。

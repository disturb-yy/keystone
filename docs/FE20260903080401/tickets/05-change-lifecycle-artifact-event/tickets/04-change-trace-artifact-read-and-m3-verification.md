# 04: Change Trace、Artifact 内容与 M3 验收

**What to build:** 本机操作者能从一个 Change 的权威 Trace 追溯 Event、ArtifactRef、AgentRun 与 HumanDecision，并安全读取经摘要校验的原始 Artifact 内容；M3 的完整 CLI/Daemon/Git/SQLite 路径具有可复盘的验收与准确导航。

**Blocked by:** 顶层 Ticket 04：Repository Init and Project；本地 Ticket 01：创建可追溯的 Change；本地 Ticket 02：可并发保护的 Change 控制命令；本地 Ticket 03：AgentRun、HumanDecision 与 Retry 恢复。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 04` 未解除）

- [ ] Control Plane 分别提供 Change Event、Artifact、Run、Decision Trace 与 Artifact 内容读取；Event 以 EventSequence 升序，其余历史以创建时间和标识升序，空集合固定为 []，响应只含稳定的公开摘要字段与因果关联。
- [ ] Project 与 Change 继续使用同一个内部 UnifiedEventLedger。M3 以受控 Migration 演进 Ticket 04 实际已落地的账本，保持唯一权威账本，并由外键、复合归属、唯一序列和追加式约束防止跨 Change 关联或历史改写。
- [ ] Artifact 内容读取重新计算保存的 SHA-256；内容缺失、长度不符或摘要不符返回 `unavailable`，不交付损坏字节、不自动修复。成功读取只返回原始字节、保存的媒体类型、长度和摘要派生 ETag，不暴露宿主机路径，也不提供 Range 或条件读取。
- [ ] 严格 JSON、固定 DTO、UTC RFC3339Nano、canonical UUIDv7、稳定 ErrorEnvelope、404 未找到、409 业务冲突及 503 unavailable 在查询与内容读取路径中保持一致；Handler 不泄漏 SQL、Git 原始错误、绝对路径或内部存储错误。
- [ ] 以真实 CLI、loopback Daemon、临时 Git Repository、LocalStateRoot 和 SQLite 验收从创建、控制、失败恢复到 Trace/Artifact 读取的完整路径；覆盖 dirty、untracked、ignored、submodule、detached HEAD、unborn HEAD、头部变化、内容缺失、摘要失配、事务回滚和多 Change 并存。
- [ ] 完成后更新受影响导航以陈述实际文件树和验证入口，并执行完整 Go 测试、静态检查、构建与差异卫生检查。Linux、WSL 与原生 Windows 均为真实验收目标；交叉编译不替代原生 Windows 证据，也不掩盖 Ticket 04 的前置 InstanceLock 验收缺口。

# 03: 中断恢复与数据根切换

**What to build:** 当 Project 初始化在 ProjectManifest 与 SQLite 之间中断时，操作者可以恢复执行 `keystone init` 而不会产生重复身份或伪造 Event；在新的 LocalStateRoot 使用同一合法 Manifest 时，也能 bootstrap 相同 ProjectID 的本机权威 Project。

**Blocked by:** 顶层 Ticket 03 的 CLI、Daemon、SQLite Readiness 与原生 Windows InstanceLock 验收；本地 Ticket 02：安全的重复 Init 与严格 ProjectManifest。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 03` 未解除）

- [ ] Manifest 文件 I/O 的可恢复失败持久化 ProjectInitializationIntent 并返回 `manifest_unavailable`；修复外部故障后，相同 key 恢复同一候选 Project，不同 key 的同 root 附着同一活动候选。
- [ ] Manifest 已确认后，SQLite finalization 原子完成新 Project、唯一 ProjectInitialized 与成功回执；已有 Project 必须验证唯一 Event，缺失、重复或不匹配 Event 返回 `internal_error`，不得自动补写或删除。
- [ ] Manifest/SQLite 任一侧缺失时仅按已确认的 reconcile 规则恢复；确定性身份冲突释放活动 root claim 并保留同 key 的稳定失败结果。
- [ ] 使用新的 `--data-dir` 时，合法 Manifest 在新的 LocalStateRoot bootstrap 相同 ProjectID，并只在该新权威库产生一次首次 ProjectInitialized。
- [ ] 真实 Daemon 重启、真实 SQLite、真实 Manifest I/O failure 与 CLI 重试验证 pending、rollback、恢复、数据根切换和可查询 Event 的外部结果。

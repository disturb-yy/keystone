# 02: 安全的重复 Init 与严格 ProjectManifest

**What to build:** 已完成首次注册的操作者可以安全重试 `keystone init`：同一 Idempotency-Key 与同一物理 RepositoryBinding 重放稳定结果，不同 key 的同一 Binding 收敛到同一 Project，而同 key 指向不同 Binding 明确冲突。已有合法 ProjectManifest 可以协调；非法或不安全 Manifest 保持原字节不变并返回稳定错误。

**Blocked by:** 顶层 Ticket 03 的 CLI、Daemon、SQLite Readiness 与原生 Windows InstanceLock 验收；本地 Ticket 01：首次 Project 注册与查询。

**Status:** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 03` 未解除）

- [ ] 同 key、同规范化物理 root 重放保存的成功 Project；同 key、不同 root 返回 `idempotency_conflict`；不同 key、同 root 不创建第二个 Project 或 ProjectInitialized。
- [ ] ProjectManifest 仅接受 V1 的单文档、单 mapping、`version` 与小写 UUIDv7 `project_id`；未知字段、重复键、anchor、alias、tag、多文档、类型错误与损坏内容一律返回 `manifest_invalid`，不改写已有字节。
- [ ] 缺失 Manifest 的创建与并发竞争安全收敛；Manifest 或其父目录为 symlink、目录或特殊文件时拒绝，不改变已有权限、不自动写 Git。
- [ ] 成功、重放和错误均由真实 CLI/HTTP 请求、Project Query、Manifest 内容和 SQLite 权威结果验收，而不只验证 YAML parser 或内部 receipt 实现。
- [ ] ProjectInitializationReceipt 在成功、重放和确定性失败后仍能以规范化物理 root 正确判定同 key 请求；业务事件不因重试而补写或重复。

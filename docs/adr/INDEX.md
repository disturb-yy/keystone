# ADR 目录索引

本目录保存 Keystone 已接受、难以逆转且需要保留取舍背景的架构决策。ADR 解释“为什么这样定”，不替代 Ticket 实施规格、源码、测试或当前运行行为证据。

## 使用规则

- 仅记录修改成本高、脱离上下文容易被误解、且经过真实方案取舍的决策。
- 新 ADR 使用递增四位编号和简短文件名；已失效的决策通过后续 ADR 说明替代关系。
- 术语定义归入根 `CONTEXT.md`，可验证的当前路径和状态归入根 `INDEX.md`。

## 当前记录

| ADR | 主题 | 状态 |
| --- | --- | --- |
| [0001](./0001-local-daemon-control-plane.md) | 本机 Daemon 的发现与控制边界 | 已接受 |
| [0002](./0002-project-initialization-authority.md) | Project 初始化的身份与权威边界 | 已接受 |
| [0003](./0003-strict-project-manifest-v1.md) | 严格的 ProjectManifest V1 边界 | 已接受 |
| [0004](./0004-project-init-cross-resource-recovery.md) | Project 初始化的跨资源恢复顺序 | 已接受 |
| [0005](./0005-change-lifecycle-checkpoint-and-status.md) | Change 检查点与运行状态分离 | 已接受 |
| [0006](./0006-artifact-reference-recovery-order.md) | Artifact 与权威引用的恢复顺序 | 已接受 |
| [0007](./0007-domain-event-audit-ledger.md) | 追加式 Domain Event 审计账本 | 已接受 |
| [0008](./0008-change-creation-source-snapshot.md) | Change 创建时固定干净源快照 | 已接受 |

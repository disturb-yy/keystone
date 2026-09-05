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
| [0009](./0009-failure-recovery-requires-human-decision.md) | 失败恢复要求人工 Decision | 已接受 |
| [0010](./0010-late-agent-run-result-fence.md) | Pause 与 Cancel 隔离晚到 AgentRun 结果 | 已接受 |
| [0011](./0011-change-command-version-precondition.md) | Change Command 的版本前置条件 | 已接受 |
| [0012](./0012-agent-run-single-write-outcome.md) | AgentRun 的一次性终态 | 已接受 |
| [0013](./0013-separate-change-command-and-decision-boundaries.md) | Change Command 与 HumanDecision 分离 | 已接受 |
| [0014](./0014-unified-event-ledger-with-narrow-contracts.md) | 统一 Event 账本保持窄边界 Contract | 已接受 |
| [0015](./0015-separate-project-and-change-receipts.md) | Project 与 Change 的 Receipt 分离 | 已接受 |
| [0016](./0016-daemon-resolves-project-for-change-creation.md) | Daemon 为 Change 创建解析 Project | 已接受 |

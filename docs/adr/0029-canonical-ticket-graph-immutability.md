# ADR-0029：每个 Change 的 Canonical Ticket Graph 不可替换

## 状态

已接受。

## 决策

V1 中，一个 Change 只接受一次成功持久化的 Canonical Ticket Graph。该图绑定同一 Change 的已验证 Plan；Daemon 为 CanonicalTicket 和图身份分配权威标识，成功后不允许重生成、编辑、替换或版本化。

## 理由与边界

不可替换的图使 Ticket、依赖、来源 Plan、创建 Event 和后续执行证据保持同一条可复盘链路，避免重生成静默改变已观察或已执行的工作。失败的 TicketGeneration 可以保留为候选与失败事实，并通过人工恢复发起新尝试；若需要改变已成功图，V1 应创建新的 Change。Graph versioning、图编辑和跨 Change 替换不在 V1 范围内。本 ADR 记录 Ticket 08 的目标契约，不证明其实现存在或解除其 `BLOCKED_BY: 07`。

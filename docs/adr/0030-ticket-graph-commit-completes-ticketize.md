# ADR-0030：Canonical Ticket Graph 提交原子完成 Ticketize

## 状态

已接受。

## 决策

Ticketize 只有在当前 active Change 的 Ticketize AgentRun 产生的 Draft 通过校验后，才可完成。Work authority 在同一事务中确认没有既有 Canonical Ticket Graph，持久化来源 Plan 与 Draft 的关联、Graph、CanonicalTicket、TicketDependency、`TicketGraphCreated`、AgentRun 成功和 `StageAdvanced`，再将 Change 推进到 Execute。

## 理由与边界

拆分提交会使 Change 已进入 Execute 却没有可查询图，或在重试与并发恢复时留下重复或半完成的权威事实。失败、暂停、取消或晚到结果不能创建部分图；失败只形成受围栏的 AgentRun 与证据，并由人工恢复决定下一次 TicketGeneration。本 ADR 定义 Ticket 08 的目标事务边界，不证明 Ticket 08 已实现或解除其 `BLOCKED_BY: 07`。

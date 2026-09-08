# ADR-0031：被围栏的 TicketGeneration 不复用

## 状态

已接受。

## 决策

若 Pause、Cancel 或执行权失效先于 Canonical Ticket Graph 提交成为权威事实，TicketGeneration 只能保留候选与 AgentRun Trace，不能创建 Graph。Cancel 后永不恢复；Resume 在重新满足条件后创建新的 TicketGeneration，不复用已经围栏的候选。

## 理由与边界

复用围栏候选会把一次已脱离当前控制状态的 AgentRun 与后续 Graph 提交重新拼接，破坏 TicketizeCompletion 的原子边界，也使暂停/取消竞态难以复盘。新的明确恢复动作产生新的候选与证据链；它不是自动 retry。该决策只定义 Ticket 08 的目标恢复语义，不证明其实现存在或解除其 `BLOCKED_BY: 07`。

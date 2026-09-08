# ADR-0039：确定性执行调度与私有 Workspace 位置

## 状态

已接受。

## 决策

Ticketize 成功时，Daemon 与 CanonicalTicketGraph 一同持久化每张 CanonicalTicket 的不可变顺序。Scheduler 以持久化的 ExecutionSession 创建顺序 FIFO 竞争同一 Project 的唯一执行槽；已取得槽的会话不被抢占。该会话内只从 RunnableTicket 中选择最小 CanonicalTicketOrder，并在授权与创建 AgentRun 的同一权威事务中将该 Ticket 置为 `assigned`。

Workspace 的物理位置由 Daemon 内部按 `<LocalStateRoot>/workspaces/<project-id>/<change-id>` 派生。默认 LocalStateRoot 可解析为 `~/.keystone`，但 Client、Runtime 输入和只读模型都不得把该用户目录或绝对 Workspace 路径作为公开契约；Runtime 只能接收已授权 Workspace 的实际位置。

## 理由与边界

持久化顺序使同一权威状态产生相同的调度结果，避免 UUID、墙钟时间、Worker 拉取先后或临时内存队列改变执行选择。Project 单槽与不抢占使 Change 间写入 Workspace 的顺序可解释，而不扩展为跨 Change 并行或多 Worker 调度器。

Workspace 路径属于本机运行细节，不应成为 Client 可选择、可枚举或跨机器稳定引用的身份。该 ADR 冻结 Ticket 09 的目标契约，不证明 Scheduler、目录创建或路径隔离已实现。

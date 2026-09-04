# Keystone 运行语义上下文

本上下文收敛 Keystone 本机 Control Plane 的稳定运行术语，避免将 Daemon 就绪、Ticket 可执行性和规划文档状态混为一谈。

## 本机 Control Plane

**LocalStateRoot**：
由本机操作者选择的数据根，用于承载一个 Keystone 本机实例的运行状态。每个 LocalStateRoot 同时至多对应一个活跃的 DaemonInstance。
_避免_：用户全局状态、Repository 状态目录、项目状态目录

**DaemonInstance**：
在一个 LocalStateRoot 上运行并拥有该根 Control Plane 权威状态的独立 Daemon 进程。
_避免_：CLI 进程、Worker、数据库连接

**DaemonInstanceID**：
标识一个 DaemonInstance 的唯一关联标识，用于让 Client 确认控制请求仍指向观察到的实例。它只防止陈旧目标，不是安全凭据。
_避免_：进程 PID、访问令牌、锁所有权

**InstanceLock**：
对一个 LocalStateRoot 排他绑定一个 DaemonInstance 的本机互斥事实。它是实例排他性的唯一权威，不能由 PID 或 RuntimeMetadata 替代。
_避免_：PID 所有权、metadata 所有权、端点所有权

**RuntimeMetadata**：
供本机 Client 发现和诊断 DaemonInstance 的运行记录。它不是实例锁、身份权威或就绪状态的判定依据。
_避免_：锁记录、Ready Metadata、进程真相

**DaemonEndpoint**：
DaemonInstance 接收本机 Control Plane 请求的 loopback 地址。Client 使用它建立请求连接，但不以它替代实例权威性判断。
_避免_：固定服务端口、远程 API 地址

**DaemonReadiness**：
DaemonInstance 能够安全服务本机 Control Plane 请求的运行状态。它只描述 DaemonInstance，不描述 Ticket 或实施文档。
_避免_：Ticket ready、ready-for-agent、规划完成

**SchemaMigrationVersion**：
一个 LocalStateRoot 所属持久状态中，已成功提交的最高 Schema Migration 版本。
_避免_：业务版本、应用版本、Ticket 版本

## 工作流与端点

**RunnableTicket**：
依赖与执行前置条件均已满足、可进入执行 frontier 的 Ticket。它不等同于文档已准备好或 Daemon 已就绪。
_避免_：ready-for-agent、规划就绪

**PlanningReadiness**：
Ticket 或规格的描述已足以被实现者认领的文档状态。它不会解除 BLOCKED_BY，也不会证明实现已完成。
_避免_：RunnableTicket、DaemonReadiness

**DaemonReadinessEndpoint**：
专门报告 DaemonReadiness 的本机 HTTP 端点。它不表示任何被 Keystone 管理的 Repository 服务状态。
_避免_：Demo Service Health Endpoint、业务健康检查

**DemoServiceHealthEndpoint**：
Golden Path 中被 Keystone 管理的示例服务自身的健康检查端点。它与 DaemonReadinessEndpoint 属于不同系统主体。
_避免_：DaemonReadinessEndpoint

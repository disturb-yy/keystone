# Daemon 包规约

本包拥有 Daemon 的启动、就绪、HTTP Handler、SQLite 和单实例锁资源生命周期。

- 只通过 `contracts/controlplane` 暴露 Control Plane DTO。
- 使用 `internal/infrastructure/localstate` 管理数据根、目录、锁和运行元数据。
- 使用 `internal/infrastructure/migration` 管理 `t_schema_migrations`，并在启动时接入
  `internal/infrastructure/workstore` 的 Project、Change、Worker 与 Planning Migration。
- Project Handler 只做 DTO 解码、边界校验和错误映射；Project 业务通过 `internal/work`
  Application 及其 ports 完成。
- Change Handler 只做严格 JSON/路径校验、DTO 转换和稳定错误映射；Change、Artifact、
  AgentRun、HumanDecision 和 Event 业务通过 `internal/work` Application 完成。Change 创建、
  Resume 与人工 retry 成功保存后只发送 Planning 唤醒提示。
- Worker Handler 只接受 loopback、严格 JSON 和当前 Bearer secret，随后调用 `workstore`
  的 Worker authority；它不在 Handler 中写 SQL、落 Artifact 或推进 AgentRun。Worker 注册和
  Planning candidate Report 落成耐久事实后，只唤醒 Planning 耐久扫描。
- Worker Supervisor 在启动期监管最多一个独立 `keystone-worker`，Planning 首次耐久恢复完成后 Daemon 才进入 readiness；secret 只通过 stdin
  启动管道传递，Worker 缺失不会伪造 Daemon not-ready。Supervisor 通过跨平台进程树
  围栏、耐久 heartbeat/Lease watchdog 和有界优雅停止处理 Worker 退出。
- Planning composition 只连接 `planning.Coordinator`、Repository Snapshot、Artifact store 与
  Workstore Worker authority；启动和事件唤醒都必须重新扫描耐久状态，不把内存信号当作事实。
- V1 Planning 只在 Daemon 内部触发；HTTP 路由不提供 Planning start endpoint。
- Verify、Commit、FinalVerify Handler 只接收 `expected_version` 和 `Idempotency-Key`，由
  Workstore 固定 policy/revision/evidence；Worker Report 前由 Daemon 独立复核候选身份。Commit
  只使用 SourceControl 的受控 parent/tree/trailer seam，HTTP 不直接写 SQL/Git。
- ExecutionReadModel 只返回 gate、Intent、Evidence、Commit 和 CandidateRevision 摘要，不返回
  WorkspacePath、Lease/token、Prompt、环境或原始命令输出。
- stop 命令不依赖数据库；关闭顺序必须是 Planning Manager、Worker Supervisor、临时 Snapshot、
  HTTP Server、数据库、元数据、锁。若前置循环未在预算内停稳，Snapshot 清理必须延后并
  返回可观察错误，不能把未确认停止当成成功。
- 注释使用中文，技术标识符保持原样。

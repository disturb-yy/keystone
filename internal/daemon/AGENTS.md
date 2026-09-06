# Daemon 包规约

本包拥有 Daemon 的启动、就绪、HTTP Handler、SQLite 和单实例锁资源生命周期。

- 只通过 `contracts/controlplane` 暴露 Control Plane DTO。
- 使用 `internal/infrastructure/localstate` 管理数据根、目录、锁和运行元数据。
- 使用 `internal/infrastructure/migration` 管理 `t_schema_migrations`，并在启动时接入
  `internal/infrastructure/workstore` 的 Project Bootstrap Migration。
- Project Handler 只做 DTO 解码、边界校验和错误映射；Project 业务通过 `internal/work`
  Application 及其 ports 完成。
- Change Handler 只做严格 JSON/路径校验、DTO 转换和稳定错误映射；Change、Artifact、
  AgentRun、HumanDecision 和 Event 业务通过 `internal/work` Application 完成。
- stop 命令不依赖数据库；关闭顺序必须是 HTTP Server、数据库、元数据、锁。
- 注释使用中文，技术标识符保持原样。

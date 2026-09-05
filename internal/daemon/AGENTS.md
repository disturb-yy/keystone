# Daemon 包规约

本包拥有 Daemon 的启动、就绪、HTTP Handler、SQLite 和单实例锁资源生命周期。

- 只通过 `contracts/controlplane` 暴露 Control Plane DTO。
- 使用 `internal/infrastructure/localstate` 管理数据根、目录、锁和运行元数据。
- 使用 `internal/infrastructure/migration` 管理 `t_schema_migrations`，不创建业务表。
- stop 命令不依赖数据库；关闭顺序必须是 HTTP Server、数据库、元数据、锁。
- 注释使用中文，技术标识符保持原样。

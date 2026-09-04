# migration 局部规约

`migration` 负责 Keystone 本机 SQLite 的只增量 Migration 资产和事务 runner。

- 只接受调用方提供的 `*sql.DB`，不拥有数据库连接或本机锁生命周期。
- Migration 版本必须是正整数且严格递增；已应用记录的名称或 checksum 漂移必须停止。
- 首个默认版本只建立 `t_schema_migrations` 元数据表，不预建业务表。
- 每个版本独立事务提交；失败时回滚，不提供 down migration 或隐式修复。
- 使用纯 Go 的 `modernc.org/sqlite` 注册 `sqlite` driver，保持跨平台无 CGO 前提。

修改后至少运行本 package 测试、根级 Go 测试、`go vet` 和 Windows 无 CGO 构建。

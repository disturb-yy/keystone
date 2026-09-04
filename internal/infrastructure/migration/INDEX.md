# migration 项目索引

## 当前状态

该 package 已落地 SQLite Migration 基线，不创建业务 Schema。

| 文件 | 职责 |
| --- | --- |
| `runner.go` | Migration 模型、默认元数据 Migration、版本校验、漂移检查和事务应用 |
| `runner_test.go` | 使用真实 SQLite 验证空库、重复、增量、回滚、漂移、未知版本和非法版本 |

## 主要入口

- `DefaultMigrations` 返回默认 Migration 副本。
- `NewRunner` 创建 Migration runner。
- `Runner.Apply` 读取 `sqlite_master` 和已应用记录，并逐版本提交事务。

## 边界

数据库连接和本机单实例锁由调用方拥有；本 package 不实现 Daemon、业务表、Repository、down migration 或隐式修复。

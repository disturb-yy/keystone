# localstate 项目索引

## 当前状态

本 package 提供本机运行状态基础原语，不承载 Daemon、HTTP、SQLite 或业务领域行为。

| 文件 | 职责 |
| --- | --- |
| `paths.go` | 解析数据根、派生固定路径及显式目录初始化 |
| `lock.go` | 锁生命周期与错误分类 |
| `lock_unix.go` | Linux/WSL 等 Unix 的非阻塞 `flock` |
| `lock_windows.go` | Windows 的非阻塞 `LockFileEx` |
| `metadata.go` | 原子发布和按实例标识清理 JSON 元数据 |

主要入口为 `Resolve`、`Paths.Initialize`、`Acquire`、`InstanceLock.Release`、`PublishMetadata` 和 `ClearMetadata`。

# Infrastructure 项目索引

## 当前状态

该目录当前包含八个可编译、可测试的基础能力 package：

| 路径 | 当前职责 | 主要入口 |
| --- | --- | --- |
| `config/` | 读取并解析日志等级环境变量 | `config.Load`、`config.ParseLevel` |
| `logging/` | 创建 JSON 结构化 logger | `logging.New` |
| `id/` | 使用标准库生成时间有序 UUIDv7 字符串 | `id.New` |
| `localstate/` | 解析本机数据根、初始化目录、跨平台单实例锁和运行元数据 | `localstate.Resolve`、`Paths.Initialize`、`Acquire`、`PublishMetadata` |
| `migration/` | 使用纯 Go SQLite driver 执行 `t_schema_migrations` 增量 Migration | `migration.NewRunner`、`Runner.Apply`、`migration.DefaultMigrations` |
| `manifest/` | 严格读取、原子创建和解析 ProjectManifest V1 | `manifest.Store.Ensure` |
| `repository/` | 只读识别 Git root、拓扑和旧 root 可验证性 | `repository.Git.Discover`、`Git.RootExists` |
| `workstore/` | Project Bootstrap SQLite Migration、intent、receipt、Project 和 Event | `workstore.New`、`Store.Reserve`、`Store.Finalize` |

每个 package 都有局部 `AGENTS.md`、`INDEX.md`、Go 源码和聚焦单测。

## 依赖关系

```text
config  → os、fmt、errors、log/slog
logging → io、log/slog
id      → uuid（Go 1.27 标准库）
```

`config`、`logging`、`id` 之间没有代码依赖；`localstate` 只依赖操作系统文件能力和平台锁原语，`migration` 只依赖 `database/sql` 与纯 Go SQLite driver；`manifest`、`repository` 和 `workstore` 依赖 Work Domain/端口模型以实现 Ticket 04 adapter。基础能力不访问 HTTP 或其他进程。

## 验证入口

在仓库根目录运行：

```text
gofmt -w internal/infrastructure/{config,logging,id,localstate,migration}/*.go
go test ./internal/infrastructure/...
go test ./...
go vet ./...
```

以上命令验证当前基础能力、本机状态边界和 SQLite Migration 的格式、行为和静态检查；涉及目录权限的测试应在支持 POSIX mode 的临时目录上运行。

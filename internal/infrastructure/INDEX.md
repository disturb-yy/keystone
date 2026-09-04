# Infrastructure 项目索引

## 当前状态

该目录当前包含三个可编译、可测试的标准库基础能力 package：

| 路径 | 当前职责 | 主要入口 |
| --- | --- | --- |
| `config/` | 读取并解析日志等级环境变量 | `config.Load`、`config.ParseLevel` |
| `logging/` | 创建 JSON 结构化 logger | `logging.New` |
| `id/` | 使用标准库生成时间有序 UUIDv7 字符串 | `id.New` |

每个 package 都有局部 `AGENTS.md`、`INDEX.md`、Go 源码和聚焦单测。

## 依赖关系

```text
config  → os、fmt、errors、log/slog
logging → io、log/slog
id      → uuid（Go 1.27 标准库）
```

三个 package 之间没有代码依赖；它们都位于基础设施边界内，不访问数据库或其他进程。

## 验证入口

在仓库根目录运行：

```text
gofmt -w internal/infrastructure/{config,logging,id}/*.go
go test ./internal/infrastructure/...
go test ./...
go vet ./...
```

以上命令验证当前 Go 基础能力的格式、行为和静态检查。

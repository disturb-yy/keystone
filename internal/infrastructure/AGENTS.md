# Infrastructure 局部规约

## 边界

`internal/infrastructure/` 当前承载八个职责明确的跨领域基础能力：

- `config/` 读取并解析 `KEYSTONE_LOG_LEVEL` 环境变量。
- `logging/` 创建由调用者指定输出目标和等级的 JSON 结构化 logger。
- `id/` 使用 Go 1.27 标准库 `uuid.NewV7()` 生成时间有序的 UUIDv7 字符串。
- `localstate/` 解析 Keystone 本机数据根，初始化固定目录，提供跨平台单实例锁和诊断元数据。
- `migration/` 使用纯 Go `modernc.org/sqlite` 在调用方连接上执行 `t_schema_migrations` 增量 Migration。
- `manifest/` 严格读写 ProjectManifest V1，并以临时文件原子发布缺失 Manifest。
- `repository/` 只读识别 Git Repository root 与旧 binding 拓扑。
- `workstore/` 持有 Work 领域 Project Bootstrap 的 SQLite Migration、状态和回执适配。

这些 package 不承载领域语义、Daemon 生命周期、Worker 监管、业务 Schema 或进程协议。`localstate` 和 `migration` 是 Ticket 02 的本机边界基础设施；它们不拥有调用方的数据库连接或单实例锁生命周期。

## 依赖约束

- `manifest/`、`repository/` 和 `workstore/` 可以依赖 `internal/work/domain` 及 `internal/work` 的端口模型，但不向 Domain 泄漏基础设施细节。
- 基础能力 package 不依赖 Interface 或其他具体基础设施实现。
- `config/` 不读取配置文件、不处理命令行参数、不执行重载或持久化。
- `logging/` 不替换 `slog.Default()`，也不拥有调用者传入的输出目标。
- `id/` 不提供持久化或领域 ID 类型；UUIDv7 的时间顺序不作为业务排序事实。

## 修改与验证

- 修改某个 package 前先阅读该 package 的 `AGENTS.md`、`INDEX.md` 及相关源码和测试。
- 新增 Go 文件必须保持 package 职责窄小，并为可观察行为补充聚焦测试。
- 使用 `gofmt` 格式化；至少运行受影响 package 的测试、`go test ./...` 和 `go vet ./...`。
- 本目录文档只记录当前文件树、边界和可验证行为，不记录未实现能力。

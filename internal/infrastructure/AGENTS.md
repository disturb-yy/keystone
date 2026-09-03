# Infrastructure 局部规约

## 边界

`internal/infrastructure/` 当前承载三个职责明确的跨领域基础能力：

- `config/` 读取并解析 `KEYSTONE_LOG_LEVEL` 环境变量。
- `logging/` 创建由调用者指定输出目标和等级的 JSON 结构化 logger。
- `id/` 使用安全随机源生成 UUIDv4 字符串。

这些 package 只使用 Go 标准库，不承载领域语义、数据库、迁移、数据目录、进程协议或全局运行状态。

## 依赖约束

- 基础能力 package 不依赖领域、Application、Interface 或具体基础设施实现。
- `config/` 不读取配置文件、不处理命令行参数、不执行重载或持久化。
- `logging/` 不替换 `slog.Default()`，也不拥有调用者传入的输出目标。
- `id/` 不提供排序、持久化或领域 ID 类型。

## 修改与验证

- 修改某个 package 前先阅读该 package 的 `AGENTS.md`、`INDEX.md` 及相关源码和测试。
- 新增 Go 文件必须保持 package 职责窄小，并为可观察行为补充聚焦测试。
- 使用 `gofmt` 格式化；至少运行受影响 package 的测试、`go test ./...` 和 `go vet ./...`。
- 本目录文档只记录当前文件树、边界和可验证行为，不记录未实现能力。

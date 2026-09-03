# Keystone

当前仓库的可核验内容如下：

- `docs/architecture-baseline/`：12 个 Markdown 架构参考文件和 1 个 JSON 摘要文件。
- `cmd/keystone/`、`cmd/keystone-daemon/`、`cmd/keystone-worker/`、`configs/`：当前只包含 `.gitkeep` 标记文件；`internal/` 为空目录。
- `contracts/`、`dashboard/`、`migrations/`、`scripts/`：当前尚未创建。
- `go.mod`：声明模块 `github.com/disturb-yy/keystone` 和 Go `1.27`，没有其他模块文件。
- 当前文件树中没有 `.go`、`.sql` 或 Dashboard 前端源码文件，因此本仓库没有可供验证的 Keystone 服务运行行为。

`docs/architecture-baseline/` 中的文件是静态架构参考文本；其中的架构图、名称和流程文字不构成当前服务行为或接口实现的证据。

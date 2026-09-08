# `contracts/controlplane` 工程规约

## 职责

本 package 只定义 CLI、Dashboard 与 Control Plane Daemon 之间的 HTTP/JSON
传输边界。当前范围是 `/v1` 版本前缀、Daemon/Project/Change 强类型 DTO、结构化错误
envelope、Planning Artifact/AgentRun 只读元数据和 `Idempotency-Key` 的非空不透明值表达。

## 依赖与边界

- 只依赖 Go 标准库。
- 不导入 Domain、Application、Infrastructure、SQLite 或具体 Daemon 实现。
- 不实现 HTTP Handler、路由、状态码、Daemon 生命周期、Command 或 Query。
- DTO 只表达边界字段，不使用通用 `data` wrapper 或无约束 `details` map；Change DTO 不暴露物理 Artifact 路径。
- Planning metadata 使用可选字段兼容旧 ArtifactRef/AgentRun；引用只暴露稳定 ID，不包含 Artifact 内容或临时 Snapshot 路径。

## 修改与验证

- 修改 DTO 时同步更新 `contract_test.go` 的 JSON 编解码覆盖。
- 新增只读 metadata 时验证 legacy 零值仍省略，避免改变既有 JSON 形状。
- `ErrorEnvelope` 的 `code`、`message` 保持必需字段，`request_id` 保持可选。
- 健康 DTO 只保留 `ready`；Project DTO 固定字段；幂等键只检查非空并保留原始值。
- 在本 package 目录运行 `gofmt`，并执行：
  `GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./contracts/controlplane -count=1`。

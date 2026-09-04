# `contracts/controlplane` 工程规约

## 职责

本 package 只定义 CLI、Dashboard 与 Control Plane Daemon 之间的 HTTP/JSON
传输边界。当前范围是 `/v1` 版本前缀、强类型健康成功 DTO、结构化错误
envelope 和 `Idempotency-Key` 的非空不透明值表达。

## 依赖与边界

- 只依赖 Go 标准库。
- 不导入 Domain、Application、Infrastructure、SQLite 或具体 Daemon 实现。
- 不实现 HTTP Handler、路由、状态码、Daemon 生命周期、Command 或 Query。
- DTO 只表达边界字段，不使用通用 `data` wrapper 或无约束 `details` map。

## 修改与验证

- 修改 DTO 时同步更新 `contract_test.go` 的 JSON 编解码覆盖。
- `ErrorEnvelope` 的 `code`、`message` 保持必需字段，`request_id` 保持可选。
- 健康 DTO 只保留 `ready`；幂等键只检查非空并保留原始值。
- 在本 package 目录运行 `gofmt`，并执行：
  `GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./contracts/controlplane -count=1`。

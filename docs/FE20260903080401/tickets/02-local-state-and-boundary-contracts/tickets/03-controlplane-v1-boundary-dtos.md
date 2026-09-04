# 03: `/v1` Control Plane 边界 DTO

**What to build:** 让后续 CLI 与 Dashboard Client 能依赖稳定的 `/v1` 传输契约表达成功响应、错误、健康状态和幂等键，而不需要知道 Daemon 内部的 Domain、SQLite 或 HTTP Handler 实现。

**Blocked by:** Ticket 01 Engineering Foundation 验收。

**Status:** implemented

- [x] Control Plane Contract 以独立 package 编译，并固定 `/v1` 版本前缀与强类型成功 DTO 的边界。
- [x] 错误 envelope 提供机器可读 `code`、安全 `message` 与可选 `request_id`，且不引入无约束 `data` wrapper 或 `details` map。
- [x] 健康 DTO 只表达 `ready` 状态；本 Ticket 不实现 HTTP 路由、状态码映射或 Daemon 生命周期。
- [x] `Idempotency-Key` 以非空、不透明 HTTP header 表达；具体 mutating Command 是否要求该 header 留给后续 Ticket。
- [x] JSON 编解码和独立编译测试证明 Contract 不导入或导出 Domain Entity，也不访问 SQLite。

验证记录：`go test ./contracts/controlplane` 与根级 `go test ./...` 已通过；package 只依赖标准库。

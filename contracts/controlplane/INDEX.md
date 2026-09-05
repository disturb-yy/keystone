# `contracts/controlplane` 项目索引

## 当前状态

该 package 已落地当前 CLI/Daemon 使用的 Control Plane `/v1` JSON 边界契约。
它只定义传输 DTO，不包含 HTTP Server、Handler、Daemon 生命周期、Domain 或
SQLite 行为；这些行为由 `internal/daemon` 负责。

## 文件

- `contract.go`：版本前缀、错误 envelope、健康响应、Daemon status/stop DTO 和幂等键类型。
- `contract_test.go`：Health、Daemon status/stop、错误 envelope、幂等键的 JSON round-trip 和字段约束测试。
- `AGENTS.md`：本 package 的职责、依赖和验证规约。

## 当前 DTO

| 类型 | 边界用途 |
| --- | --- |
| `HealthResponse` | `GET /healthz` 的 `ready` 字段；HTTP 状态码由 Handler 决定 |
| `ErrorEnvelope` | `/v1` 无效请求、实例不匹配和服务不可用的结构化错误 |
| `DaemonStatusResponse` | `GET /v1/daemon/status` 的 DatabasePath、SchemaMigrationVersion、DaemonReadiness 和 DaemonInstanceID |
| `DaemonStopRequest` | `POST /v1/daemon/stop` 携带必需的 DaemonInstanceID |
| `DaemonStopResponse` | 返回是否接受停止请求及接受请求的 DaemonInstanceID |
| `IdempotencyKey` | 保留非空、不透明的 `Idempotency-Key` 值；当前 stop DTO 不要求该 Header |

## 关系

```text
cmd/keystone → contracts/controlplane → internal/daemon HTTP Handler
```

`dashboard/` 当前仍是前端骨架，没有已落地的业务 API 调用；它是该边界的
目标客户端。`internal/daemon/server.go` 注册并实现 `/healthz`、
`/v1/daemon/status` 和 `/v1/daemon/stop`，但不把 Handler 或 SQLite 代码放入
本 package。

## 明确边界

- 只依赖 Go 标准库，不引用 Domain、Application、Infrastructure、SQLite 或具体 Daemon 实现。
- 不拥有 InstanceLock、RuntimeMetadata、SQLite 连接、Migration、readiness 或权威业务状态。
- 不定义 Worker runtime、Project、Change、Ticket 或业务 Schema；`contracts/worker` 是独立的另一条传输边界。

## 验证入口

```bash
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./contracts/controlplane -count=1
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./... -count=1
```

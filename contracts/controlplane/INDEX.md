# `contracts/controlplane` 项目索引

## 当前状态

该 package 已落地 Control Plane 的最小 `/v1` JSON 边界契约；不包含 HTTP
Server、Daemon、Domain 或 SQLite 行为。

## 文件

- `contract.go`：版本前缀、错误 envelope、健康响应和幂等键类型。
- `contract_test.go`：必需字段、可选 `request_id`、键值校验及 JSON round-trip 测试。
- `AGENTS.md`：本 package 的职责、依赖和验证规约。

## 关系

```text
CLI / Dashboard → contracts/controlplane → Control Plane Daemon
```

该关系是传输边界；本 package 不拥有 Daemon 的权威状态，也不引用内部
Domain Entity。

## 验证入口

```bash
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./contracts/controlplane -count=1
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./... -count=1
```

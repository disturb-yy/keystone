# worker 局部规约

## 职责

`contracts/worker` 只定义 Keystone Daemon 与独立 Worker 之间的传输 DTO：`Register`、`Heartbeat`、`Assignment` 和 `Report`。

## 边界

- DTO 中的标识是传输标识，不复用或导出 Domain Entity。
- `Report.Outcome` 是不透明、可扩展的字符串传输值；本包不解释其具体值，也不定义生命周期状态机。
- 本包不处理 HTTP、Daemon、Worker 监管、Runtime 调用、租约校验、Report 幂等、Artifact 采集、SQLite 或状态推进。
- 本包只使用 Go 标准库中的 JSON 标签约定，不引入第三方依赖或其他 Keystone package。

## 修改与验证

新增或修改边界字段时，先同步核对 Ticket 02 spec，再在 `worker_test.go` 中覆盖 JSON 必需字段和最小 payload，并运行：

```text
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./contracts/worker -count=1
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./... -count=1
```

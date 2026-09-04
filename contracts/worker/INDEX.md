# worker 项目索引

## 当前职责

`contracts/worker` 是 Daemon 与独立 Worker 之间的窄传输边界，当前只承载 Ticket 02 冻结的四组 JSON DTO。

| 文件 | 内容 |
| --- | --- |
| `worker.go` | `Register`、`Heartbeat`、`Assignment`、`Report` 及 `Outcome` 传输类型 |
| `worker_test.go` | 四组 DTO 的 JSON 编码、最小 payload 解码和可扩展 outcome 测试 |

## 当前边界事实

- `Register` 包含 `worker_id`、`protocol_version` 和 `capabilities`。
- `Heartbeat` 包含 `worker_id`。
- `Assignment` 包含 `agent_run_id`、不透明 `lease_token`、`workspace_path` 和 `runtime`。
- `Report` 包含 `agent_run_id`、`lease_token` 和字符串 `outcome`。
- 本包没有 HTTP Handler、Daemon、真实 Runtime、Domain、SQLite 或状态推进实现。

## 验证

```text
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./contracts/worker -count=1
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./... -count=1
```

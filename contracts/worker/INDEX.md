# worker 项目索引

## 当前职责

`contracts/worker` 是 Daemon 与独立 Worker 之间的窄传输边界，承载 Ticket 06 的严格 JSON DTO 和单值解码规则。

| 文件 | 内容 |
| --- | --- |
| `worker.go` | Register、Heartbeat、Pull、Claim、Assignment、Report DTO、Artifact payload 与 `DecodeStrict` |
| `worker_test.go` | JSON 编码、最小 payload、空 Assignment、未知字段、重复 key、多个值和 body 上限测试 |

## 当前边界事实

- `Register` 包含 `worker_id`、`protocol_version` 和 `capabilities`；Response 不包含 secret。
- `Heartbeat` 可携带 `agent_run_id` 和不回显的 `lease_token_sha256`。
- `PullResponse.assignment` 无任务时固定编码为 `null`。
- `Assignment` 包含 `agent_run_id`、opaque `lease_token`、Lease/Workspace/Runtime/输入摘要字段；Planning Assignment 以 `result_mode=planning_candidate` 请求候选结果。
- `edit` Assignment 携带固定 timeout，Worker 必须先提交一次性 RuntimeClaim 才能启动 Runtime。
- `Report` 包含 exit/time/revision、执行证据与可选 candidate Artifact、capture failure 和 guard finding；authority 仍由 Daemon 解释。
- 本包没有 HTTP Handler、Daemon、真实 Runtime、Domain、SQLite 或状态推进实现。

## 验证

```text
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./contracts/worker -count=1
GOCACHE=/tmp/keystone-ticket-02-go-cache go test ./... -count=1
```

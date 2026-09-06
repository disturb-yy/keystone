# `cmd/keystone` 项目索引

## 当前状态

该 package 已提供真实的 `keystone init`、`keystone change ...` 和
`keystone daemon start|status|stop` CLI。命令使用
归一化的 `LocalStateRoot` 读取 RuntimeMetadata，并只经
`contracts/controlplane` 的 HTTP/JSON 边界访问 Daemon；CLI 不访问 SQLite。

## 文件

| 文件 | 职责 |
| --- | --- |
| `main.go` | 组装默认依赖并执行 CLI |
| `command.go` | Cobra 命令树、共同 `--data-dir` 输入和命令编排 |
| `metadata.go` | 只读 RuntimeMetadata 与 loopback endpoint 校验 |
| `http_client.go` | Control Plane health/status/stop 的 JSON client |
| `change_command.go` | Change create/list/show、生命周期控制和 HumanDecision CLI 命令 |
| `process.go` | `keystone-daemon` 发现、启动和 readiness 轮询 |
| `errors.go` | CLI 错误分类 |
| `command_test.go` | 命令树、生命周期分支和依赖注入测试 |
| `AGENTS.md` | 本 package 的职责与验证规约 |

## 依赖关系

```text
keystone CLI
├── contracts/controlplane
├── internal/infrastructure/localstate（Resolve、Paths、Metadata）
└── internal/infrastructure/id（CLI 生成 Idempotency-Key）
```

`start`、`init` 和 Change 写命令复用同目录或 PATH 中的 `keystone-daemon` ensure seam，并传入
归一化的 `LocalStateRoot`；`init` 只提交当前目录的绝对路径，`status` 和 `stop`
只读取 metadata 并发起 HTTP 请求，Change 读取命令不启动新 Daemon。

## 验证入口

```bash
GOCACHE=/tmp/keystone-ticket-05-go-cache go test ./cmd/keystone -count=1
GOCACHE=/tmp/keystone-ticket-05-go-cache go test ./... -count=1
GOCACHE=/tmp/keystone-ticket-05-go-cache go vet ./...
```

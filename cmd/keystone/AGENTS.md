# `cmd/keystone` 工程规约

该 package 是 Keystone 的本机 CLI Interface Adapter，只负责：

- 通过 Cobra 暴露 `keystone daemon start|status|stop` 命令树；
- 在 `daemon` 父命令边界一次解析 `--data-dir`，并把归一化的
  `LocalStateRoot` 传给子命令或 `keystone-daemon`；
- 读取 `RuntimeMetadata` 发现 loopback endpoint；
- 通过 `contracts/controlplane` 的 HTTP/JSON DTO 查询健康、状态和停止结果；
- 在 `start` 中发现并启动独立的 `keystone-daemon`，在有界时间内确认 readiness。
- 在 `init` 中归一化当前目录、复用 Daemon ensure seam，并通过 loopback API
  提交 Project 初始化；CLI 不读取 Manifest 或 SQLite。
- 通过 Control Plane API 暴露 Change create/list/show/pause/resume/cancel 和
  `decide retry|cancel`；Change 写命令共用 Daemon ensure seam，纯查询只发现已有 Daemon。

该 package 不打开 SQLite、不获取或判断 `InstanceLock`、不按 PID 控制进程，
也不实现 Daemon HTTP Handler。`status` 与 `stop` 不得启动子进程；只有
`start` 可以启动或复用 Daemon。

生产代码通过依赖注入暴露 HTTP client、metadata 路径解析、Daemon 可执行文件
发现、命令 runner、时钟和超时，以便测试不依赖真实用户目录或固定端口。

修改后至少运行：

```bash
GOCACHE=/tmp/keystone-ticket-05-go-cache go test ./cmd/keystone -count=1
GOCACHE=/tmp/keystone-ticket-05-go-cache go test ./... -count=1
GOCACHE=/tmp/keystone-ticket-05-go-cache go vet ./...
```

# 06-03：Worker Supervisor 与 Process Lifecycle

**What to build：** 在 Daemon readiness 之后启动并监管一个独立的 `keystone-worker` 进程，使 Worker 主动 Register、Heartbeat、Pull、Execute、Report，并在崩溃、心跳丢失和关闭时保持 Lease/AgentRun 权威边界。

**Blocked by：** 顶层 Ticket 05：Change Lifecycle、Artifact 与 Event；本地 Ticket 02：Assignment、Lease 与 Report Authority。Runtime 执行通过本规格约定的 `RuntimeAdapter` Port 注入，具体 Codex Adapter 由 06-04 并行交付。

**Status：** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 05` 未解除）

- [ ] Daemon 完成 LocalState、Migration、SQLite readiness 和 Worker protocol listener 后再启动一个 Worker；Worker health 与 `/healthz` 的 Daemon readiness 分离。
- [ ] Worker 可执行文件优先从 Daemon 同目录发现 `keystone-worker`，再从 PATH 发现；命令、进程、时钟、HTTP 和 backoff 都可注入测试 seam。
- [ ] 每次进程启动生成新的 WorkerInstance ID/secret，通过启动管道交付；旧 secret、旧身份和旧 Lease 不复用。
- [ ] Worker 主循环先 Register，成功后按默认 5 秒 Heartbeat，并以 Pull 获取 Assignment；Register 未成功时不能 Pull 或执行本地 Runtime。
- [ ] 一次只处理一个 Assignment；Execute 调用约定的 RuntimeAdapter，完成后把独立采集的结果提交给 Daemon Report Authority，不直接写 SQLite。
- [ ] 子进程异常退出、心跳失联或 Daemon 主动撤销时，活动 Lease 按 06-02 规则收敛，仍 running 的 AgentRun 形成 `worker_lost` failed，不自动 retry。
- [ ] 重启退避为 1、2、4 秒，最大 30 秒；反复失败不形成紧循环；每次重启都可观察到新的 WorkerInstance。
- [ ] Daemon 关闭时停止新的 Pull/Report 和 Assignment 分配，先请求 Worker 退出，在有界宽限期后终止，之后关闭 HTTP、SQLite、运行元数据和 InstanceLock。
- [ ] Worker 日志不包含 protocol secret、Lease token、Runtime 环境、绝对 Workspace 路径或原始凭据；日志只记录可关联的脱敏 ID/状态。
- [ ] 新增 `cmd/keystone-worker` 或目标 package 时先按根规约创建最近一级 `AGENTS.md`/`INDEX.md`，并更新受影响导航；本票不能把规划路径当作当前已存在的事实。

## Process topology

```text
keystone-daemon
  ├─ readiness / SQLite / Control Plane
  ├─ WorkerProtocol HTTP handler
  └─ WorkerSupervisor ── stdin/pipe secret ──> keystone-worker
                                          └─ HTTP client -> daemon /worker/v1/*
```

Worker 是独立进程，不是 Daemon package 中的 goroutine，也不是可以直接导入 Repository 的 library。它只持有当前 WorkerInstance、心跳状态、Assignment、Runtime 会话和原始输出；Daemon 才拥有 Lease、AgentRun、Change 和 Event。

## Startup and supervision

1. Daemon 已完成既有 readiness 后创建 Worker protocol listener，并为本次子进程生成 WorkerInstance ID 和 secret。
2. Supervisor 通过启动管道交付 secret；不能通过 argv、JSON 初始参数、通用环境变量或日志传递。
3. Worker 启动后 Register；Daemon 只在版本和 capability 通过后把它标记为可 Pull。Worker 未注册不改变 Daemon `/healthz` 的 SQLite readiness。
4. Supervisor 监管子进程退出、Register 超时、Heartbeat deadline 和活动 Assignment。Worker 丢失交给 06-02 收敛 Lease/AgentRun，不在 Supervisor 内重写生命周期。
5. 异常退出使用 1、2、4 秒退避，最大 30 秒。成功 Register 后重置退避；重启生成新身份，不恢复旧 Worker 的内存状态。
6. 监管状态只作为 Worker health/diagnostic fact 暴露；它不能成为 ChangeStatus、AgentRunOutcome 或 Verify approval。

## Worker loop

Worker loop 使用严格的顺序：Register → Heartbeat/Pull → 若 `assignment != null` 则本地校验 Workspace → 调用 RuntimeAdapter → 独立采集 evidence → Report → 回到 Heartbeat/Pull。无 Assignment 时按有界间隔 Poll，不 busy loop；收到 `unavailable` 时用同一 Report digest 重试，不重新执行 Runtime。

心跳续租和 Runtime timeout 由不同计时器负责：默认 Heartbeat 5 秒，Lease TTL 30 秒，Runtime 最长 30 分钟。Runtime 运行时 Worker 仍发送心跳；如果 Lease 已过期，Report 不能通过重试复活它。Runtime 超时只允许形成一次失败/不可用结果，不能自动启动第二个 Runtime。

Worker 的 local Execute 只接受 Daemon 已下发的 Assignment，不能接受 Client 直接请求；它不理解 Change 的生命周期转换，也不能生成 `StageAdvanced`、`ChangeHumanRequired`、`ChangeCancelled` 或 HumanDecision。

## Shutdown and recovery

- 关闭开始时 Supervisor 停止新的 Pull 和 Report 接收/分配，避免退出阶段产生新的 Assignment。
- Worker 收到有界退出信号后停止 Pull，等待当前本地操作在宽限期内结束并尝试提交已采集 Report；超过宽限期由 Daemon 终止进程，剩余 AgentRun 按 worker_lost 规则处理。
- Supervisor 关闭 Worker 后才释放 HTTP、SQLite、runtime metadata 和 InstanceLock，保持既有 Daemon 资源逆序。
- Daemon 重启后先恢复已提交的 authority，再撤销旧 WorkerInstance 的 active Lease；新的 Worker 不自动领取旧 Assignment，retry 仍需要 Ticket 05 HumanDecision。

## Acceptance

- Daemon readiness 完成后只启动一个 Worker；Worker 未注册/退出不会伪造 Daemon not-ready 或 AgentRun succeeded。
- Worker 进程可在真实 loopback 上完成 Register、Heartbeat、Pull、fake Execute 和 Report；Daemon 不需要给 Worker DB 连接。
- Worker 异常退出后按有限退避重启，旧 secret/Worker ID/Lease 被拒绝，活动 AgentRun 按 `worker_lost` 规则收敛且没有自动 retry。
- 关闭顺序能在 HTTP、SQLite、运行元数据和 InstanceLock 释放前停止 Worker，且不会泄漏子进程或秘钥。
- 进程、HTTP、时钟和 Runtime seam 的测试覆盖 Register 失败、心跳超时、Pull 空值、Report unavailable 重试、Runtime timeout、连续崩溃和 shutdown race。

## Verification

执行 Worker/Supervisor 集成测试、`go test ./...`、必要的 `go vet ./...`、`make build`；在 Linux/WSL 验证真实 sibling/PATH 进程发现，在原生 Windows 验证进程启动/关闭和 loopback protocol。交叉编译只能作为补充，不能替代原生 Windows 行为证据。

## Out of scope

- Lease/Report authority 和 Artifact transaction（06-02）。
- Codex 命令参数、Runtime stdout/stderr/diff 采集和 ExecutionGuard 具体实现（06-04）。
- Remote Worker/pool、MQ、TLS、完整身份系统、自动 retry、Worktree scheduler 和 Ticket 生产分配。

# 06-05：Ticket 06 Smoke、平台验证与导航

**What to build：** 在前四个本地子票和顶层 Ticket 05 完成后，使用临时 Git Repository、临时 LocalStateRoot、独立 Daemon/Worker 和真实本机 Codex 完成 M4 smoke；保存可复核的脱敏摘要，并把最终实现事实同步到受影响导航。

**Blocked by：** 顶层 Ticket 05；本地 Ticket 01、02、03、04。所有前置实现和测试必须在当前 checkout 中有可验证证据，不能以 `ready-for-agent`、规划规格或静态架构图代替。

**Status：** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 05` 未解除）

- [ ] 创建临时 LocalStateRoot 和真实临时 Git Repository，写入最小可追踪 fixture；不使用当前 checkout、当前分支或真实项目数据库。
- [ ] 通过内部受控测试/应用 seam 创建或授权一个有效 Project/Change/AgentRun fixture 和已存在的 Assigned Workspace；不新增公共“伪造执行成功” HTTP API，不把本票当作 Ticket 09 Worktree scheduler。
- [ ] 启动真实本机 Daemon、独立 Worker 和可用的 Codex CLI；Worker 通过 loopback WorkerProtocol Register/Heartbeat/Pull/Report 完成一次执行。
- [ ] Prompt 要求对 fixture 中的实际源码做确定性、可核验的小修改；Worker 独立采集 stdout、stderr、exit code、before/after revision、diff 和 changed files，Daemon 保存 Artifact、AgentRun 终态和 Trace。
- [ ] 使用相同终态 Report 重放，证明 response 为 `duplicate`、Lease/AgentRun/Event/ChangeVersion 不重复推进；使用失效/过期/旧 Worker Lease Report，证明不能改变权威状态并可按规则形成 LateReport Trace。
- [ ] 验证 Runtime 自报文本不是 authority，日志/exit code/evidence 来自 Worker/Daemon 观察；验证 Worker/Runtime 没有 Keystone SQLite 连接、Lifecycle/Verify/commit/push/merge 权限。
- [ ] 在 Linux 和 WSL 分别完成真实 Codex smoke；在原生 Windows 完成 protocol/process/Lease/fake Runtime 验证，若有 Codex 可额外运行但不能用交叉编译替代。
- [ ] 只有真实运行成功后才新增本目录 `06-smoke-evidence.md`；未运行或失败时记录阻塞/缺失事实，不创建空白成功证据。
- [ ] 根据当前实际文件树更新根 `INDEX.md`、`docs/FE20260903080401/tickets/INDEX.md`、受影响 package `AGENTS.md`/`INDEX.md` 和必要的根 `README.md`；不把设计输入或旧快照描述成已实现行为。
- [ ] 检查未触碰无关用户变更，例如现有 `.gitignore` 和 `docs/architecture-baseline/`；文档、源码和测试差异均通过卫生检查。

## Smoke fixture

Fixture 必须满足：

- Repository 位于临时目录，有一个已提交的基础 revision 和一份 Codex 能理解的最小源码文件。
- LocalStateRoot 位于临时目录，Daemon 使用独立 SQLite、Artifact Store、runtime metadata 和 InstanceLock。
- Assigned Workspace 是已存在、已验证的 Git root，绑定一个有效 AgentRun/attempt；本票不创建生产 Worktree，不让 Runtime 选择 Workspace。
- AgentRun/Assignment 的创建来自内部 Application/test seam，使用真实的权威 Repository/Artifact/Event 路径；不能通过公开调试接口写一条“已完成”记录。
- Prompt 是有界且可复核的本地修改请求，不要求访问网络、真实项目凭据或 Keystone DB。

建议 fixture 只修改一个 tracked 文件，便于确认 before/after revision、diff 和 changed files；如果执行产生额外文件，必须在 evidence 中明确列出相对路径和 Artifact digest。

## Verification flow

1. 记录 Daemon、Worker、Codex CLI 版本和验证命令版本；记录 ID 时只保存可复核摘要，不保存 secret/token、用户目录或绝对路径。
2. 确认 Daemon readiness，再观察 Worker Register 和 capability；Codex 不可用时应得到 `runtime_unavailable`，不能静默切换 OpenCode。
3. Pull Assignment，验证 lease_expires_at、runtime、before_revision、instruction 和输入摘要；确认 Worker 没有 DB 连接。
4. 观察真实 Codex 在 Assigned Workspace 中运行，分别取得 stdout/stderr、exit code、diff、changed files 和 after_revision。
5. 查询 AgentRun、Artifact/ArtifactLink、Change 快照和统一 Event Trace，确认唯一终态、Artifact 摘要、Event 顺序和 Change 推进/人审结果符合 Ticket 05。
6. 原样重发同一个终态 Report，确认 `duplicate` 且没有新增终态、StageAdvanced、ChangeVersion 或重复 ArtifactLink。
7. 生成已过期或旧 Worker/Lease 的 Report，确认 authority 不变；如内容通过安全校验，确认出现 `AgentRunReportLate` Trace 和 Late Artifact，而不是生命周期推进。
8. 让 Worker 异常退出或 Daemon 重启，确认活动 Lease/AgentRun 按 worker_lost/daemon_restarted 规则收敛，新的 WorkerInstance 使用新 secret，旧 Report 不复活 Lease。
9. 清理临时目录只在 evidence 摘要已经落盘、原始 Artifact 已按本地策略保留或明确失效后进行；不得把清理后的路径写成可访问证据。

## Evidence document

`06-smoke-evidence.md` 只保留以下摘要：

- Daemon/Worker/Codex 版本和平台。
- 临时 Project/Change/AgentRun/WorkerInstance 的非敏感标识。
- Assignment、首次 Report 和 replay 的 disposition；失效 Lease Report 的结果。
- exit code、before/after revision、changed files 相对路径摘要、stdout/stderr/diff/changed_files 的 digest/size/truncated。
- AgentRun 当前状态/outcome、Change stage/status/version、Event sequence/type 和 Trace 查询结果。
- `go test ./...`、必要的 `go vet ./...`、`make build`、真实 smoke 命令和平台验证结果。

原始 stdout/stderr/diff 留在本次 Local Artifact Store，不复制进提交文档。任何 secret、Lease token、完整 Prompt、Codex auth、绝对路径或用户名都必须脱敏/省略。文档必须单独列出未执行项、失败项和可能的残余风险。

## Platform matrix

| 平台 | 最低验收 | 不能替代的证据 |
| --- | --- | --- |
| Linux | 真实 Daemon/Worker/Codex、loopback、Git fixture、Artifact/Trace、duplicate/late | 不能只用单测 |
| WSL | 独立真实 Codex/进程/文件路径 smoke | 不能只引用 Linux 结果 |
| 原生 Windows | Worker protocol、子进程启动/关闭、路径/环境、Lease/Report、fake Runtime | 交叉编译不能替代；Ticket 02 的 native lock 证据仍需满足 |
| 交叉编译 | 目标可构建、平台依赖选择正确 | 不是运行时或 Codex 证据 |

## Navigation acceptance

完成后导航必须反映实际 checkout：

- 根 `INDEX.md` 标记 `cmd/keystone-worker`、`internal/worker`、`internal/execution` 的真实存在、入口和测试；不存在的文件不能提前列为已实现。
- `contracts/worker/INDEX.md` 只描述传输 DTO；若新增字段，明确哪些是当前实现行为、哪些仍是上层 authority。
- `internal/daemon/INDEX.md`/`AGENTS.md` 只在 Worker Supervisor/Protocol 真实落地后更新对应职责，不能把规划规格当作 HTTP/SQLite 事实。
- `docs/FE20260903080401/tickets/INDEX.md` 更新 Ticket 06 及其实施状态/验证链接，保留顶层依赖关系和 `BLOCKED_BY` 的真实状态。
- 根 `README.md` 仅在结构变化或用户入口真实存在时更新；与本票无关的历史描述漂移不顺手修复。

## Acceptance

- Linux 和 WSL 各有一条真实 Codex smoke 证据，包含实际源码修改、Worker 独立 evidence、Daemon authority 和 Trace 查询。
- duplicate Report 不重复终态/推进，失效或旧 Lease Report 不能修改 authority；Worker/Runtime 无直接 SQLite、Lifecycle、Verify approval 或 Git commit/push/merge 路径。
- 原生 Windows 至少完成 protocol/process/fake Runtime 证据，跨编译只作为补充。
- 证据文档可供复核且不泄漏 secret/绝对路径；未完成的测试和风险诚实列出。
- 导航文件来自完成后的实际 tree 和源码，不保留当前事实无法验证的“已实现”描述。

## Verification

执行完整 `go test ./...`、必要的 `go vet ./...`、`make build`、Ticket 06 smoke、平台矩阵和 `git diff --check`。如 smoke 依赖本机 Codex 认证/安装而失败，记录实际错误与可复现前提，不能把 fake Runtime、版本探针或交叉编译写成真实 Codex 成功。

## Out of scope

- 生产 Worktree、Ticket scheduler、Execution DAG、实际 Ticket 分配和 Golden Path 完整生命周期（Ticket 09/12）。
- Remote Worker/pool、MQ、TLS、OpenCode 实现、自动 retry、Dashboard 页面和完整 OS sandbox。
- 清理无关工作树变更、修正与 Ticket 06 无关的 README/AGENTS 历史漂移、删除或重置用户文件。

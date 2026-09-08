# 12 — Golden Path E2E Evidence

> 状态：验收契约已对齐，尚未实现或执行。`BLOCKED_BY: 11` 仍然有效；本文件不是成功证据，也不能把规划状态写成运行完成。
>
> 本文顶层 `BLOCKED_BY: 11` 是版本化 ImplementationTicket 的交付前置条件；它不同于一个 Change 的 Canonical Ticket Graph 中 `blocked_by` 表达的 CanonicalTicket 依赖，二者不可互推。

- 里程碑：M9
- `BLOCKED_BY`：11
- 交付类型：最终可复盘验收

## 目标

以真实 Git demo Repository 和真实 Codex CLI 完整跑通 V1 Golden Path，并产出足以审计的端到端验收证据。

## 范围

- 在仓库中保存不含 `.git` 的 Go HTTP demo fixture；fixture 可包含已版本化的 ProjectManifest V2，E2E 时复制到临时目录并执行 `git init`，形成真正独立的测试 Repository。
- demo 初始仅提供 `GET /` 返回 `hello`；创建的 Change 要求新增 `/healthz`，返回 HTTP 200、`application/json` 和 `{"status":"ok"}`，并新增自动化测试。
- 完整执行 `init → Project → Change → Intent → Understand → Design → Plan → Ticketize → Worktree → Codex → Diff → Verify → Keystone Commit → Final Verify → Integrate Ready → Dashboard Trace`。
- 收集每个阶段的 API/CLI 结果、AgentRun、Artifact、Event、revision、verify 输出和 Dashboard 观察证据。
- 将普通可重复测试与需真实 Codex 的显式本机 E2E 区分；所有 runtime-backed AgentRun 的 canonical 成功都必须使用真实 Codex，宣布 V1 完成必须包含 Linux 和 WSL 的成功证据。

## 不包含

- merge、push、PR、deploy、远程 Worker、远程 Repository 或长期运维。
- 用 mock Runtime 替代最终的真实 Codex 验收。
- 将嵌套 Git Repository 提交到 Keystone 主仓库。

## 验收条件

- demo candidate 满足下文全部六条 Golden Path Acceptance Criteria，原 `GET /` 行为保持不变。
- Canonical Ticket Graph 合法、Worktree 中存在真实 Diff、独立 Verifier PASS、Keystone Commit 与 candidate revision 均可查询。
- Change 到达 `integrate_ready`，但没有发生 merge、push、PR 或 deploy。
- Dashboard 可观察 Project、Change、Ticket Graph、Current Run、Artifacts、Events、Health 和最终 Trace。
- 一份端到端证据记录可复盘所有关键输入、命令、退出码、Artifact、AgentRun、Decision 和 Git revision。

## 已冻结 Demo Acceptance Criteria

Candidate revision 上的 demo 必须同时满足以下六条；它们是 Golden Path 的固定验收契约，不替代 CanonicalTicket 各自的 Acceptance Criteria：

1. `GET /` 返回 HTTP 200。
2. `GET /` 的响应体精确为 `hello`。
3. `GET /healthz` 返回 HTTP 200。
4. `GET /healthz` 响应的解析后 media type 为 `application/json`；允许附带 charset 参数。
5. `GET /healthz` 响应体解析后的 JSON 值精确为 `{"status":"ok"}`，不以 JSON 空白或字段序列差异判定失败。
6. Candidate Repository 至少有一项确定性自动化测试直接覆盖 `/healthz`，且 ProjectManifest V2 声明的 `go test ./...` 以零退出码通过。

## 已冻结 ChangeIntent

每次 GoldenPathRun 都必须将下列文本作为 ChangeIntent 原样提交；不得因平台、Run、Runtime 或当前 Graph 调整措辞、增加上下文或改写范围：

~~~text
在当前 Go HTTP demo 中，保持 `GET /` 的 HTTP 200 和 `hello` 响应不变。新增 `GET /healthz`，使其返回 HTTP 200、media type `application/json` 和 JSON 值 `{"status":"ok"}`。新增确定性自动化测试直接覆盖 `/healthz`，并确保 `go test ./...` 通过。除实现该 endpoint 及其测试所必需的变更外，不修改 `.keystone/project.yaml` 或其他项目配置。
~~~

该原文以 ChangeIntentArtifact 保存，是后续 Planning、Ticketize、Execute、Verify、Final Verify 与 Evidence 的共同输入；它不是 Runtime prompt、Canonical Ticket Graph 或可在失败后就地修改的说明。

## 验证

```bash
go test ./...
go vet ./...
make build
make dashboard-build
git diff --check
```

再执行显式真实 Codex Golden Path E2E，并保存完整验收记录。

## 实现边界

本 Ticket 的实现边界是 GoldenPathFixture、显式 GoldenPathRunner、验收 Evidence 和相关导航；不新增生命周期、Ticket Graph、Scheduler、Verify、Commit、Dashboard 业务行为，也不扩张 V1 到 Integrate Ready 之后的 Git 或部署操作。

## 已冻结 Fixture 与初始化

Fixture 路径固定为 testdata/golden-path-demo/，不包含 .git，包含：

~~~text
testdata/golden-path-demo/
├── .keystone/project.yaml
├── go.mod
├── main.go
└── main_test.go
~~~

Fixture 使用 Go 1.27、模块名 example.com/keystone-demo。初始服务只提供 GET / 返回 HTTP 200 和 hello，不预置 /healthz 实现或测试；可以保留原 / 行为测试。服务固定支持 `--listen ADDRESS`：Runner 传入 `127.0.0.1:0` 后，服务成功 bind 必须在 stdout 首行输出唯一 JSON 行 `{"event":"demo_listening","address":"127.0.0.1:PORT"}`；该启动接口不属于 ChangeIntent 的可修改范围。

.keystone/project.yaml 是已版本化的 ProjectManifest V2：

~~~yaml
version: 2
project_id: "018f0000-0000-7000-8000-000000000000"
verify:
  commands:
    - name: go-test
      argv: ["go", "test", "./..."]
      timeout_seconds: 900
~~~

canonical run 不配置 commit.template；自定义 template 由 Ticket 10 独立验收覆盖。Fixture 不含 secret、token、绝对路径、运行时状态或 Keystone DB。

Runner 复制 fixture 后，在临时 Repository 中执行 git init -b main、`git config --local user.name "Keystone Golden Path"`、`git config --local user.email "golden-path@invalid"`、`git config --local commit.gpgSign false`、git add . 和带 `--no-verify` 的初始 commit，记录干净的初始 HEAD 作为 BaseRevision。不得读写用户全局 Git config；该 local identity 由后续 Worktree 继承。keystone init 对 V2 Manifest 执行 Project reconcile；不得将临时 Repository 的 .git 纳入 Keystone 主仓库。

## 已冻结 Runner 契约

显式 runner 路径为 scripts/golden-path-e2e.sh，只在 Linux/WSL 手工调用，不被普通 go test ./... 自动触发。它必须：

- Runner 必须要求完整的 `--keystone-revision <Git-OID>`，拒绝以分支名、调用者当前 HEAD 或未提交工作树代替；操作者在 Ticket 11 真实验收后显式提供该 revision，Runner 不自行推断“已验收”。
- 从该 revision 以 `git archive` 形成 run-local Keystone 源码快照，再构建明确的 keystone、keystone-daemon、keystone-worker 二进制到临时 bin 目录，记录 Keystone revision、Go version 和二进制 digest，不依赖未知 PATH。
- 每次 GoldenPathRun 创建独立、受控的临时根，分别承载 demo Repository、二进制、LocalStateRoot 和安全摘要；Runner 必须显式选择该 LocalStateRoot，不能复用调用者默认的用户级数据根，且 Evidence 只记录脱敏标识和 digest，不记录临时根的绝对路径。
- Runner 直接以受控子进程启动 `<run>/bin/keystone-daemon --data-dir <run>/state`，不调用内部启动上限为 15 秒的 `keystone daemon start`；在 60 秒预算内等待 DaemonReadiness。
- Runner 只可读取自身 LocalStateRoot 的 RuntimeMetadata 中的 loopback endpoint 和 DaemonInstanceID，以建立第一条公开 API 连接；随后必须以 `GET /v1/daemon/status` 交叉校验同一 DaemonInstanceID 与 DaemonReadiness。RuntimeMetadata 不得被当成权威状态、Query 结果或 Evidence。
- Run 结束时先以公开 stop 请求关闭该 Daemon 并等待最多 30 秒；若 Runner 自己启动的 Daemon/Worker 子进程树仍未退出，只能终止该树，且必须保留 Repository、Workspace、Commit、LocalStateRoot 和日志文件。
- Project init 只能通过公开 `POST /v1/projects/init` 提交，并使用预先保存的 Idempotency-Key；不得调用会隐式生成该 key 的 `keystone init`。
- Change create、Execute、每张 Ticket 的 Verify/Commit 与 Final Verify 只能使用相应公开 CLI，且每次写入均显式传入 Idempotency-Key 与适用的 expected version；所有状态观察只使用公开 `GET /v1/...` Query。
- 每次公开写入前，Runner 必须在受控临时根原子保存 GoldenPathCommandLedger 条目，至少固定逻辑操作、Change/Ticket 范围、规范请求摘要、expected version、随机 UUIDv7 Idempotency-Key 与 key fingerprint。Ledger 不拥有 Daemon 权威状态。
- 只通过公开 CLI/API 提交 Command、读取 Query、观察 Daemon/Worker；不得直接调用内部 Service、写 SQLite 或使用 debug seam。
- 只有传输中断或公开 API 明确返回暂时 unavailable 时，Runner 才能重送同一 Ledger 条目；重送必须保持请求、expected version 与 Idempotency-Key 完全相同。收到 `202 Accepted` 后只能轮询，不得为同一逻辑 Command 生成新 key、刷新 version 后改写请求或创建第二个 Intent、AgentRun、Commit 或 Event。
- 收到 `409`、idempotency conflict、非法状态、human_required、真实 AgentRun 失败、缺失真实 Diff、候选不变量失败或预算耗尽时，整次 Run 立即失败并保留现场；Runner 不得自动重新查询后发起新的逻辑写入。
- Daemon 启动预算为 60 秒；Understand、Design、Plan、Ticketize、每张 Ticket Execute、每张 Ticket Verify 和 Final Verify 各为 30 分钟；完整 GoldenPathRun 总预算为 4 小时。
- 每项有界等待在前 30 秒每秒查询一次，此后每 5 秒查询一次；传输或 Query 暂时错误最多按 1 秒、2 秒、4 秒重试三次。pending 或无 Worker 仅在其阶段预算内等待；`FAIL`、`HUMAN_REQUIRED`、缺失真实 Diff、版本/契约冲突或候选不变量失败立即终止。
- ProjectManifest V2 中每条 VerificationCommand 的 timeout 保持其已冻结值；fixture 的 `go test ./...` 仍为 900 秒，Runner 不得延长它。
- Dashboard 浏览器观察只使用 lockfile 固定版本的 Playwright 及其对应 Chromium，在 Daemon 托管的 production build 上以 headless 模式执行；不得使用 Vite dev server、mock payload、前端状态伪造、手动关闭标签页或动态 `npx` 下载。
- 浏览器通过 Playwright 的 network offline → online 控制真实切断并恢复 EventSource；必须观察到断线、重连、断线后新的公开 Query 以及完整页面刷新后的新的公开 Query，并与 Daemon 权威响应一致。浏览器、driver 或 production build 缺失、版本不符或无法完成该链路时，平台 Run 失败并保留现场。
- 不预写源码、不人工修复 Codex 结果；Codex 未形成真实 Diff 时整次 GoldenPathRun 失败。
- 无论 Run 成功或失败，在独立只读复核完成前保留已形成的候选 Repository、Workspace、Commit 和 LocalStateRoot；不得自动 reset、clean、amend、rebase 或删除 Worktree。

## 已冻结公共驱动顺序

1. 复制 fixture、初始化 Git、创建 seed commit，并观察初始 DemoServiceHealthEndpoint。
2. 以预先保存的 Idempotency-Key 调用 `POST /v1/projects/init`，查询 Project 和 ProjectInitialized Event。
3. 通过 `keystone change create` 提交上文已冻结的 ChangeIntent；Understand、Design、Plan 和 Ticketize 由生命周期自动触发。不得调用 planning start。
4. 查询 `GET /v1/changes/{change_id}/ticket-graph`，读取实际 Canonical Ticket Graph。
5. 使用默认 branch 提交 keystone change execute CHANGE_ID --expected-version N --idempotency-key KEY，不指定 --branch。
6. 依 canonical order 观察实际 Ticket 的 Execute、Diff 和 TicketExecutionEvidence；不写死 Ticket 数量或 ID。
7. 依 canonical order 通过 `keystone change verify` 与 `keystone change commit` 驱动每张 Ticket；仅在独立 Verifier PASS、Keystone Commit 和 before/after revision 收敛后，才观察下一张 Ticket。
8. 提交 keystone change final-verify，等待独立 Final Verify PASS、CandidateRevision 和 integrate_ready。
9. 在 candidate revision 上启动 Demo service，以 loopback 临时端口检查初始/最终 GET / 和最终 GET /healthz。
10. 使用 Daemon 托管的 production Dashboard，在 lockfile 固定的 headless Playwright/Chromium 中观察 `/`、`/projects/{project_id}`、`/changes/{change_id}` 和 `/needs-human`；以 network offline → online 让真实 SSE 连接受控断开并恢复后确认页面重新 Query，浏览器完整刷新后同样确认 Query API 可重建状态。不得只模拟 UI 状态。

所有 runtime-backed AgentRun（Understand、Design、Plan、Ticketize、Execute、独立 Verify、Final Verify）使用真实 Codex。Codex Adapter 继续使用：

~~~text
codex exec --json --ephemeral --sandbox workspace-write --ask-for-approval never -
~~~

Planning candidate 使用 read-only sandbox。禁止 interactive mode、危险绕过参数、OpenCode fallback、自由 shell 或 fake Runtime 作为 canonical 成功证据。Verify/Final Verify 的 go test ./... 是确定性 VerificationCommand，不替代独立 VerifierAgentRun。

## 已冻结 Demo Service 检查

Runner 分别以 BaseRevision 与 CandidateRevision 从 demo Repository 执行 `git archive`，形成两个 run-local 构建副本；不得读取 Keystone Workspace 路径或在任一副本中修复源码。每个副本以 `go build` 生成独立服务二进制，并传入 `--listen 127.0.0.1:0`。Runner 最多等待 30 秒取得首行启动 JSON 后才开始 HTTP 检查；初始副本只检查 `GET /`，Candidate 副本检查 `GET /` 与全部 `/healthz` 标准。

每个 demo 服务进程结束时先发送 SIGTERM 并等待最多 10 秒；必要时只能终止 Runner 自己的服务子进程树。HTTP 结果、启动 JSON、退出状态和服务日志保留在受控临时根，并以安全摘要和 digest 进入 review packet；不得记录临时绝对路径。

## 已冻结证据记录

一次 Run 完成全部自动与浏览器观察步骤后，Runner 只能在受控临时根创建脱敏 review packet；它不是成功 Evidence。packet 至少包含浏览器引擎/版本、Playwright 版本、headless 模式、production build digest、脱敏 origin、四个页面 URL 与深链接刷新结果、初次和重建 Query 的安全摘要、SSE 建连与安全 refresh payload、offline/online 方法和时刻、重连与新 Query 序列、每次 Query 与 Daemon 权威响应的一致性摘要，以及 packet 自身 digest。独立审阅者在不修改 Daemon、fixture、candidate Repository、Workspace 或 LocalStateRoot 的前提下，只读核对固定 Keystone revision、CandidateRevision、Artifact/digest、命令/退出码、平台来源和浏览器记录，并给出 PASS、FAIL 或 UNVERIFIED。

只有 GoldenPathReview 为 PASS 后，才可在以下文件追加该平台的成功记录：

~~~text
docs/FE20260903080401/tickets/12-golden-path-e2e-evidence.md
~~~

每个平台独立记录一次 GoldenPathRun，至少包含平台、运行编号、Keystone/Go/Git/Codex 版本、二进制 digest、阶段输入、公开命令/API、响应和退出码、GoldenPathCommandLedger key fingerprint、轮询/超时结果、Project/Change/AgentRun/Artifact/Event/Decision、BaseRevision、WorkspaceInputRevision、TicketDelta、Diff、Commit 链、CandidateRevision、六条 Demo Acceptance Criteria、Dashboard 截图/API 快照、GoldenPathReview 结论和脱敏说明。

Evidence 分为三条不可互相替代的链：

| 证据链 | 内容 |
| --- | --- |
| Keystone Source | go test ./...、go vet ./...、make build、make dashboard-build、git diff --check |
| Demo Candidate | fixture seed、BaseRevision、初始/最终 HTTP、源码 Diff、go test ./...、Git revision |
| GoldenPath Trace | 全部阶段 API/CLI、AgentRun、Artifact、Event、Decision、Verify、Commit、Dashboard 和最终状态 |

Evidence 不得包含 secret、token、本机绝对路径、完整敏感 Prompt 或未脱敏环境。正式记录以 `<demo-repo>`、`<workspace>`、`<local-state>` 等别名和 digest 替代路径；截图与 API 快照必须先做安全投影或遮盖。原始日志和未脱敏材料保留在 Artifact 或本机受控位置，Markdown 只保存安全摘要、Artifact ID 和 digest。失败、未执行、Codex 不可用或 GoldenPathReview 为 FAIL/UNVERIFIED 时不得写成成功 Evidence。

失败 Run 只能在其受控临时根保留脱敏失败摘要，不得创建或追加平台成功 Evidence；独立只读复核后才可由人工清理该 Run 的现场。

## 已冻结平台门槛

非 WSL Linux 与 WSL 各需一条独立真实 Codex GoldenPathRun，不能互相引用。Linux 证据必须来自独立运行的非 WSL Linux 主机或 VM，并以脱敏的平台指纹和实际命令证明来源；WSL 内的容器、交叉编译或另一平台记录都不能替代它。原生 Windows 继续使用 Ticket 06/09/10 的原生 protocol/process/Git/fake 或真实证据，不新增为 Ticket 12 的第三条 Run。

只有 Ticket 10/11 前置证据、两个平台真实 Codex、六条 Demo Acceptance Criteria、Graph/Diff/Verifier/Commit/CandidateRevision/integrate_ready、Dashboard production 浏览器观察和脱敏 Evidence 全部成立，并经过一次独立只读复核，才可宣布 V1 Thin Vertical Slice 完成。不得发生 merge、push、PR、deploy 或其他远程副作用。

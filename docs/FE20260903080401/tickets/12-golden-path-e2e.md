# 12 — Golden Path E2E Evidence

> 状态：验收契约已对齐，尚未实现或执行。`BLOCKED_BY: 11` 仍然有效；本文件不是成功证据，也不能把规划状态写成运行完成。

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

- demo 的 `/healthz` 满足全部六条 Golden Path Acceptance Criteria，原 `GET /` 行为保持不变。
- Canonical Ticket Graph 合法、Worktree 中存在真实 Diff、独立 Verifier PASS、Keystone Commit 与 candidate revision 均可查询。
- Change 到达 `integrate_ready`，但没有发生 merge、push、PR 或 deploy。
- Dashboard 可观察 Project、Change、Ticket Graph、Current Run、Artifacts、Events、Health 和最终 Trace。
- 一份端到端证据记录可复盘所有关键输入、命令、退出码、Artifact、AgentRun、Decision 和 Git revision。

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

Fixture 使用 Go 1.27、模块名 example.com/keystone-demo。初始服务只提供 GET / 返回 HTTP 200 和 hello，不预置 /healthz 实现或测试；可以保留原 / 行为测试。

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

Runner 复制 fixture 后，在临时 Repository 中执行 git init -b main、仅本地的 Git identity、git add . 和初始 commit，记录干净的初始 HEAD 作为 BaseRevision。keystone init 对 V2 Manifest 执行 Project reconcile；不得将临时 Repository 的 .git 纳入 Keystone 主仓库。

## 已冻结 Runner 契约

显式 runner 路径为 scripts/golden-path-e2e.sh，只在 Linux/WSL 手工调用，不被普通 go test ./... 自动触发。它必须：

- 从已验收的 Keystone revision 构建明确的 keystone、keystone-daemon、keystone-worker 二进制到临时 bin 目录，记录 Keystone revision、Go version 和二进制 digest，不依赖未知 PATH。
- 只通过公开 CLI/API 提交 Command、读取 Query、观察 Daemon/Worker；不得直接调用内部 Service、写 SQLite 或使用 debug seam。
- 对异步 Stage、AgentRun、VerificationIntent 和 Dashboard 使用有界轮询；查询网络失败可以重试，但不得创建新的业务事实。
- 同一逻辑 Command 的重送复用原 Idempotency-Key；不得因 202 Accepted 或轮询创建第二个 Intent、AgentRun、Commit 或 Event。
- 不预写源码、不人工修复 Codex 结果；Codex 未形成真实 Diff 时整次 GoldenPathRun 失败。
- 在独立只读复核完成前保留候选 Repository、Workspace、Commit 和 LocalStateRoot；不得自动 reset、clean、amend、rebase 或删除 Worktree。

## 已冻结公共驱动顺序

1. 复制 fixture、初始化 Git、创建 seed commit，并观察初始 DemoServiceHealthEndpoint。
2. 执行 keystone init，查询 Project 和 ProjectInitialized Event。
3. 通过 keystone change create 或 POST /v1/changes 提交固定 Intent；Understand、Design、Plan 和 Ticketize 由生命周期自动触发。不得调用 planning start。
4. 查询 GET /v1/changes/{change_id}/ticket-graph 或 keystone change ticket-graph CHANGE_ID，读取实际 Canonical Ticket Graph。
5. 使用默认 branch 提交 keystone change execute CHANGE_ID --expected-version N --idempotency-key KEY，不指定 --branch。
6. 依 canonical order 观察实际 Ticket 的 Execute、Diff 和 TicketExecutionEvidence；不写死 Ticket 数量或 ID。
7. 每张 Ticket 的 Verify、独立 Verifier PASS、Keystone Commit 和 before/after revision 收敛后，才观察下一张 Ticket。
8. 提交 keystone change final-verify，等待独立 Final Verify PASS、CandidateRevision 和 integrate_ready。
9. 在 candidate revision 上启动 Demo service，以 loopback 临时端口检查初始/最终 GET / 和最终 GET /healthz。
10. 使用 Daemon 托管的 production Dashboard 浏览器观察 /projects、/projects/{project_id}、/changes/{change_id} 和 /needs-human；刷新或模拟 SSE 断线后确认 Query API 可重建状态。

所有 runtime-backed AgentRun（Understand、Design、Plan、Ticketize、Execute、独立 Verify、Final Verify）使用真实 Codex。Codex Adapter 继续使用：

~~~text
codex exec --json --ephemeral --sandbox workspace-write --ask-for-approval never -
~~~

Planning candidate 使用 read-only sandbox。禁止 interactive mode、危险绕过参数、OpenCode fallback、自由 shell 或 fake Runtime 作为 canonical 成功证据。Verify/Final Verify 的 go test ./... 是确定性 VerificationCommand，不替代独立 VerifierAgentRun。

## 已冻结证据记录

成功后才创建：

~~~text
docs/FE20260903080401/tickets/12-golden-path-e2e-evidence.md
~~~

每个平台独立记录一次 GoldenPathRun，至少包含平台、运行编号、Keystone/Go/Git/Codex 版本、二进制 digest、阶段输入、公开命令/API、响应和退出码、Project/Change/AgentRun/Artifact/Event/Decision、BaseRevision、WorkspaceInputRevision、TicketDelta、Diff、Commit 链、CandidateRevision、六条 Demo Acceptance Criteria、Dashboard 截图/API 快照和脱敏说明。

Evidence 分为三条不可互相替代的链：

| 证据链 | 内容 |
| --- | --- |
| Keystone Source | go test ./...、go vet ./...、make build、make dashboard-build、git diff --check |
| Demo Candidate | fixture seed、BaseRevision、初始/最终 HTTP、源码 Diff、go test ./...、Git revision |
| GoldenPath Trace | 全部阶段 API/CLI、AgentRun、Artifact、Event、Decision、Verify、Commit、Dashboard 和最终状态 |

Evidence 不得包含 secret、token、本机绝对路径、完整敏感 Prompt 或未脱敏环境。原始日志保留在 Artifact 或本机受控位置，Markdown 只保存安全摘要、Artifact ID 和 digest。失败、未执行或 Codex 不可用时不得写成成功 Evidence。

## 已冻结平台门槛

Linux 和 WSL 各需一条独立真实 Codex GoldenPathRun，不能互相引用。原生 Windows 继续使用 Ticket 06/09/10 的原生 protocol/process/Git/fake 或真实证据；交叉编译不替代原生运行。

只有 Ticket 10/11 前置证据、两个平台真实 Codex、六条 Demo Acceptance Criteria、Graph/Diff/Verifier/Commit/CandidateRevision/integrate_ready、Dashboard production 浏览器观察和脱敏 Evidence 全部成立，并经过一次独立只读复核，才可宣布 V1 Thin Vertical Slice 完成。不得发生 merge、push、PR、deploy 或其他远程副作用。

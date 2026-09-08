# 12：Golden Path E2E Evidence 实施规格

> 状态：规划规格已对齐，尚未实现或执行。`BLOCKED_BY: 11` 是硬阻塞；本规格不把 Ticket 09–11 的规划、fake 测试、构建成功或本机版本探针写成 Golden Path 成功证据。
>
> 关联：[Ticket 12](../../12-golden-path-e2e.md)。该主 Ticket 是固定 ChangeIntent、Demo Acceptance Criteria、预算和成功 Evidence 发布规则的规范来源。

## Problem Statement

作为 Keystone V1 的交付者，我需要能够证明一次真实 Change 从独立 Git demo Repository 的初始化，经过 Understand、Design、Plan、Ticketize、Worktree、真实 Codex、Diff、Verify、Keystone Commit、Final Verify 与 Dashboard Trace，最终抵达 `integrate_ready`。当前的单元测试、规划文档、`ready-for-agent` 状态、Daemon readiness、Worker capability 自报、Codex 版本探针或交叉编译都不能替代这条运行时证据链。

这条验收还必须避免两个相反的风险：一是为了证明成功而让 Runner 绕过 Control Plane、直接写 SQLite、重写源码或重试出第二份业务事实；二是把本机临时路径、认证信息、完整 Prompt、环境和原始日志写入可提交的 Markdown。Linux 与 WSL 必须各自形成同源、独立、经审阅的真实 Codex 记录，不能把一个平台的结果移作另一个平台的证据。

## Solution

实现一个显式、手工触发的 `GoldenPathRunner`，以固定 Keystone revision 的 archive 为唯一源码输入，为每次 `GoldenPathRun` 创建独立的受控根目录、demo Repository、LocalStateRoot、二进制和复核材料。Runner 只通过公开 CLI、Control Plane API 与公开 Query 驱动完整生命周期；其唯一窄例外是读取自身 RuntimeMetadata 以发现 loopback Daemon endpoint，随后必须通过公开 Daemon status Query 交叉验证。

Runner 使用固定的 Go HTTP demo fixture 与不可变 ChangeIntent，要求真实 Codex 在 Assigned Workspace 中形成 Diff。它把公开写入预先固定到 `GoldenPathCommandLedger`，在限定预算内轮询真实权威状态，并对错误、冲突、预算耗尽、缺失 Diff 或候选不变量失败立即保留现场。候选 revision 通过独立服务构建、HTTP 检查、确定性测试以及生产 Dashboard 的 headless Playwright/Chromium 观察后，形成脱敏 `GoldenPathReviewPacket`。

独立审阅者只能只读核验固定输入、运行轨迹、平台来源和 packet 完整性。只有审阅结论为 PASS 时，人工才可将该平台的成功记录追加为 `GoldenPathEvidence`；Runner 从不自行发布成功记录。两个平台只有共享同一 `GoldenPathEvidenceSetID` 时才可共同支持 V1 Thin Vertical Slice 的结论。

## User Stories

1. 作为 Keystone V1 交付者，我希望以一个固定、已接受的 Keystone revision 运行 Golden Path，以便当前分支、未提交工作或后续提交不能改变验收对象。
2. 作为运行操作者，我希望每次验收都有新的 `GoldenPathRunID` 和独立受控根，以便不同尝试的 Repository、LocalStateRoot、日志和候选不会相互污染。
3. 作为项目维护者，我希望 demo fixture 不携带嵌套 Git 历史、secret 或运行状态，以便每次验收都从同一干净起点形成真实临时 Repository。
4. 作为 Change 发起者，我希望每个平台提交完全相同的固定 ChangeIntent，以便 Planning、Ticketize、Execute、Verify 与 Evidence 可以共享同一语义输入。
5. 作为审计者，我希望六条 `DemoAcceptanceCriteria` 具有固定、可自动判断的语义，以便原 `GET /` 行为、`/healthz`、JSON 值和自动化测试都不能被模糊的成功摘要替代。
6. 作为安全操作者，我希望 Runner 只使用本地 Git config 和受控数据根，以便不会读取或修改用户全局 Git 配置、默认 Keystone 数据根或既有 Repository。
7. 作为 Codex 使用者，我希望在创建任何 Daemon、Project 或 Command 前获得有界的外层 Codex 预检，以便明显缺失、版本异常或认证不可判定时不会产生半截业务事实。
8. 作为系统维护者，我希望 Codex 预检通过不被误认为 `RealCodexAcceptance`，以便只有真实 runtime-backed `AgentRun` 的 Report、Artifact、exit code 和公开 Query 才能证明运行时行为。
9. 作为 Control Plane 审计者，我希望所有公开写入在发起前固定请求、expected version 和 Idempotency-Key，以便传输中断只能安全重送同一逻辑 Command。
10. 作为恢复操作者，我希望 `202 Accepted` 后只轮询、冲突或非法状态立即停止，以便 Runner 不会通过新 key、刷新 version 或第二个 Intent 制造重复事实。
11. 作为生命周期观察者，我希望 Runner 按实际 Canonical Ticket Graph 的 canonical order 驱动 Execute、Verify 和 Commit，以便不把预设 Ticket 数量、ID 或前沿猜测写进验收。
12. 作为质量负责人，我希望每张 Ticket 的 Verify PASS、Keystone Commit 和 revision 收敛成为下一张 Ticket 的前置条件，以便 `CandidateRevision` 有连续、可审计的因果链。
13. 作为 demo 服务维护者，我希望 BaseRevision 与 CandidateRevision 分别从独立 archive 构建并经 loopback HTTP 检查，以便 Keystone Workspace 路径或人工修复无法伪造最终服务行为。
14. 作为 Dashboard 使用者，我希望在 Daemon 托管的 production build 中观察页面、真实 Query、SSE 断线重连与浏览器刷新，以便不会把 Vite dev server、mock payload 或客户端缓存当作运行证据。
15. 作为浏览器环境维护者，我希望 Playwright 与 Chromium 由 lockfile 固定并在 Run 前显式准备，以便动态 `npx` 获取、浏览器漂移或缺失不能产生不可复现的结果。
16. 作为安全审阅者，我希望 terminal 成功和失败 Run 都有脱敏、完整性可校验的 review packet，以便失败现场可复核而不会被自动清理。
17. 作为独立审阅者，我希望 review 结果绑定 `GoldenPathRunID`、`GoldenPathEvidenceSetID` 和 packet digest，以便 PASS、FAIL 或 UNVERIFIED 不能被移用到另一份运行材料。
18. 作为 Evidence 发布者，我希望成功记录只能在独立 PASS 后人工追加，并保留历史、撤销和替代关系，以便 Markdown 不成为 Runner 自证成功的出口。
19. 作为跨平台验收负责人，我希望 Linux 与 WSL 各有独立 Run，并共享相同 `GoldenPathEvidenceSetID`，以便不同源码、fixture、intent 或 runner 版本不能被拼成 V1 成功。
20. 作为 Linux 平台操作者，我希望来源探针明确拒绝 WSL 或 container 信号，以便 WSL 内容器、交叉编译或其他平台记录不能冒充非 WSL Linux 主机或 VM。
21. 作为隐私负责人，我希望 Evidence 使用别名、摘要和安全投影，以便本机绝对路径、hostname、用户名、IP、machine ID、token、认证原文、完整环境和敏感 Prompt 不会进入可提交文档。
22. 作为故障恢复者，我希望失败 Run 在独立只读复核前完整保留，以便无法用 reset、clean、amend、rebase 或删除 Worktree 抹去未知状态。
23. 作为后续实现者，我希望本规格不解除上游阻塞，以便 Ticket 12 不越界重建 Ticket Graph、Scheduler、Verify、Commit 或 Dashboard 业务行为。

## Implementation Decisions

### 上游条件、边界与术语

- Ticket 12 只交付 GoldenPathFixture、GoldenPathRunner、review/Evidence 产物及相关导航。它不实现 Ticket 09 的 Worktree/Execute/Diff、Ticket 10 的 Verify/Commit/FinalVerify 或 Ticket 11 的 Dashboard/Query/SSE 业务能力。
- 实现开始前必须重新确认 Ticket 10 和 Ticket 11 已有真实 acceptance evidence。规划文档、`ready-for-agent`、构建或 fake seam 不能解除 `BLOCKED_BY: 11`。
- 术语以根 `CONTEXT.md` 为准，尤其是 `GoldenPathFixture`、`GoldenPathKeystoneRevision`、`GoldenPathRun`、`GoldenPathRunID`、`GoldenPathEvidenceSet`、`GoldenPathCommandLedger`、`GoldenPathCodexPreflight`、`GoldenPathBrowserObservation`、`GoldenPathPlatformProvenance`、`GoldenPathReviewPacket`、`GoldenPathReview`、`GoldenPathReviewID`、`GoldenPathEvidence` 与 `RealCodexAcceptance`。
- 一个 `GoldenPathRun` 不是单元测试、局部 smoke、fake Runtime 或单一 AgentRun。它只能由公开 Command/Query 形成完整链路，且不拥有 Daemon 的业务状态、恢复决策或权威账本。

### Fixture、固定输入与 Demo 语义

- 版本化 fixture 是不含 `.git` 的 Go 1.27 HTTP demo；每次 Run 复制它后创建独立 Git Repository，使用固定 local identity、禁用本地 GPG signing，并以无 hook 的 seed commit 固定 `BaseRevision`。Runner 不得读写用户 global Git config。
- fixture 的 ProjectManifest 是严格 V2：包含固定 Project identity、单一 `go test ./...` VerificationCommand、900 秒 timeout，且不配置 commit template。不得将其降级为 V1 或让候选 Workspace 改写验证策略。
- 初始 demo 仅提供 HTTP 200 的 `GET /` 与精确 `hello` 响应；不得预置 `/healthz` 实现或其测试。服务必须接受 loopback listen address，并在成功 bind 后以唯一首行 JSON 宣告实际地址；该启动协议不属于 ChangeIntent 可修改范围。
- 每次 Run 原样提交主 Ticket 冻结的 ChangeIntent。该文本的 digest 是 `GoldenPathEvidenceSet` 输入，并作为 ChangeIntentArtifact 的共同语义输入；它不是 Runtime prompt、可变补充说明或 Canonical Ticket Graph 本身。
- Candidate 必须同时满足六条固定 `DemoAcceptanceCriteria`：保留 `GET /` 的 HTTP 200 和精确 `hello`；提供 HTTP 200 的 `/healthz`；其解析后 media type 为 `application/json`（允许 charset）；其 JSON 值语义等于 `{"status":"ok"}`；并具有直接覆盖 `/healthz` 的确定性测试及 V2 声明的零退出码 `go test ./...`。

### Run 身份、不可变输入与受控执行环境

- Runner 要求完整 Keystone Git OID、一个此前不存在的 run root、平台声明、Codex executable 和预备浏览器目录。Linux 运行还要求 `host` 或 `vm` 平台声明。分支名、调用者 HEAD、隐式默认路径或脏工作树都不能替代这些输入。
- 入口只可校验参数、固定 revision 和调用者工作树清洁性；随后必须从该 revision 的 archive re-exec canonical Runner。运行逻辑、fixture、Dashboard lockfile 与 runner 本身因此来自同一不可变源码快照。
- Runner 在任何 Codex 预检、Daemon 启动或公开写入前，为该受控根生成 UUIDv7 `GoldenPathRunID` 和规范化输入 manifest。manifest 的 SHA-256 是 `GoldenPathEvidenceSetID`，至少绑定完整 Keystone revision、fixture tree digest、固定 ChangeIntent digest、Runner script digest 与 Dashboard lockfile digest。
- 所有可执行 Keystone Source 检查和二进制构建以 archive 快照为输入；调用者工作树仅可只读验证 revision 与清洁性，不能提供实际二进制、fixture 或 Dashboard 资产。Go、Git、Codex 与浏览器运行时版本允许按平台不同，但必须记录。
- run root 承载 demo Repository、Keystone source snapshot、二进制、LocalStateRoot、run manifest、审阅材料和安全摘要。不得复用用户默认 Keystone 数据根；正式 Evidence 只能保存别名与 digest。

### Codex 与浏览器前置条件

- `GoldenPathCodexPreflight` 在启动 Daemon、创建 Project 或写入 CommandLedger 前解析一个指定 Codex executable，并在 30 秒内以非交互方式完成 version 与 login-status 检查。Runner 将同一解析结果置于 Daemon/Worker 的受控继承 PATH 前端。
- 预检只保存版本、结果类别、退出码、stdout/stderr digest 和 PATH digest；不保存认证原文、token、完整环境或绝对路径。缺失 binary、版本失败、认证失败或认证不可判定分别形成明确的 runner-local terminal 分类，保留现场且不创建业务链。
- `GoldenPathCodexPreflight` 成功不构成 `RealCodexAcceptance`。真实 AgentRun 后的失败只能按 Daemon 的 Report、Artifact、exit code 和公开 Query 记录；不得改写为预检分类、伪造 Worker Report 或改用 OpenCode/fake Runtime。
- Playwright 是 Dashboard 的固定 devDependency，浏览器版本由 lockfile 对应 Chromium 固定。操作者在 canonical Run 前只能以本地已锁定的 Playwright CLI 显式准备浏览器目录；Runner 只验证并使用该目录，不经 `npx` 或隐式下载取得 package 或浏览器。

### 单一公开驱动 seam 与命令恢复

- 最高层且唯一的完整验收 seam 是 `GoldenPathRunner`：它仅经公开 Control Plane API、CLI Command 和公开 `GET /v1/...` Query 驱动与观察系统。它不直接调用内部 Application Service、写 SQLite、使用 debug seam 或从 Worker 内部状态推导业务结论。
- Runner 只读自身 RuntimeMetadata 的 loopback endpoint 和 DaemonInstanceID，以建立第一条 API 连接；随后立即以公开 Daemon status Query 交叉验证同一实例及 DaemonReadiness。RuntimeMetadata 永远不是权威 Query 或 Evidence。
- Project init 使用显式、预存的 Idempotency-Key 提交公开 Project init API；不得使用会隐藏生成 key 的快捷 CLI。Change create、Execute、每张 Ticket 的 Verify/Commit 和 Final Verify 都使用对应公开 CLI，且明确携带适用的 expected version 与 Idempotency-Key。
- 每次公开写入之前原子持久化 `GoldenPathCommandLedger` 条目，固定逻辑操作、Change/Ticket scope、规范请求 digest、expected version、UUIDv7 key 与 key fingerprint。Ledger 只限制 Runner 重送，不替代 Daemon 的 CommandReceipt 或权威状态。
- 只有传输中断或 API 明确暂时 unavailable 时才允许重送，并且请求、expected version、Idempotency-Key 必须完全相同。收到 `202 Accepted` 后只能轮询；不得刷新 version 改写请求、生成新 key 或创建第二个 Intent、AgentRun、Commit 或 Event。
- `409`、idempotency conflict、非法状态、`human_required`、真实 AgentRun 失败、缺失真实 Diff、候选不变量失败或预算耗尽均立即终止该 Run。Runner 不自动发起新的逻辑写入或补救性源码修改。

### 生命周期、预算与候选服务验证

- 公共驱动顺序固定为：fixture/seed 与初始 demo 观察；Project init；固定 ChangeIntent create；Ticket Graph Query；Execute；按实际 canonical order 的 Diff/Evidence 观察；逐 Ticket Verify/Commit；Final Verify；CandidateRevision demo 服务检查；production Dashboard 浏览器观察。
- Runner 不写死 Ticket 数量或 ID。仅在每张 Ticket 的独立 Verifier PASS、Keystone Commit 和 before/after revision 收敛后才继续下一张；Final Verify PASS、CandidateRevision 与 `integrate_ready` 必须同时由公开 Query 观察。
- Daemon 启动预算为 60 秒；Understand、Design、Plan、Ticketize、每个 Execute、每个 Verify 和 Final Verify 各为 30 分钟；完整 Run 上限为 4 小时。前 30 秒每秒轮询，之后每 5 秒轮询；暂时 Query 错误最多以 1、2、4 秒重试三次。fixture 的 900 秒 VerificationCommand timeout 不得延长。
- BaseRevision 与 CandidateRevision 分别从 demo Repository archive 到独立构建副本，不能读取 Keystone Workspace 或在副本中修复源码。服务以 loopback 临时端口启动，首行启动协议最多等待 30 秒；Base 只检查 `GET /`，Candidate 检查全部六条标准。服务先接收 SIGTERM，最多等待 10 秒后才可终止 Runner 自己的子进程树。

### Dashboard、review packet 与成功 Evidence

- 浏览器观察只在 Daemon 托管的 production Dashboard 上以 headless Playwright/Chromium 完成。它覆盖 Projects、Project Detail、Change Detail、Needs Human 四个深链接，以及真实 EventSource offline-to-online 断线、重连、断线后新 Query 和完整刷新后新 Query；每次 Query 都与 Daemon 权威响应交叉核对。
- 每个 terminal Run（包括预检失败和中途失败）在受控根形成脱敏 `GoldenPathReviewPacket`。其规范 manifest 逐项列出材料 role、media type、byte length 和 SHA-256；manifest SHA-256 是 packet digest。packet 同时绑定 RunID、EvidenceSetID、终态、最后 checkpoint、失败分类（如有）和退出码；成功 packet 还必须包含浏览器与 Dashboard 观察的安全摘要。
- 独立审阅者不得是该 Run 的执行者或 Runner，且不得修改 Daemon、fixture、candidate Repository、Workspace 或 LocalStateRoot。审阅创建 UUIDv7 `GoldenPathReviewID`，将非敏感角色、独立性声明、RunID、EvidenceSetID、packet digest 和 PASS/FAIL/UNVERIFIED 结论固定关联。
- Runner 不得创建、修改或追加成功 Evidence。只有 PASS 后人工才可追加一个按平台和 RunID 命名的成功记录；该记录必须绑定 EvidenceSetID、ReviewID 与 packet digest。纠错、撤销或替代通过追加引用原 RunID 的新记录表达，不得静默重写既有成功记录。

### 平台门槛、隐私与保留策略

- 非 WSL Linux 和 WSL 各需要一条独立、真实 Codex 的 PASS Run。两者只有 `GoldenPathEvidenceSetID` 相同才可共同支持 V1 结论；同一 Run、交叉编译、WSL 内 container 或另一平台 Evidence 都不能复用。
- `GoldenPathPlatformProvenance` 只保留规范化 uname、os-release、WSL marker、container marker、平台声明与安全命令投影。WSL 声明必须观察到 WSL marker；Linux 声明必须没有 WSL/container marker 并包含 host/VM 声明。不匹配、container 信号或无法判断时以 `platform_provenance_unverified` 终止，不能取得 PASS。
- 平台指纹是可复核的来源声明，不是密码学远程证明。它不得包含 hostname、用户名、IP、machine ID、绝对路径、token、认证原文、完整环境、完整 Prompt 或未脱敏原始日志。
- 无论成功还是失败，candidate Repository、Workspace、Commit、LocalStateRoot、日志和复核材料都保留到独立只读复核完成。Runner 不得自动 reset、clean、amend、rebase、删除 Worktree 或清理现场；失败现场之后只能由人工清理。

## Testing Decisions

- `GoldenPathRunner` 是唯一最高层的完整验收 seam。好的测试只断言公开 Command、公开 Query、真实 Git/demo HTTP 行为、受限浏览器行为和可复核 Evidence，而不检查 Runner 的内部函数分解、临时目录命名、实现语言或数据库内部查询。
- fixture 级测试验证外部合同：无嵌套 Git、严格 V2 Manifest、初始无 `/healthz`、固定 listen/首行启动协议、seed 后 BaseRevision 干净，以及 Candidate 的六条 `DemoAcceptanceCriteria`。测试不得通过修改 fixture 来替代 Codex Diff。
- Runner 级受控测试覆盖参数拒绝、revision/archive 来源、run root 隔离、command ledger 的同 key 重送约束、`202` 轮询、预算/失败分类、脱敏与现场保留。它们可以用受控 transport 或测试 Daemon seam 验证外部错误路径，但不能把 fake Runtime 成功写成 canonical Golden Path 成功。
- Control Plane 集成验收通过临时真实 Git demo Repository、公开 API/CLI、真实 Daemon/Worker 生命周期和实际 Git revision 链验证 Project、Change、Graph、Diff、Verify、Commit、Final Verify 和 `integrate_ready`。这条测试只在 Ticket 09–11 已真实落地后启用。
- Dashboard 验收使用固定 Playwright/Chromium、Daemon 托管 production build 和真实 EventSource。必须验证四个深链接、offline-to-online 重连、新 Query、刷新后的状态重建以及与 Daemon Query 的一致性；Vite dev server、mock payload、静态截图或前端状态伪造均不是通过条件。
- 每个 terminal Run 验证 review packet manifest 的 SHA-256、材料摘要、RunID/EvidenceSetID/ReviewID 绑定和 FAIL/UNVERIFIED 不发布规则。成功 Evidence 的追加测试验证 append-only 关系及撤销/替代引用，不验证或保存敏感原文。
- 平台验收必须在非 WSL Linux host/VM 和 WSL 分别运行真实 Codex，且两份 PASS 使用同一 EvidenceSetID。当前 WSL、交叉编译或单机版本探针只可用于环境诊断，不能替代 Linux 成功记录。
- 源码实现完成后运行适用的 `go test ./...`、`go vet ./...`、构建、Dashboard production build 和 `git diff --check`。文档阶段只将 `git diff --check` 视为本次文档卫生验证，不能宣称未实现的代码或 E2E 已通过。
- 可复用的先例是现有 Ticket 08 的真实 graph/integration 覆盖、Ticket 06 的真实 Codex smoke 证据约束、Ticket 09/10 的临时 Git/Verify/Commit 验收边界，以及 Ticket 11 的 production Dashboard/SSE 浏览器验收定义；它们提供边界语言，不替代本 Ticket 的完整真实运行。

## Out of Scope

- 提前实现或修复 Ticket 09 的 Workspace/Execute/Diff、Ticket 10 的 Verify/Commit/FinalVerify、Ticket 11 的 Dashboard Query/SSE/静态托管，或任何上游业务 Schema、Scheduler、Gate 和 Lifecycle 行为。
- 用 mock Runtime、fake Codex、OpenCode fallback、版本探针、Worker capability、构建、交叉编译、截图或文档声明替代真实 Codex Golden Path 成功。
- merge、push、PR、deploy、远程 Repository、远程 Worker、Worker pool、远程鉴权、长期运维、团队/RBAC 或 Ticket 12 以外的原生 Windows 验收。
- 允许 Runner 直接写 SQLite、调用内部 Service、使用 debug seam、修改调用者源码、预写候选代码、人工修复 Codex 输出，或自动发起新的逻辑 Command。
- 使用用户默认 LocalStateRoot、用户全局 Git config、默认/动态浏览器下载、Vite dev server、mock UI 状态、自由 shell 或不受控环境作为 canonical 运行条件。
- 在真实 Run 和独立 PASS 前创建成功 Evidence，或将 FAIL/UNVERIFIED、未执行、Codex 不可用、平台无法证明的 Run 写成成功。
- 自动清理、reset、clean、amend、rebase、删除 Worktree 或删除失败现场；任何清理仅能在独立只读复核后由人工完成。

## Further Notes

- 当前 checkout 已有 Ticket 08 的 Canonical Ticket Graph，但 Ticket 09、Ticket 10 和 Ticket 11 的生产链仍是 Ticket 12 的实现硬阻塞。本规格描述已确认目标，不证明这些能力、fixture、Runner、浏览器工具或成功 Evidence 已存在。
- 成功 Evidence 文件必须在真实、独立 PASS 后首次人工创建；其当前不存在是正确状态，而非待补的空文档。
- 实现开始前必须重读根 `AGENTS.md`、根 `INDEX.md`、目标 package 的最近一级 `AGENTS.md`/`INDEX.md`、主 Ticket、上游规格、当前源码与测试。新增 Go package 时仍须同步创建其局部导航文档。
- 本规格使用本轮已确认的最高层 seam，不再引入新的业务 API、数据库表或跨进程协议。若上游实际实现与本规格的公开边界不一致，先回到契约对齐，不应在 Runner 中加入私有绕过。

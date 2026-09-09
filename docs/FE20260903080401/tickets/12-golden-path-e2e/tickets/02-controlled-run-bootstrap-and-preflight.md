# 12-02：受控 GoldenPathRun Bootstrap 与外部预检

> 状态：`implemented-unverified`（Runner bootstrap/preflight 代码已落地，尚未形成真实验收证据）。本次按用户授权偏离上游阻塞；父 Ticket：[12 Golden Path E2E Evidence](../../12-golden-path-e2e.md)；规格：[12-golden-path-e2e-spec.md](../spec/12-golden-path-e2e-spec.md)。
>
> 顶层硬阻塞：Ticket 11 的真实 acceptance evidence 未形成前不得开始实现；本子票还依赖 12-01 的 fixture baseline。

**What to build：** 让操作者能够以固定 Keystone revision 启动一个无业务写入的受控 Run bootstrap，获得可复核的 RunID、EvidenceSetID、二进制来源、Codex/browser go-no-go 预检、DaemonReadiness 与平台来源声明，并在任何失败时保留现场。

**Blocked by：** Ticket 11 的真实 acceptance evidence；12-01 Golden Path Fixture 与 Demo Service Baseline。

**Status：** implemented-unverified（仅表示代码已落地，不表示 GoldenPathEvidence）

## Scope

- 要求完整 Keystone Git OID、新建 run root、平台声明、Codex executable、已准备浏览器目录；Linux 另要求 host 或 vm 声明。
- 从固定 revision 的 archive 驱动 canonical Runner；入口仅校验参数、revision 与调用者工作树清洁性，不能让调用者工作树提供实际二进制、fixture 或 Dashboard 资产。
- 在任何 Codex 预检、Daemon 启动或公开写入前生成 UUIDv7 `GoldenPathRunID` 与规范输入 manifest；其 SHA-256 作为 `GoldenPathEvidenceSetID`，绑定 revision、fixture、固定 ChangeIntent、Runner 和 Dashboard lockfile。
- 在独立 run root 中构建受控 Keystone binaries、选择独立 LocalStateRoot、启动 Daemon，并仅以自身 RuntimeMetadata 建立第一条 loopback API 连接后用公开 Daemon status Query 交叉验证。
- 有界执行指定 Codex 的 version/login-status 预检，并验证由 lockfile 固定、预先准备的 Playwright/Chromium；为 Daemon/Worker 建立受控继承 PATH。
- 生成脱敏 `GoldenPathPlatformProvenance` 与失败安全摘要；停止时优先公开关闭 Daemon，只能终止 Runner 自己的残留进程树，并保留已形成的材料。

## Authority and Safety Boundaries

- `GoldenPathCodexPreflight` 通过不构成 `RealCodexAcceptance`，也不能伪造 Worker capability、Report 或 AgentRun。
- RuntimeMetadata 仅用于发现本 Run 的 Daemon endpoint，不能充当权威 Query、Evidence 或恢复决策。
- Codex binary 缺失、版本失败、认证失败/不可判定、浏览器缺失/版本不符、平台来源不符均在公开业务写入前终止；不得使用 OpenCode fallback、动态 `npx` 下载或隐式用户默认路径。
- `GoldenPathEvidenceSetID` 仅固定共同输入；Go、Git、Codex 和浏览器运行时版本按平台记录，不因版本不同自动认定同源失败。

## Acceptance Criteria

- [ ] 任何调用者分支名、当前 HEAD、脏工作树、既有 run root、隐式 Codex/浏览器路径或默认 LocalStateRoot 都会被拒绝或明确诊断，且不产生 Project、Change、Ledger 或成功 Evidence。
- [ ] canonical Runner 从 archive 来源运行，生成可验证的 RunID/EvidenceSetID，并记录 revision、fixture、Runner、lockfile、二进制与安全摘要的对应关系。
- [ ] Codex 预检在限定时间内形成安全的通过或明确失败类别；同一已解析 binary 被 Daemon/Worker 使用，但认证原文、token、完整 PATH 和绝对路径不进入可发布材料。
- [ ] Daemon 在 60 秒预算内仅以受限 RuntimeMetadata 发现后经公开 status Query 确认同一实例；停止、超时和残留进程路径均只影响本 Run 并保留现场。
- [ ] `linux`/`wsl` 声明与脱敏本机探针一致；Linux 出现 WSL/container 信号或无法判断时不能取得可继续的 platform provenance。

## Out of Scope

- Project/Change/Command 创建、Planning、Ticketize、Execute、Verify、Commit、Final Verify、Candidate demo 检查、Dashboard 浏览器观察或 Evidence 发布。
- 更改 Daemon/Worker 公共健康 API、Worker Protocol、上游生命周期行为或用户的系统级 Codex/browser 安装。

## Verification

- 使用受控参数、archive、临时根与进程树测试 bootstrap 的外部成功/失败行为、隔离、保留和脱敏。
- 在真正启动 Daemon 时，仅记录 DaemonReadiness，不将其声称为 Worker/Codex 或 Golden Path 成功。


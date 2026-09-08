# 12-01：Golden Path Fixture 与 Demo Service Baseline

> 状态：`ready-for-agent`（仅表示规划成熟度，尚未实现）。父 Ticket：[12 Golden Path E2E Evidence](../../12-golden-path-e2e.md)；规格：[12-golden-path-e2e-spec.md](../spec/12-golden-path-e2e-spec.md)。
>
> 顶层硬阻塞：Ticket 11 的真实 acceptance evidence 未形成前，不得开始本子票实现。

**What to build：** 交付一个可被独立复制、初始化并自动观察的 Go HTTP demo baseline，使操作者能从真实临时 Git Repository 获得固定 `BaseRevision`、原 `GET /` 行为和不可由后续 Change 修改的服务启动协议。

**Blocked by：** Ticket 11 的真实 acceptance evidence；没有其他 Ticket 12 子票前置。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 提供不含 `.git`、secret、token、运行时状态或 Keystone DB 的 Go 1.27 demo fixture。
- 固定严格 ProjectManifest V2、单一 `go test ./...` VerificationCommand、900 秒 timeout 与无 commit template 的 V1 Golden Path 策略。
- 让初始 demo 只提供 HTTP 200 的 `GET /` 和精确 `hello`；不得预置 `/healthz` 实现或其测试。
- 固定服务的 loopback listen 参数和首行 JSON startup 宣告，使 Runner 能安全发现实际临时端口。
- 提供 fixture clone 后的本地 Git 初始化：固定 local identity、关闭 local GPG signing、无 hook 的 seed commit 与干净 `BaseRevision`；不得读取或写入用户 global Git config。
- 建立可复用的确定性检查，区分 initial baseline 与未来 Candidate 的六条 `DemoAcceptanceCriteria`，但不自行生成 Candidate 变更。

## Authority and Safety Boundaries

- fixture 是 GoldenPathFixture，不是共享临时 Repository、子模块或嵌套 Git 历史。
- ProjectManifest V2 是固定验证策略输入；Candidate Workspace 不得用修改 Manifest 改写它。
- 服务 startup 协议、`GET /` 的既有行为和六条 DemoAcceptanceCriteria 是 Golden Path 契约，不是 Runtime prompt 或可由 CanonicalTicket 自行弱化的 AcceptanceCriterion。
- 本子票不启动 Daemon、不创建 Project/Change、不生成 `GoldenPathCommandLedger`，也不产生任何成功 Evidence。

## Acceptance Criteria

- [ ] 复制 fixture 后可创建真正独立的 Git Repository，并以固定 local identity 产生干净 `BaseRevision`；用户 global Git config 与原 Repository 不发生修改。
- [ ] 初始服务以 loopback 临时地址启动时，首行以约定 JSON 唯一宣告实际地址；`GET /` 返回 HTTP 200 与精确 `hello`，且初始版本没有 `/healthz` 实现或直接测试。
- [ ] fixture 的 ProjectManifest 严格符合 V2 Golden Path 策略，`go test ./...` 能作为固定 VerificationCommand 执行，且未配置 commit template。
- [ ] fixture 级测试证明无嵌套 Git、无敏感运行材料、启动协议稳定，以及 Candidate 检查可按六条固定语义判断，不通过预写 Candidate 源码获得通过。
- [ ] 文档与测试不把 fixture baseline、构建或 fake 结果陈述为 RealCodexAcceptance 或 GoldenPathEvidence。

## Out of Scope

- GoldenPathRunner、Daemon/Worker 启动、Codex 预检、Project/Change 创建、Planning、Ticketize、Execute、Verify、Commit、Final Verify、Dashboard 或 Evidence 发布。
- 对 Ticket 09–11 的业务实现、ProjectManifest 兼容性扩张、自动 Git 清理或任何远程 Git 副作用。

## Verification

- 运行 fixture 的确定性 Go/HTTP/Git contract 测试。
- 运行适用的根级 Go 验证与 `git diff --check`；只记录实际执行结果。


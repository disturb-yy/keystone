# 12-04：公开 Execute 至 Candidate 驱动

> 状态：`ready-for-agent`（仅表示规划成熟度，尚未实现）。父 Ticket：[12 Golden Path E2E Evidence](../../12-golden-path-e2e.md)；规格：[12-golden-path-e2e-spec.md](../spec/12-golden-path-e2e-spec.md)。
>
> 顶层硬阻塞：Ticket 11 的真实 acceptance evidence 未形成前不得开始实现；本子票直接要求 Ticket 09/10 的真实 Worktree、Verify、Commit 与 Final Verify acceptance evidence。

**What to build：** 让 GoldenPathRun 从已形成的 Canonical Ticket Graph，仅通过公开 CLI 与公开 Query 以真实 Codex 依序得到 Worktree Diff、逐 Ticket Verify/Keystone Commit、Final Verify、CandidateRevision 与 `integrate_ready`，而不在 Runner 中补写任何业务逻辑。

**Blocked by：** Ticket 11 的真实 acceptance evidence；Ticket 09 与 Ticket 10 的真实 acceptance evidence；12-03 公开 Planning 与 Canonical Ticket Graph 驱动。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 使用默认 branch 的公开 Execute Command，明确传入 expected version 和 Idempotency-Key；不得指定替代 branch 或通过 Runner 创建 Workspace。
- 按 Daemon 返回的 canonical order 观察真实 Execute、TicketDelta、CompleteChangeDiff、TicketExecutionEvidence 与 AgentRun；Codex 未形成真实 Diff 时立即失败。
- 仅在独立 Verifier PASS、Keystone Commit 与 before/after revision 收敛后，才通过公开 Verify/Commit Command 驱动下一张 Ticket。
- 通过公开 Final Verify Command 观察独立 PASS、CandidateRevision 与 `integrate_ready`；Verify/Final Verify 的固定 `go test ./...` 是确定性 VerificationCommand，不替代独立 VerifierAgentRun。
- 复用 CommandLedger、预算、重送与失败保留规则，不自动刷新 version、重写请求、修复源码或创建额外 Change。

## Authority and Safety Boundaries

- ExecutionAuthorization、Workspace、Diff、VerificationOutcome、KeystoneCommit、CandidateRevision 和 Change 生命周期均由 Daemon 依据上游 Ticket 09/10 的权威事实形成。
- Runner 不读取或修复 Keystone Workspace；不直写 Git、SQLite、Commit、Artifact、Event、Gate 或 Decision。
- 真实 AgentRun 失败、Diff 缺失、版本/契约冲突、Verifier FAIL/HUMAN_REQUIRED、Commit 异常、Final Verify 失败或预算耗尽都终止整个 Run，并保留未知现场。

## Acceptance Criteria

- [ ] Execute、Verify、Commit、Final Verify 全部仅经公开 CLI/API 发起，并带有完整 Ledger、expected version 与 Idempotency-Key 关联。
- [ ] 每张实际 CanonicalTicket 的真实 Diff、执行 Evidence、Verifier PASS、Keystone Commit 与 revision 链均可由公开 Query 复盘；Runner 不假定数量、ID 或固定图形。
- [ ] 最终只在独立 Final Verify PASS 后观察到 CandidateRevision 与 `integrate_ready`；不发生 merge、push、PR、deploy 或其他远程副作用。
- [ ] 任一失败/冲突/中断不产生第二个逻辑 Command、Commit 或成功状态，且 Workspace、Repository、LocalStateRoot、Commit 与日志在复核前保留。
- [ ] 所有 runtime-backed AgentRun 使用真实 Codex，且 Execute 阶段确实形成源码 Diff；fake Runtime 或局部测试不能替代本 slice 的成功。

## Out of Scope

- 实现或修改 Ticket 09/10 的生命周期、Scheduler、Workspace、Verifier、Commit、Migration、Worker Protocol 或 Dashboard 业务能力。
- Candidate demo 服务运行、Dashboard 观察、review packet、双平台 Evidence 发布或人工清理现场。

## Verification

- 使用真实临时 Git demo 与已落地的公开 Control Plane/Worker 链，验证从 Ticket Graph 到 CandidateRevision 的完整 revision 因果链。
- 覆盖真实 Diff 缺失、Verifier/Commit/Final Verify 失败、`202` 轮询、同 key 重送及现场保留，而不使用自动修复或状态伪造。


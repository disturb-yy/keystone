# ADR 目录索引

本目录保存 Keystone 已接受、难以逆转且需要保留取舍背景的架构决策。ADR 解释“为什么这样定”，不替代 Ticket 实施规格、源码、测试或当前运行行为证据。

## 使用规则

- 仅记录修改成本高、脱离上下文容易被误解、且经过真实方案取舍的决策。
- 新 ADR 使用递增四位编号和简短文件名；已失效的决策通过后续 ADR 说明替代关系。
- 术语定义归入根 `CONTEXT.md`，可验证的当前路径和状态归入根 `INDEX.md`。

## 当前记录

| ADR | 主题 | 状态 |
| --- | --- | --- |
| [0001](./0001-local-daemon-control-plane.md) | 本机 Daemon 的发现与控制边界 | 已接受 |
| [0002](./0002-project-initialization-authority.md) | Project 初始化的身份与权威边界 | 已接受 |
| [0003](./0003-strict-project-manifest-v1.md) | 严格的 ProjectManifest V1 边界 | 已接受 |
| [0004](./0004-project-init-cross-resource-recovery.md) | Project 初始化的跨资源恢复顺序 | 已接受 |
| [0005](./0005-change-lifecycle-checkpoint-and-status.md) | Change 检查点与运行状态分离 | 已接受 |
| [0006](./0006-artifact-reference-recovery-order.md) | Artifact 与权威引用的恢复顺序 | 已接受 |
| [0007](./0007-domain-event-audit-ledger.md) | 追加式 Domain Event 审计账本 | 已接受 |
| [0008](./0008-change-creation-source-snapshot.md) | Change 创建时固定干净源快照 | 已接受 |
| [0009](./0009-failure-recovery-requires-human-decision.md) | 失败恢复要求人工 Decision | 已接受 |
| [0010](./0010-late-agent-run-result-fence.md) | Pause 与 Cancel 隔离晚到 AgentRun 结果 | 已接受 |
| [0011](./0011-change-command-version-precondition.md) | Change Command 的版本前置条件 | 已接受 |
| [0012](./0012-agent-run-single-write-outcome.md) | AgentRun 的一次性终态 | 已接受 |
| [0013](./0013-separate-change-command-and-decision-boundaries.md) | Change Command 与 HumanDecision 分离 | 已接受 |
| [0014](./0014-unified-event-ledger-with-narrow-contracts.md) | 统一 Event 账本保持窄边界 Contract | 已接受 |
| [0015](./0015-separate-project-and-change-receipts.md) | Project 与 Change 的 Receipt 分离 | 已接受 |
| [0016](./0016-daemon-resolves-project-for-change-creation.md) | Daemon 为 Change 创建解析 Project | 已接受 |
| [0017](./0017-cli-change-mutations-require-caller-idempotency-key.md) | CLI Change 变更操作要求调用者提供幂等键 | 已接受 |
| [0018](./0018-change-intent-is-raw-immutable-artifact.md) | Change Intent 作为原样不可变 Artifact 保存 | 已接受 |
| [0019](./0019-change-snapshot-and-trace-reads-are-separate.md) | Change 快照与 Trace 查询分离 | 已接受 |
| [0020](./0020-artifact-content-is-verified-on-read.md) | Artifact 内容在读取时校验摘要 | 已接受 |
| [0021](./0021-control-plane-errors-classify-recoverability.md) | Control Plane 错误按可恢复性分类 | 已接受 |
| [0022](./0022-change-creation-is-not-semantic-deduplication.md) | Change 创建只按幂等操作去重 | 已接受 |
| [0023](./0023-work-owns-business-persistence-daemon-composes.md) | Work 拥有业务持久化，Daemon 负责组合 | 已接受 |
| [0024](./0024-change-receipt-replays-first-success-response.md) | Change Receipt 重放首次成功响应 | 已接受 |
| [0025](./0025-sqlite-enforces-work-invariants.md) | SQLite 执行 Work 的关键不变量 | 已接受 |
| [0026](./0026-worker-pull-and-process-authentication.md) | Worker Pull 与进程级协议鉴权 | 已接受 |
| [0027](./0027-lease-fenced-report-and-evidence-commit.md) | Lease 围栏、Report 幂等与证据提交 | 已接受 |
| [0028](./0028-runtime-guard-without-full-sandbox.md) | Runtime Guard 的有限保证边界 | 已接受 |
| [0029](./0029-canonical-ticket-graph-immutability.md) | 每个 Change 的 Canonical Ticket Graph 不可替换 | 已接受 |
| [0030](./0030-ticket-graph-commit-completes-ticketize.md) | Canonical Ticket Graph 提交原子完成 Ticketize | 已接受 |
| [0031](./0031-fenced-ticket-generation-is-not-reusable.md) | 被围栏的 TicketGeneration 不复用 | 已接受 |
| [0032](./0032-explicit-execute-workspace-and-ticket-completion.md) | 显式 Execute、Change Workspace 与 Ticket 级完成 | 已接受 |
| [0033](./0033-execution-session-provisioning-and-ticket-delta.md) | 执行会话、可恢复 Workspace Provisioning 与 Ticket 增量证据 | 已接受 |
| [0034](./0034-execute-request-identity-and-observation-boundary.md) | Execute 请求身份与执行观察边界 | 已接受 |
| [0035](./0035-ticket-execution-evidence-and-state-machine.md) | Ticket 执行证据与执行状态机 | 已接受 |
| [0036](./0036-execution-envelope-and-workspace-boundary.md) | 不可变执行信封与 Workspace 边界 | 已接受 |
| [0037](./0037-project-manifest-v2-verification-configuration.md) | ProjectManifest V2 承载确定性验证配置 | 已接受 |
| [0038](./0038-keystone-commit-intent-recovery.md) | KeystoneCommit 以 CommitIntent 收敛 Git 与 SQLite | 已接受 |
| [0039](./0039-deterministic-execution-scheduling-and-workspace-location.md) | 确定性执行调度与私有 Workspace 位置 | 已接受 |
| [0040](./0040-lease-dispatch-and-runtime-fencing.md) | Lease 发放、等待与 Runtime 围栏 | 已接受 |
| [0041](./0041-explicit-runtime-modes-and-git-policy.md) | 显式 Runtime 模式与 Git 策略 | 已接受 |
| [0042](./0042-execution-timeout-and-artifact-input-projection.md) | 执行 timeout 与 Artifact 输入投影 | 已接受 |
| [0043](./0043-replayable-assignment-delivery-and-runtime-claim.md) | 可重放 Assignment 交付与一次性 Runtime Claim | 已接受 |
| [0044](./0044-dual-diff-evidence-and-atomic-ticket-completion.md) | 双重 Diff 证据与原子 Ticket 完成 | 已接受 |
| [0045](./0045-explicit-execute-api-and-bounded-read-model.md) | 专用 Execute API 与受限执行读取模型 | 已接受 |
| [0046](./0046-execution-domain-ports-and-atomic-persistence.md) | 执行领域 Port 与原子持久化边界 | 已接受 |
| [0047](./0047-sourcecontrol-topology-recovery-and-native-validation.md) | SourceControl 拓扑、恢复与原生验证 | 已接受 |
| [0048](./0048-execute-fencing-and-requeue-recovery.md) | Execute 围栏与重新入队恢复 | 已接受 |
| [0049](./0049-structural-default-execution-authorization-policy.md) | 结构性默认执行授权策略 | 已接受 |
| [0050](./0050-final-verify-does-not-reopen-committed-tickets.md) | Final Verify 失败不重开已提交 Ticket | 已接受 |
| [0051](./0051-ticket-verify-commit-gates-next-execution.md) | Ticket Verify/Commit 串行收口后才执行下一张 Ticket | 已接受 |
| [0052](./0052-verification-command-execution-and-evidence.md) | 固定验证命令与独立判定证据 | 已接受 |
| [0053](./0053-verify-commit-final-verify-control-plane.md) | Verify、Commit 与 Final Verify 的 Control Plane Command | 已接受 |
| [0054](./0054-worker-v1-verification-capability-extension.md) | Worker V1 的验证能力扩展 | 已接受 |
| [0055](./0055-ticket-input-revision-and-commit-chain.md) | 逐 Ticket 输入 Revision 与受控提交链 | 已接受 |
| [0056](./0056-verification-recovery-and-snapshot-checkpoint.md) | 验证恢复只从耐久 Snapshot 继续 | 已接受 |
| [0057](./0057-execution-schema-and-atomic-migration.md) | 执行 Schema 与原子迁移 | 已接受 |
| [0058](./0058-dashboard-observation-query-and-refresh-boundary.md) | Dashboard Observation 的 Query 与 Refresh 边界 | 已接受 |

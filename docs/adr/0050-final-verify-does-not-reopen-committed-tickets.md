# ADR-0050：Final Verify 失败不重开已提交 Ticket

## 状态

已接受。

## 决策

Ticket Verify 的 FAIL 或 HUMAN_REQUIRED 发生在 KeystoneCommit 前；HumanDecision retry 只能为同一 CanonicalTicket 创建新的 Execute AgentRun，随后重新 Verify。所有 Ticket 已 Commit 后的 FinalVerification FAIL 则将 Change 保留在 `human_required`，不得 amend、reset、重开或重新分配已 Commit 的 CanonicalTicket；修复必须从 CandidateRevision 创建新的 Change。

FinalVerification 的 HUMAN_REQUIRED 可以通过 HumanDecision retry 再次执行，但仅当 CandidateRevision 与 WorkspaceSnapshot 完全相同。暂时 unavailable 在任何 verdict 写入前重送同一请求，不形成 HumanDecision 或语义失败。

## 理由与边界

已提交 Ticket 的回写会使不可变 CanonicalTicketGraph、KeystoneCommit 追溯链和 Git 历史同时出现多义恢复路径。将修复放入新的 Change 保留了已验证候选与后续补救的因果关系；只允许未变候选重跑人工要求的 Final Verify，避免 retry 成为未审计改码通道。该 ADR 不实现新的 Change、Final Verify 或恢复 API。

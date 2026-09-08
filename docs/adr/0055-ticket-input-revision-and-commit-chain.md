# ADR-0055：逐 Ticket 输入 Revision 与受控提交链

## 状态

已接受。

## 决策

Change 的 BaseRevision 始终是创建时固定的源事实，不能被后续 Git HEAD 覆盖。每个 Execute、Ticket Verify 与 FinalVerify ExecutionEnvelope 另外固定 WorkspaceInputRevision：首张 Ticket 为 BaseRevision，后续 Ticket 为前一张 KeystoneCommit 的 after revision，FinalVerify 为最后一张 KeystoneCommit 的 after revision。

Ticket 09 的 Runtime 成功和 WorkspaceSnapshot 不变量因此比较该 Ticket 的 WorkspaceInputRevision，而不是无条件比较 BaseRevision。每个 Ticket 的 CompleteChangeDiff 是相对于该输入 revision 的完整未提交状态，必须仍与 Worker 的原始 diff/changed_files 一致；已提交的前序 Ticket 通过 KeystoneCommit 链与各自 TicketDelta 保留为累计追溯事实，不伪造为当前未提交 Diff。

Ticket Verify 固定并重验当前 WorkspaceSnapshot 的 CandidateTreeIdentity。CommitIntent 的预期 parent 必须是同一 WorkspaceInputRevision；Daemon 执行 `git add --all` 后得到的 CommitTreeIdentity 必须等于已验证的 CandidateTreeIdentity，Git 成功后新 HEAD 成为下一张 Ticket 的 WorkspaceInputRevision。恢复已形成提交的 Workspace 时核验记录的当前输入 revision，而不是把 HEAD 强行回退或比作 BaseRevision。

## 理由与边界

Ticket 10 要求每张 Ticket Verify PASS 后立即由 Keystone 提交；若所有后续执行仍要求 `HEAD == BaseRevision`，第二张 Ticket 会被前一张合法提交错误围栏。分离全局源事实与逐 Ticket 输入事实保留了 BaseRevision 的审计意义，同时使受控提交链可验证。该决定不允许 Runtime 自行 commit、amend、reset 或选择 parent，也不改变首次 Worktree 创建必须从 BaseRevision 开始的规则。

# ADR-0038：KeystoneCommit 以 CommitIntent 收敛 Git 与 SQLite

## 状态

已接受。

## 决策

每张 CanonicalTicket 只有在 Ticket Verify PASS 后才可由 Daemon 创建一次 KeystoneCommit。TicketExecutionSuccess 与 VerificationEvidence 都要求没有 residual untracked 文件；新建源码必须在 edit ExecutionMode 中先 `git add` 纳入 Snapshot。Commit 前 Daemon 重验当前 WorkspaceSnapshot 与已验证快照完全相同，再以 `git add --all` 形成 CommitTreeIdentity，且该 tree 必须等于快照表达的候选内容。

Daemon 随后在 SQLite 持久化 CommitIntent，固定 Ticket、该 Ticket 的 WorkspaceInputRevision 作为预期 parent revision、CommitTreeIdentity、VerificationEvidence、CommitTemplateDigest 与 KeystoneCommitID；Git 成功后核验 HEAD 直接承接预期 parent、tree 完全匹配且工作树全净，再记录 before/after revision 与 KeystoneCommit。消息强制包含 `Keystone-Change-ID`、`Keystone-Ticket-ID` 与 `Keystone-Commit-ID` trailer。重试或重启只以 CommitIntent、直接 parent/tree 和唯一 trailer 对账；已发现对应 Commit 时只收敛权威记录，不再创建新 Commit，缺失、歧义或冲突进入 `human_required`。

Commit 只能由带 ChangeVersion 与 IdempotencyKey 的显式 CommitCommand 触发；Client 不提供自由 Git 参数。CommitCommand 是同步的，只有 KeystoneCommit 已记录才产生成功回执。提交消息使用受校验 CommitTemplate 或 CanonicalTicket title 的默认形式。Daemon 使用 Workspace 已配置的 Git author/committer，缺失身份进入 `human_required`；Commit 使用 `--no-verify`，使所有必要检查都通过已记录的 VerificationCommand 完成。

所有 CanonicalTicket 均已有 KeystoneCommit 后，Daemon 才在最终 HEAD 执行 FinalVerification。只有 FinalVerification PASS 时，该 HEAD 才成为 CandidateRevision，并与 Change 的 `integrate_ready` 状态一同成为权威事实。该链不包含 merge、push、PR、deploy 或自动回滚。

## 理由与边界

Git 与 SQLite 不能组成单一原子事务。先写可恢复意图、后执行 Git、再收敛数据库记录，能在进程中断后区分“尚未提交”“已提交待记录”和“冲突”，并避免根据 reflog 或模糊文本猜测。受控消息、trailer、作者来源和 `--no-verify` 让 Git 操作仍可按同一意图识别，而不让 hook 或 Client 自由参数成为隐藏副作用。该 ADR 不决定具体 HTTP 路径或数据库字段；它们由 Ticket 10 规格进一步冻结。

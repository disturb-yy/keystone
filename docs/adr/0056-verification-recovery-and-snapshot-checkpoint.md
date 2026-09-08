# ADR-0056：验证恢复只从耐久 Snapshot 继续

## 状态

已接受。

## 决策

Ticket Verify FAIL 后，HumanDecision `retry` 只能为同一、尚未 Commit 的 CanonicalTicket 创建新的 Execute AgentRun。Daemon 在发放新 Assignment 前必须确认 WorkspaceInputRevision 未变，且新的执行前 WorkspaceSnapshot 等于该 FAIL VerificationEvidence 已持久化的候选 Snapshot；这允许同一 Ticket 在已知失败候选上增量修复，但不允许混入另一张 Ticket、外部进程或未知围栏留下的未提交内容。

仅因完整验证证据无法形成而得到的 Ticket HUMAN_REQUIRED，可在 WorkspaceInputRevision 与 CandidateTreeIdentity 都不变时创建新的 VerifierAgentRun。由 Lease/Runtime fence、候选 Snapshot 改变、Guard finding 或未知 Workspace 状态导致的 HUMAN_REQUIRED，必须先由人把 Workspace 恢复到相应耐久 Snapshot；重验不匹配时继续保持 `human_required`。Daemon 不在 retry 中自动 `reset`、`clean`、删除文件、amend 或重写 Git 历史。

FinalVerification FAIL 的修复始终创建以 CandidateRevision 为起点的新 Change；FinalVerification HUMAN_REQUIRED 只允许在最终 revision 与 WorkspaceSnapshot 均不变时重新验证。暂时 unavailable 在任何 verdict 前重送原 Intent，不进入本 ADR 的 HumanDecision 恢复分支。

## 理由与边界

受控 Commit 前的失败候选既可能是同一 Ticket 的可审计待修复状态，也可能是围栏后无法证明归属的未知状态。把两者都交给自动清理会破坏证据或隐藏数据丢失；把两者都直接重跑又会把未知修改误归因。耐久 Snapshot 是无需自动破坏 Workspace 即可区分两类状态的最小恢复锚点。该决定不提供人工文件修复 UI、Git 清理命令或任意 Workspace 编辑权限。

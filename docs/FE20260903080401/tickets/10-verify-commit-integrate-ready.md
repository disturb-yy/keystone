# 10 — Verify, Commit and Integrate Ready

- 里程碑：M8
- `BLOCKED_BY`：09
- 交付类型：验证与 Git 收口纵切

> 状态：规划规格已对齐，尚未实现。顶层 Ticket 09 的真实实现与验收仍是硬阻塞；本 Ticket、其规格和子票的 `ready-for-agent` 只表示实施契约已经冻结，不表示 Verify、Commit、Worker V1 扩展或 Integrate Ready 已存在于当前 checkout。

## 目标

实现独立验证、Keystone 受控 Commit 和 Change-level Final Verify，使真实 Change 仅在完整证据成立后到达 Integrate Ready。每张 CanonicalTicket 必须先完成 Execute、Verify PASS 和 KeystoneCommit，Scheduler 才能授权下一张 Ticket；全部 Commit 后才允许 Change-level Final Verify。

## 范围

- 严格读取 BaseRevision 中的 ProjectManifest V2 `verify.commands` 和可选 `commit.template`，在 Execute 接受时固定 VerificationPolicySnapshot；V1 仅保留既有身份语义，不能静默升级或自动 Verify。
- 直接扩展既有 `worker/v1`：支持 `verification-v1` capability 的 Worker 接收明确的 `kind: verify` Assignment 和结构化 VerificationReport；不创建 `worker/v2`。
- 在 Assigned Workspace 以只读 `verify` ExecutionMode 运行 Workspace/Git Checks 与确定性验证命令，并对每条 Acceptance Criterion 保存独立 Verifier 的结构化结果。
- 创建只读的独立 Verifier AgentRun；Implementer 与 Verifier 不得是同一 Run，Verifier 不直接修复代码。
- Evidence 必须包含固定策略、命令、exit code、受限输出 Artifact、WorkspaceInputRevision、验证前后 Snapshot、Criterion 结果与采集 AgentRun；Implementer 自报不是 PASS Evidence。
- 在 Ticket Verify PASS 后由 Keystone 以 CommitIntent 创建 Commit，使用受校验模板或默认 title、固定 trailers、已配置 Git author/committer 和 `--no-verify`，并记录逐 Ticket revision 链。
- 实现异步 Verify/FinalVerify Intent、同步 Commit、只读 ExecutionReadModel、CLI 映射、Change-level Final Verify、CandidateRevision 与 `integrate_ready` 状态转换。

## 不包含

- merge、push、PR、deploy、远程 Git 托管或自动回滚。
- 绕过独立 Verifier 直接接受 Implementer 的成功摘要。
- Runtime 执行 `git commit`。
- `worker/v2`、自由 shell/cwd/env、ProjectManifest V1 自动改写、任意 Git 历史改写或自动清理。

## 验收条件

- 验证结果明确为 PASS、FAIL 或 HUMAN_REQUIRED；V1/无效验证配置、候选变化、必要证据不完整和已 Claim 后的围栏均不得形成 PASS；FAIL 或 HUMAN_REQUIRED 不会创建 Commit 或进入 Integrate Ready。
- 未具备 `verification-v1` Worker 时 Verify/FinalVerify Intent 保持 pending；同一请求重送只能重驱动同一 Intent，不创建并发 Run 或新的业务事实。
- PASS Ticket 的 Commit 只能由 Keystone 创建，且其 parent 必须等于该 Ticket 的 WorkspaceInputRevision、tree 必须等于已验证 CandidateTreeIdentity，并以唯一受控 trailer 在 Git 与 SQLite 间恢复对账。
- 前一张 Ticket Commit 后，下一张 Ticket 以该 Commit 的 after revision 作为 WorkspaceInputRevision；Change 的 BaseRevision 保持创建时的源事实，不被候选 HEAD 覆盖。
- Change-level Final Verify 仅在全部 Ticket 已 Commit、最终 Workspace 干净且 Snapshot 不变时运行；PASS 后可查询 CandidateRevision 和 Integrate Ready，FAIL 只能通过从候选 revision 发起新 Change 修复。
- 同一 Report、Verify/FinalVerify Intent 或 Commit Command 的重送不重复推进状态、不重复创建 AgentRun 或生成第二个 Commit。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

另执行真实临时 Git Repository 中的两张串行 Ticket Verify、Commit、Final Verify 验收，覆盖 V1 Worker/新 capability 共存、V1 Manifest 进入 `human_required`、命令失败、候选变化、CommitIntent 中断恢复、重复请求和 Final Verify FAIL/HUMAN_REQUIRED。涉及 Worktree、Git 写入和路径恢复的验证必须在 Linux/WSL 与原生 Windows 都有证据；交叉编译不能替代原生 Windows 运行。

## 实现边界

Integrate Ready 是 V1 的终点。Keystone 在本 Ticket 不执行 merge、push、PR、deploy、远程副作用、自动回滚、amend、reset 或 Worktree 清理。详细的实施顺序、DTO、恢复与测试边界见[实施规格](10-verify-commit-integrate-ready/spec/10-verify-commit-integrate-ready-spec.md)和[实施子票](10-verify-commit-integrate-ready/tickets/)。

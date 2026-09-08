# 10-04：Final Verify、Control Plane 与受限执行读取模型

**What to build：** 在所有 Ticket Gate 均已 Commit 后，完成 Change-level FinalVerify、CandidateRevision/`integrate_ready` 原子推进，以及 Verify/Commit/FinalVerify 的统一有界 ExecutionReadModel 和 CLI 读取入口。

**Blocked by：** 顶层 Ticket 09 的真实实现与验收，以及 10-01、10-02、10-03 的可观察完成证据。开始前重新核对 Change Lifecycle、Control Plane Contract、Daemon HTTP、CLI、Artifact 读取边界、Workstore 和所有相邻 package 局部规约。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 仅在全部 CanonicalTicket 都有 KeystoneCommit、最终 Workspace 干净、HEAD/branch/Snapshot 与当前提交链一致时接受 FinalVerifyCommand；原子写入 Final VerificationIntent 和 `202 Accepted` Receipt。
- 复用已固定 VerificationPolicySnapshot，创建新的独立 VerifierAgentRun，以最终 WorkspaceInputRevision 对全部 CanonicalTicket Criterion、Commit 链和命令证据进行只读审查；不复用任一 Implementer AgentRun，不创建 Git Commit。
- Final PASS 在单一 authority transaction 内写入 Final VerificationEvidence/Outcome、CandidateRevision、StageAdvanced 至 FinalVerify、ChangeStatus `integrate_ready`、Receipt/Event；FAIL 保持 Verify/human_required 并要求新 Change，HUMAN_REQUIRED 只在 revision/Snapshot 未变时可重验。
- 提供 `POST /v1/changes/{change_id}/final-verify`、`GET /v1/changes/{change_id}/execution`、对应 Control Plane DTO 以及 `keystone change final-verify`、`keystone change execution show`。
- ReadModel 以稳定 canonical order 显示 Ticket 状态、Gate、Intent、Outcome、Evidence identity、Commit before/after、CandidateRevision/Final Verification 摘要；原始 Artifact 内容继续经既有 Artifact 边界读取。
- 覆盖三个写 Command 的一致 request identity/version/Receipt 错误映射，并确保读取 API/CLI 不隐式启动 Worker、执行命令或修改数据库。

## Acceptance

- 任一 Ticket 未 Commit、最终 Workspace 有 tracked/untracked 变化、错误 stage/status 或版本冲突时，FinalVerify 不创建 Intent/AgentRun，也不推进 Change。
- Final PASS 仅在全部固定命令成功、输出完整、Snapshot 不变和全部 Criterion PASS 后发生，并与 CandidateRevision、FinalVerify stage、`integrate_ready` 同时可见。
- Final FAIL 不重开、amend、reset 或重分配已 Commit Ticket；测试证明修复只能由以 CandidateRevision 为起点的新 Change 表达。Final HUMAN_REQUIRED 只能在未变 candidate 上创建新的 VerifierAgentRun。
- `GET /execution` 和 CLI 显示权威摘要但不泄漏 WorkspacePath、Lease、token、Prompt、环境、命令输出或凭据；HTTP/CLI 错误与现有安全 ErrorEnvelope 一致。
- Verify/FinalVerify 的 `202`、Commit 的同步成功、同 key 重放、同 key 冲突、stale version、无 Worker pending、unavailable 与 terminal verdict 都有端到端测试。

## Out of scope

- Dashboard 页面、SSE、日志浏览、外部通知、Golden Path E2E 报告（Ticket 11/12）。
- merge、push、PR、deploy、远程托管、自动 rollback、自动创建修复 Change 或任意 Git 清理。
- 新的 Worker protocol version、远程 Worker、完整 OS sandbox 或通用审批/Governance DSL。

## Verification

```bash
go test ./contracts/controlplane/...
go test ./internal/governance/...
go test ./internal/infrastructure/workstore/...
go test ./internal/daemon/...
go test ./cmd/keystone/...
go test ./...
go vet ./...
make build
git diff --check
```

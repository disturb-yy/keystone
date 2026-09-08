# 10-02：Ticket Verify、Worker V1 能力与独立证据

**What to build：** 建立从已成功 TicketExecution 到异步 Ticket Verify、`verification-v1` Worker Assignment、独立 VerifierAgentRun 和不可变 VerificationEvidence 的首个端到端闭环，不创建 Git Commit。

**Blocked by：** 顶层 Ticket 09 的真实实现与验收，以及 10-01 的 V2 策略快照/领域骨架；开始前须核对 `contracts/worker`、`internal/worker`、`internal/execution`、Workstore、Daemon、Control Plane Contract 和 CLI 的当前实际边界。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 在既有 `worker/v1` Register/Assignment/Report 中增加 capability-gated `verification-v1`、显式 `kind: verify` 和类型化 `verification` 字段；保留旧 edit Assignment 的 JSON shape 与四类通用 Artifact kind，不创建 `worker/v2` 或新路由。
- Daemon 只为声明 `verification-v1` 的 Worker 创建 Verify Assignment；旧 Worker 可继续 edit，不能因未知严格字段接到 Verify 工作。Assignment 固定 intent、policy digest、WorkspaceInputRevision、CandidateTreeIdentity、有序 argv/timeout 和有界审查输入。
- 在 Worker 建立受注入、可 fake 的 VerificationExecutor：cwd 固定 Assigned Workspace root，参数数组无 shell，环境白名单，命令按顺序运行、首次非零/timeout 后写 `not_run`，每 stream 限制 256 KiB，并收集明确的截断/采集事实。
- 建立与 Implementer 不同的 VerifierAgentRun 和只读审查路径，按 canonical order 精确返回每条 AcceptanceCriterionRef 的 SHA-256、PASS/FAIL/HUMAN_REQUIRED 和 Evidence 引用；Verifier 不拥有修复、Commit、Lifecycle 或 SQLite 权限。
- 实现 VerifyCommand 的 Intent-first authority：校验 TicketExecutionSuccess/Gate/version/idempotency，原子写入 VerificationIntent、`202 Accepted` Receipt 与 Event；无能力 Worker 时保持 pending，Artifact/SQLite 未形成 verdict 的暂时不可用可重驱动同一 Intent。
- 完成 Ticket Verify Outcome/Evidence 的原子持久化：命令失败/timeout 为 FAIL；输出截断、候选变化、Guard finding、Criterion 覆盖不完整或必要证据缺失为 HUMAN_REQUIRED；PASS 必须全部命令成功、Snapshot 不变且所有 Criterion PASS。
- 提供 `POST /v1/changes/{change_id}/tickets/{ticket_id}/verify` 和 `keystone change verify` 的窄 adapter；Client 仅传 expected version 与 idempotency key。

## Acceptance

- 同一 `worker/v1` fleet 中，旧 Worker 只收到 edit，新 Worker 才收到 `kind: verify`；未知/重复 JSON 字段仍被严格拒绝。
- VerifyCommand 在 Worker 缺席时返回一次稳定 `202 Accepted` 并保持 Intent pending，不因暂时无 Worker 形成失败或第二个 AgentRun。
- Verifier 与 Implementer 的 AgentRun ID 必然不同；Verifier 的命令、cwd、环境、审查输入、WorkspaceInputRevision 和 Snapshot 全部来自 Daemon 固定输入。
- 命令的 argv、timeout、exit code、`not_run`、typed stdout/stderr Artifact 身份、截断事实和 Criterion 结果可追溯到同一 Intent/AgentRun；Report.Outcome 不可直接变为 VerificationOutcome。
- PASS、FAIL、HUMAN_REQUIRED、unavailable 和已 Claim 后 fence 各自有独立测试与状态结果；同一 Report/Command 重送不会重复 Evidence、AgentRun、Event 或 ChangeVersion。
- HTTP/CLI 不接受 Client command、Prompt、cwd、env、revision、Workspace 或 verdict，且不泄漏 token、绝对路径或原始输出。

## Out of scope

- `git add`、Git Commit、CommitIntent、下一张 Ticket 调度、revision 链和 Commit HTTP/CLI（10-03）。
- Final Verify、CandidateRevision、Integrate Ready 和完整 ExecutionReadModel（10-04）。
- 完整 OS sandbox、任意 shell、远程 Worker、Worker v2、自动清理或 Dashboard。

## Verification

```bash
go test ./contracts/worker/...
go test ./internal/worker/...
go test ./internal/execution/...
go test ./internal/governance/...
go test ./internal/infrastructure/workstore/...
go test ./internal/daemon/...
go test ./cmd/keystone/...
go test ./...
go vet ./...
make build
git diff --check
```

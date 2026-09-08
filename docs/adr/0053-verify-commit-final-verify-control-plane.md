# ADR-0053：Verify、Commit 与 Final Verify 的 Control Plane Command

## 状态

已接受。

## 决策

Ticket 10 固定以下 Control Plane 边界：

```text
POST /v1/changes/{change_id}/tickets/{ticket_id}/verify
POST /v1/changes/{change_id}/tickets/{ticket_id}/commit
POST /v1/changes/{change_id}/final-verify
GET  /v1/changes/{change_id}/execution
```

三个写命令的 JSON 请求体都只承载 `expected_version`，并必须带 `Idempotency-Key` header；Client 不提交 Workspace、revision、验证命令、Prompt、verdict 或 Git 参数。VerifyCommand 与 FinalVerifyCommand 在持久化 VerificationIntent 后返回 `202 Accepted`；CommitCommand 仅在 KeystoneCommit 已记录后返回成功。Receipt 以操作、Change、可选 CanonicalTicket 与规范请求身份共同限定：同键同请求重放首次成功响应，同键不同请求返回 `idempotency_conflict`，并依 ADR-0011 在回执重放后再检查版本。Verify/FinalVerify 同键重送可先收敛 unavailable intent，再返回保存的 `202 Accepted`；输入错误、业务冲突与尚未持久化 intent 的 unavailable 不创建 Receipt。

`GET /v1/changes/{change_id}/execution` 返回有界的 ExecutionReadModel：有序 Ticket 状态、VerificationIntent/结论与证据身份摘要、KeystoneCommit 前后 revision，以及 CandidateRevision/FinalVerification 摘要。它不返回绝对 Workspace 路径、Lease token、原始命令输出、Prompt 或凭据；证据内容仍经 Artifact 边界读取。CLI 一一映射为 `keystone change verify`、`commit`、`final-verify` 与 `execution show`，三个写命令均显式要求 `--expected-version` 和 `--idempotency-key`。

## 理由与边界

显式 Command 防止 Worker 回调、查询或 Client 自由脚本暗中触发验证和 Git 副作用。版本前置条件处理并发观察，幂等回执处理同一请求重放，二者不互相替代。受限读取模型为 Dashboard 和 CLI 提供可审计状态，同时不把本机执行环境变成可浏览 API。该 ADR 是 Ticket 10 的目标 Contract，不证明 DTO、路由、CLI 或持久化已实现。

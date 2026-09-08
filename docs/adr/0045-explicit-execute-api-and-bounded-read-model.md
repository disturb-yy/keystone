# ADR-0045：专用 Execute API 与受限执行读取模型

## 状态

已接受。

## 决策

Execute 使用专用 `POST /v1/changes/{change_id}/execute`，请求体只含 `expected_version` 和可选 `workspace_branch`，并且必须携带 `Idempotency-Key`。CLI 入口为：

```text
keystone change execute CHANGE_ID --expected-version N --idempotency-key KEY [--branch NAME]
```

接受请求及同一规范化请求的重放均返回 `202 Accepted` 和不可变 ExecuteCommandResponse，其中只包含 Change、ExecutionSessionID、Session 状态、规范化 branch、Ticket 状态和 Artifact 身份摘要。不同请求复用相同 key 返回 `idempotency_key_conflict`；活跃 Session 使用不同 key 返回 `execution_in_progress`。没有 Worker 不是 Execute 拒绝条件。

执行读取使用 `GET /v1/changes/{change_id}/execution`，CLI 为 `keystone change execution show CHANGE_ID`。读取模型不公开绝对 Workspace 路径、Lease token、原始 Prompt、Runtime 环境或凭据；Artifact 内容继续经过既有 Artifact 读取边界。

## 理由与边界

Execute 的 WorkspaceBranch、Session 与异步接收语义不同于 Pause/Resume/Cancel，不能塞入自由 `command` 字符串。`202 Accepted` 明确表示权威协调已建立，而不是 Codex 或 Worker 已完成。

本 ADR 仅冻结边界 DTO、路由、CLI 形状和错误分类；不证明 HTTP Handler、CLI、读取模型或执行调度已经实现。

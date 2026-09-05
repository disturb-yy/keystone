# Control Plane 错误按可恢复性分类

M3 的设计规定：所有失败响应使用既有 `ErrorEnvelope{code,message,request_id?}`，并且不含 SQL、Git 原始错误或本机绝对路径。`invalid_request` 为 400，`project_not_found`、`change_not_found` 与 `artifact_not_found` 为 404，`repository_dirty`、`base_revision_unavailable`、`source_snapshot_unstable`、`idempotency_conflict`、`change_version_conflict`、`lifecycle_transition_invalid` 与 `human_decision_required` 为 409，基础设施或 Artifact 内容不可用为 `unavailable` 503，未预期错误为 `internal_error` 500；不允许的方法沿用现有 405 `invalid_request` 约定。

409 明确表示调用者可以通过清理源、刷新快照、选择正确的生命周期入口或复用正确幂等键来处理，503 则表示安全重试或等待 Daemon 恢复；这避免 Client 从不稳定 message 推断恢复动作。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

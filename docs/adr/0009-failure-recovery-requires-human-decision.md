# 失败恢复要求人工 Decision

V1 不增加 failed ChangeStatus。可重试的 Stage 失败以失败的 AgentRun 和 Artifact 事实保存，并将 Change 置为 human_required；Daemon 不自动重试。Retry 是一项追加式 HumanDecision，并在同一权威操作中创建新的 AgentRun，因此已确认 Stage、失败证据和每次恢复尝试始终可追溯且不会被覆盖。

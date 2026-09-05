# Change Receipt 重放首次成功响应

M3 的设计规定：成功的 ChangeCreation、ChangeCommand 或 HumanDecision 与 Change 状态、Event、Decision（如有）及 ChangeCommandReceipt 在同一 SQLite transaction 提交。Receipt 持有操作/聚合/幂等键/规范请求比较信息，以及有界的首次 HTTP 成功状态码和规范响应体；同键同请求在未来重放时返回保存的原响应，而不是经过其他命令改变后的 ChangeView。同键不同规范请求命中既有 Receipt 时返回 `idempotency_conflict`。

未产生权威副作用的输入错误、业务冲突和 unavailable 不写 Receipt；调用者可在外部条件恢复后安全重试。代价是 Receipt 不是只存资源 ID 的轻量索引，但它避免把“同一结果”偷换为“同一资源的当前状态”。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

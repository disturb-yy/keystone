# Change Command 的版本前置条件

Idempotency-Key 只处理同一 Command 的重放，不能使陈旧的不同 Command 安全共存。因此所有改变既有 Change 的 Command 都必须带 expected_version：Daemon 先重放匹配的 CommandReceipt，再校验版本；新 Command 成功时与状态和 DomainEvent 在同一事务中递增 ChangeVersion，版本不匹配时明确拒绝，避免本机 Client 静默覆盖较新的人工操作。

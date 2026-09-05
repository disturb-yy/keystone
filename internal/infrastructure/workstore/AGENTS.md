# `workstore` 局部规约

该 package 是 Work 领域的 SQLite adapter 和业务 Migration owner。

- 只接收 Daemon 已打开的 `*sql.DB`，不拥有连接、锁或 HTTP 生命周期。
- Project、ProjectInitialized、Intent 和 Receipt 的 finalization 必须在一个事务中完成。
- 确定性失败须在同一事务为原始和当前幂等 key 写入 failed receipt；可恢复失败保留 pending intent。Rebind 只能按事务内的预期旧 root 条件更新。
- 查询和写入使用参数化 SQL；完整性错误不得自动修复或删除既有事实。

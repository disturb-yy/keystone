# `workstore` 局部规约

该 package 是 Work 领域的 SQLite adapter 和业务 Migration owner。

- 只接收 Daemon 已打开的 `*sql.DB`，不拥有连接、锁或 HTTP 生命周期。
- Project、ProjectInitialized、Intent 和 Receipt 的 finalization 必须在一个事务中完成。
- 查询和写入使用参数化 SQL；完整性错误不得自动修复或删除既有事实。

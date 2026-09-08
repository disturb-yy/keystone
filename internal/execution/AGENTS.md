# execution 包规约

该 package 定义 Worker 使用的 RuntimeAdapter、执行输入输出、证据采集和
ExecutionGuard 端口。它不访问 HTTP、SQLite、Keystone 业务 Domain 或业务状态。

- Runtime 只接收已验证的 Assigned Workspace 和有界 instruction。
- Planning candidate 使用 `read-only` Runtime sandbox，并将最后消息与 stdout/stderr 分开采集；候选结果仍不具有权威性。
- 进程退出码、stdout、stderr、Git revision、diff 和 changed files 是观察事实；
  Runtime 自报内容不能改变 AgentRun 或 Change 权威。
- Guard 只提供可验证的 Workspace、Git 命令和前后 revision 约束，不声称提供完整
  OS sandbox。
- `ProcessTree` 是外部 Runtime 的生命周期 owner：Unix 使用独立 process group，Windows
  使用 Job Object；取消时必须在有界预算内先优雅停止再强制清理整棵树。
- `Guard.PrepareEnvironmentAt` 只接受经过校验的受控临时目录，并把 `TMPDIR`、`TEMP`、
  `TMP` 固定到该目录，避免 Runtime 在 Workspace 内留下中间文件。
- 外部进程通过参数数组启动，必须传递 context，并过滤协议 secret、Lease token
  和数据库连接信息。

新增适配器必须补充聚焦测试和本目录 INDEX.md。

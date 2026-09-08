# `execution/application` 局部规约

该 package 编排显式 Execute 的 provisioning、Session 和后续执行端口。

- 只依赖 execution/domain 与抽象 Port，不访问 SQL、Git 命令、HTTP 或 Worker Protocol。
- 必须先通过持久化 Port 写入 provisioning intent，再调用 SourceControl。
- Git/SQLite 中断后的重试只协调同一 request digest，不执行删除或猜测性修复。
- ReadModel 只返回安全摘要，不携带 WorkspacePath、Lease、Prompt 或凭据。

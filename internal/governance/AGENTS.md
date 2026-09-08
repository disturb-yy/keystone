# Governance Application

`internal/governance` 负责 Verify、Commit 和 FinalVerify 的用例编排与窄 Port。

- 领域规则放在 `internal/governance/domain`，本 package 不写 SQL、HTTP 或 Git 命令。
- Workstore、SourceControl 和 Worker 通过接口接入；Daemon 负责具体装配。
- 任何状态推进都必须由 Workstore 的权威事务完成，Worker 返回值不能直接改变 Change。

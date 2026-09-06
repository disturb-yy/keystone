# `internal/work` 局部规约

该 package 是 Work & Lifecycle 的 Application 编排边界，承载 Project Bootstrap
和 Change 生命周期用例。

- Project 只通过 `RepositoryPort`、`ManifestPort` 和 `StatePort` 调用外部能力；StatePort 负责失败回执和事务内条件 rebind。
- Change 只通过 Project、Snapshot、Artifact 和 State port 调用外部能力；创建顺序固定为权威 Project、Receipt 预读、Git Snapshot、Artifact、SQLite finalization。
- 不写 HTTP、SQL 或文件系统细节；AgentRun、Decision 和 Change 状态的权威提交由 State port 负责。
- 领域错误必须保留错误链，并由边界层映射为稳定的 ErrorEnvelope。

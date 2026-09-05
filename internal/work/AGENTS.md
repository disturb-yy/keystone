# `internal/work` 局部规约

该 package 是 Ticket 04 的 Application 编排边界。

- 只通过 `RepositoryPort`、`ManifestPort` 和 `StatePort` 调用外部能力；StatePort 负责失败回执和事务内条件 rebind。
- 不写 HTTP、SQL 或文件系统细节；顺序固定为 intent、Manifest、SQLite finalization。
- 领域错误必须保留错误链，并由边界层映射为稳定的 ErrorEnvelope。

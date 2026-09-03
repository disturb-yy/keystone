# Keystone V1 正式 Ticket Plan

本目录下 V1 Implementation Baseline 已通过 `to-tickets` 收敛为 12 张正式执行 Ticket。Ticket 的范围、阻塞图和验收条件以本文件与 `tickets/INDEX.md` 为准。

实际 Ticket 文件位于：

```text
docs/FE20260903080401/tickets/
├── INDEX.md
├── 01-engineering-foundation.md
├── ...
└── 12-golden-path-e2e.md
```

这些 Ticket 使用 `BLOCKED_BY` 图管理可执行 frontier；应当一次在 fresh context 中实现一张，并以对应 Acceptance Criteria 判断 Done。当前仓库没有旧版本所指的 `.scratch/keystone-v1/`，该路径不是实施输入。

> 注意：Ticket 是 V1 Implementation Baseline 的执行拆分，不替代 Architecture v0.1，也不允许在实现过程中静默修改冻结架构。

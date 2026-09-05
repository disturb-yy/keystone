# CLI Change 变更操作要求调用者提供幂等键

M3 的设计规定：`change create`、`pause`、`resume`、`cancel` 与 `decide` 均要求调用者显式提供 `--idempotency-key`，CLI 将其映射为 Control Plane 的 `Idempotency-Key`。CLI 不为每次调用静默生成新键；这样同一逻辑操作的重试才能命中 Daemon 的持久 Receipt，而不同操作不会因客户端偶然重试而被误判为相同请求。读取命令不携带该键，也不会为查询启动 Daemon。

代价是脚本和操作者必须保存并复用重试所用的键；这是将重试语义留在调用者可见边界的必要代价。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

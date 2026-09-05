# Artifact 内容在读取时校验摘要

M3 的设计规定：`GET /v1/artifacts/{artifact_ref_id}/content` 成功时直接交付原始字节及保存的 `Content-Type`、`Content-Length` 和 SHA-256 派生的 `ETag`，不包装为 JSON，也不泄漏宿主机路径。每次读取均重算摘要；内容缺失或摘要不符不得返回给 Client，而以 `503 unavailable` 表示。M3 不支持 Range 或条件读取。

写入时的原子落盘只能避免新的悬空引用，不能证明日后磁盘内容未损坏；按引用读取时复核摘要可让 ArtifactRef 的完整性承诺在读取边界成立。M3 仅有至多 64 KiB 的 Intent，因此该取舍成本可控。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

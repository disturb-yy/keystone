# Change Intent 作为原样不可变 Artifact 保存

M3 的设计规定：创建请求只接受一个严格 JSON 对象，其中的 Intent 必须是有效 UTF-8、去除首尾空白后非空且原始长度不超过 64 KiB 的文本。校验成功后，原始文本不经规范化地保存为 `change_intent` Artifact，媒体类型为 `text/plain; charset=utf-8`；查询用摘要仅是空白归一化后的前 256 个 Unicode rune，不能替代或改写原始内容。未知字段和同一 HTTP body 中的多个 JSON 值均被拒绝。

这样 Artifact 的 digest 始终对应操作者实际提交的内容，审计与后续 Stage 获得稳定输入，同时列表和 Event 不携带大段文本。代价是输入兼容性演进必须显式进行，不能靠静默忽略字段或服务端改写文本。该决策只定义 M3 的设计合同，不表示 Ticket 05 已实施或解除其依赖。

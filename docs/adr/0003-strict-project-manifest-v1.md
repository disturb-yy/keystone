# 严格的 ProjectManifest V1 边界

V1 的 ProjectManifest 只表达版本与 ProjectID，不携带 RepositoryBinding、本机状态、端点、凭据或未实现的项目配置；Daemon 对它进行严格解释，未知、重复、损坏或不兼容的内容一律拒绝且不改写原文件。我们选择显式 schema 演进而不是静默兼容或忽略字段，以避免版本化 Repository 知识在协调时被无声丢失或误绑定。

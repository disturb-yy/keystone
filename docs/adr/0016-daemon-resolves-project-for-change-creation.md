# Daemon 为 Change 创建解析 Project

ChangeCreation 以绝对 repository_path 和 Intent 提交给 Daemon；Daemon 通过权威 RepositoryBinding 查找既有 Project 后才创建 Change。CLI 不读取 ProjectManifest、不直接提供 ProjectID，也不为取得 ID 隐式重放 ProjectInitialization。这样 Golden Path 的当前目录创建方式保持 Client 边界，同时不把 M3 扩展为通用 Project resolver。

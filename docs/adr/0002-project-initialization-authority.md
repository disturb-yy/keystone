# Project 初始化的身份与权威边界

V1 将 Project 识别为由版本化 ProjectManifest 关联的长期工程范围，并且同一 Project 同时只绑定一个活动的、非 bare Git 主工作树根；目录移动可以重新绑定，而并存的不同根或不兼容身份必须明确失败。`keystone init` 通过与 `keystone daemon start` 共享的 ensure seam 确保 Daemon 就绪，但不自行持有 SQLite 或进程生命周期；Daemon 经 Repository Port 完成 Git root、Manifest 与权威 Project 的协调。这样将重试幂等、ProjectInitialized 的首次事实和跨资源恢复决策保留在 Control Plane，而不是分散给 Client。

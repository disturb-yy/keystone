# Project 初始化的跨资源恢复顺序

ProjectManifest 文件与 SQLite 不能组成分布式事务，因此 V1 先由 Daemon 持久化可恢复的 ProjectInitializationIntent，再严格创建或核验 Manifest，最后在一个 SQLite transaction 中创建 Project、唯一的 ProjectInitialized 与成功回执。可恢复的文件失败保留 intent 供重试；不兼容身份不自动覆盖或猜测恢复。这个顺序避免在 Manifest 尚未确认时提前产生权威 Project/Event，同时让相同或不同幂等键的重试收敛到同一候选身份。

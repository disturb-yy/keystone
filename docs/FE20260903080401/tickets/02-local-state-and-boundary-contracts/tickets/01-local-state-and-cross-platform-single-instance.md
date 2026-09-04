# 01: 本地数据根与跨平台单实例状态

**What to build:** 让本机操作者能够确定性解析 Keystone 数据根、隔离测试目录，并在 Windows、Linux 与 WSL 上为同一数据根取得唯一实例锁。运行元数据可用于诊断 Daemon 的 PID、endpoint、实例标识和启动时间，但不会抢占或破坏活跃实例。

**Blocked by:** Ticket 01 Engineering Foundation 验收。

**Status:** implemented-with-native-windows-validation-gap

- [x] 默认用户级数据根与显式 `--data-dir` 均可确定性解析；显式相对目录在进程入口归一，派生状态、Artifact、Workspace 与运行子目录，且不读取或写入 Repository Manifest。
- [x] 初始化目录与运行元数据在平台支持时采用 owner-only 访问模式；路径解析保持无副作用，显式初始化才创建目录。
- [x] 同一解析后数据根的第二个获取者被明确拒绝，不同数据根可并存；当前 Linux/WSL 代码路径已有行为测试，Windows 目标已完成无 CGO 交叉编译。
  - [ ] 原生 Windows 锁行为仍需在 Windows 环境运行测试；交叉编译不替代该证据。
- [x] 锁是唯一实例权威；运行元数据原子发布并在正常关闭时清理，遗留 PID 或 endpoint 记录不能删除活锁、杀进程或覆盖活跃实例。
- [x] 现有配置、结构化日志和 UUIDv7 基础能力仍保持原有窄职责，不承担数据根、锁或进程协议。

验证记录：`go test ./...`、`go vet ./...` 和 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...` 已通过；原生 Windows 行为测试是剩余环境验证项。

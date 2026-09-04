# 05: Ticket 02 集成验收与导航对齐

**What to build:** 让后续实施者与验收者能够从准确的项目导航找到实际落地的本机状态、Migration 与两套 Contract 边界，并用统一验证确认它们共同满足 Ticket 02，而不误称 Daemon、Worker 或业务状态已实现。

**Blocked by:** 01: 本地数据根与跨平台单实例状态; 02: SQLite Migration 基线; 03: `/v1` Control Plane 边界 DTO; 04: Worker 基础边界 DTO。

**Status:** implemented-with-native-windows-validation-gap

- [x] 每个新增 Go package 均有局部 `AGENTS.md` 与 `INDEX.md`；受影响的父级与根级导航仅陈述当前实际文件树、职责和验证入口。
- [x] 集成检查证明数据根、持锁 Migration、Control Plane Contract 与 Worker Contract 的依赖方向没有让 Client 或 Worker 直接访问 Keystone SQLite。
- [x] Linux/WSL 的锁代码路径已有行为测试，Windows 使用 `LockFileEx` 的目标构建已通过；原生 Windows 锁行为仍需在 Windows 环境运行，交叉编译不替代该证据。
- [x] `go test ./...`、`go vet ./...`、`make build` 与 `git diff --check` 已在任务专用缓存/临时目录环境中通过；Dashboard 依赖目录未进入 Go package 列表。
- [x] 验收记录明确本 Ticket 不包含 Daemon 启动、HTTP 路由、业务 Schema、Worker 执行或 Artifact 生命周期。

验证限制：当前环境不是原生 Windows；另外 `go mod tidy` 受本地 file proxy 缺少 `modernc.org/gc/v3` 测试依赖缓存影响，已完成的源码测试、静态检查和构建不受该限制影响。

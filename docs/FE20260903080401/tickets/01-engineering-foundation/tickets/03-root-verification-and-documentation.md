# 03: 根级验证契约与文档事实对齐

**What to build:** 把已存在的 Go 基础能力和 Dashboard 骨架组合成单一、可从仓库根目录调用的验证体验，并使项目说明只陈述已存在的文件和可运行命令。贡献者与 Agent 可使用统一入口确认 M0 的测试、编译、静态检查和 Dashboard 生产构建结果。

**Blocked by:** 01: M0 合约对齐与 Go 基础能力; 02: Dashboard 构建骨架.

**Status:** ready-for-agent

- [ ] 根级 `test` 命令运行全部 Go 测试，根级 `build` 命令编译全部 Go package。
- [ ] 根级 `lint` 命令运行 Go vet 和 Dashboard lint，并确保 Dashboard 依赖按锁文件安装后可检查。
- [ ] 根级 `dashboard-build` 命令按锁文件安装 Dashboard 依赖并生成生产构建产物。
- [ ] 根级规约的事实段、README 与项目地图仅描述实际创建的 package、Dashboard、构建入口和可验证状态；架构规约保持不变。
- [ ] 文档不将目标架构、Daemon、Worker、Migration 或业务边界表述成当前已实现的运行行为。
- [ ] `go test ./...`、Go vet、四个根级验证入口、Dashboard 生产构建和 `git diff --check` 均通过。

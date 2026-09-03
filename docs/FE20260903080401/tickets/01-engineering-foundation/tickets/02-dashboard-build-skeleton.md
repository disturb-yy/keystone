# 02: Dashboard 构建骨架

**What to build:** 让 Keystone 拥有一个可重复安装、可静态检查并可生成生产构建产物的本地 Dashboard 骨架。贡献者能够看到静态 Keystone 占位界面，但不会把未实现的 Control Plane、Project、Change 或 Trace 能力误认为已可用。

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] Dashboard 使用 React、TypeScript、Vite 与 npm 锁文件，依赖安装可由锁文件确定。
- [ ] Dashboard 显示静态 Keystone 占位内容，并在浏览器可见内容中不承诺 Project、Change、Trace、API 或实时观察行为。
- [ ] 生产构建命令可成功生成 Dashboard 构建产物。
- [ ] 前端 lint 命令可在锁定的依赖树上运行并通过。
- [ ] 不创建路由、Control Plane 客户端、SSE、认证、业务状态、数据库访问或任何权威状态推导。

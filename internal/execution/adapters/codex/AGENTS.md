# codex adapter 包规约

该 package 只把受限 ExecutionInput 转换为固定的 Codex CLI 进程调用。

- 命令参数、Workspace cwd、stdin、stdout/stderr、退出码和 timeout 必须由
  Worker/Adapter 观察；不得信任 Runtime 自报的完成状态。
- 通过 ExecutionGuard 过滤环境并拒绝常见 Git commit、push、merge 命令。
- 不访问 Keystone SQLite、Worker Protocol、Lease 或 Change 生命周期。
- 不实现 OpenCode fallback、自动重试或完整 OS sandbox。

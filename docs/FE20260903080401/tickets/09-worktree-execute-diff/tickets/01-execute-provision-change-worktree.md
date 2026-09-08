# 09-01：显式 Execute 创建并可恢复 Change Worktree

**What to build：** 用户显式提交 Execute 后，Daemon 校验请求并为 Change 建立唯一、可追溯且可恢复的 Git Worktree，返回可安全重放的执行会话回执；此切片只建立执行会话和 Workspace，不启动 Runtime。

**Blocked by：** 顶层 Ticket 06、08 的真实实现与验收证据；它们的规划状态或 `ready-for-agent` 标签不解除阻塞。

**Status：** ready-for-agent（仅文档成熟度；顶层 Ticket 06、08 未解除）

- [ ] 首次有效 Execute 返回 `202 Accepted`；相同规范化请求重放返回同一不可变回执，幂等键冲突、活动会话冲突和版本冲突返回稳定安全的错误。
- [ ] Execute 前不创建 Worktree；Execute 接受后才持久化 provisioning intent，并在真实 Git Repository 上完成非 bare、连续双重 clean、HEAD 等于 BaseRevision 的校验。
- [ ] 默认 branch 与显式自定义 branch 均经过约束校验；Change 最多拥有一个物理身份一致的 Workspace，路径重叠、符号链接、branch、HEAD 或来源不确定时拒绝并进入 `human_required`。
- [ ] Git 写入和 SQLite 收敛可恢复：重试只协调完全匹配的 intent 结果，不删除目录、不执行 `worktree remove/prune`，也不猜测性修复。
- [ ] 执行读取模型和 CLI 只暴露安全摘要、会话状态与 Canonical Ticket 顺序，不暴露绝对 WorkspacePath、Lease、Prompt、凭据或运行环境；没有 Worker 时仍可保持等待且不会启动 Runtime。
- [ ] 使用临时真实 Git fixture 覆盖默认/自定义 branch、脏源、路径身份冲突、重复 Execute、Git/SQLite 中断和 API/CLI 回执。

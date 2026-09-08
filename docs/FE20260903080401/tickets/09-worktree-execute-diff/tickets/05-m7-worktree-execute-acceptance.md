# 09-05：M7 Worktree Execute/Diff 端到端验收与导航收口

**What to build：** 在前四张子票完成后，以真实临时 Git Repository 和受控本机执行链复核 M7 的完整 Worktree → Ticket Execute → Worker Diff → Daemon Evidence 路径，保存跨平台、脱敏且可复盘的验收摘要，并把导航同步到实际 checkout；不扩展到 Verify、Commit 或 Dashboard Golden Path。

**Blocked by：** 顶层 Ticket 06、08 的真实实现与验收证据；本地 Ticket 09-04：执行 Fence、恢复与人工重试边界。

**Status：** ready-for-agent（仅文档成熟度；顶层 Ticket 06、08 未解除）

- [ ] 临时 Repository 有干净 BaseRevision；显式 Execute 后只创建一个可追溯 Worktree，真实 Runtime 在 Assigned Workspace 形成源码 Diff，源 Repository 保持不变。
- [ ] 通过公开 CLI/API 观察 Execute 回执、ExecutionReadModel、Ticket 状态、Assignment/AgentRun、Artifact、Snapshot、Diff、Delta 和 Trace；不直接写 SQLite、不使用 debug seam、不把内部路径或 secret 写入证据。
- [ ] 完成一次默认 branch 的成功 Ticket 和至少一条失败/`human_required`/恢复路径；验证重复 Execute、重复 Report、失效 Lease Report 与控制命令竞争均符合权威结果。
- [ ] Linux 与 WSL 分别取得真实 Codex Worktree/Diff 验收证据；原生 Windows 完成真实 Git Worktree、路径/进程/协议/Fence 测试，交叉编译不替代原生运行证据。
- [ ] 运行完整 Go 测试、必要的 vet/build 和文档卫生检查；验收摘要只保留版本、非敏感标识、digest、状态和命令结果，失败或未执行项如实记录。
- [ ] 受影响的 Ticket/项目导航反映真实源码、测试和阻塞状态；不将规划规格、fake Runtime 或本票证据表述为已完成的 Ticket 10/12 行为。

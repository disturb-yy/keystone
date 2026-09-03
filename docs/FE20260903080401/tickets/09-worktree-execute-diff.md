# 09 — Worktree Execute and Diff

- 里程碑：M7
- `BLOCKED_BY`：06、08
- 交付类型：受控执行纵切

## 目标

让 READY Ticket 在一个真实 Change Git Worktree 内由 Codex 修改代码，并由 Worker 独立收集 Diff、变更文件与执行证据。

## 范围

- 在 `internal/execution` 定义 Workspace、SourceControl、Scheduler 和 Execution Authorization 边界。
- Change 创建时确认源 Repository clean 并固定 base revision；进入 Execute 时才创建 `~/.keystone/workspaces/<project-id>/<change-id>/` Worktree。
- 每个 Change 串行复用一个 Workspace；每个 AgentRun 独占当前 Assignment 的执行权。
- 支持默认 `keystone/change/<change-id>` 分支和受校验的自定义 branch name。
- Scheduler 只从 Canonical Graph 的 READY frontier 选择任务；默认低风险授权后才将 Assignment 下发给 Worker。

## 不包含

- Verify PASS、Git Commit、merge、push、PR 或 deploy。
- 高级跨 Change 并行、多个 Worker 或非 Git Workspace。
- 让 Codex 自行创建或选择 Workspace。

## 验收条件

- 真实 Git Repository 的 Change 在 Execute 前不会创建 Worktree，Execute 后在指定数据目录创建可追溯 Worktree。
- Codex 在 Assigned Workspace 中形成实际源码 Diff；Worker 记录 diff、changed files、exit status 和日志 Artifact。
- Runtime 无法将 commit、push、merge 或 Lifecycle 推进作为执行结果提交。
- 非 READY Ticket、无授权 Ticket、脏源 Repository 或无效 branch name 均不能获得 Assignment。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

另执行一次临时 Git Repository 中的真实 Worktree/Codex/Diff 验收。

## 实现边界

Worker 执行副作用，Daemon 持有调度、授权与状态真相。Worktree 是 Change 级资源，不是每张 Ticket 新建的独立权威空间。

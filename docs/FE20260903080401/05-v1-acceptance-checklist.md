# Keystone V1 Golden Path 验收清单

只有以下关键项全部满足，才可以宣布 V1 Thin Vertical Slice 跑通。

## Repository / Project

- [ ] 使用真实 Git Repository
- [ ] `keystone init` 成功
- [ ] `.keystone/project.yaml` 创建 / reconcile 正确
- [ ] Project 写入 SQLite
- [ ] ProjectInitialized Domain Event 可查询

## Change / Lifecycle

- [ ] Change 持久化
- [ ] Change 创建即 Start
- [ ] Intent Artifact 保存
- [ ] Understand 成功
- [ ] Design 成功
- [ ] Plan 成功
- [ ] Ticketize 成功
- [ ] Lifecycle 状态由 Daemon 权威持有

## Ticketize

- [ ] Structured Ticket Draft 可解析
- [ ] Keystone deterministic validation 通过
- [ ] Acceptance Criteria 存在
- [ ] BLOCKED_BY Graph 无环
- [ ] Canonical Ticket Graph 持久化

## Workspace / Runtime

- [ ] Change 创建时固定 Base Revision
- [ ] Execute 时创建真实 Git Worktree
- [ ] 自定义 branch name 可工作
- [ ] Worker 使用 Assigned Workspace
- [ ] real Codex CLI 被调用
- [ ] Runtime 实际产生源码修改
- [ ] Worker 独立采集 diff / changed files / exit status

## Verify / Git

- [ ] Workspace / Git Checks 通过
- [ ] Keystone-controlled verify command 运行
- [ ] 独立 Verifier Run 执行 Acceptance Criteria Review
- [ ] Implementer self-report 未直接作为 PASS Evidence
- [ ] Keystone 创建 commit
- [ ] 自定义 commit log / template 可工作
- [ ] Ticket before / after revision 可追踪
- [ ] Change-level Final Verify 通过
- [ ] Candidate Revision 可查询
- [ ] Change 到达 Integrate Ready

## Dashboard / Trace

- [ ] Projects 可观察
- [ ] Change Lifecycle 可观察
- [ ] Ticket / BLOCKED_BY 可观察
- [ ] Current Run 可观察
- [ ] Artifact 可观察
- [ ] Domain Event Trace 可复盘
- [ ] Daemon / Worker health 可观察
- [ ] Dashboard 没有直接修改 DB 或推导 Lifecycle Truth

# 12 — Golden Path E2E Evidence

- 里程碑：M9
- `BLOCKED_BY`：11
- 交付类型：最终可复盘验收

## 目标

以真实 Git demo Repository 和真实 Codex CLI 完整跑通 V1 Golden Path，并产出足以审计的端到端验收证据。

## 范围

- 在仓库中保存不含 `.git` 的 Go HTTP demo fixture；E2E 时复制到临时目录并执行 `git init`，形成真正独立的测试 Repository。
- demo 初始仅提供 `GET /` 返回 `hello`；创建的 Change 要求新增 `/healthz`，返回 HTTP 200、`application/json` 和 `{"status":"ok"}`，并新增自动化测试。
- 完整执行 `init → Project → Change → Intent → Understand → Design → Plan → Ticketize → Worktree → Codex → Diff → Verify → Keystone Commit → Final Verify → Integrate Ready → Dashboard Trace`。
- 收集每个阶段的 API/CLI 结果、AgentRun、Artifact、Event、revision、verify 输出和 Dashboard 观察证据。
- 将普通可重复测试与需真实 Codex 的显式本机 E2E 区分；宣布 V1 完成必须包含后者的成功证据。

## 不包含

- merge、push、PR、deploy、远程 Worker、远程 Repository 或长期运维。
- 用 mock Runtime 替代最终的真实 Codex 验收。
- 将嵌套 Git Repository 提交到 Keystone 主仓库。

## 验收条件

- demo 的 `/healthz` 满足全部六条 Golden Path Acceptance Criteria，原 `GET /` 行为保持不变。
- Canonical Ticket Graph 合法、Worktree 中存在真实 Diff、独立 Verifier PASS、Keystone Commit 与 candidate revision 均可查询。
- Change 到达 `integrate_ready`，但没有发生 merge、push、PR 或 deploy。
- Dashboard 可观察 Project、Change、Ticket Graph、Current Run、Artifacts、Events、Health 和最终 Trace。
- 一份端到端证据记录可复盘所有关键输入、命令、退出码、Artifact、AgentRun、Decision 和 Git revision。

## 验证

```bash
go test ./...
go vet ./...
make build
make dashboard-build
git diff --check
```

再执行显式真实 Codex Golden Path E2E，并保存完整验收记录。

## 实现边界

本 Ticket 只验证 V1 Thin Vertical Slice 已真实跑通。它不扩张 V1 到 Integrate Ready 之后的 Git 或部署操作。

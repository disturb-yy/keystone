# Keystone V1 Golden Path

## 1. 测试目标

测试仓库：

```text
demo/keystone-demo-service
```

初始状态故意只有：

```text
GET /
→ HTTP 200
→ hello
```

不预先实现 `/healthz`。

## 2. Change

```bash
keystone change create \
  "为 HTTP 服务增加 /healthz 健康检查接口，返回 JSON 状态，并添加自动化测试"
```

如需同时测试自定义 Git 行为：

```bash
keystone change create \
  "为 HTTP 服务增加 /healthz 健康检查接口，返回 JSON 状态，并添加自动化测试" \
  --branch feature/healthz \
  --commit-template "[{ticket_id}] {ticket_title}"
```

## 3. 建议 Acceptance Criteria

1. `GET /healthz` 返回 HTTP 200。
2. `Content-Type` 为 `application/json`。
3. Response body 为 `{"status":"ok"}`。
4. 原有 `GET /` 返回 `hello` 的行为不变。
5. 新增自动化测试。
6. `go test ./...` 通过。

## 4. 预期链路

```text
Repository
→ keystone init
→ Project
→ Change
→ Intent Artifact
→ Understanding Artifact
→ Design Artifact
→ Plan Artifact
→ Ticket Graph
→ Execute
→ Change Worktree
→ Codex
→ Diff
→ Verify
→ Keystone Commit
→ Change Final Verify
→ Integrate Ready
→ Dashboard Trace
```

## 5. Golden Path 不是 Golden Ticket Decomposition

不要求 Ticket Generator 固定生成几张 Ticket。

允许：

```text
T1: 实现 /healthz + 自动化测试
```

也允许生成多个符合 Contract 的 vertical slices。

验证重点是：

- Structured Draft 合法
- BLOCKED_BY Graph 合法
- Ticket 可执行、可验证
- Canonical Graph 由 Keystone 确认

## 6. 结束边界

Golden Path 到 `Integrate Ready` 为止。

V1 第一条 E2E 不要求：

```text
merge
push
PR
deploy
```

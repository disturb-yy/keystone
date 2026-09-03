# 10 — Verify, Commit and Integrate Ready

- 里程碑：M8
- `BLOCKED_BY`：09
- 交付类型：验证与 Git 收口纵切

## 目标

实现独立验证、Keystone 受控 Commit 和 Change-level Final Verify，使真实 Change 仅在完整证据成立后到达 Integrate Ready。

## 范围

- 读取 Project Manifest 中的 `verify.commands`，在 Assigned Workspace 运行 Workspace/Git Checks 与确定性验证命令。
- 创建只读的独立 Verifier AgentRun；Implementer 与 Verifier 不得是同一 Run，Verifier 不直接修复代码。
- Evidence 必须包含命令、exit code、输出 Artifact、输入 revision 与采集 AgentRun；Implementer 自报不是 PASS Evidence。
- 在 Ticket Verify PASS 后由 Keystone 创建 Commit，支持受校验的 commit log 或 template，并记录 Ticket 前后 revision。
- 实现 Change-level Final Verify、candidate revision 与 `integrate_ready` 状态转换。

## 不包含

- merge、push、PR、deploy、远程 Git 托管或自动回滚。
- 绕过独立 Verifier 直接接受 Implementer 的成功摘要。
- Runtime 执行 `git commit`。

## 验收条件

- 验证结果明确为 PASS、FAIL 或 HUMAN_REQUIRED；失败或人工要求不会创建 Commit 或进入 Integrate Ready。
- PASS Ticket 的 Commit 由 Keystone 创建，且前后 revision 与 Evidence 可查询。
- Change-level Final Verify 通过后可查询 candidate revision 和 Integrate Ready。
- 再次提交同一 Report、验证结果或 Commit Command 不会重复推进状态或生成重复 Commit。

## 验证

```bash
go test ./...
go vet ./...
make build
git diff --check
```

另执行一次真实临时 Git Repository 中的 Verify、Commit、Final Verify 验收。

## 实现边界

Integrate Ready 是 V1 的终点。Keystone 在本 Ticket 不执行 merge、push 或任何远程副作用。

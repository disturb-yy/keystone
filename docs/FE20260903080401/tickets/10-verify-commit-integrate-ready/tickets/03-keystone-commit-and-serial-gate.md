# 10-03：Keystone Commit、输入 Revision 链与串行 Gate

**What to build：** 在 Ticket Verify PASS 后由 Daemon 创建一次可恢复 KeystoneCommit，并以 CommitIntent、CandidateTreeIdentity 和受控 trailers 收口当前 Gate；只有收口成功后才允许 Scheduler 授权下一张 Ticket。

**Blocked by：** 顶层 Ticket 09 的真实实现与验收，以及 10-01/10-02 已形成的策略、Ticket Verify、Worker/证据链。开始前必须重读 SourceControl、Scheduler、Workstore、Change Command/Receipt、Daemon、CLI 和目标 package 的局部规约。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 在 `internal/infrastructure/sourcecontrol` 实现受类型约束的 Commit seam：重验 WorkspaceInputRevision/branch/Snapshot/no-untracked，以私有 snapshot 得到 CandidateTreeIdentity，`git add --all` 后确认 CommitTreeIdentity 相同，再通过无 shell 参数数组创建 Commit。
- 用 V2 模板或规范化 Ticket title 构造消息，限制 placeholder 和大小，拒绝 NUL/伪造受控 trailers；追加唯一 `Keystone-Change-ID`、`Keystone-Ticket-ID`、`Keystone-Commit-ID`。使用 Workspace 已配置 author/committer 和 `--no-verify`；身份缺失不创建 Commit。
- 建立 CommitIntent、KeystoneCommit、唯一 parent/tree/trailer 对账与权威 transaction；Git 成功前不产生同步 success Receipt，Git 成功后确认 direct parent/tree/clean Workspace 再记录 after revision、Gate 完成与 Event。
- 在 Scheduler/Execution authority 中使用 WorkspaceInputRevision：第一张为 BaseRevision，下一张为前一 KeystoneCommit after revision；每张 Ticket 的 Runtime 仍不得自行 commit，Ticket 09 的 HEAD 不变量比较输入 revision 而非全局 BaseRevision。
- 提供 `POST /v1/changes/{change_id}/tickets/{ticket_id}/commit` 与 `keystone change commit`；它们只接收 expected version/idempotency key，并在 KeystoneCommit 已记录时同步返回成功。
- 实现中断/重送恢复：只允许用 CommitIntent 与唯一 direct parent/tree/trailer 对账补齐记录；歧义、外部 HEAD 变化、tree/trailer 不符、dirty Workspace 或重复候选都进入 `human_required`。
- 实现 Ticket Verify FAIL/HUMAN_REQUIRED 的 Snapshot checkpoint 恢复，不自动 reset、clean、amend 或删除未提交候选。

## Acceptance

- 没有 PASS VerificationEvidence、错误 Gate、版本冲突、Snapshot/tree 不同、untracked、缺 author/committer 或外部变更时，Commit 不会运行 Git 写操作或返回成功。
- 真实临时 Git fixture 中，Commit 的 parent 是当前 Ticket WorkspaceInputRevision、tree 等于验证 CandidateTreeIdentity、消息含恰好三个正确 trailer、Workspace 完全干净，after revision 成为下一张 Ticket 的输入 revision。
- 两张串行 Ticket 证明第二张不再错误要求 `HEAD == BaseRevision`，同时仍拒绝 Runtime 自行改变 HEAD/branch。
- Git 成功/SQLite 中断、相同 Command 重送、Daemon 重启、多个匹配候选、错误 parent/tree/trailer 和 dirty Workspace 都不会生成第二个 Commit；只有唯一完整匹配可收敛既有 Intent。
- Gate 完成前 Scheduler 不产生下一张 Ticket 的 Authorization/Assignment；完成后仅按既有 canonical/FIFO 规则继续。
- Verify FAIL 的修复只能在相同未 Commit Ticket 的已知候选 Snapshot 上增量执行；fence 或未知变化仍保持 human_required，测试证明没有自动清理。

## Out of scope

- Final Verify、CandidateRevision、Integrate Ready、最终读取模型（10-04）。
- merge、push、PR、deploy、rebase、amend、reset、force、自动 rollback/cleanup 或任意 Git 代理。
- Dashboard、远程 Git、真实 Codex Golden Path 和 Worker 协议的第二版本。

## Verification

```bash
go test ./internal/infrastructure/sourcecontrol/...
go test ./internal/execution/...
go test ./internal/governance/...
go test ./internal/infrastructure/workstore/...
go test ./internal/daemon/...
go test ./cmd/keystone/...
go test ./...
go vet ./...
make build
git diff --check
```

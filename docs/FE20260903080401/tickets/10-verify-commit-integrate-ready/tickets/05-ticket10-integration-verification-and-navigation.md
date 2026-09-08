# 10-05：Ticket 10 集成验收、恢复证据与导航

**What to build：** 以真实临时 Git Repository、受控 fake Worker/Verifier 和现有 CLI/Daemon seam 验收完整 M8 证据链，补齐受影响导航文档；不扩大新的业务能力。

**Blocked by：** 顶层 Ticket 09 的真实实现与验收，以及 10-01 至 10-04 的实现和测试证据。若任何前置仍只有规划文档、fake 成功或缺少本机证据，本子票不能宣布完成。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 以不含 `.git` 的受控 fixture 复制到临时目录后 `git init`，运行至少两张有依赖的 CanonicalTicket，验证 BaseRevision、WorkspaceInputRevision、TicketDelta、Verify、Commit、Final Verify 和 CandidateRevision 的完整链。
- 验证 V1 Manifest 进入 human_required、严格 V2 digest、旧/新 `worker/v1` capability 共存、无 Worker pending、命令 FAIL/timeout/not_run、截断/候选变化 HUMAN_REQUIRED、Criterion 覆盖和同一 Report/Command 重送。
- 验证 CommitIntent 在 Git/SQLite 中断两侧的恢复、正确/错误 parent-tree-trailer、外部 HEAD/Workspace 变化、Snapshot checkpoint、Fence、Ticket retry、Final FAIL 新 Change 和 Final HUMAN 未变重验。
- 分别在 Linux、WSL 和原生 Windows 运行真实 Git fixture；Windows 记录必须来自原生执行，不能用交叉编译或仅 Linux/WSL 成果替代。
- 更新实际受影响 Go package `INDEX.md`、根 `INDEX.md`、本 Ticket 导航和 ADR 索引；只描述已在 checkout 验证的实现/测试，不把规划或未运行 smoke 记为成功。
- 运行根验证并记录精确命令、结果、已知未执行项和剩余风险。真实 Codex smoke 若依赖环境不可用，必须与 fake seam 验收明确区分。

## Acceptance

- 临时真实 Git repo 显示两张 Ticket 的 parent 链、正确受控 trailers、无重复 Commit、最终干净 Workspace、PASS FinalVerification、CandidateRevision 和 `integrate_ready`；没有 merge/push/PR/deploy。
- 每个故障分类都有可复盘的 authority 状态：pending、unavailable、FAIL、HUMAN_REQUIRED、LateReport Trace 或成功 Receipt 不互相混淆。
- Git/SQLite 中断、重复/并发请求和 Worker fence 均不产生第二个 Commit、第二个成功 verdict、错误下一张 Assignment 或被静默丢弃的候选。
- Linux、WSL 和原生 Windows 的证据均包含实际命令、退出结果和必要安全摘要；缺失任一原生目标时，完成记录明确标注为阻塞。
- `go test ./...`、`go vet ./...`、`make build`、`git diff --check` 全部通过，且导航文件不把 M8 目标契约叙述为未经验证的当前事实。

## Out of scope

- 新功能、Dashboard、远程 Git/Worker、自动部署、完整 Golden Path UI 或 Ticket 11/12 的交付。
- 清理、重置、覆盖本子票之外的用户工作树改动，或为了让验收通过而弱化 Snapshot、Lease、Receipt、CommitIntent 或原生 Windows 要求。

## Verification

```bash
go test ./...
go vet ./...
make build
git diff --check
```

另执行本子票 Scope 中的 Linux、WSL、原生 Windows 临时 Git fixture；只在真实执行成功后记录成功证据。

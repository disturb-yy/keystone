# 07-04：Isolated Snapshot 与 Daemon 自动恢复

**What to build：** 为 Planning 提供固定 `base_revision` 的临时隔离 source Snapshot，并把 Coordinator 接入现有 Daemon 生命周期，使 Change 创建后的推进和进程重启后的恢复都来自耐久事实。

**Blocked by：** 07-02、07-03、顶层 Ticket 06；Snapshot 的具体 Git/Worker 前置能力必须先根据当前 checkout 重新核验。

**Status：** implemented-with-native-windows-validation-gap

## Snapshot boundary

- Planning 只消费一个由 port 描述的已验证 Snapshot，不消费原始 Repository 工作树的任意绝对路径。
- Snapshot 固定到 Change 的 `base_revision`，必须验证 Git root、revision、相对路径边界和清理结果。
- Snapshot 不得修改原始 Repository，不得创建生产 Change Worktree，不得把临时路径写进外部 Artifact、Event、日志或 Control Plane response。
- 具体 materialization 机制在实现阶段选择，但必须可测试、可清理、失败可观察，并能证明同一 revision 输入的一致性。

## Package ownership decision

实现优先复用现有 `internal/infrastructure/repository/`，不创建新的 Snapshot package。由于当前该 package 的局部规约只允许只读 Git 识别，若临时 materialization 需要扩大职责，必须在同一变更中先更新其 `AGENTS.md` 和 `INDEX.md`，明确：

- 原始 Repository 仍只执行不改变源事实的操作；
- 临时目录的写入不等于修改源 Repository；
- Snapshot 的路径、revision、清理和错误不变量；
- 不把 raw Git output 或宿主机绝对路径暴露给 HTTP/外部 Artifact。

如果在实现前无法满足上述边界，必须停在所有权阻塞点重新对齐；不得悄然新增泛化基础设施目录。

## Daemon recovery

- Daemon 在进入 readiness 前同步扫描一次 active 且可恢复的 Change；后续周期和事件唤醒仍从 Workstore 读取事实，不依赖进程内队列。
- 已完成 AgentRun 不重复执行；拥有 running AgentRun 的 Change 按 Ticket 06/Work 的 Lease、Worker 和 attempt 围栏分类处理。
- 启动、暂停、取消、Resume 和人工 retry 都遵循“先保存事实，再决定是否调度”的顺序。
- 成功且已被 fence 的结果可以在 Resume 后按当前 stage/attempt/base revision 重新评估；失败结果使 Change 保持 `human_required`；取消或晚到结果永不推进。
- 不新增公开 `planning start` endpoint；Daemon 内部 composition 只接现有 Application/port。

## Acceptance

- 临时 Snapshot 的 revision、路径、清理、原始工作树不变和失败行为有集成测试。
- Daemon 启动恢复只会调度当前有效阶段，不重复启动已有完成事实，不让旧/晚到结果推进 Change。
- 端到端测试使用 fake Runtime/Worker seam 验证恢复顺序；真实 Codex smoke 由 Ticket 06 的前置验收提供，不能用本票伪造。
- `internal/daemon` 只负责 composition/lifecycle，不直接写 Planning SQL；`repository` 所有权变化已同步局部文档。

## Out of scope

- Ticket 09 生产 Worktree、Scheduler、Execution DAG 和真实 Ticket 执行。
- Remote Worker、消息队列、自动 retry、完整 OS sandbox 和 Dashboard 控制页。

## Verification

```bash
go test ./internal/daemon/... ./internal/infrastructure/repository/... ./internal/planning/...
go test ./...
go vet ./...
git diff --check
```

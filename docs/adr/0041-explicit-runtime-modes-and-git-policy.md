# ADR-0041：显式 Runtime 模式与 Git 策略

## 状态

已接受。

## 决策

Ticket 09 的执行信封必须绑定不可隐式升级的 ExecutionMode：

- `inspect` 使用只读 Runtime，只允许经 Guard 中介的 `status`、`diff`、`rev-parse`、`show`、`log`、`ls-files` 与 `check-ignore`。
- `edit` 是 Execute 的默认模式，使用可写的已授权 Workspace，并在 `inspect` 的允许集合外仅允许 `git add`。
- `verify` 是 Ticket 10 的只读验证模式；Worker 只可在 Workspace root 按 Manifest 的固定参数数组执行验证命令，并以 `inspect` 的 Git 读取集合审查固定证据，不被授予 `git add`、候选修改或自由 Prompt 权限；任何观测到的候选变化不能形成 PASS。
- `source_control` 仅是 Daemon 内部的受类型约束能力，用于 Worktree、身份核验、快照与恢复对账；它不是 Worker 或 Codex 可取得的 Runtime 模式。

所有其余经 Guard 中介的 Git 子命令均被拒绝。M7 不提供泛化的高权限 Runtime；未来需要改写历史、切换分支或清理工作树时，必须通过独立 Ticket 定义具体 SourceControl 动作和人工审计入口。

## 理由与边界

显式模式使“只观察”“执行源码修改”“只读验证”和“Daemon 管理 Git 事实”互不混淆，避免 Agent 获得未记录的 Repository 管理权。`edit` 保留 `git add` 以形成完整 staged/unstaged 证据，但不允许 Runtime 选择 Workspace、分支或提交历史；`verify` 的候选不变性仍由前后 WorkspaceSnapshot 观测，而非声称已实现完整 OS 隔离。

PATH Guard 不是完整操作系统沙箱，不能单独阻止绝对路径 Git、其他工具或直接文件修改；WorkspaceSnapshot、Git 身份核验与完整证据仍是成功条件。该 ADR 冻结目标契约，不证明 Runtime sandbox、Guard allowlist 或 SourceControl 已实现。

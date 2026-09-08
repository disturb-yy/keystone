# 06-04：Runtime Adapter、Codex、Evidence 与 Guard

**What to build：** 建立 Worker 可替换的 `RuntimeAdapter` 执行端口，实现第一版 `CodexAdapter`，并在 Assigned Workspace 中独立采集 stdout、stderr、exit code、diff、changed files 和 Git/Guard 事实。

**Blocked by：** 顶层 Ticket 05：Change Lifecycle、Artifact 与 Event；本地 Ticket 02：Assignment、Lease 与 Report Authority。

**Status：** ready-for-agent（仅文档成熟度；顶层 `BLOCKED_BY: 05` 未解除）

- [ ] 在 `internal/execution` 定义 RuntimeAdapter、受限执行输入、RuntimeResult、evidence capture 和 ExecutionGuard Port；Domain 不依赖 HTTP、SQL、Codex SDK 或 shell 实现。
- [ ] `internal/execution/adapters/codex` 实现 CodexAdapter，默认 PATH 发现 `codex`，命令固定为 `codex exec --json --ephemeral --sandbox workspace-write --ask-for-approval never -`。
- [ ] Prompt 通过 stdin 写入，`exec.Cmd.Dir` 固定为 Assigned Workspace；stdout 与 stderr 通过独立管道采集，Runtime JSON/自然语言不作为权威 outcome。
- [ ] 单次 Runtime 默认 timeout 30 分钟；超时、启动失败、不可用和非零 exit code 形成明确失败/不可用事实，不自动二次执行或切换 OpenCode。
- [ ] Worker/Adapter 独立采集 exit code、开始/结束时间、before/after revision、diff 和 changed files；changed files 去重排序，均为 Workspace 相对路径。
- [ ] Report Artifact 固定为 stdout、stderr、diff、changed_files，默认上限分别为 16 MiB、16 MiB、16 MiB、1 MiB，单个 Report 总计 64 MiB；超限显式 truncated，捕获/编码/摘要错误显式 failure。
- [ ] ExecutionGuard 验证既有 Workspace/Git root、执行前后 HEAD，并在 Runtime PATH 前置 Git 拒绝 wrapper，至少拒绝 `git commit`、`git push`、`git merge`；发现越界不能产生可推进的 succeeded。
- [ ] 环境筛选不携带 Worker protocol secret、Lease token、Keystone DB 连接或控制面身份；不使用 dangerous/full-access 绕过。
- [ ] OpenCode 只保留 RuntimeAdapter interface/capability 位置，不实现命令、不回退、不在 smoke 中声明支持。
- [ ] 使用 fake RuntimeAdapter 和 fake command seam 测试 cwd、stdin、输出分流、timeout、PATH wrapper、HEAD 检查和 Artifact 摘要；真实 Codex 端到端由 06-05 验证。

## RuntimeAdapter boundary

建议的纯 Port 形态是“已验证的执行输入 → 可观察的 RuntimeResult”，而不是把 `exec.Cmd` 或第三方 SDK 泄漏到 Domain：

- 输入包括 Workspace 身份、已验证的本机路径、有限 instruction、runtime name、timeout、before revision 和允许的环境摘要。
- 结果包括启动/结束观察时间、exit code、stdout/stderr 字节、after revision、diff/changed files capture、Guard findings 和 capture failures。
- Adapter 不产生 AgentRunOutcome、HumanDecision、StageAdvanced、ChangeVersion 或 Event；Daemon/06-02 负责把结果解释成权威事实。
- Worker 不从 RuntimeResult 推断“已提交”“已验证”或“Ticket DONE”；这些内容不在 M4 的执行权限内。

## Codex invocation

固定命令：

```text
codex exec --json --ephemeral --sandbox workspace-write --ask-for-approval never -
```

实现要求：

- 使用 `exec.Cmd.Dir`，不要依赖 `--cd` 或 Prompt 让 Runtime 自己选择 cwd。
- stdin 单次写入后关闭；stdout/stderr 独立、并发读取并受有界缓冲保护，避免管道阻塞。
- 默认通过 PATH 查找 `codex`；命令路径、时钟、进程启动和 RuntimeAdapter 可注入，便于 Windows/fake 测试。
- 环境以安全基线构造，保留 Codex 执行所需的非协议运行环境，但绝不注入 Worker secret、Lease token、Keystone DB DSN 或内部控制面凭据。
- `workspace-write` 和 `ask-for-approval never` 是 M4 的固定 smoke 约束；不能用 dangerous/full-access 作为失败绕过。
- 本机 Codex 认证是否可用是运行环境事实。日志和 `06-smoke-evidence.md` 只能记录可用/不可用及版本，不记录 token、用户目录或完整环境。

## Evidence capture

Worker 在 Execute 前记录可解析的 `before_revision`，在 Execute 后重新获取 `after_revision`、diff 和 changed files。diff 可以使用稳定的 Git binary/文本输出；changed files 需从 Git 状态/差异事实得到稳定的相对路径集合，不能接受 Runtime 自报的绝对路径列表。

四类 Artifact 使用强类型 payload：

```json
{
  "kind": "changed_files",
  "content_base64": "...",
  "sha256": "64 位小写十六进制摘要",
  "size_bytes": 42,
  "truncated": false,
  "capture_error": null
}
```

Daemon 负责最终摘要/长度/相对路径校验、原子写 Local Artifact Store，并在 06-02 的 transaction 中建立 AgentRunArtifactLink。无法完整采集 changed files、无法编码或摘要不匹配时返回 `unavailable`，Worker 重试同一 Report，不重跑 Codex。stdout/stderr/diff 的有界前缀如果被保存，必须标记 `truncated: true`；截断 diff 不能成为 smoke 的完整变更证据。

## ExecutionGuard

Guard 提供可验证的有限边界：

1. 校验 Assigned Workspace 已存在、是预期 Git root、路径解析不逃逸且可写；M4 不创建或修复 Worktree。
2. 在 Runtime PATH 前置 Git wrapper，拒绝常见 `commit`、`push`、`merge` 子命令，并阻断将 control-plane secret 作为环境传递。
3. 执行前后解析 HEAD；HEAD 改变、出现不可解释的切换或禁止 Git 事实时将结果标为 Guard failure。
4. 采集 diff/changed files 并将 finding 和 capture failure 传给 Report Authority；Guard 不直接写 Event/Change。

这不是完整 OS sandbox。绝对路径调用、恶意同用户进程、未覆盖的工具和任意 shell 绕过不能由 Prompt 禁令或 wrapper 单独保证；测试必须明确覆盖的是可验证边界而不是全能安全承诺。

## Acceptance

- fake Runtime 测试证明 cwd、stdin、固定 Codex args、stdout/stderr 独立采集、timeout 和 exit code 事实。
- Codex Adapter 不把 Runtime JSON/natural language 当作 AgentRun authority；非零 exit、启动失败和 timeout 形成可分类的 failed/unavailable。
- Artifact 上限、摘要、长度、截断、capture failure、相对路径和 total body limit 均有测试，绝对路径和 secret 不进入 Report/日志。
- Guard 能拒绝常见 commit/push/merge，并在执行后发现 HEAD 改变；没有测试或文档声称它提供完整 OS sandbox。
- OpenCode 只有接口/capability 位置，Codex 不可用时 Report `runtime_unavailable`，不回退、不重试第二 Runtime。

## Verification

执行 `internal/execution` Domain/Adapter/Guard/Artifact capture 测试、`go test ./...`、必要的 `go vet ./...`、`make build`。在 Linux/WSL 由 06-05 使用真实 Codex 验证，原生 Windows 使用 fake Runtime 验证进程/协议/路径边界；交叉编译不是原生执行证据。

## Out of scope

- Worker 子进程、Register/Heartbeat/Pull/Report loop 和 Supervisor（06-03）。
- Report authority、Lease、AgentRun/Change/Event transaction（06-02）。
- Ticket 09 Worktree 创建、生产 Execute/Diff 协调、Ticket scheduler、Verify approval、远程 Runtime 和完整 OS sandbox。

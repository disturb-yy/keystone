# 07-02：Stage Strategy、Prompt、Decoder 与 Fake Runtime

**What to build：** 为 Understand、Design、Plan 实现纯策略边界、固定输入映射、Prompt 构造、候选结果 decoder 和 fake Runtime seam。策略只产生候选结果，不拥有 Change、AgentRun、Event 或数据库权威。

**Blocked by：** 07-01、顶层 Ticket 06。RuntimeAdapter 的具体实现由 Ticket 06 提供；Ticket 07 只能依赖稳定的窄接口，不能把 Codex 命令行细节带进 Planning。

**Status：** ready-for-agent（仅文档成熟度）

## Stage contract

| Strategy | 输入 | 输出 |
| --- | --- | --- |
| Understand | Intent、ProjectContext、base revision | Understanding candidate |
| Design | 已验证 Understanding、ProjectContext、base revision | Design candidate |
| Plan | 已验证 Design、ProjectContext、base revision | Plan candidate |

每个 Strategy 必须显式声明 stage、schema version、必需上游输入和允许的 Runtime capability。缺少上游 Artifact、revision 不同或 Context 版本不支持时，返回可分类失败，不猜测或补造事实。

## Scope

- 为三个 Strategy 提供统一的调用接口和最小 Prompt 输入模型；Prompt 不包含协议 secret、数据库连接、绝对宿主机路径或未授权文件内容。
- Decoder 将 Runtime 的候选字节转换为严格结构化 payload，再交由 07-01 validator；Runtime 自报 status、文件列表或自然语言不能直接成为 authority。
- 提供 fake Runtime/clock seam，测试成功、非零退出、timeout、空输出、非法 JSON、schema invalid 和上游引用不匹配。
- 记录原始 Runtime 输出的有界引用/摘要供 Coordinator 形成 Failure/raw log Artifact；不在 Strategy 内写 Artifact store。

## Acceptance

- 三个 Strategy 的输入映射与固定 `base_revision` 可单测验证，且执行顺序只能是 Understand → Design → Plan。
- fake Runtime 能证明 Strategy 不访问 HTTP、SQLite、Worker 具体实现或原始 Repository 工作树。
- Runtime 返回合法候选不等于阶段成功；只有 Decoder + Validator 通过后才返回可提交的 candidate。
- Runtime 失败、解析失败和 schema 失败都保留可观察的分类，不触发下游 Strategy。

## Out of scope

- AgentRun 持久化、Change 推进、Artifact 内容写入、Lease/Report authority 和启动恢复。
- 真实 Codex 调用、OpenCode fallback、Ticketize 或生产 Worktree。

## Verification

```bash
go test ./internal/planning/...
go test ./...
git diff --check
```

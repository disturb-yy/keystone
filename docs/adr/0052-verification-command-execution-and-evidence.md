# ADR-0052：固定验证命令与独立判定证据

## 状态

已接受。

## 决策

Ticket Verify 与 Final Verify 均先形成不可变 VerificationIntent，再由 Worker 使用 `verify` ExecutionMode 在 Assigned Workspace root 顺序执行 VerificationPolicySnapshot 的固定 `verify.commands`。无可用 Worker 时 intent 保持 pending；只有 Assignment、Lease 与 RuntimeClaim 就绪后才创建 VerifierAgentRun。Worker 不接受 Client 提供的命令、cwd、环境变量或 shell 文本。命令首次非零退出或 timeout 时停止后续执行，并把未启动的已声明命令持久化为 `not_run`；未形成 verdict 的 Artifact/SQLite 暂时不可用允许同一请求重新驱动 intent，已 Claim 后的 Lease 或 Runtime fence 仍按 Ticket 09 进入 `human_required`。

Daemon 为每条命令保存有界输出 Artifact、状态、可用时的 exit code、采集 Worker/AgentRun、输入 revision、验证前后 WorkspaceSnapshot 与 VerificationPolicySnapshot。随后独立于 Implementer 的 VerifierAgentRun 只读审查这些输入，并对每条 Acceptance Criterion 输出 `criterion_ordinal`、规范化文本 SHA-256、PASS/FAIL/HUMAN_REQUIRED 与 Evidence 引用。结果必须按 canonical ordinal 恰好覆盖每条 Criterion；重复、遗漏、乱序、摘要不符或结构非法均为 HUMAN_REQUIRED。PASS 仅在全部命令成功、快照不变且每条验收标准均 PASS 时成立；命令失败或 timeout 为 FAIL。

## 理由与边界

把命令执行观察与独立验收判定分开，可避免 Implementer 的成功摘要成为自证。固定参数数组、顺序、快照与配置 digest 让每次 verdict 的输入可复盘；失败后记录 `not_run` 避免未声明的额外副作用。`verify` 只建立可观察的候选不变性，不宣称提供完整操作系统沙箱；任何观察到的候选变化都不能形成 PASS。该 ADR 不实现 Worker Assignment、Artifact 存储、Verifier Runtime 或 Ticket 10 代码。

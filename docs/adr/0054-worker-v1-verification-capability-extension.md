# ADR-0054：Worker V1 的验证能力扩展

## 状态

已接受。

## 决策

Ticket 10 保持既有 `worker/v1` 的 Register、Heartbeat、Pull、Report 路由和 `protocol_version: "v1"`，不引入 `worker/v2`。支持 Ticket 10 的 Worker 在 Register 的 `capabilities` 中声明 `verification-v1`；Daemon 只向已声明该能力的 Worker 分配 Verify 或 FinalVerify 工作。未声明该能力的旧 Worker 继续只接收既有 edit Assignment，不能因升级后的未知字段而被错误分配验证工作。

V1 Assignment 增加 capability-gated 的显式变体：`kind: "verify"` 与嵌套 `verification`。既有 Assignment 缺省 `kind` 时仍表示 edit。`verification` 只携带 Daemon 从不可变 VerificationPolicySnapshot 派生的命令顺序、参数数组、timeout、输入 revision、WorkspaceSnapshot 身份和结构化验收审查输入；它不接受 Client 自由命令、cwd、环境变量、Prompt、Git 参数、生命周期命令或数据库凭据。

V1 Report 对 `kind: "verify"` 增加 capability-gated 的 `verification` 结果：每条已声明命令的 ordinal、状态、可用时 exit code、受限 stdout/stderr 内容身份和明确的截断事实，以及逐条 AcceptanceCriterionResult。命令输出按命令 ordinal 和 stream 作为 VerificationEvidence 的类型化 Artifact 引用保存，不增加通用 WorkerArtifactKind；既有 `Report.Artifacts` 仍严格只允许 stdout、stderr、diff、changed_files 四种。Daemon 必须将结果与原 Assignment 的顺序、命令、摘要和 Criterion 引用逐项比对，Worker 的 `Report.Outcome` 只表示该 VerifierAgentRun 的传输终态，不能自行决定 VerificationOutcome。

验证 Assignment 与普通 Assignment 一样受 Lease、RuntimeClaim、Report 幂等和围栏约束。Verify 的 Worker 在 `verify` ExecutionMode 下运行固定命令并只读审查；任何观察到的候选 Workspace 变化、Guard finding、无法完整采集的必要证据或已 Claim 后的 fence 都不得形成 PASS。

## 理由与边界

能力协商允许已部署的 V1 Worker 与支持验证的 V1 Worker 并存，避免仅为可选的窄工作类型中断现有本机 Worker。显式 `kind` 防止把验证计划伪装成普通 Runtime instruction；类型化验证结果保留逐命令和逐 Criterion 的可审计性，同时不破坏 ADR-0035 已冻结的四类通用执行 Artifact。该决定定义 Ticket 10 的目标协议演进，不证明新增 DTO、Worker runner、Daemon authority 或 RuntimeClaim 已实现。

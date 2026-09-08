# ADR-0042：执行 timeout 与 Artifact 输入投影

## 状态

已接受。

## 决策

M7 的每个 ExecutionEnvelope 固定 `30m` ExecutionTimeout。Daemon 将该时长同时写入 ExecutionEnvelope 和 Worker Assignment；Worker 从实际启动 Runtime 起以该 deadline 运行。Lease 续约不延长 Runtime timeout，Client、Worker 本地配置和 Runtime 都不能覆盖它；超时取消 Runtime 并以 `runtime_timeout` 形成失败结果。

Daemon 在形成授权前读取并校验所有已引用 ArtifactContent，按照冻结的引用顺序渲染为 UTF-8 的 ExecutionInstruction，并将输入 Artifact 摘要和指令摘要共同固定。M7 的渲染结果最多 64 KiB；Worker 不取得 Artifact 下载权限、物理路径或本地挂载。临时存储不可用允许同一会话稍后重试，缺失、摘要不符、二进制、媒体类型不支持或超限则进入 `human_required`，不创建 Assignment。

## 理由与边界

timeout 必须是 Daemon 的权威执行输入，而非某台 Worker 的默认配置，才能在 Assignment、Runtime 和审计中复盘相同约束。由 Daemon 投影内容保留窄 Worker Contract，避免为 Artifact 服务、凭据和路径暴露另开读取边界。

该 ADR 不定义未来的可配置 timeout、二进制上下文格式或大 Artifact 传输协议；这些需要独立设计，不能由 Client 自由 Prompt 或 Worker 自行下载绕过。

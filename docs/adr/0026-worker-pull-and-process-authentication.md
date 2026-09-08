# ADR-0026：Worker Pull 与进程级协议鉴权

## 状态

已接受。

## 决策

V1 让 Daemon 监管一个长期本机 Worker，而由 Worker 通过 loopback `WorkerProtocol` 主动 Pull `Assignment`。Daemon 为每个 `WorkerInstance` 生成临时的进程级 Bearer secret，并通过启动管道交付；secret 不进入 JSON、命令行参数或 Runtime 环境。

Worker 使用 Register、Heartbeat、Pull、Execute、Report 完成一次执行闭环。`Execute` 是 Worker 内部的本地动作，不作为 Daemon 的公共 HTTP 控制面。协议 DTO 只表达传输边界，不解释 Change、Ticket 或 Lifecycle。

## 理由与边界

Pull 模式让 Daemon 保持调度权威，同时使 Worker 可以在未来替换为独立的 Remote Worker，而不把 Worker 进程实现耦合进 Control Plane API。启动管道交付凭据可避免把可复用 secret 暴露在协议请求体、进程参数或 Codex 子进程环境中。

V1 不因此扩展为 Remote Worker、TLS 或完整身份系统。Worker 的存活由独立心跳和进程监管观察，不能把 Worker 的注册或 Runtime 自报结果当作 Daemon 的生命周期权威。

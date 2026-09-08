# ADR-0043：可重放 Assignment 交付与一次性 Runtime Claim

## 状态

已接受。

## 决策

Daemon 向已注册 Worker 创建 Assignment 与 Lease 后，`Pull` 只交付同一份不可变 Assignment；同一 Worker 的重试可取得相同内容，但不得因此启动 Runtime。Worker 必须生成 RuntimeClaimID 并请求 RuntimeClaim。Daemon 在一个权威事务中只把第一个有效 Claim 绑定为该 Lease 的启动资格；同一 Claim 的重试返回相同结果，不同 Claim、不同 Worker、失效 Lease 或已围栏 Assignment 都不能取得启动权。

每个 Worker 同时至多持有一个已 Claim 或运行中的 Assignment。Worker 只有收到成功 Claim 后才可启动 Runtime；Claim 后尚未启动即丢失 Worker，也按 Lease 围栏进入 `human_required`，不自动重派。

## 理由与边界

Pull 的网络响应可能丢失，因而必须可安全重放；但网络重试不能等价于新的 Runtime 启动资格。独立 Claim 将“可重复收到任务”与“唯一取得写入 Workspace 的权利”分开，同时保持 Worker Contract 的窄边界。

本决定会增加最小 Claim DTO/端点，不增加 Artifact 种类、不授予 Worker 额外权威状态，也不定义多 Worker 调度或自动恢复。

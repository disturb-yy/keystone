# Change 检查点与运行状态分离

Change 的 LifecycleStage 记录最近一个已确认、已持久化的检查点，初始为 Intent，并按 Understand、Design、Plan、Ticketize、Execute、Verify、FinalVerify 前进；ChangeStatus 独立表达是否允许继续协调。这样 Pause、Human Required 与 Cancel 不会覆盖已经形成的阶段事实；`integrate_ready` 是只在 FinalVerify 成功后由 Ticket 10 写入的终态 Status，不是 M3 可进入的 Stage。

M3 只允许 active 到 paused、paused 到 active、active/paused/human_required 到 cancelled、active 到 human_required，以及 human_required 经 Retry HumanDecision 回到 active 并创建新 AgentRun。LifecycleCoordinator 是唯一可以依据已验证 Strategy 结果推进 Stage 的主体；Client 永远不能指定目标 Stage 或 Status。

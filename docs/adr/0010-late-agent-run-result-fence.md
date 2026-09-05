# Pause 与 Cancel 隔离晚到 AgentRun 结果

Pause 和 Cancel 都不强制终止已经开始的 AgentRun；其晚到结果及 Artifact 仍可被记录以保留事实。只有 Change 仍为 active 且结果对应当前检查点时，LifecycleCoordinator 才能使用结果推进 Stage；paused 时保存结果并在 Resume 后重新判定，cancelled 后结果永久只作为 Trace，绝不恢复或推进生命周期。

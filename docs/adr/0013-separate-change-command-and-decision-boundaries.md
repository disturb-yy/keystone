# Change Command 与 HumanDecision 分离

Pause、Resume、Cancel 是 ChangeCommand，使用统一的幂等键和 ChangeVersion 前置条件；Retry 与在 human_required 时的 Cancel 则是 HumanDecision，必须另经 Decision 边界创建追加事实。分离这两个入口可避免 Client 通过通用命令伪装人工恢复，并让状态控制与恢复理由在 Trace 中保持可区分。

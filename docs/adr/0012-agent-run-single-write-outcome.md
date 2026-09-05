# AgentRun 的一次性终态

AgentRun 在创建后固定其身份、Stage、attempt、输入与关联 ArtifactRef，只允许从运行中写入一次 succeeded、failed 或 human_required 终态。输入、输出和失败证据由专用 `t_agent_run_artifact_refs` 以 input、output、failure 角色和 ordinal 保存，并通过同一 Change 的复合外键约束；它不是 JSON 数组或通用 owner 表。Retry 永远创建新的 attempt；Change 的 Cancel 只表达 Control Plane 后续推进禁令，不能伪造或覆盖已开始执行的实际 outcome。该模型让 M3 的测试 Strategy 与 M4 Worker 都能保留相同的历史和晚到结果语义。

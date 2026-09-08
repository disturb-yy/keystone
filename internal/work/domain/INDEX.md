# `internal/work/domain` 项目索引

## 当前状态

该 package 定义 Project、Change、LifecycleStage、Artifact、AgentRun、HumanDecision、
Canonical Ticket Graph、Ticket、Dependency 和 ProjectInitialized 领域模型，不访问外部系统。

| 文件 | 职责 |
| --- | --- |
| `model.go` | Project 与 Change 生命周期实体、Artifact 身份与 Planning metadata/link、AgentRun kind/source revision/终态、RFC4122 UUIDv7 和状态不变量 |
| `ticket_graph.go` | Ticket Draft candidate、Canonical Graph/Ticket/Dependency 不变量和 StructuralFrontier 派生 |
| `errors.go` | Project/Change、Planning run/candidate 和 Ticket Graph 可映射的业务错误 |
| `model_test.go` | Project、Intent、生命周期转换、Planning ArtifactRef 向后兼容、AgentRun 和 Graph 不变量测试 |

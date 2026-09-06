# `internal/work/domain` 项目索引

## 当前状态

该 package 定义 Project、Change、LifecycleStage、Artifact、AgentRun、HumanDecision
和 ProjectInitialized 领域模型，不访问外部系统。

| 文件 | 职责 |
| --- | --- |
| `model.go` | Project 与 Change 生命周期实体、Artifact 身份、AgentRun 终态、RFC4122 UUIDv7 和状态不变量 |
| `errors.go` | Project/Change 可映射到 Control Plane 的业务错误 |
| `model_test.go` | Project、Intent、生命周期转换和 AgentRun 不变量测试 |

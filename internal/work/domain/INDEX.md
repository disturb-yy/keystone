# `internal/work/domain` 项目索引

## 当前状态

该 package 定义 Ticket 04 的 Project、Repository、Manifest、Intent、Receipt
和 ProjectInitialized 领域模型，不访问外部系统。

| 文件 | 职责 |
| --- | --- |
| `model.go` | Project Bootstrap 的实体、值对象、RFC4122 UUIDv7 和状态不变量 |
| `errors.go` | 可映射到 Control Plane 的业务错误 |
| `model_test.go` | ProjectID、Binding 和事件不变量测试 |

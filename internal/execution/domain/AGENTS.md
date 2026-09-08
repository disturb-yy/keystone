# `execution/domain` 局部规约

该 package 只表达 Ticket 09 执行域的纯业务概念和不变量。

- 不依赖 Git、SQLite、HTTP、Worker Protocol 或 Runtime SDK。
- Workspace 的物理路径只作为内部领域值参与身份校验，不得直接成为 Control Plane DTO。
- 执行会话、DispatchEpoch、Authorization、Snapshot 和 Evidence 均以不可变输入 revision 关联。
- 状态转换必须显式失败，不通过猜测性清理、reset、checkout 或自动重试改变事实。
- 注释使用中文，协议字段和技术标识符保持 English。

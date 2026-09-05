# `internal/work/domain` 局部规约

该 package 只定义 Project Bootstrap 的业务概念、不变量和错误语义。

- 不依赖 HTTP、SQL、文件系统、配置或基础设施实现。
- `ProjectID` 必须是小写 canonical、RFC4122 variant 的 UUIDv7；`RepositoryBinding` 必须是绝对、规范化的非 bare 主工作树根。
- `ProjectInitialized` 只表示当前 LocalStateRoot 内 Project 首次成为权威记录。
- 修改后运行本 package 的聚焦测试及根级 Go 验证。

# localstate 局部规约

`localstate` 只负责 Keystone 本机数据根路径、目录初始化、跨平台单实例锁和诊断运行元数据。

- `Resolve` 必须保持无副作用；只有 `Paths.Initialize` 创建目录。
- 锁文件是实例唯一权威，元数据只能用于诊断。
- 平台锁原语分别放在 build-tag 隔离文件中；不得引入 Domain、SQLite、HTTP 或 Repository Manifest。
- 目录使用 `0700`，状态、锁和元数据文件使用 `0600`。
- 修改后运行本 package 测试、根级 Go 测试、`go vet` 和 Windows 交叉编译。

# `repository` 项目索引

## 当前状态

该 package 将绝对调用路径解析为物理、非 bare Git 主工作树 Binding，并读取 Change 创建所需的稳定源快照。

| 文件 | 职责 |
| --- | --- |
| `git.go` | Git root、拓扑、旧 root 可验证性和连续两次 clean/HEAD Change Snapshot |
| `git_test.go` | 临时真实 Git Repository、dirty/ignored、detached/unborn Snapshot 行为测试 |

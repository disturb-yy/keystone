# `repository` 项目索引

## 当前状态

该 package 将绝对调用路径解析为物理、非 bare Git 主工作树 Binding，读取 Change 创建所需的稳定源版本，并为 Planning materialize 固定 revision 的临时隔离 Snapshot。

| 文件 | 职责 |
| --- | --- |
| `git.go` | Git root、拓扑、旧 root 可验证性和连续两次 clean/HEAD Change Snapshot |
| `git_test.go` | 临时真实 Git Repository、dirty/ignored、detached/unborn Snapshot 行为测试 |
| `snapshot.go` | 固定 commit 的临时 clone、隔离 Snapshot handle、相对路径边界和幂等清理 |
| `snapshot_test.go` | 历史 revision、一致性、源工作树不变、路径隔离及失败清理集成测试 |

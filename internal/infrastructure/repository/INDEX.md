# `repository` 项目索引

## 当前状态

该 package 将绝对调用路径解析为物理、非 bare Git 主工作树 Binding。

| 文件 | 职责 |
| --- | --- |
| `git.go` | Git root、拓扑和旧 root 可验证性识别；非目录旧 root 返回领域冲突 |
| `git_test.go` | 临时真实 Git Repository 行为测试 |

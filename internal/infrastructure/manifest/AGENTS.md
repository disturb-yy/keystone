# `manifest` 局部规约

该 package 负责 `.keystone/project.yaml` 的严格 V1 文件边界。

- 只接受单一 mapping、`version: 1` 和 `project_id` 两个字段。
- 已存在的非法 Manifest 只读拒绝，不能覆盖或改变原字节、权限和 symlink。
- 缺失文件使用 create-if-absent、完整写入、同步和重读核验；可恢复 I/O 错误映射为 `manifest_unavailable`。

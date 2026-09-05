# `manifest` 局部规约

该 package 负责 `.keystone/project.yaml` 的严格 V1 文件边界。

- 只接受单一 mapping、`version: 1` 和 `project_id` 两个字段。
- 已存在的非法 Manifest 只读拒绝，不能覆盖或改变原字节、权限和 symlink。
- 缺失文件写入同目录临时文件，完成写入、同步和关闭后以无覆盖原子方式发布，再重读核验；竞争者只重读已发布文件。临时文件失败路径必须清理，可恢复 I/O 错误映射为 `manifest_unavailable`。

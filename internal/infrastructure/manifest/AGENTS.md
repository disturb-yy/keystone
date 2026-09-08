# `manifest` 局部规约

该 package 负责 `.keystone/project.yaml` 的严格 V1 身份边界和只读解析 V2 策略边界。

- 只接受单一 mapping、`version: 1` 和 `project_id` 两个字段。
- V2 只接受 `version`、`project_id`、`verify.commands` 和可选 `commit.template`；命令参数按 argv 直接执行，不把配置解释为 shell。
- V2 parser 必须拒绝未知/重复字段、alias、custom tag、多 document、非法 UTF-8、空参数和越界值；语义摘要不能受注释或 YAML 空白影响。
- V2 只读解析不改变 Manifest 文件；V1 的创建和读取流程保持原字节、权限和 symlink 约束。
- 已存在的非法 Manifest 只读拒绝，不能覆盖或改变原字节、权限和 symlink。
- 缺失文件写入同目录临时文件，完成写入、同步和关闭后以无覆盖原子方式发布，再重读核验；竞争者只重读已发布文件。临时文件失败路径必须清理，可恢复 I/O 错误映射为 `manifest_unavailable`。

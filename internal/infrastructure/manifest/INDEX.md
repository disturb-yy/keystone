# `manifest` 项目索引

## 当前状态

该 package 实现 ProjectManifest V1 的文件创建、读取、严格解析和拓扑检查。

| 文件 | 职责 |
| --- | --- |
| `manifest.go` | Manifest 文件适配、同目录临时文件原子发布和严格 YAML 子集解析 |
| `manifest_test.go` | 合法、非法、symlink、权限和原字节保持测试 |

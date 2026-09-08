# `manifest` 项目索引

## 当前状态

该 package 实现 ProjectManifest V1 的文件创建、读取、严格解析和拓扑检查，并提供 ProjectManifest V2 的只读严格 parser 与语义 digest。

| 文件 | 职责 |
| --- | --- |
| `manifest.go` | Manifest 文件适配、同目录临时文件原子发布和严格 YAML 子集解析 |
| `v2.go` | V2 version/project/verify/commit 解析、边界校验和策略摘要 |
| `manifest_test.go` | V1 合法/非法、symlink、权限、原字节保持及 V2 digest/严格字段测试 |

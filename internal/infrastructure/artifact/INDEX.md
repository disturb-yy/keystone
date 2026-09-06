# `artifact` 项目索引

## 当前状态

该 package 提供 Change Artifact 内容的原子写入、摘要复用和校验读取。

| 文件 | 职责 |
| --- | --- |
| `store.go` | 以 SHA-256 内容身份管理本机 Artifact 文件 |
| `store_test.go` | 原文、复用、缺失和摘要失配测试 |

该 package 不拥有业务 ArtifactRef、SQLite 事务或 HTTP 响应。

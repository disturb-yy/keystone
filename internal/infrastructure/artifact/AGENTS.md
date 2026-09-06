# `artifact` 局部规约

该 package 是 Change Artifact 的本机内容适配器。

- 只接收摘要可计算的字节，使用同目录临时文件、同步和原子 rename 发布内容。
- 物理路径只在本 package 内使用；Application、HTTP 和 Domain 只看到 Artifact 身份。
- 读取必须重新校验 SHA-256 和字节长度，缺失或失配返回 `ErrArtifactUnavailable`，不自动修复。
- 不访问 SQLite，不创建 ArtifactRef，不推进 Change 生命周期。

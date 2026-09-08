# Governance Domain

该 package 只保存 Verify、Commit、FinalVerify 的业务值对象、结果和校验。

- 不依赖 HTTP、SQL、文件系统、Git 或 Worker Protocol。
- 不推进 Change 生命周期；状态推进由 Workstore/Application authority 完成。
- 验证结果必须区分 PASS、FAIL、HUMAN_REQUIRED 和 PENDING。

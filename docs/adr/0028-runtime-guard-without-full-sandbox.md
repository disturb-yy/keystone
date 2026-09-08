# ADR-0028：Runtime Guard 的有限保证边界

## 状态

已接受。

## 决策

V1 的 `ExecutionGuard` 通过隔离 Worker protocol secret、限制 `RuntimeAdapter` 输入、拒绝常见 Git 禁止命令并检查执行前后 `HEAD` 等可验证边界，阻止 Runtime 获得 Keystone 生命周期和数据库权威。Worker 固定 Runtime 的工作目录为已授权的 Assigned Workspace，并独立采集 stdout、stderr、exit code、diff 和 changed files。

## 理由与边界

该边界保留 Codex CLI、测试命令和 repository-local shell 的可用性，同时明确不能把提示词禁令或 Worker 自报状态误写成安全保证。V1 不承诺对任意本机 shell、绝对路径工具或恶意同用户进程提供完整的操作系统沙箱；需要该保证时必须另行引入并验证操作系统级隔离方案。

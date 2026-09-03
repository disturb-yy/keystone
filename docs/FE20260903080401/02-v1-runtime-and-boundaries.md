# Keystone V1 Runtime / API / Worker / Storage 边界

## 1. Control Plane

Daemon 是唯一权威 Control Plane API Surface。

```text
CLI / Dashboard
      ↓
Control Plane API
      ↓
Daemon
```

Client 只能提交 Command / Query，不能直接修改数据库或设置 Lifecycle 状态。

## 2. Worker

```text
Daemon
  ↓ Worker Protocol
Worker
  ↓
Workspace / Runtime / Git / Test / Shell
```

Worker 只拥有副作用执行权，不拥有 Durable Work Truth。

## 3. Worker Protocol 最小语义

```text
Register
Heartbeat
Execute
Report
```

推荐 Worker Pull 模式，便于未来保持 Remote Worker 扩展方向。

## 4. Runtime Adapter

```text
RuntimeAdapter
├── CodexAdapter
└── OpenCodeAdapter
```

Runtime 只在 Assigned Workspace 内工作。

禁止 Runtime：

```text
git commit
git push
git merge
推进 Lifecycle
标记 Ticket DONE
批准 Verify
修改 Canonical Ticket Graph
直接写 Keystone DB
```

## 5. Workspace

V1：一个 Change 一个 Git Worktree。

```text
Project Repository
      ↓
Change
      ↓
Workspace
      ↓
Ticket AgentRun
```

Change 内 Ticket 串行复用 Workspace。

## 6. Storage

### Repository

保存：

- Source Code
- AGENTS.md / INDEX.md
- Architecture Docs
- Versioned Project Knowledge
- `.keystone/project.yaml`

### SQLite

保存：

- Project / Change / Ticket 当前状态
- Dependency
- Workspace metadata
- AgentRun metadata
- Worker metadata
- Artifact metadata
- Decision
- Domain Event

所有表统一 `t_` 前缀。

### Local Artifact Store

保存：

- Raw Runtime Logs
- Understanding / Design / Plan
- Diff
- Test Output
- Verification Artifact

SQLite 只保存可查询摘要与引用。

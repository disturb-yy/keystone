# 05 — 运行时拓扑

## V1 物理形态

```text
Developer Machine

┌─────────────────────────────────────────────┐
│ Repository                                  │
│   └── .keystone/project manifest            │
│                                             │
│ Keystone CLI                                │
│ Keystone Dashboard                         │
│           │                                 │
│           ↓                                 │
│ Keystone Control Plane Daemon               │
│           │                                 │
│           ↓                                 │
│ Keystone Worker                             │
│           │                                 │
│           ↓                                 │
│ Workspace                                   │
│           │                                 │
│     Codex / OpenCode / Tools                │
└─────────────────────────────────────────────┘
```

## Local-first

V1 采用 Local-first。

Daemon 需要常驻，因为 Keystone 需要：

- Scheduling
- Waiting
- Recovery
- Parallel Work
- Human Gate
- Persistent Lifecycle State

CLI 是 Client，不是 Control Plane 本身。

Dashboard 同样是 Client。

## Dashboard

Dashboard 是一等 Human Interaction Surface。

两类核心用途：

### Observe

- Projects
- Changes
- Lifecycle
- Ticket Graph
- Frontier
- Runs
- Needs Human
- Evidence / Artifact / Trace

### Act

- Approval
- Decision
- Pause / Resume
- Cancel
- Escalation Handling
- Selected Governance Actions

V1 Dashboard 是 Local Web UI。

## 一个 Daemon 管多个 Project

```text
Local Keystone
├── Project A
├── Project B
└── Project C
```

CLI 通过 Repository Manifest 获取 Current Project。

Dashboard 提供 Global Project View。

## Worker

Worker 可替换，并且不拥有权威工作状态。

Worker 临时可以持有：

- process/session handle
- heartbeat
- workspace path
- runtime session id
- streaming logs
- temporary files

Worker 不拥有：

- Change State
- Ticket State
- Gate State
- Authoritative Dependency Graph
- Recovery Decision

## 未来扩展

不改变领域核心即可从：

```text
V1:
1 local daemon
1 local worker
```

演进为：

```text
Future:
1 control plane
N local/remote workers
```

## Monorepo

V1 使用 Monorepo，包含：

- CLI
- Dashboard
- Daemon
- Worker
- Core Modules
- Ports
- Adapters

Monorepo 不代表允许随意 Import。

Dashboard / CLI 使用 API Contract。

Worker 使用独立 Worker Protocol。

## 对外边界

```text
CLI / Dashboard / External Sources
        ↓
Control Plane API
        ↓
Daemon

Daemon
        ↓
Worker Protocol
        ↓
Worker
```

Client 与 Worker 都不能直接修改 Keystone DB。

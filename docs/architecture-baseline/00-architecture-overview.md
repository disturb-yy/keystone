# 00 — 总体架构概览

## 1. 定位

Keystone 是一个**独立实现的 AI Coding Control Plane**。

它不是：

- Codex Wrapper；
- OpenCode Wrapper；
- 通用 Workflow Engine；
- Git 的替代品；
- CI/CD 的替代品；
- LoopX 的扩展或运行时依赖。

它负责围绕持久化的软件变更状态，对 AI Coding 工作进行协调、治理和推进。

## 2. 总体架构

```text
                        HUMAN
                ┌─────────┴─────────┐
                ↓                   ↓
          Keystone CLI        Keystone Dashboard
                └─────────┬─────────┘
                          ↓
                  Control Plane API
                          ↓
┌─────────────────────────────────────────────────────┐
│            Keystone Control Plane Daemon           │
│                                                     │
│  1. Work & Lifecycle                               │
│  2. Intelligence & Planning                        │
│  3. Governance                                     │
│  4. Orchestration & Execution                      │
│  5. Traceability & Learning                        │
│                                                     │
│      持有 Work / Decision / Execution 权威状态      │
└──────────────────────┬──────────────────────────────┘
                       │
                 Worker Protocol
                       ↓
               Keystone Worker
                       │
                Execution Guard
                       │
                Workspace Manager
                       │
                   Workspace
                       │
         ┌─────────────┼─────────────┐
         ↓             ↓             ↓
       Codex        OpenCode        Tools
                                     │
                          Git / Shell / Test / CodeMap
```

## 3. Project 初始化

一个代码仓库通过以下方式进入 Keystone：

```text
cd <repository>
keystone init
```

逻辑上：

```text
Git Repository
     │
 keystone init
     ↓
 Project Manifest
     │
     ↓
 Keystone Project Registry
```

仓库保留版本化项目知识，Keystone 保存 Control Plane 运行状态。

## 4. 一个 Daemon 管理多个 Project

```text
Keystone Local Control Plane
├── Project: stock-lens
├── Project: order-system
└── Project: another-project
```

CLI 通过仓库里的 Project Manifest 判断“当前 Project”。

Dashboard 提供全局多 Project 视角。

## 5. 高层工作模型

```text
Project
   │
   ├── Change A
   │     └── Ticket Graph
   │           └── Ticket
   │                 └── Execution DAG
   │                       └── Agent Run
   │
   └── Change B
         └── Ticket Graph
```

一个 Project 可以同时存在多个 Active Change。

允许并行，但调度必须考虑 Project-level Dependency / Conflict。

## 6. Truth Boundary

```text
Git Repository
├── Source Code
└── Versioned Project Knowledge

Keystone
├── Project / Change / Ticket
├── Gate / Decision / Evidence
├── Execution State
├── Artifact Lineage
├── Domain Events
└── Derived Knowledge

Worker
└── Side-effect Execution

Runtime
└── Runtime-specific Behavior
```

## 7. V1 哲学

V1 是 **Thin Vertical Slice**，不是完整平台。

目标是证明：

```text
Repository
→ Project
→ Change
→ Ticket
→ Workspace
→ Agent
→ Verify
→ Integrate Ready
```

每个主要子系统都进入 V1，但只实现最简单可用版本。

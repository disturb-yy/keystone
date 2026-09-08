# 07 — 可扩展性模型

## 1. Stable Core

核心概念不应因为外部工具变化频繁修改。

稳定概念包括：

```text
Project
Change
Ticket
Lifecycle
Dependency
Artifact
Evidence
Gate
Decision
Agent Run
Event
Workspace
Actor
```

## 2. Explicit Ports

主要可替换实现边界：

```text
Runtime Adapter
Workspace Adapter
Source Control Adapter
Knowledge Provider
Context Provider
Ticket Generator
Policy Provider
Evidence Provider
Storage Adapter
External Tool Adapter
Change Source Adapter
```

## 3. Capability 是统一执行抽象

Lifecycle Stage 不直接绑定具体 Agent / Runtime。

概念上：

```text
Lifecycle Stage
      ↓
Stage Strategy
      ↓
Capability Request
      ↓
Capability Registry
      ↓
Orchestration
      ↓
Role + Runtime + Context + Tools
      ↓
Worker
```

Capability 回答：

> 要完成什么能力？

Runtime 回答：

> 哪个执行运行时来完成？

Role 回答：

> 以什么职责 / 视角来完成？

Tools 回答：

> 使用哪些工具完成？

## 4. Skill 与 Capability

Skill 是 Capability 的一种主要实现方式。

但：

```text
Capability ≠ Markdown File
Capability ≠ Runtime
Capability ≠ Agent
```

V1 Registry 可以非常简单。

## 5. Ticket Generator

`to-tickets` 是设计参考 / 默认实现方向。

核心不能写成：

```text
Ticket == 某个 Skill 的输出格式
```

更合理：

```text
Ticket Generator Port
        ↓
to-tickets-inspired implementation
```

## 6. Knowledge Provider

Keystone Core 不永久绑定 CodeMap。

```text
Knowledge Provider
├── Repository Docs
├── INDEX
├── Git
├── CodeMap
└── Future Source
```

## 7. Runtime Adapter

```text
Runtime Adapter
├── Codex
├── OpenCode
└── Future Runtime
```

Core 中不应该出现 Runtime-specific branching。

## 8. 不提前做插件平台

V1 的可扩展性意味着：

```text
interface / port
+
one default implementation
```

而不是：

```text
dynamic marketplace
remote registry
plugin ecosystem
```

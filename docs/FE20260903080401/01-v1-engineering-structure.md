# Keystone V1 工程结构与 DDD 约束

## 1. 推荐 Monorepo

```text
keystone/
├── AGENTS.md
├── INDEX.md
├── README.md
├── cmd/
│   ├── keystone/
│   ├── keystone-daemon/
│   └── keystone-worker/
├── internal/
│   ├── work/
│   ├── planning/
│   ├── governance/
│   ├── execution/
│   ├── traceability/
│   └── infrastructure/
├── contracts/
│   ├── controlplane/
│   └── worker/
├── dashboard/
├── migrations/
├── configs/
├── docs/
└── scripts/
```

## 2. Domain-first Modular Monolith

每个核心模块内部按需要自包含：

```text
internal/work/
├── AGENTS.md
├── INDEX.md
├── domain/
├── application/
├── ports/
└── adapters/
```

不采用全局大目录：

```text
internal/domain/
internal/application/
internal/adapters/
```

避免一个 Change 修改时跨多个巨大技术层目录跳转。

## 3. `infrastructure` 边界

`internal/infrastructure/` 只放真正跨领域的通用底层能力，例如：

```text
database
filesystem
process
logging
clock
ids
```

属于特定领域模块的 Adapter 应优先留在该模块，例如：

```text
internal/execution/adapters/codex
internal/execution/adapters/opencode
internal/execution/adapters/gitworktree
```

禁止演变为：

```text
common/
utils/
helpers/
```

## 4. AGENTS.md 与 INDEX.md

所有 Go package 目录必须同时存在：

```text
AGENTS.md
INDEX.md
```

### AGENTS.md

回答：

> 这个 package 应该怎样修改？

内容建议：

- 职责
- 允许依赖
- 禁止依赖
- Ownership
- 领域约束
- 代码风格
- 测试要求
- AI 修改时的 Hard Rules

### INDEX.md

回答：

> 这个 package 有什么，应该从哪里开始读？

内容建议：

- 包职责摘要
- 关键文件
- 关键类型
- 主要入口
- 上游 / 下游依赖
- 常见修改导航

## 5. 根目录文档职责

```text
README.md → 给人看：项目介绍、构建、运行
AGENTS.md → 给 Agent 看：Repository 最高级工程约束
INDEX.md  → 给人和 Agent 看：Repository 导航地图
```

下层可以增加约束，但不能违反上层 Architecture Boundary。

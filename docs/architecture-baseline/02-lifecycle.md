# 02 — 生命周期主干

## 稳定 Lifecycle

```text
Intent
  ↓
Understand
  ↓
Design
  ↓
Plan
  ↓
Ticketize
  ↓
Execute
  ↓
Verify
  ↓
Integrate
  ↓
Learn
```

## 各阶段高层含义

### Intent

描述用户到底想改变什么。

它是 Change 的入口。

### Understand

建立足够的项目理解，包括：

- Project
- Relevant Code
- Architecture
- Domain Concepts
- Constraints
- Blast Radius
- Dependencies

可能使用 Repository Docs、INDEX、CodeMap、Search、Git History、Conceptual Space 等。

### Design

确定目标行为、架构和边界应该变成什么样。

### Plan

把 Design 转换为可执行实施方案。

### Ticketize

把 Plan 拆成可独立执行、可验证的实现切片。

参考 `to-tickets` 的方向：

- 优先 tracer-bullet vertical slice；
- 明确 blocking relationships；
- Ticket 应尽量适合 fresh context；
- 必要时允许 prefactor / expand-migrate-contract。

具体 Ticket Schema 和 Ticket Generator 流程留待后续专题。

### Execute

通过 Workspace + Worker + Runtime 真正执行 Ticket。

### Verify

收集实现是否满足要求的证明。

Review 作为一种 Verification Capability，不一定单独作为一级 Stage。

### Integrate

判断 Change 是否可以进入目标代码线 / 交付路径。

实际 Git 操作通过 Adapter 执行。

### Learn

把结果、失败、Human Correction、Eval、Incident 转化为后续改进输入。

## 当前产品边界

V1 主要止于：

```text
Integrate / Change Ready
```

Deploy / Operate / Maintain 暂时作为外部系统集成点。

生产反馈可以通过 Learn 重新生成新的 Change。

## 轻量 Stage

Stage 存在，不代表每次都要走重量级流程。

小 Bug 可以：

```text
Intent
→ lightweight Understand
→ trivial Design
→ short Plan
→ one Ticket
→ Execute
→ Verify
→ Integrate
```

Lifecycle 仍然稳定。

## 生命周期推进

由一个很薄的 Lifecycle Coordinator 统一推进：

```text
Lifecycle Coordinator
        ↓
transition request
        ↓
Governance / Gate
        ↓
PASS → advance
FAIL → wait/block/escalate
```

Lifecycle Coordinator 自己不负责真正 AI Coding 工作。

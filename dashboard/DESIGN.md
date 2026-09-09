# Keystone Dashboard Design System

## Product and Audience

Keystone Dashboard 是本机单操作者使用的 Control Plane Client，用于观察 Project、Change、Ticket、运行证据和人工恢复事项。它面向需要确认系统当前权威事实、检查证据链并提交有限恢复决定的工程人员。

产品气质是深色、克制、可追溯的开发者控制台：信息密度高但不拥挤，状态优先于装饰，所有重要状态都同时使用文字和语义颜色表达。

## Evidence Inventory

### 已确认输入

- [Ticket 11 — Dashboard Observation](../docs/FE20260903080401/tickets/11-dashboard-observation.md)：固定四个页面、Daemon 托管、Query 优先、SSE 只作刷新提示以及 Command/Decision 边界。
- 已确认的视觉基线：同源生产托管、显式 URL 路由、深色开发者控制台、桌面优先的响应式布局、TDesign React 组件、无 optimistic update 和有界 Trace/Artifact 展示。
- [当前 Dashboard 入口](./src/main.tsx) 与 [当前样式](./src/styles.css)：五个页面已共用本文件的深色 token、导航和响应式 shell；Create Change 的具体写入边界由实施契约约束。
- [Dashboard package](./package.json)：当前使用 React、TypeScript、Vite、TDesign React、TDesign icons 和 React Router，依赖版本已写入锁文件。
- `tdesign-mcp-server` React 组件清单、组件文档和 DOM 资料：确认使用 `Layout`、`Menu`、`Breadcrumb`、`Card`、`Table`、`Tree`、`Timeline`、`Tag`、`Alert`、`Dialog`、`Drawer`、`Skeleton`、`Empty`、`Result`、`Button`、`Popconfirm`、`Statistic`、`Space`、`Typography` 等组件。

### Fallback 输入

- 当前仓库没有品牌规范、设计稿或既有 MASTER/page override。
- UI UX Pro Max 搜索脚本未找到；以下颜色、尺寸和动效数值是结合已确认的深色开发者控制台方向、TDesign 语义和 Keystone 控制台语境作出的 fallback 选择。

## Design Principles

1. **Authority first**：Dashboard 只展示 Daemon Query 返回的事实，不在浏览器推导 Lifecycle、Ticket 状态、Frontier、Health 或人工阻塞。
2. **Status is readable**：状态永远同时显示稳定文字、组件主题和必要的辅助说明，不能只依赖颜色。
3. **Evidence has lineage**：Event、AgentRun、ArtifactRef、Verification 和 Decision 保留各自身份与引用关系，不把原始日志堆成无法复盘的长文本。
4. **Safe action**：写操作只来自 Daemon 返回的 `available_actions`，使用当前 `ChangeVersion`，成功后重新 Query，不做 optimistic update。
5. **Calm density**：使用卡片、表格和时间轴组织高密度信息；减少大面积渐变、装饰插画和无意义动画。
6. **Local clarity**：界面表达本机 Daemon、Worker 和 Project 的边界，不暗示远程、多租户、RBAC 或完整运维控制台能力。

## Page Shell and Navigation

### Global shell

- 使用 TDesign `Layout` 组合 `Header`、`Aside` 和 `Content`。
- 桌面端侧边栏宽度为 `232px`，折叠后为 `64px`；顶部栏高度为 `64px`。
- 侧边导航使用 TDesign `Menu`，固定入口为 `Projects`、`Create Change` 和 `Needs Human`；详情页通过 `Breadcrumb` 返回所属 Project 或 Change。
- 顶部栏显示 `Keystone`、当前 Daemon readiness、Worker health 摘要和数据新鲜度；不显示 `database_path`、WorkspacePath、Lease 或 secret。
- 主内容区域默认内边距 `24px`，最大内容宽度 `1440px`，避免超宽屏上信息行过长。
- `/` 默认进入 Projects；创建页路由为 `/changes/new`，详情路由固定为 `/projects/:project_id`、`/changes/:change_id`、`/needs-human`。
- Daemon 对前端未知路径回退 `index.html`，`/v1` 和 SSE 路径不参与 SPA 回退。

### Page composition

- 页面标题区：页面标题、简短上下文、刷新状态和必要操作。
- 状态区：使用 `Card`、`Statistic`、`Tag` 或 `Alert` 展示摘要，不创建客户端计算的业务指标。
- 内容区：使用 `Table`、`Tree`、`Timeline`、`Tabs` 和 `Drawer` 承载详细观察。
- 状态反馈区：每个页面和每个可独立加载的区域都必须有 loading、empty、error 和 stale 表达。

## Semantic Color Tokens

颜色值集中在 CSS 变量中，组件优先使用 TDesign `theme`；页面不得散落硬编码状态颜色。

### Foundation

| Token | Value | 用途 |
| --- | --- | --- |
| `--ks-color-canvas` | `#0B1020` | 页面背景；深色控制台的稳定底色 |
| `--ks-color-surface` | `#111A2E` | 卡片、表格和对话框表面 |
| `--ks-color-surface-subtle` | `#17233D` | 表头、次级区域和 hover 背景 |
| `--ks-color-text-primary` | `#F2F6FC` | 主标题、关键字段和 ID 标签 |
| `--ks-color-text-secondary` | `#AAB9D0` | 描述、辅助信息和次级字段 |
| `--ks-color-text-muted` | `#73829A` | 时间、弱提示和非关键元数据 |
| `--ks-color-border` | `#2A3854` | 卡片、输入框和表格边界 |
| `--ks-color-divider` | `#1F2C45` | 内容分割线 |

### Brand

| Token | Value | 用途 |
| --- | --- | --- |
| `--ks-color-brand-700` | `#245FCB` | active、focus 和深色品牌文本 |
| `--ks-color-brand-600` | `#3B82F6` | 主按钮、链接和运行中状态 |
| `--ks-color-brand-500` | `#6EA8FE` | hover 和交互强调 |
| `--ks-color-brand-100` | `#152C53` | 信息提示、选中背景和深色品牌区域 |

### Semantic status

| 领域语义 | TDesign 主题 | Foreground | Background | 使用规则 |
| --- | --- | --- | --- | --- |
| `active` / `running` / `info` | `primary` | `#80B5FF` | `#122C50` | 表示正在协调或需要关注的正常进展 |
| `pending` / `paused` / `cancelled` | `default` | `#AAB9D0` | `#17233D` | 表示等待、暂停或结束，不表示错误 |
| `human_required` / `HUMAN_REQUIRED` | `warning` | `#F4B75E` | `#3A2610` | 明确需要人工查看或决定 |
| `failed` / `FAIL` / `error` | `danger` | `#FF9AA2` | `#3B1C24` | 表示已确认失败或请求错误 |
| `integrate_ready` / `PASS` / `success` | `success` | `#7DDAA7` | `#123523` | 表示已形成对应的权威成功事实 |
| `unavailable` / `degraded` | `warning` | `#F4B75E` | `#3A2610` | 表示暂时不可用；不得自动升级为 human required |

状态颜色不能改变领域语义。`human_required` 不等于 `failed`，`unavailable` 不等于 `human_required`，`integrate_ready` 也不表示已经 merge、push 或 deploy。

## Typography

### Font families

- Latin/UI：`Inter`, `ui-sans-serif`, `system-ui`, `-apple-system`, `BlinkMacSystemFont`, `"Segoe UI"`, sans-serif。
- Chinese fallback：`"PingFang SC"`, `"Microsoft YaHei"`, `"Noto Sans CJK SC"`。
- Revision、UUID、Artifact ID 和其他技术标识：`ui-monospace`, `SFMono-Regular`, `Consolas`, monospace。

### Type scale

| Token | Size / line height | 用途 |
| --- | --- | --- |
| `--ks-type-display` | `28px / 36px` | 页面主标题，仅每页一个 |
| `--ks-type-title` | `20px / 28px` | 区块标题和重要卡片标题 |
| `--ks-type-body` | `14px / 22px` | 默认正文、表格和操作文案 |
| `--ks-type-body-large` | `16px / 24px` | 摘要值和重要说明 |
| `--ks-type-caption` | `12px / 18px` | 时间、辅助说明、字段标签 |
| `--ks-type-code` | `12px / 20px` | UUID、revision、摘要代码 |

标题使用 `Typography.Title`，正文使用 `Typography.Text` / `Paragraph`。长 ID 默认省略显示，并通过 tooltip 或复制操作访问完整值；不把 ID 当作页面标题。

## Spacing and Sizing

以 `4px` 为基础单位，只使用以下间距：

| Token | Value |
| --- | --- |
| `--ks-space-1` | `4px` |
| `--ks-space-2` | `8px` |
| `--ks-space-3` | `12px` |
| `--ks-space-4` | `16px` |
| `--ks-space-5` | `24px` |
| `--ks-space-6` | `32px` |
| `--ks-space-7` | `40px` |
| `--ks-space-8` | `48px` |

- 页面区块之间使用 `24px`，卡片内边距默认 `20px`，紧凑表格单元格使用 `12px` 横向间距。
- 操作按钮之间至少保留 `8px`；危险操作不得紧贴主操作。
- 交互目标最小尺寸为 `40px`，图标按钮必须有 tooltip 和可访问名称。
- 表格列优先保证状态、标题和操作可读；技术 ID 可以使用等宽字体和省略。

## Radius, Border, Elevation

| Token | Value | 用途 |
| --- | --- | --- |
| `--ks-radius-sm` | `4px` | 输入框、标签和小控件 |
| `--ks-radius-md` | `8px` | 卡片、表格容器、按钮 |
| `--ks-radius-lg` | `12px` | 页面级突出区域或对话框 |
| `--ks-border-width` | `1px` | 所有普通边界 |
| `--ks-shadow-card` | `0 4px 16px rgb(31 48 75 / 6%)` | 需要层级时的卡片阴影 |
| `--ks-shadow-dialog` | `0 16px 48px rgb(31 48 75 / 16%)` | Dialog / Drawer 浮层 |

默认卡片使用边框而不是阴影；阴影只表达浮层或暂时脱离页面流的层级。

## Component Rules

### TDesign first

- 生产代码优先使用 `tdesign-react`，图标优先使用 `tdesign-icons-react`。
- 新组件实现前必须通过 `tdesign-mcp-server` 查询组件清单和文档；需要调整结构时查询 DOM；需要图标时使用 `search_icon` 确认英文图标名。
- 不复制 TDesign 组件源码，不用大量自定义 CSS 重建 `Table`、`Dialog`、`Tag`、`Timeline` 或 `Menu`。
- 自定义组件只负责 Keystone 领域组合，例如 `StatusTag`、`ObservationSection`、`TraceTimeline` 和 `CommandDialog`，不改变 TDesign 的交互语义。

### Component mapping

| 需求 | 首选组件 | 约束 |
| --- | --- | --- |
| 页面框架 | `Layout`、`Menu`、`Breadcrumb` | 导航只改变 URL，不触发隐式 Command |
| Project / Change 列表 | `Card`、`Table`、`Tag` | 按 API 顺序显示，不在客户端重排权威结果 |
| Lifecycle | `Steps`、`Tag` | Stage 与 Status 分开展示 |
| Ticket Graph | `Table`、可选 `Tree` | `BLOCKED_BY` 明确显示 dependent 与 blocker；不客户端推导执行状态 |
| Trace | `Timeline`、`Tabs`、`Drawer` | Event 按 `EventSequence`，Run/Decision/Artifact 保留身份 |
| Health | `Card`、`Statistic`、`Tag`、`Alert` | 只显示 Daemon 返回的摘要 |
| Command / Decision | `Button`、`Dialog`、`Popconfirm` | 以 Daemon 返回的 `available_actions` 为准 |
| Loading / empty / error | `Skeleton`、`Empty`、`Alert`、`Result`、`Loading` | 每种状态有明确中文文案和重新查询入口 |

### Create Change

- 使用独立的 `/changes/new` 页面；成功创建后跳转到 Daemon 返回的 Change 详情页。
- Project 必须由已注册 Project 的下拉框明确选择，选项同时显示 repository root 与 Project ID；不提供任意路径输入。
- 没有已注册 Project 时禁用提交，并显示可复制的 `keystone init` 引导；页面不提供 Project 初始化写操作。
- Intent 使用必填多行输入框，拒绝仅由空白组成的内容，并将非空内容原样提交为 `intent`。
- 幂等键默认由界面生成；高级区允许用户查看和手动覆盖。未修改表单的重试复用自动键，修改 Project 或 Intent 后生成新自动键；手动键由用户负责保持。
- Project 选择、Intent 和自动幂等键保存至当前浏览器会话，关闭浏览器后清除。
- 提交期间禁用表单和重复提交；失败时保留全部输入，提供中文恢复说明，并在高级详情显示服务端错误码。

## Forms and Feedback

- Pause、Resume、Cancel、Retry 统一使用 `Dialog` 或 `Popconfirm`；Cancel 使用 `danger`，Retry 使用 `primary`。
- `expected_version` 来自当前 Query 快照，不让用户编辑；幂等键由一次用户动作生成并在网络重试中复用，不显示为普通业务表单字段。
- Human Decision 的 reason 在对话框中提供；提交后只接受 Daemon 的权威响应。
- Command 进行中按钮显示 loading 并禁用重复点击；成功使用 `Message` / `Notification` 提示后重新 Query。
- `409` 显示“状态已变化，请重新确认”，保留页面快照，不自动重发。
- Query 失败时保留最后成功快照，并以顶部 `Alert` 标记 `stale`；不能将旧快照伪装成实时状态。

## Data Visualization

Ticket 11 不引入业务图表。可视化只服务于观察：

- Lifecycle 使用 TDesign `Steps`，当前 Stage 由 API 提供。
- Ticket 依赖使用带 `BLOCKED_BY` 列的 `Table`；只有 API 提供适合层级展示的数据时才使用 `Tree`。
- Trace 使用垂直 `Timeline`，时间和 `EventSequence` 同时显示。
- Health 和数量摘要使用 `Statistic`，不把数量推导为业务 verdict。
- 禁止用环图、进度百分比或客户端计算的“完成率”替代 Daemon 的权威状态。

## Motion

- 使用 TDesign 默认过渡，普通交互约 `120–180ms`；页面加载不使用持续跳动或无限旋转以外的装饰动画。
- SSE 刷新只更新查询状态，不对状态卡片做夸张变色或位置跳动。
- Dialog、Drawer、Toast 使用组件默认进入/退出动画。
- 对 `prefers-reduced-motion: reduce` 禁用非必要过渡，保留明确的焦点和状态变化。

## Responsive Behavior

- `>=1200px`：显示完整侧边栏、多列摘要卡片和完整表格。
- `768px–1199px`：侧边栏可折叠，摘要卡片换行，详情区域减少并列列数。
- `<768px`：侧边栏折叠为可打开导航，所有卡片单列，Trace 和 Health 纵向排列。
- 表格不强制压缩技术字段；窄屏允许水平滚动，关键状态和操作列保持可见。
- `Tree` 和 `Timeline` 在窄屏保留语义顺序，不改为仅颜色或图标表达。
- 不建设独立移动端信息架构，不改变五个固定页面和 URL 路由。

## Accessibility

- 使用 `header`、`nav`、`main`、`aside`、`section` 等语义 landmark；每页有唯一主标题。
- 正文和状态文字满足 WCAG AA 对比度目标；状态不只使用颜色，必须有文本或图标辅助。
- 所有按钮、链接、菜单项、Dialog、Drawer 和可展开行支持键盘操作，并有可访问名称。
- Dialog 打开时焦点进入对话框，关闭后返回触发元素；危险操作必须能被屏幕阅读器理解。
- 表格标题、状态列和依赖关系保持可读顺序；长 ID 使用可访问的完整文本或 tooltip。
- 错误、断线、stale 和 Human Required 状态使用 `role="status"` 或 `role="alert"` 的合适语义，不频繁打断用户。
- 颜色 token 需要在深色画布上验证；不引入仅靠红绿区分的操作路径。

## Loading, Empty, Error, Permission States

| 状态 | 视觉表达 | 行为边界 |
| --- | --- | --- |
| 初次加载 | `Skeleton` 或区域 `Loading` | 不显示推测数据 |
| 空 Project / Change / Trace | `Empty` | 明确说明“尚无数据”，不伪造成功状态 |
| Query 错误 | `Result` 或 `Alert` + 重新查询按钮 | 保留可用的最后快照并标记 stale |
| SSE 断线 | 顶部 warning `Alert` | 不改变业务状态；重连后重新 Query |
| 暂时 unavailable | warning `Tag` / `Alert` | 只显示暂时不可用，不转为 Human Required |
| Human Required | warning `Card` / `Tag` + 证据入口 | 只展示 Daemon 允许的 Retry/Cancel 决定 |
| 权限/动作不允许 | disabled `Button` + 原因说明 | 依据 `available_actions`，不在客户端重复规则 |
| Command 提交中 | `Button loading` | 禁止重复提交，复用同一幂等键 |
| Command 成功 | `Message` / `Notification` + 重新 Query | 不使用 optimistic state |
| Command 冲突 | error `Alert` | 提示版本或状态变化，等待用户重新确认 |

## Anti-patterns

- 在浏览器直接访问 SQLite、Artifact store 或 Worker Protocol。
- 根据最近一次 AgentRun、心跳时间或 SSE 事件自行推导 Lifecycle Truth。
- 把 `unavailable`、pending、旧快照或 Worker 缺失显示成 `human_required`。
- 把 `BLOCKED_BY` 反向解释成自由排序，或在浏览器修改依赖图。
- 暴露绝对 WorkspacePath、Lease token、Prompt、运行环境、凭据、完整无界日志或内部数据库路径。
- 为 Pause、Resume、Cancel、Retry 做 optimistic update，或在 409 后自动重发新的幂等请求。
- 用自定义 CSS 复制 TDesign 组件、用 emoji 替代可检索的 TDesign icon，或引入没有明确价值的 UI 依赖。
- 通过大面积渐变、动画、装饰图表或颜色墙制造“实时感”。
- 把 `integrate_ready` 文案写成“已合并”“已部署”或任何超出当前权威事实的表述。

## Delivery Checklist

- [ ] 生产代码中的 TDesign 组件/API 已通过 `tdesign-mcp-server` 的 React 文档核对。
- [ ] 图标名称已通过 `search_icon` 核对，未使用临时 emoji 或未知 icon 名称。
- [ ] `tdesign-react` 及必要的 `tdesign-icons-react` 依赖已写入 `package.json` 并锁定。
- [ ] 五个 URL 页面共用同一 shell、语义 token 和状态映射。
- [ ] Query、SSE、Command 和 Decision 遵守 Ticket 11 的 Daemon authority 边界。
- [ ] 初次 loading、empty、error、stale、断线、unavailable、Human Required 和冲突状态均可见。
- [ ] 键盘操作、焦点、对比度、缩放和窄屏布局完成浏览器验证。
- [ ] `npm ci`、`npm run lint`、`npm run build` 和根级 `make dashboard-build` 通过。
- [ ] 生产构建由 Daemon 同源托管，并验证 SPA fallback 与 `/v1` 不冲突。

## Decision Log

| 日期 | 决策 | 原因 | 状态 |
| --- | --- | --- | --- |
| 2026-09-08 | 使用 TDesign React 作为 Dashboard 组件基础，并通过 `tdesign-mcp-server` 查询组件资料 | 保持组件行为、状态反馈和无障碍规则一致 | 当前设计基线 |
| 2026-09-08 | 采用浅色蓝灰控制台视觉，品牌蓝作为唯一主强调色 | 延续现有骨架的蓝灰基调，并让生命周期状态与品牌交互区分 | 已由 2026-09-10 决策替代 |
| 2026-09-08 | 使用语义 status token，而不是页面级颜色 | 防止 `human_required`、`failed`、`unavailable` 等领域语义混淆 | 当前设计基线 |
| 2026-09-08 | 采用 `Layout` + `Menu` + `Breadcrumb` 的桌面优先响应式 shell | 适配四个固定页面和本机工程控制台的高密度观察场景 | 当前设计基线 |
| 2026-09-08 | 失败和断线保留最后快照并显式标记 stale | 让观察者区分旧事实、查询失败和权威状态变化 | 当前设计基线 |
| 2026-09-10 | 采用深色开发者控制台视觉，并覆盖现有页面和 Create Change 页面 | 用户确认当前视觉不符合要求；统一深色 token、导航、表单与状态规则，避免新旧页面割裂 | 当前设计基线 |

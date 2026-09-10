# Dashboard 间距与响应式布局独立审阅

## 固定审阅目标

- 基线提交：`55adf58503b4a8f37bce6ffb5be8ef7b7b376266`
- 被审提交：`48f4916d87fd400f998ee2ddb48d37db4fe770ba`
- 二进制 Git diff SHA-256：`365caca63f7a8377d3ec501e0ca438a879575b6584c1f555afd047ef7705ea37`
- UI contract：`dashboard-spacing-fidelity-2026-09-10-r1`
- 审阅者：`/root/dashboard_spacing_ui_reviewer`（独立于实施者）

## 审阅范围

审阅已提交的 Create Change 留白、桌面双栏说明区、窄屏单栏降级、TDesign 侧栏尺寸覆盖，以及相应浏览器回归。设计基线为 `dashboard/DESIGN.md`、`prototypes/dashboard.html` 与 `wireframes/dashboard.md`；未审阅后端、路由语义或依赖变更。

## 运行态测试矩阵与证据

| 视口 | 实际渲染证据 | 结论 |
| --- | --- | --- |
| 1440 × 900 | `.form-shell` 为 `822px 298px` 两列、间距 `16px`；说明卡位于表单右侧。表单/说明卡 body 内边距分别为 `25px`/`20px`，选择控件为 42px、提交按钮为 40px。`scrollWidth = innerWidth = 1440`。 | 通过 |
| 1024 × 900 | `.form-shell` 为单列 `786px`；说明卡从表单底部之后开始，侧栏为 190px。`scrollWidth = innerWidth = 1024`。 | 通过 |
| 375 × 900 | 侧栏为 48px，内容区宽 327px，表单壳宽 303px，说明卡在表单下方；各元素右边界不超过 375px。`scrollWidth = innerWidth = 375`。 | 通过 |

补充运行验证：

- 以生产构建预览运行 Playwright 实测，并保留三个视口截图；浏览器 console error 为 0、request failure 为 0。
- 在 375px 下，选择控件、Intent 文本域和填写必要字段后的“创建 Change”按钮均可获得焦点；后者高度为 40px、已启用，并实际发出一次模拟的 `POST /v1/changes` 请求。
- `TMPDIR=/tmp/keystone-playwright-review.KIvOu5 npm run test:e2e`：10/10 通过，包含桌面双栏、1024 单栏、375/768/1024/1440 无横向溢出与焦点检查。
- `npm run lint`：通过。
- `npm run build`：通过。Vite 报告既有的单 chunk 大小提示，不是本次布局改动导致的审阅问题。
- `git diff --check 55adf58503b4a8f37bce6ffb5be8ef7b7b376266..48f4916d87fd400f998ee2ddb48d37db4fe770ba`：通过。

## 发现的问题

### Blocker

无。

### High

无。

### Medium

无。

### Low

无。

## 通过项

- 已恢复桌面端的双栏层级：主表单与右侧提交前说明清晰分离，没有原先的窄间距堆叠。
- 1024px 及以下以单列有序折叠；375px 侧栏的 TDesign inline 尺寸已被有效覆盖，未再挤压内容区或造成页面横向滚动。
- 页面节奏与设计基线一致：内容区、卡片、表单字段、告警和交互目标均恢复为可读且可操作的留白尺度。
- 触控/键盘关键目标满足至少 40px 的高度；表单原有标签、帮助文本、错误关联和创建流程在浏览器回归中仍可用。

## 结论

**PASS**。该固定提交满足本次间距、高保真结构与响应式无堆叠的审阅目标；没有阻塞合入的问题。

import { expect, test } from 'playwright/test'
import type { Page } from 'playwright/test'

const changeObservation = {
  schema_version: 'v1',
  observed_at: '2026-09-10T00:00:00Z',
  project: { project_id: 'project-001', created_at: '2026-09-10T00:00:00Z' },
  change: { change_id: 'change-001', project_id: 'project-001', stage: 'Plan', status: 'active', version: 3, base_revision: 'revision', intent_artifact: {}, created_at: '2026-09-10T00:00:00Z', updated_at: '2026-09-10T00:00:00Z' },
  lifecycle: { availability: 'available', stage: 'Plan', status: 'active', version: 3 },
  ticket_graph: { availability: 'not_yet_available', reason_code: 'not_ready', reason_summary: 'Ticket Graph 尚未形成。' },
  execution: { availability: 'not_yet_available', reason_code: 'not_ready', reason_summary: 'Execution 尚未开始。' },
  trace: { availability: 'available', events: [], runs: [], decisions: [] },
  artifacts: { availability: 'available', artifacts: [] },
  health: { availability: 'available', daemon_ready: true, workers: [] },
  available_actions: [],
}

async function mockControlPlane(page: Page, projects: unknown[], createStatus = 201): Promise<{ requestBody?: unknown; idempotencyKey?: string }> {
  const result: { requestBody?: unknown; idempotencyKey?: string } = {}
  await page.route('**/v1/**', async (route) => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    if (pathname === '/v1/updates') {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'event: refresh\ndata: {}\n\n' })
      return
    }
    if (pathname === '/v1/daemon/status') {
      await route.fulfill({ json: { daemon_readiness: true } })
      return
    }
    if (pathname === '/v1/projects') {
      await route.fulfill({ json: { projects, has_more: false } })
      return
    }
    if (pathname === '/v1/changes' && request.method() === 'POST') {
      result.requestBody = request.postDataJSON()
      result.idempotencyKey = request.headers()['idempotency-key']
      if (createStatus === 201) {
        await route.fulfill({ json: { change: { change_id: 'change-created', project_id: 'project-001', repository_root: '/work/demo', stage: 'Intent', status: 'active', version: 1, base_revision: 'revision', intent_artifact: {}, created_at: '2026-09-10T00:00:00Z', updated_at: '2026-09-10T00:00:00Z' } } })
        return
      }
      await route.fulfill({ status: createStatus, json: { code: 'idempotency_conflict', message: 'idempotency key conflicts with change request' } })
      return
    }
    if (pathname === '/v1/changes/change-001/observation') {
      await route.fulfill({ json: changeObservation })
      return
    }
    await route.fulfill({ status: 404, json: { code: 'not_found', message: 'not found' } })
  })
  return result
}

async function openCreateChange(page: Page): Promise<void> {
  await page.goto('/changes/new', { waitUntil: 'commit' })
}

async function expectDaemonReady(page: Page): Promise<void> {
  await expect(page.getByRole('status').filter({ hasText: 'Daemon 已就绪' })).toContainText('Daemon 已就绪')
}

test('从已注册 Project 创建 Change，并保留原始 Intent 和幂等键', async ({ page }) => {
  const mock = await mockControlPlane(page, [{ project_id: 'project-001', repository_root: '/work/demo', created_at: '2026-09-10T00:00:00Z' }])
  await openCreateChange(page)
  await expectDaemonReady(page)
  await page.selectOption('#target-project', 'project-001')
  await page.locator('#change-intent').fill('实现 Create Change 表单\n保留原始输入。')
  await page.getByRole('button', { name: '创建 Change' }).click()
  await expect(page).toHaveURL(/\/changes\/change-created$/)
  await expect.poll(() => mock.requestBody).toEqual({ repository_path: '/work/demo', intent: '实现 Create Change 表单\n保留原始输入。' })
  expect(mock.idempotencyKey).toBeTruthy()
})

test('Change Detail 以权威侧栏展示生命周期、操作和健康状态', async ({ page }) => {
  await mockControlPlane(page, [])
  await page.goto('/changes/change-001', { waitUntil: 'commit' })
  await expect(page.getByRole('complementary', { name: 'Change 权威状态' })).toBeVisible()
  await expect(page.getByText('权威状态', { exact: true })).toBeVisible()
  await expect(page.getByText('Daemon / Worker Health', { exact: true })).toBeVisible()
  await expect(page.getByText('Plan', { exact: true }).first()).toBeVisible()
})

test('没有已注册 Project 时显示 CLI 引导并禁用创建', async ({ page }) => {
  await mockControlPlane(page, [])
  await openCreateChange(page)
  await expect(page.getByText('keystone init', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '刷新 Project' })).toBeVisible()
  await expect(page.getByRole('button', { name: '创建 Change' })).toBeDisabled()
})

test('创建冲突时保留输入并提供恢复说明', async ({ page }) => {
  await mockControlPlane(page, [{ project_id: 'project-001', repository_root: '/work/demo', created_at: '2026-09-10T00:00:00Z' }], 409)
  await openCreateChange(page)
  await page.selectOption('#target-project', 'project-001')
  await page.locator('#change-intent').fill('发生冲突后仍需保留的 Intent')
  await page.getByRole('button', { name: '创建 Change' }).click()
  await expect(page.getByText('此幂等键已对应另一条创建请求。请核对高级设置，或生成新的自动幂等键。')).toBeVisible()
  await expect(page.locator('#change-intent')).toHaveValue('发生冲突后仍需保留的 Intent')
})

test('在当前浏览器会话中保留未提交草稿', async ({ page }) => {
  await mockControlPlane(page, [{ project_id: 'project-001', repository_root: '/work/demo', created_at: '2026-09-10T00:00:00Z' }])
  await openCreateChange(page)
  await page.selectOption('#target-project', 'project-001')
  await page.locator('#change-intent').fill('会话草稿 Intent')
  await page.waitForFunction(() => sessionStorage.getItem('keystone.create-change.draft')?.includes('会话草稿 Intent'))
  await page.reload({ waitUntil: 'commit' })
  await expect(page.locator('#target-project')).toHaveValue('project-001')
  await expect(page.locator('#change-intent')).toHaveValue('会话草稿 Intent')
})

test('Create Change 在桌面端恢复双栏留白，并在窄屏折叠为单栏', async ({ page }) => {
  await mockControlPlane(page, [{ project_id: 'project-001', repository_root: '/work/demo', created_at: '2026-09-10T00:00:00Z' }])
  await page.setViewportSize({ width: 1440, height: 900 })
  await openCreateChange(page)
  await expectDaemonReady(page)
  await expect(page.locator('.form-shell')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Before you submit' })).toBeVisible()

  const desktopLayout = await page.locator('.form-shell').evaluate((shell) => {
    const formCard = shell.querySelector<HTMLElement>('.change-create-card')
    const sideCard = shell.querySelector<HTMLElement>('.change-create-side .t-card')
    const formBody = formCard?.querySelector<HTMLElement>('.t-card__body')
    const sideBody = sideCard?.querySelector<HTMLElement>('.t-card__body')
    const select = document.querySelector<HTMLElement>('#target-project')
    const intent = document.querySelector<HTMLElement>('#change-intent')
    const submit = document.querySelector<HTMLElement>('.change-create-actions .t-button')
    const formRect = formCard?.getBoundingClientRect()
    const sideRect = sideCard?.getBoundingClientRect()
    const styles = getComputedStyle(shell)
    return {
      columns: styles.gridTemplateColumns.split(' ').filter(Boolean).length,
      gap: styles.gap,
      formPadding: formBody ? getComputedStyle(formBody).paddingTop : '',
      sidePadding: sideBody ? getComputedStyle(sideBody).paddingTop : '',
      selectHeight: select?.getBoundingClientRect().height ?? 0,
      textareaMinHeight: intent ? parseFloat(getComputedStyle(intent).minHeight) : 0,
      submitHeight: submit?.getBoundingClientRect().height ?? 0,
      sideStartsAfterForm: Boolean(formRect && sideRect && sideRect.left > formRect.left + formRect.width),
    }
  })
  expect(desktopLayout).toEqual({ columns: 2, gap: '16px', formPadding: '25px', sidePadding: '20px', selectHeight: 42, textareaMinHeight: 176, submitHeight: 40, sideStartsAfterForm: true })

  await page.setViewportSize({ width: 1024, height: 900 })
  const compactLayout = await page.locator('.form-shell').evaluate((shell) => {
    const formCard = shell.querySelector<HTMLElement>('.change-create-card')
    const sideCard = shell.querySelector<HTMLElement>('.change-create-side .t-card')
    const formRect = formCard?.getBoundingClientRect()
    const sideRect = sideCard?.getBoundingClientRect()
    return {
      columns: getComputedStyle(shell).gridTemplateColumns.split(' ').filter(Boolean).length,
      sideStacksBelowForm: Boolean(formRect && sideRect && sideRect.top >= formRect.bottom),
      horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth,
    }
  })
  expect(compactLayout).toEqual({ columns: 1, sideStacksBelowForm: true, horizontalOverflow: false })
})

for (const width of [375, 768, 1024, 1440]) {
  test(`在 ${width}px 宽度保留深色、可聚焦且无页面横向溢出的创建页`, async ({ page }) => {
    await mockControlPlane(page, [{ project_id: 'project-001', repository_root: '/work/demo', created_at: '2026-09-10T00:00:00Z' }])
    await page.setViewportSize({ width, height: 900 })
    await openCreateChange(page)
    await expectDaemonReady(page)
    const projectSelect = page.locator('#target-project')
    await expect(projectSelect).toBeVisible()
    await projectSelect.focus()
    await expect(projectSelect).toBeFocused()
    await expect.poll(() => page.evaluate(() => ({ background: getComputedStyle(document.body).backgroundColor, horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth }))).toEqual({ background: 'rgb(11, 16, 32)', horizontalOverflow: false })
  })
}

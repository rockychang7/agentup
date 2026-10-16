import { test, expect } from '@playwright/test'
import type { Page } from '@playwright/test'

async function launch(page: Page, token: string) {
  await page.goto(`/#token=${token}`)
  await page.getByRole('combobox').selectOption('en')
  await expect(page.getByRole('switch')).toBeEnabled()
  await expect(page.getByRole('heading', { name: 'Agents', exact: true })).toBeVisible()
}
const lane = (page: Page, name: string) => page.locator('.lane').filter({ has: page.getByText(name, { exact: true }) })
async function noOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
}

test('bilingual preferences, theme, details and responsive layout', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await launch(page, `${info.project.name}-overview`)
  await expect(page).not.toHaveURL(/token=/)
  await page.getByRole('button', { name: 'Show details Codex CLI', exact: true }).click()
  await expect(page.getByText('npm install -g @openai/codex@latest', { exact: true })).toBeVisible()
  await noOverflow(page)
  await page.screenshot({ path: info.outputPath('agents-en.png'), fullPage: true })
  await page.getByRole('combobox').selectOption('zh')
  await expect(page.getByRole('heading', { name: 'Agent 概览' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Agent 概览' })).toBeVisible()
  await page.getByRole('button', { name: '切换主题 · 浅色' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await page.screenshot({ path: info.outputPath('agents-zh-light.png'), fullPage: true })
  await noOverflow(page)
  expect(errors).toEqual([])
})

test('batch upgrades are confirmed and results stay on the row after reload', async ({ page }, info) => {
  await launch(page, `${info.project.name}-upgrade`)
  await expect(page.getByRole('button', { name: 'Upgrade tasks', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Upgrade CLIs (2)', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('checkbox', { name: 'Codex CLI', exact: true })).toBeChecked()
  await expect(page.getByRole('dialog')).toContainText('npm install -g @openai/codex@latest')
  await page.screenshot({ path: info.outputPath('confirmation.png'), fullPage: true })
  await noOverflow(page)
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await page.getByRole('button', { name: 'Upgrade CLIs (2)', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Claude Code CLI', exact: true }).uncheck()
  const request = page.waitForRequest(req => new URL(req.url()).pathname === '/api/upgrades' && req.method() === 'POST')
  await page.getByRole('button', { name: 'Confirm and upgrade', exact: true }).click()
  expect((await request).postDataJSON().tools).toEqual(['codex'])
  await expect(page.getByRole('button', { name: 'Upgrading Codex CLI', exact: true })).toBeDisabled()
  await expect(lane(page, 'Codex CLI')).toContainText('Just upgraded')
  await page.reload()
  await expect(lane(page, 'Codex CLI')).toContainText('Just upgraded')
  await page.getByRole('button', { name: 'Show details Codex CLI', exact: true }).click()
  await expect(page.locator('.lane-log')).toContainText('<script>unsafe</script>')
  await expect(page.locator('.lane-log script')).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('row-result.png'), fullPage: true })
  await noOverflow(page)
})

for (const scenario of [
  { name: 'v1', command: ['npm', 'install', '-g', 'opencode-ai@latest'] },
  { name: 'v2', command: ['pnpm', 'add', '-g', '--allow-build=@opencode/cli', '@opencode/cli@latest'] },
  { name: 'native', command: ['C:\\Users\\Local User\\.opencode\\bin\\opencode.exe', 'upgrade', '--method', 'curl'] },
]) {
  test(`OpenCode ${scenario.name} displays and runs the matching upgrade command`, async ({ page }, info) => {
    await launch(page, `${info.project.name}-opencode-${scenario.name}`)
    const command = scenario.command.map(arg => /[\s"'`]/.test(arg) ? JSON.stringify(arg) : arg).join(' ')
    await page.getByRole('button', { name: 'Show details OpenCode CLI', exact: true }).click()
    await expect(page.getByText(command, { exact: true })).toBeVisible()
    await noOverflow(page)
    const request = page.waitForRequest(req => new URL(req.url()).pathname === '/api/upgrades' && req.method() === 'POST')
    await page.getByRole('button', { name: 'Upgrade OpenCode CLI', exact: true }).click()
    expect((await request).postDataJSON().tools).toEqual(['opencode'])
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(lane(page, 'OpenCode CLI')).toContainText('Just upgraded')
    const state = await (await page.request.get('/api/state')).json()
    expect(state.tasks[0].tools[0].result.command).toEqual(scenario.command)
    await noOverflow(page)
  })
}

test('offline mode disables mutations and diagnosis is bilingual', async ({ page }, info) => {
  await launch(page, `${info.project.name}-offline`)
  await page.getByRole('switch').click()
  await expect(page.getByRole('switch')).toBeChecked()
  await expect(page.getByRole('button', { name: 'Upgrade Codex CLI', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Diagnostics', exact: true }).click()
  await page.getByRole('button', { name: 'Run diagnostics', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Package managers', exact: true })).toBeVisible()
  await page.getByRole('combobox').selectOption('zh')
  await expect(page.getByRole('heading', { name: '包管理器', exact: true })).toBeVisible()
  await expect(page.getByText('可选', { exact: true }).first()).toBeVisible()
  await page.screenshot({ path: info.outputPath('diagnostics.png'), fullPage: true })
  await noOverflow(page)
})

test('failure results and raw output remain distinct', async ({ page }, info) => {
  await launch(page, `${info.project.name}-failure`)
  await page.getByRole('button', { name: 'Upgrade Codex CLI', exact: true }).click()
  await expect(lane(page, 'Codex CLI')).toContainText('Last run: Failed')
  await page.getByRole('button', { name: 'Show details Codex CLI', exact: true }).click()
  await expect(page.locator('.lane-log')).toContainText('EACCES')
  await expect(page.getByText('Upgrade did not complete. Check the raw output.')).toBeVisible()
  await page.getByRole('combobox').selectOption('zh')
  await expect(page.getByText('升级未完成，请检查原始输出。')).toBeVisible()
  await noOverflow(page)
})

test('opening without a token stays read-only', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('combobox').selectOption('en')
  await expect(page.getByRole('heading', { name: 'A valid local session is required' })).toBeVisible()
  await expect(page.getByRole('switch')).toHaveCount(0)
})

test('narrow and intermediate layouts keep controls separated', async ({ page }, info) => {
  await launch(page, `${info.project.name}-widths`)
  for (const width of [320, 700, 981, 1200]) {
    await page.setViewportSize({ width, height: 900 })
    await noOverflow(page)
    const boxes = await page.locator('.lane-row').first().locator('.lane-name, .versions, .lane-status, .lane-action, .lane-toggle').evaluateAll(nodes =>
      nodes.map(node => {
        const { left, top, right, bottom } = node.getBoundingClientRect()
        return { left, top, right, bottom }
      }),
    )
    for (let a = 0; a < boxes.length; a++) {
      for (let b = a + 1; b < boxes.length; b++) {
        expect(boxes[a].left < boxes[b].right && boxes[a].right > boxes[b].left &&
          boxes[a].top < boxes[b].bottom && boxes[a].bottom > boxes[b].top).toBe(false)
      }
    }
    await page.getByRole('button', { name: 'Upgrade CLIs (2)', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await noOverflow(page)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).toHaveCount(0)
  }
})

test('clipboard controls copy the displayed command', async ({ page, context }, info) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await launch(page, `${info.project.name}-copy`)
  await page.getByRole('button', { name: 'Show details Codex CLI', exact: true }).click()
  await page.locator('.lane-details dd').filter({ hasText: 'npm install -g @openai/codex@latest' }).getByRole('button', { name: 'Copy', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('npm install -g @openai/codex@latest')
})

test('language and theme survive a local server port change', async ({ page }, info) => {
  await launch(page, `${info.project.name}-preferences`)
  await page.getByRole('combobox').selectOption('zh')
  await page.getByRole('button', { name: '切换主题 · 浅色' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await page.goto(`http://127.0.0.1:42732/#token=${info.project.name}-new-port`)
  await expect(page.getByRole('heading', { name: 'Agent 概览' })).toBeVisible()
  await expect(page.getByRole('combobox')).toHaveValue('zh')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
})

test('service shutdown requires confirmation and keeps other instances connected', async ({ page, context }, info) => {
  const token = `${info.project.name}-shutdown`
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await launch(page, token)
  const sameInstance = await context.newPage()
  await launch(sameInstance, token)
  const otherInstance = await context.newPage()
  await otherInstance.goto(`http://127.0.0.1:42732/#token=${info.project.name}-other-instance`)
  await otherInstance.getByRole('combobox').selectOption('en')
  await expect(otherInstance.getByRole('switch')).toBeEnabled()
  let shutdownRequests = 0
  page.on('request', request => {
    if (new URL(request.url()).pathname === '/api/shutdown') shutdownRequests++
  })
  await page.getByRole('button', { name: 'Stop this service', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('Other instances are unaffected')
  await noOverflow(page)
  await page.screenshot({ path: info.outputPath('shutdown-confirmation.png'), fullPage: true })
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect(shutdownRequests).toBe(0)
  await expect(page.getByRole('switch')).toBeEnabled()
  await page.getByRole('button', { name: 'Stop this service', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm stop', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Service stopped' })).toBeVisible()
  await expect(sameInstance.getByRole('heading', { name: 'Service stopped' })).toBeVisible()
  await expect(otherInstance.getByRole('switch')).toBeEnabled()
  expect(shutdownRequests).toBe(1)
  await expect(page.getByRole('button', { name: 'Reconnect', exact: true })).toHaveCount(0)
  await page.getByRole('combobox').selectOption('zh')
  await expect(page.getByRole('heading', { name: '服务已停止' })).toBeVisible()
  await expect(page.getByText('当前本地会话已结束，其他 UI 实例不受影响。')).toBeVisible()
  await page.screenshot({ path: info.outputPath('service-stopped-zh.png'), fullPage: true })
  await noOverflow(page)
  expect(errors).toEqual([])
})

test('shutdown controls protect an active upgrade', async ({ page }, info) => {
  await launch(page, `${info.project.name}-shutdown-upgrade`)
  await page.getByRole('button', { name: 'Upgrade Codex CLI', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Upgrades are still running. Stop the service after they finish.' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Stop this service', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'Stop this service', exact: true }).click()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('a running upgrade leaves other rows usable and current rows greyed out', async ({ page }, info) => {
  await launch(page, `${info.project.name}-queue`)
  await page.getByRole('button', { name: 'Upgrade Codex CLI', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Upgrading Codex CLI', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Upgrade Claude Code CLI', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Upgrade CLIs (1)', exact: true })).toBeEnabled()
  await page.screenshot({ path: info.outputPath('upgrading.png'), fullPage: true })
  await expect(page.getByRole('button', { name: 'Upgrade Codex CLI', exact: true })).toBeDisabled()
  await expect(lane(page, 'Codex CLI').getByText('Up to date', { exact: true })).toBeVisible()
  await noOverflow(page)
})

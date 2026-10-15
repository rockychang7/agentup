import { test, expect } from '@playwright/test'
import type { Page } from '@playwright/test'

async function launchDesktop(page: Page, token: string) {
  await page.goto(`/#token=${token}`)
  await page.getByRole('combobox').selectOption('en')
  await expect(page.getByRole('switch')).toBeEnabled()
  await page.getByRole('radio', { name: 'Apps only', exact: true }).click()
  await expect(page.getByText('ChatGPT / Codex Desktop', { exact: true })).toBeVisible()
  await expect(page.getByText('Codex CLI', { exact: true })).toHaveCount(0)
}

test('Desktop apps are labeled separately from CLI, exclude IDE and stay responsive', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await launchDesktop(page, `${info.project.name}-desktop-overview`)
  await expect(page.getByText('Antigravity app', { exact: true })).toBeVisible()
  await expect(page.getByText('Antigravity IDE', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Get update Antigravity app', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Upgrade apps (1)', exact: true })).toBeEnabled()
  await expect(page.locator('.lane .kind-chip.desktop')).toHaveCount(4)
  await expect(page.locator('.lane .kind-chip.cli')).toHaveCount(0)
  await page.getByRole('button', { name: 'Show details ChatGPT / Codex Desktop', exact: true }).click()
  await expect(page.getByText('/Applications/ChatGPT.app', { exact: true })).toBeVisible()
  for (const width of [320, 700, 981, 1440]) {
    await page.setViewportSize({ width, height: 960 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const boxes = await page.locator('.lane-row').first().locator('.lane-name, .versions, .lane-status, .lane-action, .lane-toggle').evaluateAll(nodes =>
      nodes.map(node => {
        const { left, top, right, bottom } = node.getBoundingClientRect()
        return { left, top, right, bottom }
      }),
    )
    for (let a = 0; a < boxes.length; a++) for (let b = a + 1; b < boxes.length; b++) {
      expect(boxes[a].left < boxes[b].right && boxes[a].right > boxes[b].left &&
        boxes[a].top < boxes[b].bottom && boxes[a].bottom > boxes[b].top).toBe(false)
    }
  }
  await page.setViewportSize(info.project.name === 'mobile' ? { width: 390, height: 844 } : { width: 1440, height: 960 })
  await page.screenshot({ path: info.outputPath('desktop-overview.png'), fullPage: true })
  await page.getByRole('radio', { name: 'CLI + App', exact: true }).click()
  const codex = page.getByRole('region', { name: 'Codex', exact: true })
  await expect(codex.getByText('Codex CLI', { exact: true })).toBeVisible()
  await expect(codex.getByText('ChatGPT / Codex Desktop', { exact: true })).toBeVisible()
  await page.getByRole('radio', { name: 'CLI only', exact: true }).click()
  await expect(page.getByText('ChatGPT / Codex Desktop', { exact: true })).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('radio', { name: 'CLI only', exact: true })).toHaveAttribute('aria-checked', 'true')
  expect(errors).toEqual([])
})

test('guided update records a handoff, not a successful upgrade', async ({ page }, info) => {
  await launchDesktop(page, `${info.project.name}-desktop-guided`)
  await page.getByRole('button', { name: 'Get update Antigravity app', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('https://antigravity.google/download')
  await expect(page.getByRole('dialog')).toContainText('not a verified upgrade')
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  const initial = await (await page.request.get('/api/state')).json()
  expect(initial.desktop_tasks).toEqual([])
  await page.getByRole('button', { name: 'Get update Antigravity app', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Get update', exact: true }).click()
  await page.getByRole('button', { name: 'View tasks', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Upgrade tasks', exact: true })).toBeVisible()
  await expect(page.getByText('Manual action required', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Updated', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Get update' })).toHaveAttribute('href', 'https://antigravity.google/download')
  await page.reload()
  await page.getByRole('button', { name: 'Upgrade tasks', exact: true }).click()
  await expect(page.getByText('Manual action required', { exact: true }).first()).toBeVisible()
  await page.getByRole('combobox').selectOption('zh')
  await expect(page.getByText('待手动更新', { exact: true }).first()).toBeVisible()
  await page.screenshot({ path: info.outputPath('desktop-guided-task.png'), fullPage: true })
})

test('automatic Desktop plans require confirmation and submit IDs only', async ({ page }, info) => {
  await launchDesktop(page, `${info.project.name}-desktop-upgrade`)
  await page.getByRole('button', { name: 'Upgrade apps (1)', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('brew upgrade --cask chatgpt')
  await expect(page.getByRole('dialog')).toContainText('Fully quit the app')
  await page.screenshot({ path: info.outputPath('desktop-confirmation.png'), fullPage: true })
  const pending = page.waitForRequest(req => new URL(req.url()).pathname === '/api/desktop/upgrades' && req.method() === 'POST')
  await page.getByRole('button', { name: 'Confirm and upgrade', exact: true }).click()
  const body = (await pending).postDataJSON()
  expect(body.plans).toEqual(['plan:codex.desktop'])
  expect(body.command).toBeUndefined()
  expect(body.url).toBeUndefined()
  await page.getByRole('button', { name: 'View tasks', exact: true }).click()
  await expect(page.getByText('Updated', { exact: true })).toBeVisible()
  await expect(page.locator('.log-output')).toContainText('<script>unsafe</script>')
  await expect(page.locator('.log-output script')).toHaveCount(0)
})

test('offline Desktop mode disables automatic and guided operations', async ({ page }, info) => {
  await launchDesktop(page, `${info.project.name}-desktop-offline`)
  await page.getByRole('switch').click()
  await expect(page.getByRole('switch')).toBeChecked()
  await expect(page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Get update Antigravity app', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Upgrade apps (1)', exact: true })).toBeDisabled()
  const response = await page.request.post('/api/desktop/upgrades', { data: { plans: ['plan:codex.desktop'], revision: 1 } })
  expect(response.status()).toBe(400)
})

test('a changed snapshot cannot confirm an old Desktop plan', async ({ page }, info) => {
  await launchDesktop(page, `${info.project.name}-desktop-stale`)
  await page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.request.post('/api/refresh', { data: { offline: false } })
  await expect(page.getByRole('button', { name: 'Confirm and upgrade', exact: true })).toBeDisabled()
})

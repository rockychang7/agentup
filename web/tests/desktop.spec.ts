import { test, expect } from '@playwright/test'
import type { Page } from '@playwright/test'

async function launchDesktop(page: Page, token: string) {
  await page.goto(`/#token=${token}`)
  await page.getByRole('combobox').selectOption('en')
  await expect(page.getByRole('switch')).toBeEnabled()
  await page.getByRole('radio', { name: 'Desktop apps', exact: true }).click()
  await expect(page.getByText('ChatGPT / Codex Desktop', { exact: true })).toBeVisible()
  await expect(page.getByText('Codex CLI', { exact: true })).toHaveCount(0)
}
const lane = (page: Page, name: string) => page.locator('.lane').filter({ has: page.getByText(name, { exact: true }) })

test('Desktop apps and CLI tools never share a view, exclude IDE and stay responsive', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await launchDesktop(page, `${info.project.name}-desktop-overview`)
  await expect(page.getByRole('radio')).toHaveCount(2)
  await expect(page.getByText('CLI + App')).toHaveCount(0)
  await expect(page.getByText('Antigravity app', { exact: true })).toBeVisible()
  await expect(page.getByText('Antigravity IDE', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Get update Antigravity app', exact: true })).toHaveAttribute('href', 'https://antigravity.google/download')
  await expect(page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Upgrade apps (1)', exact: true })).toBeEnabled()
  await expect(page.locator('.lane')).toHaveCount(4)
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
  await page.getByRole('radio', { name: 'CLI tools', exact: true }).click()
  await expect(page.getByText('Codex CLI', { exact: true })).toBeVisible()
  await expect(page.getByText('ChatGPT / Codex Desktop', { exact: true })).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('radio', { name: 'CLI tools', exact: true })).toHaveAttribute('aria-checked', 'true')
  expect(errors).toEqual([])
})

test('guided update opens the official page and never claims an upgrade', async ({ page }, info) => {
  await launchDesktop(page, `${info.project.name}-desktop-guided`)
  const link = page.getByRole('link', { name: 'Get update Antigravity app', exact: true })
  await expect(link).toHaveAttribute('target', '_blank')
  await expect(link).toHaveAttribute('rel', /noopener/)
  const state = await (await page.request.get('/api/state')).json()
  expect(state.desktop_tasks).toEqual([])
  await expect(lane(page, 'Antigravity app')).not.toContainText('Just upgraded')
})

test('a single automatic app upgrades on click and submits IDs only', async ({ page }, info) => {
  await launchDesktop(page, `${info.project.name}-desktop-upgrade`)
  const pending = page.waitForRequest(req => new URL(req.url()).pathname === '/api/desktop/upgrades' && req.method() === 'POST')
  await page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true }).click()
  const body = (await pending).postDataJSON()
  expect(body.plans).toEqual(['plan:codex.desktop'])
  expect(body.command).toBeUndefined()
  expect(body.url).toBeUndefined()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Upgrading ChatGPT / Codex Desktop', exact: true })).toBeDisabled()
  await expect(lane(page, 'ChatGPT / Codex Desktop')).toContainText('Just upgraded')
  await page.getByRole('button', { name: 'Show details ChatGPT / Codex Desktop', exact: true }).click()
  await expect(page.locator('.lane-log')).toContainText('<script>unsafe</script>')
  await expect(page.locator('.lane-log script')).toHaveCount(0)
})

test('batch app upgrades are confirmed first', async ({ page }, info) => {
  await launchDesktop(page, `${info.project.name}-desktop-batch`)
  await page.getByRole('button', { name: 'Upgrade apps (1)', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('brew upgrade --cask chatgpt')
  await expect(page.getByRole('dialog')).toContainText('stays closed')
  await page.screenshot({ path: info.outputPath('desktop-confirmation.png'), fullPage: true })
  const pending = page.waitForRequest(req => new URL(req.url()).pathname === '/api/desktop/upgrades' && req.method() === 'POST')
  await page.getByRole('button', { name: 'Confirm and upgrade', exact: true }).click()
  expect((await pending).postDataJSON().plans).toEqual(['plan:codex.desktop'])
})

test('a running app is quit only after the user confirms, then it can upgrade', async ({ page }, info) => {
  let stops = 0
  page.on('request', req => { if (new URL(req.url()).pathname === '/api/desktop/stop') stops++ })
  await launchDesktop(page, `${info.project.name}-desktop-running`)
  await expect(lane(page, 'ChatGPT / Codex Desktop')).toContainText('The app is running. Quit it before upgrading.')
  await expect(page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Quit app ChatGPT / Codex Desktop', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('Quit ChatGPT / Codex Desktop?')
  await expect(page.getByRole('dialog')).toContainText('unsaved work')
  await page.screenshot({ path: info.outputPath('quit-confirmation.png'), fullPage: true })
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect(stops).toBe(0)
  await page.getByRole('button', { name: 'Quit app ChatGPT / Codex Desktop', exact: true }).click()
  const pending = page.waitForRequest(req => new URL(req.url()).pathname === '/api/desktop/stop')
  await page.getByRole('dialog').getByRole('button', { name: 'Quit app', exact: true }).click()
  expect((await pending).postDataJSON().installation_id).toBe('fixture:codex.desktop')
  await expect(page.getByRole('button', { name: 'Quitting app ChatGPT / Codex Desktop', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true })).toBeEnabled()
  expect(stops).toBe(1)
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
  await page.getByRole('button', { name: 'Upgrade apps (1)', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.request.post('/api/refresh', { data: { offline: false } })
  await expect(page.getByRole('button', { name: 'Confirm and upgrade', exact: true })).toBeDisabled()
})

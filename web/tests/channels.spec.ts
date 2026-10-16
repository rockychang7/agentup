import { test, expect } from '@playwright/test'
import type { Page } from '@playwright/test'

async function launchApps(page: Page, token: string) {
  await page.goto(`/#token=${token}`)
  await page.getByRole('combobox').selectOption('en')
  await expect(page.getByRole('switch')).toBeEnabled()
  await page.getByRole('radio', { name: 'Desktop apps', exact: true }).click()
}
const lane = (page: Page, name: string) => page.locator('.lane').filter({ has: page.getByText(name, { exact: true }) })

test('Codex is confirmed on its own and batch upgrades skip it', async ({ page }, info) => {
  await launchApps(page, `${info.project.name}-channels-batch`)
  await expect(page.getByText('Confirmed on its own; skipped by batch upgrades.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Upgrade ChatGPT / Codex Desktop', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'Upgrade apps (1)', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('Google.Antigravity')
  await expect(page.getByRole('dialog')).not.toContainText('9PLM9XGG6VKS')
  await expect(page.getByRole('dialog')).toContainText('Source: WinGet')
  await expect(page.getByRole('dialog')).toContainText('WinGet community source')
  const request = page.waitForRequest(req => new URL(req.url()).pathname === '/api/desktop/upgrades')
  await page.getByRole('button', { name: 'Confirm and upgrade', exact: true }).click()
  expect((await request).postDataJSON().plans).toEqual(['plan:antigravity.desktop'])
  await expect(lane(page, 'Antigravity app')).toContainText('Just upgraded')
})

test('installer plans show what is verified before the installer opens', async ({ page }, info) => {
  await launchApps(page, `${info.project.name}-channels-installer`)
  await page.getByRole('button', { name: 'Show details OpenCode Desktop', exact: true }).click()
  await expect(page.locator('.lane-details')).toContainText('Official update manifest')
  await page.getByRole('button', { name: 'Download & install OpenCode Desktop', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText('https://opencode.ai/files/bin/2.0.29/opencode-desktop-win-x64.exe')
  await expect(dialog).toContainText('Publisher: Anomaly Innovations, Inc')
  await expect(dialog).toContainText('SHA-512')
  await expect(dialog).toContainText('publisher signature')
  await page.screenshot({ path: info.outputPath('installer-confirmation.png'), fullPage: true })
  await dialog.getByRole('button', { name: 'Download & install', exact: true }).click()
  await expect(lane(page, 'OpenCode Desktop')).toContainText('Just upgraded')
})

test('privileged commands are copied for a terminal and never submitted', async ({ page, context }, info) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  let submitted = 0
  page.on('request', req => { if (new URL(req.url()).pathname === '/api/desktop/upgrades') submitted++ })
  await launchApps(page, `${info.project.name}-channels-linux`)
  await expect(page.getByRole('button', { name: 'Upgrade apps (0)', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Copy command ChatGPT / Codex Desktop', exact: true }).click()
  await expect(page.getByText('Copied. Run it in a terminal, then refresh.')).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('sudo apt-get install --only-upgrade chatgpt')
  expect(submitted).toBe(0)
  const response = await page.request.post('/api/desktop/upgrades', { data: { plans: ['plan:codex.desktop'], revision: 1 } })
  expect(response.status()).toBe(400)
  await page.screenshot({ path: info.outputPath('privileged.png'), fullPage: true })
})

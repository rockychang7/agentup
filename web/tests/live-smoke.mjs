import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { chromium, expect } from '@playwright/test'

// This smoke test requires a read-only server and never starts an upgrade.
const launch = new URL(process.argv[2] || '')
assert.equal(launch.hostname, '127.0.0.1')
const token = new URLSearchParams(launch.hash.slice(1)).get('token')
assert.ok(token, 'Pass the complete launch URL from agentup ui --offline --no-open')
const output = fileURLToPath(new URL('../../dist/preview/', import.meta.url))
const browser = await chromium.launch({ headless: true, ...(process.platform === 'win32' ? { channel: 'msedge' } : {}) })
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 }, locale: 'zh-CN' })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(`${message.location().url}: ${message.text()}`) })
  page.on('requestfailed', request => errors.push(`${request.url()}: ${request.failure()?.errorText}`))
  page.on('response', response => { if (response.status() >= 400) errors.push(`${response.status()} ${response.url()}`) })
  const response = await page.goto(launch.href)
  assert.equal(response.status(), 200)
  assert.match(response.headers()['content-security-policy'], /script-src 'self'/)
  await expect(page.getByRole('heading', { name: 'Agent 概览' })).toBeVisible()
  await expect(page.getByRole('switch')).toBeChecked()
  await expect(page.getByRole('switch')).toBeEnabled({ timeout: 120000 })
  await expect(page.getByRole('button', { name: '升级 Codex CLI', exact: true })).toBeDisabled()
  assert.ok(!page.url().includes('token='))
  const snapshot = await (await context.request.get(`${launch.origin}/api/state`)).json()
  assert.equal(snapshot.offline, true)
  assert.equal(snapshot.tasks.length, 0)
  assert.equal(snapshot.agents.length, 4)
  assert.ok(!snapshot.scanning)
  await page.screenshot({ path: `${output}/agents-live-zh.png`, fullPage: true })
  assert.ok(snapshot.desktop.installations.length >= 4)
  assert.equal(snapshot.desktop_tasks.length, 0)
  assert.deepEqual(
    [...new Set(snapshot.desktop.installations.map(info => info.product_id))].sort(),
    ['antigravity.desktop', 'claude.desktop', 'codex.desktop', 'opencode.desktop'],
  )
  assert.ok(snapshot.desktop.installations.every(info => !info.plan))
  await page.getByRole('radio', { name: '只看 App', exact: true }).click()
  await expect(page.locator('.agent-groups')).toBeVisible()
  await expect(page.getByText('Antigravity app', { exact: true })).toBeVisible()
  await expect(page.getByText('Antigravity IDE', { exact: true })).toHaveCount(0)
  for (const action of await page.locator('.lane:has(.kind-chip.desktop) .lane-action button').all()) {
    await expect(action).toBeDisabled()
  }
  await page.screenshot({ path: `${output}/desktop-live-zh.png`, fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  await page.screenshot({ path: `${output}/desktop-live-mobile.png`, fullPage: true })
  await page.setViewportSize({ width: 1440, height: 960 })
  await page.getByRole('radio', { name: 'CLI + App', exact: true }).click()

  await page.getByRole('combobox').selectOption('en')
  await expect(page.getByRole('heading', { name: 'Agents', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Diagnostics', exact: true }).click()
  await page.getByRole('button', { name: 'Run diagnostics', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Package managers', exact: true })).toBeVisible({ timeout: 120000 })
  await page.screenshot({ path: `${output}/diagnostics-live-en.png`, fullPage: true })
  await page.getByRole('combobox').selectOption('zh')
  await page.getByRole('button', { name: 'Agent 概览', exact: true }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  await page.screenshot({ path: `${output}/agents-live-mobile.png`, fullPage: true })
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Agent 概览' })).toBeVisible()
  await expect(page.getByRole('switch')).toBeEnabled()

  const rejected = await context.request.post(`${launch.origin}/api/upgrades`, {
    headers: { Origin: launch.origin, 'X-AgentUp-Token': token },
    data: { tools: ['codex'], revision: snapshot.revision },
  })
  assert.equal(rejected.status(), 400)
  assert.equal((await rejected.json()).error, 'offline_read_only')
  const desktopRejected = await context.request.post(`${launch.origin}/api/desktop/upgrades`, {
    headers: { Origin: launch.origin, 'X-AgentUp-Token': token },
    data: { plans: ['not-an-executable-plan'], revision: snapshot.revision },
  })
  assert.equal(desktopRejected.status(), 400)
  assert.equal((await desktopRejected.json()).error, 'offline_read_only')
  const crossOrigin = await context.request.post(`${launch.origin}/api/doctor`, {
    headers: { Origin: 'https://untrusted.example', 'X-AgentUp-Token': token }, data: {},
  })
  assert.equal(crossOrigin.status(), 403)
  const unauthenticated = await fetch(`${launch.origin}/api/state`)
  assert.equal(unauthenticated.status, 401)
  assert.deepEqual(errors, [])
  console.log(JSON.stringify({
    version: snapshot.version, installed: snapshot.agents.filter(agent => agent.installed).length,
    desktops: snapshot.desktop.installations.filter(info => info.installed).map(info => info.product_id),
    bilingual: true, responsive: true, offline: true, authentication: true,
    browserErrors: errors, screenshots: output,
  }, null, 2))
  await context.close()
} finally {
  await browser.close()
}

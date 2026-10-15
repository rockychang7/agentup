import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, sep, extname } from 'node:path'
import { fileURLToPath } from 'node:url'

// Only synthetic fixtures live here. This server cannot run external commands.
const assets = fileURLToPath(new URL('../../internal/webui/assets/', import.meta.url))
const sessions = new Map()
const labels = ['codex', 'claude-code', 'opencode', 'agy']
function desktopFixture(automatic) {
  return {
    installations: [
      ['codex.desktop', 'ChatGPT / Codex Desktop', '26.1007.9446', '26.1013.11914', 'https://chatgpt.com/download/'],
      ['claude.desktop', 'Claude Desktop', '', '', 'https://claude.com/download'],
      ['opencode.desktop', 'OpenCode Desktop', '2.0.29', '', 'https://opencode.ai/download'],
      ['antigravity.desktop', 'Antigravity app', '2.18.1', '', 'https://antigravity.google/download'],
    ].map(([product_id, name, version, latest_version, update_url], index) => {
      const id = `fixture:${product_id}`
      const auto = automatic && index === 0
      const installed = !!version
      const path = installed ? `/Applications/${index === 0 ? 'ChatGPT' : index === 2 ? 'OpenCode' : 'Antigravity'}.app` : ''
      return {
        id, product_id, name, kind: 'desktop', installed, version,
        latest_version: auto ? latest_version : '', latest_check: auto ? 'checked' : 'unavailable',
        upgrade_available: auto, source: auto ? 'brew-cask' : 'app-bundle',
        scope: 'system', arch: auto ? 'amd64' : 'unknown', channel: auto ? 'stable' : 'unknown',
        path, package_id: `example.${product_id}`, running: 'stopped',
        capability: !installed ? 'blocked' : auto ? 'automatic' : 'guided',
        reason: !installed ? 'not_installed' : auto ? '' : 'manual_channel', update_url,
        ...(installed ? { plan: {
          id: `plan:${product_id}`, installation_id: id, product_id,
          before_version: version, target_version: auto ? latest_version : '',
          path, package_id: `example.${product_id}`, source: auto ? 'brew-cask' : 'app-bundle',
          scope: 'system', arch: auto ? 'amd64' : 'unknown', channel: auto ? 'stable' : 'unknown',
          action: auto ? 'command' : 'guide',
          ...(auto ? { command: ['brew', 'upgrade', '--cask', 'chatgpt'] } : { url: update_url }),
          requires_exit: auto, requires_admin: false,
        } } : {}),
      }
    }),
    issues: [],
  }
}
function planned(info, plan) {
  return { ...info, plan: { id: `plan:${info.product_id}`, installation_id: info.id, product_id: info.product_id,
    before_version: info.version, target_version: info.latest_version, path: info.path, package_id: info.package_id,
    source: info.source, scope: info.scope, arch: info.arch, channel: info.channel, requires_exit: true, requires_admin: false, ...plan } }
}
// Windows WinGet/installer channels, or a Linux APT channel that needs a terminal.
function channelFixture(linux) {
  const base = (product_id, name, version, latest, extra) => ({
    id: `fixture:${product_id}`, product_id, name, kind: 'desktop', installed: !!version, version,
    latest_version: latest, latest_check: latest ? 'checked' : 'unavailable', upgrade_available: !!latest && latest !== version,
    path: `C:\\Apps\\${name}`, package_id: `example.${product_id}`, scope: 'user', arch: 'amd64', running: 'stopped',
    reason: '', update_url: 'https://opencode.ai/download', ...extra,
  })
  if (linux) return { issues: [], installations: [
    planned(base('codex.desktop', 'ChatGPT / Codex Desktop', '26.1007.1-1', '26.1013.2-1', { source: 'apt', scope: 'system', channel: 'unknown', capability: 'privileged', latest_source: 'apt', path: '/usr/share/applications/chatgpt.desktop' }),
      { action: 'privileged_command', command: ['sudo', 'apt-get', 'install', '--only-upgrade', 'chatgpt'], requires_admin: true }),
  ] }
  return { issues: [], installations: [
    planned(base('codex.desktop', 'ChatGPT / Codex Desktop', '26.1007.9446.0', '', { source: 'msix', channel: 'msstore', capability: 'automatic', latest_source: 'msstore' }),
      { action: 'command', command: ['winget.exe', 'upgrade', '--id', '9PLM9XGG6VKS', '--exact', '--source', 'msstore', '--disable-interactivity'], standalone: true }),
    planned(base('opencode.desktop', 'OpenCode Desktop', '2.0.28', '2.0.29', { source: 'installer', channel: 'official', capability: 'interactive', latest_source: 'vendor-manifest' }),
      { action: 'installer', asset: { url: 'https://opencode.ai/files/bin/2.0.29/opencode-desktop-win-x64.exe', sha512: 'iaC0Y6POBpAvIUBHSNJ0Jk4/m8fd', size: 236673488, publisher: 'Anomaly Innovations, Inc' } }),
    planned(base('antigravity.desktop', 'Antigravity app', '2.18.1', '2.23.1', { source: 'installer', channel: 'winget', capability: 'automatic', latest_source: 'winget' }),
      { action: 'command', command: ['winget.exe', 'upgrade', '--id', 'Google.Antigravity', '--exact', '--source', 'winget', '--disable-interactivity'] }),
  ] }
}
function newSession(key) {
  const state = {
    version: '0.6.0-dev', os: 'Windows', arch: 'amd64', offline: false,
    revision: 1, scanning: false, diagnosing: false, active_task: '', tasks: [], doctor: null,
    agents: labels.map((name, i) => ({
      name, installed: i !== 2, version: i === 0 ? '1.2.0' : i === 1 ? '2.1.0' : i === 3 ? '0.3.5' : '',
      latest_version: i === 0 ? '1.3.0' : i === 1 ? '2.2.0' : '',
      upgrade_available: i < 2, install_method: i === 3 ? 'self-update' : 'npm',
      path: i === 2 ? '' : `C:\\Users\\Local User\\AppData\\Roaming\\npm\\${name}.cmd`,
      upgrade_supported: i !== 2, latest_check: i === 3 ? 'unavailable' : 'checked',
      command: i === 2 ? [] : i === 3 ? ['C:\\tools\\agy.exe', 'update'] : ['npm', 'install', '-g', i === 0 ? '@openai/codex@latest' : '@anthropic-ai/claude-code@latest'],
    })),
  }
  state.desktop = desktopFixture(key.includes('-desktop-'))
  state.desktop_tasks = []
  if (key.includes('-desktop-')) state.os = 'macOS'
  if (key.includes('-channels-')) state.desktop = channelFixture(key.includes('-linux'))
  if (key.includes('-linux')) state.os = 'Linux'
  const scenario = key.match(/-opencode-(v1|v2|native)$/)?.[1]
  if (scenario) {
    const nativePath = 'C:\\Users\\Local User\\.opencode\\bin\\opencode.exe'
    const v1 = scenario === 'v1'
    Object.assign(state.agents.find(agent => agent.name === 'opencode'), {
      installed: true, version: v1 ? '1.18.0' : '2.0.28',
      latest_version: v1 ? '1.19.0' : '2.0.29', upgrade_available: true,
      upgrade_supported: true, latest_check: 'checked',
      install_method: scenario === 'native' ? 'self-update' : v1 ? 'npm' : 'pnpm',
      path: scenario === 'native' ? nativePath : 'C:\\Users\\Local User\\AppData\\Roaming\\npm\\opencode.cmd',
      command: scenario === 'native' ? [nativePath, 'upgrade', '--method', 'curl']
        : v1 ? ['npm', 'install', '-g', 'opencode-ai@latest']
          : ['pnpm', 'add', '-g', '--allow-build=@opencode/cli', '@opencode/cli@latest'],
    })
  }
  return { state, streams: new Set(), stopped: false }
}
function broadcast(session) {
  session.state.revision++
  for (const stream of session.streams) stream.write(`event: state\ndata: ${JSON.stringify(session.state)}\n\n`)
}
function json(response, code, body) {
  response.writeHead(code, { 'Content-Type': 'application/json' })
  response.end(JSON.stringify(body))
}
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.woff': 'font/woff', '.woff2': 'font/woff2' }
const handle = async (req, res) => {
  try {
    const cookieName = `fixture_${req.socket.localPort}`
    const url = new URL(req.url, 'http://127.0.0.1')
    if (!url.pathname.startsWith('/api/')) {
      const path = resolve(assets, url.pathname === '/' ? 'index.html' : `.${url.pathname}`)
      if (!path.startsWith(assets.endsWith(sep) ? assets : assets + sep)) { res.writeHead(404).end(); return }
      res.writeHead(200, { 'Content-Type': mime[extname(path)] || 'application/octet-stream' })
      res.end(await readFile(path))
      return
    }
    let body = {}
    if (req.method === 'POST') {
      let text = ''
      for await (const chunk of req) text += chunk
      body = JSON.parse(text)
    }
    if (url.pathname === '/api/session') {
      if (!body.token) return json(res, 401, { error: 'unauthorized' })
      if (!sessions.has(body.token)) sessions.set(body.token, newSession(body.token))
      res.setHeader('Set-Cookie', `${cookieName}=${encodeURIComponent(body.token)}; Path=/api/; HttpOnly; SameSite=Strict`)
      return json(res, 200, { ok: true })
    }
    const key = decodeURIComponent((req.headers.cookie || '').match(new RegExp(`${cookieName}=([^;]+)`))?.[1] || '')
    const session = sessions.get(key)
    if (!session) return json(res, 401, { error: 'unauthorized' })
    const { state } = session
    if (url.pathname === '/api/shutdown') {
      if (state.active_task && !body.force) return json(res, 409, { error: 'busy' })
      session.stopped = true
      json(res, 202, { ok: true })
      for (const stream of session.streams) {
        stream.end('event: shutdown\ndata: {}\n\n')
      }
      session.streams.clear()
      return
    }
    if (url.pathname === '/api/state') return json(res, 200, state)
    if (url.pathname === '/api/events') {
      res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store' })
      session.streams.add(res)
      res.write(`event: state\ndata: ${JSON.stringify(state)}\n\n`)
      if (session.stopped) {
        res.end('event: shutdown\ndata: {}\n\n')
        session.streams.delete(res)
        return
      }
      req.on('close', () => session.streams.delete(res))
      return
    }
    if (url.pathname === '/api/refresh') {
      state.offline = !!body.offline
      state.agents.forEach(a => a.latest_check = state.offline ? 'offline' : a.name === 'agy' ? 'unavailable' : 'checked')
      state.desktop.installations.forEach(a => {
        a.latest_check = state.offline ? 'offline' : a.upgrade_available ? 'checked' : 'unavailable'
      })
      broadcast(session)
      return json(res, 202, { ok: true })
    }
    if (url.pathname === '/api/doctor') {
      state.doctor = {
        os: state.os,
        managers: [
          { name: 'npm', found: true, version: '11.10.0', path: 'C:\\Program Files\\nodejs\\npm.cmd' },
          { name: 'pnpm', found: false, version: '', path: '' },
          { name: 'yarn', found: false, version: '', path: '' },
          { name: 'winget', found: true, version: '1.12.0', path: 'C:\\Users\\Local User\\AppData\\Local\\Microsoft\\WindowsApps\\winget.exe' },
        ],
        tools: state.agents.map(a => ({ name: a.name, found: a.installed, path: a.path })),
      }
      broadcast(session)
      return json(res, 202, { ok: true })
    }
    if (url.pathname === '/api/upgrades') {
      if (state.offline) return json(res, 400, { error: 'offline_read_only' })
      if (state.active_task) return json(res, 409, { error: 'busy' })
      if (body.revision !== state.revision) return json(res, 409, { error: 'stale_preview' })
      const id = `${Date.now()}-test`
      const task = {
        id, status: 'running', started_at: new Date().toISOString(),
        tools: body.tools.map(name => ({ name, phase: 'upgrading' })),
        logs: [{ name: body.tools[0], stream: 'stdout', text: 'Resolving packages...\n' }], log_truncated: false,
      }
      state.tasks.push(task)
      state.active_task = id
      broadcast(session)
      setTimeout(() => {
        task.status = key.includes('failure') ? 'failed' : 'completed'
        task.finished_at = new Date().toISOString()
        task.logs.push({ name: body.tools[0], stream: task.status === 'failed' ? 'stderr' : 'stdout', text: task.status === 'failed' ? 'EACCES: permission denied\n' : 'Updated successfully / 更新完成\n<script>unsafe</script>\n' })
        task.tools.forEach(item => {
          const agent = state.agents.find(a => a.name === item.name)
          item.phase = 'finished'
          item.result = { name: item.name, status: task.status === 'failed' ? 'failed' : 'success', outcome: task.status === 'failed' ? 'failed' : 'updated', old_version: agent.version, new_version: task.status === 'failed' ? '' : agent.latest_version, duration_ms: 1200, command: agent.command, ...(task.status === 'failed' ? { message: 'EACCES: permission denied' } : {}) }
          if (task.status !== 'failed') { agent.version = agent.latest_version; agent.upgrade_available = false }
        })
        state.active_task = ''
        broadcast(session)
      }, 1200)
      return json(res, 202, { id })
    }
    if (url.pathname === '/api/desktop/upgrades') {
      if (state.offline) return json(res, 400, { error: 'offline_read_only' })
      if (state.active_task) return json(res, 409, { error: 'busy' })
      if (body.revision !== state.revision) return json(res, 409, { error: 'stale_preview' })
      if (!body.plans?.length || body.command || body.url) return json(res, 400, { error: 'invalid_request' })
      const installations = body.plans.map(id => state.desktop.installations.find(info => info.plan?.id === id))
      if (installations.some(info => !info)) return json(res, 409, { error: 'stale_preview' })
      if (installations.length > 1 && installations.some(info => info.capability !== 'automatic' || info.plan.standalone)) return json(res, 400, { error: 'unsupported_upgrade' })
      if (installations.some(info => info.capability === 'privileged')) return json(res, 400, { error: 'unsupported_upgrade' })
      const id = `${Date.now()}-desktop-test`
      const task = {
        id, status: 'running', started_at: new Date().toISOString(),
        items: installations.map(info => ({ installation_id: info.id, name: info.name, phase: 'upgrading' })),
        logs: [], log_truncated: false,
      }
      state.desktop_tasks.push(task)
      state.active_task = id
      broadcast(session)
      setTimeout(() => {
        task.status = installations.some(info => info.plan.action === 'guide') ? 'handed_off' : 'completed'
        task.finished_at = new Date().toISOString()
        task.items.forEach((item, index) => {
          const info = installations[index]
          const guide = info.plan.action === 'guide'
          item.phase = 'finished'
          item.result = {
            installation_id: info.id, product_id: info.product_id, name: info.name,
            outcome: guide ? 'handed_off' : 'updated',
            old_version: info.version, new_version: guide ? '' : info.latest_version,
            duration_ms: 1200,
            ...(guide ? { url: info.plan.url } : { command: info.plan.command }),
          }
          if (!guide) {
            task.logs.push({ installation_id: info.id, stream: 'stdout', text: 'Synthetic Brew output\n<script>unsafe</script>\n' })
            info.version = info.latest_version
            info.upgrade_available = false
            delete info.plan
          }
        })
        state.active_task = ''
        broadcast(session)
      }, 1200)
      return json(res, 202, { id })
    }
    json(res, 404, { error: 'invalid_request' })
  } catch (err) {
    if (!res.headersSent) json(res, 500, { error: String(err) })
    else res.end()
  }
}
createServer(handle).listen(42731, '127.0.0.1')
createServer(handle).listen(42732, '127.0.0.1')

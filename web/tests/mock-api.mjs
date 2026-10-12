import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, sep, extname } from 'node:path'
import { fileURLToPath } from 'node:url'

// Only synthetic fixtures live here. This server cannot run external commands.
const assets = fileURLToPath(new URL('../../internal/webui/assets/', import.meta.url))
const sessions = new Map()
const labels = ['codex', 'claude-code', 'opencode', 'agy']
function newSession() {
  const state = {
    version: '0.5.0-dev', os: 'Windows', arch: 'amd64', offline: false,
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
  return { state, streams: new Set() }
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
      if (!sessions.has(body.token)) sessions.set(body.token, newSession())
      res.setHeader('Set-Cookie', `fixture=${encodeURIComponent(body.token)}; Path=/api/; HttpOnly; SameSite=Strict`)
      return json(res, 200, { ok: true })
    }
    const key = decodeURIComponent((req.headers.cookie || '').match(/fixture=([^;]+)/)?.[1] || '')
    const session = sessions.get(key)
    if (!session) return json(res, 401, { error: 'unauthorized' })
    const { state } = session
    if (url.pathname === '/api/state') return json(res, 200, state)
    if (url.pathname === '/api/events') {
      res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store' })
      session.streams.add(res)
      res.write(`event: state\ndata: ${JSON.stringify(state)}\n\n`)
      req.on('close', () => session.streams.delete(res))
      return
    }
    if (url.pathname === '/api/refresh') {
      state.offline = !!body.offline
      state.agents.forEach(a => a.latest_check = state.offline ? 'offline' : a.name === 'agy' ? 'unavailable' : 'checked')
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
    json(res, 404, { error: 'invalid_request' })
  } catch (err) {
    if (!res.headersSent) json(res, 500, { error: String(err) })
    else res.end()
  }
}
createServer(handle).listen(42731, '127.0.0.1')
createServer(handle).listen(42732, '127.0.0.1')

import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import {
  Activity, ArrowRight, ArrowUp, ArrowUpRight, Check, CheckCircle2, ChevronDown, ChevronUp,
  CircleAlert, Clock3, Copy, Globe2, ListChecks, LoaderCircle, Monitor, Moon, RefreshCw,
  Power, ShieldCheck, Sun, Terminal, Unplug, X,
} from 'lucide-react'
import { APIError, request, useSession } from './api'
import { dictionaries, initialLanguage } from './i18n'
import type { Dictionary, Language } from './i18n'
import { preference, remember } from './preferences'
import { toolLabels } from './types'
import type { Agent, Doctor, State, Task, ToolName } from './types'

type Page = 'agents' | 'tasks' | 'doctor'
type Tone = 'good' | 'warning' | 'bad' | 'muted' | 'accent'

function Badge({ tone = 'muted', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`badge ${tone}`}><span className="status-dot" />{children}</span>
}

function IconButton({ label, children, onClick, disabled = false }: {
  label: string; children: ReactNode; onClick: () => void; disabled?: boolean
}) {
  return <button type="button" className="icon-button" title={label} aria-label={label} disabled={disabled} onClick={onClick}>{children}</button>
}

function agentStatus(agent: Agent, t: Dictionary): [string, Tone] {
  if (agent.latest_check === 'pending') return [t.pending, 'accent']
  if (!agent.installed) return [t.notInstalled, 'muted']
  if (agent.latest_check === 'failed') return [t.checkFailed, 'bad']
  if (agent.upgrade_available) return [t.updateAvailable, 'warning']
  if (agent.latest_check === 'checked') return [t.upToDate, 'good']
  if (agent.latest_check === 'offline') return [t.notChecked, 'muted']
  return [t.unknownLatest, 'muted']
}

function formatCommand(command?: string[]) {
  return (command || []).map(arg => /[\s"'`]/.test(arg) ? JSON.stringify(arg) : arg).join(' ')
}
function displayVersion(value: string, t: Dictionary) {
  return value === 'unknown' ? t.unknown : value || '—'
}
function source(agent: Agent, t: Dictionary) {
  if (!agent.installed) return '—'
  return ({ 'self-update': t.selfUpdate, binary: t.binary, unknown: t.unknown } as Record<string, string>)[agent.install_method] || agent.install_method
}

function CopyButton({ value, t, onError }: { value: string; t: Dictionary; onError: (code: string) => void }) {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), 1800)
    return () => clearTimeout(timer)
  }, [copied])
  return <IconButton label={copied ? t.copied : t.copy} disabled={!value} onClick={() => {
    if (!navigator.clipboard) { onError('copyFailed'); return }
    navigator.clipboard.writeText(value).then(() => setCopied(true)).catch(() => onError('copyFailed'))
  }}>{copied ? <Check /> : <Copy />}</IconButton>
}

export function App() {
  const { state, connection, sessionError, reconnect, markStopped } = useSession()
  const [language, setLanguage] = useState<Language>(initialLanguage)
  const [theme, setTheme] = useState(() => preference('theme') === 'light' ? 'light' : 'dark')
  const [page, setPage] = useState<Page>('agents')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [selection, setSelection] = useState<ToolName[]>([])
  const [confirmation, setConfirmation] = useState<ToolName[] | null>(null)
  const [stopConfirmation, setStopConfirmation] = useState(false)
  const t = dictionaries[language]
  const busy = !state || state.scanning || state.diagnosing || !!state.active_task || submitting || connection !== 'connected'
  useEffect(() => {
    remember('language', language)
    document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en'
  }, [language])
  useEffect(() => {
    remember('theme', theme)
    document.documentElement.dataset.theme = theme
  }, [theme])
  useEffect(() => {
    if (state?.scanning) setSelection([])
  }, [state?.scanning])
  const errorText = (code: string) => t[code as keyof Dictionary] || t.request_failed

  async function act(path: string, body: unknown, done?: () => void) {
    setSubmitting(true)
    setError('')
    try {
      await request(path, body)
      done?.()
    } catch (err) {
      setError(err instanceof APIError ? err.message : 'request_failed')
    } finally { setSubmitting(false) }
  }
  const confirmAgents = state?.agents.filter(agent => confirmation?.includes(agent.name)) || []

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-symbol"><ArrowUpRight size={22} /></span><span>AgentUp<span className="brand-sub">{t.localMode} / {state?.version || '—'}</span></span></div>
      <nav aria-label="AgentUp">
        {([
          ['agents', Terminal], ['tasks', ListChecks], ['doctor', Activity],
        ] as const).map(([key, Icon]) => <button type="button" key={key} className={page === key ? 'nav-item active' : 'nav-item'} aria-current={page === key ? 'page' : undefined} onClick={() => setPage(key)}>
          <Icon size={18} /><span>{t[key]}</span>{key === 'tasks' && !!state?.active_task && <span className="live-dot" />}
        </button>)}
      </nav>
      <div className="sidebar-bottom"><ShieldCheck size={16} /><span>{t.local}<small>127.0.0.1</small></span></div>
    </aside>
    <div className="workspace">
      <header className="topbar">
        <div className="environment"><Monitor size={15} /><span>{state?.os || t.environment}<span className="divider">/</span><span className="mono">{state?.arch || '—'}</span></span></div>
        <div className="topbar-actions">
          <span className={`connection ${connection === 'connected' ? 'good' : connection === 'stopped' ? 'muted' : 'warning'}`} role="status"><span className="status-dot" /><span>{t[connection]}</span></span>
          <label className="language-select"><Globe2 size={16} /><select aria-label={t.language} value={language} onChange={e => setLanguage(e.target.value as Language)}><option value="zh">中文</option><option value="en">English</option></select></label>
          <IconButton label={`${t.theme} · ${theme === 'dark' ? t.light : t.dark}`} onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>{theme === 'dark' ? <Sun /> : <Moon />}</IconButton>
          <IconButton label={state?.active_task ? t.stopBlocked : t.stopService} disabled={connection !== 'connected' || !!state?.active_task || submitting} onClick={() => { setError(''); setStopConfirmation(true) }}><Power /></IconButton>
        </div>
      </header>
      <main>
        {connection !== 'stopped' && (error || sessionError || connection === 'reconnecting') && <div className="alert" role="alert">
          <CircleAlert size={18} /><span>{errorText(error || sessionError || 'network_error')}</span>
          {connection !== 'connected' && <button type="button" className="text-button" onClick={reconnect}>{t.retry}</button>}
          {error && <IconButton label={t.dismiss} onClick={() => setError('')}><X /></IconButton>}
        </div>}
        {connection === 'stopped' ? <div className="empty-state" role="status"><Power size={32} /><h1>{t.stopped}</h1><p>{t.stoppedNote}</p></div> : !state ? <div className="empty-state">{connection === 'connecting' ? <LoaderCircle className="spin" size={30} /> : <Unplug size={32} />}<h1>{connection === 'connecting' ? t.connecting : t.secureSession}</h1>{connection !== 'connecting' && <p>{t.sessionHint}</p>}</div> : <>
          <div className="page-heading"><div><span className="eyebrow">{t.workspace} / {page === 'agents' ? '01' : page === 'tasks' ? '02' : '03'}</span><h1>{t[page]}</h1></div><div className="heading-actions">
            {page === 'agents' && <><label className="offline-toggle"><input type="checkbox" role="switch" checked={state.offline} disabled={busy} onChange={e => void act('refresh', { offline: e.target.checked })} /><span className="switch-track" /><span>{t.offline}</span></label><button className="button secondary" disabled={busy} onClick={() => void act('refresh', { offline: state.offline })}><RefreshCw className={state.scanning ? 'spin' : ''} size={16} /><span>{state.scanning ? t.scanning : t.refresh}</span></button></>}
            {page === 'doctor' && <button className="button primary" disabled={busy} onClick={() => void act('doctor', {})}><Activity className={state.diagnosing ? 'pulse' : ''} size={16} />{state.diagnosing ? t.diagnosing : t.diagnose}</button>}
          </div></div>
          {page === 'agents' && <AgentView state={state} t={t} busy={busy} selection={selection} setSelection={setSelection} confirm={setConfirmation} onError={setError} />}
          {page === 'tasks' && <TaskView tasks={state.tasks} t={t} language={language} onError={setError} />}
          {page === 'doctor' && <DoctorView doctor={state.doctor} diagnosing={state.diagnosing} t={t} onError={setError} />}
          <footer><span>AgentUp <span className="mono">{state.version}</span></span><span><ShieldCheck size={13} />{state.offline ? t.readOnly : t.local}</span></footer>
        </>}
      </main>
    </div>
    {connection !== 'stopped' && confirmation && <Confirmation agents={confirmAgents} t={t} submitting={submitting} disabled={busy || !!state?.offline || confirmAgents.length !== confirmation.length} onCancel={() => setConfirmation(null)} onConfirm={() => void act('upgrades', { tools: confirmation, revision: state?.revision }, () => { setConfirmation(null); setSelection([]); setPage('tasks') })} />}
    {connection !== 'stopped' && stopConfirmation && <StopConfirmation t={t} submitting={submitting} disabled={!!state?.active_task || connection !== 'connected' || submitting} onCancel={() => setStopConfirmation(false)} onConfirm={() => void act('shutdown', {}, () => { setStopConfirmation(false); markStopped() })} />}
  </div>
}

function AgentView({ state, t, busy, selection, setSelection, confirm, onError }: {
  state: State; t: Dictionary; busy: boolean; selection: ToolName[]
  setSelection: (value: ToolName[]) => void; confirm: (names: ToolName[]) => void; onError: (code: string) => void
}) {
  const [filter, setFilter] = useState<'all' | 'updates' | 'installed'>('all')
  const [expanded, setExpanded] = useState<ToolName[]>([])
  const eligible = state.agents.filter(agent => agent.installed && agent.upgrade_supported && !!agent.command?.length)
  const available = eligible.filter(agent => agent.upgrade_available)
  const upgradeDisabled = busy || state.offline
  const visible = state.agents.filter(agent => filter === 'all' || (filter === 'updates' ? agent.upgrade_available : agent.installed))
  const select = (name: ToolName) => setSelection(selection.includes(name) ? selection.filter(item => item !== name) : [...selection, name])
  return <>
    <section className="stats-band" aria-label={t.total}>
      {([
        [state.agents.filter(a => a.installed).length, t.installed, ''],
        [state.agents.filter(a => a.upgrade_available).length, t.updates, 'warning'],
        [state.agents.filter(a => a.installed && (a.latest_check === 'failed' || a.latest_check === 'unavailable' || !a.upgrade_supported)).length, t.attention, ''],
        [state.agents.length, t.supported, 'muted'],
      ] as const).map(([count, label, tone]) => <div className={`stat ${tone}`} key={label}><strong>{String(count).padStart(2, '0')}</strong><span>{label}</span></div>)}
    </section>
    <div className="list-toolbar">
      <div className="segments" role="group" aria-label={t.filters}>{(['all', 'updates', 'installed'] as const).map(key => <button key={key} type="button" aria-pressed={filter === key} className={filter === key ? 'selected' : ''} onClick={() => setFilter(key)}>{key === 'updates' ? t.updateFilter : t[key]}</button>)}</div>
      <button type="button" className="text-button" disabled={upgradeDisabled || !available.length} onClick={() => setSelection(available.map(a => a.name))}><ListChecks size={15} />{t.selectUpdates}</button>
    </div>
    <div className="agent-table" role="table" aria-label={t.agents}>
      <div className="agent-row table-head" role="row">
        <div role="columnheader"><input type="checkbox" aria-label={t.selectAll} checked={!!eligible.length && eligible.every(agent => selection.includes(agent.name))} disabled={upgradeDisabled || !eligible.length} onChange={e => setSelection(e.target.checked ? eligible.map(a => a.name) : [])} /></div>
        <span role="columnheader">{t.agent}</span><span role="columnheader">{t.version}</span><span role="columnheader">{t.source}</span><span role="columnheader">{t.status}</span><span role="columnheader" />
      </div>
      {visible.map(agent => {
        const [status, tone] = agentStatus(agent, t)
        const open = expanded.includes(agent.name)
        const canUpgrade = eligible.some(a => a.name === agent.name)
        return <div className={`agent-entry ${!agent.installed ? 'not-installed' : ''}`} key={agent.name}>
          <div className="agent-row" role="row">
            <div className="row-select" role="cell"><input type="checkbox" aria-label={`${t.select} ${toolLabels[agent.name]}`} checked={selection.includes(agent.name)} disabled={upgradeDisabled || !canUpgrade} onChange={() => select(agent.name)} /></div>
            <div className="agent-name" role="cell"><span className={`agent-mark mark-${agent.name}`}><Terminal size={19} /></span><span><strong>{toolLabels[agent.name]}</strong><small className="mono">{agent.name}</small></span></div>
            <div className="versions mono" role="cell"><span>{displayVersion(agent.version, t)}</span>{agent.upgrade_available && <><ArrowRight size={13} /><span className="warning">{agent.latest_version}</span></>}</div>
            <div className="install-source" role="cell">{source(agent, t)}</div>
            <div className="row-status" role="cell"><Badge tone={tone}>{status}</Badge></div>
            <div className="row-actions" role="cell"><IconButton label={`${t.upgrade} ${toolLabels[agent.name]}`} disabled={upgradeDisabled || !canUpgrade} onClick={() => confirm([agent.name])}><ArrowUp /></IconButton><button type="button" className="icon-button" aria-expanded={open} aria-label={`${open ? t.collapse : t.details} ${toolLabels[agent.name]}`} title={open ? t.collapse : t.details} onClick={() => setExpanded(open ? expanded.filter(n => n !== agent.name) : [...expanded, agent.name])}>{open ? <ChevronUp /> : <ChevronDown />}</button></div>
          </div>
          {open && <div className="agent-details">
            <dl><dt>{t.path}</dt><dd><code>{agent.path || '—'}</code>{agent.path && <CopyButton value={agent.path} t={t} onError={onError} />}</dd>
            <dt>{t.latest}</dt><dd className="mono">{displayVersion(agent.latest_version, t)}</dd>
            <dt>{t.command}</dt><dd>{agent.command?.length ? <><code>{formatCommand(agent.command)}</code><CopyButton value={formatCommand(agent.command)} t={t} onError={onError} /></> : <span>{agent.installed ? t.noAutomaticUpgrade : t.notInstalled}</span>}</dd>
            {agent.check_message && <><dt>{t.rawDetails}</dt><dd><code>{agent.check_message}</code></dd></>}</dl>
          </div>}
        </div>
      })}
      {!visible.length && <div className="empty-inline"><CheckCircle2 size={24} /><span>{t.noMatch}</span></div>}
    </div>
    {selection.length > 0 && <div className="selection-bar"><span><span className="selection-count">{selection.length}</span> {t.selected}</span><div><button type="button" className="text-button" onClick={() => setSelection([])} disabled={busy}>{t.clear}</button><button type="button" className="button primary" disabled={upgradeDisabled} onClick={() => confirm(selection)}><ArrowUp size={16} />{t.upgradeSelected}</button></div></div>}
  </>
}

function Confirmation({ agents, t, submitting, disabled, onCancel, onConfirm }: {
  agents: Agent[]; t: Dictionary; submitting: boolean; disabled: boolean; onCancel: () => void; onConfirm: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => { dialog.current?.showModal() }, [])
  return <dialog ref={dialog} className="confirm-dialog" aria-labelledby="confirm-title" onCancel={e => { if (submitting) e.preventDefault(); else onCancel() }}>
    <div className="modal-heading"><span className="modal-icon"><ArrowUp size={21} /></span><h2 id="confirm-title">{t.confirmTitle}</h2><IconButton label={t.close} disabled={submitting} onClick={onCancel}><X /></IconButton></div>
    <p>{t.confirmText}</p>
    <div className="command-list">{agents.map(agent => <div key={agent.name}><strong>{toolLabels[agent.name]}</strong><span className="mono">{displayVersion(agent.version, t)} {agent.latest_version && <>→ {agent.latest_version}</>}</span><code>{formatCommand(agent.command)}</code></div>)}</div>
    <p className="modal-note"><ShieldCheck size={16} />{t.confirmNote}</p>
    <div className="modal-actions"><button type="button" className="button secondary" disabled={submitting} onClick={onCancel}>{t.cancel}</button><button type="button" className="button primary" disabled={disabled} onClick={onConfirm}>{submitting ? <LoaderCircle size={16} className="spin" /> : <ArrowUp size={16} />}{submitting ? t.submitting : t.confirm}</button></div>
  </dialog>
}

function StopConfirmation({ t, submitting, disabled, onCancel, onConfirm }: {
  t: Dictionary; submitting: boolean; disabled: boolean; onCancel: () => void; onConfirm: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => { dialog.current?.showModal() }, [])
  return <dialog ref={dialog} className="confirm-dialog" aria-labelledby="stop-title" onCancel={e => { if (submitting) e.preventDefault(); else onCancel() }}>
    <div className="modal-heading"><span className="modal-icon"><Power size={21} /></span><h2 id="stop-title">{t.stopService}</h2><IconButton label={t.close} disabled={submitting} onClick={onCancel}><X /></IconButton></div>
    <p>{t.stopText}</p>
    <div className="modal-actions"><button type="button" className="button secondary" disabled={submitting} onClick={onCancel}>{t.cancel}</button><button type="button" className="button danger" disabled={disabled} onClick={onConfirm}>{submitting ? <LoaderCircle size={16} className="spin" /> : <Power size={16} />}{submitting ? t.submitting : t.confirmStop}</button></div>
  </dialog>
}

function TaskView({ tasks, t, language, onError }: { tasks: Task[]; t: Dictionary; language: Language; onError: (code: string) => void }) {
  const [selected, setSelected] = useState('')
  const [follow, setFollow] = useState(true)
  const logRef = useRef<HTMLPreElement>(null)
  const task = tasks.find(task => task.id === selected) || tasks.at(-1)
  const log = task?.logs.map(line => `[${line.name}${line.stream === 'stderr' ? ' / stderr' : ''}] ${line.text}`).join('') || ''
  useEffect(() => {
    if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight
  }, [log, follow])
  const date = (value: string) => new Date(value).toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  if (!task) return <div className="empty-state"><ListChecks size={34} /><h2>{t.noTasks}</h2></div>
  return <div className="task-layout">
    <section className="task-history"><h2>{t.sessionHistory}<span>{tasks.length}</span></h2>{[...tasks].reverse().map(item => <button type="button" key={item.id} aria-pressed={item.id === task.id} className={`history-item ${item.id === task.id ? 'active' : ''}`} onClick={() => setSelected(item.id)}><span>{item.tools.map(a => toolLabels[a.name]).join(', ')}</span><small>{date(item.started_at)}</small><Badge tone={item.status === 'completed' ? 'good' : item.status === 'failed' ? 'bad' : 'accent'}>{t[item.status]}</Badge></button>)}</section>
    <section className="task-detail">
      <div className="task-heading"><h2>{t.taskResult}</h2><Badge tone={task.status === 'failed' ? 'bad' : task.status === 'running' ? 'accent' : 'good'}>{t[task.status]}</Badge></div>
      <div className="task-time"><Clock3 size={14} />{date(task.started_at)}<span className="mono">#{task.id.slice(-7)}</span></div>
      <div className="task-tools">{task.tools.map(item => <div className="task-tool" key={item.name}>
        <div><strong>{toolLabels[item.name]}</strong>{item.result && <span className="mono">{displayVersion(item.result.old_version, t)} <ArrowRight size={13} />{displayVersion(item.result.new_version, t)}</span>}</div>
        <Badge tone={!item.result ? 'accent' : item.result.outcome === 'failed' || item.result.outcome === 'unverified' ? 'bad' : item.result.outcome === 'updated' ? 'good' : 'muted'}>{item.result ? t[item.result.outcome] : t[item.phase]}</Badge>
        {item.result && <small className="mono">{(item.result.duration_ms / 1000).toFixed(1)}s</small>}
        {item.result && item.result.outcome !== 'updated' && <p className="result-note">{({ unchanged: t.unchangedNote, unverified: t.unverifiedNote, failed: t.failedNote, skipped: t.skippedNote } as Record<string, string>)[item.result.outcome]}</p>}
        {item.result?.message && <details className="raw-message"><summary>{t.rawDetails}</summary><pre>{item.result.message}</pre></details>}
      </div>)}</div>
      <div className="log-toolbar"><h3><Terminal size={16} />{t.output}</h3><div><label><input type="checkbox" checked={follow} onChange={e => setFollow(e.target.checked)} />{t.followLogs}</label><CopyButton value={log} t={t} onError={onError} /></div></div>
      <pre ref={logRef} className="log-output" tabIndex={0} aria-label={t.output}>{log || t.noLogs}</pre>
      {task.log_truncated && <p className="warning small-text">{t.truncated}</p>}
    </section>
  </div>
}

function DoctorView({ doctor, diagnosing, t, onError }: { doctor: Doctor | null; diagnosing: boolean; t: Dictionary; onError: (code: string) => void }) {
  if (!doctor) return <div className="empty-state">{diagnosing ? <LoaderCircle className="spin" size={32} /> : <Activity size={34} />}<h2>{diagnosing ? t.diagnosing : t.noDiagnosis}</h2></div>
  const report = `${t.environment}: ${doctor.os}\n\n${t.managers}\n${doctor.managers.map(m => `${m.name}: ${m.found ? t.available : m.error ? t.managerError : t.missing} ${m.version ? displayVersion(m.version, t) : ''}\n${m.path || ''}${m.error ? `\n${m.error}` : ''}`).join('\n')}\n\n${t.tools}\n${doctor.tools.map(a => `${toolLabels[a.name]}: ${a.found ? t.available : t.missing}\n${a.path || ''}${a.issue_code ? `\n${t[a.issue_code as keyof Dictionary] || t.request_failed}` : ''}`).join('\n')}`
  return <>
    <div className="diagnosis-overview"><div><ShieldCheck size={20} /><span>{doctor.os}<span className="divider">/</span>{doctor.managers.filter(m => m.found).length} {t.managers}<span className="divider">/</span>{doctor.tools.filter(a => a.found).length} {t.installed}</span></div><CopyButton value={report} t={t} onError={onError} /></div>
    <section className="diagnostic-section"><h2>{t.managers}</h2><div className="diagnostic-list">{doctor.managers.map(manager => <div className="diagnostic-row" key={manager.name}><div className="diagnostic-name"><Terminal size={17} /><strong>{manager.name}</strong></div><div className="diagnostic-info"><span className="mono">{manager.version ? displayVersion(manager.version, t) : manager.error ? t.managerError : t.optional}</span>{manager.path && <code>{manager.path}</code>}{manager.error && <details className="raw-message"><summary>{t.rawDetails}</summary><pre>{manager.error}</pre></details>}</div><Badge tone={manager.found ? 'good' : manager.error ? 'bad' : 'muted'}>{manager.found ? t.available : manager.error ? t.checkFailed : t.missing}</Badge></div>)}</div></section>
    <section className="diagnostic-section"><h2>{t.tools}</h2><div className="diagnostic-list">{doctor.tools.map(agent => <div className="diagnostic-row" key={agent.name}><div className="diagnostic-name"><Terminal size={17} /><strong>{toolLabels[agent.name]}</strong></div><div className="diagnostic-info"><code>{agent.path || '—'}</code>{agent.issue_code && <span className="warning">{t[agent.issue_code as keyof Dictionary] || t.request_failed}</span>}</div><Badge tone={agent.issue ? 'warning' : agent.found ? 'good' : 'muted'}>{agent.issue ? t.attention : agent.found ? t.available : t.notInstalled}</Badge></div>)}</div></section>
  </>
}

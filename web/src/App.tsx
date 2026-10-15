import { useEffect, useRef, useState } from 'react'
import {
  Activity, ArrowUpRight, CircleAlert, Globe2, LayoutGrid, ListChecks, LoaderCircle, Monitor, Moon, RefreshCw,
  Power, ShieldCheck, Sun, Terminal, Unplug, X,
} from 'lucide-react'
import { APIError, request, useSession } from './api'
import { dictionaries, initialLanguage } from './i18n'
import type { Dictionary, Language } from './i18n'
import { Overview } from './Overview'
import type { KindFilter } from './Overview'
import { preference, remember } from './preferences'
import { TaskCenter } from './Tasks'
import { toolLabels } from './types'
import type { Doctor } from './types'
import { Badge, CopyButton, IconButton, displayVersion } from './ui'

type Page = 'agents' | 'tasks' | 'doctor'

export function App() {
  const { state, connection, sessionError, reconnect, markStopped } = useSession()
  const [language, setLanguage] = useState<Language>(initialLanguage)
  const [theme, setTheme] = useState(() => preference('theme') === 'light' ? 'light' : 'dark')
  const [page, setPage] = useState<Page>('agents')
  const [kind, setKind] = useState<KindFilter>(() => {
    const stored = preference('kind')
    return stored === 'cli' || stored === 'desktop' ? stored : 'all'
  })
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
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
  useEffect(() => { remember('kind', kind) }, [kind])
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

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-symbol"><ArrowUpRight size={22} /></span><span>AgentUp<span className="brand-sub">{t.localMode} / {state?.version || '—'}</span></span></div>
      <nav aria-label="AgentUp">
        {([
          ['agents', LayoutGrid], ['tasks', ListChecks], ['doctor', Activity],
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
          <div className="page-heading"><div><span className="eyebrow">{t.workspace} / {page === 'agents' ? '01' : page === 'tasks' ? '02' : '03'}</span><h1>{t[page]}</h1>{page === 'agents' && <p className="page-intro">{t.agentsIntro}</p>}</div><div className="heading-actions">
            {page === 'agents' && <><label className="offline-toggle"><input type="checkbox" role="switch" checked={state.offline} disabled={busy} onChange={e => void act('refresh', { offline: e.target.checked })} /><span className="switch-track" /><span>{t.offline}</span></label><button className="button secondary" disabled={busy} onClick={() => void act('refresh', { offline: state.offline })}><RefreshCw className={state.scanning ? 'spin' : ''} size={16} /><span>{state.scanning ? t.scanning : t.refresh}</span></button></>}
            {page === 'doctor' && <button className="button primary" disabled={busy} onClick={() => void act('doctor', {})}><Activity className={state.diagnosing ? 'pulse' : ''} size={16} />{state.diagnosing ? t.diagnosing : t.diagnose}</button>}
          </div></div>
          {page === 'agents' && <Overview state={state} t={t} busy={busy} submitting={submitting} kind={kind} setKind={setKind} onError={setError}
            onUpgradeCli={tools => void act('upgrades', { tools, revision: state.revision }, () => setPage('tasks'))}
            onRunDesktop={(plans, revision) => void act('desktop/upgrades', { plans, revision }, () => setPage('tasks'))} />}
          {page === 'tasks' && <TaskCenter tasks={state.tasks} desktopTasks={state.desktop_tasks || []} t={t} language={language} onError={setError} />}
          {page === 'doctor' && <DoctorView doctor={state.doctor} diagnosing={state.diagnosing} t={t} onError={setError} />}
          <footer><span>AgentUp <span className="mono">{state.version}</span></span><span><ShieldCheck size={13} />{state.offline ? t.readOnly : t.local}</span></footer>
        </>}
      </main>
    </div>
    {connection !== 'stopped' && stopConfirmation && <StopConfirmation t={t} submitting={submitting} disabled={!!state?.active_task || connection !== 'connected' || submitting} onCancel={() => setStopConfirmation(false)} onConfirm={() => void act('shutdown', {}, () => { setStopConfirmation(false); markStopped() })} />}
  </div>
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

function DoctorView({ doctor, diagnosing, t, onError }: { doctor: Doctor | null; diagnosing: boolean; t: Dictionary; onError: (code: string) => void }) {
  if (!doctor) return <div className="empty-state">{diagnosing ? <LoaderCircle className="spin" size={32} /> : <Activity size={34} />}<h2>{diagnosing ? t.diagnosing : t.noDiagnosis}</h2></div>
  const report = `${t.environment}: ${doctor.os}\n\n${t.managers}\n${doctor.managers.map(m => `${m.name}: ${m.found ? t.available : m.error ? t.managerError : t.missing} ${m.version ? displayVersion(m.version, t) : ''}\n${m.path || ''}${m.error ? `\n${m.error}` : ''}`).join('\n')}\n\n${t.tools}\n${doctor.tools.map(a => `${toolLabels[a.name]}: ${a.found ? t.available : t.missing}\n${a.path || ''}${a.issue_code ? `\n${t[a.issue_code as keyof Dictionary] || t.request_failed}` : ''}`).join('\n')}`
  return <>
    <div className="diagnosis-overview"><div><ShieldCheck size={20} /><span>{doctor.os}<span className="divider">/</span>{doctor.managers.filter(m => m.found).length} {t.managers}<span className="divider">/</span>{doctor.tools.filter(a => a.found).length} {t.installed}</span></div><CopyButton value={report} t={t} onError={onError} /></div>
    <section className="diagnostic-section"><h2>{t.managers}</h2><div className="diagnostic-list">{doctor.managers.map(manager => <div className="diagnostic-row" key={manager.name}><div className="diagnostic-name"><Terminal size={17} /><strong>{manager.name}</strong></div><div className="diagnostic-info"><span className="mono">{manager.version ? displayVersion(manager.version, t) : manager.error ? t.managerError : t.optional}</span>{manager.path && <code>{manager.path}</code>}{manager.error && <details className="raw-message"><summary>{t.rawDetails}</summary><pre>{manager.error}</pre></details>}</div><Badge tone={manager.found ? 'good' : manager.error ? 'bad' : 'muted'}>{manager.found ? t.available : manager.error ? t.checkFailed : t.missing}</Badge></div>)}</div></section>
    <section className="diagnostic-section"><h2>{t.tools}</h2><div className="diagnostic-list">{doctor.tools.map(agent => <div className="diagnostic-row" key={agent.name}><div className="diagnostic-name"><Terminal size={17} /><strong>{toolLabels[agent.name]}</strong></div><div className="diagnostic-info"><code>{agent.path || '—'}</code>{agent.issue_code && <span className="warning">{t[agent.issue_code as keyof Dictionary] || t.request_failed}</span>}</div><Badge tone={agent.issue ? 'warning' : agent.found ? 'good' : 'muted'}>{agent.issue ? t.attention : agent.found ? t.available : t.notInstalled}</Badge></div>)}</div></section>
  </>
}

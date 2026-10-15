import { useEffect, useRef, useState } from 'react'
import { ArrowRight, Clock3, ExternalLink, ListChecks, Terminal } from 'lucide-react'
import type { Dictionary, Language } from './i18n'
import { toolLabels } from './types'
import type { DesktopTask, Task } from './types'
import { Badge, CopyButton, KindChip, displayVersion, label, taskDate } from './ui'
import type { Kind, Tone } from './ui'

type Entry = { kind: 'cli'; id: string; started: string; task: Task } | { kind: 'desktop'; id: string; started: string; task: DesktopTask }

const statusTone: Record<string, Tone> = { completed: 'good', failed: 'bad', handed_off: 'warning', running: 'accent', queued: 'muted' }
const outcomeTone = (outcome?: string): Tone => !outcome ? 'accent'
  : outcome === 'failed' || outcome === 'unverified' ? 'bad'
    : outcome === 'updated' || outcome === 'no_update' ? 'good' : outcome === 'handed_off' ? 'warning' : 'muted'

function safeURL(value?: string) {
  try {
    const url = new URL(value || '')
    return url.protocol === 'https:' && ['chatgpt.com', 'claude.com', 'opencode.ai', 'antigravity.google'].includes(url.hostname) ? url.href : ''
  } catch { return '' }
}

// CLI and desktop tasks share one history so the newest work is always on top.
export function TaskCenter({ tasks, desktopTasks, t, language, onError }: {
  tasks: Task[]; desktopTasks: DesktopTask[]; t: Dictionary; language: Language; onError: (code: string) => void
}) {
  const [selected, setSelected] = useState('')
  const entries: Entry[] = [
    ...tasks.map(task => ({ kind: 'cli' as const, id: task.id, started: task.started_at, task })),
    ...desktopTasks.map(task => ({ kind: 'desktop' as const, id: task.id, started: task.started_at, task })),
  ].sort((a, b) => Date.parse(b.started) - Date.parse(a.started))
  const entry = entries.find(item => item.id === selected) || entries[0]
  if (!entry) return <div className="empty-state panel"><ListChecks size={32} /><h2>{t.noTasks}</h2><p>{t.noTasksHint}</p></div>
  const names = (item: Entry) => item.kind === 'cli' ? item.task.tools.map(tool => toolLabels[tool.name]).join(', ') : item.task.items.map(app => app.name).join(', ')
  return <div className="task-layout panel">
    <section className="task-history"><h2>{t.sessionHistory}<span>{entries.length}</span></h2>{entries.map(item => <button type="button" key={item.id} aria-pressed={item.id === entry.id} className={`history-item ${item.id === entry.id ? 'active' : ''}`} onClick={() => setSelected(item.id)}>
      <span className="history-title"><KindChip kind={item.kind} /><span>{names(item)}</span></span>
      <small>{taskDate(item.started, language)}</small>
      <Badge tone={statusTone[item.task.status] || 'muted'}>{label(t, item.task.status)}</Badge>
    </button>)}</section>
    <section className="task-detail">
      <TaskHeading kind={entry.kind} status={entry.task.status} started={entry.started} id={entry.id} t={t} language={language} />
      {entry.kind === 'cli' ? <>
        <div className="task-tools">{entry.task.tools.map(item => <div className="task-tool" key={item.name}>
          <div><strong>{toolLabels[item.name]} CLI</strong>{item.result && <span className="mono">{displayVersion(item.result.old_version, t)} <ArrowRight size={13} />{displayVersion(item.result.new_version, t)}</span>}</div>
          <Badge tone={outcomeTone(item.result?.outcome)}>{item.result ? t[item.result.outcome] : t[item.phase]}</Badge>
          {item.result && <small className="mono">{(item.result.duration_ms / 1000).toFixed(1)}s</small>}
          {item.result && item.result.outcome !== 'updated' && <p className="result-note">{({ unchanged: t.unchangedNote, unverified: t.unverifiedNote, failed: t.failedNote, skipped: t.skippedNote } as Record<string, string>)[item.result.outcome]}</p>}
          {item.result?.message && <details className="raw-message"><summary>{t.rawDetails}</summary><pre>{item.result.message}</pre></details>}
        </div>)}</div>
        <LogPanel log={entry.task.logs.map(line => `[${line.name}${line.stream === 'stderr' ? ' / stderr' : ''}] ${line.text}`).join('')} truncated={entry.task.log_truncated} t={t} onError={onError} />
      </> : <>
        <div className="task-tools">{entry.task.items.map(item => <div className="task-tool" key={item.installation_id}>
          <div><strong>{item.name}</strong>{item.result && <span className="mono">{item.result.old_version || '—'}<ArrowRight size={12} />{item.result.new_version || '—'}</span>}</div>
          <Badge tone={outcomeTone(item.result?.outcome)}>{label(t, item.result?.outcome || item.phase)}</Badge>
          {item.result && <small className="mono">{(item.result.duration_ms / 1000).toFixed(1)}s</small>}
          {item.result?.outcome === 'handed_off' && <p className="result-note">{t.desktopGuideNote}</p>}
          {item.result && ['unchanged', 'unverified', 'failed', 'skipped', 'no_update', 'cancelled'].includes(item.result.outcome) && <p className="result-note">{({ unchanged: t.unchangedNote, unverified: t.unverifiedNote, failed: t.failedNote, skipped: t.skippedNote, no_update: t.noUpdateNote, cancelled: t.cancelledNote } as Record<string, string>)[item.result.outcome]}</p>}
          {safeURL(item.result?.url) && <a className="desktop-link" href={safeURL(item.result?.url)} target="_blank" rel="noreferrer noopener"><ExternalLink size={14} />{t.openUpdate}</a>}
          {item.result?.message && <details className="raw-message"><summary>{t.rawDetails}</summary><pre>{item.result.message}</pre></details>}
        </div>)}</div>
        <LogPanel log={entry.task.logs.map(line => `[${entry.task.items.find(item => item.installation_id === line.installation_id)?.name || line.installation_id}${line.stream === 'stderr' ? ' / stderr' : ''}] ${line.text}`).join('')} truncated={entry.task.log_truncated} t={t} onError={onError} />
      </>}
    </section>
  </div>
}

function TaskHeading({ kind, status, started, id, t, language }: { kind: Kind; status: string; started: string; id: string; t: Dictionary; language: Language }) {
  return <>
    <div className="task-heading"><h2><KindChip kind={kind} />{t.taskResult}</h2><Badge tone={statusTone[status] || 'muted'}>{label(t, status)}</Badge></div>
    <div className="task-time"><Clock3 size={14} />{taskDate(started, language)}<span className="mono">#{id.slice(-7)}</span></div>
  </>
}

function LogPanel({ log, truncated, t, onError }: { log: string; truncated: boolean; t: Dictionary; onError: (code: string) => void }) {
  const [follow, setFollow] = useState(true)
  const logRef = useRef<HTMLPreElement>(null)
  useEffect(() => {
    if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight
  }, [log, follow])
  return <>
    <div className="log-toolbar"><h3><Terminal size={16} />{t.output}</h3><div><label><input type="checkbox" checked={follow} onChange={e => setFollow(e.target.checked)} />{t.followLogs}</label><CopyButton value={log} t={t} onError={onError} /></div></div>
    <pre ref={logRef} className="log-output" tabIndex={0} aria-label={t.output}>{log || t.noLogs}</pre>
    {truncated && <p className="warning small-text">{t.truncated}</p>}
  </>
}

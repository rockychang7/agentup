import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ArrowRight, ArrowUp, ChevronDown, LoaderCircle, ShieldCheck, Terminal, X } from 'lucide-react'
import type { Dictionary } from './i18n'
import { toolLabels } from './types'
import type { Agent, Result, State, ToolName } from './types'
import { Badge, IconButton, CopyButton, displayVersion, formatCommand, label, sourceLabel } from './ui'
import type { Tone } from './ui'

type StatusFilter = 'all' | 'updates' | 'installed'

const families: ToolName[] = ['codex', 'claude-code', 'opencode', 'agy']
const binaries: Record<ToolName, string> = { codex: 'codex', 'claude-code': 'claude', opencode: 'opencode', agy: 'agy' }
function cliStatus(agent: Agent, t: Dictionary): [string, Tone] {
  if (agent.latest_check === 'pending') return [t.pending, 'accent']
  if (!agent.installed) return [t.notInstalled, 'muted']
  if (agent.latest_check === 'failed') return [t.checkFailed, 'bad']
  if (agent.upgrade_available) return [t.updateAvailable, 'warning']
  if (agent.latest_check === 'checked') return [t.upToDate, 'good']
  if (agent.latest_check === 'offline') return [t.notChecked, 'muted']
  return [t.unknownLatest, 'muted']
}

// A confirmed "nothing newer" result greys the action out; an unknown latest version does not.
const upToDate = (item: { upgrade_available: boolean; latest_check: string }) => item.latest_check === 'checked' && !item.upgrade_available
const cliUpgradeable = (agent: Agent) => agent.installed && agent.upgrade_supported && !!agent.command?.length
const cliActionable = (agent: Agent) => cliUpgradeable(agent) && agent.latest_check !== 'pending' && !upToDate(agent)
type CliRun = { result: Result; log: string }

// Rows show their own progress and their latest finished result; there is no separate task list.
function rowActivity(state: State) {
  const ids = new Set(state.active_tasks)
  const phases = { cli: new Map<string, string>() }
  const last = { cli: new Map<string, CliRun>() }
  const byTime = <T extends { started_at: string }>(list: T[]) => [...list].sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at))
  for (const task of byTime(state.tasks)) for (const tool of task.tools) {
    if (ids.has(task.id) && !tool.result) phases.cli.set(tool.name, tool.phase)
    else if (ids.has(task.id)) phases.cli.set(tool.name, 'refreshing')
    else if (tool.result) last.cli.set(tool.name, { result: tool.result, log: task.logs.filter(line => line.name === tool.name).map(line => line.text).join('') })
  }
  return { phases, last }
}

export function Overview({ state, t, locked, submitting, onError, onUpgradeCli }: {
  state: State; t: Dictionary; locked: boolean; submitting: boolean
  onError: (code: string) => void
  onUpgradeCli: (tools: ToolName[], done?: () => void) => void
}) {
  const [status, setStatus] = useState<StatusFilter>('all')
  const [expanded, setExpanded] = useState<string[]>([])
  const [cliConfirm, setCliConfirm] = useState<ToolName[] | null>(null)
  const { phases, last } = rowActivity(state)
  const cliUpdates = state.agents.filter(agent => cliActionable(agent) && agent.upgrade_available && !phases.cli.has(agent.name))
  const toggle = (id: string) => setExpanded(open => open.includes(id) ? open.filter(item => item !== id) : [...open, id])
  const matches = (installed: boolean, update: boolean) => status === 'all' || (status === 'updates' ? update : installed)
  const order = (family: ToolName) => families.indexOf(family)
  const agents = state.agents.filter(agent => matches(agent.installed, agent.upgrade_available)).sort((a, b) => order(a.name) - order(b.name))

  const cliAgents = state.agents.filter(agent => cliConfirm?.includes(agent.name))

  const stats = {
    installed: state.agents.filter(agent => agent.installed).length,
    updates: state.agents.filter(agent => agent.upgrade_available).length,
    manual: state.agents.filter(agent => agent.installed && !agent.upgrade_supported).length,
    active: phases.cli.size,
  }

  return <>
    <div className="overview-toolbar">
      <div className="filters">
        <div className="status-filter" role="group" aria-label={t.filters}>
          {(['all', 'updates', 'installed'] as const).map(key => <button type="button" key={key} aria-pressed={status === key} onClick={() => setStatus(key)}>{key === 'all' ? t.all : key === 'updates' ? t.updateFilter : t.installed}</button>)}
        </div>
      </div>
      <div className="batch-actions">
        <button type="button" className="button secondary" disabled={locked || !cliUpdates.length} onClick={() => setCliConfirm(cliUpdates.map(agent => agent.name))}><ArrowUp size={15} />{t.upgradeAllCli} ({cliUpdates.length})</button>
      </div>
    </div>
    <section className="stats panel" aria-label={t.total}>
      <Stat label={t.installed} value={stats.installed} detail={t.cliHint} />
      <Stat label={t.updates} tone="warning" value={stats.updates} detail={stats.updates ? t.updatesHint : t.noUpdatesHint} />
      <Stat label={t.needsManual} value={stats.manual} detail={t.needsManualHint} />
      <Stat label={t.inProgress} tone="accent" value={stats.active} detail={stats.active ? t.activeHint : t.idle} />
    </section>

    <section className="agent-list panel" aria-label={t.cliTitle}>
      {agents.map(agent => <CliLane key={agent.name} agent={agent} t={t} locked={locked} phase={phases.cli.get(agent.name)} last={last.cli.get(agent.name)} open={expanded.includes(`cli:${agent.name}`)} onToggle={() => toggle(`cli:${agent.name}`)} onUpgrade={() => onUpgradeCli([agent.name])} onError={onError} />)}
      {!agents.length && <div className="empty-inline">{t.noMatch}</div>}
    </section>
    {cliConfirm && <CliConfirmation agents={cliAgents} t={t} submitting={submitting} disabled={locked || cliAgents.length !== cliConfirm.length} onCancel={() => setCliConfirm(null)} onConfirm={tools => onUpgradeCli(tools, () => setCliConfirm(null))} />}
  </>
}

function Stat({ label: text, value, detail, tone }: { label: string; value: number; detail: string; tone?: Tone }) {
  return <div className="stat"><dt>{text}</dt><dd className={value && tone ? tone : ''}>{value}</dd><small>{detail}</small></div>
}

// The row action shows progress in place, so other rows stay usable meanwhile.
function ActiveButton({ phase, name, t }: { phase: string; name: string; t: Dictionary }) {
  return <button type="button" className="button small ghost" aria-label={`${label(t, phase)} ${name}`} disabled><LoaderCircle size={14} className="spin" />{label(t, phase)}</button>
}

const resultTone = (outcome: string): Tone => outcome === 'updated' ? 'good'
  : outcome === 'failed' || outcome === 'unverified' ? 'bad' : 'warning'

function resultNote(outcome: string, t: Dictionary) {
  return ({ unchanged: t.unchangedNote, unverified: t.unverifiedNote, failed: t.failedNote, skipped: t.skippedNote } as Record<string, string>)[outcome] || ''
}

// The short line under the status badge; full output stays in the row details.
function lastSummary(t: Dictionary, outcome: string) {
  if (outcome === 'updated') return t.justUpdated
  return t.lastRun + label(t, outcome)
}

function LastRun({ t, outcome, message, log }: { t: Dictionary; outcome: string; message?: string; log: string }) {
  const note = resultNote(outcome, t)
  return <>
    <dt>{t.lastResult}</dt><dd><span className={resultTone(outcome)}>{label(t, outcome)}{note && <span className="muted-text"> · {note}</span>}</span></dd>
    {message && <><dt>{t.rawDetails}</dt><dd><code>{message}</code></dd></>}
    {log && <><dt>{t.output}</dt><dd><pre className="lane-log">{log}</pre></dd></>}
  </>
}

function CliLane({ agent, t, locked, phase, last, open, onToggle, onUpgrade, onError }: {
  agent: Agent; t: Dictionary; locked: boolean; phase?: string; last?: CliRun; open: boolean; onToggle: () => void; onUpgrade: () => void; onError: (code: string) => void
}) {
  const [text, tone] = phase ? [label(t, phase), 'accent' as Tone] : cliStatus(agent, t)
  const can = cliUpgradeable(agent)
  const name = `${toolLabels[agent.name]} CLI`
  const meta = [binaries[agent.name], agent.installed ? sourceLabel(agent.install_method, t) : ''].filter(Boolean).join(' · ')
  const summary = !phase && last ? lastSummary(t, last.result.outcome) : ''
  return <div className={`lane ${agent.installed ? '' : 'not-installed'}`}>
    <div className="lane-row">
      <FamilyMark family={agent.name} />
      <div className="lane-name"><strong>{name}</strong><small>{meta}</small></div>
      <Versions current={displayVersion(agent.version, t)} latest={agent.upgrade_available ? agent.latest_version : ''} />
      <div className="lane-status"><Badge tone={tone}>{text}</Badge>{summary ? <small className={resultTone(last!.result.outcome)}>{summary}</small> : agent.installed && !can && !phase && <small>{t.noAutomaticUpgrade}</small>}</div>
      <div className="lane-action">{phase ? <ActiveButton phase={phase} name={name} t={t} /> : agent.installed && <button type="button" className={`button small ${agent.upgrade_available ? 'primary' : 'secondary'}`} aria-label={`${t.upgrade} ${name}`} title={upToDate(agent) ? t.upToDate : undefined} disabled={locked || !cliActionable(agent)} onClick={onUpgrade}><ArrowUp size={14} />{t.upgrade}</button>}</div>
      <Toggle open={open} name={name} t={t} onClick={onToggle} />
    </div>
    {open && <div className="lane-details"><dl>
      <dt>{t.path}</dt><dd><code>{agent.path || '—'}</code>{agent.path && <CopyButton value={agent.path} t={t} onError={onError} />}</dd>
      <dt>{t.source}</dt><dd><code>{agent.install_method || '—'}</code></dd>
      <dt>{t.latest}</dt><dd className="mono">{displayVersion(agent.latest_version, t)}</dd>
      <dt>{t.command}</dt><dd>{agent.command?.length ? <><code>{formatCommand(agent.command)}</code><CopyButton value={formatCommand(agent.command)} t={t} onError={onError} /></> : <span>{agent.installed ? t.noAutomaticUpgrade : t.notInstalled}</span>}</dd>
      {agent.check_message && <><dt>{t.checkDetails}</dt><dd><code>{agent.check_message}</code></dd></>}
      {last && <LastRun t={t} outcome={last.result.outcome} message={last.result.message} log={last.log} />}
    </dl></div>}
  </div>
}

function FamilyMark({ family }: { family: ToolName }) {
  return <span className={`family-mark mark-${family}`} aria-hidden="true">{toolLabels[family].slice(0, 1)}</span>
}

function Versions({ current, latest }: { current: string; latest: string }) {
  return <div className="versions mono"><span>{current}</span>{latest && <><ArrowRight size={13} /><span className="warning">{latest}</span></>}</div>
}

function Toggle({ open, name, t, onClick }: { open: boolean; name: string; t: Dictionary; onClick: () => void }) {
  return <button type="button" className="icon-button lane-toggle" title={open ? t.collapse : t.details} aria-label={`${open ? t.collapse : t.details} ${name}`} aria-expanded={open} onClick={onClick}><ChevronDown /></button>
}

// Batch confirmation keeps every item checked by default; unchecking narrows the run.
function usePicked<T>(items: T[]) {
  const [unpicked, setUnpicked] = useState<T[]>([])
  return {
    picked: items.filter(item => !unpicked.includes(item)),
    has: (item: T) => !unpicked.includes(item),
    flip: (item: T) => setUnpicked(list => list.includes(item) ? list.filter(other => other !== item) : [...list, item]),
  }
}

function useModal() {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => { dialog.current?.showModal() }, [])
  return dialog
}

function CliConfirmation({ agents, t, submitting, disabled, onCancel, onConfirm }: {
  agents: Agent[]; t: Dictionary; submitting: boolean; disabled: boolean; onCancel: () => void; onConfirm: (tools: ToolName[]) => void
}) {
  const dialog = useModal()
  const { picked, has, flip } = usePicked(agents.map(agent => agent.name))
  const multiple = agents.length > 1
  return <dialog ref={dialog} className="confirm-dialog" aria-labelledby="confirm-title" onCancel={e => { if (submitting) e.preventDefault(); else onCancel() }}>
    <div className="modal-heading"><Terminal size={20} /><h2 id="confirm-title">{t.confirmTitle}</h2><IconButton label={t.close} disabled={submitting} onClick={onCancel}><X /></IconButton></div>
    <p>{t.confirmText}</p>
    <div className="command-list">{agents.map(agent => <div key={agent.name}>
      <PlanTitle multiple={multiple} checked={has(agent.name)} onChange={() => flip(agent.name)}>{toolLabels[agent.name]} CLI</PlanTitle>
      <span className="mono">{displayVersion(agent.version, t)} {agent.latest_version && <>→ {agent.latest_version}</>}</span>
      <code>{formatCommand(agent.command)}</code>
    </div>)}</div>
    <p className="modal-note"><ShieldCheck size={16} />{t.confirmNote}</p>
    <div className="modal-actions"><button type="button" className="button secondary" disabled={submitting} onClick={onCancel}>{t.cancel}</button><button type="button" className="button primary" disabled={disabled || !picked.length} onClick={() => onConfirm(picked)}>{submitting ? <LoaderCircle size={16} className="spin" /> : <ArrowUp size={16} />}{submitting ? t.submitting : t.confirm}</button></div>
  </dialog>
}

function PlanTitle({ multiple, checked, onChange, children }: { multiple: boolean; checked: boolean; onChange: () => void; children: ReactNode }) {
  return multiple ? <label className="plan-pick"><input type="checkbox" checked={checked} onChange={onChange} /><strong>{children}</strong></label> : <strong>{children}</strong>
}

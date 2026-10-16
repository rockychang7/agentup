import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { AppWindow, ArrowRight, ArrowUp, Check, ChevronDown, CircleAlert, Copy, Download, ExternalLink, LoaderCircle, Power, ShieldCheck, Terminal, X } from 'lucide-react'
import type { Dictionary } from './i18n'
import { toolLabels } from './types'
import type { Agent, DesktopInstallation, DesktopPlan, DesktopResult, Result, State, ToolName } from './types'
import { Badge, IconButton, CopyButton, displayVersion, formatCommand, label, safeURL, sourceLabel } from './ui'
import type { Tone } from './ui'

export type KindFilter = 'cli' | 'desktop'
type StatusFilter = 'all' | 'updates' | 'installed'

const families: ToolName[] = ['codex', 'claude-code', 'opencode', 'agy']
const binaries: Record<ToolName, string> = { codex: 'codex', 'claude-code': 'claude', opencode: 'opencode', agy: 'agy' }
const productFamily: Record<string, ToolName> = {
  'codex.desktop': 'codex', 'claude.desktop': 'claude-code', 'opencode.desktop': 'opencode', 'antigravity.desktop': 'agy',
}
// Reasons repeated by the row action itself are not shown again under the status.
const silentReasons = ['', 'not_installed', 'manual_channel', 'discovery_incomplete', 'app_running']

function cliStatus(agent: Agent, t: Dictionary): [string, Tone] {
  if (agent.latest_check === 'pending') return [t.pending, 'accent']
  if (!agent.installed) return [t.notInstalled, 'muted']
  if (agent.latest_check === 'failed') return [t.checkFailed, 'bad']
  if (agent.upgrade_available) return [t.updateAvailable, 'warning']
  if (agent.latest_check === 'checked') return [t.upToDate, 'good']
  if (agent.latest_check === 'offline') return [t.notChecked, 'muted']
  return [t.unknownLatest, 'muted']
}

function appStatus(info: DesktopInstallation, t: Dictionary): [string, Tone] {
  if (info.reason === 'discovery_incomplete') return [t.discovery_incomplete, 'bad']
  if (!info.installed) return [t.notInstalled, 'muted']
  if (info.latest_check === 'failed') return [t.checkFailed, 'bad']
  if (info.upgrade_available) return [t.updateAvailable, 'warning']
  if (info.latest_check === 'checked') return [t.upToDate, 'good']
  if (info.latest_check === 'offline') return [t.notChecked, 'muted']
  return [t.unknownLatest, 'muted']
}

// A confirmed "nothing newer" result greys the action out; an unknown latest version does not.
const upToDate = (item: { upgrade_available: boolean; latest_check: string }) => item.latest_check === 'checked' && !item.upgrade_available
const cliUpgradeable = (agent: Agent) => agent.installed && agent.upgrade_supported && !!agent.command?.length
const cliActionable = (agent: Agent) => cliUpgradeable(agent) && agent.latest_check !== 'pending' && !upToDate(agent)
// Batch upgrades only include automatic plans that need no separate confirmation.
const directAction = (plan?: DesktopPlan) => plan?.action === 'command' || plan?.action === 'replace' || plan?.action === 'package'
const appAutomatic = (info: DesktopInstallation) => info.installed && info.capability === 'automatic' && directAction(info.plan) && !info.plan!.standalone
const appRunning = (info: DesktopInstallation) => info.installed && info.reason === 'app_running' && info.running === 'running'

type CliRun = { result: Result; log: string }
type AppRun = { action: string; result: DesktopResult; log: string }

// Rows show their own progress and their latest finished result; there is no separate task list.
function rowActivity(state: State) {
  const ids = new Set(state.active_tasks)
  const phases = { cli: new Map<string, string>(), apps: new Map<string, string>() }
  const last = { cli: new Map<string, CliRun>(), apps: new Map<string, AppRun>() }
  const byTime = <T extends { started_at: string }>(list: T[]) => [...list].sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at))
  for (const task of byTime(state.tasks)) for (const tool of task.tools) {
    if (ids.has(task.id) && !tool.result) phases.cli.set(tool.name, tool.phase)
    else if (ids.has(task.id)) phases.cli.set(tool.name, 'refreshing')
    else if (tool.result) last.cli.set(tool.name, { result: tool.result, log: task.logs.filter(line => line.name === tool.name).map(line => line.text).join('') })
  }
  for (const task of byTime(state.desktop_tasks || [])) for (const item of task.items) {
    if (ids.has(task.id) && !item.result) phases.apps.set(item.installation_id, item.phase)
    else if (ids.has(task.id)) phases.apps.set(item.installation_id, 'refreshing')
    else if (item.result) last.apps.set(item.installation_id, {
      action: task.action || 'upgrade', result: item.result,
      log: task.logs.filter(line => line.installation_id === item.installation_id).map(line => line.text).join(''),
    })
  }
  return { phases, last }
}

export function Overview({ state, t, locked, submitting, kind, setKind, onError, onUpgradeCli, onRunDesktop, onStopDesktop }: {
  state: State; t: Dictionary; locked: boolean; submitting: boolean; kind: KindFilter; setKind: (kind: KindFilter) => void
  onError: (code: string) => void
  onUpgradeCli: (tools: ToolName[], done?: () => void) => void
  onRunDesktop: (plans: string[], revision: number, done?: () => void) => void
  onStopDesktop: (installation: string, revision: number, done?: () => void) => void
}) {
  const [status, setStatus] = useState<StatusFilter>('all')
  const [expanded, setExpanded] = useState<string[]>([])
  const [cliConfirm, setCliConfirm] = useState<ToolName[] | null>(null)
  const [appConfirm, setAppConfirm] = useState<{ plans: DesktopPlan[]; revision: number } | null>(null)
  const [stopConfirm, setStopConfirm] = useState<{ info: DesktopInstallation; revision: number } | null>(null)
  const installations = state.desktop?.installations || []
  const issues = state.desktop?.issues || []
  const { phases, last } = rowActivity(state)
  const cliUpdates = state.agents.filter(agent => cliActionable(agent) && agent.upgrade_available && !phases.cli.has(agent.name))
  const appUpdates = installations.filter(info => appAutomatic(info) && !phases.apps.has(info.id))
  const toggle = (id: string) => setExpanded(open => open.includes(id) ? open.filter(item => item !== id) : [...open, id])
  const matches = (installed: boolean, update: boolean) => status === 'all' || (status === 'updates' ? update : installed)
  const order = (family: ToolName) => families.indexOf(family)
  const agents = state.agents.filter(agent => matches(agent.installed, agent.upgrade_available)).sort((a, b) => order(a.name) - order(b.name))
  const apps = installations.filter(info => matches(info.installed, info.upgrade_available))

  function confirmApps(items: DesktopInstallation[]) {
    const plans = items.flatMap(info => info.plan ? [info.plan] : [])
    if (!plans.length || plans.length !== items.length) { onError('stale_preview'); return }
    setAppConfirm({ plans, revision: state.revision })
  }
  // Automatic plans start on click; installers and batches are confirmed first.
  function runApp(info: DesktopInstallation) {
    if (!info.plan) { onError('stale_preview'); return }
    if (directAction(info.plan)) onRunDesktop([info.plan.id], state.revision)
    else confirmApps([info])
  }
  const cliAgents = state.agents.filter(agent => cliConfirm?.includes(agent.name))
  const appName = (plan: DesktopPlan) => installations.find(info => info.id === plan.installation_id)?.name || plan.product_id

  const stats = kind === 'cli' ? {
    installed: state.agents.filter(agent => agent.installed).length,
    updates: state.agents.filter(agent => agent.upgrade_available).length,
    manual: state.agents.filter(agent => agent.installed && !agent.upgrade_supported).length,
    active: phases.cli.size,
  } : {
    installed: installations.filter(info => info.installed).length,
    updates: installations.filter(info => info.upgrade_available).length,
    manual: installations.filter(info => info.installed && info.capability !== 'automatic' && info.capability !== 'interactive' && !appRunning(info)).length,
    active: phases.apps.size,
  }

  return <>
    <div className="overview-toolbar">
      <div className="filters">
        <div className="segmented" role="radiogroup" aria-label={t.agentKind}>
          {([['cli', t.kindCli], ['desktop', t.kindApp]] as const).map(([key, text]) => <button type="button" role="radio" key={key} aria-checked={kind === key} onClick={() => setKind(key)}>
            {key === 'cli' ? <Terminal size={14} /> : <AppWindow size={14} />}{text}
          </button>)}
        </div>
        <div className="status-filter" role="group" aria-label={t.filters}>
          {(['all', 'updates', 'installed'] as const).map(key => <button type="button" key={key} aria-pressed={status === key} onClick={() => setStatus(key)}>{key === 'all' ? t.all : key === 'updates' ? t.updateFilter : t.installed}</button>)}
        </div>
      </div>
      <div className="batch-actions">
        {kind === 'cli' && <button type="button" className="button secondary" disabled={locked || !cliUpdates.length} onClick={() => setCliConfirm(cliUpdates.map(agent => agent.name))}><ArrowUp size={15} />{t.upgradeAllCli} ({cliUpdates.length})</button>}
        {kind === 'desktop' && <button type="button" className="button secondary" title={t.upgradeAutoAppsHint} disabled={locked || !appUpdates.length} onClick={() => confirmApps(appUpdates)}><ArrowUp size={15} />{t.upgradeAutoApps} ({appUpdates.length})</button>}
      </div>
    </div>
    <section className="stats panel" aria-label={t.total}>
      <Stat label={t.installed} value={stats.installed} detail={kind === 'cli' ? t.cliHint : t.appHint} />
      <Stat label={t.updates} tone="warning" value={stats.updates} detail={stats.updates ? t.updatesHint : t.noUpdatesHint} />
      <Stat label={t.needsManual} value={stats.manual} detail={t.needsManualHint} />
      <Stat label={t.inProgress} tone="accent" value={stats.active} detail={stats.active ? t.activeHint : t.idle} />
    </section>
    {kind === 'desktop' && !!issues.length && <details className="notice">
      <summary><CircleAlert size={15} /><span>{t.discoveryIssues}</span><span className="mono">{issues.length}</span></summary>
      <pre>{issues.join('\n')}</pre>
    </details>}
    <section className="agent-list panel" aria-label={kind === 'cli' ? t.cliTitle : t.appTitle}>
      {kind === 'cli' && agents.map(agent => <CliLane key={agent.name} agent={agent} t={t} locked={locked} phase={phases.cli.get(agent.name)} last={last.cli.get(agent.name)} open={expanded.includes(`cli:${agent.name}`)} onToggle={() => toggle(`cli:${agent.name}`)} onUpgrade={() => onUpgradeCli([agent.name])} onError={onError} />)}
      {kind === 'desktop' && apps.map(info => <AppLane key={info.id} info={info} t={t} locked={locked} phase={phases.apps.get(info.id)} last={last.apps.get(info.id)} open={expanded.includes(info.id)} onToggle={() => toggle(info.id)} onRun={() => runApp(info)} onStop={() => setStopConfirm({ info, revision: state.revision })} onError={onError} />)}
      {!(kind === 'cli' ? agents : apps).length && <div className="empty-inline">{kind === 'desktop' && !installations.length ? t.desktopUnavailable : t.noMatch}</div>}
    </section>
    {cliConfirm && <CliConfirmation agents={cliAgents} t={t} submitting={submitting} disabled={locked || cliAgents.length !== cliConfirm.length} onCancel={() => setCliConfirm(null)} onConfirm={tools => onUpgradeCli(tools, () => setCliConfirm(null))} />}
    {appConfirm && <AppConfirmation plans={appConfirm.plans} name={appName} t={t} submitting={submitting} stale={appConfirm.revision !== state.revision} disabled={locked || appConfirm.revision !== state.revision} onCancel={() => setAppConfirm(null)}
      onConfirm={plans => onRunDesktop(plans, appConfirm.revision, () => setAppConfirm(null))} />}
    {stopConfirm && <StopAppConfirmation info={stopConfirm.info} t={t} submitting={submitting} stale={stopConfirm.revision !== state.revision} disabled={locked || stopConfirm.revision !== state.revision} onCancel={() => setStopConfirm(null)}
      onConfirm={() => onStopDesktop(stopConfirm.info.id, stopConfirm.revision, () => setStopConfirm(null))} />}
  </>
}

function Stat({ label: text, value, detail, tone }: { label: string; value: number; detail: string; tone?: Tone }) {
  return <div className="stat"><dt>{text}</dt><dd className={value && tone ? tone : ''}>{value}</dd><small>{detail}</small></div>
}

// The row action shows progress in place, so other rows stay usable meanwhile.
function ActiveButton({ phase, name, t }: { phase: string; name: string; t: Dictionary }) {
  return <button type="button" className="button small ghost" aria-label={`${label(t, phase)} ${name}`} disabled><LoaderCircle size={14} className="spin" />{label(t, phase)}</button>
}

const resultTone = (outcome: string): Tone => outcome === 'updated' || outcome === 'no_update' || outcome === 'stopped' ? 'good'
  : outcome === 'failed' || outcome === 'unverified' || outcome === 'cancelled' ? 'bad' : 'warning'

function resultNote(outcome: string, t: Dictionary) {
  return ({ unchanged: t.unchangedNote, unverified: t.unverifiedNote, failed: t.failedNote, skipped: t.skippedNote, no_update: t.noUpdateNote, cancelled: t.cancelledNote, handed_off: t.desktopGuideNote } as Record<string, string>)[outcome] || ''
}

// The short line under the status badge; full output stays in the row details.
function lastSummary(t: Dictionary, outcome: string, action = 'upgrade', message = '') {
  if (action === 'stop') return outcome === 'stopped' ? '' : t.stopFailed
  if (outcome === 'updated') return message ? t.updatedStillOpen : t.justUpdated
  return `${t.lastRun}${label(t, outcome)}`
}

function LastRun({ t, outcome, action, message, log }: { t: Dictionary; outcome: string; action?: string; message?: string; log: string }) {
  const note = action === 'stop' ? '' : resultNote(outcome, t)
  return <>
    <dt>{t.lastResult}</dt><dd><span className={resultTone(outcome)}>{action === 'stop' ? (outcome === 'stopped' ? t.appClosed : t.stopFailed) : label(t, outcome)}{note && <span className="muted-text"> · {note}</span>}</span></dd>
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

function latestSourceLabel(source: string | undefined, t: Dictionary) {
  return source ? label(t, `latest_${source.replace(/-/g, '_')}`) : '—'
}

// What a plan will touch, shown in details and in the confirmation.
function planTarget(plan: DesktopPlan) {
  if (plan.action === 'installer' || plan.action === 'replace' || plan.action === 'package') return plan.asset?.url || ''
  if (plan.action === 'guide') return plan.url || ''
  return formatCommand(plan.command)
}

function AppLane({ info, t, locked, phase, last, open, onToggle, onRun, onStop, onError }: {
  info: DesktopInstallation; t: Dictionary; locked: boolean; phase?: string; last?: AppRun; open: boolean
  onToggle: () => void; onRun: () => void; onStop: () => void; onError: (code: string) => void
}) {
  const [copied, setCopied] = useState(false)
  const [text, tone] = phase ? [label(t, phase), 'accent' as Tone] : appStatus(info, t)
  const running = appRunning(info)
  const reason = running ? t.app_running : info.installed && !silentReasons.includes(info.reason) ? label(t, info.reason)
    : info.plan?.standalone ? t.standaloneNote : info.plan?.requires_admin && info.capability === 'automatic' ? t.adminPromptNote : ''
  const summary = !phase && last ? lastSummary(t, last.result.outcome, last.action, last.result.message) : ''
  const meta = info.installed ? [sourceLabel(info.source, t), label(t, info.scope), info.arch !== 'unknown' ? info.arch : ''].filter(item => item && item !== '—').join(' · ') : t.appTitle
  const disabled = locked || !info.plan || upToDate(info)
  const guide = safeURL(info.plan?.url || info.update_url)
  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), 2400)
    return () => clearTimeout(timer)
  }, [copied])
  // Privileged commands need a password, so the browser only copies them for a terminal.
  function copyCommand() {
    if (!navigator.clipboard || !info.plan) { onError('copyFailed'); return }
    navigator.clipboard.writeText(formatCommand(info.plan.command)).then(() => setCopied(true)).catch(() => onError('copyFailed'))
  }
  const style = info.plan && !upToDate(info) ? 'primary' : 'secondary'
  let action: ReactNode = null
  if (phase) action = <ActiveButton phase={phase} name={info.name} t={t} />
  else if (running) action = <button type="button" className={`button small ${info.upgrade_available ? 'primary' : 'secondary'}`} aria-label={`${t.quitApp} ${info.name}`} disabled={locked} onClick={onStop}><Power size={14} />{t.quitApp}</button>
  else if (info.installed) {
    if (info.capability === 'guided') action = guide && !locked && info.plan
      ? <a className="button small secondary" href={guide} target="_blank" rel="noreferrer noopener" aria-label={`${t.openUpdate} ${info.name}`}><ExternalLink size={14} />{t.openUpdate}</a>
      : <button type="button" className="button small secondary" aria-label={`${t.openUpdate} ${info.name}`} disabled><ExternalLink size={14} />{t.openUpdate}</button>
    else if (info.capability === 'privileged') action = <button type="button" className={`button small ${style}`} aria-label={`${t.copyCommand} ${info.name}`} disabled={disabled} onClick={copyCommand}>{copied ? <Check size={14} /> : <Copy size={14} />}{copied ? t.copied : t.copyCommand}</button>
    else if (info.capability === 'interactive') action = <button type="button" className={`button small ${style}`} aria-label={`${t.downloadInstall} ${info.name}`} disabled={disabled} onClick={onRun}><Download size={14} />{t.downloadInstall}</button>
    else action = <button type="button" className={`button small ${style}`} aria-label={`${t.upgrade} ${info.name}`} title={upToDate(info) ? t.upToDate : undefined} disabled={disabled} onClick={onRun}><ArrowUp size={14} />{t.upgrade}</button>
  }
  const family = productFamily[info.product_id] || 'codex'
  return <div className={`lane ${info.installed ? '' : 'not-installed'}`}>
    <div className="lane-row">
      <FamilyMark family={family} />
      <div className="lane-name"><strong>{info.name}</strong><small>{meta}</small></div>
      <Versions current={info.version || '—'} latest={info.upgrade_available ? info.latest_version : ''} />
      <div className="lane-status"><Badge tone={tone}>{text}</Badge>{copied ? <small className="accent">{t.commandCopied}</small>
        : summary ? <small className={resultTone(last!.result.outcome)}>{summary}</small> : reason && !phase && <small>{reason}</small>}</div>
      <div className="lane-action">{action}</div>
      <Toggle open={open} name={info.name} t={t} onClick={onToggle} />
    </div>
    {open && <div className="lane-details"><dl>
      <dt>{t.installation}</dt><dd><code>{info.id}</code></dd>
      <dt>{t.location}</dt><dd><code>{info.path || '—'}</code></dd>
      <dt>{t.packageId}</dt><dd><code>{info.package_id || '—'}</code></dd>
      <dt>{t.source}</dt><dd><code>{info.source || '—'}</code></dd>
      <dt>{t.upgradeMode}</dt><dd>{label(t, info.capability)}</dd>
      <dt>{t.channel}</dt><dd>{label(t, info.channel)}</dd>
      <dt>{t.architecture}</dt><dd>{label(t, info.arch)}</dd>
      <dt>{t.processState}</dt><dd>{info.running === 'running' ? t.appRunning : info.running === 'stopped' ? t.appStopped : t.unknown}</dd>
      <dt>{t.latest}</dt><dd><code>{info.latest_version || '—'}</code></dd>
      <dt>{t.latestSource}</dt><dd>{latestSourceLabel(info.latest_source, t)}</dd>
      {info.reason && <><dt>{t.status}</dt><dd>{label(t, info.reason)}</dd></>}
      {info.plan && <><dt>{info.plan.action === 'guide' ? t.desktopEntry : info.plan.action === 'installer' || info.plan.action === 'replace' || info.plan.action === 'package' ? t.installerFile : t.command}</dt><dd><code>{planTarget(info.plan)}</code></dd></>}
      {info.check_message && <><dt>{t.checkDetails}</dt><dd><code>{info.check_message}</code></dd></>}
      {last && <LastRun t={t} outcome={last.result.outcome} action={last.action} message={last.result.message} log={last.log} />}
    </dl></div>}
  </div>
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

function AppConfirmation({ plans, name, t, submitting, disabled, stale, onCancel, onConfirm }: {
  plans: DesktopPlan[]; name: (plan: DesktopPlan) => string; t: Dictionary; submitting: boolean; disabled: boolean; stale: boolean
  onCancel: () => void; onConfirm: (plans: string[]) => void
}) {
  const dialog = useModal()
  const { picked, has, flip } = usePicked(plans.map(plan => plan.id))
  const installer = plans.some(plan => plan.action === 'installer')
  const winget = plans.some(plan => plan.channel === 'winget' || plan.channel === 'msstore')
  const note = installer ? t.installerNote : winget ? t.wingetNote : t.desktopConfirmNote
  return <dialog ref={dialog} className="confirm-dialog" aria-labelledby="desktop-confirm-title" onCancel={e => { if (submitting) e.preventDefault(); else onCancel() }}>
    <div className="modal-heading"><AppWindow size={20} /><h2 id="desktop-confirm-title">{t.desktopConfirm}</h2><IconButton label={t.close} disabled={submitting} onClick={onCancel}><X /></IconButton></div>
    <p>{t.desktopConfirmText}</p>
    <div className="command-list">{plans.map(plan => <div key={plan.id}>
      <PlanTitle multiple={plans.length > 1} checked={has(plan.id)} onChange={() => flip(plan.id)}>{name(plan)}</PlanTitle>
      <span className="mono">{plan.before_version}{plan.target_version && <> → {plan.target_version}</>}</span>
      <code>{plan.path}</code><code>{planTarget(plan)}</code>
      {plan.asset && <span className="plan-meta">{t.publisher}: {plan.asset.publisher} · {t.size}: {(plan.asset.size / 1048576).toFixed(1)} MB · SHA-512 {plan.asset.sha512.slice(0, 16)}…</span>}
      {plan.channel !== 'unknown' && !plan.asset && <span className="plan-meta">{t.channel}: {label(t, plan.channel)}</span>}
    </div>)}</div>
    <p className="modal-note"><ShieldCheck size={16} />{note}</p>
    {stale && <p className="warning small-text">{t.stale_preview}</p>}
    <div className="modal-actions"><button type="button" className="button secondary" disabled={submitting} onClick={onCancel}>{t.cancel}</button><button type="button" className="button primary" disabled={disabled || !picked.length} onClick={() => onConfirm(picked)}>{submitting ? <LoaderCircle size={15} className="spin" /> : installer ? <Download size={15} /> : <ArrowUp size={15} />}{submitting ? t.submitting : installer ? t.downloadInstall : t.confirm}</button></div>
  </dialog>
}

// Quitting closes every window of the app, so the user decides before anything is stopped.
function StopAppConfirmation({ info, t, submitting, disabled, stale, onCancel, onConfirm }: {
  info: DesktopInstallation; t: Dictionary; submitting: boolean; disabled: boolean; stale: boolean; onCancel: () => void; onConfirm: () => void
}) {
  const dialog = useModal()
  return <dialog ref={dialog} className="confirm-dialog" aria-labelledby="quit-title" onCancel={e => { if (submitting) e.preventDefault(); else onCancel() }}>
    <div className="modal-heading"><span className="modal-icon"><Power size={20} /></span><h2 id="quit-title">{t.quitTitle.replace('{name}', info.name)}</h2><IconButton label={t.close} disabled={submitting} onClick={onCancel}><X /></IconButton></div>
    <p>{t.quitText}</p>
    {stale && <p className="warning small-text">{t.stale_preview}</p>}
    <div className="modal-actions"><button type="button" className="button secondary" disabled={submitting} onClick={onCancel}>{t.cancel}</button><button type="button" className="button danger" disabled={disabled} onClick={onConfirm}>{submitting ? <LoaderCircle size={16} className="spin" /> : <Power size={16} />}{submitting ? t.submitting : t.confirmQuit}</button></div>
  </dialog>
}

function PlanTitle({ multiple, checked, onChange, children }: { multiple: boolean; checked: boolean; onChange: () => void; children: ReactNode }) {
  return multiple ? <label className="plan-pick"><input type="checkbox" checked={checked} onChange={onChange} /><strong>{children}</strong></label> : <strong>{children}</strong>
}

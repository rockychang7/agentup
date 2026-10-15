import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { AppWindow, ArrowRight, ArrowUp, Check, ChevronDown, ChevronUp, CircleAlert, Copy, Download, ExternalLink, LoaderCircle, ShieldCheck, Terminal, X } from 'lucide-react'
import type { Dictionary } from './i18n'
import { toolLabels } from './types'
import type { Agent, DesktopInstallation, DesktopPlan, State, ToolName } from './types'
import { Badge, CopyButton, IconButton, KindChip, displayVersion, formatCommand, label, sourceLabel } from './ui'
import type { Kind, Tone } from './ui'

export type KindFilter = 'all' | Kind
type StatusFilter = 'all' | 'updates' | 'installed'

const families: ToolName[] = ['codex', 'claude-code', 'opencode', 'agy']
const binaries: Record<ToolName, string> = { codex: 'codex', 'claude-code': 'claude', opencode: 'opencode', agy: 'agy' }
const vendors: Record<ToolName, string> = { codex: 'OpenAI', 'claude-code': 'Anthropic', opencode: 'OpenCode', agy: 'Google Antigravity' }
const productFamily: Record<string, ToolName> = {
  'codex.desktop': 'codex', 'claude.desktop': 'claude-code', 'opencode.desktop': 'opencode', 'antigravity.desktop': 'agy',
}
// Reasons repeated by the row action itself are not shown again under the status.
const silentReasons = ['', 'not_installed', 'manual_channel', 'discovery_incomplete']

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
  if (info.latest_check === 'checked') return [info.latest_source === 'brew-reference' ? t.referenceCurrent : t.channelCurrent, 'good']
  if (info.latest_check === 'offline') return [t.notChecked, 'muted']
  return [t.unknownLatest, 'muted']
}

const cliUpgradeable = (agent: Agent) => agent.installed && agent.upgrade_supported && !!agent.command?.length
// Batch upgrades only include automatic plans that need no separate confirmation.
const appAutomatic = (info: DesktopInstallation) => info.installed && info.capability === 'automatic' && info.plan?.action === 'command' && !info.plan.standalone

export function Overview({ state, t, busy, submitting, kind, setKind, onError, onUpgradeCli, onRunDesktop }: {
  state: State; t: Dictionary; busy: boolean; submitting: boolean; kind: KindFilter; setKind: (kind: KindFilter) => void
  onError: (code: string) => void; onUpgradeCli: (tools: ToolName[]) => void; onRunDesktop: (plans: string[], revision: number) => void
}) {
  const [status, setStatus] = useState<StatusFilter>('all')
  const [expanded, setExpanded] = useState<string[]>([])
  const [cliConfirm, setCliConfirm] = useState<ToolName[] | null>(null)
  const [appConfirm, setAppConfirm] = useState<{ plans: DesktopPlan[]; revision: number } | null>(null)
  const disabled = busy || state.offline
  const installations = state.desktop?.installations || []
  const issues = state.desktop?.issues || []
  const cliUpdates = state.agents.filter(agent => cliUpgradeable(agent) && agent.upgrade_available)
  const appUpdates = installations.filter(appAutomatic)
  const toggle = (id: string) => setExpanded(open => open.includes(id) ? open.filter(item => item !== id) : [...open, id])
  const matches = (installed: boolean, update: boolean) => status === 'all' || (status === 'updates' ? update : installed)
  const groups = families.map(family => ({
    family,
    cli: kind === 'desktop' ? [] : state.agents.filter(agent => agent.name === family && matches(agent.installed, agent.upgrade_available)),
    apps: kind === 'cli' ? [] : installations.filter(info => productFamily[info.product_id] === family && matches(info.installed, info.upgrade_available)),
  })).filter(group => group.cli.length + group.apps.length > 0)

  function confirmApps(items: DesktopInstallation[]) {
    const plans = items.flatMap(info => info.plan ? [info.plan] : [])
    if (!plans.length || plans.length !== items.length) { onError('stale_preview'); return }
    setAppConfirm({ plans, revision: state.revision })
  }
  const cliAgents = state.agents.filter(agent => cliConfirm?.includes(agent.name))

  return <>
    <section className="summary-grid" aria-label={t.total}>
      <SummaryCard kind="cli" icon={<Terminal size={18} />} title={t.cliTitle} hint={t.cliHint} stats={[
        [state.agents.filter(agent => agent.installed).length, t.installed],
        [state.agents.filter(agent => agent.upgrade_available).length, t.updates],
        [state.agents.filter(agent => agent.installed && (agent.latest_check === 'failed' || agent.latest_check === 'unavailable' || !agent.upgrade_supported)).length, t.attention],
      ]} action={<button type="button" className="button kind" disabled={disabled || !cliUpdates.length} onClick={() => setCliConfirm(cliUpdates.map(agent => agent.name))}><ArrowUp size={15} />{t.upgradeAllCli} ({cliUpdates.length})</button>} />
      <SummaryCard kind="desktop" icon={<AppWindow size={18} />} title={t.appTitle} hint={t.appHint} stats={[
        [installations.filter(info => info.installed).length, t.installed],
        [installations.filter(info => info.upgrade_available).length, t.updates],
        [installations.filter(info => info.installed && info.capability !== 'automatic').length, t.needsManual],
      ]} action={<button type="button" className="button kind" disabled={disabled || !appUpdates.length} onClick={() => confirmApps(appUpdates)}><ArrowUp size={15} />{t.upgradeAutoApps} ({appUpdates.length})</button>} />
    </section>
    <div className="overview-toolbar">
      <div className="kind-switch" role="radiogroup" aria-label={t.agentKind}>
        {([['all', t.kindAll], ['cli', t.kindCli], ['desktop', t.kindApp]] as const).map(([key, text]) => <button type="button" role="radio" key={key} aria-checked={kind === key} onClick={() => setKind(key)}>
          {key === 'cli' ? <Terminal size={14} /> : key === 'desktop' ? <AppWindow size={14} /> : null}{text}
        </button>)}
      </div>
      <div className="status-filter" role="group" aria-label={t.filters}>
        <span>{t.status}</span>
        {(['all', 'updates', 'installed'] as const).map(key => <button type="button" key={key} aria-pressed={status === key} onClick={() => setStatus(key)}>{key === 'all' ? t.all : key === 'updates' ? t.updateFilter : t.installed}</button>)}
      </div>
    </div>
    {kind !== 'cli' && !!issues.length && <details className="notice">
      <summary><CircleAlert size={15} /><span>{t.discoveryIssues}</span><span className="mono">{issues.length}</span></summary>
      <pre>{issues.join('\n')}</pre>
    </details>}
    <div className="agent-groups">
      {groups.map(group => <section className="agent-group" key={group.family} aria-label={toolLabels[group.family]}>
        <header className="group-head"><span className={`family-mark mark-${group.family}`} /><h2>{toolLabels[group.family]}</h2><small>{vendors[group.family]}</small></header>
        {group.cli.map(agent => <CliLane key={agent.name} agent={agent} t={t} disabled={disabled} open={expanded.includes(`cli:${agent.name}`)} onToggle={() => toggle(`cli:${agent.name}`)} onUpgrade={() => setCliConfirm([agent.name])} onError={onError} />)}
        {group.apps.map(info => <AppLane key={info.id} info={info} t={t} disabled={disabled} open={expanded.includes(info.id)} onToggle={() => toggle(info.id)} onRun={() => confirmApps([info])} onError={onError} />)}
      </section>)}
      {!groups.length && <div className="empty-inline">{installations.length || kind !== 'desktop' ? t.noMatch : t.desktopUnavailable}</div>}
    </div>
    {cliConfirm && <CliConfirmation agents={cliAgents} t={t} submitting={submitting} disabled={disabled || cliAgents.length !== cliConfirm.length} onCancel={() => setCliConfirm(null)} onConfirm={onUpgradeCli} />}
    {appConfirm && <AppConfirmation plans={appConfirm.plans} installations={installations} t={t} submitting={submitting} stale={appConfirm.revision !== state.revision} disabled={disabled || appConfirm.revision !== state.revision} onCancel={() => setAppConfirm(null)} onConfirm={plans => onRunDesktop(plans, appConfirm.revision)} />}
  </>
}

function SummaryCard({ kind, icon, title, hint, stats, action }: {
  kind: Kind; icon: ReactNode; title: string; hint: string; stats: (readonly [number, string])[]; action: ReactNode
}) {
  return <article className={`summary-card ${kind}`}>
    <div className="summary-head"><span className="kind-icon">{icon}</span><div><h2>{title}</h2><p>{hint}</p></div></div>
    <dl className="summary-stats">{stats.map(([count, text]) => <div key={text}><dt>{text}</dt><dd>{count}</dd></div>)}</dl>
    {action}
  </article>
}

function CliLane({ agent, t, disabled, open, onToggle, onUpgrade, onError }: {
  agent: Agent; t: Dictionary; disabled: boolean; open: boolean; onToggle: () => void; onUpgrade: () => void; onError: (code: string) => void
}) {
  const [text, tone] = cliStatus(agent, t)
  const can = cliUpgradeable(agent)
  const name = `${toolLabels[agent.name]} CLI`
  return <div className={`lane ${agent.installed ? '' : 'not-installed'}`}>
    <div className="lane-row">
      <KindChip kind="cli" />
      <div className="lane-name"><strong>{name}</strong><small className="mono">{binaries[agent.name]}</small></div>
      <div className="versions mono"><span>{displayVersion(agent.version, t)}</span>{agent.upgrade_available && <><ArrowRight size={13} /><span className="warning">{agent.latest_version}</span></>}</div>
      <div className="lane-source">{agent.installed ? sourceLabel(agent.install_method, t) : '—'}</div>
      <div className="lane-status"><Badge tone={tone}>{text}</Badge>{agent.installed && !can && <small>{t.noAutomaticUpgrade}</small>}</div>
      <div className="lane-action">{agent.installed && <button type="button" className={`button small ${can && agent.upgrade_available ? 'primary' : 'secondary'}`} aria-label={`${t.upgrade} ${name}`} disabled={disabled || !can} onClick={onUpgrade}><ArrowUp size={14} />{t.upgrade}</button>}</div>
      <Toggle open={open} name={name} t={t} onClick={onToggle} />
    </div>
    {open && <div className="lane-details"><dl>
      <dt>{t.path}</dt><dd><code>{agent.path || '—'}</code>{agent.path && <CopyButton value={agent.path} t={t} onError={onError} />}</dd>
      <dt>{t.source}</dt><dd><code>{agent.install_method || '—'}</code></dd>
      <dt>{t.latest}</dt><dd className="mono">{displayVersion(agent.latest_version, t)}</dd>
      <dt>{t.command}</dt><dd>{agent.command?.length ? <><code>{formatCommand(agent.command)}</code><CopyButton value={formatCommand(agent.command)} t={t} onError={onError} /></> : <span>{agent.installed ? t.noAutomaticUpgrade : t.notInstalled}</span>}</dd>
      {agent.check_message && <><dt>{t.rawDetails}</dt><dd><code>{agent.check_message}</code></dd></>}
    </dl></div>}
  </div>
}

function latestSourceLabel(source: string | undefined, t: Dictionary) {
  return source ? label(t, `latest_${source.replace(/-/g, '_')}`) : '—'
}

// What a plan will touch, shown in details and in the confirmation.
function planTarget(plan: DesktopPlan) {
  if (plan.action === 'installer') return plan.asset?.url || ''
  if (plan.action === 'guide') return plan.url || ''
  return formatCommand(plan.command)
}

function AppLane({ info, t, disabled, open, onToggle, onRun, onError }: {
  info: DesktopInstallation; t: Dictionary; disabled: boolean; open: boolean; onToggle: () => void; onRun: () => void; onError: (code: string) => void
}) {
  const [copied, setCopied] = useState(false)
  const [text, tone] = appStatus(info, t)
  const reason = info.installed && !silentReasons.includes(info.reason) ? label(t, info.reason) : info.plan?.standalone ? t.standaloneNote : ''
  const detail = info.installed ? [label(t, info.scope), info.arch !== 'unknown' ? info.arch : ''].filter(Boolean).join(' · ') : t.appTitle
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
  let action = null
  if (info.installed) {
    if (info.capability === 'guided') action = <button type="button" className="button small secondary" aria-label={`${t.openUpdate} ${info.name}`} disabled={disabled || !info.plan} onClick={onRun}><ExternalLink size={14} />{t.openUpdate}</button>
    else if (info.capability === 'privileged') action = <button type="button" className="button small primary" aria-label={`${t.copyCommand} ${info.name}`} disabled={disabled || !info.plan} onClick={copyCommand}>{copied ? <Check size={14} /> : <Copy size={14} />}{copied ? t.copied : t.copyCommand}</button>
    else if (info.capability === 'interactive') action = <button type="button" className={`button small ${info.plan ? 'primary' : 'secondary'}`} aria-label={`${t.downloadInstall} ${info.name}`} disabled={disabled || !info.plan} onClick={onRun}><Download size={14} />{t.downloadInstall}</button>
    else action = <button type="button" className={`button small ${info.plan ? 'primary' : 'secondary'}`} aria-label={`${t.upgrade} ${info.name}`} disabled={disabled || !info.plan} onClick={onRun}><ArrowUp size={14} />{t.upgrade}</button>
  }
  return <div className={`lane ${info.installed ? '' : 'not-installed'}`}>
    <div className="lane-row">
      <KindChip kind="desktop" />
      <div className="lane-name"><strong>{info.name}</strong><small>{detail}</small></div>
      <div className="versions mono"><span>{info.version || '—'}</span>{info.upgrade_available && <><ArrowRight size={13} /><span className="warning">{info.latest_version}</span></>}</div>
      <div className="lane-source">{info.installed ? sourceLabel(info.source, t) : '—'}</div>
      <div className="lane-status"><Badge tone={tone}>{text}</Badge>{copied ? <small className="accent">{t.commandCopied}</small> : reason && <small>{reason}</small>}</div>
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
      <dt>{t.candidateVersion}</dt><dd><code>{info.latest_version || '—'}</code></dd>
      <dt>{t.latestSource}</dt><dd>{latestSourceLabel(info.latest_source, t)}</dd>
      {info.reason && <><dt>{t.status}</dt><dd>{label(t, info.reason)}</dd></>}
      {info.plan && <><dt>{info.plan.action === 'guide' ? t.desktopEntry : info.plan.action === 'installer' ? t.installerFile : t.command}</dt><dd><code>{planTarget(info.plan)}</code></dd></>}
      {info.check_message && <><dt>{t.rawDetails}</dt><dd><code>{info.check_message}</code></dd></>}
    </dl></div>}
  </div>
}

function Toggle({ open, name, t, onClick }: { open: boolean; name: string; t: Dictionary; onClick: () => void }) {
  return <button type="button" className="icon-button lane-toggle" title={open ? t.collapse : t.details} aria-label={`${open ? t.collapse : t.details} ${name}`} aria-expanded={open} onClick={onClick}>{open ? <ChevronUp /> : <ChevronDown />}</button>
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
    <div className="modal-heading"><KindChip kind="cli" /><h2 id="confirm-title">{t.confirmTitle}</h2><IconButton label={t.close} disabled={submitting} onClick={onCancel}><X /></IconButton></div>
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

function AppConfirmation({ plans, installations, t, submitting, disabled, stale, onCancel, onConfirm }: {
  plans: DesktopPlan[]; installations: DesktopInstallation[]; t: Dictionary; submitting: boolean; disabled: boolean; stale: boolean
  onCancel: () => void; onConfirm: (plans: string[]) => void
}) {
  const dialog = useModal()
  const { picked, has, flip } = usePicked(plans.map(plan => plan.id))
  const guided = plans.some(plan => plan.action === 'guide')
  const installer = plans.some(plan => plan.action === 'installer')
  const winget = plans.some(plan => plan.channel === 'winget' || plan.channel === 'msstore')
  const note = guided ? t.desktopGuideNote : installer ? t.installerNote : winget ? t.wingetNote : t.desktopConfirmNote
  const name = (plan: DesktopPlan) => installations.find(info => info.id === plan.installation_id)?.name || plan.product_id
  return <dialog ref={dialog} className="confirm-dialog" aria-labelledby="desktop-confirm-title" onCancel={e => { if (submitting) e.preventDefault(); else onCancel() }}>
    <div className="modal-heading"><KindChip kind="desktop" /><h2 id="desktop-confirm-title">{t.desktopConfirm}</h2><IconButton label={t.close} disabled={submitting} onClick={onCancel}><X /></IconButton></div>
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
    <div className="modal-actions"><button type="button" className="button secondary" disabled={submitting} onClick={onCancel}>{t.cancel}</button><button type="button" className="button primary" disabled={disabled || !picked.length} onClick={() => onConfirm(picked)}>{submitting ? <LoaderCircle size={15} className="spin" /> : guided ? <ExternalLink size={15} /> : <ArrowUp size={15} />}{submitting ? t.submitting : guided ? t.openUpdate : installer ? t.downloadInstall : t.confirm}</button></div>
  </dialog>
}

function PlanTitle({ multiple, checked, onChange, children }: { multiple: boolean; checked: boolean; onChange: () => void; children: ReactNode }) {
  return multiple ? <label className="plan-pick"><input type="checkbox" checked={checked} onChange={onChange} /><strong>{children}</strong></label> : <strong>{children}</strong>
}

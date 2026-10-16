export type ToolName = 'codex' | 'claude-code' | 'opencode' | 'agy'
export type Check = 'pending' | 'checked' | 'offline' | 'unavailable' | 'failed'
export type Outcome = 'updated' | 'unchanged' | 'unverified' | 'failed' | 'skipped'
export type Phase = 'detecting' | 'waiting' | 'upgrading' | 'verifying' | 'finished'
export type DesktopPhase = Phase | 'stopping' | 'checking' | 'needs_elevation' | 'downloading' | 'verifying_package' | 'installing'
export interface Agent {
  name: ToolName
  installed: boolean
  version: string
  latest_version: string
  upgrade_available: boolean
  install_method: string
  path: string
  upgrade_supported: boolean
  latest_check: Check
  check_message?: string
  command?: string[]
}
export interface Result {
  name: ToolName
  status: string
  outcome: Outcome
  old_version: string
  new_version: string
  message?: string
  duration_ms: number
  command?: string[]
}
export interface Task {
  id: string
  status: 'queued' | 'running' | 'completed' | 'failed'
  started_at: string
  finished_at?: string
  tools: { name: ToolName; phase: Phase; result?: Result }[]
  logs: { name: ToolName; stream: string; text: string }[]
  log_truncated: boolean
}
export interface Doctor {
  os: string
  managers: { name: string; found: boolean; path: string; version: string; error?: string }[]
  tools: { name: ToolName; found: boolean; path: string; issue?: string; issue_code?: string }[]
}
export interface State {
  version: string
  os: string
  arch: string
  agents: Agent[]
  tasks: Task[]
  doctor: Doctor | null
  scanning: boolean
  diagnosing: boolean
  active_tasks: string[]
  offline: boolean
  revision: number
  desktop?: DesktopSnapshot
  desktop_tasks?: DesktopTask[]
}

export interface DesktopPlan {
  id: string
  installation_id: string
  product_id: string
  before_version: string
  target_version: string
  path: string
  package_id: string
  source: string
  scope: string
  arch: string
  channel: string
  action: 'command' | 'privileged_command' | 'installer' | 'guide' | 'replace' | 'package'
  command?: string[]
  url?: string
  asset?: { url: string; sha512: string; size: number; publisher: string }
  requires_exit: boolean
  requires_admin: boolean
  standalone?: boolean
}
export interface DesktopInstallation {
  id: string
  product_id: string
  name: string
  kind: 'desktop'
  installed: boolean
  version: string
  latest_version: string
  latest_check: Check
  latest_source?: string
  upgrade_available: boolean
  path: string
  package_id: string
  source: string
  scope: string
  arch: string
  channel: string
  running: 'running' | 'stopped' | 'unknown'
  capability: 'automatic' | 'interactive' | 'privileged' | 'guided' | 'blocked'
  reason: string
  check_message?: string
  update_url: string
  plan?: DesktopPlan
}
export interface DesktopSnapshot {
  installations: DesktopInstallation[]
  issues: string[]
}
export interface DesktopResult {
  installation_id: string
  product_id: string
  name: string
  outcome: 'updated' | 'no_update' | 'unchanged' | 'unverified' | 'failed' | 'skipped' | 'handed_off' | 'cancelled' | 'stopped'
  old_version: string
  new_version: string
  message?: string
  duration_ms: number
  command?: string[]
  url?: string
}
export interface DesktopTask {
  id: string
  action?: 'upgrade' | 'stop'
  status: 'queued' | 'running' | 'completed' | 'failed' | 'handed_off'
  started_at: string
  finished_at?: string
  items: { installation_id: string; name: string; phase: DesktopPhase; result?: DesktopResult }[]
  logs: { installation_id: string; stream: string; text: string }[]
  log_truncated: boolean
}
export const toolLabels: Record<ToolName, string> = {
  codex: 'Codex',
  'claude-code': 'Claude Code',
  opencode: 'OpenCode',
  agy: 'Agy',
}

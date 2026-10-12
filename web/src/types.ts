export type ToolName = 'codex' | 'claude-code' | 'opencode' | 'agy'
export type Check = 'pending' | 'checked' | 'offline' | 'unavailable' | 'failed'
export type Outcome = 'updated' | 'unchanged' | 'unverified' | 'failed' | 'skipped'
export type Phase = 'detecting' | 'waiting' | 'upgrading' | 'verifying' | 'finished'
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
  status: 'running' | 'completed' | 'failed'
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
  active_task: string
  offline: boolean
  revision: number
}
export const toolLabels: Record<ToolName, string> = {
  codex: 'Codex',
  'claude-code': 'Claude Code',
  opencode: 'OpenCode',
  agy: 'Agy',
}

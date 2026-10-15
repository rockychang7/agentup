import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { AppWindow, Check, Copy, Terminal } from 'lucide-react'
import type { Dictionary } from './i18n'

export type Tone = 'good' | 'warning' | 'bad' | 'muted' | 'accent'
export type Kind = 'cli' | 'desktop'

export function Badge({ tone = 'muted', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`badge ${tone}`}><span className="status-dot" />{children}</span>
}

export function IconButton({ label, children, onClick, disabled = false }: {
  label: string; children: ReactNode; onClick: () => void; disabled?: boolean
}) {
  return <button type="button" className="icon-button" title={label} aria-label={label} disabled={disabled} onClick={onClick}>{children}</button>
}

// The CLI/App chip uses fixed colors everywhere so the two kinds never look alike.
export function KindChip({ kind }: { kind: Kind }) {
  return <span className={`kind-chip ${kind}`}>{kind === 'cli' ? <Terminal size={13} /> : <AppWindow size={13} />}{kind === 'cli' ? 'CLI' : 'App'}</span>
}

export function CopyButton({ value, t, onError }: { value: string; t: Dictionary; onError: (code: string) => void }) {
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

export function formatCommand(command?: string[]) {
  return (command || []).map(arg => /[\s"'`]/.test(arg) ? JSON.stringify(arg) : arg).join(' ')
}

export function displayVersion(value: string, t: Dictionary) {
  return value === 'unknown' ? t.unknown : value || '—'
}

export function label(t: Dictionary, key: string) {
  return t[key as keyof Dictionary] || key
}

const sourceKeys: Record<string, keyof Dictionary> = {
  'self-update': 'selfUpdate', binary: 'binary', unknown: 'unknown', msix: 'srcMsix', installer: 'srcInstaller',
  'app-bundle': 'srcDirect', portable: 'srcPortable',
}
const sourceNames: Record<string, string> = { brew: 'Homebrew', 'brew-cask': 'Homebrew', apt: 'APT', rpm: 'RPM' }

// Install sources are internal codes; show readable names and keep the code in details.
export function sourceLabel(source: string, t: Dictionary) {
  if (!source) return '—'
  const key = sourceKeys[source]
  return key ? t[key] : sourceNames[source] || source
}

export function taskDate(value: string, language: string) {
  return new Date(value).toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Check, Copy } from 'lucide-react'
import type { Dictionary } from './i18n'

export type Tone = 'good' | 'warning' | 'bad' | 'muted' | 'accent'

export function Badge({ tone = 'muted', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`badge ${tone}`}><span className="status-dot" />{children}</span>
}

export function IconButton({ label, children, onClick, disabled = false }: {
  label: string; children: ReactNode; onClick: () => void; disabled?: boolean
}) {
  return <button type="button" className="icon-button" title={label} aria-label={label} disabled={disabled} onClick={onClick}>{children}</button>
}

// Only official update pages open from the browser.
export function safeURL(value?: string) {
  try {
    const url = new URL(value || '')
    return url.protocol === 'https:' && ['chatgpt.com', 'claude.com', 'opencode.ai', 'antigravity.google'].includes(url.hostname) ? url.href : ''
  } catch { return '' }
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
const sourceNames: Record<string, string> = { brew: 'Homebrew', 'brew-cask': 'Homebrew', apt: 'APT', rpm: 'RPM', flatpak: 'Flatpak', snap: 'Snap', appimage: 'AppImage' }

// Install sources are internal codes; show readable names and keep the code in details.
export function sourceLabel(source: string, t: Dictionary) {
  if (!source) return '—'
  const key = sourceKeys[source]
  return key ? t[key] : sourceNames[source] || source
}

export function taskDate(value: string, language: string) {
  return new Date(value).toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

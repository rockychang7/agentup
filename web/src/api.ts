import { useCallback, useEffect, useRef, useState } from 'react'
import type { State } from './types'

const fragment = new URLSearchParams(location.hash.slice(1))
const launchToken = fragment.get('token')
if (launchToken) {
  sessionStorage.setItem('agentup.token', launchToken)
  history.replaceState(null, '', location.pathname + location.search)
}
const token = launchToken || sessionStorage.getItem('agentup.token')

export class APIError extends Error {}
export async function request<T>(path: string, body?: unknown): Promise<T> {
  let response: Response
  try {
    response = await fetch(`/api/${path}`, {
      method: body === undefined ? 'GET' : 'POST',
      headers: { 'Content-Type': 'application/json', 'X-AgentUp-Token': token || '' },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    })
  } catch {
    throw new APIError('network_error')
  }
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new APIError(data.error || 'request_failed')
  return data as T
}

export function useSession() {
  const [state, setState] = useState<State | null>(null)
  const [connection, setConnection] = useState<'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'stopped'>('connecting')
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const stopped = useRef(false)
  const activeEvents = useRef<EventSource | null>(null)
  const markStopped = useCallback(() => {
    stopped.current = true
    activeEvents.current?.close()
    setConnection('stopped')
    setError('')
  }, [])
  useEffect(() => {
    if (stopped.current) return
    let closed = false
    let events: EventSource | undefined
    setConnection('connecting')
    setError('')
    async function connect() {
      try {
        if (!token) throw new APIError('unauthorized')
        await request('session', { token })
        const initial = await request<State>('state')
        if (closed) return
        setState(initial)
        events = new EventSource('/api/events')
        activeEvents.current = events
        events.onopen = () => {
          if (closed || stopped.current) return
          setConnection('connected'); setError('')
        }
        events.addEventListener('shutdown', markStopped)
        events.addEventListener('state', event => {
          if (closed || stopped.current) return
          try {
            const next = JSON.parse((event as MessageEvent).data) as State
            setState(next)
            setConnection('connected')
            setError('')
          } catch { setError('request_failed') }
        })
        events.onerror = () => {
          if (!closed && !stopped.current) setConnection('reconnecting')
        }
      } catch (err) {
        if (closed || stopped.current) return
        setError(err instanceof APIError ? err.message : 'request_failed')
        setConnection('disconnected')
      }
    }
    void connect()
    return () => { closed = true; events?.close(); activeEvents.current = null }
  }, [attempt, markStopped])
  return { state, connection, sessionError: error, markStopped, reconnect: () => {
    if (!stopped.current) setAttempt(value => value + 1)
  } }
}

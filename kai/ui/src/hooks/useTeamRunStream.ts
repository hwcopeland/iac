import { useEffect, useRef, useState } from 'react'
import { useAgentActivityStore } from '../stores/agentActivityStore'
import { useUIStore } from '../stores/uiStore'
import type { RunEvent } from '../api/types'

export interface UseTeamRunStreamResult {
  connected: boolean
  error: string | null
}

/**
 * Opens a WebSocket connection to GET /api/runs/:runId/events.
 *
 * Behaviour:
 * - Server sends buffered history immediately on open, then streams live events.
 * - On every received message: parse JSON → dispatch to agentActivityStore.
 * - On close with code 4001/4003 (WS-level 401/403): set sessionExpired.
 * - On any other close: reconnect with exponential backoff (max 30 s ± 20% jitter).
 * - Cleans up on unmount (runId change).
 */
export function useTeamRunStream(runId: string): UseTeamRunStreamResult {
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Use refs for store actions so the effect dep array stays stable
  const addEvent = useAgentActivityStore((s) => s.addEvent)
  const setSessionExpired = useUIStore((s) => s.setSessionExpired)

  const addEventRef = useRef(addEvent)
  addEventRef.current = addEvent

  const setSessionExpiredRef = useRef(setSessionExpired)
  setSessionExpiredRef.current = setSessionExpired

  useEffect(() => {
    let ws: WebSocket | null = null
    let destroyed = false
    let retryCount = 0

    function connect() {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${protocol}//${window.location.host}/api/runs/${runId}/events`

      ws = new WebSocket(url)

      ws.onopen = () => {
        setConnected(true)
        setError(null)
        retryCount = 0
      }

      ws.onmessage = (e: MessageEvent<string>) => {
        try {
          const event = JSON.parse(e.data) as RunEvent
          addEventRef.current(runId, event)
        } catch {
          // Ignore malformed frames
        }
      }

      ws.onerror = () => {
        setError('WebSocket connection error')
      }

      ws.onclose = (ev) => {
        setConnected(false)
        if (destroyed) return

        // Custom close codes for auth failures (server sends these on 401/403)
        if (ev.code === 4001 || ev.code === 4003) {
          setSessionExpiredRef.current(true)
          return
        }

        // Exponential backoff: 1s → 2s → 4s … max 30s ± 20% jitter
        const base = Math.min(1_000 * Math.pow(2, retryCount), 30_000)
        const jitter = base * 0.2 * (Math.random() * 2 - 1)
        retryCount++
        setTimeout(connect, base + jitter)
      }
    }

    connect()

    return () => {
      destroyed = true
      ws?.close()
    }
  }, [runId])

  return { connected, error }
}

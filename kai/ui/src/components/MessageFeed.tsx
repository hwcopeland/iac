import { useEffect, useRef } from 'react'
import { useAgentActivityStore } from '../stores/agentActivityStore'
import { EventRow } from './EventRow'

interface MessageFeedProps {
  runId: string
}

/**
 * Scrollable message feed showing run events (newest at bottom).
 * Shows last 200 events. Full virtualisation is Phase 4.
 */
export function MessageFeed({ runId }: MessageFeedProps) {
  const events = useAgentActivityStore((s) => s.eventsByRun[runId] ?? [])
  const bottomRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Auto-scroll to bottom when new events arrive, unless user scrolled up
  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const { scrollTop, scrollHeight, clientHeight } = container
    const isNearBottom = scrollHeight - scrollTop - clientHeight < 120
    if (isNearBottom) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [events.length])

  const displayEvents = events.slice(-200)

  if (displayEvents.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-gray-500 text-sm">
        Waiting for events…
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      className="flex-1 overflow-y-auto scrollbar-thin scrollbar-thumb-gray-700 scrollbar-track-transparent"
    >
      {displayEvents.map((event, i) => (
        <EventRow key={i} event={event} />
      ))}
      <div ref={bottomRef} />
    </div>
  )
}

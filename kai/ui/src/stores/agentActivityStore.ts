import { create } from 'zustand'
import type { RunEvent } from '../api/types'

const RING_BUFFER_SIZE = 2000

export interface AgentSummary {
  status: 'idle' | 'thinking' | 'working' | 'done' | 'error'
  lastMessage: string
  taskCount: number
}

interface AgentActivityState {
  /** Live WebSocket events per run, capped at RING_BUFFER_SIZE */
  eventsByRun: Record<string, RunEvent[]>
  /** Per-agent status derived from events */
  agentSummaries: Record<string, Record<string, AgentSummary>>
  addEvent: (runId: string, event: RunEvent) => void
  clearRun: (runId: string) => void
}

function updateSummaries(
  summaries: Record<string, Record<string, AgentSummary>>,
  runId: string,
  event: RunEvent,
): Record<string, Record<string, AgentSummary>> {
  // Only agent-scoped events update per-agent summaries
  if (
    event.type !== 'agent_thinking' &&
    event.type !== 'agent_action' &&
    event.type !== 'agent_message' &&
    event.type !== 'agent_done' &&
    event.type !== 'agent_error'
  ) {
    return summaries
  }

  const agentId = event.agent_role
  const runSummaries = summaries[runId] ?? {}
  const existing: AgentSummary = runSummaries[agentId] ?? {
    status: 'idle',
    lastMessage: '',
    taskCount: 0,
  }

  let updated: AgentSummary
  switch (event.type) {
    case 'agent_thinking':
      updated = {
        ...existing,
        status: 'thinking',
        lastMessage: event.payload.thought,
        taskCount: existing.taskCount + 1,
      }
      break
    case 'agent_action':
      updated = {
        ...existing,
        status: 'working',
        lastMessage: `${event.payload.tool}`,
      }
      break
    case 'agent_message':
      updated = {
        ...existing,
        status: 'working',
        lastMessage: event.payload.content,
      }
      break
    case 'agent_done':
      updated = {
        ...existing,
        status: 'done',
        lastMessage: event.payload.summary,
      }
      break
    case 'agent_error':
      updated = {
        ...existing,
        status: 'error',
        lastMessage: event.payload.error,
      }
      break
  }

  return {
    ...summaries,
    [runId]: { ...runSummaries, [agentId]: updated },
  }
}

export const useAgentActivityStore = create<AgentActivityState>((set) => ({
  eventsByRun: {},
  agentSummaries: {},

  addEvent: (runId, event) =>
    set((state) => {
      const existing = state.eventsByRun[runId] ?? []
      const next = [...existing, event]
      // Ring buffer: drop oldest when full
      if (next.length > RING_BUFFER_SIZE) {
        next.splice(0, next.length - RING_BUFFER_SIZE)
      }
      return {
        eventsByRun: { ...state.eventsByRun, [runId]: next },
        agentSummaries: updateSummaries(state.agentSummaries, runId, event),
      }
    }),

  clearRun: (runId) =>
    set((state) => {
      const { [runId]: _evts, ...remainingEvents } = state.eventsByRun
      const { [runId]: _sums, ...remainingSummaries } = state.agentSummaries
      return {
        eventsByRun: remainingEvents,
        agentSummaries: remainingSummaries,
      }
    }),
}))

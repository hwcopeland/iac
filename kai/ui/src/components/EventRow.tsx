import type { RunEvent } from '../api/types'

function eventIcon(type: RunEvent['type']): string {
  switch (type) {
    case 'agent_thinking':
      return '🧠'
    case 'agent_action':
      return '⚡'
    case 'agent_message':
      return '💬'
    case 'agent_done':
      return '✅'
    case 'agent_error':
      return '❌'
    case 'run_complete':
      return '🎉'
    case 'run_error':
      return '🚫'
    case 'buffer_overflow':
      return '⚠️'
  }
}

function eventText(event: RunEvent): string {
  switch (event.type) {
    case 'agent_thinking':
      return event.payload.thought
    case 'agent_action':
      return `${event.payload.tool}`
    case 'agent_message':
      return event.payload.content
    case 'agent_done':
      return event.payload.summary
    case 'agent_error':
      return event.payload.error
    case 'run_complete':
      return event.payload.summary
    case 'run_error':
      return event.payload.error
    case 'buffer_overflow':
      return 'Ring buffer overflow — oldest events were dropped'
  }
}

function agentLabel(event: RunEvent): string {
  switch (event.type) {
    case 'agent_thinking':
    case 'agent_action':
    case 'agent_message':
    case 'agent_done':
    case 'agent_error':
      return event.agent_role
    default:
      return 'system'
  }
}

function typeColor(type: RunEvent['type']): string {
  switch (type) {
    case 'agent_thinking':
      return 'text-violet-400'
    case 'agent_action':
      return 'text-amber-400'
    case 'agent_message':
      return 'text-sky-400'
    case 'agent_done':
      return 'text-emerald-400'
    case 'agent_error':
    case 'run_error':
      return 'text-rose-400'
    case 'run_complete':
      return 'text-emerald-400'
    default:
      return 'text-gray-500'
  }
}

export function EventRow({ event }: { event: RunEvent }) {
  const icon = eventIcon(event.type)
  const text = eventText(event)
  const agent = agentLabel(event)
  const ts = 'timestamp' in event ? (event as { timestamp: string }).timestamp : ''
  const time = ts
    ? new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
    : ''

  return (
    <div className="flex items-start gap-2 px-4 py-2 border-b border-gray-800 hover:bg-gray-900/50 transition-colors">
      <span className="text-base leading-5 flex-shrink-0 mt-0.5">{icon}</span>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-0.5">
          <span className="text-xs font-semibold text-indigo-400 truncate max-w-[120px]">
            {agent}
          </span>
          <span className={`text-xs ${typeColor(event.type)}`}>{event.type}</span>
          {time && (
            <span className="text-xs text-gray-600 ml-auto flex-shrink-0">{time}</span>
          )}
        </div>
        <p className="text-sm text-gray-300 truncate">{text}</p>
      </div>
    </div>
  )
}

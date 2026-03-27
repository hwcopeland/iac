import type { AgentSummary } from '../stores/agentActivityStore'

interface AgentStatusCardProps {
  agentRole: string
  summary: AgentSummary
}

function statusBadge(status: AgentSummary['status']): {
  label: string
  className: string
  icon: string
} {
  switch (status) {
    case 'idle':
      return { label: 'Idle', className: 'bg-gray-800 text-gray-400', icon: '○' }
    case 'thinking':
      return { label: 'Thinking', className: 'bg-violet-900/50 text-violet-300', icon: '🧠' }
    case 'working':
      return { label: 'Working', className: 'bg-indigo-900/50 text-indigo-300', icon: '●' }
    case 'done':
      return { label: 'Done', className: 'bg-emerald-900/50 text-emerald-300', icon: '✓' }
    case 'error':
      return { label: 'Error', className: 'bg-rose-900/50 text-rose-300', icon: '✗' }
  }
}

function agentIcon(role: string): string {
  if (role.includes('planner')) return '🗂️'
  if (role.includes('researcher') || role.includes('research')) return '🔍'
  if (role.includes('reviewer') || role.includes('review')) return '✅'
  if (role.includes('coder') || role.includes('code')) return '💻'
  return '🤖'
}

export function AgentStatusCard({ agentRole, summary }: AgentStatusCardProps) {
  const badge = statusBadge(summary.status)

  return (
    <div className="rounded-lg border border-gray-800 bg-gray-900 p-3 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1.5 min-w-0">
          <span className="text-base leading-none">{agentIcon(agentRole)}</span>
          <span className="text-sm font-medium text-gray-200 truncate">{agentRole}</span>
        </div>
        <span
          className={`text-xs font-medium px-2 py-0.5 rounded-full whitespace-nowrap flex-shrink-0 ${badge.className}`}
        >
          {badge.icon} {badge.label}
        </span>
      </div>

      {summary.lastMessage && (
        <p className="text-xs text-gray-400 truncate leading-relaxed">
          {summary.lastMessage}
        </p>
      )}

      <div className="text-xs text-gray-600">
        {summary.taskCount} task{summary.taskCount !== 1 ? 's' : ''}
      </div>
    </div>
  )
}

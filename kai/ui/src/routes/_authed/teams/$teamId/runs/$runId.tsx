import { createRoute, Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient, queryOptions } from '@tanstack/react-query'
import { authedRoute } from '../../../../_authed'
import { apiFetch } from '../../../../../api/client'
import type { TeamRun } from '../../../../../api/types'
import { useTeamRunStream } from '../../../../../hooks/useTeamRunStream'
import { useAgentActivityStore } from '../../../../../stores/agentActivityStore'
import { AgentStatusCard } from '../../../../../components/AgentStatusCard'
import { MessageFeed } from '../../../../../components/MessageFeed'

export const runRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/teams/$teamId/runs/$runId',
  component: LiveRunView,
})

function runQueryOptions(runId: string) {
  return queryOptions({
    queryKey: ['runs', runId] as const,
    queryFn: () => apiFetch<TeamRun>(`/runs/${runId}`),
    staleTime: 5_000,
    refetchInterval: (query) => {
      const data = query.state.data
      if (!data) return 5_000
      return data.status === 'running' || data.status === 'pending' ? 5_000 : false
    },
  })
}

function StatusBadge({ status }: { status: TeamRun['status'] }) {
  const configs: Record<TeamRun['status'], { label: string; className: string; icon: string }> = {
    pending: {
      label: 'Pending',
      className: 'bg-gray-800 text-gray-300',
      icon: '○',
    },
    running: {
      label: 'Running',
      className: 'bg-emerald-900/50 text-emerald-300 border border-emerald-700',
      icon: '●',
    },
    completed: {
      label: 'Completed',
      className: 'bg-emerald-900/50 text-emerald-300',
      icon: '✓',
    },
    failed: {
      label: 'Failed',
      className: 'bg-rose-900/50 text-rose-300',
      icon: '✗',
    },
    cancelled: {
      label: 'Cancelled',
      className: 'bg-gray-800 text-gray-500',
      icon: '⊘',
    },
  }
  const { label, className, icon } = configs[status]
  return (
    <span className={`inline-flex items-center gap-1.5 text-sm font-medium px-3 py-1 rounded-full ${className}`}>
      {status === 'running' ? (
        <span className="animate-pulse">{icon}</span>
      ) : (
        icon
      )}{' '}
      {label}
    </span>
  )
}

function ConnectionIndicator({ connected }: { connected: boolean }) {
  return (
    <span
      className={`inline-flex items-center gap-1 text-xs ${connected ? 'text-emerald-400' : 'text-gray-500'}`}
      title={connected ? 'Live stream connected' : 'Connecting…'}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-emerald-400 animate-pulse' : 'bg-gray-600'}`} />
      {connected ? 'Live' : 'Connecting…'}
    </span>
  )
}

function LiveRunView() {
  const { teamId, runId } = runRoute.useParams()
  const qc = useQueryClient()

  const { data: run, isLoading } = useQuery(runQueryOptions(runId))
  const { connected } = useTeamRunStream(runId)
  const agentSummaries = useAgentActivityStore((s) => s.agentSummaries[runId] ?? {})

  const cancelMutation = useMutation({
    mutationFn: () =>
      apiFetch<TeamRun>(`/runs/${runId}/cancel`, { method: 'POST' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['runs', runId] })
    },
  })

  const agentIds = Object.keys(agentSummaries)

  return (
    <div className="h-full flex flex-col">
      {/* Top bar */}
      <div className="flex-shrink-0 border-b border-gray-800 bg-gray-900/50 px-6 py-4">
        {/* Breadcrumb */}
        <nav className="flex items-center gap-2 text-xs text-gray-500 mb-3">
          <Link to="/" className="hover:text-gray-300 transition-colors">
            Dashboard
          </Link>
          <span>/</span>
          <Link
            to="/teams/$teamId"
            params={{ teamId }}
            className="hover:text-gray-300 transition-colors"
          >
            Team
          </Link>
          <span>/</span>
          <span className="text-gray-400 font-mono">{runId.slice(0, 8)}…</span>
        </nav>

        <div className="flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            {isLoading ? (
              <div className="h-6 w-72 bg-gray-800 rounded animate-pulse" />
            ) : (
              <h1 className="text-lg font-semibold text-white truncate">
                {run?.objective ?? 'Loading run…'}
              </h1>
            )}
            {run && (
              <p className="text-xs text-gray-500 mt-0.5">
                Started {new Date(run.created_at).toLocaleString()}
              </p>
            )}
          </div>

          <div className="flex items-center gap-3 flex-shrink-0">
            <ConnectionIndicator connected={connected} />
            {run && <StatusBadge status={run.status} />}
            {run?.status === 'running' && (
              <button
                onClick={() => cancelMutation.mutate()}
                disabled={cancelMutation.isPending}
                className="rounded-md border border-rose-700 bg-rose-900/30 px-3 py-1.5 text-xs font-medium text-rose-300 hover:bg-rose-900/60 disabled:opacity-50 transition-colors"
              >
                {cancelMutation.isPending ? 'Cancelling…' : 'Cancel'}
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Main content: agents panel + message feed */}
      <div className="flex-1 flex overflow-hidden">
        {/* Agent status panel */}
        <div className="w-52 flex-shrink-0 border-r border-gray-800 bg-gray-900/30 overflow-y-auto">
          <div className="px-3 py-3">
            <h2 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">
              Agents
            </h2>
            {agentIds.length === 0 ? (
              <p className="text-xs text-gray-600 italic">Waiting for agents…</p>
            ) : (
              <div className="space-y-2">
                {agentIds.map((agentId) => {
                  const summary = agentSummaries[agentId]
                  if (!summary) return null
                  return (
                    <AgentStatusCard
                      key={agentId}
                      agentRole={agentId}
                      summary={summary}
                    />
                  )
                })}
              </div>
            )}
          </div>
        </div>

        {/* Activity feed */}
        <div className="flex-1 flex flex-col overflow-hidden">
          <div className="flex-shrink-0 flex items-center justify-between px-4 py-2.5 border-b border-gray-800">
            <h2 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">
              Activity Feed
            </h2>
          </div>
          <MessageFeed runId={runId} />
        </div>
      </div>
    </div>
  )
}

import { createRoute, Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient, queryOptions } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { authedRoute } from '../../../_authed'
import { apiFetch } from '../../../../api/client'
import type { Team, TeamRun } from '../../../../api/types'

export const teamIndexRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/teams/$teamId',
  component: TeamDetail,
})

function teamQueryOptions(teamId: string) {
  return queryOptions({
    queryKey: ['teams', teamId] as const,
    queryFn: () => apiFetch<Team>(`/teams/${teamId}`),
    staleTime: 60_000,
  })
}

function teamRunsQueryOptions(teamId: string) {
  return queryOptions({
    queryKey: ['teams', teamId, 'runs'] as const,
    queryFn: () => apiFetch<TeamRun[]>(`/teams/${teamId}/runs`),
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
}

function RunStatusBadge({ status }: { status: TeamRun['status'] }) {
  const configs: Record<TeamRun['status'], { icon: string; className: string; label: string }> = {
    pending: { icon: '○', className: 'bg-gray-800 text-gray-400', label: 'Pending' },
    running: { icon: '●', className: 'bg-emerald-900/50 text-emerald-300', label: 'Running' },
    completed: { icon: '✓', className: 'bg-emerald-900/50 text-emerald-300', label: 'Completed' },
    failed: { icon: '✗', className: 'bg-rose-900/50 text-rose-300', label: 'Failed' },
    cancelled: { icon: '⊘', className: 'bg-gray-800 text-gray-500', label: 'Cancelled' },
  }
  const { icon, className, label } = configs[status]
  return (
    <span className={`inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full ${className}`}>
      {icon} {label}
    </span>
  )
}

function NewRunForm({ teamId }: { teamId: string }) {
  const [objective, setObjective] = useState('')
  const [expanded, setExpanded] = useState(false)
  const navigate = useNavigate()
  const qc = useQueryClient()

  const mutation = useMutation({
    mutationFn: (obj: string) =>
      apiFetch<TeamRun>(`/teams/${teamId}/runs`, {
        method: 'POST',
        body: JSON.stringify({ objective: obj }),
      }),
    onSuccess: (run) => {
      void qc.invalidateQueries({ queryKey: ['teams', teamId, 'runs'] })
      void navigate({
        to: '/teams/$teamId/runs/$runId',
        params: { teamId, runId: run.id },
      })
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!objective.trim()) return
    mutation.mutate(objective.trim())
  }

  return (
    <div className="rounded-xl border border-gray-800 bg-gray-900 p-6">
      <h2 className="text-base font-semibold text-white mb-4">🚀 New Run</h2>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-gray-300 mb-1.5">
            Objective
          </label>
          <textarea
            value={objective}
            onChange={(e) => setObjective(e.target.value)}
            placeholder="Describe what you want the agent team to accomplish…"
            rows={4}
            className="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2.5 text-sm text-gray-100 placeholder-gray-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 resize-none transition-colors"
          />
        </div>

        {/* Advanced section toggle */}
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="text-xs text-gray-500 hover:text-gray-300 transition-colors flex items-center gap-1"
        >
          <span>{expanded ? '▾' : '▸'}</span> Advanced options
        </button>

        {expanded && (
          <div className="rounded-lg border border-gray-700 bg-gray-800/50 p-3 text-xs text-gray-500">
            Advanced configuration (model, timeout, resources) coming in a future release.
          </div>
        )}

        {mutation.isError && (
          <p className="text-xs text-rose-400">
            Failed to create run. Please try again.
          </p>
        )}

        <div className="flex items-center justify-end gap-3">
          <button
            type="button"
            onClick={() => setObjective('')}
            className="px-4 py-2 text-sm text-gray-400 hover:text-gray-200 transition-colors"
          >
            Clear
          </button>
          <button
            type="submit"
            disabled={!objective.trim() || mutation.isPending}
            className="flex items-center gap-2 rounded-lg bg-indigo-600 px-5 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {mutation.isPending ? (
              <>
                <span className="h-3.5 w-3.5 rounded-full border-2 border-white border-t-transparent animate-spin" />
                Launching…
              </>
            ) : (
              <>▶ Launch Team</>
            )}
          </button>
        </div>
      </form>
    </div>
  )
}

function TeamDetail() {
  const { teamId } = teamIndexRoute.useParams()
  const { data: team, isLoading: teamLoading } = useQuery(teamQueryOptions(teamId))
  const { data: runs, isLoading: runsLoading } = useQuery(teamRunsQueryOptions(teamId))

  return (
    <div className="h-full overflow-y-auto">
      <div className="max-w-3xl mx-auto px-6 py-8 space-y-8">
        {/* Breadcrumb */}
        <nav className="flex items-center gap-2 text-sm text-gray-500">
          <Link to="/" className="hover:text-gray-300 transition-colors">Dashboard</Link>
          <span>/</span>
          <span className="text-gray-300">
            {teamLoading ? '…' : team?.name ?? teamId}
          </span>
        </nav>

        {/* Team header */}
        <div>
          <h1 className="text-2xl font-bold text-white">
            {teamLoading ? (
              <span className="inline-block h-7 w-48 bg-gray-800 rounded animate-pulse" />
            ) : (
              team?.name ?? teamId
            )}
          </h1>
          {team?.description && (
            <p className="mt-1 text-sm text-gray-400">{team.description}</p>
          )}
        </div>

        {/* New run form */}
        <NewRunForm teamId={teamId} />

        {/* Existing runs */}
        <section>
          <h2 className="text-base font-semibold text-gray-200 mb-3">Recent Runs</h2>
          {runsLoading ? (
            <div className="space-y-2">
              {[1, 2, 3].map((i) => (
                <div key={i} className="h-16 bg-gray-800 rounded-lg animate-pulse" />
              ))}
            </div>
          ) : !runs || runs.length === 0 ? (
            <p className="text-sm text-gray-500 italic">No runs yet — start one above.</p>
          ) : (
            <div className="space-y-2">
              {runs.map((run) => (
                <Link
                  key={run.id}
                  to="/teams/$teamId/runs/$runId"
                  params={{ teamId, runId: run.id }}
                  className="flex items-center gap-3 rounded-lg border border-gray-800 bg-gray-900 px-4 py-3 hover:border-gray-700 transition-all group"
                >
                  <RunStatusBadge status={run.status} />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-gray-200 truncate group-hover:text-white">
                      {run.objective}
                    </p>
                    <p className="text-xs text-gray-500">
                      {new Date(run.created_at).toLocaleString()}
                    </p>
                  </div>
                  <span className="text-gray-600 group-hover:text-gray-400 text-sm">→</span>
                </Link>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}

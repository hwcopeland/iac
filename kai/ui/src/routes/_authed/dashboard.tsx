import { createRoute, Link } from '@tanstack/react-router'
import { useQuery, queryOptions } from '@tanstack/react-query'
import { authedRoute } from '../_authed'
import { apiFetch } from '../../api/client'
import type { Team, TeamRun } from '../../api/types'

export const dashboardRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/',
  component: Dashboard,
})

const teamsQueryOptions = queryOptions({
  queryKey: ['teams'] as const,
  queryFn: () => apiFetch<Team[]>('/teams'),
  staleTime: 60_000,
})

function useTeamRuns(teamId: string) {
  return useQuery(
    queryOptions({
      queryKey: ['teams', teamId, 'runs'] as const,
      queryFn: () => apiFetch<TeamRun[]>(`/teams/${teamId}/runs`),
      staleTime: 10_000,
      refetchInterval: 15_000,
    }),
  )
}

function RunStatusBadge({ status }: { status: TeamRun['status'] }) {
  const map: Record<TeamRun['status'], { icon: string; className: string }> = {
    pending: { icon: '○', className: 'text-gray-400' },
    running: { icon: '●', className: 'text-emerald-400 animate-pulse' },
    completed: { icon: '✓', className: 'text-emerald-400' },
    failed: { icon: '✗', className: 'text-rose-400' },
    cancelled: { icon: '⊘', className: 'text-gray-500' },
  }
  const { icon, className } = map[status]
  return <span className={`text-sm font-bold ${className}`}>{icon}</span>
}

function TeamSection({ team }: { team: Team }) {
  const { data: runs, isLoading } = useTeamRuns(team.id)

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <Link
          to="/teams/$teamId"
          params={{ teamId: team.id }}
          className="text-base font-semibold text-gray-200 hover:text-white transition-colors"
        >
          👥 {team.name}
        </Link>
        <Link
          to="/teams/$teamId"
          params={{ teamId: team.id }}
          className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
        >
          New Run →
        </Link>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {[1, 2].map((i) => (
            <div key={i} className="h-14 bg-gray-800 rounded-lg animate-pulse" />
          ))}
        </div>
      ) : !runs || runs.length === 0 ? (
        <p className="text-sm text-gray-500 italic py-2">No runs yet.</p>
      ) : (
        <div className="space-y-2">
          {runs.slice(0, 5).map((run) => (
            <RunCard key={run.id} run={run} teamId={team.id} />
          ))}
        </div>
      )}
    </section>
  )
}

function RunCard({ run, teamId }: { run: TeamRun; teamId: string }) {
  const elapsed = formatElapsed(run.created_at)

  return (
    <Link
      to="/teams/$teamId/runs/$runId"
      params={{ teamId, runId: run.id }}
      className="flex items-center gap-3 rounded-lg border border-gray-800 bg-gray-900 px-4 py-3 hover:border-gray-700 hover:bg-gray-900/80 transition-all group"
    >
      <RunStatusBadge status={run.status} />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-gray-200 truncate group-hover:text-white">
          {run.objective}
        </p>
        <p className="text-xs text-gray-500">{elapsed}</p>
      </div>
      <span className="text-gray-600 group-hover:text-gray-400 text-sm">→</span>
    </Link>
  )
}

function formatElapsed(isoDate: string): string {
  const diff = Date.now() - new Date(isoDate).getTime()
  const minutes = Math.floor(diff / 60_000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

function Dashboard() {
  const { data: teams, isLoading, isError } = useQuery(teamsQueryOptions)
  const { user } = dashboardRoute.useRouteContext()

  return (
    <div className="h-full overflow-y-auto">
      <div className="max-w-3xl mx-auto px-6 py-8 space-y-8">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-white">Dashboard</h1>
            <p className="text-sm text-gray-400 mt-0.5">
              Welcome back, {user.display_name}
            </p>
          </div>
        </div>

        {/* Teams + runs */}
        {isLoading ? (
          <div className="space-y-6">
            {[1, 2].map((i) => (
              <div key={i} className="space-y-3">
                <div className="h-5 w-40 bg-gray-800 rounded animate-pulse" />
                <div className="h-14 bg-gray-800 rounded-lg animate-pulse" />
              </div>
            ))}
          </div>
        ) : isError ? (
          <div className="rounded-lg border border-rose-800 bg-rose-900/20 px-4 py-3 text-sm text-rose-300">
            Failed to load teams. Check your connection and try again.
          </div>
        ) : !teams || teams.length === 0 ? (
          <div className="rounded-lg border border-gray-800 bg-gray-900 px-6 py-12 text-center">
            <p className="text-gray-400 mb-4">No teams yet.</p>
            <p className="text-sm text-gray-500">Teams are created by an administrator.</p>
          </div>
        ) : (
          <div className="space-y-8">
            {teams.map((team) => (
              <TeamSection key={team.id} team={team} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

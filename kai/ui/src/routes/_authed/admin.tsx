import { createRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { authedRoute } from '../_authed'
import { apiFetch } from '../../api/client'
import { useMe } from '../../hooks/useMe'
import type { AdminUser, AdminRun, TeamRun } from '../../api/types'

export const adminRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/admin',
  component: AdminPage,
})

type Tab = 'users' | 'runs'

function StatusBadge({ status }: { status: TeamRun['status'] }) {
  const configs: Record<TeamRun['status'], { label: string; className: string }> = {
    pending: {
      label: 'Pending',
      className: 'bg-gray-800 text-gray-400',
    },
    running: {
      label: 'Running',
      className: 'bg-indigo-900/50 text-indigo-300',
    },
    completed: {
      label: 'Completed',
      className: 'bg-emerald-900/50 text-emerald-300',
    },
    failed: {
      label: 'Failed',
      className: 'bg-rose-900/50 text-rose-300',
    },
    cancelled: {
      label: 'Cancelled',
      className: 'bg-yellow-900/50 text-yellow-300',
    },
  }
  const { label, className } = configs[status]
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${className}`}
    >
      {label}
    </span>
  )
}

function AdminPage() {
  const { data: me } = useMe()
  const [tab, setTab] = useState<Tab>('users')
  const [userSearch, setUserSearch] = useState('')

  const isAdmin = me?.is_admin === true

  const { data: users, isLoading: usersLoading } = useQuery({
    queryKey: ['admin-users'] as const,
    queryFn: () => apiFetch<AdminUser[]>('/admin/users'),
    enabled: isAdmin,
  })

  const { data: runs, isLoading: runsLoading } = useQuery({
    queryKey: ['admin-runs'] as const,
    queryFn: () => apiFetch<AdminRun[]>('/admin/runs'),
    enabled: isAdmin,
  })

  // Show loading skeleton while auth status is resolving
  if (me === undefined) {
    return (
      <div className="h-full overflow-y-auto">
        <div className="max-w-5xl mx-auto px-6 py-8 space-y-6">
          <div className="h-8 w-24 bg-gray-800 rounded animate-pulse" />
          <div className="h-64 bg-gray-800 rounded-lg animate-pulse" />
        </div>
      </div>
    )
  }

  if (!isAdmin) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-center space-y-3">
          <p className="text-4xl">🛡️</p>
          <p className="text-lg font-semibold text-gray-300">Access denied</p>
          <p className="text-sm text-gray-500">
            You must be an administrator to view this page.
          </p>
        </div>
      </div>
    )
  }

  const filteredUsers =
    users?.filter((u) =>
      u.email.toLowerCase().includes(userSearch.toLowerCase()),
    ) ?? []

  return (
    <div className="h-full overflow-y-auto">
      <div className="max-w-5xl mx-auto px-6 py-8 space-y-6">
        <h1 className="text-2xl font-bold text-white">Admin</h1>

        {/* Tab bar */}
        <div className="flex gap-1 border-b border-gray-800">
          {(['users', 'runs'] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-4 py-2 text-sm font-medium capitalize transition-colors border-b-2 -mb-px ${
                tab === t
                  ? 'border-indigo-500 text-indigo-300'
                  : 'border-transparent text-gray-500 hover:text-gray-300'
              }`}
            >
              {t}
            </button>
          ))}
        </div>

        {/* ── Users tab ────────────────────────────────────────────── */}
        {tab === 'users' && (
          <div className="space-y-4">
            <input
              type="text"
              value={userSearch}
              onChange={(e) => setUserSearch(e.target.value)}
              placeholder="Search by email…"
              className="w-full max-w-sm rounded-md border border-gray-700 bg-gray-900 px-3 py-2 text-sm text-gray-200 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
            />
            {usersLoading ? (
              <div className="space-y-2">
                {[1, 2, 3].map((i) => (
                  <div key={i} className="h-12 bg-gray-800 rounded animate-pulse" />
                ))}
              </div>
            ) : (
              <div className="rounded-lg border border-gray-800 overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-800 bg-gray-900/50">
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Email
                      </th>
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Name
                      </th>
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Role
                      </th>
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Created
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-800">
                    {filteredUsers.map((user) => (
                      <tr
                        key={user.id}
                        className="bg-gray-900 hover:bg-gray-900/80 transition-colors"
                      >
                        <td className="px-4 py-3 text-gray-200">{user.email}</td>
                        <td className="px-4 py-3 text-gray-400">{user.display_name}</td>
                        <td className="px-4 py-3">
                          {user.is_admin ? (
                            <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-indigo-900/50 text-indigo-300">
                              Admin
                            </span>
                          ) : (
                            <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-800 text-gray-400">
                              User
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-xs text-gray-500">
                          {new Date(user.created_at).toLocaleDateString()}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {filteredUsers.length === 0 && (
                  <p className="text-sm text-gray-500 italic px-4 py-6 text-center">
                    No users found.
                  </p>
                )}
              </div>
            )}
          </div>
        )}

        {/* ── Runs tab ─────────────────────────────────────────────── */}
        {tab === 'runs' && (
          <div className="space-y-4">
            {runsLoading ? (
              <div className="space-y-2">
                {[1, 2, 3].map((i) => (
                  <div key={i} className="h-12 bg-gray-800 rounded animate-pulse" />
                ))}
              </div>
            ) : !runs || runs.length === 0 ? (
              <p className="text-sm text-gray-500 italic py-4">No runs found.</p>
            ) : (
              <div className="rounded-lg border border-gray-800 overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-800 bg-gray-900/50">
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        ID
                      </th>
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Team
                      </th>
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Objective
                      </th>
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Status
                      </th>
                      <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                        Created
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-800">
                    {runs.map((run) => (
                      <tr
                        key={run.id}
                        className="bg-gray-900 hover:bg-gray-900/80 transition-colors"
                      >
                        <td className="px-4 py-3 font-mono text-xs text-gray-400">
                          {run.id.slice(0, 8)}…
                        </td>
                        <td className="px-4 py-3 font-mono text-xs text-gray-400">
                          {run.team_id.slice(0, 8)}…
                        </td>
                        <td className="px-4 py-3 text-gray-300 max-w-xs">
                          <span title={run.objective}>
                            {run.objective.length > 60
                              ? `${run.objective.slice(0, 60)}…`
                              : run.objective}
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <StatusBadge status={run.status} />
                        </td>
                        <td className="px-4 py-3 text-xs text-gray-500">
                          {new Date(run.created_at).toLocaleDateString()}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

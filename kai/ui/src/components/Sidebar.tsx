import { Link, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { queryOptions } from '@tanstack/react-query'
import { apiFetch } from '../api/client'
import type { Team } from '../api/types'
import { useUIStore } from '../stores/uiStore'
import { useMe } from '../hooks/useMe'

const teamsQueryOptions = queryOptions({
  queryKey: ['teams'] as const,
  queryFn: () => apiFetch<Team[]>('/teams'),
  staleTime: 60_000,
})

export function Sidebar() {
  const toggleSidebar = useUIStore((s) => s.toggleSidebar)
  const { data: me } = useMe()
  const { data: teams } = useQuery(teamsQueryOptions)
  const routerState = useRouterState()
  const pathname = routerState.location.pathname

  return (
    <aside className="w-56 bg-gray-900 border-r border-gray-800 flex flex-col h-full flex-shrink-0">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-4 border-b border-gray-800">
        <Link to="/" className="flex items-center gap-2 text-gray-100 hover:text-white">
          <span className="text-lg font-bold tracking-tight">⚡ Kai</span>
        </Link>
        <button
          onClick={toggleSidebar}
          className="text-gray-500 hover:text-gray-300 p-1 rounded transition-colors"
          aria-label="Close sidebar"
        >
          ✕
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto px-2 py-3 space-y-1">
        <NavItem href="/" label="Dashboard" icon="🏠" active={pathname === '/'} />

        {/* Teams */}
        {teams && teams.length > 0 && (
          <div className="pt-3">
            <p className="px-2 pb-1 text-xs font-semibold text-gray-500 uppercase tracking-wider">
              Teams
            </p>
            {teams.map((team) => (
              <NavItem
                key={team.id}
                href={`/teams/${team.id}`}
                label={team.name}
                icon="👥"
                active={pathname.startsWith(`/teams/${team.id}`)}
              />
            ))}
          </div>
        )}

        {/* Account */}
        <div className="pt-3">
          <p className="px-2 pb-1 text-xs font-semibold text-gray-500 uppercase tracking-wider">
            Account
          </p>
          <NavItem href="/settings" label="Settings" icon="⚙️" active={pathname === '/settings'} />
          {me?.is_admin === true && (
            <NavItem href="/admin" label="Admin" icon="🛡️" active={pathname === '/admin'} />
          )}
        </div>
      </nav>

      {/* User info */}
      <div className="px-4 py-3 border-t border-gray-800">
        {me ? (
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-full bg-indigo-600 flex items-center justify-center text-xs font-bold text-white flex-shrink-0">
              {me.display_name.charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0">
              <p className="text-xs font-medium text-gray-200 truncate">{me.display_name}</p>
              <p className="text-xs text-gray-500 truncate">{me.email}</p>
            </div>
          </div>
        ) : (
          <div className="h-8 w-full bg-gray-800 rounded animate-pulse" />
        )}
      </div>
    </aside>
  )
}

function NavItem({
  href,
  label,
  icon,
  active,
}: {
  href: string
  label: string
  icon: string
  active: boolean
}) {
  return (
    <Link
      to={href}
      className={`flex items-center gap-2 px-3 py-2 rounded-md text-sm transition-colors ${
        active
          ? 'bg-indigo-900/50 text-indigo-300 font-medium'
          : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800'
      }`}
    >
      <span className="text-base leading-none">{icon}</span>
      <span className="truncate">{label}</span>
    </Link>
  )
}

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import type { User } from '../api/types'

/**
 * Query options for GET /api/me — the single source of auth truth.
 *
 * Uses a raw fetch instead of apiFetch so a 401 (not logged in yet) does NOT
 * trigger the sessionExpired banner. The route guard in _authed.tsx handles
 * the unauthenticated case by redirecting to /login.
 */
export const meQueryOptions = queryOptions({
  queryKey: ['me'] as const,
  queryFn: async (): Promise<User> => {
    const res = await fetch('/api/me', { credentials: 'include' })
    if (!res.ok) {
      throw new ApiError(res.status, 'Not authenticated')
    }
    return res.json() as Promise<User>
  },
  staleTime: 5 * 60 * 1000,
  retry: false,
})

export function useMe() {
  return useQuery(meQueryOptions)
}

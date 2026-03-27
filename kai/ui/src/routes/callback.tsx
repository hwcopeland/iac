import { createRoute, useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { rootRoute } from './__root'
import { useMe } from '../hooks/useMe'

export const callbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/callback',
  component: CallbackPage,
})

/**
 * OAuth callback landing page.
 *
 * After the backend exchanges the auth code and sets the session cookie,
 * it redirects here. We verify auth with GET /api/me and then navigate
 * to the dashboard. If auth failed, redirect to /login.
 */
function CallbackPage() {
  const navigate = useNavigate()
  const { data: me, isLoading, isError } = useMe()

  useEffect(() => {
    if (isLoading) return
    if (isError || !me) {
      void navigate({ to: '/login' })
      return
    }
    void navigate({ to: '/' })
  }, [me, isLoading, isError, navigate])

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-950">
      <div className="flex flex-col items-center gap-4">
        <div className="h-10 w-10 rounded-full border-2 border-indigo-500 border-t-transparent animate-spin" />
        <p className="text-sm text-gray-400">Completing sign-in…</p>
      </div>
    </div>
  )
}

import { createRoute, Outlet, redirect } from '@tanstack/react-router'
import { rootRoute } from './__root'
import { meQueryOptions } from '../hooks/useMe'
import { Layout } from '../components/Layout'
import { SessionExpiredBanner } from '../components/SessionExpiredBanner'

/**
 * Pathless layout route — wraps all authenticated pages.
 *
 * The beforeLoad guard runs BEFORE any child route component renders.
 * A 401 from GET /api/me causes an immediate redirect to /login with
 * no flash of protected UI — this is structurally impossible to bypass.
 */
export const authedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: '_authed',
  beforeLoad: async ({ context }) => {
    try {
      const user = await context.queryClient.ensureQueryData(meQueryOptions)
      if (!user) {
        throw redirect({ to: '/login' })
      }
      return { user }
    } catch (err) {
      // If it's our redirect, re-throw it
      if (err instanceof Error && err.message === 'Redirect') throw err
      // Any other error (including 401 ApiError) → go to login
      throw redirect({ to: '/login' })
    }
  },
  component: AuthedLayout,
})

function AuthedLayout() {
  return (
    <Layout>
      <SessionExpiredBanner />
      <Outlet />
    </Layout>
  )
}

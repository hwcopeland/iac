/**
 * Route tree for Kai SPA.
 *
 * Manually authored in lieu of TanStack Router CLI codegen.
 * Keep in sync with routes/ directory structure.
 *
 * Route hierarchy:
 *   / (root)
 *   ├── /login
 *   ├── /callback
 *   └── _authed (pathless layout — auth guard)
 *       ├── /                            → dashboard
 *       ├── /settings                    → settings page
 *       ├── /admin                       → admin page (is_admin guard)
 *       ├── /teams/:teamId               → team detail + new run form
 *       ├── /teams/:teamId/runs/:runId   → live run view
 *       └── /teams/:teamId/runs/:runId/results → run results / artifacts
 */

import { rootRoute } from './routes/__root'
import { loginRoute } from './routes/login'
import { callbackRoute } from './routes/callback'
import { authedRoute } from './routes/_authed'
import { dashboardRoute } from './routes/_authed/dashboard'
import { teamIndexRoute } from './routes/_authed/teams/$teamId/index'
import { runRoute } from './routes/_authed/teams/$teamId/runs/$runId'
import { resultsRoute } from './routes/_authed/teams/$teamId/runs/$runId/results'
import { settingsRoute } from './routes/_authed/settings'
import { adminRoute } from './routes/_authed/admin'

const authedRouteTree = authedRoute.addChildren([
  dashboardRoute,
  teamIndexRoute,
  runRoute,
  resultsRoute,
  settingsRoute,
  adminRoute,
])

export const routeTree = rootRoute.addChildren([
  loginRoute,
  callbackRoute,
  authedRouteTree,
])

// Re-export route instances so main.tsx can register types
export type { RouterContext } from './routes/__root'


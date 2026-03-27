import { useUIStore } from '../stores/uiStore'

/**
 * Shown when any API call returns 401 mid-session (session cookie expired).
 * Renders as a fixed top banner with a re-login link.
 * NOT shown on initial page load / login redirect (handled by route guard).
 */
export function SessionExpiredBanner() {
  const sessionExpired = useUIStore((s) => s.sessionExpired)

  if (!sessionExpired) return null

  return (
    <div
      role="alert"
      className="fixed top-0 inset-x-0 z-50 flex items-center justify-between gap-4 bg-rose-900/90 backdrop-blur-sm border-b border-rose-700 px-4 py-3 text-sm text-rose-100"
    >
      <div className="flex items-center gap-2">
        <span>⚠️</span>
        <span>Your session has expired. Please sign in again to continue.</span>
      </div>
      <a
        href="/api/auth/login"
        className="flex-shrink-0 rounded-md bg-rose-700 hover:bg-rose-600 px-3 py-1.5 text-xs font-medium text-white transition-colors"
      >
        Sign in again
      </a>
    </div>
  )
}

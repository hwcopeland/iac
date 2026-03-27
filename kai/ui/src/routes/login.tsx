import { createRoute } from '@tanstack/react-router'
import { rootRoute } from './__root'

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  component: LoginPage,
})

function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-950">
      <div className="w-full max-w-sm rounded-xl border border-gray-800 bg-gray-900 p-8 text-center shadow-xl">
        <div className="mx-auto mb-6 flex h-14 w-14 items-center justify-center rounded-xl bg-indigo-600 text-2xl">
          ⚡
        </div>
        <h1 className="mb-1 text-xl font-semibold text-white">Welcome to Kai</h1>
        <p className="mb-8 text-sm text-gray-400">AI agent teams for development &amp; research</p>
        <a
          href="/api/auth/login"
          className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-indigo-600 px-6 py-3 text-sm font-medium text-white shadow-sm transition-colors hover:bg-indigo-500"
        >
          <span>🔑</span>
          Sign in with Authentik
        </a>
      </div>
    </div>
  )
}

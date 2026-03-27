---
project: "kai"
maturity: "draft"
last_updated: "2025-07-14"
updated_by: "@staff-engineer"
scope: "React/TypeScript SPA for Kai: routing, auth flow, WebSocket live-run view,
  state management, bundle strategy, and Go binary embedding."
owner: "@staff-engineer"
dependencies:
  - docs/prd/kai.md
  - docs/tdd/kai-backend.md
  - docs/tdd/kai-deploy.md
---

# TDD: Kai Frontend (React/TypeScript SPA)

## 1. Problem Statement

### 1.1 What and Why Now

Kai needs a browser UI that lets users start agent team runs, watch them execute in
real-time, and retrieve results. The UI is a single-page application embedded in the
Go binary — users hit `https://kai.hwcopeland.net`, get the SPA, and everything
runs from there.

The most important design constraint is **auth correctness**. OpenHands on this cluster
is broken because its SPA stores auth state (`LOGIN_METHOD`) in localStorage and uses
`window.location.href` redirects based on that flag — producing an infinite redirect loop
when the Keycloak token is stale. Kai's frontend is designed so this entire class of bug
is structurally impossible.

### 1.2 Non-Goals (Phase 1)

- Mobile-optimized layout (responsive enough to monitor, not optimized)
- Offline support / service workers
- Rich text / WYSIWYG objective editor
- Per-agent log download (Phase 2)

---

## 2. Technology Choices

### 2.1 Router: TanStack Router

**Why**: File-based routes with full TypeScript inference. The `beforeLoad` hook runs
*before* the route component renders — auth guards happen at the router level, not
inside `useEffect`. This eliminates the "flash before redirect" problem.

**Rejected**: React Router v6 — `loader` functions require Remix-style data patterns
or manual `useEffect` guards that race with render.

### 2.2 Data Fetching: TanStack Query

Server state (REST responses) lives entirely in TanStack Query's cache. No custom
fetch wrapper, no Redux for server data. The `meQuery` is the auth source of truth —
a `401` from `GET /api/me` means the user is not authenticated.

### 2.3 Client State: Zustand

Two stores:
- `agentActivityStore` — live WebSocket events and per-agent summaries for active runs
- `uiStore` — sidebar open/closed, modal state, session-expired flag

No Redux, no Context for global state. Zustand is simpler and avoids Context re-render storms.

### 2.4 Styling: Tailwind CSS v4

Dark theme by default (matching Authentik, Grafana aesthetic on this cluster).
Component classes composed with `clsx` + `tailwind-merge`.

### 2.5 Build: Vite

Fast HMR in dev. Output goes to `ui/dist/` which is embedded in the Go binary via
`embed.FS`. The CI pipeline runs `npm run build` before `go build`.

### 2.6 Testing: Vitest + Playwright

- Unit: Vitest + React Testing Library for component logic
- E2E: Playwright against a local dev server with mocked API

---

## 3. Auth Design (Critical)

### 3.1 The Rule

**The SPA never stores tokens, auth flags, or user identity in localStorage or sessionStorage.**

Auth state is a single server query: `GET /api/me`. The response is either:
- `200 { id, email, display_name, is_admin }` — authenticated
- `401 { error: "unauthorized" }` — not authenticated

The session cookie (`kai_session`, HTTP-only, SameSite=Strict) is managed entirely
by the browser and the Go backend. The SPA cannot read or write it.

### 3.2 Route Guard via `beforeLoad`

```typescript
// src/routes/_authed.tsx  (layout route wrapping all protected routes)

export const Route = createFileRoute('/_authed')({
  beforeLoad: async ({ context }) => {
    const user = await context.queryClient.ensureQueryData(meQuery)
    // ensureQueryData throws on 401 — TanStack Router catches and redirects
    if (!user) {
      throw redirect({ to: '/login' })
    }
    return { user }
  },
})
```

`ensureQueryData` either returns cached data or fetches. A `401` response causes the
query to throw, which TanStack Router intercepts and redirects to `/login` — **before
a single pixel of protected UI renders**.

### 3.3 Login Flow

```typescript
// src/routes/login.tsx

function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-950">
      <div className="rounded-xl border border-neutral-800 bg-neutral-900 p-8 text-center">
        <KaiLogo className="mx-auto mb-6 h-12 w-12" />
        <h1 className="mb-2 text-xl font-semibold text-white">Welcome to Kai</h1>
        <p className="mb-6 text-sm text-neutral-400">AI agent teams for development & research</p>
        <a
          href="/api/auth/login"
          className="inline-flex items-center gap-2 rounded-lg bg-violet-600 px-6 py-3 text-sm font-medium text-white hover:bg-violet-500"
        >
          Sign in with Authentik
        </a>
      </div>
    </div>
  )
}
```

Login is a plain `<a href="/api/auth/login">`. No JavaScript navigation. The browser
follows the redirect chain: `/api/auth/login` → Authentik → `/api/auth/callback` →
backend sets cookie → `302 /` → SPA loads → `GET /api/me` succeeds.

### 3.4 Session Expiry Handling

The `SessionExpiredBanner` component watches TanStack Query's global error handler:

```typescript
// src/lib/query-client.ts

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        if (isUnauthorized(error)) return false  // don't retry 401s
        return failureCount < 3
      },
    },
  },
})

// Global mutation error handler
queryClient.setMutationDefaults([], {
  onError: (error) => {
    if (isUnauthorized(error)) {
      useUIStore.getState().setSessionExpired(true)
    }
  },
})
```

When `sessionExpired` is true, the UI overlays a modal: "Your session has expired.
Sign in again." — with a link to `/api/auth/login`. No redirect loop possible.

---

## 4. Route Structure

```
src/routes/
├── __root.tsx              # Root layout: QueryClientProvider, TooltipProvider
├── login.tsx               # /login — unauthenticated landing
├── _authed.tsx             # Layout route: beforeLoad auth guard
├── _authed/
│   ├── index.tsx           # / — dashboard
│   ├── runs/
│   │   ├── new.tsx         # /runs/new — start run form
│   │   ├── $runId/
│   │   │   ├── index.tsx   # /runs/$runId — live run view
│   │   │   └── results.tsx # /runs/$runId/results — completed run artifacts
│   ├── settings.tsx        # /settings — API keys, account
│   └── admin.tsx           # /admin — admin only (403 if not kai-admins)
```

---

## 5. Page Designs

### 5.1 Dashboard (`/`)

**Purpose**: Entry point after login. Shows recent runs and a prominent "Start New Run" CTA.

```
┌─────────────────────────────────────────────────────┐
│ ≡  Kai          [New Run ▶]                [avatar] │
├──────────┬──────────────────────────────────────────┤
│          │  Recent Runs                             │
│ Dashboard│  ┌────────────────────────────────────┐  │
│ Runs     │  │ ● Refactor auth module    2m ago   │  │
│ Settings │  │   coder×2, reviewer                │  │
│          │  ├────────────────────────────────────┤  │
│ (Admin)  │  │ ✓ Add unit tests for API  1h ago   │  │
│          │  │   completed in 14m                 │  │
│          │  ├────────────────────────────────────┤  │
│          │  │ ✗ Migrate DB schema       3h ago   │  │
│          │  │   failed: timeout                  │  │
│          │  └────────────────────────────────────┘  │
└──────────┴──────────────────────────────────────────┘
```

Data: `GET /api/teams/:id/runs?limit=20` via TanStack Query.

### 5.2 New Run (`/runs/new`)

**Purpose**: Launch a team run with an objective.

```
┌─────────────────────────────────────────────────────┐
│ ≡  Kai                                   [avatar]  │
├──────────┬──────────────────────────────────────────┤
│          │  New Run                                 │
│          │                                          │
│          │  Objective                               │
│          │  ┌──────────────────────────────────┐   │
│          │  │ Describe what you want the team  │   │
│          │  │ to accomplish...                 │   │
│          │  │                                  │   │
│          │  └──────────────────────────────────┘   │
│          │                                          │
│          │  Model    [claude-sonnet-4.5    ▾]       │
│          │                                          │
│          │  ▸ Advanced (timeout, resources)         │
│          │                                          │
│          │              [Cancel]  [Launch Team ▶]  │
└──────────┴──────────────────────────────────────────┘
```

On submit: `POST /api/teams/:id/runs` → redirect to `/runs/:runId`.

### 5.3 Live Run View (`/runs/:runId`)

**The core screen.** Real-time agent activity via WebSocket.

```
┌─────────────────────────────────────────────────────┐
│ ≡  Kai  / Runs / run-abc123              [avatar]  │
├──────────┬──────────────────────────────────────────┤
│          │ Refactor auth module          ● Running  │
│          │ Started 2m ago                           │
│          ├──────────────────────────────────────────┤
│          │ AGENTS                  ACTIVITY FEED    │
│          │ ┌──────────┐  ┌─────────────────────┐   │
│          │ │🧠 Planner│  │ planner  thinking    │   │
│          │ │ ✓ Done   │  │ "Analyzing codebase" │   │
│          │ ├──────────┤  ├─────────────────────┤   │
│          │ │🔍 Resrch │  │ researcher  action   │   │
│          │ │ ● Working│  │ read_file auth.go    │   │
│          │ ├──────────┤  ├─────────────────────┤   │
│          │ │💻 Coder 1│  │ coder_1  action      │   │
│          │ │ ○ Waiting│  │ write_file auth.go   │   │
│          │ ├──────────┤  ├─────────────────────┤   │
│          │ │💻 Coder 2│  │ coder_2  message     │   │
│          │ │ ○ Waiting│  │ "Ready for tasks"    │   │
│          │ ├──────────┤  └─────────────────────┘   │
│          │ │✅ Reviewer│   ↕ virtual scroll         │
│          │ │ ○ Waiting│                             │
│          │ └──────────┘                             │
└──────────┴──────────────────────────────────────────┘
```

### 5.4 Results (`/runs/:runId/results`)

```
┌─────────────────────────────────────────────────────┐
│  Run Completed ✓   14m 32s   ~42k tokens            │
│                                                      │
│  Summary                                             │
│  Refactored auth module into PKCE-only flow...       │
│                                                      │
│  Artifacts                                           │
│  ┌─────────────────────────────────────────────┐    │
│  │ 📄 auth.go              12KB  [Download]    │    │
│  │ 📄 auth_test.go          8KB  [Download]    │    │
│  │ 📄 CHANGES.md            2KB  [Download]    │    │
│  └─────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

---

## 6. WebSocket Integration

### 6.1 Hook

```typescript
// src/hooks/useTeamRunStream.ts

export function useTeamRunStream(runId: string) {
  const addEvent = useAgentActivityStore(s => s.addEvent)

  useEffect(() => {
    let ws: WebSocket
    let retryCount = 0
    let closed = false

    function connect() {
      ws = new WebSocket(`wss://kai.hwcopeland.net/api/runs/${runId}/events`)

      ws.onmessage = (e) => {
        const event: RunEvent = JSON.parse(e.data)
        addEvent(runId, event)
        retryCount = 0  // reset backoff on successful message
      }

      ws.onclose = () => {
        if (closed) return
        // Exponential backoff: 1s, 2s, 4s, 8s, ... max 30s ±20% jitter
        const base = Math.min(1000 * Math.pow(2, retryCount), 30_000)
        const jitter = base * 0.2 * (Math.random() * 2 - 1)
        retryCount++
        setTimeout(connect, base + jitter)
      }
    }

    connect()
    return () => {
      closed = true
      ws?.close()
    }
  }, [runId, addEvent])
}
```

### 6.2 Event Types (TypeScript discriminated union)

```typescript
// src/lib/events.ts

export type RunEvent =
  | { type: 'agent_thinking';  agent_task_id: string; agent_role: AgentRole; timestamp: string; payload: { thought: string } }
  | { type: 'agent_action';    agent_task_id: string; agent_role: AgentRole; timestamp: string; payload: { tool: string; input: unknown; output?: unknown } }
  | { type: 'agent_message';   agent_task_id: string; agent_role: AgentRole; timestamp: string; payload: { content: string; to?: string } }
  | { type: 'agent_done';      agent_task_id: string; agent_role: AgentRole; timestamp: string; payload: { summary: string; artifact_ids?: string[] } }
  | { type: 'agent_error';     agent_task_id: string; agent_role: AgentRole; timestamp: string; payload: { error: string } }
  | { type: 'run_complete';    run_id: string;         timestamp: string; payload: { summary: string; artifact_ids: string[]; duration_ms: number } }
  | { type: 'run_error';       run_id: string;         timestamp: string; payload: { error: string } }
  | { type: 'buffer_overflow'; timestamp: string }

export type AgentRole = 'planner' | 'researcher' | 'coder_1' | 'coder_2' | 'reviewer'
```

### 6.3 Zustand Store (Ring Buffer)

```typescript
// src/stores/agentActivityStore.ts

const RING_BUFFER_SIZE = 2000

interface AgentActivityState {
  eventsByRun: Record<string, RunEvent[]>  // capped at RING_BUFFER_SIZE
  agentStatus: Record<string, Record<AgentRole, AgentStatus>>
  addEvent: (runId: string, event: RunEvent) => void
}

export const useAgentActivityStore = create<AgentActivityState>((set) => ({
  eventsByRun: {},
  agentStatus: {},
  addEvent: (runId, event) => set((state) => {
    const existing = state.eventsByRun[runId] ?? []
    const updated = [...existing, event]
    // Ring buffer: evict oldest when full
    if (updated.length > RING_BUFFER_SIZE) updated.splice(0, updated.length - RING_BUFFER_SIZE)
    return {
      eventsByRun: { ...state.eventsByRun, [runId]: updated },
      agentStatus: updateAgentStatus(state.agentStatus, runId, event),
    }
  }),
}))
```

### 6.4 Virtual Scroll (MessageFeed)

2000 events × ~60px row height = 120,000px of DOM without virtualization — unusable.

```typescript
// src/components/MessageFeed.tsx

import { useVirtualizer } from '@tanstack/react-virtual'

export function MessageFeed({ runId }: { runId: string }) {
  const events = useAgentActivityStore(s => s.eventsByRun[runId] ?? [])
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 10,
  })

  // Auto-scroll to bottom on new events
  useEffect(() => {
    virtualizer.scrollToIndex(events.length - 1, { behavior: 'smooth' })
  }, [events.length])

  return (
    <div ref={parentRef} className="h-full overflow-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((item) => (
          <div
            key={item.key}
            style={{ position: 'absolute', top: item.start, width: '100%' }}
          >
            <EventRow event={events[item.index]} />
          </div>
        ))}
      </div>
    </div>
  )
}
```

---

## 7. State Management Summary

| Layer | What lives there | Why |
|-------|-----------------|-----|
| TanStack Query | REST: runs list, run detail, user, teams, artifacts | Server state with cache/revalidation |
| Zustand `agentActivityStore` | WS event stream, agent status per run | High-frequency updates, no re-render tax |
| Zustand `uiStore` | Sidebar, modals, session-expired flag | Global UI state |
| URL search params | Pagination, filters | Bookmarkable, shareable |
| React `useState` | Form inputs, local toggles | Ephemeral only |
| `localStorage` | **Nothing auth-related** | Prevents the OpenHands loop |

---

## 8. Bundle Strategy

**Target**: ≤200 KB gzipped initial load.

```typescript
// vite.config.ts
export default defineConfig({
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-react':  ['react', 'react-dom'],
          'vendor-router': ['@tanstack/react-router'],
          'vendor-query':  ['@tanstack/react-query'],
          'vendor-zustand': ['zustand'],
        },
      },
    },
  },
})
```

Markdown renderer (`react-markdown` + `highlight.js`, ~120 KB) is lazy-loaded only
on the Results page:

```typescript
const MarkdownRenderer = lazy(() => import('../components/MarkdownRenderer'))
```

---

## 9. Go Embedding

```
kai/
└── cmd/kai/
    ├── main.go
    └── ui/           ← symlink or copy of ../ui/dist after build
        └── dist/
            ├── index.html
            └── assets/
```

```go
//go:embed ui/dist
var staticFS embed.FS
```

CI order: `npm run build` → copies to `cmd/kai/ui/dist` → `go build ./cmd/kai/`.

---

## 10. Open Questions

### OQ-1: WS Cookie on Upgrade

WebSocket `Upgrade` requests send cookies automatically in browsers. Verify that the
Cilium Gateway API (`hwcopeland-gateway`) proxies `Cookie` headers on WS upgrades.
If not, a short-lived WS ticket endpoint (`GET /api/runs/:id/ws-ticket`) returning a
one-time token in the response body may be needed.

### OQ-2: Artifact Download URLs

Are artifacts served directly from `GET /api/artifacts/:id` (streamed from Go backend),
or are they pre-signed Longhorn/S3 URLs? Affects the download link implementation.

### OQ-3: Admin User Sync

Does `GET /api/admin/users` return users from the Kai PostgreSQL DB (synced on login)
or proxied from the Authentik API? Determines whether admin sees only users who have
logged in, or all Authentik users.

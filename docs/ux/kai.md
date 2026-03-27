---
project: "kai"
maturity: "draft"
last_updated: "2025-07-14"
updated_by: "@ux-designer"
scope: "UX design spec for Kai: all 7 pages, component inventory, live run view deep-dive,
  color/typography system, interaction patterns."
owner: "@ux-designer"
dependencies:
  - docs/prd/kai.md
  - docs/tdd/kai-frontend.md
---

# UX Design Spec: Kai Platform

## 1. Design Principles

1. **Information density over decoration** — developers want data, not marketing
2. **Live state is the core** — the run view is the most important screen; everything else supports it
3. **No auth complexity** — login is one button; auth confusion (the OpenHands experience) must be impossible
4. **Dark by default** — consistent with the rest of the cluster UI (Authentik, Grafana)
5. **Keyboard-first** — power users should be able to navigate without leaving the keyboard

---

## 2. Color & Typography System

### 2.1 Palette (Tailwind CSS v4 tokens)

```
Background layers:
  bg-base:       neutral-950  (#0a0a0a)   -- page background
  bg-surface:    neutral-900  (#171717)   -- cards, panels
  bg-elevated:   neutral-800  (#262626)   -- hover states, dropdowns
  bg-border:     neutral-700  (#404040)   -- dividers, input borders

Text:
  text-primary:  neutral-50   (#fafafa)   -- headings, primary content
  text-secondary:neutral-400  (#a3a3a3)   -- labels, secondary info
  text-muted:    neutral-600  (#525252)   -- timestamps, metadata

Accent:
  accent:        violet-500   (#8b5cf6)   -- primary CTAs, active states
  accent-hover:  violet-400   (#a78bfa)
  accent-muted:  violet-900   (#2e1065)   -- accent backgrounds

Status:
  success:       emerald-500  (#10b981)
  warning:       amber-500    (#f59e0b)
  error:         red-500      (#ef4444)
  running:       blue-400     (#60a5fa)   -- pulsing dot for active runs
  pending:       neutral-500  (#737373)
```

### 2.2 Typography

```
Font: System stack — Inter → -apple-system → BlinkMacSystemFont → sans-serif
Mono: JetBrains Mono → Fira Code → ui-monospace → monospace (agent output, code)

Scale:
  text-xs:   11px  -- timestamps, metadata tags
  text-sm:   13px  -- body copy, labels, table rows
  text-base: 15px  -- default body
  text-lg:   18px  -- card titles
  text-xl:   22px  -- page headings
  text-2xl:  28px  -- hero/empty state text
```

---

## 3. Layout Shell

```
┌─────────────────────────────────────────────────────────────┐
│ SIDEBAR (240px, collapsible)  │  MAIN CONTENT               │
│                               │                             │
│  ╔═══╗  Kai                   │  [page content]             │
│  ║ K ║                        │                             │
│                               │                             │
│  ─────────────────────        │                             │
│  ⊞  Dashboard                 │                             │
│  ▶  Runs                      │                             │
│     ├ Active (2)              │                             │
│     └ Recent                  │                             │
│  ⚙  Settings                  │                             │
│                               │                             │
│  ─────────────────────        │                             │
│  [admin only]                 │                             │
│  ⊛  Admin                     │                             │
│                               │                             │
│  ─────────────────────        │                             │
│  ● hampton888@gmail.com       │                             │
│  [Sign out]                   │                             │
└───────────────────────────────┴─────────────────────────────┘
```

**Sidebar behavior**:
- Expanded (240px) by default on desktop
- Collapses to icon-only (56px) at viewport < 1024px or via toggle button
- State persisted in `uiStore` (Zustand), not localStorage
- Mobile (< 768px): sidebar is a drawer, closed by default

**Header**: None — sidebar handles identity and navigation. Saves vertical space for content.

---

## 4. Page Designs

### 4.1 Login (`/login`)

Single-purpose page. No sidebar. No navigation.

```
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│                                                             │
│                      ╔═══════╗                             │
│                      ║  KAI  ║                             │
│                      ╚═══════╝                             │
│                                                             │
│                       Kai                                   │
│                 AI agent teams for                          │
│               development & research                        │
│                                                             │
│            ┌────────────────────────────┐                  │
│            │  Sign in with Authentik  → │                  │
│            └────────────────────────────┘                  │
│                    (violet-500 button)                      │
│                                                             │
│                                                             │
│              kai.hwcopeland.net · v0.1.0                    │
└─────────────────────────────────────────────────────────────┘
```

**States**:
- Default: button enabled
- No loading state — clicking `<a href="/api/auth/login">` navigates away immediately
- Error state (e.g. `?error=auth_failed` in URL): red banner above button —
  "Authentication failed. Please try again."

**Component**: `LoginPage` — no sub-components needed

---

### 4.2 Dashboard (`/`)

```
┌─────────────────────────────────────────────────────────────┐
│ SIDEBAR │                                                   │
│         │  Good morning, Hampton         [+ New Run]        │
│         │                                                   │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │  ACTIVE RUNS (2)                            │ │
│         │  ├─────────────────────────────────────────────┤ │
│         │  │  ● Refactor auth module           2m        │ │
│         │  │    planner ✓  researcher ●  coder ○  rev ○  │ │
│         │  │                                  [View →]   │ │
│         │  ├─────────────────────────────────────────────┤ │
│         │  │  ● Add OpenAPI documentation      8m        │ │
│         │  │    planner ✓  researcher ✓  coder ●  rev ○  │ │
│         │  │                                  [View →]   │ │
│         │  └─────────────────────────────────────────────┘ │
│         │                                                   │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │  RECENT RUNS                                │ │
│         │  ├─────────────────────────────────────────────┤ │
│         │  │  ✓ Fix Longhorn PVC permissions   1h ago    │ │
│         │  │    completed in 6m 14s            [Results] │ │
│         │  ├─────────────────────────────────────────────┤ │
│         │  │  ✗ Migrate PostgreSQL schema      3h ago    │ │
│         │  │    failed: coder timeout          [Details] │ │
│         │  ├─────────────────────────────────────────────┤ │
│         │  │  ✓ Write k8s NetworkPolicy TDD    5h ago    │ │
│         │  │    completed in 22m 8s            [Results] │ │
│         │  └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

**Components**:
- `ActiveRunCard` — shows agent pipeline progress inline (icons: ✓ done, ● active, ○ waiting)
- `RecentRunRow` — compact list item with status icon, title, time, action link
- `NewRunButton` — violet CTA, links to `/runs/new`

**Data**: `GET /api/teams/:id/runs?status=running` + `GET /api/teams/:id/runs?limit=10`
Both requests fire in parallel via TanStack Query.

**Empty state** (no runs yet):
```
  ╔═══════════════════════════════════════╗
  ║                                       ║
  ║    No runs yet.                       ║
  ║    Start your first agent team run.   ║
  ║                                       ║
  ║         [+ Launch First Run]          ║
  ║                                       ║
  ╚═══════════════════════════════════════╝
```

---

### 4.3 New Run (`/runs/new`)

```
┌─────────────────────────────────────────────────────────────┐
│ SIDEBAR │  New Run                                          │
│         │                                                   │
│         │  Objective                                        │
│         │  ┌────────────────────────────────────────────┐  │
│         │  │                                            │  │
│         │  │  Describe what you want the agent team     │  │
│         │  │  to accomplish. Be specific — include      │  │
│         │  │  file paths, constraints, and context.     │  │
│         │  │                                            │  │
│         │  │  e.g. "Refactor internal/auth/pkce.go to   │  │
│         │  │  use the new OIDCClient interface..."      │  │
│         │  │                                            │  │
│         │  └────────────────────────────────────────────┘  │
│         │  0 / 4000 characters                             │
│         │                                                   │
│         │  Model                                           │
│         │  ┌──────────────────────────┐                    │
│         │  │ claude-sonnet-4.5      ▾ │                    │
│         │  └──────────────────────────┘                    │
│         │                                                   │
│         │  ▸ Advanced options                              │
│         │    (timeout: 30min, resources: standard)         │
│         │                                                   │
│         │                    [Cancel]  [Launch Team ▶]     │
└─────────────────────────────────────────────────────────────┘
```

**Interactions**:
- Objective textarea: auto-resize (min 5 rows, max 15 before scroll)
- Character counter turns amber at 3500, red at 4000
- "Advanced options" accordion: reveals timeout slider (10–60 min) and resource tier select
- `[Launch Team ▶]` disabled until objective is non-empty
- On submit: POST then redirect to `/runs/:runId` (the live view)
- Keyboard: `Cmd+Enter` / `Ctrl+Enter` submits

**Components**: `ObjectiveInput`, `ModelSelect`, `AdvancedOptions` (accordion), `RunSubmitButton`

---

### 4.4 Live Run View (`/runs/:runId`) — Core Screen

This is the most important page. Two-column layout: agent status panel (left) + activity feed (right).

```
┌─────────────────────────────────────────────────────────────────┐
│ SIDEBAR │  Runs / run-abc1  Refactor auth module    ● Running   │
│         ├─────────────────────────────────────────────────────── │
│         │  AGENTS (left, 260px)  │  ACTIVITY FEED (right, flex) │
│         │                        │                              │
│         │  ┌──────────────────┐  │  ┌──────────────────────┐   │
│         │  │ 🧠 Planner       │  │  │ 00:01:42  planner    │   │
│         │  │ ✓ Completed      │  │  │ thinking             │   │
│         │  │ 1m 12s           │  │  │ "Analyzing codebase  │   │
│         │  └──────────────────┘  │  │  structure..."       │   │
│         │  ┌──────────────────┐  │  └──────────────────────┘   │
│         │  │ 🔍 Researcher    │  │  ┌──────────────────────┐   │
│         │  │ ● Working        │  │  │ 00:02:18  researcher │   │
│         │  │ 1m 06s elapsed   │  │  │ action: read_file    │   │
│         │  └──────────────────┘  │  │ internal/auth/pkce.go│   │
│         │  ┌──────────────────┐  │  └──────────────────────┘   │
│         │  │ 💻 Coder 1       │  │  ┌──────────────────────┐   │
│         │  │ ○ Waiting        │  │  │ 00:03:01  researcher │   │
│         │  └──────────────────┘  │  │ action: read_file    │   │
│         │  ┌──────────────────┐  │  │ internal/auth/oidc.go│   │
│         │  │ 💻 Coder 2       │  │  └──────────────────────┘   │
│         │  │ ○ Waiting        │  │  ┌──────────────────────┐   │
│         │  └──────────────────┘  │  │ 00:03:44  researcher │   │
│         │  ┌──────────────────┐  │  │ message              │   │
│         │  │ ✅ Reviewer      │  │  │ "Found 3 files that  │   │
│         │  │ ○ Waiting        │  │  │  need updating..."   │   │
│         │  └──────────────────┘  │  └──────────────────────┘   │
│         │                        │                              │
│         │  ────────────────────  │  [virtual scroll — 2000 max] │
│         │  Elapsed: 3m 44s       │                              │
│         │  [Cancel Run]          │  ☰ Filter by agent ▾  🔍    │
└─────────────────────────────────────────────────────────────────┘
```

#### Agent Status Card

```
┌──────────────────────────────────┐
│  [emoji] [Role Name]             │
│  [status badge]  [elapsed time]  │
│  [optional: last action summary] │
└──────────────────────────────────┘
```

Status badges:
- `● Working` — blue-400, pulsing dot animation
- `✓ Completed` — emerald-500
- `✗ Failed` — red-500
- `○ Waiting` — neutral-500

Role emojis: planner 🧠, researcher 🔍, coder_1 💻, coder_2 💻, reviewer ✅

#### Activity Feed Items

Each event type has a distinct visual treatment:

```
agent_thinking:
┌───────────────────────────────────────────────────────────┐
│ 00:03:22  🧠 planner   thinking                           │
│ ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │  ← neutral-800 bg
│ "Analyzing the authentication flow to identify..."        │
└───────────────────────────────────────────────────────────┘

agent_action (tool call):
┌───────────────────────────────────────────────────────────┐
│ 00:03:45  🔍 researcher   action: read_file               │
│ ▸ input:  internal/auth/pkce.go                           │  ← collapsible
│ ▸ output: (142 lines)                                     │  ← click to expand
└───────────────────────────────────────────────────────────┘

agent_message (inter-agent):
┌───────────────────────────────────────────────────────────┐
│ 00:04:12  🔍 researcher → 💻 coder_1   message            │
│ "Please update GenerateCodeVerifier() to use 32 bytes     │  ← violet-900/20 bg
│  instead of 16 for NIST compliance..."                    │
└───────────────────────────────────────────────────────────┘

agent_done:
┌───────────────────────────────────────────────────────────┐
│ 00:12:34  ✅ reviewer   completed                         │  ← emerald-900/30 bg
│ "Changes reviewed and approved. 2 artifacts produced."    │
└───────────────────────────────────────────────────────────┘

run_complete:
┌───────────────────────────────────────────────────────────┐
│  ✓  Run completed in 14m 32s          [View Results →]    │  ← full-width banner
└───────────────────────────────────────────────────────────┘
```

**Feed controls** (bottom of feed):
- Filter dropdown: "All agents" / "planner" / "researcher" / "coder_1" / "coder_2" / "reviewer"
- Search (filter by text content) — client-side over the ring buffer
- Auto-scroll toggle (default: on) — checkbox in corner

---

### 4.5 Results (`/runs/:runId/results`)

```
┌─────────────────────────────────────────────────────────────┐
│ SIDEBAR │  Runs / run-abc1 / Results                        │
│         │                                                   │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │  ✓ Completed   14m 32s   ~42,180 tokens     │ │
│         │  │  Refactor auth module                       │ │
│         │  │  Started 2 hours ago                        │ │
│         │  └─────────────────────────────────────────────┘ │
│         │                                                   │
│         │  Summary                                         │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │  [markdown-rendered summary from reviewer]   │ │
│         │  │  Refactored `internal/auth/pkce.go` to use  │ │
│         │  │  32-byte verifiers per NIST SP 800-63B...   │ │
│         │  └─────────────────────────────────────────────┘ │
│         │                                                   │
│         │  Artifacts (3)                                   │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │ 📄 pkce.go          8.2 KB   [Download]     │ │
│         │  │ 📄 pkce_test.go     5.1 KB   [Download]     │ │
│         │  │ 📄 CHANGES.md       1.2 KB   [Download]     │ │
│         │  └─────────────────────────────────────────────┘ │
│         │                                                   │
│         │  [← Back to Run]           [+ New Run from This] │
└─────────────────────────────────────────────────────────────┘
```

**Components**: `RunSummaryCard`, `MarkdownRenderer` (lazy-loaded), `ArtifactList`, `ArtifactRow`

**"New Run from This"**: Pre-fills the objective in `/runs/new` with context from this run.

---

### 4.6 Settings (`/settings`)

```
┌─────────────────────────────────────────────────────────────┐
│ SIDEBAR │  Settings                                         │
│         │                                                   │
│         │  Account                                         │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │  ○  Hampton Copeland                        │ │
│         │  │     hampton888@gmail.com                    │ │
│         │  │     Managed by Authentik                    │ │
│         │  └─────────────────────────────────────────────┘ │
│         │                                                   │
│         │  API Keys                             [+ New Key] │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │  CI pipeline    kai_ab12...   Never expires  │ │
│         │  │  Last used: 2h ago                [Revoke]   │ │
│         │  ├─────────────────────────────────────────────┤ │
│         │  │  Local dev      kai_xy89...   Expires Jan 1 │ │
│         │  │  Last used: 5d ago                [Revoke]   │ │
│         │  └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

**New Key modal**:
```
┌──────────────────────────────────────────┐
│  New API Key                             │
│                                          │
│  Name  [CI pipeline              ]       │
│  Expires  [Never ▾]                      │
│                                          │
│           [Cancel]  [Create Key]         │
└──────────────────────────────────────────┘
```

After creation, the key is shown once:
```
┌──────────────────────────────────────────┐
│  ✓ Key created                           │
│                                          │
│  kai_aBcD1234eFgH5678...                 │
│  [Copy to clipboard]                     │
│                                          │
│  This key will not be shown again.       │
│                    [Done]                │
└──────────────────────────────────────────┘
```

---

### 4.7 Admin (`/admin`)

Only visible to users where `is_admin = true`. The sidebar link is conditionally rendered.
Navigating to `/admin` without admin role returns a 403 from the API, and the frontend shows an error page.

```
┌─────────────────────────────────────────────────────────────┐
│ SIDEBAR │  Admin                                            │
│         │                                                   │
│         │  Users (8)                        [🔍 Search]    │
│         │  ┌─────────────────────────────────────────────┐ │
│         │  │  Hampton Copeland   admin   Last: just now   │ │
│         │  │  alice@example.com  member  Last: 3h ago     │ │
│         │  │  bob@example.com    member  Last: 1d ago     │ │
│         │  └─────────────────────────────────────────────┘ │
│         │                                                   │
│         │  Recent Runs (all teams)                         │
│         │  [same table as dashboard, but all users]        │
└─────────────────────────────────────────────────────────────┘
```

---

## 5. Component Inventory

| Component | Location | Used By |
|-----------|----------|---------|
| `AppShell` | layout | all authenticated pages |
| `Sidebar` | layout | `AppShell` |
| `LoginPage` | pages | `/login` |
| `Dashboard` | pages | `/` |
| `ActiveRunCard` | molecules | Dashboard |
| `RecentRunRow` | molecules | Dashboard |
| `NewRunForm` | pages | `/runs/new` |
| `ObjectiveInput` | atoms | `NewRunForm` |
| `ModelSelect` | atoms | `NewRunForm` |
| `AdvancedOptions` | molecules | `NewRunForm` |
| `LiveRunView` | pages | `/runs/:runId` |
| `AgentStatusCard` | molecules | `LiveRunView` |
| `MessageFeed` | organisms | `LiveRunView` |
| `EventRow` | molecules | `MessageFeed` |
| `FeedFilterBar` | molecules | `LiveRunView` |
| `ResultsView` | pages | `/runs/:runId/results` |
| `RunSummaryCard` | molecules | `ResultsView` |
| `ArtifactList` | molecules | `ResultsView` |
| `ArtifactRow` | atoms | `ArtifactList` |
| `MarkdownRenderer` | atoms | `ResultsView` (lazy) |
| `SettingsPage` | pages | `/settings` |
| `APIKeyTable` | molecules | `SettingsPage` |
| `NewKeyModal` | molecules | `SettingsPage` |
| `NewKeyRevealModal` | molecules | `SettingsPage` |
| `AdminPage` | pages | `/admin` |
| `UserTable` | molecules | `AdminPage` |
| `SessionExpiredBanner` | overlay | `AppShell` |
| `ErrorPage` | pages | 404, 403, 500 |

---

## 6. Interaction Patterns

### 6.1 Loading States

- **Skeleton screens** (not spinners) for initial page load — prevents layout shift
- **Inline spinners** only for actions (button submits, key creation)
- TanStack Query `isLoading` → show skeleton; `isFetching` (background refresh) → no visual change

### 6.2 Error States

- **Network errors**: toast notification bottom-right, auto-dismiss 5s
- **API errors (400/422)**: inline red text below the relevant field
- **403 Forbidden**: full-page error component with message and "← Back to Dashboard"
- **404 Not Found**: full-page error component
- **Session expired**: modal overlay (not a page redirect) with "Sign in again" link

### 6.3 Empty States

Each list has a contextual empty state:
- No active runs → "No active runs. [+ Start New Run]"
- No recent runs → "No completed runs yet. [+ Start New Run]"
- No API keys → "No API keys. Create one to use the API programmatically."
- No artifacts → "No artifacts were produced by this run."

### 6.4 Keyboard Navigation

- `n` → New Run (from any authenticated page)
- `Escape` → close modal/drawer
- `Cmd+K` / `Ctrl+K` → command palette (Phase 2)
- Tab order follows visual layout

---

## 7. Responsive Behavior

| Viewport | Sidebar | Content |
|----------|---------|---------|
| ≥1280px | Expanded (240px) | Full two-column run view |
| 1024–1279px | Collapsed to icons (56px) | Two-column run view |
| 768–1023px | Hidden, hamburger toggle | Single-column (agents above feed) |
| <768px | Drawer (full-height overlay) | Single-column, feed only |

The Live Run View on mobile collapses to a tabbed view: "Agents" tab / "Feed" tab.

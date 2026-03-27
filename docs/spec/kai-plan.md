# Kai Platform — Implementation Plan

**Status**: Draft  
**Updated**: 2026-03-27  

---

## Pre-flight Checklist

These must be done **before any code is written**. Most are manual Authentik/Bitwarden steps.

### Manual Authentik Steps
- [ ] Apply `rke2/kai/authentik/providers-kai.yaml` blueprint to Authentik at `auth.hwcopeland.net`
  - Creates `kai` OIDC provider + application
  - Generates client_id and client_secret
- [ ] Create Authentik group `kai-admins` and add your user to it
- [ ] Verify `groups` scope mapping exists (should already be in `rke2/authentik/blueprints/scope-mappings.yaml`)

### Bitwarden Items to Create (UUIDs go in ExternalSecret manifests)
- [ ] `kai-db-password` — PostgreSQL password for `kai` user
- [ ] `kai-auth-client-secret` — Authentik OAuth2 client secret
- [ ] `kai-auth-client-id` — Authentik OAuth2 client ID
- [ ] `kai-session-secret` — 32+ random bytes base64, for session HMAC
- [ ] `kai-callback-token` — 32+ random bytes, shared secret for agent pod callbacks
- [ ] `kai-litellm-api-key` — reuse existing LiteLLM key from `openhands` namespace

### Open Questions to Resolve Before Phase 1

| # | Question | Recommended Resolution |
|---|----------|----------------------|
| OQ-1 | Does Longhorn support ReadWriteMany? | Check: `kubectl get sc longhorn -o yaml \| grep allowVolumeExpansion`. If RWX not available, Phase 1 uses sequential agents (RWO + one pod at a time) |
| OQ-2 | CiliumNetworkPolicy vs NetworkPolicy for cross-ns egress | Test a simple pod in `kai` ns pinging `openhands-litellm`. If standard NetworkPolicy doesn't work, switch to CiliumNetworkPolicy |
| OQ-3 | Authentik application slug | Use `kai` |
| OQ-4 | `team_runs.initiated_by` vs `user_id` | Keep `initiated_by` — team membership controls access |
| OQ-5 | WS cookie on Cilium Gateway upgrade | Verify after Phase 3 deploy; fallback is a WS ticket endpoint |
| OQ-6 | Agent image contents (Phase 1 minimum) | bash + curl + git + Python 3 (for LiteLLM client) |

---

## Phase 0 — Repo Scaffolding & CI

**Goal**: The repo structure exists, Docker images build and push to Zot, nothing is deployed yet.  
**Gate**: `build-kai.yml` CI workflow passes; images appear in `zot.hwcopeland.net/kai/`.

### Tasks

**@devops-engineer**
- [ ] Create `rke2/kai/` directory structure following `rke2/chem/flux/` pattern:
  ```
  rke2/kai/
  ├── flux/
  │   ├── apps.yaml           # Flux Kustomization for kai namespace
  │   ├── namespace.yaml
  │   └── k8s-jobs/
  │       └── kustomization.yaml
  ├── authentik/
  │   └── providers-kai.yaml  # Authentik blueprint
  └── crd/
      └── agentsandbox-crd.yaml
  ```
- [ ] Create `.github/workflows/build-kai.yml`:
  - Trigger: push to `main` affecting `kai/**` or `.github/workflows/build-kai.yml`
  - Runner: `[self-hosted, arc-chem]`
  - Steps: `npm run build` (frontend) → `go build ./cmd/kai/` → `docker buildx build` → push to `zot.hwcopeland.net/kai/kai-api`
  - Image tag format: `build-{zero-padded-run-number}-{sha7}` (same as chem)
  - Registry cache: `type=registry,mode=max` (no `type=gha`)
- [ ] Create `Dockerfile` for `kai-api` (multi-stage: node build → go build → distroless)
- [ ] Create `Dockerfile` for `kai-agent` (base: ubuntu:24.04 + bash + curl + git + Python 3)
- [ ] Add Flux `ImageRepository` + `ImagePolicy` + `ImageUpdateAutomation` for both images

**@senior-engineer**
- [ ] Initialize Go module: `go mod init github.com/hwcopeland/iac/kai`
- [ ] Create `cmd/kai/main.go` — skeleton that starts HTTP on `:8080` and returns 200 from `/healthz`
- [ ] Create `internal/config/config.go` — full `envconfig` struct (all fields, all required tags)
- [ ] Initialize React/TypeScript project: `npm create vite@latest ui -- --template react-ts`
- [ ] Add TanStack Router, TanStack Query, Zustand, Tailwind CSS v4 dependencies

---

## Phase 1 — Auth End-to-End

**Goal**: A real user can sign in via Authentik, get a `kai_session` cookie, and `GET /api/me` returns their profile.  
**Gate**: Navigate to `https://kai.hwcopeland.net`, click "Sign in with Authentik", complete Authentik consent, land on dashboard with user info displayed.

### Dependencies
- Phase 0 complete
- Authentik `kai` provider blueprint applied (pre-flight)
- Bitwarden items created (pre-flight)

### Tasks

**@data-engineer**
- [ ] Write `000001_init.up.sql` migration covering: `users`, `sessions`, `api_keys`, `teams`, `team_members`
  - Schema per `docs/tdd/kai-data.md` §3
  - `users.id TEXT` (Authentik sub, not UUID)
  - `sessions.token_hash TEXT` (SHA-256 of cookie, not raw)
- [ ] Write migration runner in `internal/db/migrate.go` with advisory lock (pg_advisory_lock key: 7472836172)
- [ ] Write query functions in `internal/db/queries/`: `UpsertUser`, `CreateSession`, `GetSessionByTokenHash`, `GetUserByID`, `DeleteSessionByTokenHash`

**@senior-engineer**
- [ ] Implement `internal/auth/oidc.go` — `OIDCClient` with JWKS cache via `lestrrat-go/jwx/v2`
  - JWKS URL: `{AUTH_ISSUER_URL}.well-known/jwks.json`
  - Pre-fetch on startup; auto-refresh every 15 min
- [ ] Implement `internal/auth/pkce.go` — `GenerateCodeVerifier()`, `S256Challenge()`, `GenerateState()`
- [ ] Implement `internal/auth/session.go` — `GenerateSessionToken()`, `SHA256Hex()`
- [ ] Implement `internal/api/auth.go` — `LoginHandler`, `CallbackHandler`, `LogoutHandler`
  - `LoginHandler`: generate PKCE pair + state → store in-memory (10-min TTL) → redirect to Authentik
  - `CallbackHandler`: validate state → exchange code → verify ID token → extract claims → upsert user → create session → set HTTP-only cookie → redirect to `/`
  - `LogoutHandler`: delete session from DB → clear cookie → redirect to `/login`
- [ ] Implement `internal/auth/middleware.go` — `SessionMiddleware` (cookie → token_hash → DB lookup → user in context)
- [ ] Implement `GET /api/me` handler — returns `{id, email, display_name, avatar_url, is_admin}`
- [ ] Wire router with auth routes and `/api/me`

**@senior-engineer (frontend)**
- [ ] Implement `LoginPage` (`/login`) — single "Sign in with Authentik" `<a href="/api/auth/login">`
- [ ] Implement `_authed` layout route with `beforeLoad` guard (`ensureQueryData(meQuery)` → throw redirect on 401)
- [ ] Implement `meQuery` — `GET /api/me`, returns user object
- [ ] Implement skeleton `Dashboard` page that shows `user.email`

**@devops-engineer**
- [ ] Create `rke2/kai/flux/k8s-jobs/` manifests:
  - `namespace.yaml`
  - `deployment.yaml` — kai-api Deployment with `envFrom: [{secretRef: {name: kai-api-secrets}}]`
  - `service.yaml` — ClusterIP on 8080 + 8081
  - `httproute.yaml` — Cilium HTTPRoute → `hwcopeland-gateway` → `kai-api-service:8080`
  - `postgres-statefulset.yaml` — PostgreSQL 16, Longhorn PVC
  - `externalsecret.yaml` — all Bitwarden items → `kai-api-secrets` Secret
  - `agentsandbox-crd.yaml` — install the CRD
  - `rbac.yaml` — ClusterRole + ClusterRoleBinding for operator

---

## Phase 2 — Run Lifecycle

**Goal**: A user can start a team run, AgentSandbox CRDs are created, agent pods launch, and the backend receives callbacks.  
**Gate**: `POST /api/teams/:id/runs` creates DB rows + K8s AgentSandbox objects; pods start; `POST /internal/callback` updates task status; run transitions to `completed`.

### Dependencies
- Phase 1 complete (auth works)
- OQ-1 resolved (RWX vs RWO for workspace PVC)
- `kai-agent` image built and available in Zot

### Tasks

**@data-engineer**
- [ ] Write `000002_runs.up.sql` migration: `team_runs`, `agent_tasks`, `run_events`, `artifacts`
  - Schema per `docs/tdd/kai-data.md` §3.6–3.9
  - Composite index on `run_events(run_id, created_at)`
- [ ] Write query functions: `CreateTeamRun`, `UpdateTeamRunStatus`, `CreateAgentTask`, `UpdateAgentTaskStatus`, `CreateRunEvent`, `GetRunEvents`, `CreateArtifact`, `ListArtifacts`

**@senior-engineer (operator)**
- [ ] Implement `kaiv1alpha1` CRD Go types in `internal/operator/types.go`
- [ ] Implement `AgentSandboxReconciler` in `internal/operator/reconciler.go`
  - `reconcilePending`: create NetworkPolicy + Pod → transition to Running → requeue at timeout
  - `reconcileRunning`: check pod phase + timeout → markTerminated or requeue
  - `reconcileTerminating`: delete pod → markTerminated
  - `reconcileTerminated`: TTL GC (300s, matching DockingJob pattern)
- [ ] Implement `buildAgentPod()` in `internal/operator/pod.go`
  - Inject: `KAI_RUN_ID`, `KAI_TEAM_ID`, `KAI_AGENT_ROLE`, `KAI_SANDBOX_NAME`, `KAI_CALLBACK_URL`, `LITELLM_BASE_URL`
  - Mount workspace PVC at `/workspace`
  - ImagePullSecret: `zot-pull-secret`
- [ ] Implement `buildNetworkPolicy()` in `internal/operator/netpol.go`
  - Allow egress: DNS (53), kai-api:8081, openhands-litellm:4000
  - Deny all ingress + all other egress
- [ ] Implement workspace PVC lifecycle in `internal/operator/workspace.go`
- [ ] Wire operator into `cmd/kai/main.go` alongside HTTP server

**@senior-engineer (API)**
- [ ] Implement `POST /api/teams` + `GET /api/teams` handlers
- [ ] Implement `POST /api/teams/:id/runs` — creates `team_run` + workspace PVC + starts orchestrator goroutine
- [ ] Implement `GET /api/runs/:id` — returns run + agent tasks
- [ ] Implement `internal/events/hub.go` — in-process EventHub with ring buffer (2000 events/run)
- [ ] Implement `POST /internal/callback` on port 8081 — validates `X-Kai-Callback-Token` → updates DB → publishes events to hub
- [ ] Implement run orchestrator: sequential agent dispatch (planner → researcher → coder×2 → reviewer)

**@senior-engineer (agent)**
- [ ] Write minimal `kai-agent` entrypoint (bash script or Go binary) that:
  - Reads `KAI_RUN_ID`, `KAI_AGENT_ROLE`, `KAI_CALLBACK_URL`, `KAI_CALLBACK_TOKEN` from env
  - Does minimal work (echo "hello from {role}")
  - POSTs result to `KAI_CALLBACK_URL` with `X-Kai-Callback-Token` header
  - This is a stub — real LLM integration is Phase 4

---

## Phase 3 — Live UI

**Goal**: The React SPA shows real-time agent activity via WebSocket. A user can start a run, watch it execute, and see events stream in.  
**Gate**: Start a run → `/runs/:runId` shows agent status cards updating in real-time; activity feed populates with events; run completes and banner shows "View Results →".

### Dependencies
- Phase 2 complete (run lifecycle works)

### Tasks

**@senior-engineer (backend)**
- [ ] Implement `GET /api/runs/:id/events` WebSocket handler
  - Upgrade via `gorilla/websocket`
  - `CheckOrigin`: allow `https://kai.hwcopeland.net`
  - Subscribe to hub → replay history → stream live events
  - Slow-consumer policy: disconnect after 5s if send buffer full
  - Ping every 30s
- [ ] Verify WS cookie auth works through Cilium Gateway (test with `wscat`)

**@senior-engineer (frontend)**
- [ ] Implement `useTeamRunStream(runId)` hook — WS connect, exponential backoff, ring buffer via `agentActivityStore`
- [ ] Implement `agentActivityStore` (Zustand) — 2000-event ring buffer, agent status derived state
- [ ] Implement `LiveRunView` page — two-column layout (AgentStatusCards + MessageFeed)
- [ ] Implement `AgentStatusCard` — role emoji, phase badge (working/done/waiting/failed), elapsed timer
- [ ] Implement `MessageFeed` — virtualized via `@tanstack/react-virtual`, auto-scroll, filter bar
- [ ] Implement `EventRow` — distinct styling per event type (thinking/action/message/done/error)
- [ ] Implement `Dashboard` active runs section with `ActiveRunCard` components

---

## Phase 4 — Results, Settings & Polish

**Goal**: Complete all 7 pages. API keys work. Admin page works. Bundle is ≤200KB gzipped.  
**Gate**: All acceptance criteria from `docs/prd/kai.md` are met. Playwright E2E suite passes.

### Tasks

**@senior-engineer**
- [ ] Implement `GET /api/runs/:id/artifacts` + `GET /api/artifacts/:id` (download)
- [ ] Implement API key endpoints: `GET/POST /api/keys`, `DELETE /api/keys/:id`
- [ ] Implement `GET /api/admin/users`, `GET /api/admin/runs`
- [ ] Implement `ResultsView` page — summary card, artifact list, lazy MarkdownRenderer
- [ ] Implement `SettingsPage` — API key table, new key modal with one-time reveal
- [ ] Implement `AdminPage` — user table, all-runs table (admin guard)
- [ ] Implement `SessionExpiredBanner` overlay
- [ ] Vite bundle audit — verify initial chunk ≤200KB gzipped; lazy-load MarkdownRenderer

**@sdet**
- [ ] Write Playwright E2E suite: login flow, new run, live view events, results download, logout
- [ ] Write Go unit tests: auth middleware (mock DB), PKCE handlers (mock OIDC), EventHub (race detector)
- [ ] Add E2E job to `build-kai.yml` CI workflow

**@devops-engineer**
- [ ] Add Prometheus `ServiceMonitor` for `kai-api` metrics scraping
- [ ] Add Grafana dashboard for key metrics (active runs, WS connections, reconcile latency)
- [ ] Verify Longhorn backup policy covers `kai-postgres` PVC

---

## Dependency Graph

```
Pre-flight
    └── Phase 0 (scaffolding + CI)
            └── Phase 1 (auth)
                    └── Phase 2 (run lifecycle)
                            └── Phase 3 (live UI)
                                    └── Phase 4 (results + polish)
```

Each phase is a deployable increment — the cluster can run Phase N while Phase N+1 is developed.

---

## Key Files Reference

| File | Phase | Owner |
|------|-------|-------|
| `.github/workflows/build-kai.yml` | 0 | @devops-engineer |
| `kai/cmd/kai/main.go` | 0 | @senior-engineer |
| `kai/internal/config/config.go` | 0 | @senior-engineer |
| `kai/internal/db/migrations/000001_init.up.sql` | 1 | @data-engineer |
| `kai/internal/auth/oidc.go` | 1 | @senior-engineer |
| `kai/internal/auth/pkce.go` | 1 | @senior-engineer |
| `kai/internal/api/auth.go` | 1 | @senior-engineer |
| `kai/internal/auth/middleware.go` | 1 | @senior-engineer |
| `rke2/kai/flux/k8s-jobs/deployment.yaml` | 1 | @devops-engineer |
| `rke2/kai/flux/k8s-jobs/externalsecret.yaml` | 1 | @devops-engineer |
| `rke2/kai/crd/agentsandbox-crd.yaml` | 1 | @devops-engineer |
| `kai/internal/db/migrations/000002_runs.up.sql` | 2 | @data-engineer |
| `kai/internal/operator/reconciler.go` | 2 | @senior-engineer |
| `kai/internal/operator/pod.go` | 2 | @senior-engineer |
| `kai/internal/events/hub.go` | 2 | @senior-engineer |
| `kai/internal/api/internal.go` | 2 | @senior-engineer |
| `kai/ui/src/hooks/useTeamRunStream.ts` | 3 | @senior-engineer |
| `kai/ui/src/stores/agentActivityStore.ts` | 3 | @senior-engineer |
| `kai/ui/src/routes/_authed/runs/$runId/index.tsx` | 3 | @senior-engineer |

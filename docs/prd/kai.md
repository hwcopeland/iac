---
project: "iac"
maturity: "draft"
last_updated: "2025-01-27"
updated_by: "@product-owner"
scope: "Kai — a self-hosted multi-agent agentic platform deployed on RKE2 with Authentik SSO, Kubernetes sandbox CRDs, Go backend, and React/TypeScript frontend"
owner: "@product-owner"
dependencies:
  - docs/tdd/flux-gitops.md
  - rke2/authentik/blueprints/providers-openhands.yaml
  - rke2/chem/flux/k8s-jobs/crd/dockingjob-crd.yaml
---

# PRD: Kai — Multi-Agent Agentic Platform

## 1. Problem Statement

### The Problem

The operator runs a self-hosted RKE2 Kubernetes cluster with a functioning Authentik SSO
identity provider, Flux GitOps automation, and a proven CRD-based job controller pattern (the
`DockingJob` CRD used in the chem namespace). The existing OpenHands deployment at
`rke2/openhands/` provides AI coding assistance but exhibits several critical friction points:

- **Keycloak middleman**: OpenHands uses a Keycloak-as-broker pattern to reach Authentik. This
  introduces an extra hop, requires runtime postStart lifecycle patches to fix upstream token
  handling bugs, and doubles the secret surface. The operator already runs Authentik — the
  broker is unnecessary complexity.
- **Single-agent, single-user feel**: OpenHands is designed for one user, one agent, one task
  at a time. There is no first-class concept of a "team" — a coordinated group of specialized
  agents working together toward a shared goal.
- **No native multi-agent orchestration**: Research tasks, software development workflows, and
  analysis pipelines naturally decompose into multi-role workflows (plan → execute → review).
  OpenHands has no primitives for this composition.
- **Opaque runtime lifecycle**: The runtime-api spawns sandbox pods but the user has no
  visibility into what each agent is doing, what resources are active, or how to review results.
- **No multi-user isolation**: Multiple users cannot run independent teams simultaneously with
  resource and permission isolation.

### Who Has This Problem

- **Solo researchers** who want to dispatch a coordinated research team (coordinator +
  researchers + synthesizer) to answer complex questions or explore a topic deeply.
- **Small team leads** who want a dev team (planner + coder(s) + reviewer) to execute against
  a codebase or technical task with human oversight at key checkpoints.
- **The administrator** (the operator himself) who needs to manage users, audit what teams are
  running, and control resource consumption on shared cluster infrastructure.

### Evidence From the Codebase

- `rke2/openhands/patch-token-manager.yaml` and `patch-runtime-api.yaml`: Multi-hundred-line
  postStart scripts patching Python source at container startup — a direct symptom of forcing
  Keycloak-broker auth onto a system that did not design for it.
- `rke2/openhands/values.yaml`: `ENABLE_ENTERPRISE_SSO: "1"`, `KEYCLOAK_EXTERNAL_URL`, and
  `KEYCLOAK_SERVER_URL_EXT` all present — three config keys just to express "use Authentik."
- `rke2/authentik/blueprints/scope-mappings.yaml`: A working groups scope mapping already
  emits `groups` claim in OIDC tokens. No additional Authentik configuration needed for
  group-based RBAC.
- `rke2/chem/flux/k8s-jobs/crd/dockingjob-crd.yaml`: A proven CRD pattern for launching
  isolated Kubernetes pods from a Go controller already exists in this repo.

---

## 2. Goals & Success Criteria

### Product Goals

| # | Goal | Why It Matters |
|---|------|----------------|
| G1 | Users can log in to Kai using their existing Authentik identity via direct OIDC PKCE — no Keycloak, no middleware | Eliminates the broker complexity that plagues OpenHands |
| G2 | Users can create, monitor, and dismantle named agent teams (dev or research) from a clean web UI | The core product value: multi-agent coordination |
| G3 | Each agent in a team runs in a fully isolated Kubernetes pod provisioned via a custom CRD | Security isolation + resource governance per-agent |
| G4 | Users can watch live agent activity (logs, events, current task) in real time from the UI | Observability is table stakes for agentic work |
| G5 | Authentik groups (`kai-admin`, `kai-user`) drive platform access and role boundaries | Multi-user support without bespoke user management |
| G6 | The entire platform deploys and upgrades via Flux GitOps following the existing `rke2/<namespace>/` pattern | No snowflake deployments; operator can GitOps the platform like everything else |

### MVP Launch Criteria

The MVP is shippable when:
1. A user can log in from a fresh browser session using Authentik SSO.
2. A user can create a dev team or research team, naming it and providing a goal.
3. All agents in the team spin up as isolated pods within ~60 seconds of team creation.
4. The UI shows live logs/status for each agent in the team.
5. A user can stop/delete a team and all agent pods are cleaned up.
6. An admin can see all teams across all users and terminate any team.
7. The deployment lives at `rke2/kai/` in this repo and is managed by Flux.

### Success Metrics (post-launch)

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| Time from "create team" to first agent active | < 90 seconds | Backend telemetry on pod Ready events |
| Team completion rate (team reaches final state without crashing) | > 80% | Status field on `AgentTeam` CRD |
| UI perceived responsiveness | < 200ms for navigation actions | Browser performance marks |
| Auth flow time (login to dashboard) | < 5 seconds | Measured on cold session |
| Agent pod cleanup after team deletion | 100% within 30 seconds | No orphan pods in `kai` namespace |
| Zero Keycloak dependencies | 0 references | `grep -r keycloak rke2/kai/` returns empty |

---

## 3. User Personas

### Persona A: Alex — The Solo Researcher

**Role**: Academic researcher / independent analyst  
**Technical level**: Comfortable with web UIs; knows what Kubernetes is but doesn't operate it  
**Use frequency**: 3–5 sessions per week, each lasting 30–120 minutes  
**Goal**: Dispatch a research team to explore a topic (e.g., "summarize the state of mRNA
therapeutics for metabolic disease"), review synthesized output, iterate on focus areas  
**Pain today**: Running OpenHands means one agent, one long conversation, no parallelism.
Alex has to manually sequence: search → read → synthesize → write. A multi-agent research
team would let a coordinator delegate to parallel researchers while a synthesizer assembles
results.  
**What Alex needs from Kai**:
- Simple team creation wizard: name, goal, pick team type (Research)
- Visual dashboard showing all researcher agents and their current status
- Ability to view synthesized results as the synthesizer writes them
- Ability to nudge a specific agent ("researcher-2: focus on clinical trials only")

### Persona B: Morgan — The Small Team Lead

**Role**: Software engineer / tech lead managing a small side project  
**Technical level**: Strong developer; familiar with K8s concepts  
**Use frequency**: Daily during active sprints; 1–2 hour sessions  
**Goal**: Hand off a bounded engineering task (e.g., "implement a REST endpoint for user
preferences with tests") to a dev team and review the output  
**Pain today**: OpenHands can write code but Morgan must orchestrate: write spec → ask agent
to implement → manually review → ask agent to fix → repeat. No planner to decompose the task,
no reviewer to catch issues before Morgan sees them.  
**What Morgan needs from Kai**:
- Dev team with clear role separation: Planner (decomposes), Coder(s) (implement), Reviewer
  (critiques before output reaches Morgan)
- Live view of the Planner's task decomposition as it forms
- Ability to see each Coder's diff/output as it progresses
- Review gate: Reviewer output is surfaced prominently for Morgan's approval before the task
  is marked complete
- Team history: Morgan can reload a completed team to review what was produced

### Persona C: The Admin (the operator, hwcopeland)

**Role**: Platform owner and cluster operator  
**Technical level**: Expert; manages the entire RKE2 cluster  
**Use frequency**: Weekly platform maintenance; on-demand incident response  
**Goal**: Ensure the platform operates healthily, users aren't consuming runaway resources,
and Authentik group membership controls access correctly  
**What the Admin needs from Kai**:
- Admin view: all teams across all users, with owner, creation time, status, resource usage
- Ability to force-terminate any team
- Platform configuration surface (LLM endpoint, default resource limits per agent)
- Clear audit trail: who created what team, when, what goal
- Authentik group `kai-admin` grants admin access; `kai-user` grants standard access
- No access if neither group is present

---

## 4. Core User Stories with Acceptance Criteria

### Epic 1: Authentication & Identity

---

**KAI-101 — Authentik SSO Login**  
*As a user, I want to log in to Kai using my Authentik account so that I don't need a
separate Kai password.*

**Acceptance Criteria:**
- Given I am not logged in, when I navigate to `https://kai.hwcopeland.net`, then I am
  redirected to Authentik at `https://auth.hwcopeland.net` via an OIDC Authorization Code
  + PKCE flow.
- Given I authenticate successfully in Authentik, when I am redirected back to Kai with an
  authorization code, then Kai exchanges the code for tokens and creates a server-side
  session without storing the raw access token in `localStorage`.
- Given my Authentik account is in neither `kai-admin` nor `kai-user` group, when I
  complete SSO, then I see an "Access Denied" page with a message explaining I need to be
  added to an Authentik group.
- Given my Authentik access token expires, when I make an API request, then Kai
  transparently refreshes the token using the refresh token without requiring me to
  re-authenticate.
- Given I click "Log out", when the logout completes, then my Kai session is invalidated,
  Authentik's backchannel logout is triggered, and I am returned to the Kai login page.

**Technical notes for implementors:** The Authentik `groups` scope mapping
(`rke2/authentik/blueprints/scope-mappings.yaml`) already emits `groups` in the ID token.
Kai's backend reads `claims.groups` to assign roles. No Keycloak. No token broker.

---

**KAI-102 — Role Assignment from Authentik Groups**  
*As an admin, I want Kai roles to derive automatically from Authentik group membership so
that user access is managed in one place.*

**Acceptance Criteria:**
- Given my Authentik account is in the `kai-admin` group, when I log in, then I have the
  `admin` role and can see the Admin panel.
- Given my Authentik account is in the `kai-user` group (only), when I log in, then I have
  the `user` role and cannot see admin-only UI elements.
- Given I am in both `kai-admin` and `kai-user`, when I log in, then the `admin` role takes
  precedence.
- Given an Authentik admin removes me from all `kai-*` groups, when my token refreshes,
  then my next API call returns 403 and I am shown the "Access Denied" page.

---

### Epic 2: Team Management

---

**KAI-201 — Create a Team**  
*As a user, I want to create a named agent team with a goal so that I can dispatch
coordinated AI work.*

**Acceptance Criteria:**
- Given I am on the dashboard, when I click "New Team", then I see a creation form with
  fields: Team Name (required, 3–64 chars), Goal / Objective (required, free text), Team
  Type (required: Dev Team or Research Team).
- Given I submit a valid team creation form, when the backend processes it, then an
  `AgentTeam` CRD resource is created in the `kai` namespace with my user ID as the owner
  annotation.
- Given the `AgentTeam` CRD is created, when the Kai controller reconciles it, then the
  appropriate `AgentSandbox` CRD resources are created (one per agent role) within 15
  seconds.
- Given the `AgentSandbox` CRDs are created, when the controller reconciles each sandbox,
  then a Kubernetes Pod is launched for each agent within 30 seconds of sandbox creation.
- Given any team creation input is invalid, when I try to submit, then inline validation
  errors are shown and the form is not submitted.
- Given I already have 5 active teams, when I try to create a 6th, then I see an error
  message stating the team limit (configurable by admin, default 5 per user).

---

**KAI-202 — View Teams Dashboard**  
*As a user, I want to see all my active teams at a glance so that I can quickly jump into
any ongoing work.*

**Acceptance Criteria:**
- Given I am logged in, when I navigate to the dashboard, then I see a card for each of my
  active teams showing: name, type, status (Pending / Running / Completed / Failed), agent
  count, and creation time.
- Given I have no active teams, when I view the dashboard, then I see an empty state with a
  prominent "Create your first team" call-to-action.
- Given one of my team's statuses changes (e.g., Running → Completed), when the dashboard
  is open, then the card updates within 5 seconds without a page refresh (via WebSocket or
  SSE).

---

**KAI-203 — Team Detail View**  
*As a user, I want to open a team and see each agent's status and output so that I can
follow the work in progress.*

**Acceptance Criteria:**
- Given I click on a team card, when the team detail page loads, then I see a panel for
  each agent in the team showing: role name, current status, and the last 50 lines of
  agent log output.
- Given an agent is actively running, when new log lines are emitted, then the log panel
  updates in real time (within 3 seconds) via a streaming connection.
- Given an agent completes its task, when I view that agent's panel, then I can see its
  final output/artifact prominently rendered (Markdown for text, syntax-highlighted code
  for code artifacts).
- Given an agent has failed, when I view that agent's panel, then the failure reason is
  shown prominently with the last 100 lines of stderr.

---

**KAI-204 — Stop / Delete a Team**  
*As a user, I want to stop an active team and clean up all its resources so that I don't
leave orphaned pods consuming cluster resources.*

**Acceptance Criteria:**
- Given I am viewing a team, when I click "Stop Team" and confirm the dialog, then the
  `AgentTeam` CRD is marked for deletion.
- Given the `AgentTeam` CRD is being deleted, when the controller processes the deletion,
  then all associated `AgentSandbox` CRDs and their underlying Pods are deleted within 30
  seconds.
- Given a team is stopped, when I reload the teams dashboard, then the team no longer
  appears (or shows as "Terminated" for a configurable retention window).
- Given I close a browser tab mid-task, when I return later, then the team is still running
  (teams are not tied to browser sessions).

---

### Epic 3: Agent Sandbox Runtime

---

**KAI-301 — AgentSandbox CRD Lifecycle**  
*As a platform operator, I want each agent to run in an isolated Kubernetes pod managed via
a CRD so that resources are governed and agents cannot interfere with each other.*

**Acceptance Criteria:**
- Given an `AgentSandbox` CRD is created, when the controller reconciles it, then a Pod is
  launched with: resource limits enforced (default: 1 CPU, 2Gi memory), no host network
  access, no privileged mode, read-only root filesystem (with an emptyDir scratch volume).
- Given an `AgentSandbox` Pod crashes or exits non-zero, when the controller observes this,
  then the `AgentSandbox` status is updated to `Failed` with the exit reason.
- Given an `AgentSandbox` CRD is deleted, when the controller reconciles the deletion, then
  the associated Pod is deleted and no orphan resources remain.
- Given an agent's Pod has been Running for longer than the configured max TTL (default: 2
  hours), when the controller checks TTL, then the Pod is terminated and the sandbox is
  marked `Expired`.
- Given the controller crashes and restarts, when it comes back up, when it reconciles all
  existing `AgentSandbox` CRDs, then it correctly accounts for all running Pods without
  creating duplicates.

---

**KAI-302 — Dev Team Composition**  
*As a user selecting "Dev Team", I want the correct agent roles to be created automatically
so that the team is ready to execute software development tasks.*

**Acceptance Criteria:**
- Given I create a Dev Team, when the team is provisioned, then the following agent
  sandboxes are created: 1 × Planner, 2 × Coder (default, configurable), 1 × Reviewer.
- Given the Planner agent is Running, when it processes the team goal, then it emits a
  structured task decomposition that is stored as a team artifact and visible in the UI.
- Given the Planner has produced tasks, when a Coder agent receives its assignment, then
  it begins executing and its output (code diffs, file changes) is streamed to the UI.
- Given Coder(s) have completed their work, when the Reviewer agent runs, then it produces
  a structured review report surfaced prominently in the UI before the task is marked
  complete.

---

**KAI-303 — Research Team Composition**  
*As a user selecting "Research Team", I want the correct agent roles to be created
automatically so that the team is ready to execute research and synthesis tasks.*

**Acceptance Criteria:**
- Given I create a Research Team, when the team is provisioned, then the following agent
  sandboxes are created: 1 × Coordinator, 2 × Researcher (default, configurable),
  1 × Synthesizer.
- Given the Coordinator is Running, when it processes the team goal, then it decomposes the
  goal into research sub-questions and assigns one per Researcher agent.
- Given Researcher agents are Running in parallel, when each completes its sub-question,
  then its findings are persisted as a team artifact and the Synthesizer is notified.
- Given all Researchers have completed, when the Synthesizer runs, then it produces a
  unified synthesis document stored as the team's primary output artifact.

---

### Epic 4: Admin Capabilities

---

**KAI-401 — Admin Team Overview**  
*As an admin, I want to see all active teams across all users so that I can monitor
platform health and resource usage.*

**Acceptance Criteria:**
- Given I have the `admin` role, when I navigate to the Admin panel, then I see a table of
  all active teams across all users with columns: User, Team Name, Type, Status, Agent
  Count, CPU/Memory usage, Created At.
- Given I am a regular user, when I try to access the Admin panel URL directly, then I
  receive a 403 response and am redirected to my own dashboard.

---

**KAI-402 — Admin Force-Terminate Team**  
*As an admin, I want to force-terminate any team so that I can reclaim resources or stop
runaway agents.*

**Acceptance Criteria:**
- Given I am an admin viewing the team table, when I click "Terminate" on any team and
  confirm, then the team and all its agent pods are deleted within 30 seconds regardless
  of owner.
- Given a team is terminated by an admin, when the owning user's dashboard refreshes, then
  the team shows as "Terminated by Admin" (not silently disappearing).

---

### Epic 5: Deployment & Operations

---

**KAI-501 — Flux GitOps Deployment**  
*As the operator, I want Kai to be managed by Flux from the `rke2/kai/` path in this
monorepo so that it follows the same GitOps pattern as every other namespace.*

**Acceptance Criteria:**
- Given the `rke2/kai/` directory contains a `kustomization.yaml`, when Flux reconciles,
  then Kai is deployed to the `kai` namespace without manual `kubectl apply`.
- Given a PR is merged to `main` updating Kai manifests, when Flux's reconciliation
  interval elapses (≤ 5 minutes), then the updated resources are applied to the cluster.
- Given a Flux reconciliation fails (bad manifest), when the operator inspects the cluster,
  then the previous working state is preserved and the failure is visible in Flux status.

---

**KAI-502 — Secrets via External Secrets Operator**  
*As the operator, I want all Kai secrets sourced from Bitwarden via the existing
ClusterSecretStore so that no secrets are stored in Git.*

**Acceptance Criteria:**
- Given the `rke2/kai/` manifests are applied, when the `ExternalSecret` resources are
  reconciled, then the following Kubernetes Secrets are created from Bitwarden: OIDC client
  secret, database password, session signing key.
- Given no Kai secrets exist in Git, when `git grep -i "secret\|password\|key" rke2/kai/`
  is run, then no plaintext secret values appear.

---

## 5. Feature List

### MVP — Must Have

| Feature | Notes |
|---------|-------|
| Authentik OIDC PKCE auth | Direct, no Keycloak. Groups → roles. |
| User dashboard (own teams) | Card grid, live status via WebSocket/SSE |
| Team creation wizard | Name, goal, type (Dev / Research) |
| Dev Team composition | Planner + 2× Coder + Reviewer |
| Research Team composition | Coordinator + 2× Researcher + Synthesizer |
| `AgentTeam` CRD + controller | Go controller, lives in `kai` namespace |
| `AgentSandbox` CRD + controller | One CRD per agent, creates isolated Pod |
| Live agent log streaming | WebSocket from backend → UI log panels |
| Team artifact storage | Final outputs persisted, viewable after run |
| Team stop / delete | Clean teardown with pod cleanup |
| Admin panel | All teams across users, force-terminate |
| Flux GitOps deployment | `rke2/kai/` kustomization, Flux-managed |
| ExternalSecrets for all secrets | Bitwarden-backed |
| HTTPRoute via Gateway API | Pattern: `kai.hwcopeland.net` |
| Basic resource limits per agent | CPU/memory defaults; admin-configurable |

### Phase 2 — Should Have

| Feature | Notes |
|---------|-------|
| Team templates / saved presets | Save a team config and re-run it |
| Agent message bus (inter-agent) | Structured messages between agents in a team; MVP uses shared artifact store |
| Human-in-the-loop gates | Pause team at defined checkpoints for user input |
| LLM model selection per team | Choose from available LiteLLM models at team creation |
| Team history / archive | Completed teams visible with full output for 7 days |
| Per-user resource quotas | Admin sets max concurrent teams / max pods per user |
| Notification on team completion | In-app notification when team finishes |
| Authentik blueprint for Kai | Auto-provision OIDC provider via GitOps |

### Phase 3 — Could Have

| Feature | Notes |
|---------|-------|
| Custom team compositions | User defines their own roles and agent count |
| Agent-to-agent messaging UI | Visual conversation thread between agents |
| Output export (ZIP, PDF) | Download all team artifacts |
| Grafana dashboard for Kai | Team latency, pod counts, failure rates |
| Webhook on team completion | POST to external URL when team finishes |
| Mobile-responsive UI | Polish pass for smaller screens |

### Won't Have (ever, for this project)

- Billing, subscriptions, usage metering
- SaaS multi-tenancy (organization hierarchy)
- Keycloak or any OIDC broker middleware
- Public user registration (all users must be in Authentik)
- OpenHands code or assets (build from scratch)
- Plugin marketplace
- GitHub/GitLab OAuth integration (not needed; Authentik is the IdP)

---

## 6. Non-Goals (Explicit Out of Scope)

| Non-Goal | Reason |
|----------|--------|
| **No Keycloak** | Authentik is already deployed; Keycloak adds a useless broker hop and was the root cause of all OpenHands auth fragility |
| **No billing or metering** | Single-operator homelab; resource governance via K8s limits is sufficient |
| **No public-facing registration** | All users are pre-provisioned in Authentik; self-signup is a security risk on a home cluster |
| **No SaaS multi-tenancy** | One "organization" (the operator's household/research group); Authentik groups are sufficient |
| **No OpenHands code** | Kai is a clean-room implementation inspired by OpenHands concepts but not derived from it |
| **No LLM proxy in-platform** | Kai reuses the existing `openhands-litellm` proxy already deployed on the cluster |
| **No complex org hierarchy** | Admin + User is sufficient; no teams-within-orgs nesting |
| **No mobile-first UI** | Desktop web is the primary surface; mobile responsiveness is Phase 3 polish |

---

## 7. Success Metrics

### Platform Health (Operational)

| Metric | Target | Notes |
|--------|--------|-------|
| Team creation to first agent Running | < 90s | Measures controller + scheduler latency |
| Agent pod cleanup after deletion | < 30s, 100% | Zero orphan pods |
| Auth round-trip (login → dashboard) | < 5s on LAN | Measures Authentik + Kai latency |
| API p99 latency | < 500ms | For all non-streaming endpoints |
| Controller crash recovery time | < 60s | Pod restart + reconcile loop |

### User Experience

| Metric | Target | Notes |
|--------|--------|-------|
| Team completion rate | > 80% | Percentage of teams that reach Completed state |
| UI update lag (live status) | < 5s | Time from backend event to UI render |
| Time-to-first-value (new user) | < 10 minutes | From login to first running team |

### Platform Cleanliness (vs. OpenHands)

| Metric | Target | Notes |
|--------|--------|-------|
| Keycloak references in `rke2/kai/` | 0 | Hard requirement |
| Plaintext secrets in `rke2/kai/` | 0 | Enforced by ExternalSecrets pattern |
| Runtime source-code patches | 0 | No postStart hacks; build it right |
| Lines of CRD YAML vs. OpenHands patch YAML | Fewer | Qualitative: simpler is better |

---

## 8. Constraints

### Infrastructure Constraints

| Constraint | Detail |
|-----------|--------|
| **Cluster type** | RKE2 (Rancher Kubernetes Engine 2), self-hosted on bare metal (Ubuntu) |
| **Nodes** | 1 control-plane (k8s00, tainted NoSchedule) + 4 worker nodes (k8s01–k8s05); agent pods land on workers |
| **CNI** | Cilium in strict mode (observed from cluster); NetworkPolicy is available and should be used for agent pod isolation |
| **Storage** | Longhorn CSI for persistent volumes; use `longhorn-ssd` class for latency-sensitive data; `longhorn` for bulk |
| **Ingress** | Gateway API (`gateway.networking.k8s.io/v1`), parent `hwcopeland-gateway` in `kube-system`, sectionName `https`; DNS via Cloudflare operator annotation |
| **Secrets** | `ClusterSecretStore` `bitwarden-login` backed by Bitwarden CLI; all secrets as `ExternalSecret` resources |
| **Registry** | Private Zot registry at `zot.hwcopeland.net`; image pull secrets via ExternalSecrets pattern |
| **Load balancer** | MetalLB in L2 mode, IP pool 192.168.1.200–220; not needed for Kai (Gateway API handles ingress) |

### Authentication Constraints

| Constraint | Detail |
|-----------|--------|
| **IdP** | Authentik at `https://auth.hwcopeland.net`; must register Kai as an OIDC provider via Authentik blueprint in `rke2/authentik/blueprints/` |
| **Auth flow** | Authorization Code + PKCE; no implicit flow; no client credentials flow for user auth |
| **Groups claim** | Authentik scope mapping `groups` already configured and emits `groups: ["kai-admin", ...]` in ID token |
| **No Keycloak** | Hard constraint; the existing `aiauth.hwcopeland.net` Keycloak instance used by OpenHands must NOT be referenced |
| **Token storage** | Access + refresh tokens stored server-side (Go session store); only a session cookie sent to browser |
| **Logout** | Must implement Authentik backchannel logout endpoint |

### GitOps Constraints

| Constraint | Detail |
|-----------|--------|
| **Single repo** | All manifests in `hwcopeland/iac` monorepo; no separate Kai infrastructure repo |
| **Path convention** | `rke2/kai/` for cluster manifests; follows `rke2/<namespace>/` pattern |
| **Flux source** | `GitRepository` `tooling` in namespace `tooling` is the existing source of truth |
| **Kustomization** | Must add a `Kustomization` for Kai in the bootstrap entry point (analogous to `rke2/chem/flux/apps.yaml`) |
| **No Helm charts for custom code** | Go backend + React frontend are deployed as plain Deployments; Helm only for third-party dependencies (e.g., PostgreSQL) |
| **Image automation** | Kai images hosted on `zot.hwcopeland.net/kai/`; use Flux `ImageRepository` + `ImagePolicy` + `ImageUpdateAutomation` (follow chem image-automation pattern) |

### Technology Constraints

| Constraint | Detail |
|-----------|--------|
| **Backend language** | Go (matches existing chem controller; operator preference) |
| **Frontend language** | React + TypeScript (modern, typed, component-friendly for agent activity panels) |
| **CRD API group** | `kai.hwcopeland.net`; kinds: `AgentTeam` (v1), `AgentSandbox` (v1) |
| **LLM proxy** | Reuse existing `openhands-litellm` service in `openhands` namespace via in-cluster DNS; do not deploy a new LLM proxy |
| **Database** | PostgreSQL for Kai state (team metadata, user sessions, artifacts); deploy via Bitnami Helm chart or direct Deployment; credentials via ExternalSecrets |
| **No external dependencies** | Kai must be fully functional on LAN with no internet access (all dependencies in-cluster or local registry) |

---

## 9. Risks & Assumptions

### Assumptions

| # | Assumption | Validation needed? |
|---|-----------|-------------------|
| A1 | The existing Authentik `groups` scope mapping correctly includes `kai-admin` and `kai-user` group names once those groups are created in Authentik | Yes — create groups in Authentik, test OIDC flow before backend development begins |
| A2 | The existing `openhands-litellm` LiteLLM proxy can be shared with Kai agents without auth conflicts | Yes — confirm in-cluster DNS access and auth requirements from `openhands` namespace to `litellm` |
| A3 | Four worker nodes provide sufficient CPU/memory for 3–5 concurrent teams (each team = 4 pods × ~1 CPU, 2Gi) ≈ 20 CPUs, 40Gi for 5 teams | Validate against current cluster utilization via `kubectl top nodes` |
| A4 | Cilium NetworkPolicy is available and enforced (not just installed but active) | Confirm `cilium status` on the cluster |
| A5 | Agent pods running LLM-backed workloads need egress to the LiteLLM proxy but not to the public internet | Confirm all LLM calls route through the in-cluster proxy |

### Risks

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|------------|
| R1 | Inter-agent communication protocol complexity exceeds MVP scope | Medium | High | MVP scope: agents share state via a simple artifact store (PVC or database table); no real-time message bus in MVP |
| R2 | Agents run LLM calls that exhaust available API quota | Medium | Medium | Per-team LLM call budgeting in Phase 2; MVP relies on operator awareness |
| R3 | Authentik blueprint registration for Kai OIDC provider causes conflicts with existing OpenHands blueprint | Low | Medium | Use unique `pk` and `slug` values; test in a separate Authentik test app first |
| R4 | Go controller reconciliation loops create race conditions when multiple teams are created simultaneously | Medium | Medium | Use standard controller-runtime patterns with owner references; write unit tests for the reconciler |
| R5 | Agent pods escape resource limits and affect cluster stability | Low | High | Enforce LimitRange in `kai` namespace; set pod-level resource limits via `AgentSandbox` spec; use Cilium NetworkPolicy to block unexpected traffic |
| R6 | Frontend WebSocket connections don't survive proxy/gateway timeouts | Medium | Low | Use SSE as fallback; implement client-side reconnect with exponential backoff |
| R7 | Building an agent orchestration protocol from scratch is underestimated in complexity | High | High | MVP cuts scope to: shared artifact store + simple sequential workflow (Planner → Coders → Reviewer). No real-time inter-agent messaging in MVP. |

---

## 10. Dependencies & Stakeholders

### External Dependencies

| Dependency | Owned By | Status | Risk if Unavailable |
|-----------|----------|--------|---------------------|
| Authentik at `auth.hwcopeland.net` | Operator | Running, managed in `rke2/authentik/` | Auth fails entirely; Kai is inaccessible |
| LiteLLM proxy (`openhands-litellm`) | Operator (OpenHands namespace) | Running | Agents cannot call LLMs; all agent work fails |
| External Secrets Operator + Bitwarden CLI | Operator | Running, `bitwarden-login` ClusterSecretStore active | Secrets don't sync; deployments crash |
| Flux in `tooling` namespace | Operator | Running | GitOps changes don't apply automatically |
| Gateway API (`hwcopeland-gateway`) | Operator (`kube-system`) | Running | Kai not accessible externally |
| Longhorn CSI | Operator (`longhorn-system`) | Running | PVC creation for artifact storage fails |
| Zot registry at `zot.hwcopeland.net` | Operator | Running | Image pulls fail; controller/backend cannot start |

### Required Pre-Work (Before Development Begins)

1. **Authentik groups created**: `kai-admin` and `kai-user` groups must exist in Authentik
   before the OIDC flow can be tested.
2. **Authentik OIDC provider registered**: A blueprint `rke2/authentik/blueprints/providers-kai.yaml`
   must be created registering Kai as an OIDC application with a client_id, redirect URIs,
   and the `groups` scope mapping.
3. **Bitwarden item created**: A new Bitwarden item (UUID to be determined) for Kai secrets
   (OIDC client secret, DB password, session signing key).
4. **`kai` namespace consideration**: Decide whether Kai agent pods run in the same `kai`
   namespace or a separate `kai-sandboxes` namespace. Separate namespace is recommended for
   NetworkPolicy isolation and resource quota management.

### Stakeholders

| Stakeholder | Role | Input Needed |
|-------------|------|-------------|
| hwcopeland (operator/admin) | Product owner + cluster operator | Final approval on architecture, resource limits, LiteLLM sharing |

---

## 11. Prioritization Rationale

**Why Kai is P1 now:**
OpenHands' auth fragility (evidenced by the multi-hundred-line postStart patches) represents
ongoing operational risk. Every OpenHands version upgrade risks breaking the patches. Building
Kai clean eliminates this technical debt while delivering the multi-agent capability that
OpenHands cannot provide.

**Why MVP cuts inter-agent messaging:**
Real-time inter-agent communication (a proper message bus, pub/sub, agent-to-agent protocol)
is architecturally complex and high-risk to build correctly. The DockingJob CRD precedent
shows a successful simpler pattern: a controller orchestrates jobs via shared state (database
+ PVC). Kai MVP follows this pattern: agents read/write to a shared artifact store, and the
controller manages sequencing. Phase 2 adds a real-time message bus when usage patterns
clarify what agents actually need to communicate.

**Why React/TypeScript over alternatives:**
The live agent activity panels (streaming logs, real-time status) require a reactive component
model. React's ecosystem (react-query for polling, zustand/jotai for state, xterm.js for
terminal-style log rendering) handles all of these patterns. The operator is building this
solo; TypeScript's type system reduces runtime errors in the frontend significantly.

**What was explicitly deprioritized:**
- Mobile UI (no known mobile use case for a research/dev platform)
- LLM model selection per team (LiteLLM defaults are sufficient for MVP)
- Team history/archive (adds DB complexity; MVP shows active teams only)
- Custom team compositions (Dev and Research cover known use cases)

---

## 12. Open Questions

| # | Question | Owner | Needed By |
|---|---------|-------|-----------|
| OQ1 | Should Kai agent pods run in the `kai` namespace or a separate `kai-sandboxes` namespace? Separate namespace enables stricter NetworkPolicy and ResourceQuota isolation but adds complexity. | @staff-engineer + operator | Before TDD |
| OQ2 | Should Kai reuse the `openhands-litellm` service (cross-namespace service reference) or deploy its own LiteLLM instance? Sharing is simpler but creates a dependency on the OpenHands namespace. | Operator | Before TDD |
| OQ3 | What is the inter-agent communication protocol for MVP? Options: (a) shared PostgreSQL table agents poll, (b) shared PVC volume, (c) a lightweight in-cluster message queue (NATS). | @staff-engineer | Before TDD |
| OQ4 | Should the Go backend and the CRD controller be the same binary (monolith) or separate deployments? Separate allows independent scaling; same binary is simpler for MVP. | @staff-engineer | Before TDD |
| OQ5 | What base container image do agent sandbox pods use? Options: (a) a minimal `ubuntu` image with a Kai agent runtime binary, (b) a Python environment (if LLM SDKs are Python-based), (c) a custom Go-based agent runner. | @staff-engineer | Before Sprint 1 |
| OQ6 | Does the operator want to keep the existing OpenHands deployment running in parallel with Kai, or should Kai fully replace it? (Affects whether we can share `openhands-litellm`.) | Operator | Before development starts |
| OQ7 | What Authentik `client_id` and redirect URIs should be used for Kai? (Required for the Authentik blueprint.) | Operator | Before Sprint 1 |

---

## Appendix A: Architectural Sketch (Informational — Not Prescriptive)

> This sketch describes a likely architecture for the benefit of @staff-engineer when writing
> the TDD. It is NOT a binding technical decision.

```
                         ┌─────────────────────────────────────────┐
                         │           kai namespace                  │
                         │                                          │
  Browser ─────HTTPS────▶│  React SPA (served by Go backend)        │
  (PKCE)                 │        │                                 │
                         │        ▼                                 │
  Authentik ◀──OIDC─────▶│  Go Backend (HTTP API + WebSocket)        │
  auth.hwcopeland.net    │        │                                 │
                         │        ├──DB──▶ PostgreSQL               │
                         │        │                                 │
                         │        ▼                                 │
                         │  Kai Controller (Go, controller-runtime) │
                         │        │                                 │
                         │        ├──creates──▶ AgentTeam CRD       │
                         │        │                  │              │
                         │        │                  ▼              │
                         │        └──creates──▶ AgentSandbox CRD   │
                         │                          │               │
                         │                          ▼               │
                         │                    Agent Pod(s)          │
                         │                    (isolated, resource   │
                         │                     limited, NetworkPol) │
                         │                          │               │
                         │                          ▼               │
                         │                   LiteLLM Proxy          │
                         │               (openhands namespace)      │
                         └─────────────────────────────────────────┘
```

### Key CRD Shapes (Informational)

```yaml
# AgentTeam — top-level team resource
kind: AgentTeam
spec:
  owner: "user-id-from-authentik-sub"
  displayName: "My Dev Team"
  goal: "Implement user preferences API with tests"
  teamType: "dev"  # or "research"
  maxAgents: 4
  ttlSeconds: 7200
status:
  phase: "Running"  # Pending | Running | Completed | Failed | Terminated
  agents: [...]

# AgentSandbox — one per agent
kind: AgentSandbox
spec:
  teamRef: "my-dev-team"
  role: "planner"  # planner | coder | reviewer | coordinator | researcher | synthesizer
  image: "zot.hwcopeland.net/kai/agent-runner:latest"
  resources:
    limits: { cpu: "1", memory: "2Gi" }
    requests: { cpu: "100m", memory: "256Mi" }
status:
  phase: "Running"
  podName: "kai-sandbox-abc123"
  startTime: "2025-01-27T..."
```

---

## Appendix B: OpenHands Pattern Analysis — What to Adopt vs. Avoid

| Pattern | OpenHands Does | Kai Should Do |
|---------|---------------|---------------|
| OIDC auth | Keycloak broker → Authentik (two hops) | Direct Authentik PKCE (one hop) |
| Runtime auth bugs | postStart Python patches to fix token bugs | Build auth correctly from scratch |
| Sandbox pods | runtime-api spawns Pods imperatively via REST | Controller-runtime reconciles `AgentSandbox` CRDs declaratively |
| Secret management | Multiple ExternalSecrets, one Bitwarden item | Same pattern — reuse it |
| HTTP routing | HTTPRoute via Gateway API | Same pattern — reuse it |
| Deployment | Helm chart with custom patches | Plain Kustomization; Helm only for dependencies |
| LLM proxy | Bundled LiteLLM with Helm | Reuse existing `openhands-litellm` |
| DNS | Cloudflare operator annotations | Same pattern — reuse it |
| GitOps | Part of the `rke2/openhands/` kustomization, referenced from tooling | `rke2/kai/` kustomization, same bootstrap pattern |

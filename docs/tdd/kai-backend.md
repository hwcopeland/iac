---
project: "kai"
maturity: "draft"
last_updated: "2025-07-14"
updated_by: "@staff-engineer"
scope: "Go backend service for Kai: HTTP API, Authentik OIDC auth middleware, WebSocket event
  hub, Kubernetes CRD reconciler, agent pod lifecycle management, and internal callback listener."
owner: "@staff-engineer"
dependencies:
  - docs/prd/kai.md
  - docs/tdd/kai-deploy.md
  - docs/tdd/kai-data.md
  - docs/tdd/kai-auth.md
  - docs/tdd/kai-sandbox-crd.md
---

# TDD: Kai Backend (Go API + Operator)

## 1. Problem Statement

### 1.1 What and Why Now

Kai needs a Go backend binary that serves three roles simultaneously:

1. **HTTP API** — REST endpoints consumed by the React SPA and external API clients
2. **WebSocket hub** — real-time event streaming for live run views
3. **Kubernetes operator** — CRD reconciler that manages `AgentSandbox` pod lifecycle

The decision to ship these as one binary (Phase 1) is deliberate: it eliminates inter-process
coordination overhead, reduces operational surface area, and avoids premature decomposition
before the domain is well understood. The `EventBus` and `Operator` interfaces are explicitly
designed for extraction in Phase 3 if needed.

### 1.2 Constraints

- **No Keycloak** — direct Authentik PKCE only (the Keycloak broker chain caused an unfixable
  auth redirect loop in OpenHands and must not be repeated)
- **No localStorage auth state** — the SPA never stores tokens; all auth state lives in an
  HTTP-only cookie and the server-side `sessions` table
- **Reuse `openhands-litellm`** — do not deploy a second LiteLLM proxy; agent pods call the
  existing `openhands-litellm.openhands.svc.cluster.local:4000` directly
- **Single cluster, single namespace** — `kai` namespace, standard RKE2 patterns

### 1.3 Non-Goals (Phase 1)

- Distributed event bus (Redis pub/sub) — deferred to Phase 3
- Split binary (API vs operator) — deferred to Phase 3
- Multi-cluster agent dispatch
- Billing or rate-limiting per user

---

## 2. Technology Choices

### 2.1 HTTP Router: `chi`

**Chosen**: `github.com/go-chi/chi/v5`

**Why chi over gin**:
- Chi is `net/http`-compatible: any stdlib `http.Handler` middleware works without adapter shims
- Gin couples all handlers to its custom `*gin.Context`; every third-party middleware requires
  a gin adapter or custom wrapper
- Chi's middleware composition (`r.Use(...)`) is explicit and debuggable
- Smaller binary footprint

**Rejected alternatives**:
- `gorilla/mux` — maintenance mode since 2022
- `gin` — coupled context, adapter tax on stdlib middleware
- `echo` — similar coupling problem to gin
- `net/http` stdlib — no route parameter extraction, too verbose for REST

### 2.2 WebSocket: `gorilla/websocket`

**Chosen**: `github.com/gorilla/websocket`

**Why**:
- Battle-tested; handles partial frames, ping/pong, close handshakes correctly
- Explicit control over write deadlines (essential for slow-consumer policy)
- `nhooyr/websocket` is a valid alternative but gorilla has more real-world exposure in
  production K8s controllers

### 2.3 Kubernetes Client: `controller-runtime`

**Chosen**: `sigs.k8s.io/controller-runtime`

**Why**:
- Informer cache provides efficient list/watch without per-reconcile API calls
- `Reconciler` interface is idiomatic Go operator pattern
- Status sub-resource support is first-class
- Used by every major Kubernetes operator framework (Kubebuilder, Operator SDK)

**Rejected**: `client-go` raw — would require manual informer setup; more code, more bugs

### 2.4 Database Client: `pgx/v5 + pgxpool`

**Chosen**: `github.com/jackc/pgx/v5` with `pgxpool`

**Why**:
- Native PostgreSQL binary protocol (not database/sql wire encoding)
- `pgxpool` is goroutine-safe, connection-pooling built in
- Supports `pgx.Batch` for multi-statement transactions without ORM overhead
- `database/sql` + `lib/pq` adds two translation layers; pgx removes them

### 2.5 OIDC/JWT: `lestrrat-go/jwx/v2`

**Chosen**: `github.com/lestrrat-go/jwx/v2`

**Why**:
- First-class JWKS auto-refresh via `jwk.Cache` — fetches `https://auth.hwcopeland.net/application/o/kai/.well-known/jwks.json` on startup, caches with TTL
- RS256 and ES256 out of the box (Authentik uses RS256 by default)
- `jwt.Parse` with `jwt.WithKeySet(cache)` validates signature + standard claims in one call
- Clean interface for extracting custom claims (groups, email, sub)

**Rejected**: `golang-jwt/jwt` — no JWKS fetch built in, requires manual key management

### 2.6 Config: `envconfig`

**Chosen**: `github.com/kelseyhightower/envconfig`

**Why**:
- Twelve-factor: all configuration from environment variables
- Struct tags with `required:"true"` — binary fails fast at startup if a required var is missing
- No config file parsing to maintain; Kubernetes `envFrom` populates everything

### 2.7 Logging: `log/slog` (stdlib)

Go 1.21+ structured logging. No third-party dependency. JSON output in production
(`slog.NewJSONHandler`), text output in development (`slog.NewTextHandler`).

```go
// cmd/kai/main.go
var logLevel = new(slog.LevelVar)
if cfg.Dev {
    slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
} else {
    slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
}
```

### 2.8 Metrics: `prometheus/client_golang`

Standard `/metrics` endpoint. Key metrics:
- `kai_http_requests_total{method,path,status}` — request counter
- `kai_ws_connections_active` — gauge
- `kai_runs_total{status}` — run lifecycle counter
- `kai_agent_sandbox_reconcile_duration_seconds` — operator histogram
- `kai_db_query_duration_seconds{query}` — DB latency histogram

---

## 3. Binary Structure

```
kai/
├── cmd/
│   └── kai/
│       └── main.go          # entry point: parse config, wire components, start servers
├── internal/
│   ├── api/
│   │   ├── router.go        # chi router setup, middleware chain
│   │   ├── auth.go          # PKCE login/callback/logout handlers
│   │   ├── me.go            # GET /api/me
│   │   ├── teams.go         # team CRUD
│   │   ├── runs.go          # run lifecycle handlers
│   │   ├── events.go        # WebSocket upgrade handler
│   │   ├── artifacts.go     # artifact list/download
│   │   ├── apikeys.go       # API key management
│   │   ├── admin.go         # admin-only handlers
│   │   └── internal.go      # POST /internal/callback (port 8081)
│   ├── auth/
│   │   ├── middleware.go    # session cookie validation middleware
│   │   ├── pkce.go          # PKCE challenge/verifier generation
│   │   ├── oidc.go          # Authentik OIDC client (jwx cache, token exchange)
│   │   ├── session.go       # session create/validate/destroy
│   │   └── apikey.go        # API key hash/validate
│   ├── db/
│   │   ├── pool.go          # pgxpool setup + health check
│   │   ├── migrate.go       # golang-migrate runner with advisory lock
│   │   ├── queries/         # one file per domain (users.go, runs.go, sessions.go, ...)
│   │   └── migrations/      # embed.FS migrations (000001_init.up.sql, ...)
│   ├── events/
│   │   ├── hub.go           # in-process EventHub: subscribe/publish/ring buffer
│   │   ├── types.go         # RunEvent discriminated union types
│   │   └── bus.go           # EventBus interface (for Phase 3 Redis swap)
│   ├── operator/
│   │   ├── reconciler.go    # AgentSandbox reconciler (controller-runtime)
│   │   ├── pod.go           # pod template builder for agent sandboxes
│   │   ├── netpol.go        # NetworkPolicy template builder
│   │   └── workspace.go     # workspace PVC lifecycle management
│   └── config/
│       └── config.go        # envconfig struct + validation
```

---

## 4. Configuration

```go
// internal/config/config.go

type Config struct {
    // Server
    ListenAddr         string `envconfig:"LISTEN_ADDR" default:":8080"`
    InternalListenAddr string `envconfig:"INTERNAL_LISTEN_ADDR" default:":8081"`
    Dev                bool   `envconfig:"DEV" default:"false"`

    // Database
    DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

    // Authentik OIDC
    AuthIssuerURL    string `envconfig:"AUTH_ISSUER_URL" required:"true"`
    // e.g. https://auth.hwcopeland.net/application/o/kai/
    AuthClientID     string `envconfig:"AUTH_CLIENT_ID" required:"true"`
    AuthClientSecret string `envconfig:"AUTH_CLIENT_SECRET" required:"true"`
    AuthRedirectURL  string `envconfig:"AUTH_REDIRECT_URL" required:"true"`
    // e.g. https://kai.hwcopeland.net/api/auth/callback

    // Session
    SessionSecret   string `envconfig:"SESSION_SECRET" required:"true"`
    // 32+ random bytes, base64-encoded; used as HMAC key if needed
    SessionDuration time.Duration `envconfig:"SESSION_DURATION" default:"168h"` // 7 days

    // Internal auth
    CallbackToken string `envconfig:"SECRET_CALLBACK_TOKEN" required:"true"`

    // Kubernetes
    KubeNamespace string `envconfig:"KUBE_NAMESPACE" default:"kai"`
    AgentImage    string `envconfig:"AGENT_IMAGE" required:"true"`
    // e.g. zot.hwcopeland.net/kai/kai-agent:build-000042-abc1234

    // LiteLLM
    LiteLLMBaseURL string `envconfig:"LITELLM_BASE_URL" default:"http://openhands-litellm.openhands.svc.cluster.local:4000"`
    LiteLLMAPIKey  string `envconfig:"LITELLM_API_KEY" required:"true"`
}
```

All values come from Kubernetes `Secret` resources populated by ExternalSecrets (Bitwarden).
The `Deployment` uses `envFrom: [{secretRef: {name: kai-api-secrets}}]`.

---

## 5. HTTP API

### 5.1 Router Setup

```go
// internal/api/router.go

func NewRouter(deps *Dependencies) http.Handler {
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.RealIP)
    r.Use(middleware.RequestID)
    r.Use(slogMiddleware)       // structured request logging
    r.Use(prometheusMiddleware) // request counter/duration
    r.Use(middleware.Recoverer)

    // Health / metrics
    r.Get("/healthz", healthzHandler)
    r.Get("/readyz", readyzHandler(deps.DB))
    r.Handle("/metrics", promhttp.Handler())

    // Static SPA (embedded via embed.FS)
    r.Handle("/", spaHandler(deps.StaticFS))
    r.Handle("/assets/*", spaHandler(deps.StaticFS))

    // Auth (no session required)
    r.Route("/api/auth", func(r chi.Router) {
        r.Get("/login", deps.Auth.LoginHandler)
        r.Get("/callback", deps.Auth.CallbackHandler)
        r.Post("/logout", deps.Auth.LogoutHandler)
    })

    // Authenticated API
    r.Route("/api", func(r chi.Router) {
        r.Use(deps.Auth.SessionMiddleware) // 401 if no valid session
        r.Get("/me", deps.API.MeHandler)

        r.Route("/teams", func(r chi.Router) {
            r.Get("/", deps.API.ListTeams)
            r.Post("/", deps.API.CreateTeam)
            r.Get("/{teamID}", deps.API.GetTeam)
            r.Route("/{teamID}/runs", func(r chi.Router) {
                r.Get("/", deps.API.ListRuns)
                r.Post("/", deps.API.CreateRun)
            })
        })

        r.Route("/runs", func(r chi.Router) {
            r.Get("/{runID}", deps.API.GetRun)
            r.Get("/{runID}/events", deps.API.EventsWS) // WS upgrade
            r.Get("/{runID}/artifacts", deps.API.ListArtifacts)
            r.Delete("/{runID}", deps.API.CancelRun)
        })

        r.Route("/artifacts", func(r chi.Router) {
            r.Get("/{artifactID}", deps.API.DownloadArtifact)
        })

        r.Route("/keys", func(r chi.Router) {
            r.Get("/", deps.API.ListAPIKeys)
            r.Post("/", deps.API.CreateAPIKey)
            r.Delete("/{keyID}", deps.API.RevokeAPIKey)
        })

        // Admin only
        r.Route("/admin", func(r chi.Router) {
            r.Use(deps.Auth.AdminMiddleware) // 403 if not in kai-admins group
            r.Get("/users", deps.API.AdminListUsers)
            r.Get("/runs", deps.API.AdminListAllRuns)
        })
    })

    return r
}
```

### 5.2 Auth Middleware

```go
// internal/auth/middleware.go

func (a *AuthService) SessionMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Check for API key header first
        if bearerToken := extractBearer(r); bearerToken != "" {
            user, err := a.validateAPIKey(r.Context(), bearerToken)
            if err == nil {
                ctx := context.WithValue(r.Context(), ctxKeyUser, user)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }
        }

        // 2. Check session cookie
        cookie, err := r.Cookie("kai_session")
        if err != nil {
            http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
            return
        }

        tokenHash := sha256Hex(cookie.Value)
        session, err := a.db.GetSessionByTokenHash(r.Context(), tokenHash)
        if err != nil || session.ExpiresAt.Before(time.Now()) {
            clearSessionCookie(w)
            http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
            return
        }

        user, err := a.db.GetUserByID(r.Context(), session.UserID)
        if err != nil {
            http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
            return
        }

        ctx := context.WithValue(r.Context(), ctxKeyUser, user)
        ctx = context.WithValue(ctx, ctxKeySession, session)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func (a *AuthService) AdminMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user := UserFromContext(r.Context())
        if user == nil || !user.IsAdmin {
            http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### 5.3 PKCE Login Handler

```go
// internal/api/auth.go

func (h *AuthHandlers) LoginHandler(w http.ResponseWriter, r *http.Request) {
    // Generate PKCE pair
    verifier, err := auth.GenerateCodeVerifier() // 32 random bytes, base64url-encoded
    if err != nil {
        http.Error(w, "internal error", 500)
        return
    }
    challenge := auth.S256Challenge(verifier) // SHA-256(verifier), base64url-encoded

    // Generate state token (CSRF)
    state, err := auth.GenerateState() // 16 random bytes, base64url-encoded
    if err != nil {
        http.Error(w, "internal error", 500)
        return
    }

    // Store state → verifier mapping (in-memory with 10-minute TTL)
    h.pkceStore.Set(state, verifier, 10*time.Minute)

    // Build Authentik authorization URL
    authURL := h.oidc.AuthCodeURL(state,
        oauth2.SetAuthURLParam("code_challenge", challenge),
        oauth2.SetAuthURLParam("code_challenge_method", "S256"),
    )

    http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *AuthHandlers) CallbackHandler(w http.ResponseWriter, r *http.Request) {
    state := r.URL.Query().Get("state")
    code := r.URL.Query().Get("code")

    // Validate state
    verifier, ok := h.pkceStore.GetAndDelete(state)
    if !ok {
        http.Error(w, "invalid state", http.StatusBadRequest)
        return
    }

    // Exchange code for tokens
    token, err := h.oidc.Exchange(r.Context(), code,
        oauth2.SetAuthURLParam("code_verifier", verifier),
    )
    if err != nil {
        http.Error(w, "token exchange failed", http.StatusBadRequest)
        return
    }

    // Validate and parse ID token
    idToken, err := h.oidc.VerifyIDToken(r.Context(), token)
    if err != nil {
        http.Error(w, "invalid id token", http.StatusUnauthorized)
        return
    }

    // Extract claims
    claims, err := auth.ExtractClaims(idToken)
    if err != nil {
        http.Error(w, "invalid claims", http.StatusUnauthorized)
        return
    }

    // Upsert user
    user, err := h.db.UpsertUser(r.Context(), db.UpsertUserParams{
        ID:          claims.Sub,
        Email:       claims.Email,
        DisplayName: claims.Name,
        AvatarURL:   claims.Picture,
        IsAdmin:     containsGroup(claims.Groups, "kai-admins"),
    })
    if err != nil {
        http.Error(w, "db error", http.StatusInternalServerError)
        return
    }

    // Create session
    rawToken, err := auth.GenerateSessionToken() // 32 random bytes, base64url-encoded
    if err != nil {
        http.Error(w, "internal error", 500)
        return
    }
    tokenHash := auth.SHA256Hex(rawToken)

    _, err = h.db.CreateSession(r.Context(), db.CreateSessionParams{
        ID:          uuid.New(),
        UserID:      user.ID,
        TokenHash:   tokenHash,
        ExpiresAt:   time.Now().Add(h.cfg.SessionDuration),
        UserAgent:   r.UserAgent(),
        IPAddr:      realIP(r),
    })
    if err != nil {
        http.Error(w, "db error", http.StatusInternalServerError)
        return
    }

    // Set HTTP-only cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "kai_session",
        Value:    rawToken,
        Path:     "/",
        HttpOnly: true,
        Secure:   true,
        SameSite: http.SameSiteStrictMode,
        Expires:  time.Now().Add(h.cfg.SessionDuration),
    })

    http.Redirect(w, r, "/", http.StatusFound)
}
```

---

## 6. WebSocket Event Hub

### 6.1 Design

The in-process EventHub connects agent callbacks (via `POST /internal/callback`) to
browser WebSocket connections (`GET /api/runs/:runID/events`).

```
Agent Pod → POST /internal/callback → EventHub.Publish(runID, event)
                                              ↓
                                    Hub fan-out to all subscribers
                                              ↓
                                    WS Client 1 (browser tab A)
                                    WS Client 2 (browser tab B)
```

### 6.2 Hub Implementation

```go
// internal/events/hub.go

const (
    RingBufferSize    = 2000
    SlowConsumerDelay = 5 * time.Second
    WriteBufSize      = 256 // events, not bytes
)

type Hub struct {
    mu          sync.RWMutex
    subscribers map[string][]*Subscriber // runID → subscribers
    buffers     map[string]*RingBuffer   // runID → event history
}

type Subscriber struct {
    ch     chan RunEvent
    runID  string
    closed atomic.Bool
}

func (h *Hub) Subscribe(runID string) (*Subscriber, []RunEvent) {
    h.mu.Lock()
    defer h.mu.Unlock()

    sub := &Subscriber{
        ch:    make(chan RunEvent, WriteBufSize),
        runID: runID,
    }
    h.subscribers[runID] = append(h.subscribers[runID], sub)

    // Return existing buffer so new subscribers catch up
    history := h.buffers[runID].Snapshot()
    return sub, history
}

func (h *Hub) Publish(runID string, event RunEvent) {
    h.mu.Lock()
    // Append to ring buffer (evicts oldest if full)
    if h.buffers[runID] == nil {
        h.buffers[runID] = NewRingBuffer(RingBufferSize)
    }
    h.buffers[runID].Push(event)
    subs := h.subscribers[runID]
    h.mu.Unlock()

    // Fan-out (non-blocking with slow-consumer policy)
    for _, sub := range subs {
        select {
        case sub.ch <- event:
        default:
            // Slow consumer: attempt drain with deadline
            go h.evictSlowConsumer(sub)
        }
    }
}

func (h *Hub) evictSlowConsumer(sub *Subscriber) {
    timer := time.NewTimer(SlowConsumerDelay)
    defer timer.Stop()
    select {
    case sub.ch <- RunEvent{Type: "buffer_overflow"}:
    case <-timer.C:
        sub.closed.Store(true)
        close(sub.ch)
        h.unsubscribe(sub)
    }
}
```

### 6.3 WebSocket Handler

```go
// internal/api/events.go

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return r.Header.Get("Origin") == "https://kai.hwcopeland.net"
    },
}

func (h *APIHandlers) EventsWS(w http.ResponseWriter, r *http.Request) {
    runID := chi.URLParam(r, "runID")
    user := auth.UserFromContext(r.Context())

    // Authorize: user must be member of the team that owns this run
    if err := h.db.AssertRunAccess(r.Context(), runID, user.ID); err != nil {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    sub, history := h.hub.Subscribe(runID)
    defer h.hub.Unsubscribe(sub)

    // Send historical events first
    for _, evt := range history {
        if err := writeJSON(conn, evt); err != nil {
            return
        }
    }

    // Stream live events
    pingTicker := time.NewTicker(30 * time.Second)
    defer pingTicker.Stop()

    for {
        select {
        case evt, ok := <-sub.ch:
            if !ok {
                return // slow consumer evicted
            }
            conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := writeJSON(conn, evt); err != nil {
                return
            }
        case <-pingTicker.C:
            conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        case <-r.Context().Done():
            return
        }
    }
}
```

---

## 7. Event Types

```go
// internal/events/types.go

type EventType string

const (
    EventAgentThinking  EventType = "agent_thinking"
    EventAgentAction    EventType = "agent_action"
    EventAgentMessage   EventType = "agent_message"
    EventAgentDone      EventType = "agent_done"
    EventAgentError     EventType = "agent_error"
    EventRunComplete    EventType = "run_complete"
    EventRunError       EventType = "run_error"
    EventBufferOverflow EventType = "buffer_overflow"
)

type RunEvent struct {
    ID          string          `json:"id"`           // UUID
    RunID       string          `json:"run_id"`
    AgentTaskID string          `json:"agent_task_id,omitempty"`
    AgentRole   string          `json:"agent_role,omitempty"`
    Type        EventType       `json:"type"`
    Timestamp   time.Time       `json:"timestamp"`
    Payload     json.RawMessage `json:"payload"`
}

// Payload types (discriminated by Type)

type AgentThinkingPayload struct {
    Thought string `json:"thought"`
}

type AgentActionPayload struct {
    Tool   string          `json:"tool"`   // "bash", "read_file", "write_file", "search"
    Input  json.RawMessage `json:"input"`
    Output json.RawMessage `json:"output,omitempty"`
}

type AgentMessagePayload struct {
    Content string `json:"content"`
    To      string `json:"to,omitempty"` // recipient agent role, empty = broadcast
}

type AgentDonePayload struct {
    Summary     string `json:"summary"`
    ArtifactIDs []string `json:"artifact_ids,omitempty"`
}

type RunCompletePayload struct {
    Summary     string   `json:"summary"`
    ArtifactIDs []string `json:"artifact_ids"`
    DurationMs  int64    `json:"duration_ms"`
}
```

---

## 8. Kubernetes Operator (CRD Reconciler)

### 8.1 Operator Overview

The Go binary starts a `controller-runtime` manager alongside the HTTP server:

```go
// cmd/kai/main.go (simplified)

func main() {
    cfg := config.MustLoad()

    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:             scheme,
        Namespace:          cfg.KubeNamespace,
        MetricsBindAddress: ":9090",
        HealthProbeBindAddress: ":8082",
    })

    // Register reconciler
    if err := (&operator.AgentSandboxReconciler{
        Client: mgr.GetClient(),
        Hub:    hub,
        DB:     db,
        Cfg:    cfg,
    }).SetupWithManager(mgr); err != nil {
        slog.Error("setup reconciler failed", "err", err)
        os.Exit(1)
    }

    // Start HTTP server in goroutine
    go func() {
        if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            slog.Error("http server failed", "err", err)
            os.Exit(1)
        }
    }()

    // Start internal callback listener
    go func() {
        if err := internalServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            slog.Error("internal server failed", "err", err)
            os.Exit(1)
        }
    }()

    // Start manager (blocks until context cancelled)
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        slog.Error("manager failed", "err", err)
        os.Exit(1)
    }
}
```

### 8.2 Reconciler

```go
// internal/operator/reconciler.go

type AgentSandboxReconciler struct {
    client.Client
    Hub *events.Hub
    DB  *db.Queries
    Cfg *config.Config
}

func (r *AgentSandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := slog.With("sandbox", req.NamespacedName)

    var sandbox kaiv1alpha1.AgentSandbox
    if err := r.Get(ctx, req.NamespacedName, &sandbox); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    switch sandbox.Status.Phase {
    case "", kaiv1alpha1.PhasePending:
        return r.reconcilePending(ctx, log, &sandbox)
    case kaiv1alpha1.PhaseRunning:
        return r.reconcileRunning(ctx, log, &sandbox)
    case kaiv1alpha1.PhaseTerminating:
        return r.reconcileTerminating(ctx, log, &sandbox)
    case kaiv1alpha1.PhaseTerminated:
        return r.reconcileTerminated(ctx, log, &sandbox)
    }
    return ctrl.Result{}, nil
}

func (r *AgentSandboxReconciler) reconcilePending(ctx context.Context, log *slog.Logger, sandbox *kaiv1alpha1.AgentSandbox) (ctrl.Result, error) {
    // Create NetworkPolicy for this sandbox
    netpol := r.buildNetworkPolicy(sandbox)
    if err := r.Create(ctx, netpol); err != nil && !apierrors.IsAlreadyExists(err) {
        return ctrl.Result{}, fmt.Errorf("create netpol: %w", err)
    }

    // Create Pod
    pod := r.buildAgentPod(sandbox)
    if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
        return ctrl.Result{}, fmt.Errorf("create pod: %w", err)
    }

    // Update status
    sandbox.Status.Phase = kaiv1alpha1.PhaseRunning
    sandbox.Status.PodRef = pod.Name
    sandbox.Status.StartTime = &metav1.Time{Time: time.Now()}
    if err := r.Status().Update(ctx, sandbox); err != nil {
        return ctrl.Result{}, err
    }
    log.Info("sandbox running", "pod", pod.Name)

    // Requeue after timeout to enforce deadline
    return ctrl.Result{RequeueAfter: time.Duration(sandbox.Spec.TimeoutSeconds) * time.Second}, nil
}

func (r *AgentSandboxReconciler) reconcileRunning(ctx context.Context, log *slog.Logger, sandbox *kaiv1alpha1.AgentSandbox) (ctrl.Result, error) {
    // Check if pod is still running
    var pod corev1.Pod
    if err := r.Get(ctx, types.NamespacedName{Name: sandbox.Status.PodRef, Namespace: sandbox.Namespace}, &pod); err != nil {
        if apierrors.IsNotFound(err) {
            // Pod disappeared — mark terminated with error
            return r.markTerminated(ctx, sandbox, "pod_missing", "Pod disappeared unexpectedly")
        }
        return ctrl.Result{}, err
    }

    // Check timeout
    if sandbox.Status.StartTime != nil {
        elapsed := time.Since(sandbox.Status.StartTime.Time)
        timeout := time.Duration(sandbox.Spec.TimeoutSeconds) * time.Second
        if elapsed > timeout {
            log.Warn("sandbox timed out", "elapsed", elapsed, "timeout", timeout)
            return r.evictPod(ctx, sandbox, &pod, "timeout")
        }
    }

    // Pod completed normally
    if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
        status := "succeeded"
        msg := ""
        if pod.Status.Phase == corev1.PodFailed {
            status = "failed"
            msg = extractPodFailureReason(&pod)
        }
        return r.markTerminated(ctx, sandbox, status, msg)
    }

    // Requeue to check again
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AgentSandboxReconciler) reconcileTerminated(ctx context.Context, log *slog.Logger, sandbox *kaiv1alpha1.AgentSandbox) (ctrl.Result, error) {
    // TTL garbage collection: 300 seconds after termination (matches DockingJob pattern)
    if sandbox.Status.EndTime == nil {
        return ctrl.Result{}, nil
    }
    ttl := 300 * time.Second
    age := time.Since(sandbox.Status.EndTime.Time)
    if age < ttl {
        return ctrl.Result{RequeueAfter: ttl - age}, nil
    }
    log.Info("deleting expired sandbox", "age", age)
    return ctrl.Result{}, r.Delete(ctx, sandbox)
}
```

---

## 9. Internal Callback Listener

Agent pods report completion to the backend via a dedicated internal HTTP server.
This listener binds to `:8081` (not exposed via Ingress; only reachable in-cluster).

```go
// internal/api/internal.go

type CallbackRequest struct {
    AgentTaskID string          `json:"agent_task_id"`
    Status      string          `json:"status"` // "succeeded", "failed"
    Summary     string          `json:"summary,omitempty"`
    ArtifactIDs []string        `json:"artifact_ids,omitempty"`
    Error       string          `json:"error,omitempty"`
    Events      []events.RunEvent `json:"events,omitempty"`
}

func (h *InternalHandlers) CallbackHandler(w http.ResponseWriter, r *http.Request) {
    // Validate callback token (shared secret, not OIDC)
    if r.Header.Get("X-Kai-Callback-Token") != h.cfg.CallbackToken {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    var req CallbackRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    // Publish events to hub (fan-out to WS subscribers)
    for _, evt := range req.Events {
        h.hub.Publish(evt.RunID, evt)
    }

    // Update agent_task status in DB
    if err := h.db.UpdateAgentTaskStatus(r.Context(), db.UpdateAgentTaskStatusParams{
        ID:       req.AgentTaskID,
        Status:   req.Status,
        EndTime:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
    }); err != nil {
        slog.Error("update agent task failed", "err", err, "task_id", req.AgentTaskID)
    }

    // Publish terminal event
    runID, _ := h.db.GetRunIDForTask(r.Context(), req.AgentTaskID)
    termEvt := events.RunEvent{
        ID:          uuid.NewString(),
        RunID:       runID,
        AgentTaskID: req.AgentTaskID,
        Type:        events.EventAgentDone,
        Timestamp:   time.Now(),
    }
    if req.Status == "failed" {
        termEvt.Type = events.EventAgentError
    }
    h.hub.Publish(runID, termEvt)

    w.WriteHeader(http.StatusNoContent)
}
```

---

## 10. Run Lifecycle (Orchestrator)

When `POST /api/teams/:teamID/runs` is called, the backend orchestrates the agent team:

```go
// internal/api/runs.go

func (h *APIHandlers) CreateRun(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    var req CreateRunRequest
    json.NewDecoder(r.Body).Decode(&req)
    user := auth.UserFromContext(r.Context())
    teamID := chi.URLParam(r, "teamID")

    // 2. Create team_run record in DB
    run, err := h.db.CreateTeamRun(r.Context(), db.CreateTeamRunParams{
        ID:        uuid.New(),
        TeamID:    teamID,
        UserID:    user.ID,
        Objective: req.Objective,
        Status:    "pending",
    })

    // 3. Create workspace PVC for this run (shared by all agent pods)
    if err := h.operator.CreateWorkspacePVC(r.Context(), run.ID.String()); err != nil {
        http.Error(w, "failed to create workspace", 500)
        return
    }

    // 4. Start run orchestration in background goroutine
    go h.runOrchestrator.Start(run.ID.String(), req.Objective)

    // 5. Return run ID immediately (client polls or subscribes to WS)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(run)
}
```

The `runOrchestrator` manages the agent team sequence:

```
Phase 1: Launch planner AgentSandbox → waits for callback
Phase 2: Launch researcher AgentSandbox (parallel-capable in Phase 2)
Phase 3: Launch coder_1 + coder_2 AgentSandboxes
Phase 4: Launch reviewer AgentSandbox → waits for callback
Phase 5: Mark run complete, publish EventRunComplete, cleanup workspace PVC
```

---

## 11. SPA Embedding

The React build output (`dist/`) is embedded in the Go binary via `embed.FS`:

```go
// cmd/kai/main.go

//go:embed ui/dist
var staticFS embed.FS

// In router setup:
func spaHandler(fs embed.FS) http.Handler {
    sub, _ := fs.Sub("ui/dist")
    fsHandler := http.FileServer(http.FS(sub))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Serve static assets directly
        if _, err := sub.Open(r.URL.Path[1:]); err == nil {
            fsHandler.ServeHTTP(w, r)
            return
        }
        // All other paths → index.html (SPA client-side routing)
        index, _ := sub.Open("index.html")
        defer index.Close()
        content, _ := io.ReadAll(index)
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write(content)
    })
}
```

The CI build pipeline (`build-kai.yml`) runs `npm run build` before `go build`, ensuring
the embedded FS is always up-to-date.

---

## 12. Error Handling Strategy

### 12.1 HTTP Layer

All errors return JSON:

```go
type APIError struct {
    Error   string `json:"error"`
    Code    string `json:"code,omitempty"`    // machine-readable, e.g. "run_not_found"
    Details string `json:"details,omitempty"` // human-readable, only in Dev mode
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(APIError{Error: msg, Code: code})
}
```

### 12.2 Database Layer

- `pgx.ErrNoRows` → handler returns 404 (not 500)
- All DB functions return typed errors; handlers check with `errors.Is`
- Connection errors → 503 with Retry-After header

### 12.3 Operator Layer

- Reconciler errors → `ctrl.Result{RequeueAfter: backoff}` (controller-runtime handles backoff)
- Permanent errors (e.g., invalid spec) → set condition on CRD status, do not requeue

---

## 13. Observability

### 13.1 Health Endpoints

```
GET /healthz  → 200 always (liveness)
GET /readyz   → 200 if DB ping succeeds + K8s cache synced (readiness)
```

### 13.2 Structured Logging

Every request logs: `method`, `path`, `status`, `duration_ms`, `request_id`, `user_id` (if authenticated).
Every reconcile logs: `sandbox`, `phase_from`, `phase_to`, `duration_ms`.

### 13.3 Prometheus Metrics

```
kai_http_requests_total{method, path_pattern, status_code}
kai_http_request_duration_seconds{method, path_pattern}
kai_ws_connections_active{run_id}
kai_runs_total{status}
kai_agent_sandboxes_total{phase, agent_role}
kai_operator_reconcile_duration_seconds
kai_db_query_duration_seconds{operation}
```

---

## 14. Testing Approach

### 14.1 Unit Tests

- Auth middleware: table-driven tests with mock DB
- PKCE handlers: mock OIDC server (httptest)
- EventHub: race detector tests (`go test -race`)
- Reconciler: `envtest` (controller-runtime test environment)

### 14.2 Integration Tests

- `testcontainers-go` for PostgreSQL — real DB, not mocks
- Full request cycle tests via `httptest.NewServer`

### 14.3 Test Commands

```bash
# Unit + integration
go test ./... -race

# With test DB
DATABASE_URL=postgres://kai:test@localhost:5432/kai_test go test ./internal/db/...

# Operator (requires envtest binaries)
KUBEBUILDER_ASSETS=$(setup-envtest use -p path) go test ./internal/operator/...
```

---

## 15. Open Questions

### OQ-1: Agent Image Contents

What tools/languages are pre-installed in `kai-agent`? This determines sandbox
security requirements. Options:
- Minimal: bash, curl, git only (most secure)
- Development: + Python, Node, Go toolchains (higher risk, more useful)
- Configurable: base image per agent role

**Decision needed before Phase 1 closes.**

### OQ-2: PKCE State Store

Phase 1 uses in-memory state store for `state → code_verifier` mapping. This means
PKCE login breaks if the pod restarts mid-flow (rare but possible). Phase 2 option:
use PostgreSQL with 10-minute TTL rows. Impact is low for Phase 1 given single-pod deployment.

### OQ-3: Monolith vs Split Binary

Phase 1 ships API + operator as one binary. If the operator reconcile loop becomes
CPU/memory heavy under load (many concurrent runs), split into `kai-api` and
`kai-operator` deployments. The interface boundaries are already clean.

### OQ-4: WS Auth on Upgrade

WebSocket `Upgrade` requests don't always send cookies in all browser/proxy configs.
The `CheckOrigin` guard + SameSite=Strict cookie should be sufficient for browsers,
but need to verify with Cilium Gateway API proxy behavior.

### OQ-5: LiteLLM Namespace Access

Agent pods in the `kai` namespace need egress to
`openhands-litellm.openhands.svc.cluster.local:4000`. This requires a NetworkPolicy
that explicitly allows cross-namespace egress. Verify with Cilium NetworkPolicy
semantics (not standard k8s NetworkPolicy — Cilium uses CiliumNetworkPolicy).

### OQ-6: Authentik Application Slug

The Authentik OIDC application needs to be created at `auth.hwcopeland.net`.
Suggested slug: `kai`. Client ID and secret go into Bitwarden, then ExternalSecret.
This is a manual Authentik setup step before Phase 1 can be tested end-to-end.

### OQ-7: Coder Parallelism

Phase 1 default: 2 coder agents per run. This is configurable via
`AGENT_CODER_COUNT` env var. Confirm acceptable for MVP resource usage.

---

## 16. Phase Plan

### Phase 1 — Foundation (binary compiles and auth works)
- Binary builds with chi + pgx + controller-runtime wired
- Database migrations run on startup
- `/api/auth/login` → `/api/auth/callback` → `GET /api/me` round-trip works
- AgentSandbox CRD installed; reconciler creates pods
- Gate: Real OIDC login with Authentik succeeds, session cookie set

### Phase 2 — Run Lifecycle
- `POST /api/teams/:id/runs` creates AgentSandbox CRDs
- Agent pod callback via `POST /internal/callback` updates DB
- WebSocket delivers events to browser
- Gate: Full run lifecycle from API call to WS event stream

### Phase 3 — Production Hardening
- Replace in-memory PKCE store with PostgreSQL
- Add Redis pub/sub EventBus (replace in-process Hub)
- Split binary if needed
- Full Prometheus metrics + Grafana dashboard
- Gate: Handles 10 concurrent runs without WS event loss

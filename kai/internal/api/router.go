package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/config"
	"github.com/hwcopeland/iac/kai/internal/db"
	"github.com/hwcopeland/iac/kai/internal/events"
)

// Server holds the shared dependencies wired at startup.
type Server struct {
	oidc      *auth.OIDCClient
	pool      *db.Pool
	cfg       *config.Config
	hub       *events.Hub
	k8sClient sigs.Client // may be nil; checked before use in orchestrateRun
}

// NewServer constructs an API server with all required dependencies.
// k8sClient may be nil — when nil, orchestrateRun simulates agent completion.
func NewServer(
	oidc *auth.OIDCClient,
	pool *db.Pool,
	cfg *config.Config,
	hub *events.Hub,
	k8sClient sigs.Client,
) *Server {
	return &Server{
		oidc:      oidc,
		pool:      pool,
		cfg:       cfg,
		hub:       hub,
		k8sClient: k8sClient,
	}
}

// Router builds and returns the chi router with all routes registered.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// ── Public: auth flow ────────────────────────────────────────────────────
	r.Get("/api/auth/login", s.handleAuthLogin)
	r.Get("/api/auth/callback", s.handleAuthCallback)
	r.Get("/api/auth/logout", s.handleAuthLogout)

	// ── Protected: session required ──────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(auth.SessionMiddleware(s.pool))

		// Phase 1
		r.Get("/api/me", s.handleMe)

		// Phase 2 — teams
		r.Post("/api/teams", s.handleCreateTeam)
		r.Get("/api/teams", s.handleListTeams)
		r.Get("/api/teams/{teamID}", s.handleGetTeam)

		// Phase 2 — runs
		r.Post("/api/teams/{teamID}/runs", s.handleCreateRun)
		r.Get("/api/teams/{teamID}/runs", s.handleListRuns)
		r.Get("/api/runs/{runID}", s.handleGetRun)
		r.Post("/api/runs/{runID}/cancel", s.handleCancelRun)

		// Phase 3 placeholder — WebSocket event stream
		r.Get("/api/runs/{runID}/events", s.handleRunEvents)
	})

	// ── Infrastructure ───────────────────────────────────────────────────────
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}

// writeJSON is a convenience helper that sets Content-Type, writes the status
// code, and JSON-encodes v. Encoding errors are silently swallowed because
// response headers have already been sent by the time the encoder runs.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		_ = err // headers already sent; nothing useful we can do
	}
}


package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/db"
)

// Server holds the shared dependencies wired at startup.
type Server struct {
	oidc *auth.OIDCClient
	pool *db.Pool
}

// NewServer constructs an API server with the given OIDC client and DB pool.
func NewServer(oidc *auth.OIDCClient, pool *db.Pool) *Server {
	return &Server{oidc: oidc, pool: pool}
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
		r.Get("/api/me", s.handleMe)
		// Phase 2+: runs, agents, websocket routes registered here.
	})

	// ── Infrastructure ───────────────────────────────────────────────────────
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// ── SPA fallback ─────────────────────────────────────────────────────────
	// Phase 3: go:embed of built UI wired in cmd/kai/main.go.
	// For now, return 404 for unknown paths so the SPA handler can be added
	// without breaking existing routes.

	return r
}

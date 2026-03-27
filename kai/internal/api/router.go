package api

import (
	"encoding/json"
	"io/fs"
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
	ui        fs.FS       // embedded ui/dist; may be nil in tests
}

// NewServer constructs an API server with all required dependencies.
// k8sClient may be nil — when nil, orchestrateRun simulates agent completion.
// ui may be nil — when nil, the SPA catch-all handler is skipped.
func NewServer(
	oidc *auth.OIDCClient,
	pool *db.Pool,
	cfg *config.Config,
	hub *events.Hub,
	k8sClient sigs.Client,
	ui fs.FS,
) *Server {
	return &Server{
		oidc:      oidc,
		pool:      pool,
		cfg:       cfg,
		hub:       hub,
		k8sClient: k8sClient,
		ui:        ui,
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

		// Phase 3 — WebSocket event stream
		r.Get("/api/runs/{runID}/events", s.handleRunEvents)

		// Phase 4 — artifacts
		r.Get("/api/runs/{runID}/artifacts", s.handleListArtifacts)
		r.Get("/api/artifacts/{artifactID}/download", s.handleDownloadArtifact)

		// Phase 4 — API keys
		r.Get("/api/keys", s.handleListAPIKeys)
		r.Post("/api/keys", s.handleCreateAPIKey)
		r.Delete("/api/keys/{keyID}", s.handleRevokeAPIKey)

		// Phase 4 — admin (IsAdmin check inside handler; returns 403 for non-admins)
		r.Get("/api/admin/users", s.handleAdminListUsers)
		r.Get("/api/admin/runs", s.handleAdminListRuns)
	})

	// ── Infrastructure ───────────────────────────────────────────────────────
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// ── SPA catch-all: serve embedded ui/dist, fall back to index.html ───────
	if s.ui != nil {
		fileServer := http.FileServer(http.FS(s.ui))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Try to serve the exact file; if not found serve index.html so
			// TanStack Router can handle client-side navigation.
			_, err := s.ui.Open(r.URL.Path[1:]) // strip leading /
			if err != nil {
				// Serve index.html for all unknown paths (SPA routing).
				r.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, r)
		})
	}

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


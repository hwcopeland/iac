package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
)

// POST /api/teams
//
// Creates a new team and adds the authenticated user as owner.
// Body: {"name":"...", "slug":"..."}
// Returns: {"id":"...", "name":"...", "slug":"..."}
func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.Slug == "" {
		http.Error(w, "name and slug are required", http.StatusBadRequest)
		return
	}

	teamID, err := queries.CreateTeam(r.Context(), s.pool, body.Name, body.Slug, user.ID)
	if err != nil {
		slog.Error("create team", "err", err, "user", user.ID)
		http.Error(w, "failed to create team", http.StatusInternalServerError)
		return
	}

	if err := queries.AddTeamMember(r.Context(), s.pool, teamID, user.ID, "owner"); err != nil {
		// Team was created; best-effort ownership add — log but do not fail the request.
		slog.Error("add team owner member", "err", err, "teamID", teamID, "user", user.ID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":   teamID,
		"name": body.Name,
		"slug": body.Slug,
	})
}

// GET /api/teams
//
// Lists teams where the authenticated user is a member.
func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	teams, err := queries.ListTeamsForUser(r.Context(), s.pool, user.ID)
	if err != nil {
		slog.Error("list teams", "err", err, "user", user.ID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type teamResponse struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	out := make([]teamResponse, len(teams))
	for i, t := range teams {
		out[i] = teamResponse{ID: t.ID, Name: t.Name, Slug: t.Slug}
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/teams/{teamID}
//
// Returns a single team. Only team members may access it.
// Returns 404 for both "not found" and "not a member" to avoid leaking existence.
func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	teamID := chi.URLParam(r, "teamID")

	isMember, err := queries.IsTeamMember(r.Context(), s.pool, teamID, user.ID)
	if err != nil {
		slog.Error("check team membership", "err", err, "teamID", teamID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	team, err := queries.GetTeam(r.Context(), s.pool, teamID)
	if err != nil {
		slog.Error("get team", "err", err, "teamID", teamID)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":   team.ID,
		"name": team.Name,
		"slug": team.Slug,
	})
}

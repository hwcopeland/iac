package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
)

// GET /api/admin/users
//
// Returns all users in the system, ordered by creation time descending.
// Requires the authenticated user to have is_admin = true.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	users, err := queries.ListAllUsers(r.Context(), s.pool)
	if err != nil {
		slog.Error("handleAdminListUsers: list", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type userOut struct {
		ID          string    `json:"id"`
		Email       string    `json:"email"`
		DisplayName string    `json:"display_name"`
		IsAdmin     bool      `json:"is_admin"`
		CreatedAt   time.Time `json:"created_at"`
	}

	out := make([]userOut, len(users))
	for i, u := range users {
		out[i] = userOut{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			IsAdmin:     u.IsAdmin,
			CreatedAt:   u.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/admin/runs
//
// Returns up to 100 runs across all teams, ordered newest first.
// Requires the authenticated user to have is_admin = true.
func (s *Server) handleAdminListRuns(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	runs, err := queries.ListAllRuns(r.Context(), s.pool, 100)
	if err != nil {
		slog.Error("handleAdminListRuns: list", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type runOut struct {
		ID          string    `json:"id"`
		TeamID      string    `json:"team_id"`
		InitiatedBy string    `json:"initiated_by"`
		Objective   string    `json:"objective"`
		Status      string    `json:"status"`
		Model       string    `json:"model"`
		CreatedAt   time.Time `json:"created_at"`
	}

	out := make([]runOut, len(runs))
	for i, run := range runs {
		out[i] = runOut{
			ID:          run.ID,
			TeamID:      run.TeamID,
			InitiatedBy: run.InitiatedBy,
			Objective:   run.Objective,
			Status:      run.Status,
			Model:       run.Model,
			CreatedAt:   run.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

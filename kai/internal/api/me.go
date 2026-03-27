package api

import (
	"encoding/json"
	"net/http"

	"github.com/hwcopeland/iac/kai/internal/auth"
)

// GET /api/me (requires SessionMiddleware)
//
// Returns the authenticated user's profile as JSON.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	role := "user"
	if user.IsAdmin {
		role = "admin"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
		"avatar_url":   user.AvatarURL,
		"role":         role,
	}); err != nil {
		// Response headers already sent; nothing meaningful to do.
		_ = err
	}
}

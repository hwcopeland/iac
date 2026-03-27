package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
)

// GET /api/keys
//
// Lists all API keys for the authenticated user. The key hash is never returned.
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keys, err := queries.ListAPIKeys(r.Context(), s.pool, user.ID)
	if err != nil {
		slog.Error("handleListAPIKeys: list", "err", err, "userID", user.ID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Normalise nil slice → empty JSON array.
	if keys == nil {
		keys = []queries.APIKey{}
	}
	writeJSON(w, http.StatusOK, keys)
}

// POST /api/keys
//
// Creates a new API key for the authenticated user.
// Body: {"name":"my key"}
// Returns the raw key exactly once — it cannot be recovered after this response.
func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	rawKey, keyHash, keyPrefix, err := auth.GenerateAPIKey()
	if err != nil {
		slog.Error("handleCreateAPIKey: generate key", "err", err)
		http.Error(w, "failed to generate api key", http.StatusInternalServerError)
		return
	}

	id, err := queries.CreateAPIKey(r.Context(), s.pool, user.ID, body.Name, keyHash, keyPrefix)
	if err != nil {
		slog.Error("handleCreateAPIKey: insert", "err", err, "userID", user.ID)
		http.Error(w, "failed to create api key", http.StatusInternalServerError)
		return
	}

	// raw key shown once — the caller must copy it; it cannot be retrieved again.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"name":       body.Name,
		"key":        rawKey,
		"key_prefix": keyPrefix,
	})
}

// DELETE /api/keys/{keyID}
//
// Revokes an API key. The delete is scoped to the authenticated user so callers
// can only revoke their own keys. Idempotent: deleting a non-existent key returns 204.
func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keyID := chi.URLParam(r, "keyID")

	if err := queries.RevokeAPIKey(r.Context(), s.pool, keyID, user.ID); err != nil {
		slog.Error("handleRevokeAPIKey: delete", "err", err, "keyID", keyID, "userID", user.ID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
)

// GET /api/runs/{runID}/artifacts
//
// Returns all artifacts for a run. Requires team membership for the run's team.
func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	runID := chi.URLParam(r, "runID")

	run, err := queries.GetTeamRun(r.Context(), s.pool, runID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Return 404 to avoid leaking run existence to non-members.
	isMember, err := queries.IsTeamMember(r.Context(), s.pool, run.TeamID, user.ID)
	if err != nil || !isMember {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	artifacts, err := queries.ListArtifacts(r.Context(), s.pool, runID)
	if err != nil {
		slog.Error("handleListArtifacts: list", "err", err, "runID", runID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type artifactOut struct {
		ID          string `json:"id"`
		RunID       string `json:"run_id"`
		Name        string `json:"name"`
		MimeType    string `json:"mime_type"`
		SizeBytes   int64  `json:"size_bytes"`
		StoragePath string `json:"storage_path"`
	}

	out := make([]artifactOut, len(artifacts))
	for i, a := range artifacts {
		out[i] = artifactOut{
			ID:          a.ID,
			RunID:       a.RunID,
			Name:        a.Name,
			MimeType:    a.MimeType,
			SizeBytes:   a.SizeBytes,
			StoragePath: a.StoragePath,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/artifacts/{artifactID}/download
//
// Returns artifact metadata and a 501 stub for binary download.
// Actual file serving from the agent workspace PVC is not implemented in Phase 4.
func (s *Server) handleDownloadArtifact(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	artifactID := chi.URLParam(r, "artifactID")

	artifact, err := queries.GetArtifact(r.Context(), s.pool, artifactID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Authorise: caller must be a member of the run's team.
	run, err := queries.GetTeamRun(r.Context(), s.pool, artifact.RunID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	isMember, err := queries.IsTeamMember(r.Context(), s.pool, run.TeamID, user.ID)
	if err != nil || !isMember {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Binary download from PVC not yet implemented in Phase 4.
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"id":           artifact.ID,
		"name":         artifact.Name,
		"mime_type":    artifact.MimeType,
		"size_bytes":   artifact.SizeBytes,
		"storage_path": artifact.StoragePath,
		"message":      "artifact download from PVC not yet implemented",
	})
}

package api

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/hwcopeland/iac/kai/internal/db/queries"
	"github.com/hwcopeland/iac/kai/internal/events"
)

// InternalRouter returns the http.Handler for the internal-only server
// (default :8081). It is mounted on a separate listener in cmd/kai/main.go
// so it is never reachable from the public internet.
func (s *Server) InternalRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Post("/internal/callback", s.handleCallback)
	return r
}

// POST /internal/callback
//
// Called by agent pods when a task finishes. Validates the shared secret,
// updates the agent_task status in the database, and publishes a task_completed
// event to the in-process EventHub so the orchestrator can advance the pipeline.
//
// Returns 204 No Content on success.
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	// ── Token validation (constant-time) ─────────────────────────────────────
	provided := r.Header.Get("X-Kai-Callback-Token")
	if s.cfg.CallbackToken == "" ||
		subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.CallbackToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// ── Parse body ────────────────────────────────────────────────────────────
	var body struct {
		TaskID  string `json:"taskId"`
		RunID   string `json:"runId"`
		Status  string `json:"status"`  // "succeeded" | "failed"
		Message string `json:"message"` // human-readable detail
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.TaskID == "" || body.RunID == "" {
		http.Error(w, "taskId and runId are required", http.StatusBadRequest)
		return
	}

	// Map agent status vocab to DB vocab (same strings, but validate explicitly).
	taskStatus := "succeeded"
	if body.Status == "failed" {
		taskStatus = "failed"
	}

	// ── Update DB ─────────────────────────────────────────────────────────────
	var errMsg *string
	if body.Message != "" && taskStatus == "failed" {
		errMsg = &body.Message
	}
	if err := queries.UpdateAgentTaskStatus(r.Context(), s.pool, body.TaskID, taskStatus, nil, errMsg); err != nil {
		slog.Error("handleCallback: update task status", "err", err, "taskID", body.TaskID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// ── Publish event to hub ──────────────────────────────────────────────────
	payload, _ := json.Marshal(map[string]string{
		"taskId":  body.TaskID,
		"runId":   body.RunID,
		"status":  body.Status,
		"message": body.Message,
	})
	tid := body.TaskID
	s.hub.Publish(body.RunID, events.RunEvent{
		ID:          uuid.New().String(),
		RunID:       body.RunID,
		AgentTaskID: &tid,
		EventType:   "task_completed",
		Payload:     string(payload),
		CreatedAt:   time.Now(),
	})

	w.WriteHeader(http.StatusNoContent)
}


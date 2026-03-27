package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
	"github.com/hwcopeland/iac/kai/internal/events"
	"github.com/hwcopeland/iac/kai/internal/operator"
)

// agentRoles defines the fixed sequential pipeline executed in every run.
var agentRoles = []string{"planner", "researcher", "coder_1", "coder_2", "reviewer"}

// POST /api/teams/{teamID}/runs
//
// Creates a team_run row, five agent_task rows (one per pipeline stage),
// and starts the orchestrator goroutine.
// Body: {"objective":"...", "model":"claude-sonnet-4.5"}
// Returns: {"id":"...", "teamId":"...", "objective":"...", "status":"pending"}
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	teamID := chi.URLParam(r, "teamID")

	isMember, err := queries.IsTeamMember(r.Context(), s.pool, teamID, user.ID)
	if err != nil {
		slog.Error("handleCreateRun: check membership", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var body struct {
		Objective string `json:"objective"`
		Model     string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Objective == "" {
		http.Error(w, "objective is required", http.StatusBadRequest)
		return
	}
	if body.Model == "" {
		body.Model = "claude-sonnet-4.5"
	}

	runID, err := queries.CreateTeamRun(r.Context(), s.pool, teamID, user.ID, body.Objective, body.Model)
	if err != nil {
		slog.Error("handleCreateRun: create run", "err", err, "teamID", teamID)
		http.Error(w, "failed to create run", http.StatusInternalServerError)
		return
	}

	// Create all five agent tasks upfront in pipeline order.
	taskIDs := make([]string, 0, len(agentRoles))
	for _, role := range agentRoles {
		taskID, err := queries.CreateAgentTask(r.Context(), s.pool, runID, role)
		if err != nil {
			slog.Error("handleCreateRun: create agent task", "err", err, "runID", runID, "role", role)
			http.Error(w, "failed to create agent tasks", http.StatusInternalServerError)
			return
		}
		taskIDs = append(taskIDs, taskID)
	}

	// Launch orchestrator in background — it outlives the request.
	go s.orchestrateRun(context.Background(), runID, teamID, taskIDs)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        runID,
		"teamId":    teamID,
		"objective": body.Objective,
		"status":    "pending",
	})
}

// GET /api/runs/{runID}
//
// Returns a run with its agent tasks. Only team members may access it.
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
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

	// Authorise via team membership — return 404 to avoid leaking run existence.
	isMember, err := queries.IsTeamMember(r.Context(), s.pool, run.TeamID, user.ID)
	if err != nil || !isMember {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	tasks, err := queries.GetAgentTasksForRun(r.Context(), s.pool, runID)
	if err != nil {
		slog.Error("handleGetRun: get tasks", "err", err, "runID", runID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type taskOut struct {
		ID     string `json:"id"`
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	taskList := make([]taskOut, len(tasks))
	for i, t := range tasks {
		taskList[i] = taskOut{ID: t.ID, Role: t.AgentRole, Status: t.Status}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":        run.ID,
		"teamId":    run.TeamID,
		"objective": run.Objective,
		"status":    run.Status,
		"model":     run.Model,
		"createdAt": run.CreatedAt,
		"tasks":     taskList,
	})
}

// GET /api/teams/{teamID}/runs
//
// Lists up to 50 runs for a team, newest first. Requires team membership.
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	teamID := chi.URLParam(r, "teamID")

	isMember, err := queries.IsTeamMember(r.Context(), s.pool, teamID, user.ID)
	if err != nil {
		slog.Error("handleListRuns: check membership", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	runs, err := queries.ListTeamRuns(r.Context(), s.pool, teamID, 50)
	if err != nil {
		slog.Error("handleListRuns: list runs", "err", err, "teamID", teamID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type runOut struct {
		ID        string    `json:"id"`
		TeamID    string    `json:"teamId"`
		Objective string    `json:"objective"`
		Status    string    `json:"status"`
		Model     string    `json:"model"`
		CreatedAt time.Time `json:"createdAt"`
	}
	out := make([]runOut, len(runs))
	for i, run := range runs {
		out[i] = runOut{
			ID: run.ID, TeamID: run.TeamID, Objective: run.Objective,
			Status: run.Status, Model: run.Model, CreatedAt: run.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/runs/{runID}/cancel
//
// Transitions a non-terminal run to "cancelled" and publishes a run_cancelled
// event so the orchestrator and any subscribers are notified.
func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
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

	isMember, err := queries.IsTeamMember(r.Context(), s.pool, run.TeamID, user.ID)
	if err != nil || !isMember {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch run.Status {
	case "completed", "failed", "cancelled":
		http.Error(w, "run is already in a terminal state", http.StatusConflict)
		return
	}

	if err := queries.UpdateTeamRunStatus(r.Context(), s.pool, runID, "cancelled", nil); err != nil {
		slog.Error("handleCancelRun: update status", "err", err, "runID", runID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	s.hub.Publish(runID, events.RunEvent{
		ID:        uuid.New().String(),
		RunID:     runID,
		EventType: "run_cancelled",
		Payload:   `{"status":"cancelled"}`,
		CreatedAt: time.Now(),
	})

	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

// orchestrateRun drives the sequential agent pipeline for a run.
// It is invoked as a goroutine by handleCreateRun and uses context.Background
// so it outlives the originating request. The k8sClient field may be nil (e.g.
// in tests or local dev); when nil a fake completion event is published after
// 100 ms to simulate an agent completing its work.
func (s *Server) orchestrateRun(ctx context.Context, runID, teamID string, taskIDs []string) {
	for i, taskID := range taskIDs {
		role := agentRoles[i]

		// On the first task, transition the run itself to "running".
		if i == 0 {
			if err := queries.UpdateTeamRunStatus(ctx, s.pool, runID, "running", nil); err != nil {
				slog.Error("orchestrateRun: set run running", "err", err, "runID", runID)
				return
			}
		}

		// Mark the task as running.
		if err := queries.UpdateAgentTaskStatus(ctx, s.pool, taskID, "running", nil, nil); err != nil {
			slog.Error("orchestrateRun: set task running", "err", err, "taskID", taskID)
			_ = queries.UpdateTeamRunStatus(ctx, s.pool, runID, "failed", strPtr("task setup failed"))
			return
		}

		// Publish task_started event.
		tid := taskID // heap-escape for pointer use below
		s.hub.Publish(runID, events.RunEvent{
			ID:          uuid.New().String(),
			RunID:       runID,
			AgentTaskID: &tid,
			EventType:   "task_started",
			Payload:     `{"role":"` + role + `","taskId":"` + taskID + `"}`,
			CreatedAt:   time.Now(),
		})

		// Subscribe to run events BEFORE scheduling the agent so we cannot
		// miss the completion signal even if the sandbox finishes quickly.
		eventCh, cancel := s.hub.Subscribe(ctx, runID)

		if s.k8sClient == nil {
			// No k8s client: simulate agent completion after a short delay.
			go func(tID string) {
				time.Sleep(100 * time.Millisecond)
				s.hub.Publish(runID, events.RunEvent{
					ID:          uuid.New().String(),
					RunID:       runID,
					AgentTaskID: &tID,
					EventType:   "task_completed",
					Payload:     `{"status":"succeeded","taskId":"` + tID + `"}`,
					CreatedAt:   time.Now(),
				})
			}(taskID)
		} else {
			// Create an AgentSandbox CRD; the operator reconciler provisions the pod.
			// KAI_TASK_ID is injected as an extra env var so the agent knows its task UUID
			// and can include it in the /internal/callback POST body.
			sandboxName := "kai-" + taskID
			sandbox := &operator.AgentSandbox{
				TypeMeta: metav1.TypeMeta{
					APIVersion: operator.Group + "/" + operator.Version,
					Kind:       "AgentSandbox",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sandboxName,
					Namespace: s.cfg.KubeNamespace,
				},
				Spec: operator.AgentSandboxSpec{
					RunID:     runID,
					TeamID:    teamID,
					AgentRole: role,
					Image:     s.cfg.AgentImage,
					// Inject taskID so the agent can include it in its callback.
					Env: []corev1.EnvVar{
						{Name: "KAI_TASK_ID", Value: taskID},
					},
				},
			}
			if err := s.k8sClient.Create(ctx, sandbox); err != nil {
				slog.Error("orchestrateRun: create sandbox", "err", err, "taskID", taskID)
				cancel()
				_ = queries.UpdateAgentTaskStatus(ctx, s.pool, taskID, "failed", nil, strPtr("sandbox creation failed"))
				_ = queries.UpdateTeamRunStatus(ctx, s.pool, runID, "failed", strPtr("sandbox creation failed"))
				return
			}
		}

		// Wait for task_completed event for this specific task, or timeout.
		timer := time.NewTimer(s.cfg.RunTimeout)
		taskFinalStatus := "succeeded"

	waitLoop:
		for {
			select {
			case event := <-eventCh:
				if event.EventType == "task_completed" &&
					event.AgentTaskID != nil && *event.AgentTaskID == taskID {
					// Parse the status out of the payload so callback-originated
					// failures propagate correctly.
					var p struct {
						Status string `json:"status"`
					}
					if jerr := json.Unmarshal([]byte(event.Payload), &p); jerr == nil && p.Status == "failed" {
						taskFinalStatus = "failed"
					}
					break waitLoop
				}
			case <-timer.C:
				taskFinalStatus = "timed_out"
				break waitLoop
			case <-ctx.Done():
				cancel()
				timer.Stop()
				return
			}
		}

		cancel()
		timer.Stop()

		// Persist final task status.
		if err := queries.UpdateAgentTaskStatus(ctx, s.pool, taskID, taskFinalStatus, nil, nil); err != nil {
			slog.Error("orchestrateRun: finalize task", "err", err, "taskID", taskID, "status", taskFinalStatus)
		}

		if taskFinalStatus != "succeeded" {
			slog.Warn("orchestrateRun: task did not succeed", "runID", runID, "role", role, "status", taskFinalStatus)
			reason := role + " task " + taskFinalStatus
			_ = queries.UpdateTeamRunStatus(ctx, s.pool, runID, "failed", &reason)
			s.hub.Publish(runID, events.RunEvent{
				ID:          uuid.New().String(),
				RunID:       runID,
				AgentTaskID: &tid,
				EventType:   "run_failed",
				Payload:     `{"reason":"` + reason + `"}`,
				CreatedAt:   time.Now(),
			})
			return
		}
	}

	// All tasks succeeded — mark the run complete.
	if err := queries.UpdateTeamRunStatus(ctx, s.pool, runID, "completed", nil); err != nil {
		slog.Error("orchestrateRun: complete run", "err", err, "runID", runID)
	}
	s.hub.Publish(runID, events.RunEvent{
		ID:        uuid.New().String(),
		RunID:     runID,
		EventType: "run_completed",
		Payload:   `{"status":"completed"}`,
		CreatedAt: time.Now(),
	})
}

// strPtr returns a pointer to s. Helper for passing optional *string params.
func strPtr(s string) *string { return &s }


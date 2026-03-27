package queries

import (
	"context"
	"time"

	"github.com/hwcopeland/iac/kai/internal/db"
)

// ----------------------------------------------------------------------------
// TeamRun
// ----------------------------------------------------------------------------

// TeamRun mirrors the team_runs table.
type TeamRun struct {
	ID          string
	TeamID      string
	InitiatedBy string
	Objective   string
	Status      string
	Model       string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       *string
}

// CreateTeamRun inserts a new team_run row and returns its UUID.
func CreateTeamRun(ctx context.Context, pool *db.Pool, teamID, initiatedBy, objective, model string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO team_runs (team_id, initiated_by, objective, model)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, teamID, initiatedBy, objective, model).Scan(&id)
	return id, err
}

// UpdateTeamRunStatus transitions a run to a new status and stamps the
// appropriate timestamp:
//   - "running"                         → started_at = NOW() (only if not already set)
//   - "completed" | "failed" | "cancelled" → completed_at = NOW()
//
// errMsg is written to the error column unconditionally; pass nil to clear it.
func UpdateTeamRunStatus(ctx context.Context, pool *db.Pool, runID, status string, errMsg *string) error {
	_, err := pool.Exec(ctx, `
		UPDATE team_runs SET
			status       = $2,
			started_at   = CASE
			                   WHEN $2 = 'running' AND started_at IS NULL THEN NOW()
			                   ELSE started_at
			               END,
			completed_at = CASE
			                   WHEN $2 IN ('completed', 'failed', 'cancelled') THEN NOW()
			                   ELSE completed_at
			               END,
			error        = $3
		WHERE id = $1
	`, runID, status, errMsg)
	return err
}

// GetTeamRun returns a single run by UUID.
// Returns pgx.ErrNoRows if not found.
func GetTeamRun(ctx context.Context, pool *db.Pool, runID string) (*TeamRun, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, team_id, initiated_by, objective, status, model,
		       created_at, started_at, completed_at, error
		FROM team_runs
		WHERE id = $1
	`, runID)

	var r TeamRun
	if err := row.Scan(
		&r.ID, &r.TeamID, &r.InitiatedBy, &r.Objective, &r.Status, &r.Model,
		&r.CreatedAt, &r.StartedAt, &r.CompletedAt, &r.Error,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListTeamRuns returns up to limit runs for a team, ordered newest first.
func ListTeamRuns(ctx context.Context, pool *db.Pool, teamID string, limit int) ([]TeamRun, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, team_id, initiated_by, objective, status, model,
		       created_at, started_at, completed_at, error
		FROM team_runs
		WHERE team_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, teamID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []TeamRun
	for rows.Next() {
		var r TeamRun
		if err := rows.Scan(
			&r.ID, &r.TeamID, &r.InitiatedBy, &r.Objective, &r.Status, &r.Model,
			&r.CreatedAt, &r.StartedAt, &r.CompletedAt, &r.Error,
		); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// ----------------------------------------------------------------------------
// AgentTask
// ----------------------------------------------------------------------------

// AgentTask mirrors the agent_tasks table.
type AgentTask struct {
	ID          string
	RunID       string
	AgentRole   string
	Status      string
	SandboxName *string
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       *string
}

// CreateAgentTask inserts a new agent_task row and returns its UUID.
func CreateAgentTask(ctx context.Context, pool *db.Pool, runID, agentRole string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO agent_tasks (run_id, agent_role)
		VALUES ($1, $2)
		RETURNING id
	`, runID, agentRole).Scan(&id)
	return id, err
}

// UpdateAgentTaskStatus transitions a task to a new status, optionally setting
// sandbox_name and error.  Timestamp rules mirror UpdateTeamRunStatus:
//   - "running"                              → started_at = NOW() (if not already set)
//   - "succeeded" | "failed" | "timed_out"  → completed_at = NOW()
func UpdateAgentTaskStatus(ctx context.Context, pool *db.Pool, taskID, status string, sandboxName *string, errMsg *string) error {
	_, err := pool.Exec(ctx, `
		UPDATE agent_tasks SET
			status       = $2,
			sandbox_name = COALESCE($3, sandbox_name),
			started_at   = CASE
			                   WHEN $2 = 'running' AND started_at IS NULL THEN NOW()
			                   ELSE started_at
			               END,
			completed_at = CASE
			                   WHEN $2 IN ('succeeded', 'failed', 'timed_out') THEN NOW()
			                   ELSE completed_at
			               END,
			error        = $4
		WHERE id = $1
	`, taskID, status, sandboxName, errMsg)
	return err
}

// GetAgentTasksForRun returns all tasks for a run ordered by insertion time
// (proxy for creation order since UUIDs are non-sequential).
func GetAgentTasksForRun(ctx context.Context, pool *db.Pool, runID string) ([]AgentTask, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, run_id, agent_role, status, sandbox_name,
		       started_at, completed_at, error
		FROM agent_tasks
		WHERE run_id = $1
		ORDER BY ctid  -- stable physical insertion order within a run
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []AgentTask
	for rows.Next() {
		var t AgentTask
		if err := rows.Scan(
			&t.ID, &t.RunID, &t.AgentRole, &t.Status, &t.SandboxName,
			&t.StartedAt, &t.CompletedAt, &t.Error,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ----------------------------------------------------------------------------
// RunEvent
// ----------------------------------------------------------------------------

// RunEvent mirrors the run_events table.
// Payload is returned as a raw JSON string for the caller to decode.
type RunEvent struct {
	ID          string
	RunID       string
	AgentTaskID *string
	EventType   string
	Payload     string // raw JSON
	CreatedAt   time.Time
}

// CreateRunEvent inserts an event into run_events and returns its UUID.
// agentTaskID may be nil for run-level events not tied to a specific task.
// payload must be a valid JSON string; use "{}" for an empty payload.
func CreateRunEvent(ctx context.Context, pool *db.Pool, runID string, agentTaskID *string, eventType, payload string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO run_events (run_id, agent_task_id, event_type, payload)
		VALUES ($1, $2, $3, $4::jsonb)
		RETURNING id
	`, runID, agentTaskID, eventType, payload).Scan(&id)
	return id, err
}

// GetRunEvents returns up to limit events for a run in chronological order
// (oldest first), suitable for WebSocket history replay.
func GetRunEvents(ctx context.Context, pool *db.Pool, runID string, limit int) ([]RunEvent, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, run_id, agent_task_id, event_type, payload::text, created_at
		FROM run_events
		WHERE run_id = $1
		ORDER BY created_at ASC
		LIMIT $2
	`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []RunEvent
	for rows.Next() {
		var e RunEvent
		if err := rows.Scan(
			&e.ID, &e.RunID, &e.AgentTaskID, &e.EventType, &e.Payload, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ----------------------------------------------------------------------------
// Artifact
// ----------------------------------------------------------------------------

// Artifact mirrors the artifacts table.
// StoragePath is an opaque reference to the backing object store
// (S3 key, PVC-relative path, etc.) — the application layer resolves it.
type Artifact struct {
	ID           string
	RunID        string
	AgentTaskID  *string
	Name         string
	MimeType     string
	SizeBytes    int64
	StoragePath  string
	CreatedAt    time.Time
}

// CreateArtifact inserts an artifact record and returns its UUID.
// taskID may be nil for artifacts not tied to a specific agent task.
func CreateArtifact(ctx context.Context, pool *db.Pool, runID string, agentTaskID *string, name, mimeType string, sizeBytes int64, storagePath string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (run_id, agent_task_id, name, mime_type, size_bytes, storage_path)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, runID, agentTaskID, name, mimeType, sizeBytes, storagePath).Scan(&id)
	return id, err
}

// GetArtifact fetches a single artifact by UUID.
// Returns pgx.ErrNoRows if not found.
func GetArtifact(ctx context.Context, pool *db.Pool, artifactID string) (*Artifact, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, run_id, agent_task_id, name, mime_type, size_bytes, storage_path, created_at
		FROM artifacts
		WHERE id = $1
	`, artifactID)

	var a Artifact
	if err := row.Scan(
		&a.ID, &a.RunID, &a.AgentTaskID, &a.Name, &a.MimeType,
		&a.SizeBytes, &a.StoragePath, &a.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

// ListArtifacts returns all artifacts for a run, ordered by creation time.
func ListArtifacts(ctx context.Context, pool *db.Pool, runID string) ([]Artifact, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, run_id, agent_task_id, name, mime_type, size_bytes, storage_path, created_at
		FROM artifacts
		WHERE run_id = $1
		ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(
			&a.ID, &a.RunID, &a.AgentTaskID, &a.Name, &a.MimeType,
			&a.SizeBytes, &a.StoragePath, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

// ListAllRuns returns up to limit runs across all teams, ordered newest first.
// Intended for admin use only — callers must verify admin access before calling.
func ListAllRuns(ctx context.Context, pool *db.Pool, limit int) ([]TeamRun, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, team_id, initiated_by, objective, status, model,
		       created_at, started_at, completed_at, error
		FROM team_runs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []TeamRun
	for rows.Next() {
		var r TeamRun
		if err := rows.Scan(
			&r.ID, &r.TeamID, &r.InitiatedBy, &r.Objective, &r.Status, &r.Model,
			&r.CreatedAt, &r.StartedAt, &r.CompletedAt, &r.Error,
		); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

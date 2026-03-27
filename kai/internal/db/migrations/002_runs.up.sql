-- 002_runs.up.sql
-- Phase 2 schema: agent runs, per-agent tasks, event stream, and file artifacts.
-- See docs/tdd/kai-data.md §4 for design rationale.
--
-- NOTE: If migrating an environment where 001_initial.up.sql already included
-- these tables (pre-split schema), run the corresponding down migration first or
-- use `migrate force 2` to mark this version applied without executing it.

-- ----------------------------------------------------------------------------
-- team_runs
-- initiated_by records who clicked "Launch" — not a team-ownership FK.
-- status CHECK enforced at DB level; transitions managed in application layer.
-- model column persists which LLM was used for reproducibility.
-- created_at DESC index supports the default "newest first" listing query.
-- ----------------------------------------------------------------------------
CREATE TABLE team_runs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id      UUID        NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    initiated_by TEXT        NOT NULL REFERENCES users(id),
    objective    TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending','running','completed','failed','cancelled')),
    model        TEXT        NOT NULL DEFAULT 'claude-sonnet-4.5',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);
CREATE INDEX idx_team_runs_team_id       ON team_runs(team_id);
CREATE INDEX idx_team_runs_initiated_by  ON team_runs(initiated_by);
CREATE INDEX idx_team_runs_status        ON team_runs(status);
CREATE INDEX idx_team_runs_created_at    ON team_runs(created_at DESC);

-- ----------------------------------------------------------------------------
-- agent_tasks
-- agent_role CHECK constrains to the fixed set of roles in a Kai team run.
-- status CHECK matches the state machine in the operator reconciler.
-- sandbox_name: AgentSandbox CRD name in K8s; NULL until pod is scheduled.
-- ----------------------------------------------------------------------------
CREATE TABLE agent_tasks (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_role   TEXT        NOT NULL
                             CHECK (agent_role IN ('planner','researcher','coder_1','coder_2','reviewer')),
    status       TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending','running','succeeded','failed','timed_out')),
    sandbox_name TEXT,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);
CREATE INDEX idx_agent_tasks_run_id ON agent_tasks(run_id);
CREATE INDEX idx_agent_tasks_status ON agent_tasks(status);

-- ----------------------------------------------------------------------------
-- run_events
-- Persisted for WebSocket history replay: new subscribers replay from this
-- table; the in-process ring buffer is ephemeral.
-- Composite index (run_id, created_at) covers both the filter and the ASC sort
-- for the primary access pattern:
--   WHERE run_id = $1 ORDER BY created_at ASC LIMIT n
-- agent_task_id is SET NULL on task deletion so events survive task removal.
-- ----------------------------------------------------------------------------
CREATE TABLE run_events (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_task_id UUID        REFERENCES agent_tasks(id) ON DELETE SET NULL,
    event_type    TEXT        NOT NULL,
    payload       JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_run_events_run_id_created ON run_events(run_id, created_at);

-- ----------------------------------------------------------------------------
-- artifacts
-- agent_task_id is SET NULL on task deletion so artifacts survive task removal.
-- storage_path: opaque path for the object-store backend.
-- ----------------------------------------------------------------------------
CREATE TABLE artifacts (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_task_id UUID        REFERENCES agent_tasks(id) ON DELETE SET NULL,
    name          TEXT        NOT NULL,
    mime_type     TEXT        NOT NULL DEFAULT 'application/octet-stream',
    size_bytes    BIGINT      NOT NULL DEFAULT 0,
    storage_path  TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_artifacts_run_id ON artifacts(run_id);

-- 001_initial.up.sql
-- Full schema for the Kai platform.
-- See docs/tdd/kai-data.md §3 for authoritative rationale on all design decisions.

-- ----------------------------------------------------------------------------
-- users
-- id is the Authentik sub claim (SHA-256 derived string, NOT a UUID).
-- Casting to uuid::uuid would break if the IdP changes its sub format; TEXT avoids that.
-- Upserted on every login so email, display_name, avatar_url, is_admin stay current.
-- ----------------------------------------------------------------------------
CREATE TABLE users (
    id            TEXT        PRIMARY KEY,
    email         TEXT        NOT NULL DEFAULT '',
    display_name  TEXT        NOT NULL DEFAULT '',
    avatar_url    TEXT        NOT NULL DEFAULT '',
    is_admin      BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ----------------------------------------------------------------------------
-- sessions
-- token_hash: SHA-256(raw_cookie_value), hex-encoded.
-- Raw cookie value is never stored — DB read cannot yield a usable session token.
-- expires_at index supports the periodic cleanup job (DELETE WHERE expires_at < NOW()).
-- ----------------------------------------------------------------------------
CREATE TABLE sessions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_agent  TEXT        NOT NULL DEFAULT '',
    ip_addr     TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX idx_sessions_user_id    ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- ----------------------------------------------------------------------------
-- api_keys
-- key_hash:   SHA-256(raw_key), hex-encoded.
-- key_prefix: first 8 chars of raw key shown in UI (e.g. "kai_ab12…"); never enough
--             to reconstruct the raw key.
-- Raw key format: kai_<base64url(32 random bytes)>
-- SHA-256 is appropriate here (not bcrypt) because the key is 256-bit crypto-random.
-- ----------------------------------------------------------------------------
CREATE TABLE api_keys (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL UNIQUE,
    key_prefix   TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_user_id  ON api_keys (user_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys (key_hash);

-- ----------------------------------------------------------------------------
-- teams
-- created_by references the user who created the team.
-- Authorization for run access checks team_members membership, not created_by.
-- ----------------------------------------------------------------------------
CREATE TABLE teams (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    slug       TEXT        NOT NULL UNIQUE,
    created_by TEXT        NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_teams_slug ON teams (slug);

-- ----------------------------------------------------------------------------
-- team_members
-- role CHECK ensures only valid roles are stored at DB level.
-- user_id index supports "list teams for user" queries.
-- ----------------------------------------------------------------------------
CREATE TABLE team_members (
    team_id   UUID        NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id   TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role      TEXT        NOT NULL CHECK (role IN ('owner', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX idx_team_members_user_id ON team_members (user_id);

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
                             CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    model        TEXT        NOT NULL DEFAULT 'claude-sonnet-4.5',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);

CREATE INDEX idx_team_runs_team_id     ON team_runs (team_id);
CREATE INDEX idx_team_runs_initiated_by ON team_runs (initiated_by);
CREATE INDEX idx_team_runs_status      ON team_runs (status);
CREATE INDEX idx_team_runs_created_at  ON team_runs (created_at DESC);

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
                             CHECK (agent_role IN ('planner', 'researcher', 'coder_1', 'coder_2', 'reviewer')),
    status       TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'timed_out')),
    sandbox_name TEXT,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);

CREATE INDEX idx_agent_tasks_run_id ON agent_tasks (run_id);
CREATE INDEX idx_agent_tasks_status ON agent_tasks (status);

-- ----------------------------------------------------------------------------
-- run_events
-- Persisted for WebSocket history replay (new subscribers replay from this table;
-- the in-process ring buffer is ephemeral).
-- Composite index (run_id, created_at) covers both the filter and sort for
-- the primary access pattern:
--   WHERE run_id = $1 ORDER BY created_at ASC LIMIT 2000
-- agent_task_id is SET NULL on task deletion (events survive task removal).
-- Events older than 30 days can be pruned by a nightly cron.
-- ----------------------------------------------------------------------------
CREATE TABLE run_events (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_task_id UUID        REFERENCES agent_tasks(id) ON DELETE SET NULL,
    event_type    TEXT        NOT NULL,
    payload       JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_run_events_run_id  ON run_events (run_id, created_at);
CREATE INDEX idx_run_events_task_id ON run_events (agent_task_id);

-- ----------------------------------------------------------------------------
-- artifacts
-- storage_path: path within the run workspace PVC (kai-workspace-{run_id}).
-- agent_task_id is SET NULL on task deletion (artifacts survive task removal).
-- ----------------------------------------------------------------------------
CREATE TABLE artifacts (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_task_id UUID        REFERENCES agent_tasks(id) ON DELETE SET NULL,
    name          TEXT        NOT NULL,
    mime_type     TEXT        NOT NULL DEFAULT 'application/octet-stream',
    storage_path  TEXT        NOT NULL,
    size_bytes    BIGINT      NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_artifacts_run_id ON artifacts (run_id);

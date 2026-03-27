---
project: "kai"
maturity: "draft"
last_updated: "2025-07-14"
updated_by: "@data-engineer"
scope: "PostgreSQL schema, migrations, indexes, and data access patterns for the Kai platform."
owner: "@data-engineer"
dependencies:
  - docs/prd/kai.md
  - docs/tdd/kai-backend.md
---

# TDD: Kai Data Layer

## 1. Problem Statement

Kai needs a PostgreSQL database that stores users, sessions, teams, agent runs, task
events, and artifacts. The schema must support the real-time run lifecycle (multiple
agents updating status concurrently), efficient WebSocket history replay (last N events
for a run), and API key auth.

---

## 2. Technology Choices

- **PostgreSQL 16** — dedicated StatefulSet in `kai` namespace (no shared DB found on cluster)
- **golang-migrate** — same tool already operational on this cluster (OpenHands); embedded via `embed.FS`
- **pgx/v5** — native binary protocol driver (no ORM)
- **Advisory lock** on migration startup prevents concurrent migration races during rolling deploys

---

## 3. Schema

### 3.1 `users`

```sql
CREATE TABLE users (
    id           TEXT        PRIMARY KEY,        -- Authentik sub claim (hashed string, NOT UUID)
    email        TEXT        NOT NULL DEFAULT '', -- may be empty until first login with email scope
    display_name TEXT        NOT NULL DEFAULT '',
    avatar_url   TEXT        NOT NULL DEFAULT '',
    is_admin     BOOLEAN     NOT NULL DEFAULT FALSE, -- true if in Authentik group "kai-admins"
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Why `id TEXT` not UUID**: Authentik's `sub` claim is a SHA-256 derived string like
`7f3a9b2e...`. Casting to UUID::uuid at insert time would break if the IdP changes its
sub format. TEXT avoids the fragile cast.

**Upsert on every login** (not insert-or-ignore) so `last_login_at`, `email`, and
`is_admin` stay current:

```sql
INSERT INTO users (id, email, display_name, avatar_url, is_admin, last_login_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (id) DO UPDATE SET
    email         = EXCLUDED.email,
    display_name  = EXCLUDED.display_name,
    avatar_url    = EXCLUDED.avatar_url,
    is_admin      = EXCLUDED.is_admin,
    last_login_at = NOW();
```

### 3.2 `sessions`

```sql
CREATE TABLE sessions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE, -- SHA-256(raw_cookie_value), hex-encoded
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_agent  TEXT        NOT NULL DEFAULT '',
    ip_addr     TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_user_id    ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at); -- for cleanup job
```

**Why `token_hash` not raw token**: The raw cookie value is 32 random bytes (256-bit
entropy). Storing it raw means a DB read gives an attacker a valid session token. SHA-256
of the raw value is stored instead; the DB never holds anything that can be used directly.

**Cleanup**: A periodic DELETE removes expired sessions:
```sql
DELETE FROM sessions WHERE expires_at < NOW();
```

### 3.3 `api_keys`

```sql
CREATE TABLE api_keys (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,               -- user-chosen label e.g. "CI pipeline"
    key_hash     TEXT        NOT NULL UNIQUE,         -- SHA-256(raw_key), hex-encoded
    key_prefix   TEXT        NOT NULL,               -- first 8 chars of raw key for display: "kai_ab12..."
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ                          -- NULL = never expires
);

CREATE INDEX idx_api_keys_user_id  ON api_keys(user_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
```

**Why SHA-256 not bcrypt for API keys**: API keys are 32 bytes from `crypto/rand` (256-bit
entropy). bcrypt is designed for low-entropy passwords; the cost is unnecessary and adds
latency on every authenticated API call. SHA-256 is appropriate when the input is a
cryptographically random high-entropy value.

Raw key format: `kai_<base64url(32 random bytes)>` — prefix makes keys identifiable.

### 3.4 `teams`

```sql
CREATE TABLE teams (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    slug        TEXT        NOT NULL UNIQUE, -- URL-safe identifier
    created_by  TEXT        NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_teams_slug ON teams(slug);
```

### 3.5 `team_members`

```sql
CREATE TABLE team_members (
    team_id   UUID        NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id   TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role      TEXT        NOT NULL CHECK (role IN ('owner', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX idx_team_members_user_id ON team_members(user_id);
```

### 3.6 `team_runs`

```sql
CREATE TABLE team_runs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id      UUID        NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    initiated_by TEXT        NOT NULL REFERENCES users(id), -- who clicked "Launch"
    objective    TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending','running','completed','failed','cancelled')),
    model        TEXT        NOT NULL DEFAULT 'claude-sonnet-4.5',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);

CREATE INDEX idx_team_runs_team_id    ON team_runs(team_id);
CREATE INDEX idx_team_runs_initiated_by ON team_runs(initiated_by);
CREATE INDEX idx_team_runs_status     ON team_runs(status);
CREATE INDEX idx_team_runs_created_at ON team_runs(created_at DESC);
```

**Column name `initiated_by` not `user_id`**: Makes the intent clear — this is not
a "belongs to user" relationship, it's "who started it". Teams own runs; a user triggers one.

### 3.7 `agent_tasks`

```sql
CREATE TABLE agent_tasks (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_role   TEXT        NOT NULL
                             CHECK (agent_role IN ('planner','researcher','coder_1','coder_2','reviewer')),
    status       TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending','running','succeeded','failed','timed_out')),
    sandbox_name TEXT,                               -- AgentSandbox CRD name in K8s
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);

CREATE INDEX idx_agent_tasks_run_id ON agent_tasks(run_id);
CREATE INDEX idx_agent_tasks_status ON agent_tasks(status);
```

### 3.8 `run_events`

```sql
CREATE TABLE run_events (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_task_id UUID        REFERENCES agent_tasks(id) ON DELETE SET NULL,
    event_type    TEXT        NOT NULL, -- matches RunEvent.type enum
    payload       JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_run_events_run_id     ON run_events(run_id, created_at);
CREATE INDEX idx_run_events_task_id    ON run_events(agent_task_id);
```

**Why persist events**: New WebSocket subscribers get history replayed from this table
(the in-process ring buffer is ephemeral). Events older than 30 days can be pruned.

**Composite index `(run_id, created_at)`**: The primary access pattern is
`WHERE run_id = $1 ORDER BY created_at ASC LIMIT 2000` — the composite index
covers both filter and sort without a filesort.

### 3.9 `artifacts`

```sql
CREATE TABLE artifacts (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID        NOT NULL REFERENCES team_runs(id) ON DELETE CASCADE,
    agent_task_id UUID        REFERENCES agent_tasks(id) ON DELETE SET NULL,
    name          TEXT        NOT NULL,              -- original filename
    mime_type     TEXT        NOT NULL DEFAULT 'application/octet-stream',
    storage_path  TEXT        NOT NULL,              -- path within run workspace PVC
    size_bytes    BIGINT      NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_artifacts_run_id ON artifacts(run_id);
```

---

## 4. Migration Structure

```
internal/db/migrations/
├── 000001_init.up.sql
├── 000001_init.down.sql
├── 000002_add_api_keys.up.sql   (if split from init)
└── ...
```

Embedded in binary:

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS
```

Migration runner with advisory lock:

```go
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
    conn, err := pool.Acquire(ctx)
    defer conn.Release()

    // Advisory lock: key = hash("kai-migrations")
    _, err = conn.Exec(ctx, "SELECT pg_advisory_lock(7472836172)")
    defer conn.Exec(ctx, "SELECT pg_advisory_unlock(7472836172)")

    src, _ := iofs.New(migrationsFS, "migrations")
    m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
    return m.Up()
}
```

---

## 5. Open Questions

### OQ-1: Run ownership for collaboration

`team_runs.initiated_by` records who started the run. If multiple team members should
be able to view/cancel runs they didn't start, authorization should check `team_members`
membership, not `initiated_by`. This is the current design — confirm it's correct.

### OQ-2: Event retention

How long should `run_events` rows be kept? Suggested: 30 days, pruned by a nightly
cron job. Very long runs with many events could bloat the table.

### OQ-3: Workspace PVC reference

Should `team_runs` have a `workspace_pvc_name TEXT` column to track the shared PVC,
or is it derived from `run_id` by convention (`kai-workspace-{run_id}`)?
Convention is simpler but a column makes it queryable.

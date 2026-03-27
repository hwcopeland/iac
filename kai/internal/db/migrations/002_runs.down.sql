-- 002_runs.down.sql
-- Reverses 002_runs.up.sql.
-- Drop in reverse dependency order so foreign-key constraints are satisfied.

DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS run_events;
DROP TABLE IF EXISTS agent_tasks;
DROP TABLE IF EXISTS team_runs;

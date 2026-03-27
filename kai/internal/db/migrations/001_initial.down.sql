-- 001_initial.down.sql
-- Drop all tables created by 001_initial.up.sql in reverse dependency order.
-- Indexes are dropped automatically when their parent table is dropped.

DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS run_events;
DROP TABLE IF EXISTS agent_tasks;
DROP TABLE IF EXISTS team_runs;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

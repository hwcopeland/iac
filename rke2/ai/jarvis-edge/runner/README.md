# JARVIS cluster-side agent runner

Charter roadmap #2 (agent control-plane). Turns a voice-initiated long-running
task into a tracked, independent k8s `Job` that survives the laptop closing.
This directory holds the **runner** half (the Job that runs one task and exits);
the launch/track/announce half (`jarvis_runner_mcp.py`, the edge announce
thread, `gate_and_respond` wiring) ships separately.

## Files

| File | Purpose |
|------|---------|
| `jarvis_runner_entrypoint.py` | Job process: runs one `claude -p` task (subscription mode), writes start/result facts to mem0 `runs` scope, exits 0/non-0 so the Job `.status` is truthful. |
| `runner_mcp_config.json` | MCP surface for the run's claude — read-only kube/overview/mem0/personal. **Excludes** `jarvis_runner` (no recursion), `jarvis_sonos`, `jarvis_persona`, `jarvis_delegate`. |
| `job-template.yaml` | Parametrized `Job` (`__PLACEHOLDER__` tokens substituted by `launch_run`). Canonical source + the thing you `kubectl apply --dry-run` to validate. |
| `rbac-runner.yaml` | `jarvis-runner` SA (read-only, like the edge) + a dedicated `jarvis-run-launcher` SA/Role with the narrow create/get/list/watch-Jobs capability. |

## Source-of-truth split

- **k8s Job `.status`** is the authoritative run registry (Active/Succeeded/
  Failed) — exact, free, RBAC-gated, reaped by `ttlSecondsAfterFinished`.
- **mem0 `user_id="runs"`** is the secondary store: the human-readable result
  blurb the Sonos announce loop narrates and `run_status` surfaces.

## Required Dockerfile change (image rebuild)

The jarvis-edge `Dockerfile` must COPY the runner dir into the image so the
Job entrypoint + MCP config exist at `/app/runner/`. Add to the COPY block:

```dockerfile
COPY runner/jarvis_runner_entrypoint.py runner/runner_mcp_config.json \
     /app/runner/
```

`runner_mcp_config.json` references `/app/jarvis_mem0_mcp.py` and
`/app/jarvis_overview_mcp.py` — those land in `/app/` via the mem0 wiring
(see memory `project_mem0-unified-memory`); a run with a missing MCP server
simply won't expose those tools (claude tolerates it) but `jarvis_kube` +
`jarvis_personal` are already in the image today.

Rebuild via in-cluster kaniko on `feat/jarvis-identity-gate` (never on the
Mac), then `kubectl rollout restart deploy/jarvis-stack -n ai`. Apply the
RBAC once: `kubectl apply -f runner/rbac-runner.yaml` (owner-confirmed).

## Safety (charter principle 3)

- A run's `jarvis-runner` SA is **read-only** — apply-mode runs mutate the
  repo via `git push` to the `jarvis` branch (delegate's model), never via the
  k8s API. A compromised run cannot apply/scale/delete cluster state.
- Job-create power lives ONLY in `jarvis-run-launcher`, never on the edge's
  default `jarvis-readonly` SA.
- `backoffLimit: 0` (no silent retries of a mutating task),
  `activeDeadlineSeconds: 3600`, `--max-budget-usd` cap spend/runaway.
- Fresh per-run `emptyDir` for `.claude` creds — never the edge's live PVC
  subPath — so a run can't race the edge's OAuth-refresh writes.
- The Verify phase must use a **safe read-only** task (`mode: read`); do NOT
  auto-run destructive verification.

## Open decision for Hampton

`rbac-runner.yaml` defaults to a **dedicated** `jarvis-run-launcher` SA for
the Job-create capability (keeps the voice loop's SA read-only). The
alternative is an additive minimal Role on `jarvis-readonly`. The dedicated
SA is the safer default; confirm before apply.

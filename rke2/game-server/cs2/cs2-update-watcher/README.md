# CS2 Update Watcher

Automatically restarts the live CS2 game server when Valve ships a new build, so
players don't hit **"server is running an older version of the game"** and get
refused at connect time.

## Why this exists

When Valve updates CS2, connecting clients update first, then refuse to join any
server still on the old build. Both server images in this repo run
`steamcmd +app_update 730` on container boot (see
[`../cs2-surf/entrypoint.sh`](../cs2-surf/entrypoint.sh) step 1, and the kus
image does the same), so **a pod restart is how the update gets pulled.** This
watcher detects staleness and triggers that restart — and keeps nudging until
the server is genuinely current, because a single steamcmd run can race the
Steam CDN and a reboot can still come up behind.

## Design decision: standalone controller, not a sidecar

A sidecar in the server pod can't cleanly restart its own pod (it would have to
delete the pod it lives in, racing its own termination). A standalone CronJob
sidesteps that ordering problem entirely, can target **whichever** deployment is
active, and survives the very restarts it triggers. That's the shape here.

## Detection signal

**Primary:** Steam Web API `ISteamApps/UpToDateCheck`:

```
https://api.steampowered.com/ISteamApps/UpToDateCheck/v1/?appid=730&version=<installed_build>
→ {response:{success, up_to_date, required_version, ...}}
```

The installed build is the `PatchVersion=` field read from `steam.inf` on the
shared PVC (`/home/steam/cs2/game/csgo/steam.inf`, mounted read-only at
`/cs2/...` in the watcher), **with the dots stripped**: `PatchVersion=1.41.7.2`
becomes `14172`, which is exactly the space `required_version` is expressed in
(verified against the live API 2026-07-23 — `version=100` returns
`required_version: 14172, message: "Server version required: 1.41.7.2"`).
`ClientVersion` must NOT be used: it is always numerically far above
`required_version`, so a stale server would read as up-to-date. This answers
the exact question we care about — *is MY installed build current?* — rather
than just "did something change."

RSS feeds (SteamDB / the CS blog) are a viable *fallback* signal but are noisier
and don't know your installed version, so they're not used here. UpToDateCheck
is authoritative for "am I behind," needs no API key, and any failure/unknown
response is treated as "do nothing" rather than risking a flap.

## How it picks the active deployment

Two server Deployments share the PVC `cs2-modded-claim`, and only one is scaled
to 1 at a time (`cs2-surf-server` is currently active; `cs2-modded-server` is the
older kus image). The watcher does **not** hardcode either:

1. Reads `service/cs2-modded-service`'s `.spec.selector.app` → the active family.
2. Finds the Deployment with that `app` label.
3. Confirms it has `replicas >= 1`; if the active server is scaled down, it does
   nothing.

Swap the Service selector (`../cs2-surf/k8s/service-patch.yaml`) and the watcher
follows automatically.

## The restart-until-updated loop

The CronJob runs every 10 minutes. **Each run is one decision, not a loop** — so
the worst a single run can do is issue one `kubectl rollout restart`. The
"keep going until current" behaviour spans runs, with cross-run state held in the
`cs2-update-watcher-state` ConfigMap (`attempts`, `target_version`,
`last_restart_ts`).

Per pass:

1. Discover active deployment (above). Skip if down.
2. Read installed `PatchVersion` from `steam.inf`. Skip if missing/unparseable.
3. Query UpToDateCheck. On failure/unknown → **skip** (never restart on doubt).
4. **Up to date** → reset state, exit. Fully idempotent: it will not restart a
   current server.
5. **Behind** → restart, subject to the safety limits below, then record the
   attempt + timestamp. The next pass re-checks and stops once current.

## Safety limits

| Guard | Default | Purpose |
|---|---|---|
| `COOLDOWN_SECONDS` | `900` (15m) | Min gap between restarts, so steamcmd + reboot + CDN can settle before the next nudge. Prevents killing the pod mid-download. |
| `MAX_ATTEMPTS` | `5` | Stop nudging if still behind after N tries for the same target build — a genuinely-unavailable update can't restart the server forever. Logs loudly for alerting. |
| New-build reset | — | When `required_version` changes, the attempt counter resets, so a build that exhausted attempts last week doesn't block this week's update. |
| API failure = no-op | — | Any UpToDateCheck error/unknown leaves the server alone. |
| `EMPTY_ONLY` | `false` | Optional: only restart when 0 human players (A2S_INFO query, same as the surf-web api). Off by default because a stale server is unjoinable anyway — waiting just prolongs the outage. Flip to `true` for polite restarts on optional updates. |

All limits are env vars on the CronJob — tune them in `controller.yaml`, no image
rebuild needed.

## RBAC (least privilege, `game-server` only)

Dedicated ServiceAccount `cs2-update-watcher` with a namespaced Role:

- `deployments`: `get`, `list`, `patch` (patch = how `rollout restart` works)
- `services`: `get`, `list` (read active selector)
- `pods`: `get`, `list`
- `configmaps`: `get`, `update` (cross-run state)

No `create`/`delete`, no `exec`, no secrets, no cluster-wide scope.

## Alerting — how a human hears about it (`alerts.yaml`)

Restarting only fixes *available* updates. The dangerous case is the
**ModSharp coupling**: CS2 updates on the PVC at every boot, ModSharp gamedata
is pinned in the image (`../cs2-surf/Dockerfile`), and a signature-breaking
Valve update SIGSEGVs the server (`Address resolve failed`) until a human bumps
the pin. Upstream gamedata fixes lag Valve, so the automation is strictly
**detect + notify — never auto-bump**. Two `severity: critical` PrometheusRules
(critical is the only severity Alertmanager emails) ride on kube-state-metrics:

- `CS2ServerCrashLooping` — ≥3 container restarts in 30m on the server pod.
  The email carries the full runbook ("check Kxnrl/modsharp-public releases,
  bump `MODSHARP_VERSION` in the Dockerfile, push to main").
- `CS2UpdateWatcherFailing` — ≥2 failed watcher Jobs. `watch.sh` exits `2` when
  it gives up at `MAX_ATTEMPTS` (and on every later pass while still behind),
  so a stuck-behind-build server keeps failed Jobs — and this alert — lit.

## Files

- `rbac.yaml` — ServiceAccount + Role + RoleBinding
- `controller.yaml` — state ConfigMap, script ConfigMap (`watch.sh` inline), CronJob
- `alerts.yaml` — PrometheusRule (crashloop + watcher-gave-up, both critical/email)

No custom image: uses stock `alpine/k8s:1.30.4` (bash + kubectl + python3 + curl),
the same image the ads CronJob uses.

## Deploy

These manifests are **hand-applied** (the `game-server` namespace is not
Flux-managed). From the repo root:

```bash
kubectl apply -f game-server/cs2/cs2-update-watcher/rbac.yaml
kubectl apply -f game-server/cs2/cs2-update-watcher/controller.yaml
kubectl apply -f game-server/cs2/cs2-update-watcher/alerts.yaml
```

### Verify / operate

```bash
# Trigger a one-off check immediately instead of waiting for the schedule:
kubectl create job -n game-server --from=cronjob/cs2-update-watcher watcher-manual

# Watch a run:
kubectl logs -n game-server -l app.kubernetes.io/name=cs2-update-watcher --tail=100 -f

# Inspect cross-run state:
kubectl get cm cs2-update-watcher-state -n game-server -o jsonpath='{.data}'; echo

# Pause the watcher (e.g. during maintenance):
kubectl patch cronjob cs2-update-watcher -n game-server -p '{"spec":{"suspend":true}}'
# resume with "suspend":false
```

If `MAX_ATTEMPTS` is hit, the logs say so explicitly — check steamcmd output on
the server pod; the update may be genuinely unavailable or the PVC may be wedged.

---
project: "iac"
maturity: "ready-for-tdd"
last_updated: "2026-05-23"
updated_by: "@product-owner"
scope: "Audit, repair, and reconcile the home-cluster observability stack (Grafana + kube-prometheus-stack Prometheus + Loki + Alloy + Tempo) so every dashboard, alert, and scrape target reflects truthful cluster state, and so git matches what is running."
owner: "@product-owner"
dependencies: []
---

## REVISED 2026-05-23: Loki object-store target is **Garage**, not MinIO

The original PRD/TDD assumed MinIO at `10.41.0.200:9000` on the Synology was
the intended Loki S3 backend. That was wrong — it was a stalled experiment
from an earlier session and MinIO was never actually deployed. The cluster's
real object store is **Garage** (Rust-based S3-compatible, in-cluster) running
at `garage.garage-system.svc.cluster.local:3900`, deployed via
`rke2/garage-system/`. Credentials follow the same plain-Secret-in-namespace
pattern khemeia uses (`rke2/chem/khemeia/deploy/garage-secret.yaml`), not
ExternalSecret-from-Bitwarden.

Wherever this doc says "MinIO", read "Garage". Mechanism deltas:
- No ExternalSecret needed — copy `garage-secret` Secret into the `monitor`
  namespace following khemeia's pattern.
- Endpoint is `http://garage.garage-system.svc.cluster.local:3900` (in-cluster,
  not the Synology IP).
- Bucket `loki-chunks` must be created via `garage bucket create` on the
  garage-0 pod, with `garage bucket allow` for the access key.

## Problem Statement

The observability stack in `rke2/monitor/` is the operator's primary signal for whether
anything in the home cluster is healthy. Right now that signal is unreliable in ways that
have already cost a debugging session and would cost more in a real incident:

- **Dashboards lie.** On 2026-05-23 the "Grafana Self Performance" dashboard showed Loki at
  ~100% error rate. The cluster was not actually broken — Prometheus simply had no
  `loki_*` request metrics because no ServiceMonitor existed for the `loki` service. The
  panel computed `errors / max(total, 1)` against an empty `total` and rendered red. A
  monitoring gap presented as a monitoring outage.
- **Git does not match the cluster.** `rke2/monitor/loki/loki-values.yaml` contains a
  half-done filesystem→MinIO S3 migration (adds `bucketnames`, `s3forcepathstyle`,
  `extraEnvFrom: loki-minio-secret`) that the installed Loki chart 6.51.0 rejects on schema
  validation. `helm upgrade` against this file would fail. Multiple monitor-adjacent
  manifests are untracked: `grafana-self-perf-dashboard.yaml`, `kubetest-power-dashboard.yaml`,
  `loki/external-secret-minio.yaml`, `loki/minio-compose.yaml`, `loki/servicemonitor.yaml`,
  `node-exporter-values.yaml`, `synology-exporter/`, `mikrotik-exporter/mktxp.conf`.
- **Tactical fixes are accruing.** `rke2/monitor/loki/servicemonitor.yaml` was just shipped
  as a band-aid for the Loki scrape gap and is explicitly marked "delete once chart
  upgrade lands." It is a TODO in YAML form. This PRD supersedes that band-aid: the
  cleanup must finish the S3 migration, enable the chart's native ServiceMonitor, and
  delete the hand-rolled one.
- **Tempo is deployed but receives no traces.** `rke2/monitor/tempo/` runs a single-binary
  Tempo with OTLP listeners on 4317/4318 and a Grafana datasource pointed at it, but no
  workload in the repo sets `OTEL_EXPORTER_*` or otherwise emits to `tempo.monitor:3200`.
  It is consuming a 20Gi Longhorn PVC and a pod slot for no benefit.
- **Dead components are suspected.** `alloy-values.yaml` writes to
  `loki-gateway.monitor.svc.cluster.local`, which is the nginx Deployment shipped by the
  Loki chart, but Grafana's datasource also points at `loki-gateway` while external
  exposure is via Cilium Gateway API. The role of `loki-gateway` in a single-binary
  deployment needs to be confirmed or removed. Loki canary pods (if present in cluster)
  are showing 502s in gateway logs with no investigation; their value is unclear.
- **Alert tree is untuned.** The hand-curated `grafana-alert-rules-platform.yaml` defines
  5 platform alerts, but kube-prometheus-stack also ships hundreds of default Prometheus
  rules whose firing/silence state is unknown to the operator. There is no documented
  silence list and no on-call runbook for what to do when each alert fires.

The cumulative effect: the operator either ignores Grafana (because false reds are
common) or investigates ghosts (like today's "Loki outage"). Both outcomes mean a real
incident may be missed or delayed.

## Goals & Non-Goals

**Goals**
- Every panel on every dashboard in the `monitor` namespace either reflects truthful,
  populated data or is removed.
- Every active alert is actionable: the operator knows what to do when it fires, or it is
  silenced with a documented reason.
- `git diff rke2/monitor/` against `helm get values -n monitor <release>` (and equivalent
  for raw manifests) is empty after the cleanup. Future `helm upgrade` is safe.
- The Loki filesystem→S3 migration is either completed end-to-end or reverted; no
  half-state is left in git.
- A short observability README in `rke2/monitor/README.md` documents what the stack
  scrapes, where data lives, what each chart provides, and how the operator extends it.

**Non-Goals**
- **Replacing the stack.** kube-prometheus-stack, Loki, Alloy, and (probably) Tempo stay.
  This is reconciliation, not re-platforming. Migrations to Mimir, Victoria Metrics,
  OpenTelemetry Collector, etc. are out of scope.
- **Log retention / cost tuning.** Loki ingestion volume, retention windows, and MinIO
  bucket lifecycle are a separate concern with separate trade-offs; track in a follow-up
  PRD if needed.
- **Adding new dashboards for chem/khemeia or game-server projects.** Those projects own
  their own dashboards (`grafana-dashboard-khemeia.yaml`, `cs2-surf/k8s/grafana-dashboard.yaml`).
  This cleanup verifies they still work but does not extend them.
- **New observability features.** No new exporters, no eBPF profiling, no synthetic
  monitoring. The bar is "what we have, working correctly."

## User Stories

The user is a single solo operator (hwcopeland) running this cluster for personal and lab
workloads. They are highly technical but time-constrained and context-switch heavily
between this cluster, JARVIS, khemeia, and CS2 work. Their interaction pattern is:
glance at Grafana when something feels off, expect to either see a clear problem or
nothing notable.

**Must-have**
1. *As the operator, when a panel goes red I trust it.* When the Grafana Self Performance
   "Loki Error Rate" panel reads 0% I know Loki is healthy; when it reads 50% I know
   Loki is failing — not that a metric is missing.
   - Acceptance: Every panel on every dashboard in `rke2/monitor/` either renders a
     populated series under steady-state cluster conditions, or shows an explicit
     "No data" state with a panel description that tells the operator what would make
     data appear.
2. *As the operator, when an alert fires I know what to do.* Each enabled alert routes to
   me with enough context that I can act without re-reading the rule.
   - Acceptance: Every alert in `grafana-alert-rules-platform.yaml` and every enabled
     default kube-prometheus-stack rule has either a runbook URL annotation, a one-line
     "what to do" annotation, or is explicitly silenced with reason in the alertmanager
     config.
3. *As the operator, I can `helm upgrade` any chart in `rke2/monitor/` without surprise.*
   Git is the source of truth; the cluster matches; upgrades are mechanical.
   - Acceptance: For each helm release in the `monitor` namespace, `git diff` between
     the in-repo values file and `helm get values <release> -n monitor` is empty (or
     differences are documented and intentional).
4. *As the operator, I can find what each component does in 30 seconds.* A new project I
   want to add metrics for is straightforward to wire up.
   - Acceptance: `rke2/monitor/README.md` exists and documents: what each chart/manifest
     is for, what Alloy scrapes, where logs/metrics/traces are stored, how to add a new
     ServiceMonitor, how to add a new dashboard via ConfigMap label, and how alerts
     route.

**Should-have**
5. *As the operator, observability storage costs and footprint are within my Longhorn
   budget.* I do not get paged for `kubelet_volume_stats` against my own monitoring PVCs.
   - Acceptance: Loki, Prometheus, and Tempo PVC sizes are documented and either fit
     comfortably under their current provisioned size or are resized to fit usage.
6. *As the operator, dashboards I do not use are not on screen.* Stale dashboards from
   experiments or kube-prometheus-stack defaults that don't apply to a 4-node
   single-tenant cluster should be hidden or deleted.
   - Acceptance: Each dashboard surfaced in Grafana is either tagged for the operator's
     active workstreams (platform, surf, khemeia, JARVIS, power) or removed.

**Could-have**
7. *As the operator, I can drill from a log line to a trace and back.* If Tempo stays,
   it works end-to-end for at least one service.
   - Acceptance: At least one workload (TBD which) emits OTLP to Tempo and the
     trace-to-logs / logs-to-traces link in Grafana is verified working — or Tempo is
     removed.

## Scope

**In scope**
- Inventory and audit of all dashboards, alert rules, recording rules, ServiceMonitors,
  PodMonitors, exporters, and helm releases in or adjacent to the `monitor` namespace.
- Completing or reverting the Loki S3 migration.
- Removing the hand-rolled `loki/servicemonitor.yaml` band-aid after the chart-native
  ServiceMonitor is enabled.
- Decision on Tempo (keep + wire up one producer, or remove).
- Decision on `loki-gateway` and Loki canary (keep with documented purpose, or remove).
- Documenting silences and runbook annotations for the alert tree.
- Adding `rke2/monitor/README.md`.
- Committing all currently-untracked monitor-related YAML, or deleting it.
- Bringing `helm get values` and git into agreement for `kube-prometheus-stack`, `loki`,
  and `alloy` releases.

**Out of scope** (explicit non-goals from above plus operationally separate concerns)
- New observability backends or major version migrations of any component.
- Log retention windows, MinIO bucket lifecycle, ingestion-rate cost tuning.
- New dashboards or alerts for application workloads (chem, JARVIS, surf — those projects
  own their own observability).
- Multi-cluster, multi-tenant, or RBAC-in-Grafana work.
- Synthetic monitoring (blackbox-exporter, uptime checks).

**Deferred to follow-up**
- Recording rules for high-cardinality queries (defer until a slow panel is identified).
- LokiRequestErrors / LokiRequestPanic alert imports from the chart's bundled rules (open
  question 3 below).
- Promtail → Alloy migration cleanup (Alloy is already the only agent; verify no Promtail
  remnants).

## User Journey

**Before (current state)**
1. Operator opens https://grafana.hwcopeland.net.
2. Operator lands on a dashboard with mixed signals: some panels populated, some red
   with NaN, some empty, some referencing services that no longer exist.
3. Operator sees a red panel ("Loki at 100% error rate"). Cannot tell if it is real.
4. Operator opens a terminal, kubectl-greps Loki pod logs, sees nothing unusual,
   discovers Loki is actually healthy.
5. Operator wastes 30+ minutes investigating a monitoring gap. Trust in dashboards drops.
6. Operator may or may not act on real signal later because of accumulated false-positive
   fatigue.

**After (proposed state)**
1. Operator opens https://grafana.hwcopeland.net.
2. Operator lands on a curated dashboard list scoped to active workstreams. Every panel
   reflects real data or shows an honest "No data — explanation here" state.
3. When a panel goes red, the panel's description and the linked alert annotation
   explain what the condition means and what to check first.
4. If an alert fires, the operator receives it with a runbook URL or a one-line action.
5. Operator extends the stack (new exporter, new dashboard) by following the README and
   committing a single ConfigMap with the standard `grafana_dashboard: "1"` label —
   no chart drift, no untracked YAML.

**Edge / error branches**
- *Scrape target disappears:* dashboards show "No data" not red. ServiceMonitor health
  is itself surfaced on a "Monitoring Health" dashboard so gaps are visible.
- *MinIO unavailable:* Loki write-path failure is alertable; previous-day logs still
  queryable from the chunks already in S3 (read-path tolerates write-path outage).
- *Operator deletes a dashboard ConfigMap by accident:* git revert restores it; the
  sidecar reloads it within the next sync interval.

## Requirements

**Functional**
- F1. All ServiceMonitor/PodMonitor selectors match real pod labels and produce non-empty
  `up{}` metrics in Prometheus.
- F2. Every dashboard ConfigMap labeled `grafana_dashboard: "1"` either renders all
  panels with populated data OR has been audited and intentionally retained with
  documented "no data" panels.
- F3. Loki ingestion path (Alloy → Loki write endpoint → MinIO/filesystem) is end-to-end
  working and verifiable via a synthetic log line query.
- F4. Loki query path (Grafana datasource → Loki → backing store) is end-to-end working,
  evidenced by a known-existing log query returning results.
- F5. Helm releases (`kube-prometheus-stack`, `loki`, `alloy`, plus any other in
  `monitor`) have an in-repo values file that, when applied, produces zero `helm diff`
  against the live release.
- F6. Tempo is either receiving OTLP from at least one in-repo producer with traces
  visible in Grafana, OR removed (manifests deleted, PVC released, datasource removed).
- F7. `loki-gateway` Deployment and Loki canary StatefulSet (if present) are either
  documented as load-bearing in the README or removed.
- F8. The hand-rolled `rke2/monitor/loki/servicemonitor.yaml` is deleted, with its
  responsibility taken over by `monitoring.serviceMonitor.enabled: true` in the Loki
  chart values.

**Non-functional**
- NF1. The cleanup must not lose log data already stored. Existing Loki chunks in
  the current backing store must remain queryable (or be migrated to the new backing
  store before old storage is reclaimed).
- NF2. The cleanup must not break the homepage's `siteMonitor: https://grafana.hwcopeland.net`
  health link or any in-repo dashboard ConfigMap consumed by other projects (khemeia,
  cs2-surf).
- NF3. Alert routing (alertmanager → wherever the operator receives notifications)
  remains functional throughout; if changes are made, the operator must verify at least
  one test-fire reaches them.
- NF4. SSO (Authentik OIDC → Grafana) remains functional; no changes to the OIDC client
  config are required.

**Constraints**
- C1. Single operator, single cluster (4-node RKE2 + Cilium Gateway API). No HA
  requirements; single-replica Loki / Prometheus / Tempo are acceptable.
- C2. Storage is Longhorn-backed for stateful components plus a local MinIO instance at
  `10.41.0.200:9000` for object storage.
- C3. All changes are reviewed by the operator before apply; no automated rollout.
- C4. No new secrets stores; continue with ExternalSecret + existing backend.

## Risks & Assumptions

**Risks**
- R1. *Data loss during Loki S3 migration.* If the migration is botched, the operator
  loses queryable history. Mitigation: backup chunk dir before cutover; verify schema
  config across the boundary date; staff-engineer's TDD must cover this.
- R2. *Removing a "dead" component that turns out to be load-bearing.* `loki-gateway` may
  be the only ingest endpoint Alloy can reach in the current network policy; removing it
  could silently break ingestion. Mitigation: validate one component at a time with a
  test log line round-trip.
- R3. *Alert silences hide real signal.* Bulk-silencing default kube-prometheus-stack
  alerts to reduce noise may also silence a real alert. Mitigation: silence on a
  per-rule basis with a comment, not by disabling the whole rule group.
- R4. *Cleanup itself becomes a new monitoring outage.* `helm upgrade` to reconcile drift
  could fail on a value chart 6.51.0 rejects. Mitigation: dry-run / `helm diff` before
  every apply.

**Assumptions**
- A1. The operator wants to keep all four pillars (metrics, logs, traces, alerts) in
  some form; the question is "fix" not "remove." (See open question 1 for Tempo.)
- A2. MinIO at `10.41.0.200:9000` is the intended long-term object store for Loki, not a
  staging step toward something else.
- A3. The operator does not need multi-tenancy or auth on Loki; `auth_enabled: false` is
  intentional.
- A4. The kube-prometheus-stack chart and Loki chart are the right delivery mechanism;
  switching to operator-managed (Prometheus Operator CR + Loki Operator) is out of scope.

## Dependencies & Stakeholders

**Dependencies**
- MinIO availability at `10.41.0.200:9000` (separate infra concern).
- Longhorn storage class healthy.
- ExternalSecrets operator running (sources `loki-minio-secret`, `grafana-oidc-secret`).
- Authentik OIDC provider for Grafana SSO.

**Stakeholders**
- Operator (hwcopeland) — sole consumer of the stack; sole decision-maker on trade-offs.
- @staff-engineer — produces the TDD covering the S3 migration plan, dashboard/alert
  triage method, and component keep/kill matrix.
- @project-manager — decomposes the approved PRD + TDD into Docket issues.
- @devops-engineer — likely the implementer for helm reconciliation, ServiceMonitor wiring,
  and runbook capture.

## Prioritization Rationale

This is P1 (do next) because:
- Cost of inaction is recurring: every false-positive dashboard wastes operator time and
  erodes trust in the very tool the operator relies on during real incidents.
- The Loki migration sits half-done in git; the longer it sits, the harder it is to
  remember the original intent. Today is the cheapest day to finish it.
- The work has a natural and bounded scope (inventory + reconcile + document); it is
  not the start of an open-ended platform project.
- It blocks confidence in any future observability work (e.g. khemeia v0.4.0 needs
  trustworthy Grafana for the pipeline workbench).

It is deprioritized below: active P0 security work, active khemeia v0.4.0 work, and
JARVIS work-in-progress. It is prioritized above: new monitor features, recording rule
optimization, cost tuning.

## Open Questions

1. **Tempo: keep or remove?** Tempo is deployed with a 20Gi PVC but no workload is
   emitting OTLP. Two paths: (a) wire OTLP from at least one in-repo workload (khemeia
   API or surf API are natural candidates) and keep it; (b) delete the StatefulSet, PVC,
   config, datasource and reclaim resources. Needs operator decision before TDD.
2. **Loki canary: kill it?** The chart's `lokiCanary.enabled` ships a synthetic
   write/read canary. Today its tail queries appear as 502s in gateway logs. Is it
   providing value (early-warning on Loki health) that the Grafana Self Performance
   dashboard does not already cover, or is it noise to delete?
3. **Loki chart-bundled alerts: enable or not?** `loki.monitoring.rules.enabled: true`
   imports `LokiRequestErrors`, `LokiRequestPanic`, `LokiRequestLatency` recording rules
   and alerts. These would have caught today's incident (or at least its real version
   the next time it happens). Default? Or curated subset?
4. **`loki-gateway`: kept or removed?** Single-binary Loki does not require the nginx
   gateway. Alloy and Grafana both currently point at it. Is the gateway providing
   value (auth, rate-limit, path-rewriting) or is it a leftover from a microservices
   topology that was never adopted?
5. **Alert notification destination.** Where do alerts route today (operator email,
   ntfy, Discord, none)? Needs to be inventoried so silences and routing rules can be
   tuned, and so a "test fire" can be verified post-cleanup.
6. **Default kube-prometheus-stack rules.** Wholesale `defaultRules.create: false` and
   curate from scratch, or `defaultRules.create: true` with explicit per-rule silences?
   This is a maintenance-burden trade-off.

## Operator Decisions (2026-05-23)

The six open questions above were answered by the operator. The TDD should treat
these as committed direction, not suggestions to re-litigate.

1. **Tempo: KEEP and wire up.** Instrument both khemeia API and surf-web API to
   emit OTLP. Additionally, **discover and list every other in-cluster API that
   could benefit from tracing** as a sub-task — candidates include (verify
   against running workloads): authentik, longhorn proxy, plex-system ARR stack,
   chem compute-infrastructure result-writer, any FastAPI/Flask services. TDD
   should pick a minimal first instrumentation (one service) to prove the
   pipeline end-to-end before fanning out.
2. **Loki canary: KEEP, fix it up.** There is an existing pipeline tying the
   canary to Mikrotik metrics ingestion (mktxp exporter at
   `rke2/monitor/mikrotik-exporter/`). The manifests have since been
   committed and sanitized in `c49409b2` (see "Pending Operator Actions"
   below for the rotation work that gates re-enablement). TDD should:
   - fix whatever's wrong with the canary itself (the 502s on tail queries
     traced to gateway DNS — may resolve when loki-gateway is removed, see #4),
   - verify mktxp scrape end-to-end once the password rotation lands.
3. **Loki chart-bundled alerts: ENABLE FULL SET.**
   `monitoring.rules.enabled: true`, all five rules
   (`LokiRequestErrors`, `LokiRequestPanics`, `LokiRequestLatency`,
   `LokiTooManyCompactorsRunning`, `LokiCanaryLatency`). Tune thresholds only
   if a specific alert proves noisy in operation.
4. **loki-gateway: REMOVE.** Disable via chart values, repoint Alloy
   (`alloy-values.yaml`) and the Grafana Loki datasource at `loki:3100`
   directly. TDD must include the repoint steps and confirm no other consumer
   (canary? external scraper? curl-based runbooks?) depends on the gateway
   before deletion.
5. **Alert destination: EMAIL (interim).** Route Alertmanager to email
   (`hampton888@gmail.com`) as the only receiver for now. **Explicit follow-up
   noted by operator:** when a Mac joins the cluster, switch to push
   notifications via that host. TDD should configure the email receiver
   cleanly (SMTP creds via ExternalSecret, not inline) so swapping receivers
   later is a one-line change.
6. **Default kube-prometheus-stack rules: KEEP ON, CURATE SILENCES.**
   `defaultRules.create: true`. TDD should produce an inventory of every
   currently-firing default alert and either (a) confirm it's signal we want,
   or (b) add a named silence with a documented justification. Silences live
   in-repo (Alertmanager config), not as ad-hoc Alertmanager UI silences.

## Pending Operator Actions

Tracked here so they don't get lost while the TDD is being written. These
gate *execution* of the canary/mktxp workstream (decision #2). The original
plan included a RouterOS password rotation + api-ssl switch; the operator
opted out (2026-05-23) on the grounds that the password was never pushed to
git, the user is read-only, and the routers are on a private LAN. Trade-off
documented; revisit if threat model changes.

- [x] ~~Store the existing `mktxp_user` password in Bitwarden as item `mktxp-routers`~~ — done 2026-05-23 via bitwarden-cli pod (UUID `7518356e-151c-4cc3-8970-e7c718a4dedd`).
- [x] ~~Fill the Bitwarden UUID into `external-secret-mktxp.yaml`~~ — done in commit `d1f81030`.
- [x] ~~Apply the ExternalSecret, restart Deployment~~ — done. ESO reconciles, Secret renders correctly, CCR-2004 scrape verified (20 active DHCP leases visible).
- [x] ~~Commit `external-secret-mktxp.yaml`~~ — `d1f81030`.
- [ ] **Investigate cAP (10.0.0.254) auth failure.** mktxp logs show `invalid user name or password (6)` on cAP only. CCR-2004 works with the same credentials. Possible causes: `mktxp_user` doesn't exist on cAP, cAP's `mktxp_user` has a different password, or cAP's API is disabled. Tracked as GH issue (Phase 4 follow-up).

**Deferred (operator may revisit later):**
- Rotation of the mktxp password.
- Switch RouterOS API to api-ssl (port 8729). Commands when ready:
  `/ip service enable api-ssl` + `/ip service disable api` on each router.
  Then flip `use_ssl=True`, `port=8729`, `plaintext_login=False` in
  `mktxp.conf.example` AND `external-secret-mktxp.yaml`.

## Handoff

This PRD is the start of a chain:

1. **Operator review** — ✅ done 2026-05-23. Decisions captured above.
2. **@staff-engineer** — produce a TDD covering: the exact list of dashboards to keep /
   delete / fix; alert curation (silences for defaults + the five enabled Loki rules);
   the Loki S3 migration plan with rollback steps; the loki-gateway removal sequence
   (Alloy + Grafana repoint, then disable); the Tempo instrumentation rollout plan
   (which API first, then the discovery-driven fan-out); the Mikrotik exporter
   recovery + commit-to-git workstream; the email Alertmanager receiver wiring;
   the helm reconciliation procedure for each release.
3. **@project-manager** — decompose the approved TDD into Docket issues. Likely shape:
   one "discovery / inventory" issue, one issue per chart reconciliation, one issue per
   keep/kill decision execution, one issue for runbook + README documentation. Expect
   the work to fit a single sprint if the open questions are decided cheaply.
4. **@devops-engineer** — implementation, behind operator approval for each apply.
5. **@product-owner** (me) — review the rolled-up result against the success criteria in
   "Goals" and "User Stories" before marking the project complete.

---
project: "iac"
maturity: "ready-for-decomposition"
last_updated: "2026-05-23"
updated_by: "@staff-engineer"
scope: "Reconcile the home-cluster observability stack (kube-prometheus-stack + Loki + Alloy + Tempo) so dashboards/alerts/scrape-targets reflect cluster reality, git matches the cluster, the half-done Loki S3 migration is either completed or reverted, and Tempo is wired to at least one in-cluster producer."
owner: "@staff-engineer"
dependencies:
  - ../prd/observability-cleanup.md
---

## 1. Scope summary

Source PRD: [`docs/prd/observability-cleanup.md`](../prd/observability-cleanup.md). The
six operator decisions are committed direction:

1. **Tempo: keep, wire it.** Khemeia API + surf-api OTLP, plus a discovery pass.
2. **Loki canary: keep, fix.** Currently failing tail queries; reuse for mktxp scrape verification.
3. **Loki chart alerts: enable full set.** `monitoring.rules.enabled: true`, all five rules.
4. **loki-gateway: remove.** Disable, repoint Alloy + Grafana datasource at `loki:3100`.
5. **Alert destination: email (interim).** SMTP via ExternalSecret, `hampton888@gmail.com`.
6. **defaultRules: keep on, curate silences.** In-repo Alertmanager config, not UI silences.

Out of scope (from PRD): replacing the stack, retention/cost tuning, new dashboards for
application workloads, multi-tenancy, synthetic monitoring.

---

## 2. Current-state inventory

Captured 2026-05-23 against the live cluster. This section is the discovery the PRD's
success criteria require; everything downstream depends on it being correct.

### 2.1 Helm releases in `monitor` namespace

| Release | Chart | Version | Rev | Notes |
|---|---|---|---|---|
| `kube-prometheus-stack` | kube-prometheus-stack | 82.14.1 (app v0.89.0) | 5 | Multiple `node-exporter-values.yaml` overrides applied via kubectl patch, never folded into the values file (drift). |
| `loki` | loki | 6.51.0 (app 3.6.4) | 1 | Single-binary, filesystem storage, `existingClaim: storage-loki-0` (20Gi). |
| `alloy` | alloy | 1.6.2 (app 1.14.0) | 2 | DaemonSet, ships container logs + Mikrotik syslog → `loki-gateway`. |

Drift between `helm get values <release>` and the in-repo file is summarized in §2.7.

### 2.2 Dashboard ConfigMaps (`grafana_dashboard: "1"`)

The Grafana sidecar searches all namespaces. Total found: **33** ConfigMaps.

| ConfigMap | Source | Verdict | Justification |
|---|---|---|---|
| `monitor/grafana-self-perf-dashboard` | file (untracked) | **fix + keep + commit** | Triggered the PRD ("100% red" panel). With Loki ServiceMonitor live, panels should now populate. Verify, then commit. |
| `monitor/kubetest-power-dashboard` | file (untracked) | **keep + commit** | RAPL power dashboard. Backed by node-exporter RAPL collector (already enabled). Active workstream. |
| `monitor/cs2-surf-server-dashboard` | `rke2/game-server/cs2/cs2-surf/k8s/grafana-dashboard.yaml` | keep | Owned by cs2-surf workstream. Not in scope to extend; just confirm renders. |
| `monitor/khemeia-docking-dashboard` | `rke2/chem/khemeia/deploy/grafana-dashboard-khemeia.yaml` (label-applied) | keep | Owned by khemeia. Out of scope per PRD. |
| `chem/grafana-dashboard-khemeia` | same file | keep | (same — note the same dashboard is loaded twice from two namespaces; cosmetic, but document once in README). |
| `monitor/kube-prometheus-stack-*` (29 dashboards) | chart-bundled | **mass-audit, default = keep** | Default kube-prometheus-stack dashboards. Methodology: §2.4. Drop only the obviously-irrelevant ones (e.g. `aix`, `darwin` node dashboards). |
| `monitor/kube-prometheus-stack-nodes-aix` | chart | **drop via `defaultDashboardsEnabled` carveout** | AIX. No AIX nodes. Dead. |
| `monitor/kube-prometheus-stack-nodes-darwin` | chart | **drop via carveout** | macOS. No macOS nodes (the planned Mac is for push-notifications, not a cluster node). |
| `monitor/kube-prometheus-stack-etcd` | chart | keep, but scrape needs fixing | etcd targets are DOWN — see §2.5; either fix the SM selector (RKE2 etcd port differs) or accept "No Data". |
| `monitor/kube-prometheus-stack-controller-manager` | chart | keep, scrape DOWN | RKE2 hides this; ServiceMonitor selector needs adjustment OR alert silenced. |
| `monitor/kube-prometheus-stack-scheduler` | chart | keep, scrape DOWN | Same as above. |
| `monitor/kube-prometheus-stack-proxy` | chart | keep, scrape DOWN | kube-proxy not running in RKE2 (Cilium kube-proxy replacement). Silence + drop. |

The chart's `defaultDashboardsEnabled: true` is implicit (default). To selectively drop
AIX/Darwin/etc., either patch with `defaultRules.disabled` (no equivalent for dashboards
exists pre-chart-version-65) **or** override the specific ConfigMap with a no-op patch.
**Decision:** Set `defaultDashboardsEnabled: true` (explicit) and **don't fight chart
dashboards**; tag with `grafana_folder` so the irrelevant ones live in a "Defaults"
folder the operator can collapse. This is cheaper than maintaining a carveout list.

### 2.3 ServiceMonitors / PodMonitors

| Object | NS | Target | Health | Verdict |
|---|---|---|---|---|
| ServiceMonitor `loki` | monitor | `loki:3100`, `loki-canary:3500` | up (just shipped `a9e15509`) | **Delete in same change as S3 migration** (§3) once chart-bundled `monitoring.serviceMonitor.enabled: true` is live. |
| ServiceMonitor `mktxp-exporter` | monitor | mktxp on :49090 | up (live) — but auth will fail post-rotation until ExternalSecret applied | Keep. Verify after operator rotation (PRD "Pending Operator Actions"). Note: also exists as a PodMonitor inside `mikrotik-exporter.yaml` — see §2.4 — duplication is fine but document. |
| ServiceMonitor `snmp-exporter-synology` | monitor | snmp-exporter → 10.41.0.200 | **DOWN** — HTTP 400 from SNMP target | **Fix or remove.** Likely a Synology SNMPv2c community/auth mismatch. Out-of-scope-adjacent: include as cleanup item but do not block. |
| ServiceMonitor `khemeia-controller` | chem | khemeia API `/metrics` | up | Keep. (Already Prometheus-instrumented; OTLP is what's pending.) |
| ServiceMonitor `longhorn-prometheus-servicemonitor` | longhorn-system | longhorn manager | up | Out of scope. |
| ServiceMonitor `zot` | tooling | zot registry | up | Out of scope. |
| ServiceMonitor `kube-prometheus-stack-kube-controller-manager` | monitor | RKE2 controller-manager :10257 | **DOWN** (connection refused) | RKE2 doesn't expose this metric port by default. **Disable in values** (`kubeControllerManager.enabled: false`) — alert noise. |
| ServiceMonitor `kube-prometheus-stack-kube-scheduler` | monitor | RKE2 scheduler :10259 | **DOWN** | **Disable in values** (`kubeScheduler.enabled: false`). |
| ServiceMonitor `kube-prometheus-stack-kube-etcd` | monitor | etcd :2381 | **DOWN** | RKE2 etcd listens on a different port + needs cert mTLS. **Decision:** disable in values (`kubeEtcd.enabled: false`) and accept loss of etcd metrics until a follow-up wires it via the RKE2 etcd snapshot port + cert. |
| ServiceMonitor `kube-prometheus-stack-kube-proxy` | monitor | kube-proxy :10249 | **DOWN** | Cilium kube-proxy replacement — kube-proxy doesn't exist. **Disable** (`kubeProxy.enabled: false`). |
| ServiceMonitor `kube-prometheus-stack-kubelet` | monitor | kubelet | up | Keep. |
| ServiceMonitor `kube-prometheus-stack-coredns` | monitor | CoreDNS | up | Keep. |
| ServiceMonitor `kube-prometheus-stack-apiserver` | monitor | apiserver | up | Keep. |
| Others (alertmanager, grafana, ksm, node-exporter, operator, prometheus) | monitor | self-monitor | up | Keep. |
| PodMonitor `cilium-agent` | monitor | cilium :9962 | up on 5/6 nodes, **DOWN on `nixos-gpu`** | Investigate as part of nixos-gpu cleanup (file separately — already a known wart per [[project_gpu_node]]). |
| PodMonitor `mktxp-exporter` | monitor | mktxp :49090 | (duplicate of SM above) | Keep, with note explaining duplication or pick one. |

**Action items from §2.3:**
- Disable four broken chart-default SMs (`kubeControllerManager`, `kubeScheduler`, `kubeEtcd`, `kubeProxy`) in `kube-prometheus-stack-values.yaml`.
- Decide on PodMonitor-vs-ServiceMonitor for mktxp (recommend: keep PodMonitor in `mikrotik-exporter.yaml`, delete the auto-generated ServiceMonitor — pods are what we scrape).

### 2.4 PrometheusRules (35 total)

- 1 hand-curated: `grafana-alert-rules-platform` (5 rules: pod-crash-loop, oom-killed, node-not-ready, pvc-filling-up, deployment-replicas-mismatch) — **delivered via Grafana sidecar, not as a PrometheusRule CR**. Note: this is Grafana-native alerting, not Prometheus alerting; route is configured in Grafana, not Alertmanager. **Decision needed:** unify on Alertmanager (operator wants email — the simplest path is Alertmanager with `defaultRules` + chart-bundled Loki rules; the 5 Grafana-native rules can stay in Grafana or migrate). **Recommend:** keep Grafana-native for now, leave note in README, revisit only if double-firing occurs.
- 34 chart-default `kube-prometheus-stack-*` rules. Currently firing in cluster (snapshot 2026-05-23):

| Firing alert | Count | Severity | Disposition |
|---|---|---|---|
| `TargetDown` | 4 (kube-system) | warning | **Resolved by §2.3 disabling broken SMs.** |
| `TargetDown` | 1 (monitor) | warning | snmp-exporter — fix or silence with reason. |
| `KubeDaemonSetMisScheduled` | 3 (longhorn) | warning | Longhorn DS scheduling on nodes it shouldn't (e.g. nixos-gpu). **Silence** with reason: "longhorn DS uses node-selectors; misscheduled count is expected on this cluster." |
| `KubeDaemonSetRolloutStuck` | 3 (longhorn) | warning | Same root cause — **silence with same reason**. |
| `KubeJobFailed` | 2 (game-server) | warning | Real signal — old cs2 demo-upload cronjob failures. **Investigate, don't silence**; possibly clean up failed Job objects. |
| `etcdMembersDown` | 1 (kube-system) | warning | Caused by broken etcd SM (§2.3). **Goes away when SM is disabled.** |
| `etcdInsufficientMembers` | 1 (kube-system) | critical | Same as above. |
| `KubeVersionMismatch` | 1 (-) | warning | RKE2 node-version drift across nixos-gpu vs other nodes. Real signal, **needs investigation or 1-week silence**. |
| `KubeProxyDown` | 1 (-) | critical | kube-proxy not present (Cilium). **Goes away when kubeProxy SM is disabled.** |
| `Watchdog` | 1 | none | Synthetic always-firing canary. Expected. Routed to `null` receiver (already in default config). |

**Curation methodology (decision #6):**

We commit to **two channels** for silences, each with a documented "why":

1. **`alertmanager.config.inhibit_rules`** (in `kube-prometheus-stack-values.yaml`):
   for cross-alert relationships (e.g. "if critical fires, suppress warning/info for the
   same {namespace,alertname}"). The default config already has these.
2. **PrometheusRule patches via `defaultRules.disabled` in the chart**: for permanent
   silences (alerts that don't apply to this cluster). Example: `KubeProxyDown` if for
   some reason we couldn't disable the kubeProxy SM cleanly.

Per-incident silences (e.g. "longhorn misscheduled on nixos-gpu for the next 30 days while
we sort out the node taints") live as named entries in an **in-repo silences file**
(`rke2/monitor/alertmanager-silences.yaml`) consumed by a `silences` sidecar or
applied via a CronJob calling `amtool silence add`. **Decision: defer the silence sidecar
machinery to a follow-up**; for the initial cleanup, use chart values (`alertmanager.config`)
for both inhibit_rules and a small set of always-on silences. If the silences list grows
past ~5 entries, revisit.

**Workflow for new silences:**
1. Operator pages → opens `rke2/monitor/alertmanager-silences.yaml` (or chart values).
2. Adds entry with `matchers`, `endsAt`, `comment` (the why).
3. Commits with the runbook link or incident reference in the commit message.
4. `helm upgrade` to roll out.

### 2.5 Workloads in `rke2/monitor/` — running vs. git

| Workload (live) | Source | Matches git? | Notes |
|---|---|---|---|
| `alertmanager-...-0` (StatefulSet) | kube-prometheus-stack chart | ✓ | Config = chart defaults; no in-repo Alertmanager config yet. |
| `alloy-{4vclx,mltm5}` (DaemonSet) | `loki/alloy-values.yaml` | ≈ | Live matches file. Writes to `loki-gateway` (to be repointed §4). |
| `kube-prometheus-stack-grafana-...` (Deploy) | chart values | ≈ | OK. |
| `kube-state-metrics`, `operator`, `prometheus`, `node-exporter` | chart | ✓ | OK except: node-exporter has live `--collector.rapl` + `runAsUser:0` patches that exist in `node-exporter-values.yaml` (untracked-but-present) but were never folded into the helm release values. **Drift.** Live = patched-via-kubectl; git = chart defaults. |
| `loki-0` (StatefulSet) | `loki/loki-values.yaml` | ✗ | **Major drift — see §3.** Live = filesystem + `existingClaim: storage-loki-0` 20Gi. Git HEAD = filesystem + `size: 20Gi`. Local uncommitted = S3 + `size: 5Gi`. Three states. |
| `loki-canary-{8zqzc,nfmmr}` (DaemonSet) | chart-bundled | ✓ | Currently degraded — push 499s, tail 502s. Root cause: alloy/canary point at `loki-gateway` with trailing-dot FQDN, gateway nginx can't resolve. Fix: §4 (kill the gateway). |
| `loki-chunks-cache-0`, `loki-results-cache-0` | chart-bundled memcached | ✓ | Keep. Tied to chart, fine. |
| `loki-gateway-...` (Deploy) | chart-bundled | ✓ | **To be removed §4.** |
| `mktxp-exporter` (Deploy) | `mikrotik-exporter/mikrotik-exporter.yaml` (committed) + `mikrotik/mikrotik-exporter.yaml` (older copy, also committed) | ≈ | **Two copies of the same Deployment in git.** Old at `mikrotik/`, new sanitized at `mikrotik-exporter/`. Live = the newer one (118 days old → predates the sanitized commit; suggests the same manifest was applied previously by hand). **Action: delete `rke2/monitor/mikrotik/` directory.** |
| `snmp-exporter-synology` (Deploy) | `synology-exporter/synology-exporter.yaml` (untracked) | ≈ | Live present, file untracked. **Commit the file**, then investigate the HTTP 400 separately. |
| `tempo-0` (StatefulSet) | `tempo/tempo-deployment.yaml` | ✓ | Matches. Receives 0 OTLP traffic — to be fixed §5. |

### 2.6 Untracked files in `rke2/monitor/`

From `git status --short`:

| File | Action |
|---|---|
| `loki/loki-values.yaml` (MODIFIED) | Revert local drift to HEAD; re-add S3 keys with **correct chart 6.51.0 schema** in §3. |
| `grafana-self-perf-dashboard.yaml` | Commit (audit + fix first, §2.2). |
| `kubetest-power-dashboard.yaml` | Commit. |
| `loki/external-secret-minio.yaml` | Commit (after Bitwarden UUID filled in — §3). |
| `loki/minio-compose.yaml` | **Decide: keep where, or move.** This is a docker-compose for the MinIO instance that runs on the Synology, not k8s. Wrong directory — move to `infra-notes/` or delete and reference Synology config separately. |
| `mikrotik-exporter/external-secret-mktxp.yaml` | Commit (after UUID filled — covered by PRD pending actions, §6). |
| `node-exporter-values.yaml` | **Fold into `kube-prometheus-stack-values.yaml`** (so a single `-f` does both), then delete. Comment in file already says it was applied via kubectl patch — drift. |
| `synology-exporter/` (entire dir) | Commit. |

### 2.7 Git drift table (file → live → resolution)

| File | git state | live state | resolution |
|---|---|---|---|
| `loki/loki-values.yaml` | filesystem + 20Gi (HEAD) | filesystem + `existingClaim: storage-loki-0` (20Gi PVC) | Phase 0 of §3: align git HEAD to use `existingClaim`. Then §3 migration. |
| `kube-prometheus-stack-values.yaml` | chart defaults + Grafana customization | + node-exporter RAPL patch + four broken kube-* SMs untouched | Fold node-exporter-values.yaml in; add `kubeProxy.enabled: false`, `kubeScheduler.enabled: false`, `kubeControllerManager.enabled: false`, `kubeEtcd.enabled: false`. |
| `loki/alloy-values.yaml` | writes to `loki-gateway` | matches | §4 changes endpoint to `loki:3100`. |
| `tempo/tempo-deployment.yaml` | as-is | matches | No drift; just no producers. §5 fixes producer side. |
| `loki/servicemonitor.yaml` | exists | exists | **Delete in same PR as §3** (chart's bundled SM takes over). |
| `mikrotik/mikrotik-exporter.yaml` | exists | superseded by `mikrotik-exporter/` | **Delete `mikrotik/` directory.** |

---

## 3. Loki S3/MinIO migration plan

### 3.1 Goal & constraints

- Move Loki's chunks + index from filesystem (Longhorn 20Gi PVC `storage-loki-0`) to MinIO at `10.41.0.200:9000`, bucket `loki-chunks`.
- **Cannot lose existing chunks** (NF1 from PRD). Today's `loki-0` is the only copy of N days of logs.
- Use Loki chart 6.51.0 schema: keys are `bucketNames.chunks`, `bucketNames.ruler`, `bucketNames.admin`, and `s3ForcePathStyle` (camelCase). The locally-edited `loki-values.yaml` uses the wrong (Loki-app-native) snake_case keys; that's why the PRD notes `helm upgrade` would fail.

### 3.2 Pre-flight checklist

- [ ] Operator confirms MinIO at `10.41.0.200:9000` is healthy: `mc admin info localminio` or `curl -f http://10.41.0.200:9000/minio/health/live`.
- [ ] Bucket `loki-chunks` exists with rw perms for the access key. `mc mb localminio/loki-chunks` if not.
- [ ] Bitwarden item for MinIO Loki credentials exists; UUID known; `loki/external-secret-minio.yaml` has the real UUID (currently `<bitwarden-uuid>`).
- [ ] ExternalSecret applied; `kubectl get secret -n monitor loki-minio-secret` returns 2 keys (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`).
- [ ] Backup snapshot of `storage-loki-0` PVC via Longhorn UI (recurring backup OR one-shot). **Hard gate** — do not proceed without this.

### 3.3 Migration approach: SCHEMA BOUNDARY + COEXISTENCE (recommended)

Two viable strategies for "don't lose chunks":

**Option A — Schema boundary (recommended).** Loki's `schemaConfig` supports multiple
entries differentiated by `from:` date. Add a second schema entry that uses `s3` from a
future date (e.g. tomorrow), leaving the existing `filesystem`-period schema in place.
Loki queries both stores transparently; writes go to the new store.

- **Strengths:** zero data motion, zero downtime risk, fully reversible (revert values, the filesystem schema still applies).
- **Weaknesses:** the filesystem PVC stays around forever (or until a retention boundary expires it); Loki must keep being able to read both stores; a future PV detach without S3 having all chunks is a footgun. Need to keep `existingClaim: storage-loki-0` indefinitely.
- **Operational fit:** matches "we don't want to lose chunks" + "single solo operator who context-switches" — set it and forget it.

**Option B — One-shot migration job.** Run a Kubernetes Job that uses `loki migrate` (or
`mc mirror` for chunks + tsdb files into the bucket structure Loki expects) before
cutting over. Then change schema to s3 wholesale and detach the PVC.

- **Strengths:** clean end state, no perpetual filesystem PVC.
- **Weaknesses:** chunk-file ↔ S3-key mapping is non-obvious; high probability of needing two attempts; risk of permanent data loss if `mc mirror` misses files. `loki migrate` is for Cassandra/BoltDB → tsdb, not filesystem → S3.

**Recommendation: Option A.** The chunks-on-Longhorn cost (20Gi) is trivial compared to
the risk of a botched data migration on the cluster the operator actually relies on.
Schedule a separate follow-up to retire the filesystem PVC once a defined retention window
has passed (e.g. 90 days after S3 cutover).

### 3.4 Exact values-file diff (illustrative — chart 6.51.0 schema)

Reverting `singleBinary.persistence` to use the existing claim and using correct camelCase
keys for the s3 sub-block:

```yaml
# rke2/monitor/loki/loki-values.yaml — proposed
deploymentMode: SingleBinary

global:
  dnsService: "rke2-coredns-rke2-coredns"
  dnsNamespace: "kube-system"

backend: {replicas: 0}
read:    {replicas: 0}
write:   {replicas: 0}

singleBinary:
  replicas: 1
  persistence:
    enabled: true
    existingClaim: storage-loki-0  # keep using the 20Gi PVC, do NOT reprovision
  extraEnvFrom:
    - secretRef:
        name: loki-minio-secret

monitoring:
  serviceMonitor:
    enabled: true   # supersedes the hand-rolled rke2/monitor/loki/servicemonitor.yaml
  rules:
    enabled: true   # decision #3: enable all 5 chart-bundled Loki alerts

lokiCanary:
  enabled: true     # already on by default; explicit per decision #2

gateway:
  enabled: false    # decision #4

loki:
  auth_enabled: false
  commonConfig:
    replication_factor: 1

  # Boundary: existing filesystem schema below; new s3 schema starts the day
  # AFTER cutover (so writes-in-flight don't span schemas).
  schemaConfig:
    configs:
      - from: "2024-04-01"        # original
        store: tsdb
        object_store: filesystem
        schema: v13
        index:
          prefix: index_
          period: 24h
      - from: "<CUTOVER_DATE+1>"  # to be filled at execution time, YYYY-MM-DD
        store: tsdb
        object_store: s3
        schema: v13
        index:
          prefix: s3_index_
          period: 24h

  storage:
    type: s3
    s3:
      endpoint: http://10.41.0.200:9000
      bucketNames:                # CAMEL CASE — required by chart 6.51.0
        chunks: loki-chunks
        ruler: loki-chunks
        admin: loki-chunks
      region: us-east-1
      s3ForcePathStyle: true
      insecure: true
```

Things changed vs. the operator's local drift:
- `bucketnames` → `bucketNames.{chunks,ruler,admin}` (chart schema, will pass validation).
- `s3forcepathstyle` → `s3ForcePathStyle`.
- `size: 5Gi` → `existingClaim: storage-loki-0` (do not reprovision; current PVC is 20Gi and holds live chunks).
- Added second schema entry rather than replacing the filesystem one.
- `monitoring.serviceMonitor.enabled: true`, `monitoring.rules.enabled: true`, `gateway.enabled: false`.

### 3.5 Cutover sequence

1. **Apply ExternalSecret** for MinIO creds (no Loki restart).
2. **`helm diff upgrade loki ...`** with the new values. Expect no destructive changes (PVC kept). Operator reviews diff.
3. **Pick `<CUTOVER_DATE+1>`** = the next UTC day boundary. Fill into values.
4. **`helm upgrade loki ...`**. Loki pod restarts (StatefulSet single replica, expect ~60s gap).
5. **Smoke test (write):** push a synthetic log line via `curl` to the gateway-less endpoint:
   ```bash
   curl -H "Content-Type: application/json" \
     -X POST http://loki.monitor.svc.cluster.local:3100/loki/api/v1/push \
     -d '{"streams":[{"stream":{"job":"smoke","cutover":"s3"},"values":[["'$(date +%s%N)'","s3 cutover smoke test"]]}]}'
   ```
6. **Smoke test (read, post-cutover):** `{job="smoke"}` query in Grafana → must return the synthetic line within ~10s.
7. **Smoke test (read, pre-cutover):** any known-existing log (e.g. `{namespace="chem"}` for older logs) → must still return data (proves filesystem schema still reads).
8. **Smoke test (S3 object existence):** `mc ls localminio/loki-chunks/` → at least one `chunks/...` object created.
9. **Watch alerts**: `LokiRequestErrors` should remain quiet through the next 30 minutes.
10. **Commit** the new `loki-values.yaml`. Delete `rke2/monitor/loki/servicemonitor.yaml` in the SAME commit (the chart now manages it).

### 3.6 Rollback procedure

If steps 5–8 fail:
1. `helm rollback loki <previous-rev>` — restores filesystem-only schema and chart-bundled SM disabled.
2. Re-apply the standalone `loki/servicemonitor.yaml` if it was deleted (or revert the commit).
3. Loki pod restart; reads + writes resume from filesystem PVC. No chunks lost (we never deleted any).
4. Capture failure mode (`kubectl logs loki-0 -c loki`, `mc admin trace localminio`).

### 3.7 Servicemonitor decision: delete-with-migration

**Recommend: delete `rke2/monitor/loki/servicemonitor.yaml` in the SAME change as the
S3 migration.** Two reasons:
- The chart-bundled SM has different labels/selector; running both produces duplicate scrapes.
- The whole point of the band-aid was "delete once chart upgrade lands." This IS that upgrade.
- A follow-up PR to delete it would be a pure churn commit.

### 3.8 Open question: existing chunk fate

The recommended Option A keeps filesystem chunks queryable forever (or until the PV is
manually deleted). The PRD says "log retention / cost tuning" is out of scope, so the
20Gi keeps growing... actually it doesn't, because the filesystem schema gets no new
writes after cutover (write goes to s3). The existing 20Gi is **frozen at its current
size + whatever pre-cutover logs are already there**. Operator should decide: keep that
PVC forever (cheap, ~20Gi Longhorn) or schedule PVC deletion after retention window
(say 90 days). **Recommend: keep, revisit in 90 days.** Tracked as Open Question Q1.

---

## 4. loki-gateway removal sequence

### 4.1 Consumer audit

`grep -r loki-gateway rke2/ docs/` finds:
- `rke2/monitor/loki/alloy-values.yaml:80` — `loki.write` endpoint URL.
- `rke2/monitor/kube-prometheus-stack-values.yaml:42` — Grafana datasource URL.
- `helm get values loki` → `gateway.enabled` (default true → set false).

NOT found (cross-checked):
- No HTTPRoute exposes `loki-gateway` (Loki has no public DNS; only internal).
- No curl-based runbook in docs/ references it (no `docs/` runbooks exist yet — to be created via the README task).
- Loki canary's writes go to `loki-gateway` too (canary chart sub-chart default) — when
  gateway is disabled by the loki chart, the canary's endpoint flips to `loki:3100`
  automatically. **Verify:** read `loki-canary` Deployment spec post-`helm upgrade` to
  confirm `-addr=loki:3100`. (Per chart values, canary's `address` derives from
  `gateway.enabled`; when false, canary points at the single-binary service directly.)

### 4.2 Sequence

Doing this in the same `helm upgrade` as §3 risks coupling two failure modes. **Recommend
splitting**:

1. **Change A (§3 first):** Loki S3 migration + chart-bundled SM. Gateway still on.
2. **Smoke-test for ~24h** — let alerts settle, observe canary metrics.
3. **Change B (§4 here):** Repoint Alloy + Grafana, disable gateway, in ONE `helm upgrade` per release.

But: §3 alone already cancels and recreates the loki StatefulSet. Adding gateway-off would
double the canary noise. Splitting is safer.

**Change B steps:**
1. Edit `rke2/monitor/loki/alloy-values.yaml` — change `loki.write "local"` URL to
   `http://loki.monitor.svc.cluster.local:3100/loki/api/v1/push`.
2. Edit `rke2/monitor/kube-prometheus-stack-values.yaml` — change Grafana Loki datasource
   `url` to `http://loki.monitor.svc.cluster.local:3100`.
3. Edit `rke2/monitor/loki/loki-values.yaml` — set `gateway.enabled: false` (already in §3.4 above; can be deferred to here if §3 is being kept minimal).
4. `helm upgrade alloy ...` first — Alloy restarts, points at `loki:3100`, confirm push 200s in `loki-0` logs (`grep "POST /loki/api/v1/push" + grep " 204 "`).
5. `helm upgrade kube-prometheus-stack ...` — Grafana picks up new datasource on next pod restart (datasource is provisioned at startup; either restart Grafana pod or wait for next sidecar reload).
6. `helm upgrade loki ...` — gateway Deployment disappears, canary should auto-repoint.
7. **Verify the original incident is gone**: `kubectl logs loki-canary-* | grep -c "context deadline exceeded"` → should drop to ~0.

### 4.3 Rollback

Per-step rollback: revert the values file change for that release and `helm rollback`.
Most-likely failure mode: Grafana caches the old datasource and continues to error 502
until restart — workaround is `kubectl rollout restart deploy/kube-prometheus-stack-grafana`.

---

## 5. Tempo instrumentation plan

### 5.1 Candidate discovery

Cluster HTTP services that are reasonable OTLP candidates, ranked by signal/effort:

| Service | Lang/Framework | Existing OTLP? | Effort | Value | Rank |
|---|---|---|---|---|---|
| `chem/khemeia-controller` | Go (FastAPI-like, custom router) | **YES — fully wired in `telemetry.go`, just lacks `OTEL_EXPORTER_OTLP_ENDPOINT` env var in deploy manifest** | XS (env var) | HIGH (active workstream) | **1 — fastest win** |
| `game-server/surf-api` | Python / FastAPI 0.115 + uvicorn | NO | S (add `opentelemetry-instrumentation-fastapi`, init in `main.py`) | HIGH (recent code, small surface) | **2 — operator's nominated first** |
| `chem/library-prep`, `target-prep`, `admet`, `fpocket`, `p2rank`, `prolif-runner`, `gnina`, `vina-gpu` | Python / Flask + gunicorn | NO | M (8 services × adding flask middleware + gunicorn config) | MEDIUM (already metric'd via Prometheus; traces would show docking pipeline causality) | 3 (fan-out post-MVP) |
| `chem/result-writer` | Python | NO | S | LOW (not user-facing; writes DB rows) | 4 |
| `authentik/authentik-server` | Python / Django | NO | M (third-party — would need to confirm Authentik upstream supports OTLP out of the box; rather not patch) | LOW (auth is already low-incident) | skip |
| `plex-system/{sonarr,radarr,prowlarr,lidarr,sabnzbd,overseerr}` | mixed (.NET / Node) | NO | high per-service | LOW (operator doesn't debug these via traces) | skip |
| `theswamp/u4u-engine` | unknown | NO | unknown | MEDIUM (could be useful for the FBDD workstream but separate project) | skip |
| `ai/*` (ollama, comfyui, etc.) | NO | N/A | LOW (model inference latency isn't a span-shaped problem) | skip |

**Pick for first end-to-end proof: khemeia-controller.** Why over surf-api:
- Code is **already instrumented**. The only thing missing is the env var. Time-to-first-trace ≈ 5 minutes.
- Goes from `OTEL_EXPORTER_OTLP_ENDPOINT="tempo.monitor:4317"` → `helm`/manifest apply → restart → traces appear.
- Proves the Tempo write-path + Grafana trace-search end-to-end without touching app code.
- Then surf-api (decision #1's other named target) becomes the "instrument from scratch" case study — write the recipe once on surf-api, fan out.

### 5.2 Khemeia instrumentation (MVP)

Add to `rke2/chem/khemeia/deploy/api-deployment.yaml` under the controller container's `env:`:

```yaml
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: "tempo.monitor.svc.cluster.local:4317"
- name: OTEL_SERVICE_NAME
  value: "khemeia-controller"
- name: OTEL_TRACES_SAMPLER
  value: "parentbased_traceidratio"
- name: OTEL_TRACES_SAMPLER_ARG
  value: "0.1"   # 10% sampling — homelab volume, no need for 100%
```

Smoke test:
1. `kubectl rollout restart deploy/khemeia-controller -n chem`.
2. Issue a few API requests (`curl khemeia.hwcopeland.net/api/health`).
3. Grafana → Explore → Tempo datasource → search for service `khemeia-controller`, last 15m → spans appear.
4. From a span, click trace-to-logs → should jump to Loki query filtered by trace_id (per `tracesToLogsV2` in `tempo-grafana-datasource.yaml`). For this to work, **khemeia must log the trace_id**. Verify the logger; if missing, add a separate task to inject trace_id via slog/whatever.

### 5.3 surf-api instrumentation (recipe-write target)

Add to `surf-web/api/pyproject.toml` (illustrative):

```toml
dependencies = [
  # existing...
  "opentelemetry-distro>=0.50",
  "opentelemetry-exporter-otlp>=1.30",
  "opentelemetry-instrumentation-fastapi>=0.50",
]
```

In `app/main.py` before `app = FastAPI(...)`:

```python
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
# ... after FastAPI is created:
FastAPIInstrumentor.instrument_app(app)
```

Env in `rke2/game-server/cs2/surf-web/k8s/deployment.yaml`:

```yaml
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: "http://tempo.monitor.svc.cluster.local:4318"   # HTTP for Python default
- name: OTEL_EXPORTER_OTLP_PROTOCOL
  value: "http/protobuf"
- name: OTEL_SERVICE_NAME
  value: "surf-api"
- name: OTEL_TRACES_SAMPLER_ARG
  value: "0.5"   # leaderboard QPS is low; sample more
```

Smoke test: same as 5.2 with `service:surf-api`.

### 5.4 Fan-out: khemeia worker containers

After 5.2 + 5.3 pass, add a follow-up to instrument the Flask docking containers
(`library-prep`, `target-prep`, `admet`, etc.). Single recipe applies to all 8 (Flask
middleware + the same env block). This is decoupled from the cleanup TDD — propose as
**a separate follow-up TDD** or a docket epic since it touches 8 chem containers and the
common Python toolkit work already planned ([[project_khemeia_worker_sdk]]).

### 5.5 Tempo retention

Tempo's `tempo-config.yaml` has no retention block — it defaults to keeping traces
forever, bounded only by the 20Gi PVC. At 10% sampling on khemeia + 50% on surf-api, this
is fine for months. **Defer retention tuning** unless the PVC fills.

### 5.6 Open question: TraceQL service-map dependency

`tempo-grafana-datasource.yaml` enables `serviceMap` with the Prometheus datasource UID.
For the service map to render, Tempo's metrics-generator must be enabled, which requires a
Prometheus remote-write target. **Currently not configured.** First instrumentation will
NOT show a service map. Worth fixing in a follow-up; not a blocker.

---

## 6. Mikrotik exporter recovery & verification

Assumes operator completes PRD "Pending Operator Actions":
1. Password rotated on both routers.
2. `api-ssl` enabled, `api` disabled (port 8729 + TLS).
3. `mktxp_user` group = `read`.
4. New password stored in Bitwarden as item `mktxp-routers`, UUID captured.
5. `<BITWARDEN-UUID-MKTXP>` filled into `rke2/monitor/mikrotik-exporter/external-secret-mktxp.yaml`.

### 6.1 Verification sequence

After operator completes 1–5:

1. **Apply the ExternalSecret:**
   ```bash
   kubectl apply -f rke2/monitor/mikrotik-exporter/external-secret-mktxp.yaml
   kubectl get externalsecret -n monitor mktxp-credentials -w
   ```
   Expect `STATUS=SecretSynced READY=True` within `refreshInterval` (1h) — force-trigger with
   `kubectl annotate externalsecret mktxp-credentials -n monitor force-sync=$(date +%s) --overwrite`
   if impatient.

2. **Verify the Secret contents:**
   ```bash
   kubectl get secret -n monitor mktxp-credentials -o jsonpath='{.data.mktxp\.conf}' \
     | base64 -d | head -20
   ```
   Confirm `username = mktxp_user` and a non-empty `password` line for both router blocks.

3. **Delete the manually-created secret** if present (so ESO becomes the owner):
   ```bash
   kubectl delete secret -n monitor mktxp-credentials
   # ESO will recreate it within seconds.
   ```

4. **Roll the exporter:**
   ```bash
   kubectl rollout restart deploy/mktxp-exporter -n monitor
   kubectl rollout status deploy/mktxp-exporter -n monitor --timeout=60s
   ```

5. **Smoke-test the exporter connectivity to the routers:**
   ```bash
   kubectl logs -n monitor deploy/mktxp-exporter --tail=50 | grep -E "(CCR-2004|cAP|error)"
   ```
   Expect "successfully connected" lines for both `[CCR-2004]` and `[cAP]`. Errors here mean
   the rotation / SSL switch / user perm is wrong — go back to operator pending-actions.

6. **Verify Prometheus is scraping:**
   ```bash
   kubectl exec -n monitor prometheus-kube-prometheus-stack-prometheus-0 -c prometheus \
     -- wget -qO- 'http://localhost:9090/api/v1/targets?state=active' \
     | grep mktxp
   ```
   Expect `health: up`.

7. **Smoke-test queries** in Grafana (paste into a panel or Prometheus UI):
   - `mktxp_interface_rx_bytes` — should return a sample per interface per router.
   - `mktxp_health_voltage` — voltage from CCR-2004.
   - `mktxp_wireless_clients_total` — from the cAP.

8. **Commit `external-secret-mktxp.yaml`** with the real UUID.

### 6.2 Cleanup

- Delete `rke2/monitor/mikrotik/mikrotik-exporter.yaml` and the `mikrotik/` directory (superseded by `mikrotik-exporter/`, both currently in git).
- Confirm the duplicate ServiceMonitor (auto-generated from `app: mktxp-exporter` selector match against the chart's catch-all? unlikely — there's no chart with that selector; confirm only one SM/PM exists per `kubectl get sm,pm -n monitor | grep mktxp`).

### 6.3 Tie-in with Loki canary

The PRD's decision #2 connects the canary fix to mktxp. The connection is the **`mikrotik_firewall` syslog source** in `alloy-values.yaml` — Mikrotik fires syslog → Alloy → Loki. The mktxp exporter is parallel (metrics, not logs) but operator views them together. **No code dependency**; verification side, after mktxp works, also tail `{job="mikrotik_firewall"}` in Loki for a known router-event (a forward log entry) to confirm the full Mikrotik observability path is alive.

---

## 7. Alertmanager email receiver

### 7.1 Bitwarden item shape

Create a new Bitwarden item:
- **Name:** `alertmanager-smtp`
- **Type:** Login
- **Username:** SMTP auth user (e.g. `hampton888@gmail.com` for Gmail app-password; or `apikey` for SendGrid; or whatever the chosen SMTP provider expects)
- **Password:** SMTP auth password / app-password / API key
- **Custom fields (Bitwarden field types — `text`):**
  - `smtp_host` → e.g. `smtp.gmail.com`
  - `smtp_port` → e.g. `587`
  - `smtp_from` → `alerts@hwcopeland.net` (or `hampton888@gmail.com`)
  - `smtp_to` → `hampton888@gmail.com`

The `bitwarden-fields` ClusterSecretStore (already established) reads custom fields by name.

### 7.2 ExternalSecret manifest (new file)

New file: `rke2/monitor/alertmanager-smtp-secret.yaml` — pulls all 6 values from the
Bitwarden item into a single `alertmanager-smtp` Secret.

```yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: alertmanager-smtp
  namespace: monitor
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-fields   # for the custom fields
  target:
    name: alertmanager-smtp
    creationPolicy: Owner
  data:
    - secretKey: smtp_user
      remoteRef: { key: "<BITWARDEN-UUID-SMTP>", property: "username" }
    - secretKey: smtp_password
      remoteRef: { key: "<BITWARDEN-UUID-SMTP>", property: "password" }
    - secretKey: smtp_host
      remoteRef: { key: "<BITWARDEN-UUID-SMTP>", property: "smtp_host" }
    - secretKey: smtp_port
      remoteRef: { key: "<BITWARDEN-UUID-SMTP>", property: "smtp_port" }
    - secretKey: smtp_from
      remoteRef: { key: "<BITWARDEN-UUID-SMTP>", property: "smtp_from" }
    - secretKey: smtp_to
      remoteRef: { key: "<BITWARDEN-UUID-SMTP>", property: "smtp_to" }
```

Note: `bitwarden-login` uses one store, `bitwarden-fields` the other. Verify exact store
names by `kubectl get clustersecretstore`. (The PRD says both `bitwarden-login` and
`bitwarden-fields` exist; mktxp uses `bitwarden-login` so this is parallel.)

### 7.3 Alertmanager config (in `kube-prometheus-stack-values.yaml`)

The chart supports `alertmanager.config` as inline YAML and `alertmanager.secrets` to
mount additional Secrets. Approach: mount `alertmanager-smtp` Secret as files, then
reference via the Alertmanager `*_file` directives.

```yaml
alertmanager:
  alertmanagerSpec:
    secrets:
      - alertmanager-smtp   # mounted at /etc/alertmanager/secrets/alertmanager-smtp/
  config:
    global:
      resolve_timeout: 5m
      # SMTP defaults — referenced by receivers below
      smtp_smarthost_file: /etc/alertmanager/secrets/alertmanager-smtp/smtp_host   # see note below
      smtp_from_file: /etc/alertmanager/secrets/alertmanager-smtp/smtp_from
      smtp_auth_username_file: /etc/alertmanager/secrets/alertmanager-smtp/smtp_user
      smtp_auth_password_file: /etc/alertmanager/secrets/alertmanager-smtp/smtp_password
      smtp_require_tls: true
    receivers:
      - name: email-operator
        email_configs:
          - to_file: /etc/alertmanager/secrets/alertmanager-smtp/smtp_to
            send_resolved: true
      - name: "null"
    route:
      receiver: email-operator
      group_by: [namespace, alertname]
      group_wait: 30s
      group_interval: 5m
      repeat_interval: 12h
      routes:
        - matchers: ['alertname = "Watchdog"']
          receiver: "null"
        - matchers: ['alertname = "InfoInhibitor"']
          receiver: "null"
    inhibit_rules:
      # keep the existing chart-default inhibit rules; they're sensible
      - source_matchers: ['severity = critical']
        target_matchers: ['severity =~ "warning|info"']
        equal: [namespace, alertname]
      - source_matchers: ['severity = warning']
        target_matchers: ['severity = info']
        equal: [namespace, alertname]
```

**Caveat:** Alertmanager supports `*_file` for some SMTP fields and not others, and the
`smtp_smarthost` format is `host:port` (so a single field), not host+port separately. The
above shows the intent; the implementer will need to either:

- (a) concatenate host+port at template-render time in the ExternalSecret (single field `smtp_smarthost = host:port` rendered server-side), OR
- (b) inline non-secret values (`smtp_smarthost`) directly in chart values and `_file` only for the secret bits (user/password). **Recommend (b)** — port and from-address are not secret; only username and password need file-mount.

Revised pragmatic config:

```yaml
alertmanager:
  alertmanagerSpec:
    secrets:
      - alertmanager-smtp
  config:
    global:
      smtp_smarthost: smtp.gmail.com:587   # inline — not a secret
      smtp_from: alerts@hwcopeland.net
      smtp_auth_username_file: /etc/alertmanager/secrets/alertmanager-smtp/smtp_user
      smtp_auth_password_file: /etc/alertmanager/secrets/alertmanager-smtp/smtp_password
      smtp_require_tls: true
    receivers:
      - name: email-operator
        email_configs:
          - to: hampton888@gmail.com    # inline
            send_resolved: true
      - name: "null"
    # ... route + inhibit_rules as above
```

This drops the ExternalSecret to just `username` + `password`. Bitwarden item only needs
those two fields. Reverts to `bitwarden-login` store (same as mktxp).

### 7.4 Test-fire procedure

1. `helm upgrade kube-prometheus-stack -f ...` → Alertmanager pod restart.
2. `kubectl exec -n monitor alertmanager-kube-prometheus-stack-alertmanager-0 -c alertmanager -- amtool config show` → confirm config loaded.
3. **Force a test alert** via amtool:
   ```bash
   kubectl exec -n monitor alertmanager-kube-prometheus-stack-alertmanager-0 -c alertmanager \
     -- amtool --alertmanager.url=http://localhost:9093 alert add \
        alertname=TestEmailReceiver severity=critical namespace=test \
        --annotation=summary="Test fire from observability-cleanup"
   ```
4. Wait `group_wait` (30s) + delivery latency. Operator confirms email arrives at `hampton888@gmail.com`.
5. **Resolve:** `amtool alert add ... --end-at=$(date -u -v+1S +%Y-%m-%dT%H:%M:%SZ)` or wait it out — confirm "[RESOLVED]" email arrives (validates `send_resolved: true`).
6. **Document** the test in a runbook entry in `rke2/monitor/README.md`.

### 7.5 Followups (Mac push notifications)

Once a Mac joins the cluster (decision #5 follow-up), swap the receiver. Likely implementations:
- `webhook_configs` posting to `ntfy.sh` (operator-hosted) or
- macOS-side helper (e.g. `terminal-notifier` triggered by an Alertmanager webhook).

The above email config makes that a one-line route change — receiver swap, not a teardown.

---

## 8. defaultRules curation methodology

(See §2.4 for the inventory.) Methodology, restated:

1. **All `defaultRules.create: true`** — keep the full chart-bundled rule set as the
   baseline. Don't fight upstream.

2. **Per-cluster carveouts via chart values, not silences**, when the rule fundamentally
   doesn't apply:
   - `kubeProxy.enabled: false` — no kube-proxy. Removes the SM AND silences `KubeProxyDown` indirectly (no target → no `up` series → alert doesn't evaluate to true... but the chart-bundled `KubeProxyDown` alert uses `absent()`, so it WILL still fire. Actual mitigation: disable the rule via `defaultRules.disabled.kubernetesSystem.KubeProxyDown: true`).
   - `kubeControllerManager.enabled: false`, `kubeScheduler.enabled: false`, `kubeEtcd.enabled: false` — same pattern; some require both `<component>.enabled: false` AND a corresponding `defaultRules.disabled` entry. Confirm at execution time by trial and error against `helm template`.

3. **Per-incident silences via in-repo Alertmanager config**, when the rule applies in
   principle but the firing is expected on this cluster:
   - `KubeDaemonSetMisScheduled{namespace="longhorn-system"}` — longhorn DS legitimately
     skips nodes. Add a silence (Alertmanager `inhibit_rules` with a synthetic InfoInhibitor
     pattern, OR a routed `null` receiver for that specific matcher).
   - `KubeDaemonSetRolloutStuck{namespace="longhorn-system"}` — same.

4. **Real signals get fixed, not silenced:**
   - `KubeJobFailed{namespace="game-server"}` — clean up old failed Jobs. Don't silence.
   - `KubeVersionMismatch` — investigate (probably the nixos-gpu node drift). Don't silence permanently.

5. **Documented why for every silence.** Either:
   - Chart values silences: comment in `kube-prometheus-stack-values.yaml`.
   - Per-incident silences: commit message links incident or PRD section.

6. **Review trigger:** quarterly. If a silence has been in place for 90 days without a
   re-firing, ask "is this still needed?" Add a `last_reviewed:` field in the values file
   comments as a forcing function.

7. **Never use the Alertmanager UI** to add silences — they don't survive Alertmanager
   pod restart (config is in ConfigMap/Secret) and they don't show up in git review.

---

## 9. Sequencing

The workstreams have partial dependencies. Proposed order, with what blocks what:

```
Phase 0 — Inventory commit (parallelizable, low-risk):
  ├─ Commit untracked manifests as-is (synology, kubetest-power, self-perf, node-exporter-values)
  ├─ Delete duplicate rke2/monitor/mikrotik/ directory
  └─ Fold node-exporter-values.yaml into kube-prometheus-stack-values.yaml

Phase 1 — kube-prometheus-stack drift reconciliation:
  ├─ Disable kubeProxy/kubeScheduler/kubeControllerManager/kubeEtcd SMs
  ├─ Disable corresponding default rules
  ├─ Set up Alertmanager SMTP receiver (§7) — requires the Bitwarden item
  └─ helm upgrade kube-prometheus-stack
     ↓
Phase 2 — Loki S3 migration (§3) — must precede Phase 3 to avoid two cancel/recreate cycles on loki-0:
  ├─ Apply ExternalSecret for MinIO creds
  ├─ helm upgrade loki with new values + chart-bundled SM + monitoring.rules
  ├─ Delete rke2/monitor/loki/servicemonitor.yaml (same commit)
  └─ Smoke tests (write, read pre-cutover, read post-cutover)
     ↓
Phase 3 — loki-gateway removal (§4):
  ├─ helm upgrade alloy with loki:3100 endpoint
  ├─ helm upgrade kube-prometheus-stack with loki:3100 Grafana datasource URL
  └─ helm upgrade loki with gateway.enabled: false
     ↓
Phase 4 — Mikrotik exporter recovery (§6) — INDEPENDENT of Phases 1-3; blocked only on operator's pending router actions:
  ├─ (operator) rotate password, switch to api-ssl, fill Bitwarden UUID
  ├─ Apply ExternalSecret, verify, roll exporter
  └─ Smoke-test queries
     ↓
Phase 5 — Tempo instrumentation MVP (§5) — INDEPENDENT of all of the above:
  ├─ Add env vars to khemeia-controller deploy, roll, verify trace search
  └─ (optional MVP+) Add OTEL deps + init to surf-api, roll, verify
     ↓
Phase 6 — defaultRules curation (§8) — depends on Phase 1 (so the chart-level disables are in place first):
  ├─ Add 2-3 silence/inhibit rules for known-expected firings (longhorn DS, etc.)
  └─ Document each silence's reason in chart values comments
     ↓
Phase 7 — README (`rke2/monitor/README.md`):
  └─ Write after everything else is stable. Documents what was just built.
```

Phases 0, 4, 5 are independent and can run in parallel. Phases 1→2→3 are sequential
(same release, same component, sequential cancel/recreate). Phase 6 should follow Phase 1.
Phase 7 last.

---

## 10. Open technical questions for the operator

Each requires an answer before or during execution:

- **Q1. Filesystem PVC retention post-S3 cutover.** Recommend keeping `storage-loki-0` indefinitely (cheap, low risk). Operator: agree, or schedule deletion (e.g. T+90 days)? (§3.8)
- **Q2. etcd metrics.** Keep silenced (lose etcd observability) or schedule a follow-up to wire RKE2 etcd properly (different port + cert mTLS)? Affects ability to detect etcd disk-fragmentation issues. (§2.3 / §2.4)
- **Q3. Grafana-native vs Alertmanager-native rules.** The 5 hand-curated rules in `grafana-alert-rules-platform.yaml` are Grafana-native (delivered via sidecar). The Alertmanager email receiver only fires for Prometheus alerts. **Will the operator route Grafana-native rules to email separately (via Grafana contact points), migrate them to PrometheusRule CRs, or accept that those 5 rules don't email?** (§2.4)
- **Q4. SMTP provider for Alertmanager.** Gmail SMTP needs an app-password and isn't ideal for transactional alerts. Operator preference: Gmail, SendGrid, Mailgun, AWS SES, self-hosted (Postfix on a VM)? Affects the Bitwarden item shape and `smtp_smarthost`. (§7)
- **Q5. snmp-exporter Synology.** Currently broken (HTTP 400). Fix in this cleanup, or delete the exporter and accept loss of Synology disk/temp metrics? (§2.3)
- **Q6. nixos-gpu cilium-agent down.** Out of scope per strict reading of the PRD, but the firing alert pollutes the inbox. Fix here as a one-liner, or silence with a TTL until [[project_gpu_node]] is properly addressed? (§2.3)
- **Q7. Service-map for Tempo.** The `serviceMap` panel in Grafana requires Tempo's metrics-generator + Prometheus remote-write. Enable now (small extra config, more Prometheus storage) or defer? (§5.6)
- **Q8. Tempo retention.** No retention is set; PVC will fill eventually. Accept defer until alarm fires, or set a 7-day TTL now? (§5.5)
- **Q9. Migrate cs2-surf / khemeia dashboards to a "Projects" folder?** Currently they're at root alongside chart defaults. Add `grafana_folder` annotation to organize? (§2.2)

### 10.1 Operator Decisions (2026-05-23)

All 9 questions answered. @project-manager and downstream agents should treat
these as committed direction.

| # | Topic | Decision |
|---|---|---|
| Q1 | Loki filesystem PVC post-S3 cutover | **Keep `storage-loki-0` indefinitely.** No deletion scheduled. |
| Q2 | RKE2 etcd metrics | **Fix in this cleanup.** Wire RKE2 etcd properly (custom port + cert mTLS). Adds one issue to scope. |
| Q3 | Grafana-native vs Alertmanager rules | **Migrate all 5 rules in `grafana-alert-rules-platform.yaml` to PrometheusRule CRs.** Single routing/silence path. |
| Q4 | SMTP provider for Alertmanager | **Gmail with app-password.** Bitwarden item shape: `smtp-alertmanager` with username (Gmail address) + password (app-password). `smtp_smarthost: smtp.gmail.com:587`, TLS required. |
| Q5 | snmp-exporter-synology (broken, HTTP 400) | **Fix it.** Likely v2c community string or MIB module mismatch. Restores NAS disk/temp/SMART monitoring. |
| Q6 | cilium-agent on nixos-gpu (down) | **Fix here if one-liner; otherwise silence with TTL** tied to the `project_gpu_node` workstream. Decision-at-execution: investigate first, fall back to silence. |
| Q7 | Tempo metrics-generator (service-map) | **Enable now.** Add `metrics_generator` config + Prometheus remote-write target. Powers Grafana service-map and TraceQL dependency view. |
| Q8 | Tempo retention TTL | **7 days.** Set `compactor.compaction.block_retention: 168h` from day one. |
| Q9 | Dashboard folder organization | **Yes — TDD owns the folder taxonomy.** Proposed structure: see §10.2 below. |

### 10.2 Dashboard folder structure (per Q9)

Apply via the `grafana_dashboard_folder` annotation on each ConfigMap (sidecar
maps annotation → Grafana folder). Proposed taxonomy:

- **`Defaults`** — all 24 kube-prometheus-stack chart dashboards (cluster, nodes, namespace, pods, workloads, etcd, scheduler, etc.). Read-only by convention; if you need to modify one, copy it to `Platform` first.
- **`Platform`** — operator-curated infrastructure dashboards: `grafana-self-perf`, `kubetest-power`, Loki self-monitoring, Tempo, Alloy. Hand-edited acceptable.
- **`Projects`** — application-specific dashboards: `cs2-surf` (player/map/RTV panels), `khemeia` (docking jobs, GPU utilization), future per-project dashboards. Owners: the project, not the platform.
- **`Adhoc`** — exploratory/work-in-progress dashboards that haven't earned a home yet. Periodic prune (e.g. anything untouched >90d) part of the platform monthly review.

@project-manager: this is one issue ("apply folder annotations to all dashboard ConfigMaps") and a documentation update to the platform README.

---

## 11. Handoff to @project-manager

Proposed Docket issue shape (one parent per phase, child issues for the discrete steps).
Final decomposition is @project-manager's call.

**Parent: "Observability cleanup (PRD/TDD)"** — links docs/prd + docs/tdd.

1. **Phase 0: Commit & deduplicate monitor manifests** (S)
   - Commit untracked: synology-exporter/, kubetest-power-dashboard.yaml, grafana-self-perf-dashboard.yaml, node-exporter-values.yaml.
   - Delete `rke2/monitor/mikrotik/` directory (superseded).
   - Fold node-exporter RAPL values into kube-prometheus-stack-values.yaml.

2. **Phase 1: kube-prometheus-stack drift reconciliation** (M)
   - Disable broken chart-default SMs (kubeProxy/Scheduler/ControllerManager/Etcd).
   - Disable corresponding default rules.
   - Bitwarden item for SMTP creds + ExternalSecret manifest.
   - Add Alertmanager email receiver config to chart values.
   - helm upgrade + test-fire verification.

3. **Phase 2: Loki S3 migration** (L)
   - Bitwarden item check, ExternalSecret apply, MinIO bucket prep.
   - Backup `storage-loki-0` PVC via Longhorn.
   - helm upgrade with corrected schema + monitoring + S3 schema-boundary.
   - Delete `loki/servicemonitor.yaml` in same commit.
   - Pre-cutover read + post-cutover write smoke tests.

4. **Phase 3: loki-gateway removal** (M)
   - Repoint alloy + Grafana to `loki:3100`.
   - Disable gateway via chart values.
   - Verify canary 502s stop firing.

5. **Phase 4: Mikrotik exporter recovery** (S, blocked on operator pending actions)
   - Apply external-secret-mktxp.yaml, verify, restart exporter.
   - Delete `rke2/monitor/mikrotik/` (or done in Phase 0).
   - Commit external-secret-mktxp.yaml with real UUID.

6. **Phase 5a: Tempo MVP — khemeia traces** (XS)
   - Add OTEL env vars to api-deployment.yaml; rollout; verify trace search in Grafana.

7. **Phase 5b: Tempo — surf-api instrumentation** (S)
   - Add OTLP deps + FastAPI middleware to surf-web/api.
   - Build + deploy via existing GHA pipeline.
   - Verify in Grafana.

8. **Phase 5c (optional, follow-up TDD): Khemeia worker fan-out** (M-L)
   - Instrument 8 Flask docking containers; recipe from Phase 5b applies. Defer.

9. **Phase 6: defaultRules curation** (S)
   - 2-3 silence entries for longhorn DS firings, with documented reasons.
   - Investigate + clean up old failed Jobs in game-server (real signal).
   - Investigate KubeVersionMismatch (likely nixos-gpu).

10. **Phase 7: `rke2/monitor/README.md`** (S)
    - Documents: what each chart provides, what Alloy scrapes, where logs/metrics/traces live, how to add a ServiceMonitor, how to add a dashboard (`grafana_dashboard: "1"` label), how alerts route, how silences work.
    - Owned by @technical-writer if available, otherwise @devops-engineer.

---

## 12. Risks & open issues (consolidated)

- **R1 — Loki S3 cutover failure.** Mitigated by Option A schema-boundary (no destructive ops) + Longhorn snapshot pre-flight + tested rollback. **Residual risk: low.**
- **R2 — Repointing Alloy/Grafana off loki-gateway.** Mitigated by per-step `helm diff` + sequential upgrade with smoke test between each. **Residual risk: low.**
- **R3 — SMTP creds wrong.** test-fire procedure catches it before any real alert depends on email. **Residual risk: trivial.**
- **R4 — Disabling defaultRules silences a future real signal.** The four we disable (kube-proxy/scheduler/controller-manager/etcd) are RKE2 architectural, not transient; the four firing alerts (longhorn) are silenced narrowly by matcher, not blanket. **Residual risk: low.**
- **R5 — node-exporter restart on RAPL fold-in.** Daemonset rollout disrupts node-exporter metrics for ~30s/node. Schedule during a quiet window. **Residual risk: trivial.**
- **R6 — Operator decisions still pending.** Q1–Q9 above. Most are deferable; Q3 (Grafana-native rules) and Q4 (SMTP provider) affect Phase 1 and should be answered before that phase begins.

# Homelab TODO

Long-running follow-ups for the cluster that don't fit in any single
project's scope. Newest items at the top.

## Observability

### Loki — broad log aggregation ✅ RESOLVED 2026-06-03

**Status**: DONE. Root cause was **not** a namespace allowlist — the
config used `loki.source.kubernetes`, which tails logs through the
Kubernetes **API** and saturated the API client rate limiter cluster-wide
(`client rate limiter Wait ... context canceled`), silently dropping
almost every stream (hence ~2 pod label values). Two further gaps: Alloy
had **no tolerations** so it only ran on the 2 untainted workers, and
there was no retention policy.

**Fix** (`rke2/monitor/loki/`): switched Alloy to **node-local file
tailing** of `/var/log/pods` (`local.file_match` + `loki.source.file` +
`stage.cri`, filtered to each node via `NODENAME`), mounted host
`/var/log`, added `tolerations: operator Exists` (now on all 6 nodes),
and added 14d retention (`compactor.retention_enabled` +
`reject_old_samples_max_age=336h`). Chunks live in Garage S3, so no PVC
sizing concern. **Verified: 19 namespaces / ~450 pods now ingested
(was ~2).**

**Follow-up**: on **nixos-gpu**, the alloy pod's kubelet readiness probe
(`:12345/-/ready`) times out, so the pod shows NotReady and the DaemonSet
rollout stays parked — even though `/-/ready` answers fine from a peer pod
and logs ship normally. This is a host(kubelet)→pod reachability quirk
specific to nixos-gpu (same symptom as the storage-exporter probe on that
node); it needs the nixos-gpu CNI/networking fix, not an Alloy change.

**Related**: `docs/HOMELAB-TODO.md#tempo--distributed-tracing`. The gap was
surfaced by the 2026-04-14 cs2-surf RCON chat-spam incident (incident doc
since removed).

---

### Tempo — distributed tracing

**Status**: Tempo deployed at `rke2/monitor/tempo/` with a Grafana
datasource hooked up. No application is currently emitting traces, and
no instrumentation libraries are deployed.

**Goal**: at least the chem stack (khemeia-controller, result-writer,
plus the Go API in `chem/khemeia/api/`) emits OpenTelemetry traces to
Tempo, so end-to-end request flow through the docking pipeline is
visible in Grafana's Trace view alongside the Loki logs and Prometheus
metrics for the same time window.

**Sub-tasks**:
- Stand up an OTLP collector (or use Alloy's built-in OTLP receiver
  since Alloy is already in the cluster for Loki).
- Add OpenTelemetry SDK to the Go API. Trace HTTP handlers, MySQL calls,
  GCS uploads, RCON / k8s-job dispatches.
- Add OTel to result-writer (Python) and the Python prep/parse jobs.
- Configure Tempo retention + storage backend.
- Wire Grafana TraceQL queries into the existing chem dashboards;
  enable trace-to-logs and trace-to-metrics drill-down using the
  Loki + Prometheus datasources.

**Why it matters**: docking jobs span ~5 services (controller → MySQL
→ k8s job dispatch → vina pod → result-writer → MySQL again). Right
now debugging a slow or failed run means stitching together logs from
multiple pods by hand. Tempo would collapse that into a single
waterfall view.

**Dependencies**: Loki broad-scrape (above) should land first or in
parallel — trace-to-logs drill-down only works if the pod's logs are
actually in Loki.

---

## Other open items

### chem databases → consolidate onto a Postgres container

**Status**: `chem/chembl-mysql` has been in `Init:CrashLoopBackOff` for
days (~1900+ restarts) — its init container looks for a DB dump that
isn't present ("ERROR: No dump file found"). It is hand-applied (not in
git). Left running for now rather than deleted.

**Goal**: migrate ChEMBL (and the other chem datastores currently on
ad-hoc MySQL) onto a single managed Postgres container, then fix the
load/restore path from there. Removes the crashlooping MySQL orphan and
gives the chem stack one coherent database backend.

**Why it matters**: the current MySQL pod is dead weight (perpetual
crashloop, no data) and untracked, so it's both noise and drift.

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

**Follow-up — UNRESOLVED: nixos-gpu host→pod datapath is broken.**
On nixos-gpu only, the node's host netns **cannot reach its own local pods**
(pod CIDR `10.42.5.0/24`): ICMP + TCP, any payload size, 100% dropped. This
kills every kubelet httpGet readiness/liveness probe for pod-network pods on
the node (alloy `1/2`, storage-exporter `0/1`) and cilium's own
`cilium-health-ep`. Logs still ship (egress works), so this is *only* the
readiness flag + the parked DaemonSet rollout — not a data outage. Affects
any pod-network pod here with an httpGet probe; hostNetwork pods
(node-exporter) and probe-less pods (dcgm) are unaffected.

Investigated 2026-06-03, **ruled out**: rp_filter (0 on cilium ifaces),
MTU (uniform 1500), host firewall (INPUT/FORWARD policy accept), routing
(`ip route get` to a local pod correctly returns `dev cilium_host`), kernel
(6.12.74, modern), BPF program load (attaches via tcx, no errors). Key clue:
host→`cilium_host` (10.42.5.35) **works**, but host→any pod endpoint
(10.42.5.x) **fails** — traffic dies between `cilium_host` and the pod `lxc`
veths. Cross-node pod→pod works fine (remote→nixos-gpu pods OK).

**Tried, did NOT fix**: (a) removing the bogus `enp15s0` `/8` address —
that *is* a real separate bug (the `10.0.0.0/8` link route swallows the
service CIDR `10.43.0.0/16`, so host→Service escapes to the LAN; and it
ARP-FAILs cluster IPs on the NIC) but it is NOT the probe cause, and the
`/8` turns out to be **load-bearing for pod external egress** (removing it
live broke the geoip init-container download), so it can't just be dropped
without also fixing pod egress/masquerade. (b) `enable-host-legacy-routing`
per-node via CiliumNodeConfig + agent restart + endpoint rebuild — host→pod
still 100% dropped under Legacy host routing too.

**Next ideas** (needs deeper session): inspect the `cilium_host`→`lxc` BPF
redirect with `cilium-dbg monitor --type drop` while pinging a local pod;
check `bpf-lb-sock`/socketLB host-ns behavior; try `routingMode: tunnel`
(vxlan) for this node vs native; or compare a `cilium-dbg bpf endpoint list`
/ `cilium-dbg map get cilium_lxc` against a healthy node. The multi-homed
flat-`10.0.0.0/8` LAN (home 10.0.x + k8s 10.41.x sharing one L2, colliding
with pod 10.42 / svc 10.43) is the likely underlying culprit and probably
needs the host re-addressed onto non-overlapping prefixes
(`enp15s0` → `/24` + explicit home route) *together with* a pod-egress
masquerade fix, not piecemeal.

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

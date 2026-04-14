# Homelab TODO

Long-running follow-ups for the cluster that don't fit in any single
project's scope. Newest items at the top.

## Observability

### Loki — broad log aggregation

**Status**: Loki + Alloy already deployed at `rke2/monitor/loki/`. Alloy
is currently only scraping a small slice of namespaces — verified during
the 2026-04-14 cs2-surf RCON spam incident: querying Loki for
`{pod=~"cs2-surf-server.*"}` returned zero streams, and the entire
`pod` label only had ~2 distinct values cluster-wide. Whatever Alloy is
configured to scrape, it is not the `game-server`, `chem`, or other
application namespaces.

**Goal**: every container log in the cluster is shipped to Loki with
useful labels (`namespace`, `pod`, `container`, `app`) and queryable from
Grafana for at least 14 days. After-the-fact forensics on any incident
should be possible without needing to have already shelled into the pod.

**Blocking**:
- `rke2/monitor/loki/alloy-values.yaml` likely has a restrictive
  `loki.source.kubernetes` discovery selector or namespace allowlist.
  Audit and broaden.
- Loki retention period needs to be sized against expected log volume
  for the broader scrape (currently sized for the small set).
- Storage backend / chunks-cache PVC sizing should be reviewed before
  flipping the firehose on.

**Why it matters**: the cs2-surf incident showed we have zero post-hoc
forensic capability for any pod outside the currently-scraped set. When
something breaks, "check Loki" should be the first move; today it isn't
because the data isn't there.

**Related**: `docs/HOMELAB-TODO.md#tempo--distributed-tracing`,
`rke2/game-server/cs2/cs2-surf/INCIDENT-2026-04-14-rcon-spam.md` (the
incident that surfaced this gap).

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

(none yet — add freely)

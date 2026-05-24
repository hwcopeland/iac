# `monitor` namespace — observability stack

Single source of truth for what the cluster scrapes, how it stores it, and
how the operator extends it. Living doc; update when components change.

Last touched: 2026-05-23 (observability cleanup project — see
`docs/prd/observability-cleanup.md` + `docs/tdd/observability-cleanup.md`).

## Components

| Component | Chart / mode | Purpose |
|---|---|---|
| **kube-prometheus-stack** | helm, `kube-prometheus-stack-values.yaml` | Prometheus + Grafana + Alertmanager + node-exporter + kube-state-metrics |
| **Loki** | helm `loki-6.51.0`, SingleBinary | Log aggregation; chunks → Garage S3 |
| **Alloy** | helm, `loki/alloy-values.yaml` | Log collection DaemonSet — pod-log scrape → Loki, geoIP enrichment |
| **Tempo** | raw manifests `tempo/` | Trace storage; metrics-generator emits span_metrics + service_graphs |
| **Garage** (out of ns) | `rke2/garage-system/` | S3-compatible object store; Loki chunks live here, bucket `loki-chunks` |
| **snmp-exporter-synology** | raw manifests `synology-exporter/` | NAS metrics over SNMPv2c. Requires SNMP enabled in DSM |
| **mktxp-exporter** | raw manifests `mikrotik-exporter/` | RouterOS metrics from CCR-2004. cAP via CAPsMAN on the CCR |

## Data paths

```
                   pods/containers
                         │  stdout/stderr → kubelet logs on disk
                         ▼
                       Alloy  (DaemonSet, all nodes)
                         │  loki.write → loki:3100/loki/api/v1/push
                         ▼
                       Loki  (SingleBinary)
                         │  chunks  → Garage S3 (loki-chunks)
                         │  WAL + tsdb-shipper cache → Longhorn PVC (storage-loki-0, 20Gi)
                         ▼
                      Grafana  (Loki datasource → loki:3100)


   exporters (node-exporter, mktxp, snmp-synology, app-level)
                         │  /metrics endpoints
                         ▼
                      Prometheus  (kube-prometheus-stack)
                         │  remote-write receiver also accepts from Tempo
                         ▼
                      Grafana  (Prometheus datasource)


   instrumented apps (khemeia-controller, surf-api ...)
                         │  OTLP gRPC :4317 / HTTP :4318
                         ▼
                       Tempo  (SingleBinary, local PVC)
                         │  metrics-generator → remote_write back into Prometheus
                         │     traces_spanmetrics_* / traces_service_graph_*
                         ▼
                      Grafana  (Tempo datasource → service-map / TraceQL)
```

## Alert routing

- Alertmanager config lives in `kube-prometheus-stack-values.yaml` under
  `alertmanager.config`.
- SMTP credentials: Bitwarden item `SMTP` (UUID
  `a1ba9084-f041-4da6-aa90-ae98428b7cbb`) → ExternalSecret
  `alertmanager-smtp-secret.yaml` → Secret `alertmanager-smtp` → mounted
  at `/etc/alertmanager/secrets/alertmanager-smtp/`.
- Default route: `null` (silence-by-default).
- `severity=critical` → `email-operator` → `hampton888@gmail.com`.
- Known-noise routes (longhorn DaemonSet drift, KubeVersionMismatch on
  nixos-gpu) explicitly null-routed.

## Adding things

### New scrape target (a Pod or Service in any namespace)

Create a `ServiceMonitor` or `PodMonitor` with label
`release: kube-prometheus-stack`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-app
  namespace: my-ns
  labels:
    release: kube-prometheus-stack   # required for kps Prometheus to pick it up
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: my-app
  endpoints:
    - port: http-metrics
      interval: 30s
```

### New dashboard

ConfigMap with label `grafana_dashboard: "1"` in ANY namespace (sidecar
searches all namespaces). Add annotation `grafana_folder: <name>` to place
in a folder (see "Folder taxonomy" below); without the annotation it lands
in `Defaults`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-dashboard
  namespace: my-ns
  labels:
    grafana_dashboard: "1"
  annotations:
    grafana_folder: Projects
data:
  my-dashboard.json: |
    { ... }
```

### New alert rule

`PrometheusRule` CR with label `release: kube-prometheus-stack`. Example
in `platform-prometheus-rules.yaml`. Email delivery requires
`severity: critical` (warnings are tracked but not emailed by default).

### New OTLP trace producer

Set in the workload's env:

```yaml
env:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "tempo.monitor.svc.cluster.local:4317"   # gRPC
    # OR for HTTP:
    # value: "http://tempo.monitor.svc.cluster.local:4318"
    # - name: OTEL_EXPORTER_OTLP_PROTOCOL
    #   value: "http/protobuf"
  - name: OTEL_SERVICE_NAME
    value: "my-app"
  - name: OTEL_TRACES_SAMPLER
    value: "parentbased_traceidratio"
  - name: OTEL_TRACES_SAMPLER_ARG
    value: "0.1"   # 10% sampling; tune per app.
```

## Folder taxonomy (Grafana)

| Folder | Contains | Owners |
|---|---|---|
| `General` | kube-prometheus-stack chart dashboards (~24) + any unannotated CMs. The sidecar writes annotation-less ConfigMaps here. Read-only by convention — copy to `Platform` to modify. | Helm |
| `Platform` | Operator-curated infra dashboards: `Cluster Overview`, `Tempo — Service RED`, `Loki — Log Health`, `MikroTik — Network Overview`, `Power — Cluster Overview`, `Grafana Self Performance`, `Kubetest — Homelab Power`. | hwcopeland |
| `Projects` | Application dashboards: `CS2 Surf Server Overview`. Future per-app dashboards land here. | Project owner |
| `Khemeia` | Khemeia compute-chem platform dashboards. | khemeia team |

Set via `metadata.annotations.grafana_folder: <Folder>` on each ConfigMap.
Without the annotation, the dashboard lands in `General` (the kps grafana
sidecar's default — `defaultFolderName: Defaults` becomes the *write
path* concat'd with `folder`, not a Grafana folder name. Annotating
explicitly is the only reliable way to place dashboards).

In-repo dashboard manifests live under `rke2/monitor/dashboards/` so
they're easy to find as a group:

```
rke2/monitor/dashboards/
  cluster-overview.yaml      → Platform
  tempo-service-red.yaml     → Platform
  loki-log-health.yaml       → Platform
  mikrotik-network.yaml      → Platform
  power-overview.yaml        → Platform
```

Two older dashboards (`grafana-self-perf-dashboard.yaml`,
`kubetest-power-dashboard.yaml`) still live at `rke2/monitor/` root from
their original commits; move to `dashboards/` opportunistically.

## Storage

| Component | Backend | Path / bucket | Retention |
|---|---|---|---|
| Prometheus TSDB | Longhorn PVC | `prometheus-...-0` (kps default size) | 15d (chart default) |
| Loki chunks | Garage S3 | `loki-chunks` bucket | not yet set (compactor TODO) |
| Loki WAL + cache | Longhorn PVC | `storage-loki-0` (20Gi) | n/a (transient) |
| Tempo traces | Local PVC | `tempo-storage-0` | 7d (`compactor.compaction.block_retention: 168h`) |
| Alertmanager | Longhorn PVC | `alertmanager-...-0` (chart default) | n/a (state) |

## Helm release reconciliation

Each release in `monitor` has its values file checked into this directory.
Workflow:

```bash
# Diff before applying — install helm-diff plugin if needed:
helm diff upgrade <release> <chart> -f <values.yaml> -n monitor --version <pinned>

# Apply:
helm upgrade <release> <chart> -f <values.yaml> -n monitor --version <pinned>
```

Pinned chart versions (keep these in sync with reality):

| Release | Chart | Version |
|---|---|---|
| `kube-prometheus-stack` | `prometheus-community/kube-prometheus-stack` | `82.14.1` |
| `loki` | `grafana/loki` | `6.51.0` |
| `alloy` | `grafana/alloy` | (latest tracked in alloy-values.yaml) |

## Known operational gotchas

- **NixOS GPU node** has its own RKE2 version (`v1.33.7` vs cluster's `v1.34.3`). Causes `KubeVersionMismatch` — null-routed per Phase 6 of obs cleanup.
- **longhorn DaemonSets** show `numberMisscheduled=3` because longhorn-manager / longhorn-csi-plugin / engine-image-ei-* pods are running on ctlpln1/2/3 from before the DS dropped control-plane tolerations. Tracked in GH issue #52.
- **Synology SNMP** needs to be enabled in DSM (Control Panel → Terminal & SNMP) for `snmp-exporter-synology` to scrape. Until then it shows `TargetDown`.
- **cAP wireless AP** at 10.0.0.254 is CAPsMAN-managed by CCR-2004 — observed indirectly via CAPsMAN metrics on the CCR. Don't scrape it directly.
- **RKE2 etcd metrics** are bound to localhost on each ctlpln node by default. To expose: add `etcd-expose-metrics: true` to `/etc/rancher/rke2/config.yaml` on each server + rolling restart. Currently disabled (GH issue #31, pending operator decision).

## Cross-references

- PRD: `docs/prd/observability-cleanup.md`
- TDD: `docs/tdd/observability-cleanup.md`
- Operator decisions (Q1–Q9): TDD §10.1
- Garage (object store): `rke2/garage-system/`
- Khemeia (Tempo MVP consumer): `rke2/chem/khemeia/`

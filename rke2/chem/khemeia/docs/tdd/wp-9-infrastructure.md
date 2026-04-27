---
project: "khemeia"
maturity: "draft"
last_updated: "2026-04-19"
updated_by: "@staff-engineer"
scope: "Cross-cutting infrastructure: Garage object store, provenance system, CRD framework, job controller, compute classes, advance API, BLOB migration"
owner: "@staff-engineer"
dependencies:
  - ../spec/wp-9-infrastructure.md
---

# WP-9: Cross-Cutting Infrastructure -- Technical Design Document

## 1. Problem Statement

The Khemeia SBDD platform has a working docking prototype with MySQL BLOB storage, ad-hoc K8s
Job creation, no provenance tracking, and no compute class differentiation. Every downstream
work package (WP-1 through WP-8) is blocked on shared infrastructure that does not yet exist:
an S3-compatible object store for binary artifacts, a provenance system for lineage tracking,
a CRD-based job framework with retry logic, GPU-aware compute scheduling, and a stage-advance
API that lets researchers drive the pipeline forward.

### Why Now

WP-9 is the critical path for all other work packages. Nothing advances past prototype status
without provenance, object storage, and a structured job framework. The MySQL BLOB approach
cannot scale -- receptor PDBQT, docked poses, MD trajectories, and generated compounds must
move to object storage before any new stage writes binary data.

### Constraints

- Single RKE2 cluster, `chem` namespace, Flux GitOps, Kustomize (no Helm)
- Existing plugin system (`plugins/*.yaml`, `plugin.go`) must continue working
- Single MySQL instance (`docking-mysql`) -- no second database
- Single GPU node (`nixos-gpu`, RTX 3070, NixOS 25.11) with non-FHS driver paths
- Self-hosted registry at `zot.hwcopeland.net` (infra images under `infra/`, app under `chem/`)
- Images pulled via `zot-pull-secret`, secrets via ExternalSecret + Bitwarden

### Acceptance Criteria

See Section 11 for per-phase criteria. The top-level acceptance criteria from the spec are:

1. Given a compound through library prep and docking, `/api/v1/provenance/{id}/ancestors`
   returns the full chain including LibraryPrep job, source SDF, and TargetPrep job.
2. No orphan artifacts (Garage objects without provenance) or orphan provenance records.
3. All seven Garage buckets exist and are accessible from pods in `chem`.
4. `khemeia-scratch` auto-deletes after 7 days; `khemeia-trajectories` after 90.
5. Auto-gate CRD children transition to `Running` within 30s.
6. Manual-gate CRDs stay `Pending` until advance is called.
7. Failed jobs retry up to `maxRetries`, then `Failed` with event description.
8. Advance with invalid artifact IDs returns 400; valid call creates child CRD with provenance.
9. GPU compute class produces NixOS host-path mounts, toleration, and `LD_LIBRARY_PATH`.
10. CUDA smoke test runs on `nixos-gpu`, sees RTX 3070 via `nvidia-smi`.

---

## 2. Context and Prior Art

### Current State

| Component | Location | State |
|-----------|----------|-------|
| Job runner | `api/job_runner.go` | Creates K8s Jobs directly from Go structs. Poll-based completion. Orphan reconciliation on startup. |
| Parallel docking | `api/parallel_docking.go` | 3-phase fan-out (prep, parallel prep, parallel dock). Uses `runParallelWorkers` pattern. |
| Plugin system | `api/plugin.go`, `deploy/plugins-configmap.yaml` | YAML-driven: image, command, resources, input/output schema. Generates DB tables, API routes. |
| BLOB storage | `docking_workflows.receptor_pdbqt`, `docking_results.docked_pdbqt`, `job_artifacts.content` | `MEDIUMBLOB`/`LONGBLOB` columns in MySQL. |
| RBAC | `deploy/rbac.yaml` | Role: `batch/jobs`, `pods/log`, `configmaps`, `services` in `chem`. No CRD permissions. |
| Auth | `api/auth.go` | OIDC JWT + API tokens + internal CIDR bypass. |
| Deployment | `deploy/api-deployment.yaml` | Single replica, `khemeia-controller` SA, MySQL env from ExternalSecret. |
| MySQL | `deploy/mysql.yaml` | Single instance, 10Gi Longhorn PVC, ClusterIP+LB service. |

### How Others Solve This

- **Nextflow / Snakemake**: Workflow DAG engines with provenance built into the execution
  model. Over-featured for our manual-gate model; we only need artifact-level lineage, not
  full DAG replay.
- **MLflow / DVC**: Experiment tracking with artifact stores and run lineage. Relevant pattern
  for provenance + S3, but these are experiment-tracking tools, not job orchestrators.
- **Argo Workflows**: CRD-based workflow engine for Kubernetes. Correct abstraction layer
  but introduces a heavyweight dependency (Argo server, artifact repository, executor).
  Our controller is simpler because we do not need DAG scheduling -- we have `advance`.
- **Garage** (deuxfleurs.fr): Lightweight, S3-compatible, self-hosted object store designed
  for small clusters. Used in production by Deuxfleurs' own services. Single-binary,
  SQLite-backed metadata, Rust. Significantly simpler to operate than MinIO post-AGPL.

---

## 3. Alternatives Considered

### Object Store

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| **Garage** | Small footprint, S3-compatible, bucket lifecycle policies, MIT license, single binary | Newer project, smaller community than MinIO | **Selected** -- lifecycle policies (scratch/trajectories) are first-class; no CronJob sweeper |
| MinIO | Mature, wide adoption | AGPL license, feature-stripped community edition, hostile self-host trajectory | Rejected -- license |
| Longhorn PVC + CronJob | No new component | No lifecycle policies, no S3 API, sweeper CronJob is fragile, no presigned URLs | Rejected -- operational complexity |

### Provenance Store

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| **MySQL + recursive CTEs** | No new component, existing infra, MySQL 8.0 supports `WITH RECURSIVE` | Graph queries are verbose, performance on deep trees unknown | **Selected** -- simplicity, adequate for hundreds of artifacts per pipeline run |
| Separate graph DB (Neo4j, etc.) | Native graph traversal | New component to operate, overkill for our scale | Rejected |
| SQLite with recursive CTEs | Fast traversal, embeddable | Separate file, no shared access from multiple pods | Rejected |

### CRD vs. Extended Plugin System

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| **CRDs coexisting with plugins** | Kubernetes-native status subresource, watches, standard tooling | CRD registration complexity, RBAC extension needed | **Selected** per operator decision |
| Migrate everything to CRDs | Single abstraction | Breaking change to working docking plugin, unnecessary churn | Rejected |
| Extend plugin YAML only | No CRDs needed | No status subresource, no kubectl integration, ConfigMap size limits | Rejected |

---

## 4. Architecture and System Design

### Component Diagram

```
+----------------------------+         +------------------------+
|   SvelteKit Frontend       |         |   Authentik OIDC       |
|   (web/)                   |-------->|   (auth.hwcopeland.net)|
+----------------------------+         +------------------------+
            |
            | HTTPS (HTTPRoute)
            v
+-------------------------------------------+
|   Khemeia API Server  (api/main.go)       |
|                                           |
|   +------------------+  +--------------+  |
|   | Plugin Handlers  |  | CRD Handlers |  |   <-- NEW: advance, status, provenance endpoints
|   | (existing)       |  | (new)        |  |
|   +------------------+  +--------------+  |
|          |                     |          |
|   +------+---------+   +------+-------+  |
|   | job_runner.go  |   | crd_runner.go|  |   <-- NEW: CRD-based job controller
|   | (existing)     |   | (new)        |  |
|   +------+---------+   +------+-------+  |
|          |                     |          |
|   +------+---------------------+-------+  |
|   |         S3 Client (s3/client.go)   |  |   <-- NEW: thin AWS SDK v2 wrapper
|   +------+-----+----------------------+  |
|          |     |                          |
+-------------------------------------------+
           |     |
   +-------+     +----------+
   |                        |
   v                        v
+------------+    +------------------+
|  MySQL     |    |  Garage          |
|  (existing)|    |  (new StatefulSet|
|            |    |   in chem ns)    |
| - docking  |    |                  |
| - provenance|   | Buckets:         |
| - qe, etc. |   | - receptors      |
+------------+    | - libraries      |
                  | - poses          |
                  | - trajectories   |
                  | - reports        |
                  | - panels         |
                  | - scratch        |
                  +------------------+

           K8s API
              |
    +---------+---------+
    |                   |
    v                   v
+--------+       +-----------+
| K8s    |       | Khemeia   |
| Jobs   |       | CRDs      |    <-- NEW: TargetPrep, DockJob, etc.
| (batch)|       | (new CRDs)|
+--------+       +-----------+
```

### Integration Points

1. **API Server <-> MySQL**: Provenance queries, existing plugin job tables, CRD metadata.
   The API server manages the provenance table via Go code (same pattern as existing
   `initPluginDB`).
2. **API Server <-> Garage**: Via S3 client library. All binary artifact read/write goes
   through the `s3` Go package. Job containers also get Garage credentials as env vars to
   write artifacts directly.
3. **API Server <-> K8s API**: CRD watches (informer), Job creation from CRD specs, status
   updates on CRD instances.
4. **CRD Controller <-> Compute Classes**: ConfigMap lookup to resolve class name to
   node selectors, tolerations, resources, and volume mounts.

---

## 5. Garage Deployment

### StatefulSet

```yaml
# deploy/infra/garage.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: garage
  namespace: chem
  labels:
    app.kubernetes.io/name: garage
spec:
  serviceName: garage
  replicas: 1           # single-node layout for homelab
  selector:
    matchLabels:
      app.kubernetes.io/name: garage
  template:
    metadata:
      labels:
        app.kubernetes.io/name: garage
    spec:
      containers:
        - name: garage
          image: zot.hwcopeland.net/infra/garage:v1.1.0
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 3900    # S3 API
              name: s3
            - containerPort: 3902    # Admin API
              name: admin
            - containerPort: 3901    # RPC (inter-node, unused for single-node but required)
              name: rpc
          env:
            - name: GARAGE_RPC_SECRET
              valueFrom:
                secretKeyRef:
                  name: garage-secret
                  key: rpc-secret
          volumeMounts:
            - name: data
              mountPath: /var/lib/garage/data
            - name: meta
              mountPath: /var/lib/garage/meta
            - name: config
              mountPath: /etc/garage.toml
              subPath: garage.toml
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
          readinessProbe:
            httpGet:
              path: /health
              port: 3903       # Web UI / health port
            initialDelaySeconds: 10
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /health
              port: 3903
            initialDelaySeconds: 30
            periodSeconds: 30
      imagePullSecrets:
        - name: zot-pull-secret
      volumes:
        - name: config
          configMap:
            name: garage-config
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: longhorn
        resources:
          requests:
            storage: 200Gi
    - metadata:
        name: meta
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: longhorn
        resources:
          requests:
            storage: 1Gi
```

### Configuration

```toml
# deploy/infra/garage-config.toml (mounted as ConfigMap)
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"

replication_factor = 1  # single-node

[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"
root_domain = ".s3.garage.local"

[s3_web]
bind_addr = "[::]:3903"
root_domain = ".web.garage.local"

[admin]
api_bind_addr = "[::]:3902"

[rpc]
bind_addr = "[::]:3901"
```

### ExternalSecret for Credentials

```yaml
# deploy/infra/garage-secret.yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: garage-secret
  namespace: chem
spec:
  refreshInterval: "1h"
  secretStoreRef:
    kind: ClusterSecretStore
    name: bitwarden-login
  target:
    name: garage-secret
    creationPolicy: Owner
  data:
    - secretKey: rpc-secret
      remoteRef:
        key: <bitwarden-item-id-for-garage-rpc>
        property: password
    - secretKey: access-key-id
      remoteRef:
        key: <bitwarden-item-id-for-garage-s3>
        property: username
    - secretKey: secret-access-key
      remoteRef:
        key: <bitwarden-item-id-for-garage-s3>
        property: password
```

### Bucket Creation

Buckets are created via a one-shot K8s Job that runs after Garage is healthy. This job uses
the Garage admin API (`garage bucket create`) rather than the S3 API because bucket lifecycle
policies are a Garage admin operation.

```yaml
# deploy/infra/garage-init-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: garage-bucket-init
  namespace: chem
spec:
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: init
          image: zot.hwcopeland.net/infra/garage:v1.1.0
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -e
              ADMIN="http://garage.chem.svc.cluster.local:3902"

              # Wait for Garage to be ready
              until curl -sf "$ADMIN/health"; do sleep 2; done

              # Configure cluster layout (single node)
              NODE_ID=$(garage -c /etc/garage.toml node id 2>/dev/null | head -1 | awk '{print $1}')
              garage -c /etc/garage.toml layout assign "$NODE_ID" -z dc1 -c 1G 2>/dev/null || true
              garage -c /etc/garage.toml layout apply --version 1 2>/dev/null || true

              # Create buckets
              for BUCKET in khemeia-receptors khemeia-libraries khemeia-poses \
                            khemeia-trajectories khemeia-reports khemeia-panels \
                            khemeia-scratch; do
                garage -c /etc/garage.toml bucket create "$BUCKET" 2>/dev/null || true
              done

              # Create API key and grant access
              garage -c /etc/garage.toml key create khemeia-app 2>/dev/null || true
              KEY_ID=$(garage -c /etc/garage.toml key info khemeia-app 2>/dev/null | grep "Key ID" | awk '{print $3}')
              for BUCKET in khemeia-receptors khemeia-libraries khemeia-poses \
                            khemeia-trajectories khemeia-reports khemeia-panels \
                            khemeia-scratch; do
                garage -c /etc/garage.toml bucket allow --read --write --owner "$BUCKET" --key khemeia-app 2>/dev/null || true
              done

              echo "Garage initialization complete"
          env:
            - name: GARAGE_RPC_SECRET
              valueFrom:
                secretKeyRef:
                  name: garage-secret
                  key: rpc-secret
          volumeMounts:
            - name: config
              mountPath: /etc/garage.toml
              subPath: garage.toml
      volumes:
        - name: config
          configMap:
            name: garage-config
```

**Lifecycle policies** for `khemeia-scratch` (7-day) and `khemeia-trajectories` (90-day):
Garage does not natively support S3 lifecycle policies. Implement as a CronJob that runs
`garage bucket list` and deletes objects older than the threshold. This is a known trade-off
vs. MinIO's native lifecycle support, accepted because the sweeper CronJob is simpler than
operating MinIO.

```yaml
# deploy/infra/garage-lifecycle-cronjob.yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: garage-lifecycle
  namespace: chem
spec:
  schedule: "0 3 * * *"   # daily at 3am
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: sweep
              image: zot.hwcopeland.net/infra/garage:v1.1.0
              command: ["/bin/sh", "-c"]
              args:
                - |
                  # AWS CLI-based sweep using Garage S3 endpoint
                  # Delete objects older than 7 days in scratch
                  aws --endpoint-url http://garage.chem.svc.cluster.local:3900 \
                    s3 ls s3://khemeia-scratch/ --recursive | \
                    awk -v cutoff=$(date -d '-7 days' +%Y-%m-%d) '$1 < cutoff {print $4}' | \
                    xargs -I{} aws --endpoint-url http://garage.chem.svc.cluster.local:3900 \
                      s3 rm "s3://khemeia-scratch/{}"

                  # Delete objects older than 90 days in trajectories
                  aws --endpoint-url http://garage.chem.svc.cluster.local:3900 \
                    s3 ls s3://khemeia-trajectories/ --recursive | \
                    awk -v cutoff=$(date -d '-90 days' +%Y-%m-%d) '$1 < cutoff {print $4}' | \
                    xargs -I{} aws --endpoint-url http://garage.chem.svc.cluster.local:3900 \
                      s3 rm "s3://khemeia-trajectories/{}"
              env:
                - name: AWS_ACCESS_KEY_ID
                  valueFrom:
                    secretKeyRef:
                      name: garage-secret
                      key: access-key-id
                - name: AWS_SECRET_ACCESS_KEY
                  valueFrom:
                    secretKeyRef:
                      name: garage-secret
                      key: secret-access-key
                - name: AWS_DEFAULT_REGION
                  value: "garage"
```

### Kustomization

```yaml
# deploy/infra/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - garage-secret.yaml
  - garage-config.yaml
  - garage.yaml
  - garage-init-job.yaml
  - garage-lifecycle-cronjob.yaml
```

The top-level `deploy/kustomization.yaml` adds `- infra/` to its resources list.

### Service

```yaml
# Part of deploy/infra/garage.yaml
apiVersion: v1
kind: Service
metadata:
  name: garage
  namespace: chem
spec:
  selector:
    app.kubernetes.io/name: garage
  ports:
    - port: 3900
      targetPort: 3900
      name: s3
    - port: 3902
      targetPort: 3902
      name: admin
  type: ClusterIP
```

Internal endpoint: `garage.chem.svc.cluster.local:3900` (S3), `:3902` (admin). No external
exposure needed -- only the API server and job pods access Garage.

---

## 6. Provenance Schema

### MySQL Table Design

```sql
CREATE TABLE provenance (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    artifact_id     CHAR(36) NOT NULL,              -- UUID v7 (time-ordered)
    artifact_type   ENUM(
        'receptor', 'library', 'compound', 'docked_pose', 'refined_pose',
        'admet_result', 'generated_compound', 'linked_compound', 'report',
        'selectivity_matrix', 'fep_result'
    ) NOT NULL,
    s3_bucket       VARCHAR(64) NULL,               -- Garage bucket name
    s3_key          VARCHAR(512) NULL,              -- Garage object key
    checksum_sha256 CHAR(64) NULL,                  -- hex-encoded SHA-256 of artifact bytes
    created_by_job  VARCHAR(255) NOT NULL,          -- K8s Job name or CRD instance name
    job_kind        VARCHAR(64) NULL,               -- CRD kind (e.g., "DockJob") or "plugin"
    job_namespace   VARCHAR(64) NOT NULL DEFAULT 'chem',
    parameters      JSON NULL,                      -- job parameters snapshot
    tool_versions   JSON NULL,                      -- {"rdkit": "2024.03.6", ...}
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uq_artifact_id (artifact_id),
    INDEX idx_artifact_type (artifact_type),
    INDEX idx_created_by_job (created_by_job),
    INDEX idx_created_at (created_at),
    INDEX idx_s3_key (s3_bucket, s3_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE provenance_edges (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    parent_id       CHAR(36) NOT NULL,              -- artifact_id of the input/parent
    child_id        CHAR(36) NOT NULL,              -- artifact_id of the output/child

    UNIQUE KEY uq_edge (parent_id, child_id),
    INDEX idx_parent (parent_id),
    INDEX idx_child (child_id),
    CONSTRAINT fk_parent FOREIGN KEY (parent_id) REFERENCES provenance(artifact_id),
    CONSTRAINT fk_child FOREIGN KEY (child_id) REFERENCES provenance(artifact_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**Design rationale**: Two-table design (nodes + edges) instead of a single table with a
`parent_artifacts JSON` array. Normalized edges enable efficient recursive CTE traversal
via indexed joins. The `provenance_edges` table is a standard adjacency list for a DAG.

### Ancestor Traversal (upstream: "what produced this?")

```sql
WITH RECURSIVE ancestors AS (
    -- Base case: the artifact itself
    SELECT p.artifact_id, p.artifact_type, p.created_by_job, p.s3_bucket, p.s3_key,
           p.created_at, 0 AS depth
    FROM provenance p
    WHERE p.artifact_id = ?

    UNION ALL

    -- Recursive case: follow edges upstream
    SELECT p.artifact_id, p.artifact_type, p.created_by_job, p.s3_bucket, p.s3_key,
           p.created_at, a.depth + 1
    FROM ancestors a
    JOIN provenance_edges e ON e.child_id = a.artifact_id
    JOIN provenance p ON p.artifact_id = e.parent_id
    WHERE a.depth < 50   -- safety limit
)
SELECT * FROM ancestors ORDER BY depth ASC;
```

### Descendant Traversal (downstream: "what consumed this?")

```sql
WITH RECURSIVE descendants AS (
    SELECT p.artifact_id, p.artifact_type, p.created_by_job, p.s3_bucket, p.s3_key,
           p.created_at, 0 AS depth
    FROM provenance p
    WHERE p.artifact_id = ?

    UNION ALL

    SELECT p.artifact_id, p.artifact_type, p.created_by_job, p.s3_bucket, p.s3_key,
           p.created_at, d.depth + 1
    FROM descendants d
    JOIN provenance_edges e ON e.parent_id = d.artifact_id
    JOIN provenance p ON p.artifact_id = e.child_id
    WHERE d.depth < 50
)
SELECT * FROM descendants ORDER BY depth ASC;
```

### Performance Notes

At our scale (hundreds of artifacts per pipeline run, tree depth under 10), these recursive
CTEs perform well. The indexes on `provenance_edges(parent_id)` and
`provenance_edges(child_id)` cover both traversal directions. If we ever reach thousands of
artifacts with deep trees, the depth limit (50) protects against runaway queries, and we can
add materialized path columns (`/root-id/parent-id/child-id/...`) as an optimization without
schema changes.

---

## 7. S3 Client Library

### Package: `api/s3/client.go`

New Go package under `api/s3/` (or inline in `api/` since the project is a single Go module).
Thin wrapper over the AWS SDK v2 S3 client.

### Interface

```go
package s3

import (
    "context"
    "io"
    "time"
)

// Client provides S3 operations against Garage.
type Client interface {
    // PutArtifact uploads a binary artifact and returns the S3 key.
    PutArtifact(ctx context.Context, bucket, key string, reader io.Reader, contentType string) error

    // GetArtifact returns a ReadCloser for the artifact content.
    GetArtifact(ctx context.Context, bucket, key string) (io.ReadCloser, error)

    // GetPresignedURL returns a time-limited download URL.
    GetPresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)

    // ListArtifacts lists objects under a prefix.
    ListArtifacts(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error)

    // DeleteArtifact removes an object.
    DeleteArtifact(ctx context.Context, bucket, key string) error

    // HeadArtifact checks if an object exists and returns metadata.
    HeadArtifact(ctx context.Context, bucket, key string) (*ObjectInfo, error)
}

// ObjectInfo represents S3 object metadata.
type ObjectInfo struct {
    Key          string
    Size         int64
    LastModified time.Time
    ContentType  string
    ETag         string
}
```

### Implementation

```go
// GarageClient implements Client using AWS SDK v2.
type GarageClient struct {
    s3Client  *s3.Client
    presigner *s3.PresignClient
}

// NewGarageClient creates a client configured for the Garage endpoint.
func NewGarageClient(endpoint, region, accessKey, secretKey string) (*GarageClient, error) {
    // Configure AWS SDK v2 with custom endpoint resolver for Garage
    // endpoint = "http://garage.chem.svc.cluster.local:3900"
    // region = "garage"
    // Use path-style addressing (Garage requirement)
    ...
}
```

### Feature Flag for MySQL Fallback

During the BLOB migration (Phase 1), the S3 client operates in dual-read mode controlled
by an environment variable:

```
GARAGE_ENABLED=true|false    (default: false until migration completes)
```

When `GARAGE_ENABLED=false`:
- `PutArtifact` writes to MySQL BLOB columns (existing behavior).
- `GetArtifact` reads from MySQL BLOB columns.

When `GARAGE_ENABLED=true`:
- `PutArtifact` writes to Garage.
- `GetArtifact` tries Garage first; falls back to MySQL BLOB on `NoSuchKey` error (for
  pre-migration data that has not been moved yet). This dual-read allows gradual migration.

After the BLOB migration script runs and all existing data is in Garage, set
`GARAGE_ENABLED=true` and remove the fallback path in a follow-up cleanup.

### Key Naming Convention

```
{bucket}/{job-kind}/{job-name}/{artifact-name}.{ext}

Examples:
  khemeia-receptors/TargetPrep/targetprep-1714500000/4HHB.pdbqt
  khemeia-poses/DockJob/dockjob-1714500000/CHEMBL12345-pose1.pdbqt
  khemeia-scratch/DockJob/dockjob-1714500000/temp-grid.map
```

### Error Handling

- Network errors: Retry with exponential backoff (3 attempts, 1s/2s/4s).
- `NoSuchKey`: Return specific `ErrNotFound` sentinel for callers to distinguish
  "not found" from "Garage is down".
- `BucketNotFound`: Fatal -- log and return error. Buckets should exist from init.
- Context cancellation: Propagate immediately.

### New Dependency

Add `github.com/aws/aws-sdk-go-v2` and `github.com/aws/aws-sdk-go-v2/service/s3` to
`go.mod`. These are MIT-licensed and widely used. The SDK is the standard approach for any
S3-compatible store.

---

## 8. CRD Definitions

CRDs are defined as YAML manifests in `deploy/crds/`. Two representative CRDs are shown
below. All share a common set of status fields; the `spec` varies per stage.

### Common Base (expressed as Go types for documentation; actual implementation is CRD YAML)

```go
// CommonStatus is embedded in every Khemeia CRD's status subresource.
type CommonStatus struct {
    Phase          string       `json:"phase"`           // Pending, Running, Succeeded, Failed
    StartTime      *metav1.Time `json:"startTime,omitempty"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    RetryCount     int32        `json:"retryCount"`
    Conditions     []Condition  `json:"conditions,omitempty"`
    Events         []Event      `json:"events,omitempty"`
    ProvenanceRef  string       `json:"provenanceRef,omitempty"` // artifact_id of this job's output
}

// CommonSpec fields present in every CRD spec.
type CommonSpec struct {
    Gate         string `json:"gate"`         // "auto" or "manual", default "manual"
    Timeout      string `json:"timeout"`      // e.g., "2h"
    RetryPolicy  string `json:"retryPolicy"`  // "never", "on-failure", "always"
    MaxRetries   int32  `json:"maxRetries"`   // default 3
    ComputeClass string `json:"computeClass"` // "cpu", "cpu-high-mem", "gpu"
    ParentJob    *ParentRef `json:"parentJob,omitempty"`
}

type ParentRef struct {
    Kind               string   `json:"kind"`
    Name               string   `json:"name"`
    SelectedArtifactIDs []string `json:"selectedArtifactIds"`
}
```

### TargetPrep CRD

```yaml
# deploy/crds/targetprep-crd.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: targetpreps.khemeia.io
spec:
  group: khemeia.io
  names:
    kind: TargetPrep
    listKind: TargetPrepList
    plural: targetpreps
    singular: targetprep
    shortNames:
      - tp
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required: ["pdbId"]
              properties:
                pdbId:
                  type: string
                  description: "PDB ID of the target protein"
                nativeLigand:
                  type: string
                  default: "TTT"
                  description: "Residue name of native ligand for binding site detection"
                engine:
                  type: string
                  default: "adfr"
                  enum: ["adfr", "fpocket"]
                  description: "Binding site detection engine"
                # -- Common fields --
                gate:
                  type: string
                  default: "manual"
                  enum: ["auto", "manual"]
                timeout:
                  type: string
                  default: "1h"
                retryPolicy:
                  type: string
                  default: "on-failure"
                  enum: ["never", "on-failure", "always"]
                maxRetries:
                  type: integer
                  default: 3
                computeClass:
                  type: string
                  default: "cpu"
                parentJob:
                  type: object
                  properties:
                    kind:
                      type: string
                    name:
                      type: string
                    selectedArtifactIds:
                      type: array
                      items:
                        type: string
            status:
              type: object
              properties:
                phase:
                  type: string
                  default: "Pending"
                startTime:
                  type: string
                  format: date-time
                completionTime:
                  type: string
                  format: date-time
                retryCount:
                  type: integer
                  default: 0
                provenanceRef:
                  type: string
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      type:
                        type: string
                      status:
                        type: string
                      reason:
                        type: string
                      message:
                        type: string
                      lastTransitionTime:
                        type: string
                        format: date-time
                events:
                  type: array
                  items:
                    type: object
                    properties:
                      timestamp:
                        type: string
                        format: date-time
                      message:
                        type: string
      subresources:
        status: {}
      additionalPrinterColumns:
        - name: Phase
          type: string
          jsonPath: .status.phase
        - name: PDB
          type: string
          jsonPath: .spec.pdbId
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
```

### DockJob CRD

```yaml
# deploy/crds/dockjob-crd.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: dockjobs.khemeia.io
spec:
  group: khemeia.io
  names:
    kind: DockJob
    listKind: DockJobList
    plural: dockjobs
    singular: dockjob
    shortNames:
      - dj
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required: ["receptorArtifactId", "libraryArtifactId"]
              properties:
                receptorArtifactId:
                  type: string
                  description: "Provenance artifact_id of the prepared receptor"
                libraryArtifactId:
                  type: string
                  description: "Provenance artifact_id of the prepared library"
                engine:
                  type: string
                  default: "vina"
                  enum: ["vina", "smina", "gnina", "diffdock"]
                exhaustiveness:
                  type: integer
                  default: 8
                chunkSize:
                  type: integer
                  default: 10000
                # -- Common fields (same as TargetPrep) --
                gate:
                  type: string
                  default: "manual"
                  enum: ["auto", "manual"]
                timeout:
                  type: string
                  default: "168h"
                retryPolicy:
                  type: string
                  default: "on-failure"
                  enum: ["never", "on-failure", "always"]
                maxRetries:
                  type: integer
                  default: 3
                computeClass:
                  type: string
                  default: "gpu"
                parentJob:
                  type: object
                  properties:
                    kind:
                      type: string
                    name:
                      type: string
                    selectedArtifactIds:
                      type: array
                      items:
                        type: string
            status:
              type: object
              properties:
                phase:
                  type: string
                  default: "Pending"
                startTime:
                  type: string
                  format: date-time
                completionTime:
                  type: string
                  format: date-time
                retryCount:
                  type: integer
                  default: 0
                provenanceRef:
                  type: string
                workerCount:
                  type: integer
                  description: "Number of parallel docking workers launched"
                resultCount:
                  type: integer
                  description: "Number of docking results stored"
                bestAffinity:
                  type: number
                  description: "Best (lowest) binding affinity in kcal/mol"
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      type: { type: string }
                      status: { type: string }
                      reason: { type: string }
                      message: { type: string }
                      lastTransitionTime: { type: string, format: date-time }
                events:
                  type: array
                  items:
                    type: object
                    properties:
                      timestamp: { type: string, format: date-time }
                      message: { type: string }
      subresources:
        status: {}
      additionalPrinterColumns:
        - name: Phase
          type: string
          jsonPath: .status.phase
        - name: Engine
          type: string
          jsonPath: .spec.engine
        - name: Results
          type: integer
          jsonPath: .status.resultCount
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
```

### CRD Registration

CRDs are applied before any controller that references them. The Flux kustomization for
`deploy/crds/` must have `dependsOn` on nothing (it is a root resource). The main
`deploy/kustomization.yaml` adds `- crds/` to its resources.

### RBAC Extension

The existing `khemeia-controller` Role needs additional rules:

```yaml
# Append to deploy/rbac.yaml
  - apiGroups: ["khemeia.io"]
    resources: ["targetpreps", "dockjobs", "refinejobs", "admetjobs",
                "generatejobs", "linkjobs", "selectivityjobs", "rbfejobs",
                "abfejobs", "librarypreps", "ingeststructurejobs", "reportjobs"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["khemeia.io"]
    resources: ["targetpreps/status", "dockjobs/status", "refinejobs/status",
                "admetjobs/status", "generatejobs/status", "linkjobs/status",
                "selectivityjobs/status", "rbfejobs/status", "abfejobs/status",
                "librarypreps/status", "ingeststructurejobs/status", "reportjobs/status"]
    verbs: ["get", "update", "patch"]
```

**Note**: This uses a namespace-scoped Role (not ClusterRole) since CRDs are cluster-scoped
resources but instances are namespaced. The Role grants access to CRD instances in `chem`.
CRD definitions themselves are applied by Flux with cluster-admin privileges.

---

## 9. Job Controller

### Design: Extension of `job_runner.go`

The CRD job controller is a new file `api/crd_controller.go` that runs alongside the existing
`job_runner.go`. It does NOT replace the existing plugin job runner. Both coexist:

- **Existing path**: `PluginSubmit` -> `RunPluginJob` / `RunParallelDockingJob` (unchanged)
- **New path**: CRD created (by `advance` API or kubectl) -> CRD controller watches ->
  creates K8s Job -> monitors -> updates CRD status

### Watch/Informer Pattern

```go
// CRDController watches Khemeia CRD instances and drives their lifecycle.
type CRDController struct {
    dynamicClient dynamic.Interface
    jobClient     typed.JobInterface
    s3Client      s3.Client
    db            *sql.DB          // for provenance
    namespace     string
    computeClasses map[string]ComputeClass
    stopCh        chan struct{}
}

// Start begins watching all registered CRD types.
func (c *CRDController) Start(ctx context.Context) error {
    // For each CRD kind, set up a dynamic informer:
    for _, gvr := range registeredCRDs {
        informer := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
            c.dynamicClient, 30*time.Second, c.namespace, nil,
        ).ForResource(gvr)

        informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
            AddFunc:    c.onAdd,
            UpdateFunc: c.onUpdate,
        })

        go informer.Informer().Run(ctx.Done())
    }
    return nil
}
```

### CRD Instance Lifecycle

```
CRD Created
    |
    v
[phase: Pending]
    |
    +-- gate: auto?
    |     |
    |     yes --> Create K8s Job immediately
    |     |
    |     no (manual) --> Wait for /advance API call
    |
    v
[phase: Running]  <-- status updated when K8s Job starts
    |
    +-- K8s Job succeeded?
    |     |
    |     yes --> Store artifacts in Garage
    |     |       Record provenance
    |     |       Set phase: Succeeded
    |     |
    |     no --> retryCount < maxRetries?
    |            |
    |            yes --> Increment retryCount
    |            |       Create new K8s Job
    |            |       Stay in Running
    |            |
    |            no --> Set phase: Failed
    |                   Record failure event
```

### K8s Job Creation from CRD Spec

The controller reads the CRD spec, resolves the compute class, and builds a `batchv1.Job`.
This is similar to the existing `RunPluginJob` but parameterized by CRD fields instead of
plugin YAML.

```go
func (c *CRDController) createJobForCRD(crd *unstructured.Unstructured) (*batchv1.Job, error) {
    kind := crd.GetKind()
    name := crd.GetName()
    spec := crd.Object["spec"].(map[string]interface{})

    // Resolve compute class
    className, _ := spec["computeClass"].(string)
    cc := c.computeClasses[className]

    // Build Job from CRD-kind-specific template
    job := c.buildJobTemplate(kind, name, spec, cc)

    // Add Garage credentials as env vars so job containers can write artifacts
    c.injectGarageEnv(job)

    // Add MySQL credentials for provenance recording
    c.injectMySQLEnv(job)

    return c.jobClient.Create(context.Background(), job, metav1.CreateOptions{})
}
```

### Status Updates

The controller watches K8s Job status via the existing informer pattern. When a Job
completes or fails:

```go
func (c *CRDController) updateCRDStatus(crd *unstructured.Unstructured, phase string) error {
    status := map[string]interface{}{
        "phase": phase,
    }
    if phase == "Running" {
        status["startTime"] = metav1.Now().Format(time.RFC3339)
    }
    if phase == "Succeeded" || phase == "Failed" {
        status["completionTime"] = metav1.Now().Format(time.RFC3339)
    }

    // Append event
    events := getEvents(crd)
    events = append(events, map[string]interface{}{
        "timestamp": time.Now().Format(time.RFC3339),
        "message":   fmt.Sprintf("Phase changed to %s", phase),
    })
    status["events"] = events

    // Update via dynamic client status subresource
    return c.updateStatusSubresource(crd, status)
}
```

### Retry Logic

On failure, if `retryCount < maxRetries` and `retryPolicy` is `on-failure` or `always`:

1. Increment `retryCount` in the CRD status.
2. Add an event: `"Retry {n}/{max}: recreating K8s Job after failure"`.
3. Delete the failed K8s Job (TTL would eventually clean it, but explicit deletion avoids
   confusion).
4. Create a new K8s Job with the same spec.

For the `gpu` compute class, this handles `nixos-gpu` reboots. The pod fails due to node
unavailability, the controller sees the failure, waits for the next reconciliation cycle
(30s informer resync), and recreates the job. When `nixos-gpu` comes back, the job picks
up the GPU node via its toleration.

### Dynamic Client vs. Generated Client

We use `k8s.io/client-go/dynamic` rather than code-generated typed clients. Rationale:
- Avoids running `controller-gen` and maintaining generated code for 12 CRD types.
- Dynamic client works with `unstructured.Unstructured`, which is adequate for our
  use case (read spec fields, update status).
- Trade-off: no compile-time type safety for CRD fields. Acceptable because the CRD
  schemas have validation in the OpenAPI spec, and the controller only accesses a small
  set of well-defined fields.

---

## 10. Compute Classes

### ConfigMap Format

```yaml
# deploy/compute-classes.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: khemeia-compute-classes
  namespace: chem
data:
  classes.yaml: |
    classes:
      cpu:
        description: "Standard CPU workloads"
        resources:
          cpu: "2"
          memory: "4Gi"
        nodeSelector: {}
        tolerations: []
        volumes: []
        volumeMounts: []
        env: []

      cpu-high-mem:
        description: "Memory-intensive CPU workloads"
        resources:
          cpu: "4"
          memory: "16Gi"
        nodeSelector: {}
        tolerations: []
        volumes: []
        volumeMounts: []
        env: []

      gpu:
        description: "GPU workloads on NixOS node"
        resources:
          cpu: "4"
          memory: "8Gi"
          nvidia.com/gpu: "1"
        nodeSelector:
          gpu: "rtx3070"
        tolerations:
          - key: "gpu"
            value: "true"
            effect: "NoSchedule"
        volumes:
          - name: nvidia-driver
            hostPath:
              path: /run/opengl-driver
          - name: nix-store
            hostPath:
              path: /nix/store
        volumeMounts:
          - name: nvidia-driver
            mountPath: /run/opengl-driver
            readOnly: true
          - name: nix-store
            mountPath: /nix/store
            readOnly: true
        env:
          - name: LD_LIBRARY_PATH
            value: /run/opengl-driver/lib
```

### Go Struct

```go
// ComputeClass defines the scheduling and resource configuration for a job.
type ComputeClass struct {
    Description  string                     `yaml:"description"`
    Resources    map[string]string          `yaml:"resources"`
    NodeSelector map[string]string          `yaml:"nodeSelector"`
    Tolerations  []corev1.Toleration        `yaml:"tolerations"`
    Volumes      []corev1.Volume            `yaml:"volumes"`
    VolumeMounts []corev1.VolumeMount       `yaml:"volumeMounts"`
    Env          []corev1.EnvVar            `yaml:"env"`
}

// LoadComputeClasses reads the ConfigMap and returns a map of class name -> ComputeClass.
func LoadComputeClasses(client kubernetes.Interface, namespace string) (map[string]ComputeClass, error) {
    cm, err := client.CoreV1().ConfigMaps(namespace).Get(
        context.Background(), "khemeia-compute-classes", metav1.GetOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to load compute classes ConfigMap: %w", err)
    }

    var config struct {
        Classes map[string]ComputeClass `yaml:"classes"`
    }
    if err := yaml.Unmarshal([]byte(cm.Data["classes.yaml"]), &config); err != nil {
        return nil, fmt.Errorf("failed to parse compute classes: %w", err)
    }

    return config.Classes, nil
}
```

### Job Template Generation

When the CRD controller creates a K8s Job, it merges the compute class into the pod spec:

```go
func (c *CRDController) applyComputeClass(podSpec *corev1.PodSpec, className string) error {
    cc, ok := c.computeClasses[className]
    if !ok {
        return fmt.Errorf("unknown compute class: %s", className)
    }

    container := &podSpec.Containers[0]

    // Resources
    container.Resources.Requests = parseResourceList(cc.Resources)
    container.Resources.Limits = parseResourceList(cc.Resources)

    // Node selector
    podSpec.NodeSelector = cc.NodeSelector

    // Tolerations
    podSpec.Tolerations = append(podSpec.Tolerations, cc.Tolerations...)

    // Volumes & mounts (GPU host paths, etc.)
    podSpec.Volumes = append(podSpec.Volumes, cc.Volumes...)
    container.VolumeMounts = append(container.VolumeMounts, cc.VolumeMounts...)

    // Environment (LD_LIBRARY_PATH for GPU, etc.)
    container.Env = append(container.Env, cc.Env...)

    return nil
}
```

### Default Compute Class per CRD Kind

Hardcoded in the controller, overridable in the CRD spec:

| CRD Kind | Default Class |
|----------|--------------|
| TargetPrep | cpu |
| LibraryPrep | cpu |
| DockJob | gpu |
| RefineJob | gpu |
| ADMETJob | cpu |
| GenerateJob | gpu |
| LinkJob | gpu |
| SelectivityJob | cpu-high-mem |
| RBFEJob | gpu |
| ABFEJob | gpu |
| IngestStructureJob | cpu |
| ReportJob | cpu |

---

## 11. Advance API

### Endpoint

```
POST /api/v1/jobs/{kind}/{name}/advance
```

### Request Body

```json
{
    "downstream_kind": "DockJob",
    "selected_artifact_ids": ["018f7a5c-...", "018f7a5d-..."],
    "downstream_params": {
        "engine": "gnina",
        "exhaustiveness": 32,
        "chunkSize": 5000
    }
}
```

### Response (201 Created)

```json
{
    "name": "dockjob-1714500123",
    "kind": "DockJob",
    "namespace": "chem",
    "parentJob": {
        "kind": "TargetPrep",
        "name": "targetprep-1714500000"
    },
    "provenance_action_id": "018f7a60-..."
}
```

### Handler Implementation

```go
func (h *APIHandler) HandleAdvance(w http.ResponseWriter, r *http.Request) {
    // 1. Parse path: extract {kind} and {name}
    kind, name := parseJobPath(r.URL.Path)

    // 2. Validate source CRD exists and is Succeeded
    sourceCRD, err := h.dynamicClient.Resource(gvrForKind(kind)).
        Namespace(h.namespace).Get(r.Context(), name, metav1.GetOptions{})
    if err != nil { ... }

    phase := getPhase(sourceCRD)
    if phase != "Succeeded" {
        writeError(w, "source job must be in Succeeded phase", http.StatusConflict)
        return
    }

    // 3. Validate selected artifact IDs belong to source job
    var req AdvanceRequest
    json.NewDecoder(r.Body).Decode(&req)

    if err := h.validateArtifactOwnership(name, req.SelectedArtifactIDs); err != nil {
        writeError(w, err.Error(), http.StatusBadRequest)
        return
    }

    // 4. Create downstream CRD instance
    downstreamName := fmt.Sprintf("%s-%d", strings.ToLower(req.DownstreamKind), time.Now().UnixNano())
    downstream := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "khemeia.io/v1alpha1",
            "kind":       req.DownstreamKind,
            "metadata": map[string]interface{}{
                "name":      downstreamName,
                "namespace": h.namespace,
            },
            "spec": mergeMaps(req.DownstreamParams, map[string]interface{}{
                "parentJob": map[string]interface{}{
                    "kind":                kind,
                    "name":                name,
                    "selectedArtifactIds":  req.SelectedArtifactIDs,
                },
            }),
        },
    }

    created, err := h.dynamicClient.Resource(gvrForKind(req.DownstreamKind)).
        Namespace(h.namespace).Create(r.Context(), downstream, metav1.CreateOptions{})
    if err != nil { ... }

    // 5. Record advance action in provenance
    actionID := h.recordAdvanceProvenance(name, downstreamName, req.SelectedArtifactIDs)

    // 6. Return
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "name":                  created.GetName(),
        "kind":                  req.DownstreamKind,
        "namespace":             h.namespace,
        "parentJob":             map[string]string{"kind": kind, "name": name},
        "provenance_action_id":  actionID,
    })
}
```

### Status Endpoint

```
GET /api/v1/jobs/{kind}/{name}/status
```

Returns the CRD's status subresource directly:

```json
{
    "phase": "Succeeded",
    "startTime": "2026-04-19T10:00:00Z",
    "completionTime": "2026-04-19T11:30:00Z",
    "retryCount": 0,
    "provenanceRef": "018f7a5c-...",
    "conditions": [...],
    "events": [...],
    "producedArtifactIds": ["018f7a5c-...", "018f7a5d-..."]
}
```

The `producedArtifactIds` field is computed at query time by looking up provenance records
where `created_by_job = {name}`.

### Artifact Validation

`validateArtifactOwnership` queries the provenance table:

```sql
SELECT COUNT(*) FROM provenance
WHERE artifact_id IN (?, ?, ?)
AND created_by_job = ?
```

If count does not equal the number of requested IDs, return 400 with the specific IDs that
do not belong to the source job.

### Provenance Recording

The advance action itself is recorded as a provenance edge. For each selected artifact ID,
an edge is created from that artifact to a new "advance action" provenance node:

```sql
-- Create an "advance_action" provenance record
INSERT INTO provenance (artifact_id, artifact_type, created_by_job, job_kind, parameters)
VALUES (?, 'advance_action', ?, ?, ?);

-- Create edges from selected artifacts to the advance action
INSERT INTO provenance_edges (parent_id, child_id)
VALUES (?, ?);  -- for each selected artifact -> advance action
```

The `advance_action` artifact type is not in the original spec enum but is needed to
represent the act of advancing. This is an append to the `artifact_type` ENUM. Alternatively,
advance actions can be recorded as metadata on the downstream CRD's provenance record
rather than as a separate artifact. Recommend the latter to avoid polluting the artifact
type space -- record the advance as edges directly from source artifacts to downstream
job outputs.

**Revised approach**: No separate advance_action type. When the downstream job produces its
outputs, provenance edges are created from the selected parent artifacts to each output
artifact. The `parentJob` field on the CRD serves as the advance record in the K8s layer.

---

## 12. BLOB Migration

### Current BLOBs

| Table | Column | Type | Content |
|-------|--------|------|---------|
| `docking_workflows` | `receptor_pdbqt` | MEDIUMBLOB | Prepared receptor PDBQT text |
| `docking_results` | `docked_pdbqt` | MEDIUMBLOB | Docked ligand pose PDBQT text |
| `job_artifacts` | `content` | LONGBLOB | Plugin job output files (cube, molden, etc.) |
| `ligands` | `pdbqt` | MEDIUMBLOB | Prepared ligand PDBQT (OpenBabel output) |
| `pseudopotentials` | `content` | MEDIUMBLOB | UPF pseudopotential files |

### Migration Strategy: Big-Bang Before WP-1

A one-shot migration script runs as a K8s Job after Garage is deployed and buckets exist.
It:

1. Queries each BLOB-containing table for rows with non-null BLOB content.
2. Uploads each BLOB to the appropriate Garage bucket.
3. Records a provenance entry for each migrated artifact.
4. Updates the MySQL row to store the S3 key (in a new nullable column) and NULLs out the
   BLOB column to reclaim space.
5. Runs in batches of 100 to avoid memory pressure.

### Migration Script (Go, runs as a K8s Job)

```go
// cmd/blob-migration/main.go

func migrateReceptors(db *sql.DB, s3Client s3.Client) error {
    rows, err := db.Query(`
        SELECT name, pdbid, receptor_pdbqt
        FROM docking_workflows
        WHERE receptor_pdbqt IS NOT NULL
        AND s3_receptor_key IS NULL
    `)
    // For each row:
    //   1. Upload to khemeia-receptors/{pdbid}/{name}.pdbqt
    //   2. Record provenance
    //   3. UPDATE docking_workflows SET s3_receptor_key = ?, receptor_pdbqt = NULL WHERE name = ?
    ...
}

func migrateDockingResults(db *sql.DB, s3Client s3.Client) error {
    // Similar pattern for docking_results.docked_pdbqt
    // Upload to khemeia-poses/DockJob/{workflow_name}/{compound_id}.pdbqt
    ...
}

func migrateJobArtifacts(db *sql.DB, s3Client s3.Client) error {
    // job_artifacts.content -> khemeia-scratch or appropriate bucket based on content_type
    // Upload to khemeia-reports/{plugin}/{job_name}/{filename}
    ...
}
```

### Schema Changes

Add nullable S3 key columns to existing tables:

```sql
ALTER TABLE docking_workflows ADD COLUMN s3_receptor_key VARCHAR(512) NULL;
ALTER TABLE docking_results ADD COLUMN s3_pose_key VARCHAR(512) NULL;
ALTER TABLE job_artifacts ADD COLUMN s3_key VARCHAR(512) NULL;
ALTER TABLE ligands ADD COLUMN s3_pdbqt_key VARCHAR(512) NULL;
ALTER TABLE pseudopotentials ADD COLUMN s3_key VARCHAR(512) NULL;
```

### Read Path During Migration

The API handlers (`handlers_generic.go`, `handlers.go`) must check `s3_*_key` first:
- If `s3_*_key IS NOT NULL`: fetch from Garage.
- If `s3_*_key IS NULL`: read from the BLOB column (pre-migration data).

This dual-read is encapsulated in the S3 client's fallback mode (Section 7).

### Post-Migration Cleanup

After confirming all BLOBs have been migrated (no rows where `s3_*_key IS NULL AND
blob_column IS NOT NULL`):

1. Set `GARAGE_ENABLED=true` permanently.
2. Remove the MySQL fallback code path from the S3 client.
3. Optionally: `ALTER TABLE ... DROP COLUMN receptor_pdbqt` etc. to reclaim space. This
   is a breaking change if any code still references those columns, so defer until all
   handlers are updated.

---

## 13. Phased Implementation Plan

### Phase 1: Garage Deployment + S3 Client (S, ~3 days)

**Goal**: Garage running in `chem` namespace, S3 client library operational, all buckets
created.

**Deliverables**:
1. `deploy/infra/` directory with Garage StatefulSet, ConfigMap, Service, ExternalSecret.
2. Bucket init Job.
3. Lifecycle CronJob for scratch/trajectory expiry.
4. `api/s3/` package with `GarageClient` implementation.
5. Feature flag (`GARAGE_ENABLED`) for MySQL fallback.
6. Update `deploy/kustomization.yaml` to include `infra/`.
7. Mirror `duxnodes/garage:v1.1.0` to `zot.hwcopeland.net/infra/garage:v1.1.0`.

**Acceptance criteria**:
- Garage pod is Running with 200Gi PV attached.
- All 7 buckets exist (verify via `garage bucket list`).
- Write a test object to `khemeia-scratch`, read it back, verify content matches.
- S3 client unit tests pass (against Garage instance or test mock).

**Dependencies**: None. This is the first phase.

### Phase 2: Provenance Schema + API (S, ~2 days)

**Goal**: Provenance tables created, API endpoints operational, recursive traversal working.

**Deliverables**:
1. `provenance` and `provenance_edges` tables created by `initPluginDB` on startup (added
   to the shared database initialization path).
2. API endpoints:
   - `GET /api/v1/provenance/{artifactId}` -- single record
   - `GET /api/v1/provenance/{artifactId}/ancestors` -- recursive upstream
   - `GET /api/v1/provenance/{artifactId}/descendants` -- recursive downstream
   - `GET /api/v1/provenance/job/{jobName}` -- all artifacts by job
   - `POST /api/v1/provenance/record` -- create provenance entry (used by job containers)
3. Go helper functions for provenance recording (called by the S3 client on `PutArtifact`
   and by the CRD controller on job completion).

**Acceptance criteria**:
- Insert a test provenance chain (3 levels deep), query ancestors and descendants, verify
  correct traversal.
- `POST /api/v1/provenance/record` creates a valid provenance entry.
- `GET /api/v1/provenance/{id}` returns the correct record.

**Dependencies**: Phase 1 (S3 client, for `PutArtifact` provenance integration).

### Phase 3: CRD Framework + Job Controller (M, ~5 days)

**Goal**: CRD definitions registered, job controller watching CRD instances, compute classes
operational, K8s Jobs created from CRD specs.

**Deliverables**:
1. `deploy/crds/` directory with CRD YAML for TargetPrep and DockJob (minimum). Other CRDs
   added as their WPs begin.
2. `deploy/compute-classes.yaml` ConfigMap.
3. `api/crd_controller.go` with dynamic informer, Job creation, status updates, retry logic.
4. Compute class resolution (ConfigMap -> pod spec).
5. GPU class with NixOS host-path mounts, toleration, `LD_LIBRARY_PATH`.
6. Extended RBAC in `deploy/rbac.yaml`.
7. Controller startup in `main.go`: load compute classes, start CRD controller alongside
   existing plugin system.

**Acceptance criteria**:
- `kubectl apply -f` a TargetPrep CRD instance with `gate: auto` -> controller creates a
  K8s Job within 30s -> Job runs -> CRD status transitions `Pending -> Running -> Succeeded`.
- A DockJob with `gate: manual` stays `Pending` until advance is called (Phase 4).
- A DockJob with `computeClass: gpu` produces a K8s Job with `nvidia.com/gpu: 1`,
  `gpu=true:NoSchedule` toleration, `/run/opengl-driver` and `/nix/store` host-path mounts,
  and `LD_LIBRARY_PATH=/run/opengl-driver/lib`.
- A Job that fails is retried up to `maxRetries`. After `maxRetries`, CRD phase is `Failed`
  with an event.

**Dependencies**: Phase 2 (provenance recording in job completion path).

### Phase 4: Advance API + Status Endpoint (S, ~2 days)

**Goal**: Researchers can advance from one stage to the next via the API, with provenance
tracking and artifact validation.

**Deliverables**:
1. `POST /api/v1/jobs/{kind}/{name}/advance` handler.
2. `GET /api/v1/jobs/{kind}/{name}/status` handler.
3. Artifact validation (selected IDs must belong to source job).
4. Provenance edge creation on advance.
5. Route registration in `main.go`.

**Acceptance criteria**:
- Create a TargetPrep CRD, mark it Succeeded (manually or via a test job), call advance
  with valid artifact IDs and `downstream_kind: DockJob` -> DockJob CRD created with
  `parentJob` referencing TargetPrep.
- Calling advance with artifact IDs that don't belong to the source returns 400.
- The provenance graph shows edges from TargetPrep outputs to DockJob (after DockJob runs
  and produces outputs).
- `GET /status` returns phase, conditions, events, and produced artifact IDs.

**Dependencies**: Phase 3 (CRD framework for creating downstream CRD instances).

### Phase 5: BLOB Migration (S, ~2 days)

**Goal**: All existing MySQL BLOBs moved to Garage, S3 keys stored in MySQL, `GARAGE_ENABLED`
set to `true`.

**Deliverables**:
1. Schema migration: add `s3_*_key` columns to existing tables.
2. Migration script (`cmd/blob-migration/main.go`) packaged as a Docker image.
3. K8s Job manifest for running the migration.
4. Updated API handlers to read from S3 key first, fall back to BLOB.
5. Provenance records for all migrated artifacts.
6. Verification script that confirms no orphan BLOBs remain.

**Acceptance criteria**:
- All rows in `docking_workflows`, `docking_results`, `job_artifacts`, `ligands`, and
  `pseudopotentials` that had BLOB data now have `s3_*_key` populated and BLOB columns
  NULLed.
- API responses for docking results, receptor PDBQT, and artifacts return the same content
  as before (now served from Garage).
- No orphan provenance records (every S3 object has a provenance entry).
- No orphan S3 objects (every provenance record points to a reachable S3 object).

**Dependencies**: Phase 1 (Garage), Phase 2 (provenance).

### Phase 6: Integration Testing + GPU Smoke Test (S, ~1 day)

**Goal**: End-to-end validation of the full infrastructure stack.

**Deliverables**:
1. E2E test: TargetPrep -> advance -> DockJob, verifying provenance chain.
2. GPU smoke test: CUDA workload pod via `gpu` compute class on `nixos-gpu`.
3. Lifecycle test: verify `khemeia-scratch` objects are deleted after 7 days (can use a
   backdated object + manual CronJob trigger).
4. Failure/retry test: intentionally fail a Job, verify retry and eventual `Failed` status.

**Acceptance criteria**:
- E2E: TargetPrep succeeds, advance creates DockJob, DockJob runs, provenance chain from
  DockJob output back to TargetPrep is queryable.
- GPU: `nvidia-smi` output shows RTX 3070 from inside a pod created by the `gpu` compute
  class.
- Retry: a deliberately failing job retries 3 times, then transitions to `Failed` with
  events describing each retry.

**Dependencies**: All previous phases.

---

## 14. Risks and Open Questions

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Garage instability at v1.1.0 | Low | High (data loss) | Longhorn PV provides storage durability independent of Garage process. Longhorn snapshots as backup. |
| Recursive CTE performance on deep provenance trees | Low | Medium | Depth limit (50) in queries. Monitor query time. Add materialized paths if needed. |
| NixOS GPU node reboots during long-running jobs | Medium | Medium | Retry policy with `maxRetries: 3`. Controller recreates failed jobs. No manual tainting. |
| CRD schema changes after data exists | Medium | Medium | CRD versioning (`v1alpha1`). Migration path to `v1beta1` when schema stabilizes. |
| Dynamic client type unsafety | Low | Low | CRD OpenAPI validation catches invalid specs at admission time. Controller accesses few fields. |
| Garage lifecycle CronJob vs. native lifecycle | Low | Low | Accepted trade-off. CronJob is simple, runs daily, low operational burden. |

### Open Questions (Resolved by Operator)

1. **Garage vs. PVC**: Garage selected. (Resolved)
2. **Provenance store**: MySQL with recursive CTEs. (Resolved)
3. **Object store sizing**: 200 GB initial PV. (Resolved)
4. **CRD vs. ConfigMap**: CRDs coexist with plugin system. (Resolved)
5. **Gate conditions**: Skipped entirely. Advance API works without them. (Resolved)
6. **BLOB migration timing**: Big-bang before WP-1. (Resolved)
7. **GPU node reboots**: Fail and retry via CRD retry policy. (Resolved)

### Remaining Open Questions

1. **Garage S3 key in Bitwarden**: The ExternalSecret references need Bitwarden item IDs
   for Garage RPC secret and S3 credentials. These need to be created in Bitwarden before
   deployment.
2. **Garage image mirroring**: `duxnodes/garage:v1.1.0` needs to be pulled from Docker Hub
   and pushed to `zot.hwcopeland.net/infra/garage:v1.1.0`. Manual step or CI addition.
3. **Provenance for existing docking results**: The BLOB migration (Phase 5) creates
   provenance records for migrated data. But there is no lineage information for pre-existing
   docking results (we do not know which TargetPrep produced a given receptor). These
   migrated records will have `parentJob: null` -- orphan roots in the provenance graph.
   Acceptable for historical data.

---

## 15. Testing Strategy

### Unit Tests

- **S3 client**: Mock the AWS SDK, test put/get/list/delete/presign operations, test
  fallback mode (`GARAGE_ENABLED=false`), test retry behavior on network errors.
- **Provenance**: Use a test MySQL instance, build a multi-level provenance graph, verify
  ancestor and descendant CTE queries return correct results at each depth.
- **Compute class resolution**: Given a class name, verify the output pod spec has correct
  resources, node selectors, tolerations, volume mounts, and env vars. Cover all three
  classes (`cpu`, `cpu-high-mem`, `gpu`).
- **Advance handler**: Validate artifact ownership check (mock provenance DB), test 400
  on invalid IDs, test 409 on non-Succeeded source, test happy path creates CRD.

### Integration Tests

- **Garage round-trip**: Deploy Garage (via testcontainers or real cluster), write an object,
  read it back, verify checksum, list objects by prefix, delete and verify gone.
- **CRD lifecycle**: Create a CRD instance, verify the controller creates a K8s Job, wait
  for completion, verify CRD status transitions.
- **Provenance graph end-to-end**: Create two CRDs (TargetPrep -> DockJob via advance),
  query the provenance graph from the DockJob output, verify the chain includes both jobs.

### E2E Tests

- **Full pipeline slice**: TargetPrep -> advance -> DockJob -> advance -> RefineJob
  (when WP-3 CRD exists). Verify provenance chain, all artifacts in Garage, CRD statuses.
- **GPU smoke**: Submit a job with `computeClass: gpu`, verify it runs on `nixos-gpu`,
  `nvidia-smi` output captured in logs.

---

## 16. Observability and Operational Readiness

### Health Signals

The existing `/health` and `/readyz` endpoints should be extended:

- `/readyz` should check Garage connectivity (HEAD on a known bucket) in addition to MySQL.
- CRD controller health: a Prometheus gauge `khemeia_crd_watcher_active{kind}` (1 = watching,
  0 = stopped). If any watcher dies, the readiness probe should fail.

### Key Metrics (when Prometheus scraping is configured)

- `khemeia_s3_operations_total{bucket, operation, status}` -- counter of S3 operations.
- `khemeia_provenance_queries_total{type}` -- counter of ancestor/descendant queries.
- `khemeia_crd_phase_transitions_total{kind, from_phase, to_phase}` -- CRD lifecycle events.
- `khemeia_job_retry_total{kind}` -- counter of job retries.
- `khemeia_garage_lifecycle_sweep_total{bucket, objects_deleted}` -- CronJob sweep stats.

### Alerts

- Garage pod not Ready for > 5 minutes.
- CRD stuck in `Running` for > 2x its timeout duration.
- BLOB migration verification finds orphan records.
- Lifecycle CronJob fails 3 consecutive times.

### Diagnosability at 3am

- CRD status subresource contains phase, conditions, and timestamped events. `kubectl describe`
  on any CRD instance shows the full lifecycle history.
- All S3 operations log bucket, key, and duration.
- Job controller logs include the CRD kind, name, and phase transition on every action.
- Garage admin API (`/health`, `/bucket/list`) available for debugging storage issues.

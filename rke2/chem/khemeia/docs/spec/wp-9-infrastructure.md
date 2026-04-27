# WP-9: Cross-Cutting Infrastructure

## Owner

TBD

## Scope

Build the shared infrastructure that all other work packages depend on: a provenance system,
S3 object store, common job CRD framework, event bus, compute class scheduling, and a steering
API. This WP runs in parallel from day one. It produces the foundation that every stage-specific
WP consumes.

### Current state

The existing prototype has minimal shared infrastructure:
- **MySQL** (`deploy/mysql.yaml`): single-instance MySQL for the `docking` database. Schema
  includes `docking_workflows`, `docking_results`, `ligands`, staging tables.
- **K8s Jobs**: created programmatically by `parallel_docking.go` and `job_runner.go`. No
  CRD abstraction -- jobs are built from Go structs and submitted directly.
- **RBAC**: `rbac.yaml` defines a `khemeia-controller` ServiceAccount with `batch/jobs` CRUD
  and `pods/log` read in the `chem` namespace.
- **Storage**: receptor PDBQT and ligand PDBQT stored as MySQL BLOBs. No object store.
- **No provenance**: no lineage tracking beyond the `docking_workflows.pdbid` column.
- **No event bus**: job status is polled by the API server watching K8s Job status.
- **No compute classes**: all jobs use the same resource requests. No GPU scheduling.
- **No steering**: jobs run to completion or fail. No pause, resume, or branching.

This WP replaces the ad-hoc infrastructure with a systematic, provenance-tracked, event-driven
framework.

## Deliverables

### Provenance system

1. **Provenance data model:**
   - Every artifact in the system (receptor, library, docked pose, ADMET prediction, report)
     has a provenance record
   - Provenance record fields:
     - `artifact_id`: globally unique ID (UUID v7 for time-ordering)
     - `artifact_type`: enum (receptor, library, compound, docked_pose, refined_pose,
       admet_result, generated_compound, linked_compound, report, selectivity_matrix,
       fep_result)
     - `created_by`: job name that produced this artifact
     - `created_at`: timestamp
     - `parent_artifacts`: array of artifact IDs that were inputs to the producing job
     - `job_ref`: reference to the CRD instance (kind + name + namespace)
     - `parameters`: key-value map of the job parameters that produced this artifact
     - `tool_versions`: map of tool name to version (e.g., `{"rdkit": "2024.03.6",
       "gromacs": "2024.4", "smina": "2020.12.10"}`)
     - `checksum`: SHA-256 of the artifact file (for S3-stored artifacts)
   - Storage: provenance records in MySQL (new `provenance` table) with S3 key references
     to the actual artifacts
   - Query API: given any artifact, traverse the provenance graph upstream ("what produced
     this?") and downstream ("what consumed this?")

2. **Provenance API endpoints:**
   - `GET /api/v1/provenance/{artifactId}` -- get provenance record for an artifact
   - `GET /api/v1/provenance/{artifactId}/ancestors` -- traverse upstream (all inputs,
     recursively)
   - `GET /api/v1/provenance/{artifactId}/descendants` -- traverse downstream (all
     consumers, recursively)
   - `GET /api/v1/provenance/job/{jobName}` -- get all artifacts produced by a job
   - `POST /api/v1/provenance/record` -- record a new provenance entry (used by job
     containers)

### S3 object store (MinIO)

3. **MinIO deployment:**
   - Deploy MinIO in the `chem` namespace as a StatefulSet with persistent storage
   - Bucket structure:

     | Bucket | Purpose | Lifecycle |
     |--------|---------|-----------|
     | `khemeia-receptors` | Prepared receptor PDB/PDBQT files | Permanent |
     | `khemeia-libraries` | Conformer SDF/PDBQT files, compound libraries | Permanent |
     | `khemeia-poses` | Docked and refined pose files | Permanent |
     | `khemeia-trajectories` | MD trajectory files (XTC, TRR) | 90-day retention |
     | `khemeia-reports` | Generated HTML/PDF reports | Permanent |
     | `khemeia-panels` | Selectivity panel definitions and structures | Permanent |
     | `khemeia-scratch` | Temporary job working files | 7-day retention |

   - Key naming convention: `{bucket}/{job-type}/{job-name}/{artifact-name}.{ext}`
   - Access: S3-compatible API. Credentials via ExternalSecret (consistent with existing
     `zot-pull-secret` pattern).
   - Internal endpoint: `minio.chem.svc.cluster.local:9000`

4. **S3 client library** (Go, for the API server):
   - Thin wrapper around the MinIO Go client
   - Functions: `PutArtifact(bucket, key, reader)`, `GetArtifact(bucket, key)`,
     `GetPresignedURL(bucket, key, expiry)`, `ListArtifacts(bucket, prefix)`
   - Automatic provenance recording: every `PutArtifact` call registers a provenance record
   - Configurable: feature flag to fall back to MySQL BLOB storage during migration

### Job CRD framework

5. **Common CRD base specification:**
   - All stage CRDs (TargetPrep, LibraryPrep, DockJob, RefineJob, ADMETJob, GenerateJob,
     LinkJob, SelectivityJob, RBFEJob, ABFEJob, IngestStructureJob, ReportJob) derive from
     a common base
   - Common status fields (every CRD has these):
     - `phase`: enum (Pending, Running, Succeeded, Failed, Paused)
     - `startTime`, `completionTime`
     - `provenance`: reference to the provenance record for this job's outputs
     - `retryCount`, `maxRetries` (default 3)
     - `conditions`: array of Kubernetes-style conditions (type, status, reason, message,
       lastTransitionTime)
     - `events`: array of timestamped event messages (job lifecycle events)
   - Common spec fields:
     - `gate`: enum (auto, manual). If `manual`, the job is created in `Pending` and waits
       for user approval. If `auto`, it starts immediately when dependencies are met.
     - `timeout`: duration (default per CRD type, overridable)
     - `retryPolicy`: enum (never, on-failure, always)
     - `computeClass`: reference to a compute class (see Deliverable 7)
   - CRD registration: define CRDs as YAML manifests in `deploy/crds/`. Register via
     Flux GitOps.

6. **Job controller** (Go, extends the existing `job_runner.go`):
   - Watch all Khemeia CRD instances
   - For each CRD in `Pending` with `gate: auto`: check dependency readiness, create K8s
     Jobs
   - For each CRD in `Running`: monitor K8s Job status, update CRD status, emit events
   - For each CRD in `Failed` with `retryCount < maxRetries`: recreate the K8s Job
   - Handle `Paused`: do not create new Jobs, preserve existing state
   - Emit events to the event bus (Deliverable 8) on every phase transition

### Compute classes

7. **Compute class definitions:**

   | Class | Node Selector | Resource Requests | Use Cases |
   |-------|--------------|-------------------|-----------|
   | `cpu` | No GPU required | `cpu: 2, memory: 4Gi` | Library prep, filtering, analysis, reporting |
   | `cpu-high-mem` | No GPU, high memory nodes | `cpu: 4, memory: 16Gi` | Large library processing, AiZynthFinder |
   | `gpu` | `nvidia.com/gpu` | `cpu: 4, memory: 8Gi, nvidia.com/gpu: 1` | Docking (Gnina, DiffDock), generation (ChemMamba, DiffLinker), FEP |
   | `gpu-multi` | `nvidia.com/gpu` | `cpu: 8, memory: 16Gi, nvidia.com/gpu: 2` | Large RBFE jobs, batch DiffDock |

   - Compute classes are stored as a ConfigMap (`deploy/compute-classes.yaml`)
   - Each CRD spec references a compute class by name. The job controller translates the
     class into node selectors and resource requests when creating K8s Jobs.
   - Default compute class per CRD type (e.g., DockJob defaults to `gpu`, LibraryPrep
     defaults to `cpu`). Overridable in the CRD spec.

### Event bus

8. **Event bus deployment:**
   - **Primary choice: NATS** (lightweight, K8s-native, simple pub/sub)
   - **Fallback: Redis Streams** (if NATS is not feasible; Redis is already familiar from
     other cluster services)
   - Deploy in the `chem` namespace
   - Topic naming convention: `khemeia.{crd-kind}.{job-name}.{event-type}`
     - Event types: `created`, `running`, `succeeded`, `failed`, `paused`, `resumed`,
       `progress` (for periodic progress updates during long jobs)
   - Consumers:
     - **API server**: subscribes to all events, updates CRD status, pushes WebSocket updates
       to UI (WP-7)
     - **Job controller**: subscribes to dependency completion events to trigger downstream
       jobs
     - **Provenance recorder**: subscribes to `succeeded` events to finalize provenance
       records
   - Retention: 7 days for event replay. Old events are garbage-collected.

9. **Event schema** (JSON):
   ```json
   {
     "event_id": "uuid-v7",
     "timestamp": "2026-04-19T12:00:00Z",
     "kind": "DockJob",
     "job_name": "dock-7jrn-run1",
     "event_type": "succeeded",
     "payload": {
       "phase": "Succeeded",
       "duration_seconds": 1823,
       "artifacts_produced": ["artifact-uuid-1", "artifact-uuid-2"],
       "summary": "Docked 100 compounds, best affinity -9.2 kcal/mol"
     }
   }
   ```

### Steering API

10. **Steering API endpoints:**
    - `POST /api/v1/jobs/{kind}/{name}/pause` -- pause a running job. For multi-pod jobs
      (parallel docking), pause means: do not launch new pods, let in-progress pods finish.
      Collect partial results.
    - `POST /api/v1/jobs/{kind}/{name}/resume` -- resume a paused job from where it left
      off. Relaunch pods for remaining work.
    - `POST /api/v1/jobs/{kind}/{name}/branch` -- create a new job with modified parameters,
      linked to the original via provenance. The branch starts from the original's inputs
      (not from scratch). Example: branch a DockJob with different engine parameters.
    - `POST /api/v1/jobs/{kind}/{name}/approve` -- approve a job with `gate: manual` to
      transition from `Pending` to `Running`.
    - `GET /api/v1/jobs/{kind}/{name}/status` -- unified status endpoint for any CRD kind.
    - All steering actions are recorded in provenance (who, when, what action, on which job).

11. **Gate conditions:**
    - Per-stage configurable conditions that must be met before auto-advancing to the next
      stage. Examples:
      - DockJob -> RefineJob: "at least 10 compounds with affinity < -7 kcal/mol"
      - RefineJob -> ADMETJob: "at least 5 refined poses completed"
      - ADMETJob -> GenerateJob: "at least 3 compounds with MPO score > 60"
    - Gate conditions are defined in the downstream CRD's spec as a `gateCondition` field
      (JSON expression evaluated against the upstream job's status)
    - If `gate: manual`, the condition is advisory (shown to the user in WP-7 UI) but the
      user must still manually approve

### Tests

12. **Tests:**
    - Unit tests for provenance graph traversal (build a test graph, verify ancestor and
      descendant queries)
    - Unit tests for S3 client wrapper (mock MinIO, verify put/get/list operations)
    - Unit tests for compute class resolution (given a CRD with `computeClass: gpu`, verify
      the K8s Job has `nvidia.com/gpu` resource requests)
    - Unit tests for gate condition evaluation (given a DockJob status, verify the condition
      evaluator correctly returns pass/fail)
    - Integration test: deploy MinIO, write an artifact, read it back, verify checksum
    - Integration test: create a DockJob CRD instance, verify the job controller creates
      K8s Jobs and updates CRD status
    - Integration test: publish an event to the event bus, verify a subscriber receives it
    - E2E smoke test: create a TargetPrep -> DockJob chain with `gate: auto`, verify that
      completing TargetPrep automatically triggers DockJob

## Acceptance Criteria

1. Provenance: given a compound that has been through library prep (WP-2) and docking (WP-3),
   the `GET /api/v1/provenance/{artifactId}/ancestors` endpoint returns a chain that includes
   the LibraryPrep job, the source SDF file, and the TargetPrep job.
2. Provenance: every artifact stored in S3 has a corresponding provenance record. There are
   no orphan artifacts (S3 objects without provenance) and no orphan provenance records
   (records pointing to non-existent S3 objects).
3. MinIO: all buckets listed in Deliverable 3 exist and are accessible from pods in the
   `chem` namespace. Write a test file, read it back, verify content matches.
4. MinIO: the `khemeia-scratch` bucket automatically deletes objects older than 7 days.
   The `khemeia-trajectories` bucket deletes objects older than 90 days.
5. CRD framework: a TargetPrep CRD instance created with `gate: auto` transitions to
   `Running` within 30 seconds and the controller creates the expected K8s Job.
6. CRD framework: a DockJob CRD instance created with `gate: manual` stays in `Pending`
   until `POST /api/v1/jobs/DockJob/{name}/approve` is called, then transitions to
   `Running`.
7. Retry: a Job that fails (pod exit code != 0) is retried up to `maxRetries` times. After
   `maxRetries`, the CRD status is `Failed` with an event describing the failure.
8. Event bus: a `succeeded` event published when a DockJob completes is received by a test
   subscriber within 5 seconds.
9. Steering: pausing a parallel docking job (with 5 remaining chunk pods) stops new pod
   creation. Resuming relaunches the remaining chunks. The final result includes results
   from both pre-pause and post-resume pods.
10. Steering: branching a DockJob creates a new DockJob CRD with a `branchedFrom` field in
    provenance linking to the original. The branch uses the same TargetPrep and LibraryPrep
    inputs.
11. Compute classes: a DockJob with `computeClass: gpu` produces K8s Jobs with
    `nvidia.com/gpu: 1` resource requests. A LibraryPrep with `computeClass: cpu` produces
    K8s Jobs without GPU requests.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | None | This WP has no upstream dependencies. It starts on day one. |
| Blocks | WP-1 | TargetPrep CRD, S3 storage, provenance |
| Blocks | WP-2 | LibraryPrep CRD, S3 storage, provenance |
| Blocks | WP-3 | DockJob/RefineJob CRDs, GPU compute classes, S3 storage |
| Blocks | WP-4 | ADMETJob CRD, event bus for status, S3 for results |
| Blocks | WP-5 | GenerateJob/LinkJob CRDs, GPU compute classes, provenance chain |
| Blocks | WP-6 | SelectivityJob/RBFEJob/ABFEJob CRDs, GPU compute classes |
| Blocks | WP-7 | Event bus for live status updates, provenance for the provenance browser |
| Blocks | WP-8 | IngestStructureJob/ReportJob CRDs, S3 for report storage |

## Out of Scope

- Multi-cluster scheduling (all jobs run on the single RKE2 cluster)
- Workflow engine (Argo Workflows, Tekton, etc.) -- the job controller handles sequencing
  directly via CRD watches and gate conditions
- Data lake or data warehouse (this is operational storage, not analytics)
- External API gateway (the Go API server handles all external-facing traffic)
- Secret management changes (existing ExternalSecret pattern is sufficient)
- MySQL schema migration tooling (manual migrations via SQL scripts, same as current)
- Observability (Prometheus/Grafana stack is already deployed separately on the cluster)

## Open Questions

1. **NATS vs Redis Streams**: NATS is purpose-built for messaging and has a smaller footprint.
   Redis Streams reuses existing Redis knowledge. The cluster does not currently run either
   in the `chem` namespace. Which introduces less operational burden?

2. **Provenance storage**: MySQL for provenance records keeps the stack simple (existing MySQL
   instance) but graph traversal queries (ancestor/descendant) are not MySQL's strength.
   Should provenance be stored in a graph-aware system (e.g., embedded SQLite with recursive
   CTEs, or a purpose-built graph) or is MySQL with recursive CTE queries sufficient?

3. **MinIO sizing**: How much storage is needed? Estimate: receptors (small, <1 MB each),
   libraries (10-100 MB for 100K compounds), trajectories (10-50 MB per compound), reports
   (<10 MB each). For 100 docking runs with 1000 compound refinements each: ~50-500 GB
   trajectory storage. What PV size to provision?

4. **CRD vs ConfigMap for job definitions**: The current plugin YAML system uses ConfigMaps
   (`deploy/plugins-configmap.yaml`). CRDs are more Kubernetes-native and support status
   subresources. Should the existing plugin system migrate to CRDs, or coexist?

5. **Gate condition language**: What expression language for gate conditions? Options:
   (a) CEL (Common Expression Language, used by K8s ValidatingAdmissionPolicy),
   (b) JSONPath expressions against the upstream CRD status, (c) simple key-threshold pairs
   (e.g., `{"field": "bestAffinity", "op": "lt", "value": -7}`). CEL is powerful but complex;
   simple pairs cover 90% of cases.

6. **Migration from MySQL BLOBs**: The existing `docking_workflows.receptor_pdbqt` column
   stores receptor files as BLOBs. When should migration to S3 happen? Options: (a) big-bang
   migration before WP-1 starts, (b) dual-write during a transition period (feature flag),
   (c) write-new-to-S3, read-old-from-MySQL, never migrate historical data.

## Technical Constraints

- **Namespace**: All infrastructure components deploy in the `chem` namespace. No new
  namespaces unless required for isolation (e.g., MinIO operator in `minio-operator`
  namespace).
- **Flux GitOps**: All manifests must be compatible with the existing Flux deployment model.
  CRDs go in `deploy/crds/`, controllers in `deploy/`, infrastructure (MinIO, NATS) in
  `deploy/infra/`.
- **RBAC**: The `khemeia-controller` ServiceAccount needs extended permissions for CRD CRUD,
  MinIO access, and event bus publish/subscribe. Define a new ClusterRole
  `khemeia-infrastructure` or extend the existing role.
- **Existing job_runner.go**: The new job controller extends (not replaces) the existing
  `job_runner.go`. The existing plugin-based job system must continue to work for current
  plugins. CRD-based jobs are a parallel path that eventually supersedes the plugin jobs.
- **MySQL**: The existing MySQL instance is adequate for provenance records (small rows,
  indexed queries). Do not deploy a second database for provenance.
- **No Helm**: The cluster uses Kustomize via Flux, not Helm. All manifests must be
  Kustomize-compatible.
- **Image registry**: MinIO, NATS/Redis images must be mirrored to `zot.hwcopeland.net/infra/`
  (not `chem/`, which is for application containers).

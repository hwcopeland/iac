# WP-9: Cross-Cutting Infrastructure

## Owner

TBD

## Scope

Build the shared infrastructure that all other work packages depend on: a provenance system,
object store, common job CRD framework, compute class scheduling, and a stage-advance API.
This WP runs in parallel from day one. It produces the foundation that every stage-specific
WP consumes.

The pipeline is researcher-driven, not auto-driven. Every stage defaults to `gate: manual` —
results land, the researcher reviews them in the UI (WP-7), selects a subset, and explicitly
advances to the next stage. There is no pause/resume/branch surface; "branch" is implicit in
each `advance` (selected subset becomes a child job with parent provenance). Auto-advance is
opt-in for cases where it makes sense (e.g. parallel docking shards rolling up to a single
DockJob result).

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

### Object store (Garage)

3. **Garage deployment:**
   - Deploy [Garage](https://garagehq.deuxfleurs.fr) in the `chem` namespace as a StatefulSet
     with Longhorn-backed persistent storage. Garage is a small, self-hostable, S3-compatible
     object store. It replaces MinIO, whose community edition has been stripped of features and
     whose licensing trajectory is hostile to self-hosters.
   - Why Garage over PVC-only: bucket lifecycle policies (auto-expire scratch and trajectories)
     are first-class. With raw PVCs we'd write a CronJob to sweep old files.
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
   - Internal endpoint: `garage.chem.svc.cluster.local:3900` (S3 API), `:3902` (admin)

4. **S3 client library** (Go, for the API server):
   - Thin wrapper around the AWS Go SDK v2 (Garage speaks S3, no Garage-specific client needed)
   - Functions: `PutArtifact(bucket, key, reader)`, `GetArtifact(bucket, key)`,
     `GetPresignedURL(bucket, key, expiry)`, `ListArtifacts(bucket, prefix)`
   - Automatic provenance recording: every `PutArtifact` call registers a provenance record
   - Configurable: feature flag to fall back to MySQL BLOB storage during migration

The split: queryable metadata (descriptors, scores, ADMET predictions, fingerprints,
provenance) stays in MySQL; opaque blob bytes (PDBQT, trajectories, reports) live in Garage
with the S3 key recorded in MySQL. The "generalized queryable database" property is
preserved — every searchable property is a SQL column, not an object-store object.

### Job CRD framework

5. **Common CRD base specification:**
   - All stage CRDs (TargetPrep, LibraryPrep, DockJob, RefineJob, ADMETJob, GenerateJob,
     LinkJob, SelectivityJob, RBFEJob, ABFEJob, IngestStructureJob, ReportJob) derive from
     a common base
   - Common status fields (every CRD has these):
     - `phase`: enum (Pending, Running, Succeeded, Failed)
     - `startTime`, `completionTime`
     - `provenance`: reference to the provenance record for this job's outputs
     - `parentJob`: reference to the upstream Job and the artifact subset that seeded this
       one (set by the `advance` API; null for root jobs)
     - `retryCount`, `maxRetries` (default 3)
     - `conditions`: array of Kubernetes-style conditions (type, status, reason, message,
       lastTransitionTime)
     - `events`: array of timestamped event messages (job lifecycle events)
   - Common spec fields:
     - `gate`: enum (auto, manual). **Default is `manual`.** Manual jobs are created in
       `Pending` and wait for an `advance` call from the UI. Auto is opt-in for internal
       fan-in cases (e.g. parallel docking shards rolling up to a parent DockJob).
     - `timeout`: duration (default per CRD type, overridable)
     - `retryPolicy`: enum (never, on-failure, always)
     - `computeClass`: reference to a compute class (see Deliverable 7)
   - CRD registration: define CRDs as YAML manifests in `deploy/crds/`. Register via
     Flux GitOps.

6. **Job controller** (Go, extends the existing `job_runner.go`):
   - Watch all Khemeia CRD instances
   - For each CRD in `Pending` with `gate: auto`: check dependency readiness, create K8s
     Jobs
   - For each CRD in `Pending` with `gate: manual`: leave alone. Wait for the `advance` API
     (Deliverable 10) to create the K8s Job
   - For each CRD in `Running`: monitor K8s Job status, update CRD status
   - For each CRD in `Failed` with `retryCount < maxRetries`: recreate the K8s Job
   - Status updates use the K8s informer / watch stream — no separate event bus needed
     for the v0 controller (see "Deferred" section below)

### Compute classes

7. **Compute class definitions:**

   | Class | Node Selector | Tolerations | Resource Requests | Use Cases |
   |-------|--------------|------------|-------------------|-----------|
   | `cpu` | none | none | `cpu: 2, memory: 4Gi` | Library prep, filtering, analysis, reporting |
   | `cpu-high-mem` | high-memory nodes | none | `cpu: 4, memory: 16Gi` | Large library processing, AiZynthFinder |
   | `gpu` | `gpu=rtx3070` | `gpu=true:NoSchedule` | `cpu: 4, memory: 8Gi, nvidia.com/gpu: 1` | Docking (Gnina, DiffDock), generation (ChemMamba, DiffLinker), FEP |

   - Compute classes are stored as a ConfigMap (`deploy/compute-classes.yaml`)
   - Each CRD spec references a compute class by name. The job controller translates the
     class into node selectors, tolerations, and resource requests when creating K8s Jobs.
   - Default compute class per CRD type (e.g., DockJob defaults to `gpu`, LibraryPrep
     defaults to `cpu`). Overridable in the CRD spec.

   **NixOS GPU node footnote.** The `gpu` class targets `nixos-gpu` (RTX 3070, 8 GB VRAM),
   which is a NixOS workstation joined to the cluster. NVIDIA driver libs and binaries live
   under `/run/opengl-driver` (symlinks into `/nix/store`), not the FHS `/usr/lib`. The
   k8s-device-plugin advertises `nvidia.com/gpu` and exposes `/dev/nvidia*` device files,
   but CDI auto-injection of libcuda/nvidia-smi doesn't work cleanly on NixOS. Pods built
   from the `gpu` class therefore include three additional spec elements:

   ```yaml
   volumeMounts:
     - { name: nvidia-driver, mountPath: /run/opengl-driver, readOnly: true }
     - { name: nix-store,     mountPath: /nix/store,         readOnly: true }
   volumes:
     - { name: nvidia-driver, hostPath: { path: /run/opengl-driver } }
     - { name: nix-store,     hostPath: { path: /nix/store } }
   env:
     - { name: LD_LIBRARY_PATH, value: /run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu }
     - { name: OCL_ICD_VENDORS, value: /run/opengl-driver/etc/OpenCL/vendors }
   ```

   This is baked into the job controller's GPU-class job-template so individual stage CRDs
   don't need to care. Single GPU per node — no `gpu-multi` class until more hardware
   joins.

### Stage advance API

8. **`advance` endpoint:**
   - `POST /api/v1/jobs/{kind}/{name}/advance` -- the single primitive for moving the
     pipeline forward. Body:
     ```json
     {
       "downstream_kind": "RefineJob",
       "selected_artifact_ids": ["uuid-1", "uuid-2", "..."],
       "downstream_params": { "engine": "gnina", "exhaustiveness": 32 }
     }
     ```
   - Behavior:
     1. Validate that the source job is in `Succeeded` and that the selected artifact IDs
        belong to it.
     2. Create a new CRD instance of `downstream_kind` with `parentJob` pointing at the
        source job and the selected artifact subset, plus the supplied parameters.
     3. Record the advance action in provenance (who, when, source → downstream link).
     4. Return the new CRD's name and namespace.
   - Implicit branching: every `advance` call is effectively a branch — selecting a
     different subset or different params from the same source produces a new sibling
     CRD with its own provenance lineage. There's no separate "branch" primitive.

9. **`status` endpoint:**
   - `GET /api/v1/jobs/{kind}/{name}/status` -- unified status endpoint for any CRD kind.
     Returns phase, conditions, produced artifact IDs, and gate-condition evaluation if
     applicable.

10. **Gate conditions (advisory):**
    - Per-stage `gateCondition` field on the downstream CRD's spec — a JSON expression
      evaluated against the upstream job's status. Examples:
      - DockJob → RefineJob: "at least 10 compounds with affinity < -7 kcal/mol"
      - RefineJob → ADMETJob: "at least 5 refined poses completed"
      - ADMETJob → GenerateJob: "at least 3 compounds with MPO score > 60"
    - With the default `gate: manual`, gate conditions are advisory only — they're shown
      to the researcher in the UI as "this stage is ready to advance" badges. The
      researcher decides whether to advance.
    - With `gate: auto`, the condition gates the controller's auto-creation of the
      downstream Job.

### Deferred (not in this WP)

The following pieces were considered and explicitly deferred:

- **Event bus** (NATS / Redis Streams). The v0 controller can drive UI updates via the
  K8s informer/watch stream plus a WebSocket endpoint on the API server — same effect
  with one less moving part. Revisit when there are multiple controller processes that
  need to coordinate, or when event replay for late-joining subscribers becomes useful.
- **Pause / resume / branch primitives.** Folded into `advance`. Pause/resume in
  particular has no value in a manual-gate model — if the researcher hasn't advanced the
  stage, nothing downstream is running to pause.
- **`gpu-multi` compute class.** Single-GPU node today. Defer until additional hardware
  joins.

### Tests

11. **Tests:**
    - Unit tests for provenance graph traversal (build a test graph, verify ancestor and
      descendant queries)
    - Unit tests for S3 client wrapper (against a Garage test instance via testcontainers,
      verify put/get/list operations)
    - Unit tests for compute class resolution (given a CRD with `computeClass: gpu`, verify
      the K8s Job has `nvidia.com/gpu: 1` request, the `gpu=true:NoSchedule` toleration,
      and the NixOS lib mounts)
    - Unit tests for gate condition evaluation (given a DockJob status, verify the condition
      evaluator correctly returns pass/fail)
    - Unit tests for `advance` (selected artifact IDs that don't belong to source job →
      400; valid call → child CRD created with `parentJob` link)
    - Integration test: deploy Garage, write an artifact, read it back, verify checksum
    - Integration test: create a DockJob CRD instance, verify the job controller creates
      K8s Jobs and updates CRD status
    - E2E smoke test: TargetPrep succeeds → researcher calls `advance` with selected
      artifact IDs → DockJob is created with `parentJob` referencing TargetPrep, and runs
    - E2E smoke test on real GPU: a small CUDA workload pod scheduled via the `gpu`
      compute class on `nixos-gpu` reports the RTX 3070 via `nvidia-smi`

## Acceptance Criteria

1. Provenance: given a compound that has been through library prep (WP-2) and docking (WP-3),
   the `GET /api/v1/provenance/{artifactId}/ancestors` endpoint returns a chain that includes
   the LibraryPrep job, the source SDF file, and the TargetPrep job.
2. Provenance: every artifact stored in Garage has a corresponding provenance record. There
   are no orphan artifacts (Garage objects without provenance) and no orphan provenance
   records (records pointing to non-existent Garage objects).
3. Garage: all buckets listed in Deliverable 3 exist and are accessible from pods in the
   `chem` namespace. Write a test file, read it back, verify content matches.
4. Garage: the `khemeia-scratch` bucket automatically deletes objects older than 7 days.
   The `khemeia-trajectories` bucket deletes objects older than 90 days.
5. CRD framework: a child CRD created via `advance` (with the parent in `gate: auto`)
   transitions to `Running` within 30 seconds and the controller creates the expected K8s
   Job.
6. CRD framework: a DockJob CRD instance with `gate: manual` (the default) stays in
   `Pending` until `POST /api/v1/jobs/DockJob/{name}/advance` is called from the UI; then
   the child CRD it produces transitions to `Running`.
7. Retry: a Job that fails (pod exit code != 0) is retried up to `maxRetries` times. After
   `maxRetries`, the CRD status is `Failed` with an event describing the failure.
8. Advance: calling `advance` with selected artifact IDs that don't belong to the source
   job returns 400. Valid call creates a child CRD with `parentJob` referencing the source
   and the artifact subset; the provenance graph reflects the parent → child link.
9. Compute classes: a DockJob with `computeClass: gpu` produces K8s Jobs with
   `nvidia.com/gpu: 1`, the `gpu=true:NoSchedule` toleration, the NixOS host-path mounts
   (`/run/opengl-driver`, `/nix/store`), and
   `LD_LIBRARY_PATH=/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu`,
   `OCL_ICD_VENDORS=/run/opengl-driver/etc/OpenCL/vendors`.
   A LibraryPrep with `computeClass: cpu` produces K8s Jobs without GPU requests.
10. GPU smoke: a CUDA workload submitted with `computeClass: gpu` runs on `nixos-gpu`,
    sees the RTX 3070 via `nvidia-smi`, and successfully links against `libcuda.so` from
    the host driver.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | None | This WP has no upstream dependencies. It starts on day one. |
| Blocks | WP-1 | TargetPrep CRD, Garage storage, provenance |
| Blocks | WP-2 | LibraryPrep CRD, Garage storage, provenance |
| Blocks | WP-3 | DockJob/RefineJob CRDs, GPU compute class, Garage storage |
| Blocks | WP-4 | ADMETJob CRD, Garage for results, advance API for triage handoff |
| Blocks | WP-5 | GenerateJob/LinkJob CRDs, GPU compute class, provenance chain |
| Blocks | WP-6 | SelectivityJob/RBFEJob/ABFEJob CRDs, GPU compute class |
| Blocks | WP-7 | Provenance graph for the browser, advance API as the UI's stage-handoff verb, K8s informer status stream |
| Blocks | WP-8 | IngestStructureJob/ReportJob CRDs, Garage for report storage |

## Out of Scope

- Multi-cluster scheduling (all jobs run on the single RKE2 cluster)
- Workflow engine (Argo Workflows, Tekton, etc.) -- the job controller handles sequencing
  directly via CRD watches plus the `advance` API
- Pause / resume / branch primitives — folded into `advance` (see Deferred section)
- Full event bus (NATS, Redis Streams) — deferred; K8s informers + WebSocket cover v0
- Data lake or data warehouse (this is operational storage, not analytics)
- External API gateway (the Go API server handles all external-facing traffic)
- Secret management changes (existing ExternalSecret pattern is sufficient)
- MySQL schema migration tooling (manual migrations via SQL scripts, same as current)
- Observability (Prometheus/Grafana stack is already deployed separately on the cluster)

## Open Questions

1. **Garage vs Longhorn-PVC for blobs**: Garage gives us bucket lifecycle (auto-expire
   scratch and trajectories) and S3 semantics, at the cost of one more StatefulSet to
   operate. Longhorn-PVC would mean every job writes to a shared volume and we sweep old
   files with a CronJob. Which is the right complexity floor for our scale?

2. **Provenance storage**: MySQL for provenance records keeps the stack simple (existing
   MySQL instance) but graph traversal queries (ancestor/descendant) are not MySQL's
   strength. Should provenance be stored in a graph-aware system (e.g., embedded SQLite
   with recursive CTEs, or a purpose-built graph) or is MySQL with recursive CTE queries
   sufficient?

3. **Object store sizing**: Estimate: receptors (small, <1 MB each), libraries (10-100 MB
   for 100K compounds), trajectories (10-50 MB per compound), reports (<10 MB each). For
   100 docking runs with 1000 compound refinements each: ~50-500 GB trajectory storage.
   What PV size to provision for Garage's data nodes?

4. **CRD vs ConfigMap for job definitions**: The current plugin YAML system uses ConfigMaps
   (`deploy/plugins-configmap.yaml`). CRDs are more Kubernetes-native and support status
   subresources. Should the existing plugin system migrate to CRDs, or coexist?

5. **Gate condition language**: What expression language for gate conditions? Options:
   (a) CEL (Common Expression Language, used by K8s ValidatingAdmissionPolicy),
   (b) JSONPath expressions against the upstream CRD status, (c) simple key-threshold pairs
   (e.g., `{"field": "bestAffinity", "op": "lt", "value": -7}`). CEL is powerful but
   complex; simple pairs cover 90% of cases.

6. **Migration from MySQL BLOBs**: The existing `docking_workflows.receptor_pdbqt` column
   stores receptor files as BLOBs. When should migration to Garage happen? Options:
   (a) big-bang migration before WP-1 starts, (b) dual-write during a transition period
   (feature flag), (c) write-new-to-Garage, read-old-from-MySQL, never migrate historical
   data.

7. **`nixos-gpu` reachability**: the desktop is a workstation that may be rebooted ad-hoc.
   Are jobs allowed to fail/retry across reboots, or do we taint the node out of the
   cluster when interactive use is expected? Also, RKE2 version skew (agent 1.33 vs server
   1.34) — when do we align?

## Technical Constraints

- **Namespace**: All infrastructure components deploy in the `chem` namespace. No new
  namespaces unless required for isolation.
- **Flux GitOps**: All manifests must be compatible with the existing Flux deployment
  model. CRDs go in `deploy/crds/`, controllers in `deploy/`, infrastructure (Garage) in
  `deploy/infra/`.
- **RBAC**: The `khemeia-controller` ServiceAccount needs extended permissions for CRD
  CRUD and Garage access. Define a new ClusterRole `khemeia-infrastructure` or extend the
  existing role.
- **Existing job_runner.go**: The new job controller extends (not replaces) the existing
  `job_runner.go`. The existing plugin-based job system must continue to work for current
  plugins. CRD-based jobs are a parallel path that eventually supersedes the plugin jobs.
- **MySQL**: The existing MySQL instance is adequate for provenance records (small rows,
  indexed queries). Do not deploy a second database for provenance.
- **No Helm**: The cluster uses Kustomize via Flux, not Helm. All manifests must be
  Kustomize-compatible.
- **Image registry**: Garage container images must be mirrored to
  `zot.hwcopeland.net/infra/` (not `chem/`, which is for application containers).
- **NixOS GPU node**: the `gpu` compute class targets `nixos-gpu` (RTX 3070, NixOS 25.11,
  10.41.0.10). NVIDIA driver libs/binaries live under `/run/opengl-driver` (symlinks
  into `/nix/store`). The job controller's GPU-class job-template must inject the host-path
  mounts for `/run/opengl-driver` and `/nix/store` and set
  `LD_LIBRARY_PATH=/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu`
  and `OCL_ICD_VENDORS=/run/opengl-driver/etc/OpenCL/vendors` on every GPU pod. Tolerations for
  `gpu=true:NoSchedule` are mandatory. See `memory/project_gpu_node.md` for the full
  caveats list.

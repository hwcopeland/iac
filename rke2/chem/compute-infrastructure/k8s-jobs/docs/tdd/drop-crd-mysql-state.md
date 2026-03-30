---
project: "k8s-jobs"
maturity: "draft"
last_updated: "2026-03-29"
updated_by: "@staff-engineer"
scope: "Full controller rewrite: drop CRD, MySQL-backed state, staging table + result-writer for lock-free writes, two-phase ligand prep/docking pipeline, PVC elimination"
owner: "@staff-engineer"
dependencies: []
---

# TDD: Docking Controller Rewrite -- Staging Table Architecture with Two-Phase Pipeline

## 1. Problem Statement

The docking job controller has accumulated structural debt from its Airflow migration origin. Six tightly coupled problems need to be solved together because they share a root cause: the shared PVC (`pvc-autodock`) and the file-based data pipeline.

1. **Overlapping state mechanisms, none adequate.** The controller uses `sync.Map` (lost on restart), a `DockingJob` CRD (deployed but never read/written by the controller), and K8s Job label queries (fragile, false positives from stale TTL-expired jobs).

2. **Shared PVC as inter-step communication.** All pipeline steps mount a 20Gi Longhorn RWX PVC to pass intermediate files (SDF splits, PDBQT ligand files, docking logs). This creates coupling between steps through filesystem layout conventions, a Longhorn RWX dependency (NFS share manager) that is operationally heavy for scratch space, and cleanup complexity from accumulated files.

3. **Jupyter/jovyan PVC dependency.** The `copy-ligand-db` step copies an SDF file from `claim-jovyan` (a JupyterHub user PVC) to the shared PVC. This couples the docking system to JupyterHub's PVC naming scheme and requires a running Jupyter environment to stage ligand data.

4. **File-based ligand data flow.** Ligand data starts as a monolithic SDF file on a PVC, gets split by `split_sdf.sh`, converted by `ligandprepv2.py`, then consumed by `dockingv2.py` -- all via filesystem. The pipeline cannot start without the SDF being physically present, and batch boundaries are determined at runtime by a shell script.

5. **Too many pipeline steps.** The current 7-step pipeline (copy-ligand-db, prepare-receptor, split-sdf, prepare-ligands, docking, postprocessing, export-mysql) exists because each step produces files consumed by the next on the shared PVC.

6. **CRD deployed but unused.** The `DockingJob` CRD is deployed via Kustomize but the controller never reads or writes it. No informer, no reconciler, no typed client.

7. **Write contention at scale.** If batch pods were to write directly to indexed main tables, concurrent writes to `docking_results` (with 4 secondary indexes) would cause lock contention. If pods returned results via stdout for the controller to write, the controller becomes a serialization bottleneck parsing and inserting potentially millions of result rows.

### Why Now

- MySQL is already deployed and the controller already connects to it for `GetResults`.
- The PVC-based architecture makes the pipeline hard to test, hard to debug, and impossible to run without Longhorn RWX.
- Ligand data should be a managed resource, not an opaque SDF file on a Jupyter user's PVC.
- The ligand prep phase (3D conformer generation + PDBQT conversion) is protein-agnostic and should be done once per compound library, not repeated for every docking workflow.

### Acceptance Criteria

- After a controller pod restart, `GET /api/v1/dockingjobs/{name}` returns the correct phase, batch progress, and result for all previously submitted workflows.
- The CRD YAML, its Kustomize reference, and any CRD-specific code are removed.
- The `sync.Map` fields (`jobStatuses`, `results`) are removed from `DockingJobController`.
- `pvc-autodock` PVC and all PVC volume mounts are removed from K8s Job specs. Jobs use `emptyDir` for scratch space.
- `JupyterUser`, `UserPvcPrefix`, `claim-*`, and all jovyan references are removed.
- `copy-ligand-db`, `split-sdf`, `postprocessing`, and `export-mysql` steps are eliminated from the pipeline.
- Ligands are stored in a MySQL `ligands` table with a `pdbqt` column (NULL until prepped). Prep is a one-time operation per compound library.
- A new `POST /api/v1/prep` endpoint triggers ligand prep for a `source_db`.
- Batch pods write results to an unindexed `staging` table (append-only, fast writes).
- A new result-writer Deployment (single replica, long-running) drains the staging table into the appropriate main tables.
- Batch containers are read-only against main tables (SELECT ligands and receptor data) and write-only against the staging table.
- Postprocessing (best energy) is computed by the controller via SQL `MIN()` after all batches complete.
- The REST API contract is updated: `jupyter_user` removed from request; `ligand_db` repurposed as a filter on `ligands.source_db`.
- No new Go dependencies beyond `database/sql` and `github.com/go-sql-driver/mysql` (both already imported).

### Constraints

- Single-replica controller (`replicas: 1`, no concurrent write contention on workflow state).
- Single-replica result-writer (single writer to indexed tables = zero lock contention).
- MySQL accessed over cluster network (`ClusterIP` service). Controller pod already has `MYSQL_*` env vars.
- Controller uses vanilla `client-go` -- no controller-runtime, no code-gen. Keep it that way.
- Workflow recovery on restart is out of scope.
- Ligand import tooling is out of scope (schema defined, import tool is separate work).
- Batch parallelism optimization is out of scope (sequential for v1).

---

## 2. Context and Prior Art

### Current Code Structure

```
controller/
  main.go      -- DockingJobController struct, processDockingJob pipeline,
                   K8s Job creation functions (7 steps), reconcileJobs (no-op)
  handlers.go  -- APIHandler: CRUD handlers, GetResults (already uses MySQL)

docker/autodock-vina/scripts/
  proteinprepv2.py       -- Downloads PDB, removes native ligand, creates PDBQT + grid_center.txt
  ligandprepv2.py        -- Reads SDF file, converts each mol to PDB, runs prepare_ligand4 -> PDBQT
  dockingv2.py           -- Reads receptor PDBQT + grid_center.txt + ligand PDBQTs, runs Vina
  split_sdf.sh           -- Splits monolithic SDF into batch files using awk
  3_post_processing.sh   -- Finds best energy across all docked log files
  export_energies_mysql.py -- Parses Vina .log files from PVC, bulk-inserts into docking_results

config/
  pvc-autodock.yaml      -- 20Gi RWX Longhorn PVC (to be removed)
  seed-test-fixture.yaml -- Seeds test SDF to claim-jovyan path on PVC (to be removed)
  deployment.yaml        -- Controller Deployment + Service
  mysql.yaml             -- MySQL Deployment + PVC + Service
  rbac.yaml              -- ServiceAccount, Role, RoleBinding

crd/
  dockingjob-crd.yaml    -- CRD definition (never used at runtime, to be removed)

jobs/
  job-templates.yaml     -- Go-template YAML for all 7 job types (to be rewritten)
  01-copy-ligand-db.yaml -- Standalone copy job template (to be removed)
```

### How `processDockingJob` Works Today

1. `CreateJob` handler builds a `DockingJob` struct from the HTTP request, seeds `jobStatuses.Store(name, {Phase: "Running"})`, fires `go controller.processDockingJob(job)`.
2. `processDockingJob` walks through 7 sequential/batched pipeline steps:
   - **Step 1 (copy-ligand-db):** Alpine container copies SDF from `claim-jovyan` path to PVC root. Depends on Jupyter user PVC.
   - **Step 2 (prepare-receptor):** `proteinprepv2.py` downloads PDB from RCSB, removes native ligand, produces `{pdbid}.pdbqt` + `grid_center.txt` on PVC. Started concurrently with step 3.
   - **Step 3 (split-sdf):** `split_sdf.sh` splits the SDF file into batch files on PVC. Prints batch count to stdout.
   - **Step 4 (prepare-ligands, per batch):** `ligandprepv2.py` reads batch SDF, converts each molecule to PDB, runs `prepare_ligand4` to produce PDBQT files.
   - **Step 5 (docking, per batch):** `dockingv2.py` reads receptor PDBQT + grid_center.txt + ligand PDBQTs from PVC, runs Vina.
   - **Step 6 (postprocessing):** `3_post_processing.sh` finds best energy across all `.log` files on PVC.
   - **Step 7 (export-mysql):** `export_energies_mysql.py` reads `.log` files, parses mode-1 affinities, bulk-inserts into `docking_results`.
3. Each step creates a K8s Job and polls `waitForJobCompletion` (5-second tick, 10-minute timeout).
4. State transitions are stored in `sync.Map`.
5. Postprocessing result is cached in `results sync.Map`.

### Key Observations About Current Python Scripts

**`proteinprepv2.py`** -- Self-contained. Downloads from RCSB, uses BioPython for PDB parsing, calls `prepare_receptor4` (MGLTools). Outputs `{pdbid}.pdbqt` and `grid_center.txt` to working directory. No dependency on PVC data beyond write access to working directory. **This script will be modified** to output receptor PDBQT (base64) and grid center as JSON to stdout, so the controller can capture and store the data without a shared volume.

**`ligandprepv2.py`** -- Reads an SDF file from filesystem, iterates molecules with RDKit, converts to PDB, runs `prepare_ligand4`. **This script will be replaced** by `prep_ligands.py` which reads SMILES from the `ligands` MySQL table and writes results to the `staging` table.

**`dockingv2.py`** -- Reads receptor PDBQT + grid_center.txt + ligand PDBQTs from filesystem directories. **This script will be replaced** by `dock_batch.py` which reads pre-calculated PDBQTs from the `ligands` table and receptor data from `docking_workflows`, then writes results to `staging`.

**`split_sdf.sh`** -- Awk-based SDF splitter. **Eliminated entirely.** Chunking moves to SQL `LIMIT/OFFSET` on the `ligands` table by ID range.

**`3_post_processing.sh`** -- Shell script that finds best energy from `.log` files. **Eliminated entirely.** The controller computes best energy via `SELECT MIN(affinity_kcal_mol) FROM docking_results`.

**`export_energies_mysql.py`** -- Reads `.log` files, parses Vina output, inserts to MySQL. **Eliminated entirely.** The result-writer handles all main-table writes.

### Prior Art

This design uses a well-established pattern for high-throughput write workloads:

- **Staging table pattern:** Used widely in data warehousing (ETL staging areas), message queue implementations, and write-ahead logs. The core idea: separate the fast-write path (unindexed append) from the indexed-read path (main tables), with a single consumer draining from one to the other.
- **Single-writer principle:** By funneling all writes to indexed tables through a single result-writer pod, we eliminate lock contention that would occur with N concurrent batch pods competing for the same indexes. This is the same principle behind WAL in databases and append-only logs in distributed systems.
- **Two-phase computation:** Separating ligand prep (protein-agnostic, done once) from docking (protein-specific, done per target) is standard in computational chemistry pipelines. Tools like ZINC and Enamine REAL databases pre-calculate 3D conformers for exactly this reason.

---

## 3. Alternatives Considered

### A. Fix the CRD pattern -- add informer/reconciler (REJECTED)

Add a dynamic client or code-gen for the `docking.khemia.io` group, build a proper informer cache, and reconcile `DockingJob` CR status.

**Strengths:** Kubernetes-native. `kubectl get dockingjobs` would show real status.
**Weaknesses:** Significant complexity: needs code-gen or dynamic client, RBAC for the custom API group, a real reconciler loop (the current `reconcileJobs` is a no-op). The controller's workflow is inherently sequential, which is a poor fit for the eventual-consistency reconciler pattern. Does not address the PVC, ligand data, or write contention problems.

**Rejected because:** The operator already has the REST API. Adding operator-framework complexity to make `kubectl` work as a secondary interface is not worth the cost, and it does not solve the pipeline architecture problems.

### B. Keep PVC, just fix state management (REJECTED)

Move state to MySQL but keep `pvc-autodock` as the inter-step communication mechanism.

**Strengths:** Smaller change. Scripts stay the same.
**Weaknesses:** Retains the Longhorn RWX dependency, the Jupyter PVC coupling, the fragile filesystem layout conventions, and the cleanup problem. The PVC is the root cause of most pipeline complexity.

**Rejected because:** The PVC is the core architectural problem. Fixing only state management addresses a symptom.

### C. Batch pods write directly to main tables (EVALUATED, NOT RECOMMENDED)

Each batch pod writes its results directly to `docking_results` (which has 4 secondary indexes).

**Strengths:** Simpler architecture -- no staging table, no result-writer pod.
**Weaknesses:** With N concurrent batch pods writing to `docking_results`, each INSERT acquires row locks and updates 4 secondary indexes. At scale (hundreds of thousands of ligands, many concurrent batches), this creates lock contention. InnoDB next-key locking on secondary indexes means even non-overlapping inserts can block each other. The performance problem scales with batch parallelism -- the exact dimension we want to optimize in the future.

**Not recommended because:** It works for sequential batches in v1, but creates an architectural ceiling that prevents future parallelism without a redesign.

### D. Batch pods return results via stdout, controller writes to DB (EVALUATED, NOT RECOMMENDED)

Each batch pod outputs results as JSONL to stdout. The controller captures stdout after each batch completes, parses results, and bulk-inserts into `docking_results`.

**Strengths:** Batch pods need no MySQL write access. Simple container code.
**Weaknesses:** The controller becomes a serialization bottleneck. For a batch of 10,000 ligands, it must `io.ReadAll` the pod logs (~1MB), parse 10,000 JSON lines, and execute a bulk INSERT. With future parallelism, the controller would need to capture and insert results for N batches concurrently, competing with its own API serving. Pod log retrieval is also unreliable -- logs can be truncated, and the K8s log API has size limits.

**Not recommended because:** It works for v1's sequential batches but makes the controller a bottleneck that prevents scaling. Log-based data transfer is also fragile.

### E. Object storage (S3/MinIO) for ligands (REJECTED)

Store ligand files in S3-compatible object storage.

**Strengths:** Better fit for large binary blobs than MySQL. Familiar pattern.
**Weaknesses:** Adds a new infrastructure dependency (MinIO deployment, credentials, bucket lifecycle). Individual PDBQT files are small (~5-50KB). MySQL handles this volume fine. The system already has MySQL; adding MinIO doubles operational surface.

**Rejected because:** MySQL is already deployed and adequate for the data sizes involved.

### F. Staging table + result-writer (RECOMMENDED)

All batch pods write to an unindexed `staging` table (append-only, PK only). A single long-running result-writer pod polls the staging table, parses payloads, and writes to the appropriate main table (`ligands.pdbqt` for prep results, `docking_results` for dock results), then deletes processed staging rows.

**Strengths:**
- Batch pod writes are fast: INSERT into a table with no secondary indexes = minimal lock contention.
- Main table writes are serialized through a single writer = zero lock contention on indexed tables.
- Decouples batch compute from result persistence -- batch pods finish faster because they don't wait for indexed writes.
- Architecture scales naturally: when batch parallelism is added later, the staging table absorbs concurrent writes without degradation, and the result-writer drains at its own pace.
- Clean separation of concerns: batch pods do compute + fast writes; result-writer does data routing + indexed writes.

**Weaknesses:**
- One more component to deploy and monitor (the result-writer pod).
- Results are not immediately visible in main tables -- there is a propagation delay (seconds, bounded by poll interval).
- If the result-writer crashes, staging rows accumulate until it restarts (but no data is lost).

**Recommended because:** It solves the write contention problem at its root, scales with future parallelism, and the additional operational cost (one more Deployment) is modest.

---

## 4. Architecture and System Design

### Core Pattern: Staging Table + Result-Writer

```
                                   STAGING TABLE
                                   (unindexed, PK only)
                                        |
    Prep pods ----INSERT(prep)--------> |
    Dock pods ----INSERT(dock)--------> |
                                        |
                                   RESULT-WRITER
                                   (single replica, long-running)
                                        |
                            +-----------+-----------+
                            |                       |
                    UPDATE ligands.pdbqt    INSERT docking_results
                    (for prep payloads)    (for dock payloads)
                            |                       |
                    DELETE staging rows     DELETE staging rows
```

**Why this works:**
- The staging table has no secondary indexes. INSERT is an append to the clustered index (auto-increment PK). InnoDB page splits are rare because inserts are always at the tail. Multiple batch pods can INSERT concurrently with negligible lock contention.
- The result-writer is the sole writer to `ligands` (UPDATE pdbqt) and `docking_results` (INSERT). These tables have secondary indexes, but a single writer means zero contention.
- The result-writer polls, processes a batch of rows, writes to the target table, then deletes the processed staging rows. A crash loses no data -- undeleted staging rows are simply reprocessed on restart (idempotent writes recommended).

### Two-Phase Workflow

**Phase A: Ligand Prep (one-time per compound library)**

Pre-calculates 3D structures and PDBQT files for all ligands in a `source_db`. This is done once per compound library. The `ligands.pdbqt` column starts NULL and gets populated during prep.

1. Operator calls `POST /api/v1/prep` with `{"source_db": "ChEBI_complete", "chunk_size": 10000}`.
2. Controller queries `SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NULL` to determine how many ligands need prep.
3. Controller chunks by ID ranges and spawns prep pods.
4. Each prep pod:
   - SELECTs `id, smiles` from `ligands` table (by ID range, WHERE pdbqt IS NULL).
   - For each ligand: RDKit 3D conformer generation from SMILES, `prepare_ligand4` to produce PDBQT.
   - INSERTs `{ligand_id, pdbqt_b64}` into staging table with `job_type='prep'`.
5. Result-writer: reads prep staging rows, UPDATE `ligands SET pdbqt = ? WHERE id = ?`, deletes staging rows.

**Phase B: Docking (per protein target, reuses prepped ligands)**

1. Operator calls `POST /api/v1/dockingjobs` with pdbid, source_db, etc.
2. Controller creates workflow row in `docking_workflows` (phase='Running').
3. **Prepare-receptor:** 1 pod runs `proteinprepv2.py` (modified), outputs receptor PDBQT (base64) + grid center as JSON to stdout. Controller captures and stores in `docking_workflows` row.
4. **Dock batches:** N pods (sequential in v1), each:
   - SELECTs pre-calculated ligand PDBQTs from `ligands` table (WHERE source_db = ? AND pdbqt IS NOT NULL, by ID range). READ-ONLY against main tables.
   - SELECTs receptor PDBQT + grid center from `docking_workflows` table. READ-ONLY.
   - Writes receptor and ligand PDBQTs to emptyDir, runs Vina.
   - INSERTs `{ligand_id, compound_id, pdb_id, affinity_kcal_mol, workflow_name}` into staging table with `job_type='dock'`.
5. Result-writer: reads dock staging rows, bulk INSERT into `docking_results`, deletes staging rows.
6. Controller: after all batch pods complete, waits briefly for result-writer to drain, then runs `SELECT MIN(affinity_kcal_mol) FROM docking_results WHERE workflow_name = ?` for best energy.

### Pipeline Comparison

```
OLD PIPELINE (7 steps, PVC-dependent):
  copy-ligand-db                           [PVC: claim-jovyan -> pvc-autodock]
    |
  prepare-receptor + split-sdf (concurrent) [PVC: write PDBQT, grid_center.txt, batch SDFs]
    |
  for each batch:
    prepare-ligands                        [PVC: read batch SDF, write PDBQTs]
    docking                                [PVC: read receptor + ligand PDBQTs, write logs]
    |
  postprocessing                           [PVC: read all .log files, stdout best energy]
    |
  export-mysql                             [PVC: read all .log files, write to MySQL]


NEW PIPELINE (2 phases, no PVC):

  Phase A -- Ligand Prep (one-time per compound library):
    for each chunk:
      prep pod                             [emptyDir: SMILES -> RDKit 3D -> PDBQT]
                                            -> writes to staging table (job_type='prep')
                                            -> result-writer drains to ligands.pdbqt

  Phase B -- Docking (per protein target):
    prepare-receptor                       [emptyDir: download PDB, create PDBQT + grid center]
                                            -> controller captures from stdout, stores in docking_workflows
      |
    for each chunk:
      dock pod                             [emptyDir: read PDBQTs from DB, run Vina]
                                            -> writes to staging table (job_type='dock')
                                            -> result-writer drains to docking_results
      |
    controller: SELECT MIN(affinity)       [best energy from docking_results]
```

### Component Architecture

**Controller pod** (existing, rewritten):
- REST API for workflow CRUD + new prep endpoint.
- Orchestrates K8s Jobs (prep pods, dock pods, prepare-receptor pod).
- Stores workflow state in `docking_workflows` table.
- Captures receptor data from prepare-receptor pod stdout.
- Reads from MySQL (workflow state, ligand counts).
- Writes to MySQL (workflow state updates only -- NOT results, NOT ligand pdbqt data).
- Does NOT write to the staging table.

**Result-writer pod** (NEW, long-running Deployment, single replica):
- Runs continuously. Polls `staging` table every 2-5 seconds.
- For `job_type='prep'`: parses payload JSON, executes `UPDATE ligands SET pdbqt = ? WHERE id = ?`.
- For `job_type='dock'`: parses payload JSON, executes bulk `INSERT INTO docking_results (...)`.
- Deletes processed staging rows after successful write to target table.
- Needs `MYSQL_*` env vars (same secret as controller and batch pods).
- Exposes `/health` and `/readyz` endpoints for K8s probes.
- Simple Go binary or Python script (language choice deferred to implementation).
- Idempotent writes recommended: for prep, `UPDATE ... WHERE id = ? AND pdbqt IS NULL` prevents double-writes. For dock, the `docking_results` table allows duplicates (no unique constraint on ligand+workflow), so duplicate staging rows produce duplicate result rows -- acceptable since the result-writer deletes staging rows after successful insertion, and a crash-restart simply reprocesses.

**Prep pods** (batch, ephemeral):
- Spawned by controller for the ligand prep phase (`POST /api/v1/prep`).
- SELECT `id, smiles` from `ligands` table (by ID range, WHERE pdbqt IS NULL).
- For each ligand: RDKit 3D conformer generation from SMILES, `prepare_ligand4`, produce PDBQT.
- INSERT into `staging` table: `job_type='prep'`, `payload={"ligand_id": N, "pdbqt_b64": "..."}`.
- emptyDir for scratch.
- Read from `ligands` (SELECT), write to `staging` (INSERT). No writes to main indexed tables.

**Dock pods** (batch, ephemeral):
- Spawned by controller for the docking phase of a workflow.
- SELECT pre-calculated PDBQTs from `ligands` table (WHERE source_db = ? AND pdbqt IS NOT NULL, by ID range).
- SELECT receptor PDBQT + grid center from `docking_workflows` table.
- Write receptor + ligand PDBQTs to emptyDir, run Vina.
- INSERT into `staging` table: `job_type='dock'`, `payload={"ligand_id": N, "compound_id": "...", "pdb_id": "...", "affinity_kcal_mol": -7.1, "workflow_name": "..."}`.
- emptyDir for scratch.
- Read from `ligands` + `docking_workflows` (SELECT), write to `staging` (INSERT). No writes to main indexed tables.

**Prepare-receptor pod** (single, ephemeral):
- Same as before but with emptyDir instead of PVC.
- Modified `proteinprepv2.py` outputs receptor data as JSON to stdout: `{"receptor_pdbqt_b64": "...", "grid_center": [x, y, z]}`.
- Controller captures stdout, parses JSON, stores in `docking_workflows` row.

### What Changes (Controller)

```
BEFORE:
  HTTP request -> CreateJob handler -> sync.Map.Store("Running")
                                    -> go processDockingJob()
                                         -> createCopyLigandDbJob()      [PVC]
                                         -> createPrepareReceptorJob()   [PVC]
                                         -> createSplitSdfJob()          [PVC]
                                         -> for batch: createPrepareLigandsJob()
                                                     + createDockingJobExecution() [PVC]
                                         -> createPostProcessingJob()    [PVC]
                                         -> createMySQLExportJob()       [PVC]
                                         -> sync.Map.Store("Completed"/"Failed")

  HTTP GET      -> GetJob handler   -> sync.Map.Load()  [authoritative]
                                    -> fallback: list K8s Jobs by label

AFTER:
  POST /api/v1/prep -> PrepHandler -> query unprepped ligand count
                                   -> chunk by ID range, spawn prep pods
                                   -> (result-writer handles staging -> ligands.pdbqt)

  POST /api/v1/dockingjobs -> CreateJob -> INSERT INTO docking_workflows (phase='Running')
                                        -> go processDockingJob()
                                             -> createPrepareReceptorJob()   [emptyDir]
                                             -> capture receptor data from stdout
                                             -> UPDATE docking_workflows (receptor, grid center)
                                             -> query ligand count (WHERE pdbqt IS NOT NULL)
                                             -> compute batch count
                                             -> for batch: createDockBatchJob()  [emptyDir]
                                             -> wait for result-writer to drain staging
                                             -> SELECT MIN(affinity) for best energy
                                             -> UPDATE docking_workflows SET phase='Completed'

  GET /api/v1/dockingjobs/{name} -> SELECT FROM docking_workflows WHERE name = ?
```

### What Does NOT Change

- The REST API base paths for existing endpoints (`/api/v1/dockingjobs`, `/health`, `/readyz`).
- The `waitForJobCompletion` polling loop (5-second tick, 10-minute timeout).
- The deployment topology (single controller pod + MySQL pod). One new pod added (result-writer).
- The `GetLogs` handler (reads pod logs, no state dependency).
- The MySQL infrastructure (`config/mysql.yaml`).

### Component Changes

**`DockingJobController` struct (main.go):**
- Remove `results sync.Map` and `jobStatuses sync.Map` fields.
- Add `db *sql.DB` field, initialized in `NewDockingJobController`.
- Remove `DockingJobList` type and the `metav1.TypeMeta`/`metav1.ListMeta` embeddings.
- Remove `DockingJobFinalizer` constant.
- Remove `reconcileJobs` method and its ticker in `Run()`.
- Remove `createCopyLigandDbJob`, `createSplitSdfJob`, `createPrepareLigandsJob`, `createPostProcessingJob`, `createMySQLExportJob` methods.
- Rewrite `createPrepareReceptorJob` to use `emptyDir` volume, capture receptor data from stdout.
- Add `createDockBatchJob` method (dock pods read from DB, write to staging).
- Add `createPrepJob` method (prep pods read SMILES from DB, write to staging).
- Rewrite `processDockingJob` for the new 2-phase pipeline.
- Add `captureReceptorData` method (reads JSON from pod stdout, parses PDBQT + grid center).
- Remove `parseBatchCountFromLogs` (batch count = ligand count / chunk_size).
- Replace `captureResult` with SQL-based best energy computation.
- Remove `pvcVolume` and `pvcMount` helper functions. Add `emptyDirVolume` and `emptyDirMount`.
- Remove `DefaultAutodockPvc`, `DefaultUserPvcPrefix`, `DefaultMountPath`, `DefaultJupyterUser`, `DefaultLigandDb` constants.
- Add `ensureSchema()` for startup DDL.

**`APIHandler` struct (handlers.go):**
- Add `db *sql.DB` field (shared reference from controller).
- Remove the `postprocessingResult` fallback method.
- Rewrite `GetJob` to query MySQL only (remove K8s Job label fallback).
- Rewrite `ListJobs` to query MySQL.
- Rewrite `CreateJob` to INSERT into MySQL, remove `jupyter_user` from request.
- Add `PrepHandler` for `POST /api/v1/prep`.
- Update `DeleteJob` to DELETE from MySQL.
- Update `ReadinessCheck` to ping MySQL.
- Rewrite `GetResults` to use the shared `db` instead of opening a new connection per request.

**`DockingJobSpec` struct:**
- Remove `AutodockPvc`, `UserPvcPrefix`, `MountPath`, `JupyterUser` fields.
- `LigandDb` becomes `SourceDb` (semantically: which source database to filter ligands from).

**`DockingJobRequest` struct:**
- Remove `JupyterUser` field.
- `LigandDb` field remains but its meaning changes: it filters the `ligands` table by `source_db`.

### Connection Pool Management

`sql.Open("mysql", dsn)` once in `NewDockingJobController`. Set `db.SetMaxOpenConns(10)` (controller + concurrent batch result writes). Set `db.SetConnMaxLifetime(5 * time.Minute)`. The `*sql.DB` handle is passed to `APIHandler` at construction time. The current `GetResults` handler opens a new `sql.Open` on every request -- this changes to using the shared pool.

### Readiness Probe Enhancement

The `/readyz` handler verifies MySQL connectivity with `db.PingContext(ctx)`. If the DB is unreachable, the controller is not ready to accept work.

---

## 5. Data Models and Storage

### Table: `docking_workflows`

```sql
CREATE TABLE IF NOT EXISTS docking_workflows (
    name              VARCHAR(255) PRIMARY KEY,
    phase             ENUM('Pending', 'Running', 'Completed', 'Failed') NOT NULL DEFAULT 'Pending',
    pdbid             VARCHAR(32)  NOT NULL,
    source_db         VARCHAR(255) NOT NULL,
    native_ligand     VARCHAR(32)  NOT NULL DEFAULT 'TTT',
    chunk_size        INT          NOT NULL DEFAULT 10000,
    image             VARCHAR(512) NOT NULL,
    batch_count       INT          NOT NULL DEFAULT 0,
    completed_batches INT          NOT NULL DEFAULT 0,
    current_step      VARCHAR(64)  NULL,
    message           TEXT         NULL,
    result            TEXT         NULL,
    receptor_pdbqt    MEDIUMBLOB   NULL,
    grid_center_x     FLOAT        NULL,
    grid_center_y     FLOAT        NULL,
    grid_center_z     FLOAT        NULL,
    created_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at        TIMESTAMP    NULL,
    completed_at      TIMESTAMP    NULL,
    INDEX idx_phase (phase),
    INDEX idx_created_at (created_at)
);
```

**Design decisions:**

- **`name` as PK:** The workflow name (`docking-{unix-timestamp}`) is already the unique identifier. No auto-increment needed.
- **`phase` as ENUM:** Enforces the state machine (`Pending -> Running -> Completed|Failed`). 1-byte storage.
- **`source_db`:** Replaces `ligand_db`. Names the source database used to filter the `ligands` table (e.g., `ChEBI_complete`).
- **`receptor_pdbqt`:** MEDIUMBLOB (up to 16MB, typical PDBQT is ~200-500KB). Stores the prepared receptor file so dock-batch pods can retrieve it from DB.
- **`grid_center_{x,y,z}`:** Three FLOAT columns for the docking grid center coordinates. Dock-batch pods need these to run Vina.
- **`current_step`:** Tracks pipeline progress. Values: `prepare-receptor`, `dock-batch`. Nullable (null when not actively running a step).
- **`result`:** The computed best energy string (e.g., "Best energy: -7.1 kcal/mol").
- **No JSON blob for spec:** Individual columns are queryable and schema-enforced.

### Table: `ligands`

```sql
CREATE TABLE IF NOT EXISTS ligands (
    id            INT AUTO_INCREMENT PRIMARY KEY,
    compound_id   VARCHAR(255) NOT NULL,
    smiles        TEXT         NOT NULL,
    pdbqt         MEDIUMBLOB   NULL,      -- NULL until prep phase populates it
    source_db     VARCHAR(255) NOT NULL,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_source_db (source_db),
    UNIQUE INDEX idx_compound_source (compound_id, source_db)
);
```

**Design decisions:**

- **`id` as auto-increment PK:** Used for deterministic ordering and ID-range chunking. The controller computes batch boundaries as ID ranges (e.g., WHERE id BETWEEN start AND end).
- **`compound_id`:** The chemical compound identifier from the source database (e.g., `CHEBI:15365`). Uniqueness is per `(compound_id, source_db)`.
- **`smiles`:** The SMILES string representation of the compound. This is the input for ligand prep (RDKit 3D conformer generation). TEXT type supports arbitrary-length SMILES.
- **`pdbqt`:** MEDIUMBLOB, NULL until the prep phase populates it. The prep pipeline (Phase A) reads SMILES, generates 3D conformers via RDKit, converts to PDBQT via `prepare_ligand4`, and the result-writer updates this column from the staging table. Once populated, this column is read by dock-batch pods. **The NULL/non-NULL state of this column is the mechanism that tracks prep progress.**
- **`source_db`:** The name of the source database (e.g., `ChEBI_complete`, `ZINC`, `DrugBank`). Used to filter ligands when submitting a workflow.
- **Population:** This table is populated by external import tooling (out of scope for this TDD). The import tool reads SDF or SMILES files and inserts compounds with their compound IDs and SMILES. The `pdbqt` column starts NULL and gets populated by the prep phase.

### Table: `docking_results`

```sql
CREATE TABLE IF NOT EXISTS docking_results (
    id                INT AUTO_INCREMENT PRIMARY KEY,
    workflow_name     VARCHAR(255) NOT NULL,
    pdb_id            VARCHAR(10)  NOT NULL,
    ligand_id         INT          NOT NULL,
    compound_id       VARCHAR(255) NOT NULL,
    affinity_kcal_mol FLOAT        NOT NULL,
    created_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_workflow (workflow_name),
    INDEX idx_pdbid    (pdb_id),
    INDEX idx_affinity (affinity_kcal_mol),
    INDEX idx_ligand   (ligand_id)
);
```

**Changes from current schema:**

- **Added `ligand_id`:** Foreign key (by convention, not constraint) to `ligands.id`. Enables joining results back to ligand metadata.
- **Added `compound_id`:** Denormalized from `ligands` table for query convenience.
- **Removed `batch_label`:** No longer meaningful with ID-range chunking.
- **Removed `ligand_name`:** Replaced by `compound_id` and `ligand_id`.

**Migration note:** The existing `docking_results` data uses the old schema. A migration should add `ligand_id` and `compound_id` columns (nullable initially) and drop `batch_label` and `ligand_name`. Old data will have NULL values for new columns -- acceptable since old results predate the ligands table.

**Note on write path:** This table is written to ONLY by the result-writer (single writer). Dock-batch pods never INSERT here directly. They write to the staging table, and the result-writer drains staging rows into this table via bulk INSERT.

### Table: `staging`

```sql
CREATE TABLE IF NOT EXISTS staging (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    job_type   ENUM('prep', 'dock') NOT NULL,
    payload    JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- No additional indexes! PK only. This is intentional for write throughput.
```

**Design decisions:**

- **No secondary indexes.** The entire point of this table is fast writes. The only index is the auto-increment PK (clustered index). INSERTs are always appended at the tail of the B-tree, minimizing page splits and lock contention.
- **`job_type` as ENUM:** Discriminator for the result-writer. `prep` payloads are routed to `UPDATE ligands`, `dock` payloads are routed to `INSERT docking_results`. 1-byte storage.
- **`payload` as JSON:** Flexible schema per job type. The result-writer parses this based on `job_type`.
  - For `prep`: `{"ligand_id": 42, "pdbqt_b64": "base64-encoded-pdbqt-content"}`
  - For `dock`: `{"ligand_id": 42, "compound_id": "CHEBI:15365", "pdb_id": "7jrn", "affinity_kcal_mol": -7.1, "workflow_name": "docking-1711756800"}`
- **`created_at`:** Useful for monitoring staging table drain rate and detecting stale rows.
- **Lifecycle:** Rows are created by batch pods and deleted by the result-writer after successful processing. Under normal operation, the staging table should be near-empty. A growing staging table indicates the result-writer is falling behind or has crashed.

### How Containers Get Their Data

**Prep pods** receive MySQL connection info via env vars:
```yaml
env:
  - name: SOURCE_DB
    value: "ChEBI_complete"
  - name: BATCH_START_ID
    value: "1"
  - name: BATCH_END_ID
    value: "10000"
  - name: MYSQL_HOST
    value: "docking-mysql.chem.svc.cluster.local"
  - name: MYSQL_PORT
    value: "3306"
  - name: MYSQL_USER
    value: "root"
  - name: MYSQL_DATABASE
    value: "docking"
  - name: MYSQL_PASSWORD
    valueFrom:
      secretKeyRef:
        name: docking-mysql-secret
        key: root-password
```

**Dock-batch pods** receive additional workflow context:
```yaml
env:
  - name: WORKFLOW_NAME
    value: "docking-1711756800"
  - name: PDBID
    value: "7jrn"
  - name: SOURCE_DB
    value: "ChEBI_complete"
  - name: BATCH_START_ID
    value: "1"
  - name: BATCH_END_ID
    value: "10000"
  - name: MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_DATABASE / MYSQL_PASSWORD
    # same pattern as above
```

The container then:
1. SELECTs receptor data from `docking_workflows` (dock pods only).
2. SELECTs ligand data from `ligands` table using `WHERE id BETWEEN ? AND ?`.
3. Processes locally in `emptyDir`.
4. INSERTs results into `staging` table.

### Schema Migration Strategy

Use startup DDL in `NewDockingJobController`. The controller runs `ensureSchema()` on startup, which executes all four `CREATE TABLE IF NOT EXISTS` statements plus any `ALTER TABLE` for the `docking_results` schema change. This is the same pattern already used by `export_energies_mysql.py`.

---

## 6. API Contracts

### Request Changes

**`POST /api/v1/dockingjobs` -- Create Docking Workflow**

```json
{
    "pdbid": "7jrn",
    "ligand_db": "ChEBI_complete",
    "native_ligand": "TTT",
    "ligands_chunk_size": 10000,
    "image": "zot.hwcopeland.net/chem/autodock-vina:latest"
}
```

**Changes:**
- **Removed:** `jupyter_user` field. No longer relevant.
- **`ligand_db`:** Field name kept for backward compatibility, but semantics change. Previously: an SDF filename on the Jupyter PVC. Now: a filter value matched against `ligands.source_db`. If no ligands exist for the given `source_db` with non-NULL `pdbqt`, the controller returns `400 Bad Request` ("no prepped ligands found for source_db 'X' -- run POST /api/v1/prep first").

**`POST /api/v1/prep` -- Trigger Ligand Prep (NEW)**

```json
{
    "source_db": "ChEBI_complete",
    "chunk_size": 10000,
    "image": "zot.hwcopeland.net/chem/autodock-vina:latest"
}
```

**Response (202 Accepted):**
```json
{
    "source_db": "ChEBI_complete",
    "total_ligands": 50000,
    "unprepped_ligands": 50000,
    "batch_count": 5,
    "status": "Started"
}
```

If no unprepped ligands exist for the given `source_db`: returns `400 Bad Request` ("no unprepped ligands found for source_db 'X'").

### Response Changes

**`GET /api/v1/dockingjobs/{name}` -- Get Workflow**

Response shape unchanged:
```json
{
    "name": "docking-1711756800",
    "pdbid": "7jrn",
    "ligand_db": "ChEBI_complete",
    "status": "Running",
    "batch_count": 5,
    "completed_batches": 2,
    "message": "",
    "created_at": "2026-03-29T10:00:00Z",
    "start_time": "2026-03-29T10:00:01Z",
    "completion_time": null
}
```

The behavioral difference: after a controller restart, this returns the correct MySQL-persisted state instead of falling back to unreliable K8s Job label queries.

**`GET /api/v1/dockingjobs` -- List Workflows**

**Breaking change (intentional simplification):**

Old response:
```json
{
    "workflows": {"docking-123": ["docking-123-copy-ligand", "docking-123-split-sdf", ...]},
    "count": 1
}
```

New response:
```json
{
    "workflows": [
        {
            "name": "docking-1711756800",
            "phase": "Completed",
            "pdbid": "7jrn",
            "source_db": "ChEBI_complete",
            "batch_count": 5,
            "completed_batches": 5,
            "created_at": "2026-03-29T10:00:00Z"
        }
    ],
    "count": 1
}
```

The sub-job list was an implementation detail leak (K8s Job names are internal). No known consumer uses it.

**`GET /api/v1/dockingjobs/{name}/results` -- Get Results**

Unchanged. Still queries `docking_results` for aggregated stats.

**`DELETE /api/v1/dockingjobs/{name}` -- Delete Workflow**

Unchanged behavior. Now also deletes from `docking_workflows` table and associated `docking_results` rows.

### Internal State Transitions (MySQL Writes by Controller)

| Pipeline Event | MySQL Operation |
|---|---|
| `CreateJob` handler | `INSERT INTO docking_workflows (...) VALUES (...)` with `phase='Running'` |
| `processDockingJob` start | `UPDATE ... SET started_at=NOW(), current_step='prepare-receptor'` |
| After receptor prep | `UPDATE ... SET receptor_pdbqt=?, grid_center_x=?, grid_center_y=?, grid_center_z=?, current_step='dock-batch', batch_count=?` |
| After each dock-batch pod completes | `UPDATE docking_workflows SET completed_batches=completed_batches+1` |
| Pipeline success | `UPDATE ... SET phase='Completed', completed_at=NOW(), current_step=NULL, result=?, message=?` |
| Pipeline failure | `UPDATE ... SET phase='Failed', current_step=NULL, message=?` |
| `DeleteJob` handler | `DELETE FROM docking_results WHERE workflow_name = ?`, `DELETE FROM docking_workflows WHERE name = ?` |

**Note:** The controller never writes to `staging`, `ligands.pdbqt`, or `docking_results`. Those writes are handled by batch pods (staging) and the result-writer (main tables).

---

## 7. Volume Changes

### Remove

| Resource | File | Reason |
|---|---|---|
| `pvc-autodock` PVC | `config/pvc-autodock.yaml` | Replaced by `emptyDir` in each Job |
| PVC volume mounts | `controller/main.go` (all `create*Job` methods) | No PVC to mount |
| `pvcVolume()` helper | `controller/main.go` | No longer used |
| `pvcMount()` helper | `controller/main.go` | No longer used |

### Add

**`emptyDir` volumes in K8s Job specs:**

```go
func emptyDirVolume(name string) corev1.Volume {
    return corev1.Volume{
        Name: name,
        VolumeSource: corev1.VolumeSource{
            EmptyDir: &corev1.EmptyDirVolumeSource{},
        },
    }
}
```

Each K8s Job mounts `emptyDir` at `/data` for scratch space. The volume is created when the pod starts and destroyed when the pod terminates. No persistence, no RWX, no Longhorn dependency.

### Receptor Data Flow (No Shared Volume)

Since there is no shared PVC, the receptor prep output (PDBQT file + grid center) cannot be directly read by dock-batch pods via the filesystem. The solution:

1. `prepare-receptor` job writes `{pdbid}.pdbqt` and `grid_center.txt` to its `emptyDir`.
2. The modified `proteinprepv2.py` outputs a JSON object to stdout after prep completes: `{"receptor_pdbqt_b64": "...", "grid_center": [x, y, z]}`.
3. The controller captures stdout, parses the JSON, stores receptor data in `docking_workflows` table (`receptor_pdbqt` BLOB, `grid_center_{x,y,z}` FLOATs).
4. Each `dock-batch` container SELECTs receptor data from `docking_workflows` on startup, writes receptor files to its own `emptyDir`, then proceeds with docking.

---

## 8. Files to Remove

| File | Reason |
|---|---|
| `crd/dockingjob-crd.yaml` | CRD definition -- never used at runtime |
| `crd/` directory | Empty after removing the CRD |
| `config/pvc-autodock.yaml` | Shared PVC replaced by `emptyDir` |
| `config/seed-test-fixture.yaml` | Seeds SDF to `claim-jovyan` path -- no longer relevant |
| `jobs/01-copy-ligand-db.yaml` | Standalone copy job template -- step eliminated |

### Files to Modify

| File | Change |
|---|---|
| `controller/main.go` | Major rewrite: remove `sync.Map`, add `*sql.DB`, remove CRD types, remove 5 of 7 `create*Job` methods, rewrite `processDockingJob` for 2-phase pipeline, add receptor data capture, remove PVC helpers, add `emptyDir` helpers, remove `reconcileJobs`, add `ensureSchema()`, add prep pipeline methods |
| `controller/handlers.go` | Major rewrite: MySQL-backed handlers, remove `JupyterUser` from request, remove K8s Job label fallback from `GetJob`, remove `postprocessingResult`, rewrite `ListJobs`, use shared `*sql.DB`, add MySQL ping to `ReadinessCheck`, add `PrepHandler` for `POST /api/v1/prep` |
| `kustomization.yaml` | Remove `crd/dockingjob-crd.yaml`, `config/pvc-autodock.yaml`, `config/seed-test-fixture.yaml` from resources list; add result-writer Deployment |
| `jobs/job-templates.yaml` | Complete rewrite for new pipeline (prepare-receptor + dock-batch + prep), remove PVC references, add `emptyDir` volumes |

### Python Scripts to Modify/Remove

| Script | Action | Reason |
|---|---|---|
| `proteinprepv2.py` | **Modify** | Add JSON stdout output: after prep completes, print `{"receptor_pdbqt_b64": "...", "grid_center": [x, y, z]}` to stdout. Core logic unchanged. |
| `ligandprepv2.py` | **Remove** | Replaced by `prep_ligands.py` |
| `dockingv2.py` | **Remove** | Replaced by `dock_batch.py` |
| `split_sdf.sh` | **Remove** | Chunking done by SQL ID ranges in the controller |
| `3_post_processing.sh` | **Remove** | Best energy computed by controller via SQL `MIN()` |
| `export_energies_mysql.py` | **Remove** | Result-writer handles all main-table writes |

### Files to Create

| File | Description |
|---|---|
| `docker/autodock-vina/scripts/prep_ligands.py` | Reads SMILES from `ligands` table (by ID range), RDKit 3D conformer generation, `prepare_ligand4` for PDBQT, writes results to `staging` table with `job_type='prep'` |
| `docker/autodock-vina/scripts/dock_batch.py` | Reads pre-calculated PDBQTs from `ligands` + receptor from `docking_workflows`, runs Vina in emptyDir, writes results to `staging` table with `job_type='dock'` |
| `config/result-writer-deployment.yaml` | Deployment + Service for the result-writer pod (single replica, long-running) |
| Result-writer source (Go or Python) | Polls staging table, routes to main tables, deletes processed rows. Location TBD at implementation. |

### `prep_ligands.py` Interface

```
Inputs (env vars):
  SOURCE_DB        -- ligand source database name
  BATCH_START_ID   -- starting ligand ID (inclusive)
  BATCH_END_ID     -- ending ligand ID (inclusive)
  MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD, MYSQL_DATABASE

Behavior:
  1. SELECT id, smiles FROM ligands WHERE source_db = ? AND id BETWEEN ? AND ? AND pdbqt IS NULL
  2. For each ligand:
     a. RDKit: parse SMILES, generate 3D conformer (AllChem.EmbedMolecule)
     b. Write PDB to emptyDir
     c. Run prepare_ligand4 to produce PDBQT
     d. Read PDBQT, base64-encode
     e. INSERT INTO staging (job_type, payload) VALUES ('prep', '{"ligand_id": N, "pdbqt_b64": "..."}')
  3. Log progress to stderr: "Prepped ligand N/M"

Exit codes:
  0 -- success (all ligands processed; individual failures logged to stderr, skipped)
  1 -- fatal error (DB connection failure, no ligands found)
```

### `dock_batch.py` Interface

```
Inputs (env vars):
  WORKFLOW_NAME    -- workflow identifier (for querying receptor data)
  PDBID            -- PDB ID of the receptor
  SOURCE_DB        -- ligand source database name
  BATCH_START_ID   -- starting ligand ID (inclusive)
  BATCH_END_ID     -- ending ligand ID (inclusive)
  MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD, MYSQL_DATABASE

Behavior:
  1. SELECT receptor_pdbqt, grid_center_x, grid_center_y, grid_center_z
     FROM docking_workflows WHERE name = ?
  2. Write receptor PDBQT to emptyDir
  3. SELECT id, compound_id, pdbqt FROM ligands
     WHERE source_db = ? AND id BETWEEN ? AND ? AND pdbqt IS NOT NULL
  4. For each ligand:
     a. Decode PDBQT from DB, write to emptyDir
     b. Run Vina docking
     c. Parse mode-1 affinity from Vina output
     d. INSERT INTO staging (job_type, payload) VALUES ('dock',
        '{"ligand_id": N, "compound_id": "...", "pdb_id": "...",
          "affinity_kcal_mol": -7.1, "workflow_name": "..."}')
  5. Log progress to stderr: "Docked ligand N/M"

Exit codes:
  0 -- success (all ligands processed; individual docking failures logged to stderr, skipped)
  1 -- fatal error (DB connection failure, receptor data missing, no ligands found)
```

---

## 9. Migration and Rollout

### Pre-conditions

- MySQL instance is running and accessible (already true).
- Ligands have been imported into the `ligands` table for at least one `source_db` (required before docking can run; import tooling is out of scope but must be built before this is useful).

### Rollout Strategy

This is a breaking change to the pipeline architecture. There is no gradual migration path because the PVC-based and DB-based pipelines are fundamentally different. The rollout is atomic: deploy the new controller + result-writer, remove the old resources.

**Step 1: Deploy schema.**
The new controller runs `ensureSchema()` on startup, creating/modifying all four tables. This is idempotent.

**Step 2: Deploy the result-writer.**
The result-writer Deployment is added to Kustomize. It starts polling the (empty) staging table immediately.

**Step 3: Import ligands.**
Before the pipeline can run, the `ligands` table must be populated (SMILES + compound IDs). Then the operator runs `POST /api/v1/prep` to trigger PDBQT generation.

**Step 4: Deploy the new controller image.**
Update the controller Deployment to the new image. The new controller:
- Connects to MySQL and runs `ensureSchema()`.
- No longer has `sync.Map` state -- any workflows running during the previous pod's lifetime are not recovered (same behavior as current system on restart).
- Starts accepting API requests with the new pipeline.

**Step 5: Remove dead resources.**
- Remove `crd/dockingjob-crd.yaml` from repo and Kustomize.
- Remove `config/pvc-autodock.yaml` from repo and Kustomize.
- Remove `config/seed-test-fixture.yaml` from repo and Kustomize.
- `kubectl delete crd dockingjobs.docking.khemia.io` (or let Flux prune).
- `kubectl delete pvc pvc-autodock -n chem` after confirming no running workflows use it.

### Handling In-Flight Workflows

Neither the old nor the new design supports workflow recovery on restart. The improvement is that after restart, the API correctly reports the workflow's last known state from MySQL instead of falling back to unreliable K8s label queries.

Any workflows in-flight during the cutover will be orphaned. Manually clean up:
1. `kubectl delete jobs -n chem -l docking.khemia.io/parent-job={name}` for each orphaned workflow.
2. `UPDATE docking_workflows SET phase='Failed', message='Orphaned during migration' WHERE phase='Running';`

### Rollback Plan

If the new controller has issues:
1. Revert the controller image to the previous version (Flux git revert).
2. Stop or remove the result-writer Deployment.
3. Re-apply `pvc-autodock.yaml` and `seed-test-fixture.yaml` to Kustomize if they were removed.
4. Re-apply the CRD if it was removed.
5. The old controller starts with empty `sync.Map` -- same behavior as any restart under the old design.
6. The `docking_workflows`, `ligands`, and `staging` tables remain in MySQL (harmless, not accessed by old code).

---

## 10. Risks and Open Questions

### Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| MySQL unavailable at controller startup | Low | Controller fails readiness, no traffic routed | Readiness probe pings MySQL; retry `sql.Open` with backoff |
| MySQL unavailable mid-workflow | Low | `processDockingJob` UPDATE fails; batch jobs fail to write staging | Controller: log MySQL error, continue if possible. Batch jobs: exit 1, controller marks workflow Failed. |
| Result-writer crashes | Low | Staging rows accumulate until restart; results not visible in main tables | K8s restarts the pod (Deployment). No data loss -- staging rows are durable. Monitoring: alert on staging table row count growing over threshold. |
| Result-writer falls behind batch pod throughput | Low | Staging table grows; results appear with delay | Single-writer throughput should exceed batch pod output rate for sequential batches. Monitor staging table size. If needed: increase poll frequency or batch size in result-writer. |
| `ListJobs` response shape change breaks a consumer | Low | API consumer breaks | No known consumers of the sub-job list. If found, provide a compatibility endpoint. |
| Batch container MySQL connection overload | Low | Too many concurrent batch pods saturate MySQL connections | Controller processes batches sequentially (same as today). `db.SetMaxOpenConns(10)` on controller. Batch pods open/close connections per-job. |
| PDBQT BLOB too large for MySQL row | Very Low | Receptor prep fails to store | MEDIUMBLOB supports 16MB. Largest PDBQT files are ~2MB. Monitor `LENGTH(receptor_pdbqt)`. |
| Ligand table grows very large | Medium | Slow queries, storage pressure | Index on `source_db`. ID-range queries are fast with indexed PK. Monitor table size. Consider partitioning by `source_db` if needed later. |
| Staging table not drained (result-writer bug) | Low | Data accumulates, disk pressure | Monitor: `SELECT COUNT(*) FROM staging` should be near zero. Alert if > 10,000 rows. |
| Duplicate result rows from result-writer crash-restart | Low | Slightly inflated result counts | For prep: use `UPDATE ... WHERE pdbqt IS NULL` (idempotent). For dock: duplicates are possible but low-impact (same ligand+affinity pair). Acceptable for v1. |
| Long-running workflow orphaned during rollout | Medium | Workflow stuck in `Running` | Document: manually clean up after rollout. Future: add startup recovery scan (out of scope). |

### Open Questions

1. **Result-writer implementation language.** Go binary or Python script? **Recommendation: Go**, co-located in the `controller/` directory. Keeps the build simple (single Go module), avoids adding a Python runtime dependency to a separate pod. But Python is acceptable if faster to implement since it is a simple poll-process-delete loop.

2. **Result-writer poll interval.** 2 seconds (responsive but more DB queries) vs. 5 seconds (less load but more delay) vs. adaptive (poll faster when rows exist, slower when empty)? **Recommendation: adaptive.** Start at 5 seconds. When rows are found, poll again immediately. After N empty polls, back off to 5 seconds. This gives sub-second drain when batches are running and minimal load when idle.

3. **Staging table cleanup after result-writer processes rows.** DELETE individual rows or batch-DELETE by ID range? **Recommendation: batch-DELETE** (`DELETE FROM staging WHERE id <= ?` after processing all rows up to that ID). Fewer round-trips.

4. **`dock_batch.py` error handling for individual ligands.** If one ligand fails docking (bad PDBQT, Vina crash), should the entire batch fail? **Recommendation: skip the failing ligand, log to stderr, continue with remaining ligands.** The staging table only contains successful results. The controller can detect gaps by comparing expected vs. actual result count.

5. **Receptor PDBQT output format from `proteinprepv2.py`.** Options: (a) base64-encode the file after a delimiter line, (b) print a JSON object with `receptor_pdbqt_b64` and `grid_center` fields. **Recommendation: option (b)** -- JSON is easier to parse in Go.

6. **`docking_results` schema migration for existing data.** Old results have `batch_label` and `ligand_name` columns. Options: (a) `ALTER TABLE` to add new columns, drop old ones; old data has NULLs. (b) Create `docking_results_v2` table. **Recommendation: option (a)** -- old data is test data from development.

7. **How does the controller know the result-writer has fully drained staging for a workflow before computing best energy?** Options: (a) poll `SELECT COUNT(*) FROM staging WHERE payload->'$.workflow_name' = ?` until zero, (b) poll `SELECT COUNT(*) FROM staging` until zero (simpler but waits for all staging, not just this workflow), (c) add a short fixed delay after last batch pod completes. **Recommendation: option (a)** -- targeted, correct, and the JSON path query is cheap on a small table.

---

## 11. Testing Strategy

### Unit Tests

- Test MySQL query construction for each handler (use `sqlmock` or a test MySQL instance).
- Test batch count computation from ligand count and chunk size (edge cases: 0 ligands, count < chunk_size, count exactly divisible, count not divisible).
- Test receptor data JSON parsing from `proteinprepv2.py` stdout.
- Test `DockingJobStatus` -> MySQL row -> `DockingJobResponse` round-trip serialization.
- Test result-writer payload parsing for both `prep` and `dock` job types.
- Test result-writer idempotent prep writes (second UPDATE with same ligand_id is a no-op when pdbqt is already non-NULL).
- Test staging table drain logic (process N rows, delete, verify empty).

### Integration Tests

- Start a MySQL container (testcontainers or Docker Compose), run the controller, submit a workflow via the REST API, verify MySQL rows at each state transition.
- Pre-populate `ligands` table with test SMILES, trigger `POST /api/v1/prep`, verify staging rows are created and result-writer drains them to `ligands.pdbqt`.
- Submit a docking workflow against prepped ligands, verify dock-batch pods create staging rows, result-writer drains them to `docking_results`.
- Simulate controller restart: kill the controller mid-workflow, restart it, verify `GetJob` returns the last persisted state from MySQL.
- Simulate result-writer restart: kill it while staging rows exist, restart, verify all rows are eventually drained to main tables.
- Verify `ListJobs` returns the new response format with workflow summaries.
- Verify `DeleteJob` cleans up `docking_workflows`, `docking_results` rows, and associated K8s Jobs.

### Manual Verification Checklist

- [ ] Import test ligands into `ligands` table.
- [ ] Run `POST /api/v1/prep` for the test source_db. Verify staging rows appear and result-writer drains them.
- [ ] Verify `SELECT COUNT(*) FROM ligands WHERE pdbqt IS NOT NULL AND source_db = 'test'` matches expected count.
- [ ] Submit a docking workflow. Verify `GET /api/v1/dockingjobs/{name}` returns `Running` with batch progress.
- [ ] Wait for completion. Verify `GET` returns `Completed` with best energy result.
- [ ] Verify `GET /api/v1/dockingjobs/{name}/results` returns correct affinity stats.
- [ ] Restart the controller pod. Verify `GET` still returns `Completed` with result.
- [ ] Restart the result-writer pod. Verify staging rows (if any) are drained after restart.
- [ ] Submit a workflow, kill the controller mid-run, restart, verify `GET` returns `Running` with last known progress.
- [ ] `DELETE` a workflow, verify MySQL rows are removed and K8s Jobs are cleaned up.
- [ ] Verify `GET /readyz` returns unhealthy when MySQL is down.
- [ ] Verify `kubectl get crd dockingjobs.docking.khemia.io` returns NotFound after CRD removal.
- [ ] Verify `kubectl get pvc pvc-autodock -n chem` returns NotFound after PVC removal.
- [ ] Verify no K8s Jobs reference `pvc-autodock` in their volume mounts.
- [ ] Submit a workflow with an invalid `ligand_db` (no matching `source_db` with prepped ligands), verify 400 response.
- [ ] Verify staging table row count stays near zero during normal operation.

---

## 12. Observability and Operational Readiness

### Key Signals

- **Controller logs:** All MySQL write errors logged at `ERROR` level with workflow name and operation. Batch result parsing failures logged at `WARN`.
- **Readiness probe:** `/readyz` fails if `db.PingContext()` fails (controller). Result-writer has its own `/readyz` with MySQL ping.
- **MySQL connection pool:** Log pool stats (`db.Stats()`) periodically (every 60 seconds at `DEBUG`) to detect connection leaks.
- **Staging table health:** The result-writer logs its drain rate: "Processed N staging rows (prep=X, dock=Y)". A key operational signal: `SELECT COUNT(*) FROM staging` should be near zero. Alert if > 10,000 rows (indicates result-writer is behind or crashed).
- **Batch job progress:** Prep and dock pods log progress to stderr (`Prepped ligand N/M`, `Docked ligand N/M`). Staging INSERTs happen during processing (not at end), so partial progress is captured even if the pod crashes.

### 3am Diagnosability

If paged because docking workflows are stuck:

1. **Check controller pod logs:** Is it running? Are there MySQL connection errors?
2. **Check result-writer pod logs:** Is it running? Is it draining staging rows? Look for `Processed N staging rows` messages.
3. **Query staging table health:**
   ```sql
   SELECT job_type, COUNT(*), MIN(created_at), MAX(created_at) FROM staging GROUP BY job_type;
   ```
   If rows are accumulating (large count, old `MIN(created_at)`), the result-writer is stuck or crashed.
4. **Query workflow state:**
   ```sql
   SELECT name, phase, current_step, completed_batches, batch_count,
          TIMESTAMPDIFF(MINUTE, started_at, NOW()) AS running_minutes
   FROM docking_workflows
   WHERE phase = 'Running';
   ```
   This gives the exact state of every in-flight workflow -- a significant improvement over the current state where the in-memory map is inaccessible without the controller running.
5. **Check batch jobs:** `kubectl get jobs -n chem -l docking.khemia.io/parent-job={name}` to see which K8s Jobs are alive.
6. **Check ligand availability:** `SELECT source_db, COUNT(*), SUM(pdbqt IS NOT NULL) AS prepped FROM ligands GROUP BY source_db;` to verify ligands are imported and prepped.
7. **Check batch container logs:** `kubectl logs job/{name}-dock-batch-{N} -n chem` -- stderr shows processing progress.

### Cleanup

After a workflow completes, the only persistent artifacts are MySQL rows (`docking_workflows` + `docking_results`). Staging rows are deleted by the result-writer after processing. There are no PVC files to clean up, no orphaned data on shared storage. K8s Jobs are cleaned by `TTLSecondsAfterFinished: 300`.

---

## 13. Implementation Phases

### Phase 1: MySQL Schema + Controller State Migration (Size: M)

Migrate the controller's state management from `sync.Map` to MySQL. This is the foundation for all subsequent phases.

**Scope:**
- Add `*sql.DB` to `DockingJobController`, initialize in `NewDockingJobController`.
- Implement `ensureSchema()` -- creates all 4 tables (`docking_workflows`, `ligands`, `docking_results`, `staging`), modifies `docking_results` if needed.
- Replace all `jobStatuses.Store()`/`jobStatuses.Load()`/`results.Store()` calls with MySQL INSERT/UPDATE/SELECT.
- Replace `GetJob` K8s fallback with MySQL SELECT.
- Replace `ListJobs` K8s Job listing with MySQL SELECT (new response shape).
- Update `CreateJob` to INSERT into MySQL, remove `jupyter_user` from request.
- Update `DeleteJob` to DELETE from MySQL (both tables).
- Update `ReadinessCheck` to ping MySQL.
- Rewrite `GetResults` to use shared `*sql.DB`.
- Remove `sync.Map` fields, `postprocessingResult` method, `DockingJobFinalizer`, `reconcileJobs`, `DockingJobList`.
- Clean up `DockingJob`/`DockingJobSpec` structs (remove CRD-related embeddings, remove PVC/Jupyter fields).

**Dependencies:** None. Can start immediately.
**Pipeline still uses PVC in this phase** -- the pipeline code is not changed yet. The existing 7-step pipeline continues to work while state management is moved to MySQL.

### Phase 2: Pipeline Redesign (Size: L)

Eliminate PVC-based steps, create the staging-table-backed two-phase pipeline.

**Scope:**
- Create `prep_ligands.py` script (SMILES from DB -> RDKit 3D -> PDBQT -> staging table).
- Create `dock_batch.py` script (PDBQTs from DB + receptor from DB -> Vina -> staging table).
- Create result-writer binary/script + `config/result-writer-deployment.yaml`.
- Modify `proteinprepv2.py` to output receptor data as JSON to stdout.
- Remove `createCopyLigandDbJob`, `createSplitSdfJob`, `createPrepareLigandsJob`, `createPostProcessingJob`, `createMySQLExportJob` from controller.
- Rewrite `createPrepareReceptorJob` to use `emptyDir`, add receptor data capture from stdout.
- Add `createDockBatchJob` to controller (dock pods read DB, write staging).
- Add `createPrepJob` to controller (prep pods read DB, write staging).
- Add `POST /api/v1/prep` endpoint and handler.
- Rewrite `processDockingJob` for the new pipeline (prepare-receptor -> dock batches -> wait for drain -> compute best energy).
- Add `captureReceptorData` method (JSON parsing from pod stdout).
- Replace `captureResult` with SQL-based best energy computation.
- Replace `pvcVolume`/`pvcMount` with `emptyDirVolume`/`emptyDirMount`.
- Remove `parseBatchCountFromLogs` (batch count = ligand count / chunk_size).
- Remove `DefaultAutodockPvc`, `DefaultUserPvcPrefix`, `DefaultJupyterUser`, `DefaultLigandDb` constants.
- Rewrite `jobs/job-templates.yaml` for new pipeline.
- Update Docker image build to include `prep_ligands.py`, `dock_batch.py`, and remove old scripts.

**Dependencies:** Phase 1 deployed and verified.

### Phase 3: Remove Dead Code and Files (Size: S)

Clean up all artifacts from the old architecture.

**Scope:**
- Delete `crd/dockingjob-crd.yaml` and `crd/` directory.
- Delete `config/pvc-autodock.yaml`.
- Delete `config/seed-test-fixture.yaml`.
- Delete `jobs/01-copy-ligand-db.yaml`.
- Update `kustomization.yaml` to remove references to deleted files.
- Delete `ligandprepv2.py`, `dockingv2.py`, `split_sdf.sh`, `3_post_processing.sh`, `export_energies_mysql.py` from Docker image scripts directory.
- After deployment: `kubectl delete crd dockingjobs.docking.khemia.io`.
- After deployment: `kubectl delete pvc pvc-autodock -n chem`.

**Dependencies:** Phase 2 deployed and verified.

### Phase 4: Documentation Update (Size: S)

**Scope:**
- Rewrite `README.md`: architecture overview, API examples, env vars, two-phase workflow, result-writer operations.
- Document the `ligands` table schema and import requirements.
- Document `prep_ligands.py` and `dock_batch.py` interfaces for container image maintainers.
- Document result-writer operations (monitoring, restart behavior, staging table health).

**Dependencies:** Phase 2 deployed. Can run in parallel with Phase 3.

### Out of Scope

- **Workflow recovery on restart.** The controller does not scan for `Running` workflows on startup and attempt to resume them. This would require tracking which K8s Jobs have already completed for a workflow, and is complex enough to warrant its own TDD.
- **Ligand import tooling.** The `ligands` table schema is defined here. A tool to import SDF/SMILES files into this table is separate work.
- **Batch parallelism optimization.** Batches run sequentially in v1 (same as current behavior). The staging table architecture is designed to support concurrent batches in the future, but that optimization is not implemented here.

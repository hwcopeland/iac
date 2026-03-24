---
project: "compute-infrastructure"
maturity: "proof-of-concept"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "Performance characteristics, bottlenecks, and scaling constraints of the molecular docking workflow — batch parallelism, shared PVC I/O, container scheduling overhead, and the Airflow-to-K8s migration context"
owner: "@staff-engineer"
dependencies:
  - architecture.md
  - operations.md
---

# Performance Specification

## Domain Context

This is an HPC-class workload. The goal is virtual ligand screening — running molecular docking
simulations (AutoDock4 or AutoDock Vina) across potentially millions of ligands to identify
candidates with strong binding affinity to a target protein. Throughput is the primary
performance metric: total ligands docked per hour constrains which databases are screenable in
a practical timeframe. The ChEBI complete database (`ChEBI_complete.sdf`) is the default target.

---

## Architecture Summary (Performance-Relevant)

The workflow consists of six sequential stages, the middle two running in parallel batches:

```
copy-ligand-db  ->  prepare-receptor  ->  split-sdf
                                              |
                               +------+------+------+
                               |      |      |      |
                          [batch0] [batch1] ... [batchN]
                           prep    prep         prep
                           dock    dock         dock
                               |      |      |      |
                               +------+------+------+
                                              |
                                      postprocessing
```

Each stage runs as a Kubernetes `batch/v1` Job. The shared PVC (`pvc-autodock`) is the
inter-stage communication mechanism — there are no message queues, databases, or in-memory
channels between steps.

---

## Parallelism Strategy

### Batch-Level Parallelism (Inter-Job)

The SDF ligand database is split into N batches via `split_sdf.sh`. Each batch then runs
`prepare-ligands` and `docking` as independent Kubernetes Jobs, which the scheduler can
place on different nodes.

**What is actually in the code:** The batch count returned by `createSplitSdfJob` is
hardcoded to `return 5, nil` (see `k8s-jobs/controller/main.go`, line 365). The `split_sdf.sh`
script itself correctly computes the number of batches from the chunk size parameter and prints
the count to stdout, but the controller ignores the script's output and unconditionally uses 5.
The `ligandsChunkSize` parameter (default 10,000) is passed to the script but the resulting
batch count is discarded.

**Consequence:** With `ChEBI_complete` (~94,000 compounds as of early 2024), a chunk size of
10,000 should produce ~10 batches. The controller will only process the first 5 batches. Any
ligands in batches 5-9 are silently dropped. The effective parallelism ceiling is currently 5
regardless of the configured chunk size.

**Parallelism execution model (in the controller):** Batch jobs are currently submitted in a
serial loop — `createPrepareLigandsJob` then `createDockingJobExecution` are called for batch 0,
then batch 1, etc. However, the controller does not wait between batch submissions (no
`waitForJobCompletion` between prepare-ligands and docking within a batch, and no gate between
batches). The Kubernetes scheduler therefore receives all batch jobs rapidly in sequence and
can schedule them in parallel across available worker nodes (k8s01–k8s05).

**Actual parallelism ceiling:** The cluster has 4 worker nodes (k8s01, k8s02, k8s04, k8s05).
With no resource limits on workflow job pods and 5 batches each requiring a prepare-ligands and
docking pod, up to 10 pods could be scheduled simultaneously — but no node affinity or
anti-affinity rules exist to distribute them, so the Kubernetes default-scheduler will spread
based on resource availability. Without resource requests on workflow pods, the scheduler has
no signal to make informed placement decisions.

### Intra-Batch Parallelism (Intra-Job, Per Container)

Both container images (`autodock4` and `autodock-vina`) install GNU `parallel` via apt. However,
neither `dockingv2.py` (AutoDock4) nor `dockingv2.py` (Vina) uses it — both iterate over PDBQT
files with a sequential Python `for` loop calling `subprocess.run()` one at a time. `parallel`
is installed but unused.

The `ligandprepv2.py` scripts similarly iterate sequentially over ligands using `subprocess.run`
for each `prepare_ligand4` call.

**Consequence:** Within a single batch, ligands are processed one at a time. With a chunk size
of 10,000 ligands and no intra-job parallelism, a single docking pod is constrained to serial
execution per ligand, making the per-pod throughput entirely dependent on single-core AutoDock
performance.

---

## Storage I/O: The Shared PVC

### Architecture

`pvc-autodock` is a 20Gi `ReadWriteMany` PVC backed by Longhorn (`longhorn-storage` storage
class). Every workflow job pod mounts this same PVC. It is the only mechanism for data flow
between stages:

- `copy-ligand-db` writes the source SDF to the PVC root.
- `prepare-receptor` writes `.pdbqt`, `grid_center.txt`, and receptor map files to the PVC root.
- `split-sdf` writes N batch SDF files (e.g., `ChEBI_complete_batch0.sdf`) to the PVC root.
- `prepare-ligands` reads its batch SDF from the PVC root and writes converted PDBQT files to
  `{MountPath}/output/`.
- `docking` reads PDBQT files from `{MountPath}/output/` and writes result files (`.pdbqt`,
  `.log`, `.dlg`) back to the PVC.
- `postprocessing` reads all `.dlg` result files from the PVC root.

At peak, 5 docking pods and 5 prepare-ligands pods can all simultaneously read from and write
to the same Longhorn RWX volume.

### Longhorn Performance Characteristics

Longhorn implements RWX via an NFS-based protocol over the cluster network. All I/O passes
through the Longhorn manager pods rather than directly hitting local disk. Key implications:

- **Not local NVMe.** Even if worker nodes have NVMe drives, Longhorn RWX adds NFS protocol
  overhead and routes through the Longhorn data plane. Effective throughput is significantly
  lower than raw local disk bandwidth.
- **Replication factor:** Longhorn defaults to 3 replicas. The `longhorn-storage` StorageClass
  in this repo does not override this. Every write is committed to 3 replicas before
  acknowledgment, tripling write I/O on the cluster network.
- **Network bottleneck:** The cluster is a homelab bare-metal setup with 1GbE or 10GbE links
  (hardware not documented in the repo). With 5 concurrent docking pods each performing I/O
  on the same Longhorn volume, network saturation on the storage path is a realistic concern
  at scale.
- **No `longhorn-ssd` usage:** The SSD storage class (`longhorn-ssd`) exists in the repo but
  the `pvc-autodock` PVC uses `longhorn-storage` (the default HDD-backed class). The SSD class
  has no `parameters` set to pin to SSD-tagged nodes, making its differentiation unclear.

### I/O Volume Estimates

The ChEBI_complete SDF is large (estimated 500MB–2GB for ~94K compounds). With a chunk size of
10,000 and 5 batches:

- `split-sdf` writes 5 batch SDF files (~100–400MB each) sequentially.
- `prepare-ligands` (x5 batches in parallel) reads its batch SDF and writes 10,000 PDB
  intermediates + 10,000 PDBQT files per batch — approximately 100–500MB of small file I/O
  per batch, amplified by the number of parallel pods.
- `docking` (x5 batches in parallel) reads PDBQT files and writes `.dlg` / `.pdbqt` result
  files — AutoDock4 produces verbose `.dlg` files that can reach 1–10MB per ligand. At 10,000
  ligands per batch, total result data per batch could reach 10–100GB.

The postprocessing step currently only extracts the best energy value using shell `grep/awk` —
this is I/O-bound over potentially millions of result files.

---

## Controller Performance: The 5-Second Reconcile Loop

The Go controller runs a `time.NewTicker(5 * time.Second)` loop that calls `reconcileJobs()`.
The `reconcileJobs()` function is a stub that returns `nil` immediately without doing any work.
The reconcile loop exists in structure but has no effect on workflow execution.

Job creation is triggered via the `POST /api/v1/dockingjobs` HTTP handler, which calls
`go h.controller.processDockingJob(job)` — a goroutine that runs without any lifecycle tracking.
There is no persistent state for active docking jobs, no retry of the goroutine if the
controller pod restarts mid-workflow, and no way to resume a partially-completed workflow.

**API server performance:** The HTTP server uses `net/http.ListenAndServe` with no timeouts set
on the server. The `ListJobs` handler lists all Kubernetes jobs with the label
`docking.khemia.io/parent-job` on every request — no caching, no pagination. Under concurrent
workflow submissions, this creates N * (number of batch jobs) Kubernetes API calls per list
request.

**Controller resource allocation:** The controller deployment requests 100m CPU / 128Mi memory
and is limited to 500m CPU / 512Mi memory. Given the non-blocking nature of `processDockingJob`
(goroutine) and the no-op reconcile loop, these limits are more than adequate for current usage,
but not right-sized for a fully-implemented reconciliation loop.

---

## Resource Limits: Workflow Job Pods

**No resource requests or limits are set on any workflow job pod.** This applies to:
- `copy-ligand-db` (alpine cp)
- `prepare-receptor` (proteinprepv2.py)
- `split-sdf` (split_sdf.sh with awk)
- `prepare-ligands` (ligandprepv2.py — RDKit + subprocess per ligand)
- `docking` (dockingv2.py — AutoDock4 or Vina binary)
- `postprocessing` (3_post_processing.sh — grep/awk)

**Consequence:** The Kubernetes scheduler has no signal for placement. Pods may co-schedule on
the same node, competing for CPU and memory. AutoDock4 is compute-intensive — running multiple
docking jobs on the same node without limits can saturate CPU and cause scheduler instability.
`prepare-ligands` with RDKit and 10,000 ligands can consume substantial memory. Without memory
limits, OOM kills are possible with no guard against them.

---

## Migration Rationale: Airflow vs. Kubernetes Jobs

The Airflow DAG (`rke2/airflow/dags/autodock4.py`) used `KubernetesPodOperator` to run the
same workload steps as Kubernetes pods, with Airflow as the orchestrator. The migration to
the native K8s controller was motivated by:

1. **Airflow infrastructure overhead:** Airflow required a scheduler, webserver, triggerer,
   4 workers, and a PostgreSQL instance (with `max_connections: 200`). All of these are
   persistent services consuming cluster resources even when no docking jobs are running.

2. **XCom data path:** The Airflow DAG used `do_xcom_push=True` on `split_sdf` to pass the
   batch count via Airflow's XCom mechanism (writing to `/airflow/xcom/return.json` inside
   the container). This was fragile — XCom is transported through the Airflow database, adding
   latency and a database dependency to the critical path.

3. **Dynamic task mapping (`docking.expand`):** The Airflow DAG used dynamic task mapping to
   fan out docking tasks, but this required Airflow to query XCom data after `split_sdf`
   completed before it could schedule the parallel batch tasks. The K8s controller avoids this
   by computing batch labels directly (though it currently does so incorrectly with the
   hardcoded `return 5`).

4. **Worker utilization:** Airflow workers are long-lived CeleryExecutor pods that hold capacity
   regardless of whether docking jobs are running. Kubernetes Jobs release node capacity
   immediately on completion, improving overall cluster utilization.

**What was not solved by the migration:** The fundamental I/O architecture is identical —
the shared PVC is still the only inter-step data channel. Kubernetes Jobs do not inherently
improve the storage I/O bottleneck; they merely remove the Airflow scheduling overhead.

---

## Known Bottlenecks (Confirmed by Code)

| Bottleneck | Location | Severity | Status |
|---|---|---|---|
| Hardcoded batch count of 5 | `main.go:365` | Critical | Bug — ligands beyond batch 4 are silently dropped |
| Sequential ligand processing per pod | `dockingv2.py` (both variants) | High | GNU `parallel` installed but unused |
| Sequential ligand preparation per pod | `ligandprepv2.py` (both variants) | High | No subprocess parallelism |
| No resource requests/limits on job pods | All job templates | High | Scheduler blind; OOM risk |
| Longhorn RWX for concurrent I/O | `pvc-autodock` PVC | Medium | NFS overhead; 3x write amplification |
| No result file index or aggregation DB | `3_post_processing.sh` | Medium | grep over potentially millions of files |
| Controller reconcile loop is a no-op | `main.go:516` | Medium | No recovery from mid-workflow failures |
| `waitForJobCompletion` for split-sdf only | `main.go:361` | Medium | No pod-completion waiting between prepare-ligands and docking within a batch |
| No-timeout HTTP server | `main.go:172` | Low | Theoretical DoS vector |
| `imagePullPolicy: Always` on all job pods | All job templates | Low | Image pull on every pod start; unnecessary latency |
| PDB download at runtime (wget) | `proteinprepv2.py:26` | Low | Network dependency in hot path; no retry beyond exit() |

---

## Performance-Critical Paths

### Critical Path 1: Docking Throughput

```
split-sdf completion
    -> [parallel] batch0 prepare-ligands -> batch0 docking
                  batch1 prepare-ligands -> batch1 docking
                  ...
    -> postprocessing
```

The dominant time cost is the docking step itself. AutoDock4 with `ga_num_evals 2500000` and
`ga_run 10` (10 LGA runs per ligand) is computationally expensive. For a single ligand,
AutoDock4 can take 30 seconds to several minutes depending on ligand complexity and available
CPU. At 10,000 ligands per batch, 5 batches, with sequential per-pod execution:

**Rough lower bound:** 10,000 ligands/batch × 30 sec/ligand × 5 batches (sequential within pod)
÷ 5 parallel pods = 300,000 seconds ≈ 83 hours per screen (optimistic).

This is without accounting for I/O latency, pod scheduling overhead, or the fact that
prepare-ligands must complete before docking can start within each batch.

AutoDock Vina (`dockingv2.py` in the vina variant) is significantly faster than AutoDock4 —
typically 100–1000x for equivalent accuracy — but the same sequential execution model applies.

### Critical Path 2: SDF Split I/O

`split_sdf.sh` uses a single `awk` process to scan the entire SDF file and write N batch files.
For a multi-gigabyte SDF, this is a sequential I/O-bound operation over Longhorn RWX. Batch
processing cannot begin until this completes (`waitForJobCompletion` is the only correctly-
implemented wait gate in the controller).

### Critical Path 3: Ligand Preparation I/O

`prepare_ligand4` (MGLTools) is called via `subprocess.run` once per ligand. Each call
invokes a Python process, parses the input PDB, and writes a PDBQT file. At 10,000 ligands,
this is 10,000 subprocess invocations per batch pod with no parallelism. The overhead of
process spawning and PDB I/O through Longhorn RWX is non-trivial.

---

## Gaps: What Is Not Measured or Benchmarked

The codebase contains no benchmarking infrastructure, no performance tests, and no timing
instrumentation of any kind. The following are unknown:

- Actual throughput of the cluster (ligands/hour) for either AutoDock4 or Vina
- Longhorn RWX throughput under concurrent job load (IOPS, latency, bandwidth)
- Time breakdown by pipeline stage (no timing logged, no metrics emitted)
- Memory high-water mark for prepare-ligands or docking pods
- Effect of `ChEBI_complete` SDF size on split-sdf wall time
- Scheduling latency between job creation and pod start (no pod start timestamps logged)
- I/O wait fraction vs. CPU fraction in docking pods

There is no Prometheus, Grafana, or any monitoring stack deployed in the repo.

---

## Scaling Constraints

### Cluster Ceiling (Horizontal)

The cluster has 4 schedulable worker nodes. Scaling beyond 5 batches requires either:
1. More nodes (hardware constraint), or
2. Accepting that additional batches will queue behind the 4-node capacity.

The current hardcoded batch count of 5 is below the theoretical 4-node ceiling only because
each batch requires two sequential pods (prepare-ligands then docking), so the effective
concurrency at any given moment is at most 4 (one pod per node).

### Storage Ceiling

Longhorn RWX does not scale linearly with concurrent writers. Adding more batch pods increases
contention on the shared volume. The `pvc-autodock` PVC is 20Gi — this may be insufficient for
large screens where result files (`.dlg`, `.pdbqt`) accumulate to tens or hundreds of GB.

### Controller Ceiling (Vertical)

The controller is single-replica (`replicas: 1`). It maintains no persistent state — an active
workflow's goroutine is lost on pod restart. There is no horizontal scaling path for the
controller without implementing persistent state (e.g., CRD status subresource, etcd via the
existing K8s API).

---

## Comparison: Airflow vs. K8s Controller (Performance)

| Dimension | Airflow (legacy) | K8s Controller (current) |
|---|---|---|
| Orchestrator overhead | Scheduler + 4 workers + PostgreSQL running continuously | Single 100m/128Mi controller pod |
| Batch count accuracy | Dynamic via XCom (correct, but fragile) | Hardcoded to 5 (incorrect) |
| Batch parallelism | `docking.expand()` — true dynamic fan-out | Sequential job creation, parallel by K8s scheduler |
| State persistence | Airflow DB (PostgreSQL) | None — goroutine only, lost on restart |
| Failure recovery | Airflow retry policies (3 retries, 5 min delay on most tasks) | No retry logic in controller |
| Intra-batch parallelism | None (same sequential Python loop) | None (same sequential Python loop) |
| Storage I/O | Shared Longhorn PVC (identical) | Shared Longhorn PVC (identical) |
| Resource limits on jobs | None defined in Airflow DAG | None defined in job templates |

The migration reduced persistent infrastructure overhead but did not improve the computational
throughput, storage I/O architecture, or resource management of the workflow itself.

---

## Maturity Assessment

The workflow is **proof-of-concept**. It can run end-to-end for small ligand databases but has
confirmed correctness bugs (hardcoded batch count), no parallelism within pods, no resource
governance, no observability, no persistence for active workflows, and no benchmarked baseline.
It is not suitable for production-scale screening of databases larger than ~50,000 ligands
without addressing the issues cataloged above.

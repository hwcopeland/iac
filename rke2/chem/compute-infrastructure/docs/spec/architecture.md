---
project: "compute-infrastructure"
maturity: "experimental"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "System architecture of the AutoDock molecular docking platform: cluster topology, workflow components, migration state, and known gaps"
owner: "@staff-engineer"
dependencies:
  - operations.md
  - security.md
---

# Architecture Specification — compute-infrastructure

## 1. System Purpose

compute-infrastructure is a computational chemistry platform operated by Khemiatic-Energistics that runs virtual ligand-screening workflows on a self-hosted bare-metal Kubernetes cluster. The core workload is molecular docking: given a protein receptor (identified by PDB ID) and a ligand database (SDF file), the system prepares both structures, splits the ligand set into parallel batches, performs AutoDock4 scoring for each batch, and aggregates results.

The platform is in active migration from Apache Airflow (legacy, still deployed) to a custom Go-based Kubernetes controller (new, also deployed). Both systems exist simultaneously in production. Neither is authoritative; the migration is incomplete.

---

## 2. Current Deployment State

The two workflow orchestration paths coexist on a single RKE2 cluster, sharing the same storage backend.

### 2.1 Cluster Topology

**Distribution:** RKE2 v1 (Rancher Kubernetes Engine 2), bare-metal Ubuntu nodes.

| Node | IP | Role |
|---|---|---|
| k8s00 | 192.168.1.105 | Control plane (tainted `NoSchedule`) |
| k8s01 | 192.168.1.106 | Worker |
| k8s02 | 192.168.1.107 | Worker |
| k8s04 | 192.168.1.248 | Worker |
| k8s05 | 192.168.1.190 | Worker |

**CNI:** Calico, IPIP mode, pod CIDR `10.43.0.0/16`, block size /26.

**Load Balancer:** MetalLB in L2 mode, IP pool `192.168.1.200–192.168.1.220`.

**Container Registry:** Private OCI registry (Zot) at `zot.hwcopeland.net`. Pull credentials sourced from Bitwarden via external-secrets.io (`ClusterSecretStore: bitwarden-login`).

### 2.2 Namespaces and Tenants

| Namespace | Contents |
|---|---|
| `chem` | docking-controller (Go), ExternalSecret for Zot pull credentials |
| `jupyterhub` | JupyterHub, Apache Airflow (Helm), Airflow PostgreSQL |
| `metallb-system` | MetalLB |
| `longhorn-system` | Longhorn storage driver |
| `kube-system` | cert-manager |

Note: Airflow and JupyterHub share the `jupyterhub` namespace. This reflects the legacy deployment and is not an intentional design choice.

### 2.3 Storage

**CSI Driver:** Longhorn (self-hosted distributed block storage).

| Storage Class | Reclaim Policy | Notes |
|---|---|---|
| `longhorn-storage` | Delete | Default workflow class |
| `longhorn-ssd` | Delete | SSD-backed variant; no disk selector currently configured in the class manifest |

Key PVCs:

| PVC | Size | Access Mode | Consumer |
|---|---|---|---|
| `pvc-autodock` | 20Gi | ReadWriteMany | All docking job containers (shared working directory) |
| `pvc-dags` | 1Gi | ReadWriteMany | Airflow scheduler + workers |
| `pvc-logs` | — | — | Airflow logs |
| `data-airflow-postgresql-0` | 10Gi | ReadWriteOnce | Airflow PostgreSQL |
| `claim-{username}` | 2Gi | ReadWriteMany | JupyterHub per-user (dynamically provisioned) |

The `pvc-autodock` PVC is the critical shared data store: ligand databases, receptor files, split SDF batches, docking results, and post-processed output all live here. There is no isolation between workflow runs at the storage layer — concurrent jobs writing to the same PVC will collide.

### 2.4 Ingress and DNS

There is no ingress controller. Services are exposed as LoadBalancer type via MetalLB:

- `jupyter.hwcopeland.net` — JupyterHub (GitHub OAuth, `Khemiatic-Energistics` org)
- `zot.hwcopeland.net` — private OCI registry
- Airflow webserver on LoadBalancer port 8080 (no DNS alias defined in-repo)

---

## 3. Workflow: The 6-Step Docking Pipeline

The molecular docking pipeline is logically identical across both the Airflow and controller implementations. The steps are:

| Step | Job Type | Image | What It Does |
|---|---|---|---|
| 1 | `copy-ligand-db` | `alpine:latest` | Copies `{ligandDb}.sdf` from the user's JupyterHub PVC into the shared autodock PVC |
| 2 | `prepare-receptor` | `hwcopeland/auto-docker:latest` | Runs `proteinprepv2.py` to download the PDB structure and prepare the receptor PDBQT file and grid center |
| 3 | `split-sdf` | `hwcopeland/auto-docker:latest` | Runs `split_sdf.sh` to divide the ligand SDF into N batches of `ligandsChunkSize` molecules each; returns batch count |
| 4 | `prepare-ligands` (per batch) | `hwcopeland/auto-docker:latest` | Runs `ligandprepv2.py` to convert each ligand batch from SDF to PDBQT format |
| 5 | `docking` (per batch) | `hwcopeland/auto-docker:latest` | Runs `dockingv2.py` — builds GPF/DPF parameter files, runs AutoGrid4 + AutoDock4 LGA, writes `.dlg` result files |
| 6 | `postprocessing` | `hwcopeland/auto-docker:latest` | Runs `3_post_processing.sh` to aggregate `.dlg` output across all batches |

Steps 4 and 5 repeat once per batch. The batch count is determined at runtime by step 3 (the SDF split). Steps 4 and 5 together form the parallelizable inner loop.

The default parameters baked into both systems:

| Parameter | Default |
|---|---|
| PDB ID | `7jrn` |
| Ligand DB | `ChEBI_complete` |
| Native Ligand (for grid center) | `TTT` |
| Ligands Chunk Size | `10000` |
| Compute Image | `hwcopeland/auto-docker:latest` |
| Autodock PVC | `pvc-autodock` |
| User PVC Prefix | `claim-` |
| Mount Path | `/data` |

---

## 4. Legacy Path: Apache Airflow DAG

**Location:** `rke2/airflow/dags/autodock4.py`

**How it works:** A single Airflow DAG (`autodock`) uses `KubernetesPodOperator` for each workflow step. The DAG is parameterized — users trigger it via the Airflow UI or API with `pdbid`, `ligand_db`, `jupyter_user`, `native_ligand`, and `ligands_chunk_size`.

**Batch parallelism:** After `split_sdf` completes, the batch count is returned via XCom (`do_xcom_push=True`). A Python `@task` function (`get_batch_labels`) generates the list of batch label strings. The `docking` task group is then `.expand()`-ed over the label list — Airflow dynamic task mapping.

**Infrastructure:** Airflow is deployed via Helm chart to the `jupyterhub` namespace. Four worker replicas. DAGs are git-synced from `https://github.com/hwcopeland/airflow-dags` (60s interval). PostgreSQL is co-deployed in the same namespace.

**Known issue in the DAG:** The `perform_docking` step invokes `dockingv2.sh` but the container command in the DAG is `['python3','/autodock/scripts/dockingv2.sh']` — this is a bug (calling python3 on a shell script). The Vina Dockerfile and the controller both call this correctly as a shell command. This bug exists in the current Airflow DAG.

**Xcom limitation:** The Airflow path depends on XCom for the batch count returned by `split_sdf`. The controller path hardcodes batch count to `5` (see §5.2 gap below).

---

## 5. New Path: Go Docking Controller

**Location:** `k8s-jobs/controller/`

The docking-controller is a Go binary (Go 1.21) deployed as a single-replica `Deployment` in the `chem` namespace. It serves two functions simultaneously: a REST API server (port 8080) and a periodic reconciliation loop (5-second ticker).

### 5.1 Controller Components

```
k8s-jobs/
├── controller/
│   ├── main.go          — DockingJobController struct, reconcile loop, job creation functions
│   ├── handlers.go      — APIHandler, HTTP route handlers
│   ├── Dockerfile       — multi-stage Go build on golang:1.21-alpine, runtime on alpine:latest
│   └── go.mod           — k8s.io/api v0.28.0, k8s.io/client-go v0.28.0
├── crd/
│   └── dockingjob-crd.yaml   — DockingJob CRD (group: docking.khemia.io, version: v1)
├── config/
│   ├── deployment.yaml   — Deployment + ClusterIP Service in chem namespace
│   └── rbac.yaml         — ServiceAccount + Role + RoleBinding (namespace-scoped)
├── jobs/
│   ├── job-templates.yaml    — Go-template-style standalone Job manifests (not applied by the controller)
│   └── 01-copy-ligand-db.yaml — Single-job example manifest
├── zot-pull-secret.yaml   — ExternalSecret for Zot registry credentials
└── kustomization.yaml     — Kustomize root: CRD + RBAC + deployment + pull secret
```

### 5.2 Controller Architecture: What It Actually Does

The controller does **not** watch or list `DockingJob` custom resources. It does not use a watch/informer. The `reconcileJobs()` method is a no-op stub that returns `nil` immediately. The CRD exists and is registered in Kubernetes, but the controller never reads it.

**How jobs are triggered:** Exclusively via the REST API. A `POST /api/v1/dockingjobs` call decodes the request, constructs a `DockingJob` struct in-memory (not persisted to Kubernetes), and calls `go h.controller.processDockingJob(job)` as a goroutine.

**Job execution flow:** `processDockingJob` creates Kubernetes batch/v1 Jobs directly using `client-go`. The sequence is:

1. `createCopyLigandDbJob` — creates the Job, returns immediately (does not wait for completion)
2. `createPrepareReceptorJob` — creates the Job, returns immediately
3. `createSplitSdfJob` — creates the Job, waits for completion (polls every 5 seconds, 10-minute timeout), returns **hardcoded batch count of 5** regardless of actual SDF file size
4. For each of 5 batches: `createPrepareLigandsJob`, then `createDockingJobExecution` (both fire-and-forget, no completion wait)
5. `createPostProcessingJob` — created after all batch iterations, no wait

**Critical gap:** Steps 1, 2, 4, and 5 are not waited on before the next step proceeds. The controller submits all Jobs but does not gate execution on prior step completion — the actual sequencing relies on the underlying docking scripts referencing files that must have been written by prior steps. If a prior step is slow, subsequent steps will fail at the script level, not be retried automatically, and the controller will not reflect this in any status update.

**Status is in-memory only:** `DockingJobStatus` (phase, batchCount, completedBatches, etc.) is maintained on the local Go struct, but is never written back to the Kubernetes API. There is no `kubectl get dockingjob` status visible to operators. The `GetJob` handler derives status at read-time by listing child Jobs with the `docking.khemia.io/parent-job` label selector.

**Job labels:** All child Jobs are labeled with three keys:
- `docking.khemia.io/workflow: {job-name}`
- `docking.khemia.io/job-type: {step-type}`
- `docking.khemia.io/parent-job: {job-name}`
Batch jobs additionally carry `docking.khemia.io/batch: {batch-label}`.

**TTL:** All Jobs set `ttlSecondsAfterFinished: 300` — they are garbage-collected 5 minutes after completion. This means `GetJob` status will show zero child jobs once they expire.

### 5.3 REST API

The controller exposes HTTP on `:8080` (ClusterIP service maps port 80 to 8080):

| Method | Path | Handler | Notes |
|---|---|---|---|
| `GET` | `/health` | `HealthCheck` | Always returns `{"status":"healthy"}` |
| `GET` | `/readyz` | `ReadinessCheck` | Always returns `{"status":"ready"}` |
| `GET` | `/api/v1/dockingjobs` | `ListJobs` | Lists workflows grouped by parent-job label |
| `POST` | `/api/v1/dockingjobs` | `CreateJob` | Starts a new docking workflow |
| `GET` | `/api/v1/dockingjobs/{name}` | `GetJob` | Returns derived status from child job list |
| `DELETE` | `/api/v1/dockingjobs/{name}` | `DeleteJob` | Deletes all child jobs by label selector |
| `GET` | `/api/v1/dockingjobs/{name}/logs` | `GetLogs` | Returns logs from first pod of matching job |

No authentication or authorization is implemented on these endpoints. Access control relies solely on Kubernetes network policy (which is not currently configured for the `chem` namespace).

### 5.4 RBAC

The controller runs as the `docking-controller` ServiceAccount in the `chem` namespace with a namespace-scoped Role that grants:
- `batch/jobs`: get, list, create, delete, watch, update, patch
- `core/pods`, `core/pods/log`, `core/configmaps`: get, list, watch
- `core/services`: get, list, create, delete

The Role does **not** include permissions for the `docking.khemia.io` custom resource group. This is consistent with the controller never reading `DockingJob` CRs in practice.

### 5.5 The CRD and Why It Exists

The `DockingJob` CRD (`dockingjobs.docking.khemia.io`) is registered in the cluster via the Kustomization. Its presence allows `kubectl get dockingjobs` for operator visibility and documents the desired declarative API surface. However, the controller does not use a watch/informer to act on DockingJob objects created via `kubectl apply`. The CRD serves as a schema placeholder and an aspirational API — the reconciliation loop to make it functional is not yet implemented. Creating a `DockingJob` CR via kubectl will not trigger any controller action.

---

## 6. Compute Images

Two Docker images implement the docking science:

| Image | Dockerfile | Docking Engine |
|---|---|---|
| `hwcopeland/auto-docker:latest` (autodock4) | `docker/autodock4/Dockerfile` | AutoDock4 + AutoGrid4 binary (Scripps v4.2.6) |
| Autodock Vina variant | `docker/autodock-vina/Dockerfile` | AutoDock Vina 1.1.2 binary |

Both images are built from `ubuntu:20.04` (multi-stage, builder + runtime), install `openbabel`, `python3`, and `AutoDockTools_py3` via pip, and bundle the docking scripts at `/autodock/scripts/`. Only the AutoDock4 image is referenced by the active workflow defaults. The Vina image is built but not referenced in any current job definition.

Script inventory (both images share the same set):
- `proteinprepv2.py` — receptor preparation (downloads PDB, writes PDBQT + grid_center.txt)
- `ligandprepv2.py` — ligand preparation (SDF batch → PDBQT files)
- `dockingv2.py` — AutoDock4 docking run (GPF/DPF generation, autogrid4 + autodock4 execution)
- `split_sdf.sh` — SDF file splitter (awk-based, outputs batch count to stdout)
- `3_post_processing.sh` — result aggregation

---

## 7. Infrastructure Provisioning (IaC)

Cluster provisioning and application deployment are managed by Ansible. Playbooks are in `iac/ansible/playbooks/`:

| Playbook | Purpose |
|---|---|
| `kubernetes.yml` | RKE2 server + agent installation, Calico IPPool, Helm install |
| `autodocker.yml` | Application deployment (Airflow, JupyterHub) |
| `backend.yaml`, `jupyterhub.yaml` | Service-specific deployment tasks |

Roles:
- `rke2-server` — installs RKE2 server, taints control plane, installs Helm
- `rke2-agent` — joins agents to the server (token in defaults file — see §9)
- `rke2-common` — shared RKE2 configuration
- `backend` — backend service tasks
- `frontend` — frontend/JupyterHub tasks

There is no CI/CD pipeline. No automated build, test, image push, or Kubernetes deployment pipeline exists anywhere in the repository.

---

## 8. JupyterHub Integration

JupyterHub provides the user-facing data science interface. It is deployed via Helm to the `jupyterhub` namespace with:
- GitHub OAuth authentication (restricted to `Khemiatic-Energistics` org)
- Custom image `hwcopeland/jupyter-chem:latest`
- Per-user persistent storage: `claim-{username}{servername}` PVCs (2Gi, `longhorn-storage`, ReadWriteMany)
- The per-user PVC naming convention is the source of the `claim-` prefix that the docking workflow uses in step 1 to locate a user's ligand SDF files

The relationship: users store their ligand SDF files in their JupyterHub home directories (on their `claim-{username}` PVC). The docking workflow step 1 copies those files to the shared `pvc-autodock` before processing begins. This creates a dependency: the docking workflow must be parameterized with the correct `jupyter_user` to find the ligand database.

---

## 9. Known Gaps and Honest Assessment

This section documents what is missing or broken in the current system.

### 9.1 Controller Correctness Gaps

| Gap | Location | Impact |
|---|---|---|
| `reconcileJobs()` is a no-op stub | `main.go:515-517` | The 5-second reconcile loop does nothing — the CRD is not functional |
| Batch count hardcoded to `5` | `main.go:365` | Workflows with ligand DBs that split into != 5 batches will have wrong batch counts |
| Steps 1, 2, 4, 5 not awaited | `processDockingJob()` | Race conditions: subsequent steps start before prior steps complete |
| Status never written to Kubernetes | `processDockingJob()` | `kubectl get dockingjob` shows no status; in-memory state is lost on controller restart |
| Job TTL of 5 minutes | All job functions | Status derived from child jobs becomes unavailable after 5 minutes |
| No error recovery / retry | `processDockingJob()` | A failed step aborts the goroutine with no retry or cleanup |

### 9.2 Security Gaps

| Gap | Location | Impact |
|---|---|---|
| RKE2 join token committed in plaintext | `iac/ansible/roles/rke2-agent/defaults/main.yaml`, `iac/ansible/inventory/hwc_prox.yaml` | Anyone with repo access can join nodes to the cluster |
| No authentication on controller REST API | `main.go:startAPIServer()` | Any pod in the cluster can create/delete docking jobs |
| No network policy in `chem` namespace | Missing | Lateral movement risk from compromised workload containers |
| Unpinned base image tags | All Dockerfiles | Supply chain risk on `alpine:latest`, `ubuntu:20.04`, `golang:1.21-alpine` |
| Docking job containers have no resource limits | All job templates | Node resource exhaustion risk |

### 9.3 Missing Infrastructure

- No monitoring stack (Prometheus, Grafana, alerting) — no dashboards, no alerts
- No CI/CD pipeline — no automated build, test, scan, or deploy
- No ingress controller — raw LoadBalancer services, no TLS termination for internal services
- No backup policy for Longhorn volumes (`reclaimPolicy: Delete` on both storage classes)
- No pod disruption budgets
- No horizontal pod autoscaling for docking jobs

### 9.4 Inconsistencies

- Ansible `group_vars/k8s_hosts/all.yaml` references `../../rke2/airlfow/values.yaml` (typo: "airlfow") — path is broken
- `k8s-jobs/README.md` references `api/handlers.go` but the file was moved to `handlers.go` at the controller root (the `api/` directory and the old path are deleted per git status)
- Job template files under `k8s-jobs/jobs/` use Go template syntax (`{{ .WorkflowName }}`) but are not processed by any tooling in the current controller implementation — they are documentation artifacts, not applied manifests
- Label domain inconsistency: job templates in `k8s-jobs/jobs/` use `docking.k8s.io/` labels, but the controller code uses `docking.khemia.io/` labels — these will not match if the templates are ever used directly

---

## 10. Component Interaction Diagram

```
User (JupyterHub)
    |
    | stores {ligandDb}.sdf in ~/
    |
    v
claim-{username} PVC (2Gi, longhorn-storage)
    |
    | Step 1: cp
    |
    v
pvc-autodock (20Gi, longhorn-storage, RWX)
    |
    +-- [Step 2] prepare-receptor     (writes {pdbid}.pdbqt, grid_center.txt)
    +-- [Step 3] split-sdf            (writes {ligandDb}_batch{N}.sdf files)
    +-- [Step 4] prepare-ligands x N  (writes {batchLabel}/*.pdbqt)
    +-- [Step 5] docking x N          (writes {batchLabel}/docked/*.dlg)
    +-- [Step 6] postprocessing       (aggregates .dlg results)

Orchestration (Legacy):
  Airflow DAG (jupyterhub namespace)
    -> KubernetesPodOperator per step
    -> XCom for batch count

Orchestration (New):
  docking-controller (chem namespace)
    -> REST API: POST /api/v1/dockingjobs
    -> direct client-go Job creation
    -> in-memory state (not persisted)
```

---

## 11. Migration State Summary

The Airflow-to-controller migration has completed the scaffolding phase:
- The CRD, controller binary, RBAC, and Deployment manifest are all present and deployable
- The REST API for job creation and status is functional
- The core workflow steps are implemented in the controller

What remains to complete the migration:
1. Fix the hardcoded batch count (read actual split output from the completed Job)
2. Implement proper step sequencing (wait for each step before starting the next)
3. Implement `reconcileJobs()` to watch/list `DockingJob` CRs and reconcile their state
4. Write controller status back to the `DockingJob` CR via the Kubernetes API (requires adding `docking.khemia.io` group permissions to the RBAC Role)
5. Validate the workflow end-to-end in the new path before decommissioning Airflow

Until these are addressed, the new controller should be treated as a parallel development path, not a production replacement. Airflow remains the only fully functional (if imperfect) orchestration path.

# Project Briefing: compute-infrastructure

> Generated: 2026-03-22 | Role: Developer + Operator | Scope: Full overview

---

## What Is This?

**compute-infrastructure** is a self-hosted computational chemistry platform for running
high-throughput virtual drug screening via molecular docking (AutoDock4 / AutoDock Vina).
Researchers at **Khemiatic-Energistics** submit docking jobs through a REST API or JupyterHub
notebooks; a custom Go controller orchestrates a 6-step pipeline of Kubernetes `batch/v1`
Jobs that processes ligand databases against a target protein receptor on a bare-metal RKE2
cluster.

The project is actively **migrating from Apache Airflow** (legacy, still deployed) **to a
Kubernetes-native controller** (`k8s-jobs/`). Both systems are currently live.

---

## Quick Start

### Prerequisites

- Go 1.21+
- Docker
- `kubectl` configured to reach the cluster (`192.168.1.105`, RKE2)
- `helm` v3

### Build & Deploy the Controller

```bash
# Build the Go binary locally
cd k8s-jobs/controller
go build -o docking-controller .

# Build and push the container image
cd k8s-jobs
docker build -t zot.hwcopeland.net/chem/docking-controller:latest -f controller/Dockerfile .
docker push zot.hwcopeland.net/chem/docking-controller:latest

# Deploy to Kubernetes via Kustomize
kubectl apply -k k8s-jobs/
```

### Run Locally Against the Cluster

```bash
cd k8s-jobs/controller
KUBECONFIG=~/.kube/config NAMESPACE=chem go run .
# API is available at http://localhost:8080
```

### Submit a Docking Job

```bash
curl -X POST http://docking-controller:80/api/v1/dockingjobs \
  -H "Content-Type: application/json" \
  -d '{"pdbid":"7jrn","ligand_db":"ChEBI_complete","jupyter_user":"jovyan"}'
```

### Re-deploy Airflow or JupyterHub (Legacy)

```bash
cd rke2/airflow  && helm upgrade --install airflow apache-airflow/airflow -n jupyterhub -f values.yaml
cd rke2/jupyter  && helm upgrade --install jupyterhub jupyterhub/jupyterhub --namespace jupyterhub -f values.yaml
```

### Provision Bare Metal (Ansible)

```bash
cd iac/ansible
ansible-playbook playbooks/kubernetes.yml   # RKE2 cluster
ansible-playbook playbooks/jupyterhub.yaml  # JupyterHub + Airflow
```

---

## Architecture at a Glance

```
Researcher
    │  (browser / JupyterHub notebook)
    ▼
JupyterHub  ──────────────────────────────────────────────────┐
(jupyterhub ns, GitHub OAuth, claim-{user} PVC 2Gi RWX)      │
                                                               │ REST API
    │ POST /api/v1/dockingjobs                                 │
    ▼                                                          │
Docking Controller (chem ns, ClusterIP :80→:8080)             │
  Go HTTP API + reconcile loop (5s tick, currently no-op)     │
    │                                                          │
    │  creates batch/v1 Jobs sequentially                      │
    ▼                                                          │
  [1] copy-ligand-db     copies from user PVC → pvc-autodock  │
  [2] prepare-receptor   downloads PDB from RCSB, preps .pdbqt│
  [3] split-sdf          splits ligand SDF into N batches      │
  [4] prepare-ligands    (×N) converts ligands to .pdbqt       │
  [5] docking            (×N) runs AutoDock4 / Vina per batch  │
  [6] postprocessing     collects rankings, outputs results     │
    │                                                          │
    └──── all share: pvc-autodock (20Gi RWX, Longhorn) ───────┘

Cluster: RKE2 bare-metal (1 server + 4 agents, 192.168.1.x)
Storage: Longhorn (distributed block, 2 classes: longhorn-storage / longhorn-ssd)
LB:      MetalLB L2, pool 192.168.1.200-220
Secrets: Bitwarden → External Secrets Operator → K8s Secrets
Registry: zot.hwcopeland.net (private OCI, pull secret via ExternalSecret)
```

---

## Module Map

| Directory | Purpose |
|---|---|
| `k8s-jobs/` | **Active work.** Kubernetes-native controller + CRD for docking workflows |
| `k8s-jobs/controller/` | Go controller: HTTP API + job orchestration (`main.go`, `handlers.go`) |
| `k8s-jobs/crd/` | `DockingJob` CRD definition (`docking.khemia.io/v1`) |
| `k8s-jobs/config/` | Deployment manifest + RBAC for the controller |
| `k8s-jobs/jobs/` | Job template references (not rendered; informational) |
| `docker/autodock4/` | AutoDock4 worker container image + Python pipeline scripts |
| `docker/autodock-vina/` | AutoDock Vina worker container image (alternative engine) |
| `rke2/` | Helm values + upgrade scripts for cluster services |
| `rke2/airflow/` | **Legacy.** Apache Airflow deployment (being replaced) |
| `rke2/jupyter/` | JupyterHub deployment + custom chemistry notebook image |
| `rke2/longhorn/` | Longhorn storage configuration |
| `rke2/metallb/` | MetalLB L2 load balancer configuration |
| `iac/ansible/` | Bare-metal provisioning (RKE2 install, cluster bootstrap) |
| `docs/iac/` | Ansible architecture diagram (PlantUML, WIP) |
| `test/containers/` | Vestigial Airflow smoke-test scaffold (not a test suite) |

---

## Key Files

| File | Why Read It First |
|---|---|
| `k8s-jobs/controller/main.go` | Controller core: `DockingJobController` struct, all 6 job-creation methods, reconcile loop, `main()`. The entire orchestration logic lives here. |
| `k8s-jobs/controller/handlers.go` | HTTP API handlers: `CreateJob` fires `processDockingJob` as a goroutine (fire-and-forget 202). Status and log retrieval live here. |
| `k8s-jobs/crd/dockingjob-crd.yaml` | CRD schema, field defaults, and status phase enum. Defines the contract for `DockingJob` resources. |
| `docker/autodock4/scripts/dockingv2.py` | The scientific core: reads grid coordinates, iterates ligands, calls AutoDock4/Vina per-ligand. |
| `rke2/airflow/dags/autodock4.py` | **Read to understand the workflow.** The Airflow DAG this controller mirrors. Explains why steps are ordered as they are and how XCom passed batch counts (which the controller has not yet implemented). |
| `k8s-jobs/config/rbac.yaml` | ServiceAccount + namespaced Role (not ClusterRole) for the controller. Shows what Kubernetes permissions the controller has. |
| `k8s-jobs/kustomization.yaml` | Kustomize entry point: CRD + RBAC + Deployment + pull secret. The one command that deploys everything. |
| `iac/ansible/playbooks/kubernetes.yml` | Full cluster provisioning playbook. Read before touching any node config. |

---

## Development Workflow

### Controller (Go)

```bash
# All Go code is package main — two files only
k8s-jobs/controller/main.go      # Controller logic
k8s-jobs/controller/handlers.go  # HTTP handlers

# Build
cd k8s-jobs/controller && go build -o docking-controller .

# Run locally
KUBECONFIG=~/.kube/config NAMESPACE=chem go run .

# No linters, no formatters configured — use gofmt manually
gofmt -w .
```

### Python Scripts (inside Docker only)

Scripts in `docker/autodock4/scripts/` require external binaries only present in the Docker
image (`prepare_receptor4`, `autodock4`, `autogrid4`). They cannot be run outside Docker.

```bash
# Build the worker image
cd docker/autodock4
docker build -t hwcopeland/auto-docker:latest .
```

### Kubernetes / Kustomize

```bash
# Apply everything
kubectl apply -k k8s-jobs/

# Check controller status
kubectl get pods -n chem
kubectl logs -n chem deployment/docking-controller

# List docking jobs
kubectl get dockingjobs -n chem   # or: kubectl get dj -n chem

# Watch child batch jobs
kubectl get jobs -n chem -l docking.khemia.io/workflow=<job-name>
```

---

## Infrastructure & Deployment

**Cluster topology:**
- 1 RKE2 server node: `k8s00` / `192.168.1.105` (control-plane tainted NoSchedule)
- 4 RKE2 agent nodes: `k8s01-04` / `192.168.1.106`, `.107`, `.248`, `.190`
- CNI: Calico, IPIP mode, pod CIDR `10.43.0.0/16`

**No CI/CD pipeline exists.** All builds and deployments are manual:
- Images: `docker build` → `docker push` → `kubectl apply`
- Helm services: `upgrade.sh` scripts in each service directory
- Cluster changes: Ansible playbooks run manually

**Environments:** One — production. No dev/staging separation.

**Namespaces:**
- `chem` — docking controller and workflow jobs
- `jupyterhub` — JupyterHub + Airflow (legacy)
- `longhorn-system` — Longhorn storage
- `metallb-system` — MetalLB load balancer
- `kube-system` — cert-manager

**Storage:**
- `pvc-autodock` (20Gi RWX, Longhorn): shared PVC for all workflow steps
- `claim-{username}` (2Gi RWX, Longhorn): per-user JupyterHub notebook storage
- `longhorn-storage` StorageClass: default for most PVCs
- `longhorn-ssd` StorageClass: for SSD-backed volumes

**Secrets strategy:** Bitwarden → External Secrets Operator → Kubernetes Secrets (pull secrets). GitHub OAuth credentials managed by Ansible Vault (`iac/ansible/roles/frontend/vars/vault.yaml`).

**Monitoring:** None. No Prometheus, no Grafana, no Loki, no alerting. Observability is `kubectl logs` only.

---

## Testing

**Current coverage: 0%.** There are no test files in the codebase.

| What | Status |
|---|---|
| Go unit tests | None — no `*_test.go` files |
| Python unit tests | None — no `test_*.py` files |
| Integration tests | Vestigial (`test/containers/setup.sh`, Airflow-era only) |
| CI pipeline | Does not exist |
| Code coverage | Not measured |
| Linters | None configured |

The only automated validation is `go build` inside the controller's Dockerfile (compilation check only).

**Biggest testability blocker (Go):** `DockingJobController` embeds a concrete `*kubernetes.Clientset` — no interface abstraction, no fake client injection possible. Unit tests require a real or mocked cluster. Fixing this requires extracting a `JobInterface` or `KubernetesClientInterface` before any meaningful tests can be written.

---

## Security Posture

> **Two active credential exposures require immediate action.**

### CRITICAL — Rotate Immediately

| Finding | Location | Impact |
|---|---|---|
| RKE2 cluster join token in plaintext | `iac/ansible/inventory/hwc_prox.yaml:14`, `roles/rke2-agent/defaults/main.yaml:2` | Anyone who cloned this repo can join rogue nodes to the cluster |
| JupyterHub proxy auth token + cookie secret | `rke2/jupyter/jupyterhub_template.yaml:455-457` | Cookie secret allows forging any researcher's session |

### High Priority

| Finding | Description |
|---|---|
| No auth on controller API | Any pod in the cluster can create/delete/log jobs — no API key, no token, no mTLS |
| Shell injection via user inputs | `pdbid`, `ligand_db`, `jupyter_user` from API requests flow unsanitized into shell commands inside containers with PVC access |
| Unpinned `:latest` Docker images | Default worker image `hwcopeland/auto-docker:latest` pulled from Docker Hub — supply chain risk |

### Medium Priority

| Finding | Description |
|---|---|
| kubeconfig mode `0644` | World-readable on server node — JupyterHub container escape → cluster admin |
| No pod security context | Workflow job pods run as root; no capability drops |
| Airflow webserver without TLS | LoadBalancer on port 8080, no TLS termination — credentials traverse LAN in cleartext |

### What's Done Right

- Controller ServiceAccount uses a **namespaced Role** (not ClusterRole) — blast radius limited to `chem` namespace
- Zot pull secret managed via **ExternalSecrets + Bitwarden** — no credentials in VCS (except the two critical findings above)
- JupyterHub OAuth scoped to `read:org` on `Khemiatic-Energistics` org — minimum necessary scope
- Controller container runs as **non-root** (`appuser`, UID 1000) in a multi-stage build

---

## Known Bugs & Tech Debt

| Severity | Location | Description |
|---|---|---|
| **Bug** | `main.go:365` | `createSplitSdfJob` hardcodes `return 5, nil` — batch count is always 5 regardless of actual SDF split output |
| **Bug** | `main.go:516` | `reconcileJobs()` is a no-op stub (`return nil`) — no actual reconciliation happens |
| **Bug** | `main.go:179-231` | Steps after split-sdf don't wait for predecessor jobs — race condition between pipeline steps |
| **Bug** | `handlers.go:117` | Job names use `time.Now().Unix()` — two POSTs in the same second collide on Kubernetes job names |
| **Bug** | `handlers.go:~200` | URL routing for `/logs` uses a fragile string length check (`path[len(path)-5:] == "/logs"`) |
| **Debt** | `main.go:35` | `DockingJobFinalizer` defined but not implemented |
| **Debt** | CRD vs controller | CRD is defined but controller uses REST API, not informer/watch — CRD objects submitted directly to K8s are ignored |
| **Debt** | Static templates vs controller | `jobs/job-templates.yaml` uses label domain `docking.k8s.io/` but controller generates `docking.khemia.io/` labels |
| **Debt** | README | References `controller/api/handlers.go` (deleted), wrong API group `docking.k8s.io/v1`, and non-existent env vars `API_PORT` / `RECONCILE_INTERVAL` |
| **Debt** | Binary in VCS | `k8s-jobs/controller/docking-controller` (compiled binary) is untracked — should be `.gitignore`d |
| **Debt** | Ansible | Hard-coded IPs, `k8s_user` not auto-created, `rke2_node_token` not auto-generated (see `iac/ansible/todo.md`) |

---

## Onboarding Recommendations

### Where to Start Reading

1. `rke2/airflow/dags/autodock4.py` — read this first to understand the workflow logic (even though it's legacy)
2. `k8s-jobs/controller/main.go` — the core of the active system; pay attention to `processDockingJob()` and the 6 job-creation methods
3. `k8s-jobs/controller/handlers.go` — how the REST API maps to workflow execution
4. `k8s-jobs/crd/dockingjob-crd.yaml` — the data contract

### Before You Touch Anything

- **Rotate the RKE2 join token and JupyterHub secrets** (see Security section above) — these are live credential exposures
- Add `k8s-jobs/controller/docking-controller` to `.gitignore`

### Key Patterns to Follow

- All Kubernetes jobs get labels: `docking.khemia.io/workflow`, `docking.khemia.io/job-type`, `docking.khemia.io/batch`, `docking.khemia.io/parent-job`
- Shared state flows through `pvc-autodock` mounted at `/data` — that's how pipeline steps communicate
- Secrets come from Bitwarden via ExternalSecrets — don't put credentials in files

### High-Value Next Steps (for developer+operator)

1. **Fix the hardcoded batch count** (`main.go:365`) — this is a functional defect in every docking run
2. **Add auth middleware** to the controller API (even a static bearer token from ExternalSecrets is a meaningful improvement)
3. **Implement reconciliation** in `reconcileJobs()` — without it the CRD is cosmetic
4. **Set up a CI pipeline** — at minimum `go build` + `go vet` on push; there are currently zero automated checks
5. **Add job step ordering** — use `waitForJobCompletion` consistently, not only for split-sdf
6. **Extract `kubernetes.Interface`** in the controller to enable unit testing without a live cluster

### Documentation Gaps

- No `docs/spec/`, `docs/tdd/`, `docs/prd/`, or `docs/ux/` — no formal specs exist for any component
- No `CONTRIBUTING.md` or coding standards document
- README has multiple stale references (file paths, API groups, env vars)

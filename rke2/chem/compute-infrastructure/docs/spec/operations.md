---
project: "compute-infrastructure"
maturity: "experimental"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "Deployment procedures, environment management, observability posture, and operational runbooks for all compute-infrastructure components"
owner: "@staff-engineer"
dependencies:
  - architecture.md
  - security.md
---

# Operations

## Overview

compute-infrastructure runs a self-hosted molecular docking platform on a bare-metal RKE2
Kubernetes cluster. There is **no CI/CD pipeline** — all builds, image pushes, and deployments
are performed manually by the operator. There is **one environment** (production); there is no
dev or staging separation. Both the legacy Airflow system and the new Go controller are
currently deployed simultaneously in different namespaces.

This document describes what operational practices exist today, along with known gaps that
represent risk.

---

## Environments

| Environment | Status | Description |
|---|---|---|
| Production | Active | Single bare-metal RKE2 cluster, `192.168.1.x` subnet |
| Staging | Does not exist | No staging environment |
| Development | Does not exist | Developers run the controller locally against the prod cluster via `KUBECONFIG` |

There is no environment promotion process. All changes go directly to the production cluster.

---

## Cluster Topology

| Node | IP | Role | Notes |
|---|---|---|---|
| k8s00 | 192.168.1.105 | RKE2 server / control-plane | Tainted `NoSchedule` — no workloads |
| k8s01 | 192.168.1.106 | RKE2 agent | Workload node |
| k8s02 | 192.168.1.107 | RKE2 agent | Workload node |
| k8s04 | 192.168.1.248 | RKE2 agent | Workload node |
| k8s05 | 192.168.1.190 | RKE2 agent | Workload node |

- **CNI:** Calico, IPIP mode, pod CIDR `10.43.0.0/16`
- **Load balancer:** MetalLB L2 mode, IP pool `192.168.1.200–192.168.1.220`
- **Container registry:** Private Zot registry at `zot.hwcopeland.net`

---

## Namespaces

| Namespace | Contents | Deployment method |
|---|---|---|
| `chem` | Docking controller, `DockingJob` CRD, workflow batch jobs | Kustomize (`kubectl apply -k k8s-jobs/`) |
| `jupyterhub` | JupyterHub, Airflow (legacy), PostgreSQL for both | Helm (`upgrade.sh` scripts) |
| `longhorn-system` | Longhorn distributed storage | Helm (via Ansible backend role) |
| `metallb-system` | MetalLB load balancer | Helm (via Ansible backend role) |
| `kube-system` | cert-manager | Helm (via Ansible backend role) |

---

## Storage

| PVC | Namespace | Capacity | Access Mode | Storage Class | Purpose |
|---|---|---|---|---|---|
| `pvc-autodock` | `chem` | 20Gi | ReadWriteMany | `longhorn-storage` | Shared working directory for all 6 docking pipeline steps |
| `pvc-dags` | `jupyterhub` | 10Gi | ReadWriteMany | `longhorn-storage` | Airflow DAG files (synced from GitHub) |
| `pvc-logs` | `jupyterhub` | — | — | `longhorn-storage` | Airflow scheduler and worker logs |
| `data-airflow-postgresql-0` | `jupyterhub` | 10Gi | ReadWriteOnce | `longhorn-storage` | Airflow's PostgreSQL database |
| `claim-{username}` | `jupyterhub` | 2Gi | ReadWriteMany | `longhorn-storage` | Per-user JupyterHub notebook storage (dynamically provisioned) |
| JupyterHub hub DB | `jupyterhub` | — | ReadWriteOnce | `longhorn-storage` | JupyterHub's PostgreSQL database |

**Longhorn configuration:** 3 replicas, `reclaimPolicy: Delete`, data directory
`/mnt/longhorn-storage`. The `longhorn-ssd` StorageClass exists for SSD-backed volumes but
no PVCs are currently using it.

**No backup policy is defined.** Longhorn's snapshot/backup features are not configured in
this repository. Loss of `pvc-autodock` would destroy all active docking run data. Loss of
the PostgreSQL PVCs would destroy Airflow DAG history and JupyterHub user metadata.

---

## Deployment Procedures

### No CI/CD Pipeline

There is no `.github/workflows/` directory or any automated pipeline. No build, test, lint,
image push, or deployment step runs automatically on code change. All deployment is operator-
initiated from the command line.

The only automated validation is `go build` inside the controller's Dockerfile — compilation
is checked but `go vet`, unit tests, and integration tests do not exist.

### Path 1: Go Controller (Active — Kustomize)

This is the current primary deployment path. All controller resources live in `k8s-jobs/` and
are applied via Kustomize.

**Build and push the controller image:**

```bash
cd k8s-jobs
docker build -t zot.hwcopeland.net/chem/docking-controller:latest -f controller/Dockerfile .
docker push zot.hwcopeland.net/chem/docking-controller:latest
```

**Deploy or update the controller:**

```bash
kubectl apply -k k8s-jobs/
```

This applies, in order: the `DockingJob` CRD, RBAC (ServiceAccount + Role + RoleBinding), the
Deployment and Service, and the ExternalSecret for the Zot pull secret. The Kustomize entry
point is `k8s-jobs/kustomization.yaml`.

**Verify the deployment:**

```bash
kubectl get pods -n chem
kubectl logs -n chem deployment/docking-controller
kubectl get dockingjobs -n chem
```

**Rollback:** There is no automated rollback procedure. The operator must manually re-apply a
previous image tag or a previous manifest state. Because the controller image is always pushed
as `:latest`, there is no image history to roll back to — previous versions are not preserved.

### Path 2: Airflow / JupyterHub (Legacy — Helm)

These services are deployed by running `upgrade.sh` scripts in the respective service
directories. These scripts are single-line `helm upgrade --install` commands.

**Re-deploy or update Airflow:**

```bash
cd rke2/airflow
helm upgrade --install airflow apache-airflow/airflow -n jupyterhub -f values.yaml
```

**Re-deploy or update JupyterHub:**

```bash
cd rke2/jupyter
helm upgrade --install jupyterhub jupyterhub/jupyterhub --namespace jupyterhub -f values.yaml
```

Before running these commands, the operator must substitute the JupyterHub OAuth credentials
into `rke2/jupyter/values.yaml`. The values file contains unresolved Ansible template
variables (`{{ github_client_id }}`, `{{ github_client_secret }}`). The intended flow is
Ansible-mediated substitution using vault variables at
`iac/ansible/roles/frontend/vars/vault.yaml`, but this substitution is not performed
automatically — it requires the Ansible playbook path (see below) or manual credential
injection.

**Rollback:** Helm retains release history. Roll back with:

```bash
helm rollback airflow [REVISION] -n jupyterhub
helm rollback jupyterhub [REVISION] -n jupyterhub
```

### Path 3: Bare-Metal Provisioning (Ansible)

Ansible playbooks provision and configure the underlying nodes. This path is used when adding
nodes, re-installing RKE2, or bootstrapping a fresh cluster.

**Full cluster provisioning:**

```bash
cd iac/ansible
ansible-playbook playbooks/kubernetes.yml
```

This runs two plays: (1) installs RKE2 on all `k8s_hosts` (server then agents), and (2) runs
the `backend` role against localhost, which installs cert-manager, MetalLB, and Longhorn via
Helm.

**Prerequisites:**
- The `k8s_user` system user must exist on all nodes before running the playbook (not
  automated; documented as a known gap in `iac/ansible/todo.md`)
- `ansible-galaxy collection install community.kubernetes` must be run manually
- The `rke2_node_token` in `iac/ansible/inventory/hwc_prox.yaml` must be current (currently
  hardcoded in plaintext — a known security issue)

**JupyterHub + Airflow provisioning:**

```bash
cd iac/ansible
ansible-playbook playbooks/jupyterhub.yaml
```

Note: `AIRFLOW_CONFIG` in `iac/ansible/inventory/group_vars/k8s_hosts/all.yaml` contains a
typo (`airlfow` instead of `airflow`) — the path is currently broken.

### Path 4: Docker Image Builds (Worker Images)

The AutoDock worker images (`docker/autodock4/`, `docker/autodock-vina/`) are built and pushed
manually:

```bash
cd docker/autodock4
docker build -t hwcopeland/auto-docker:latest .
docker push hwcopeland/auto-docker:latest
```

These images are pushed to Docker Hub under the `hwcopeland` account, not the private Zot
registry. The controller's default worker image constant (`DefaultImage`) references
`hwcopeland/auto-docker:latest` from Docker Hub.

---

## Secrets Management

Secrets follow two distinct patterns:

**Pattern A — ExternalSecrets + Bitwarden (correct path):**
The Zot registry pull secret in the `chem` namespace is managed via an `ExternalSecret`
resource that references a `ClusterSecretStore` named `bitwarden-login`. Credentials are
fetched from a Bitwarden item UUID and rotated on a 1-hour refresh interval. The resulting
Kubernetes secret is `kubernetes.io/dockerconfigjson` type.

**Pattern B — Plaintext in version control (known security debt):**
Two credentials are committed in plaintext:
- RKE2 cluster join token: in `iac/ansible/inventory/hwc_prox.yaml` and
  `iac/ansible/roles/rke2-agent/defaults/main.yaml`
- JupyterHub proxy token and cookie secret: in `rke2/jupyter/jupyterhub_template.yaml`
  (template file, not active values)

JupyterHub GitHub OAuth credentials are injected via Ansible Vault at deploy time from
`iac/ansible/roles/frontend/vars/vault.yaml` — these are not in the repo.

**Credential rotation procedure:** No documented procedure exists. Rotation requires:
1. Generating new values
2. Updating Bitwarden (for Zot credentials) or Ansible Vault (for GitHub OAuth)
3. Re-running the relevant playbook or Helm upgrade
4. For the RKE2 join token: draining all agents, regenerating the token on the server,
   updating the inventory file, and rejoining agents

---

## Observability

**Current state: zero monitoring infrastructure.**

There is no Prometheus, Grafana, Loki, Alertmanager, or any other observability stack deployed
or defined in this repository. The only way to inspect system state is `kubectl` commands run
manually against the cluster.

### What Is Available

| Signal | How to Access |
|---|---|
| Controller health | `GET /health` on the controller API (returns `{"status":"healthy"}`) |
| Controller readiness | `GET /readyz` (returns `{"status":"ready"}`) |
| Controller pod logs | `kubectl logs -n chem deployment/docking-controller` |
| Docking workflow step logs | `kubectl logs job/<job-name> -n chem` |
| Workflow step logs via API | `GET /api/v1/dockingjobs/{name}/logs[?task=<type>]` |
| Kubernetes job status | `kubectl get jobs -n chem -l docking.khemia.io/workflow=<name>` |
| DockingJob CRD status | `kubectl get dockingjobs -n chem` (shows Phase, PDB ID, Ligand DB, Age) |
| Longhorn UI | Available via LoadBalancer service in `longhorn-system` |
| Airflow webserver | LoadBalancer on port 8080 in `jupyterhub` namespace |
| Node/pod resource usage | `kubectl top nodes` / `kubectl top pods` (requires metrics-server) |

### Known Observability Gaps

- No alerting of any kind — a failed docking run, OOMKilled pod, or crashed controller will
  not notify anyone
- `reconcileJobs()` in the controller is a stub (`return nil`) — no reconciliation loop runs,
  so stuck or failed jobs are never automatically detected or retried
- Job status in the controller is in-memory only — a controller restart loses all status for
  in-flight jobs
- DockingJob CRD `status` fields (`phase`, `batchCount`, `completedBatches`) are updated only
  in the local Go struct; the updates are never written back to the Kubernetes API — the CRD
  objects in etcd always show `Pending`
- No structured logging — controller logs are plaintext via Go's `log` package
- No request tracing
- Pod-level resource utilization for docking jobs (CPU/GPU/memory) is not measured

---

## Incident Response

No formal incident response runbooks exist. The following is the de facto operator procedure
derived from the codebase.

### Controller Is Down

```bash
# Check pod status
kubectl get pods -n chem
kubectl describe pod -n chem -l app.kubernetes.io/name=docking-controller

# Check logs
kubectl logs -n chem deployment/docking-controller --previous

# Check liveness probe — controller has /health endpoint with 30s initial delay, 10s period
# If pod is CrashLoopBackOff, check image pull (ExternalSecret for zot-pull-secret may have
# expired or Bitwarden may be unreachable)
kubectl get externalsecret -n chem zot-pull-secret
kubectl describe externalsecret -n chem zot-pull-secret

# Force redeploy
kubectl rollout restart deployment/docking-controller -n chem
```

### A Docking Job Is Stuck

```bash
# List all child batch jobs for the workflow
kubectl get jobs -n chem -l docking.khemia.io/workflow=<job-name>

# Check the specific failing step
kubectl describe job <step-job-name> -n chem
kubectl logs job/<step-job-name> -n chem

# Jobs with TTLSecondsAfterFinished=300 auto-delete 5 minutes after completion.
# If a job is stuck Running, inspect the pod:
kubectl get pods -n chem -l job-name=<step-job-name>
kubectl describe pod <pod-name> -n chem

# NOTE: reconcileJobs() is a no-op. There is no automatic detection or recovery.
# Manual intervention is always required for stuck jobs.

# Delete a stuck job manually
kubectl delete job <step-job-name> -n chem
```

### PVC Space Exhaustion (pvc-autodock)

The `pvc-autodock` PVC is 20Gi RWX and shared across all concurrent docking runs. Completed
job pods are auto-deleted after 5 minutes (`TTLSecondsAfterFinished: 300`), but the data
written to the PVC is not cleaned up automatically.

```bash
# Check PVC usage via a temporary pod
kubectl run -it --rm debug --image=alpine -n chem --restart=Never \
  --overrides='{"spec":{"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"pvc-autodock"}}],"containers":[{"name":"debug","image":"alpine","command":["sh"],"volumeMounts":[{"name":"data","mountPath":"/data"}]}]}}'

# Inside the pod:
df -h /data
du -sh /data/*
```

No automated cleanup exists. Operators must manually remove old run directories from the PVC.

### Node Failure

Because there is a single production environment with no staging, a node failure affects live
capacity immediately. Longhorn replicates volumes across 3 nodes; workloads will reschedule to
remaining agents. The control-plane node (`k8s00`) is tainted `NoSchedule` so it does not run
docking workloads.

Recovery: replace the failed node, re-run `ansible-playbook playbooks/kubernetes.yml` with the
new node in inventory.

### Airflow Is Down

```bash
# Check Airflow pods in jupyterhub namespace
kubectl get pods -n jupyterhub

# Re-deploy via upgrade.sh
cd rke2/airflow && bash upgrade.sh
```

Note: With the ongoing migration to the k8s-jobs controller, Airflow is legacy — new docking
runs should use the controller REST API rather than the Airflow DAG.

---

## Release Process

There is no formal release process. The project does not use versioned releases, tags, or
changelogs. The current "release" process is:

1. Developer pushes code to `main` on GitHub
2. Operator manually builds the Docker image and pushes to Zot registry
3. Operator runs `kubectl apply -k k8s-jobs/` or the relevant `upgrade.sh` script
4. Operator manually verifies the deployment by checking pod status and logs

**Controller image tagging:** All controller images are pushed as `:latest`. There is no
image versioning — rolling back requires re-building from a specific git SHA.

**No pre-deployment validation:** No `go vet`, no linting, no tests, no dry-run of manifests
before apply.

---

## Known Operational Gaps and Risks

This table describes the operational gaps identified from codebase inspection, ordered by
risk impact.

| Severity | Gap | Impact | Location |
|---|---|---|---|
| Critical | RKE2 join token in plaintext VCS | Any repo clone can join rogue nodes to the cluster | `iac/ansible/inventory/hwc_prox.yaml`, `roles/rke2-agent/defaults/main.yaml` |
| Critical | `reconcileJobs()` is a no-op | Failed/stuck docking jobs are never detected or recovered automatically | `k8s-jobs/controller/main.go:516` |
| Critical | Controller image always `:latest` | No rollback path; any bad push immediately breaks production | `k8s-jobs/controller/Dockerfile`, `k8s-jobs/config/deployment.yaml` |
| High | Zero monitoring/alerting | No visibility into failures without manual `kubectl` inspection | Entire codebase |
| High | Hardcoded batch count of 5 | Every docking run processes exactly 5 batches regardless of actual SDF split output; results are silently wrong for databases that produce a different batch count | `k8s-jobs/controller/main.go:365` |
| High | No API authentication | Any pod in the cluster can create, list, delete, or read logs from docking jobs | `k8s-jobs/controller/handlers.go` |
| High | Job steps 1 and 2 not awaited | `createCopyLigandDbJob` and `createPrepareReceptorJob` return immediately after job creation — race condition if receptor prep takes longer than ligand copy | `k8s-jobs/controller/main.go:179-231` |
| High | No backup policy | Loss of `pvc-autodock` or PostgreSQL PVCs is unrecoverable | Longhorn config |
| Medium | Single environment | Any deployment error affects live research workflows | Architecture |
| Medium | Worker image unpinned `:latest` from Docker Hub | Supply chain risk; image content changes without notice | `DefaultImage` constant, `docker/autodock4/Dockerfile` |
| Medium | Job name collision | Two API calls within the same second generate identical job names, causing a Kubernetes conflict | `k8s-jobs/controller/handlers.go:117` |
| Medium | No CI/CD | No automated build or validation gate before deployment | `.github/` absent |
| Medium | Ansible playbooks incomplete | `k8s_user` not auto-created; `rke2_node_token` hardcoded; IP addresses hardcoded | `iac/ansible/todo.md` |
| Medium | Longhorn `reclaimPolicy: Delete` | PVC deletion immediately destroys data, no grace period | `iac/ansible/roles/backend/tasks/main.yaml` |
| Low | Compiled binary in repo | `k8s-jobs/controller/docking-controller` is an untracked binary; confusing and unnecessary | Git status |
| Low | CRD status never persisted | `DockingJob` objects always show `Pending` in `kubectl get dockingjobs` because status writes are not implemented | `k8s-jobs/controller/main.go` |
| Low | `upgrade.sh` scripts are single-liners with no guards | No namespace existence check, no pre-upgrade diff, no rollback on failure | `rke2/airflow/upgrade.sh`, `rke2/jupyter/upgrade.sh` |

---

## Migration State: Airflow to K8s Controller

Both systems are currently live and serving requests simultaneously.

| Capability | Airflow (Legacy) | K8s Controller (New) |
|---|---|---|
| DAG/workflow submission | Airflow webserver UI or API | REST API `POST /api/v1/dockingjobs` |
| Step execution | `KubernetesPodOperator` | `batch/v1` Job objects created by controller |
| Batch fan-out | Airflow dynamic task mapping | `processDockingJob()` goroutine loop |
| Dynamic batch count | XCom from `split_sdf.sh` output | Hardcoded `return 5` (bug) |
| Step dependency tracking | Airflow task upstream/downstream | `waitForJobCompletion()` on split-sdf only |
| Job retries | Airflow retry config (3 retries, 5-min delay) | `restartPolicy: OnFailure` on batch pods |
| Observability | Airflow webserver UI, logs stored in `pvc-logs` | `kubectl logs` only |
| Namespace | `jupyterhub` | `chem` |

**Airflow decommission is not yet scheduled.** Until the controller's reconciliation loop is
implemented and the batch count bug is fixed, Airflow remains the more reliable path for
actual research runs.

---

## Day-2 Operations Reference

### Check Overall Cluster Health

```bash
kubectl get nodes
kubectl get pods -A | grep -v Running | grep -v Completed
kubectl top nodes
```

### List Active Docking Workflows

```bash
# Via CRD (note: phase field always shows Pending — known bug)
kubectl get dockingjobs -n chem

# Via controller API (more accurate)
curl http://docking-controller.chem.svc.cluster.local/api/v1/dockingjobs

# Via job labels
kubectl get jobs -n chem -l docking.khemia.io/parent-job
```

### Submit a New Docking Job

```bash
curl -X POST http://docking-controller:80/api/v1/dockingjobs \
  -H "Content-Type: application/json" \
  -d '{"pdbid":"7jrn","ligand_db":"ChEBI_complete","jupyter_user":"jovyan"}'
# Returns 202 Accepted with job name; processDockingJob runs as goroutine
```

### Watch a Running Job

```bash
# Watch all child jobs for a workflow
kubectl get jobs -n chem -l docking.khemia.io/workflow=<job-name> -w

# Get step logs via API
curl "http://docking-controller:80/api/v1/dockingjobs/<job-name>/logs?task=prepare-receptor"

# Get logs directly
kubectl logs job/<job-name>-split-sdf -n chem
```

### Force Delete a Stuck Workflow

```bash
# Delete all child jobs for a workflow
kubectl delete jobs -n chem -l docking.khemia.io/workflow=<job-name>
# Delete the DockingJob CRD object if one exists
kubectl delete dockingjob <job-name> -n chem
```

### Inspect Storage

```bash
# Check PVC binding status
kubectl get pvc -n chem
kubectl get pvc -n jupyterhub

# Check Longhorn volumes via its UI
# Access: check LoadBalancer IP for longhorn-system/longhorn-frontend service
kubectl get svc -n longhorn-system
```

### Rebuild and Redeploy the Controller

```bash
cd /path/to/compute-infrastructure
docker build -t zot.hwcopeland.net/chem/docking-controller:latest \
  -f k8s-jobs/controller/Dockerfile k8s-jobs/
docker push zot.hwcopeland.net/chem/docking-controller:latest
kubectl apply -k k8s-jobs/
kubectl rollout status deployment/docking-controller -n chem
```

---

## Appendix: Service Endpoints

| Service | Address | Notes |
|---|---|---|
| Docking controller API | `http://docking-controller.chem.svc.cluster.local/api/v1/dockingjobs` | ClusterIP only — not externally exposed |
| JupyterHub | `https://jupyter.hwcopeland.net` | LoadBalancer via MetalLB; GitHub OAuth |
| Airflow webserver | MetalLB IP on port 8080 | No TLS — credentials in cleartext over LAN |
| Zot container registry | `https://zot.hwcopeland.net` | Pull secret managed via ExternalSecrets |
| Longhorn UI | MetalLB IP (LoadBalancer in `longhorn-system`) | Not externally documented |

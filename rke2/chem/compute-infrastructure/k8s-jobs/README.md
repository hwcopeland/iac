# K8s Jobs Migration

This directory contains the Kubernetes-based replacement for the Airflow-based docking workflow.

## Architecture

The migration consists of:
1. **Kubernetes Jobs** - Standalone job templates that can run each step of the docking workflow
2. **Custom Resource Definition (CRD)** - Defines the `DockingJob` resource for declarative job management
3. **Controller** - A Go-based controller that manages job orchestration and exposes a REST API

### Components

```
k8s-jobs/
├── crd/
│   └── dockingjob-crd.yaml       # Custom Resource Definition
├── jobs/
│   ├── job-templates.yaml         # Reusable job templates
│   └── 01-copy-ligand-db.yaml    # Individual job manifests
├── controller/
│   ├── main.go                   # Main controller with API server
│   ├── api/handlers.go           # HTTP API handlers
│   ├── Dockerfile                # Container build file
│   └── go.mod                    # Go dependencies
└── config/
    ├── deployment.yaml           # Controller deployment & services
    └── rbac.yaml                # RBAC configuration
```

## API Endpoints

The controller exposes a REST API on port 8080:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | /health | Health check |
| GET | /api/v1/dockingjobs | List all docking workflows |
| POST | /api/v1/dockingjobs | Create a new docking job |
| GET | /api/v1/dockingjobs?name={name} | Get job status |
| DELETE | /api/v1/dockingjobs?name={name} | Delete a docking job |
| GET | /api/v1/dockingjobs/{name}/logs | Get job logs |

### Example: Create a New Docking Job

```bash
curl -X POST http://docking-controller:80/api/v1/dockingjobs \
  -H "Content-Type: application/json" \
  -d '{
    "pdbid": "7jrn",
    "ligand_db": "ChEBI_complete",
    "jupyter_user": "jovyan",
    "native_ligand": "TTT",
    "ligands_chunk_size": 10000,
    "image": "hwcopeland/auto-docker:latest"
  }'
```

### Example: Check Job Status

```bash
curl http://docking-controller:80/api/v1/dockingjobs?name=docking-1234567890
```

### Example: Get Logs

```bash
# Get logs for a specific task type
curl "http://docking-controller:80/api/v1/dockingjobs/docking-123/logs?task=prepare-receptor"

# Get all logs for a workflow
curl "http://docking-controller:80/api/v1/dockingjobs/docking-123/logs"
```

## Workflow Steps

The controller orchestrates the following steps:

1. **copy-ligand-db** - Copies ligand database from user PVC to autodock PVC
2. **prepare-receptor** - Prepares the protein receptor using AutoDock
3. **split-sdf** - Splits the SDF file into manageable batches
4. **prepare-ligands** (per batch) - Prepares ligands for docking
5. **docking** (per batch) - Performs the actual docking calculations
6. **postprocessing** - Post-processes and aggregates results

## Installation

### 1. Apply the CRD

```bash
kubectl apply -f crd/dockingjob-crd.yaml
```

### 2. Apply RBAC and Deployment

```bash
kubectl apply -f config/rbac.yaml
kubectl apply -f config/deployment.yaml
```

### 3. Build and Deploy the Controller

```bash
cd controller
docker build -t docking-controller:latest .
kubectl apply -f ../config/deployment.yaml
```

## Using the CRD (Alternative to API)

You can also create jobs using the Kubernetes API directly:

```bash
kubectl apply -f - <<EOF
apiVersion: docking.k8s.io/v1
kind: DockingJob
metadata:
  name: my-docking-job
spec:
  pdbid: "7jrn"
  ligandDb: "ChEBI_complete"
  jupyterUser: "jovyan"
  nativeLigand: "TTT"
  ligandsChunkSize: 10000
  image: "hwcopeland/auto-docker:latest"
EOF
```

## Monitoring

### Check Job Status

```bash
# List all docking jobs
kubectl get dockingjobs

# Get detailed job information
kubectl get dockingjob my-docking-job -o yaml
```

### View Logs

```bash
# View controller logs
kubectl logs -l app=docking-controller

# View specific job logs
kubectl logs job/copy-ligand-db-my-docking-job
```

## Migration from Airflow

| Airflow Concept | Kubernetes Equivalent |
|----------------|----------------------|
| DAG | DockingJob (CRD) |
| Task | Kubernetes Job |
| Task Group | Batch jobs with labels |
| XCom | Shared PVC / ConfigMap |
| Airflow API | Controller REST API |
| Scheduler | Controller reconciliation loop |

## Job Labels

All jobs are labeled for easy querying:

- `docking.k8s.io/workflow` - Parent workflow name
- `docking.k8s.io/job-type` - Type of job (copy-ligand-db, prepare-receptor, etc.)
- `docking.k8s.io/batch` - Batch label (for batch jobs)

Example queries:

```bash
# Get all jobs for a workflow
kubectl get jobs -l docking.k8s.io/workflow=my-docking-job

# Get all prepare-receptor jobs
kubectl get jobs -l docking.k8s.io/job-type=prepare-receptor
```

## Environment Variables

The controller supports the following environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| NAMESPACE | default | Kubernetes namespace to operate in |
| KUBECONFIG | /etc/kubernetes/admin.conf | Path to kubeconfig |
| API_PORT | 8080 | HTTP port for API server |
| RECONCILE_INTERVAL | 5s | Interval between reconciliation loops |

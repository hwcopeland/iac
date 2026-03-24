---
project: "compute-infrastructure"
maturity: "proof-of-concept"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "Authentication, authorization, secret management, trust boundaries, and known credential exposures across the RKE2 cluster and docking controller"
owner: "@staff-engineer"
dependencies:
  - operations.md
  - architecture.md
---

# Security Specification — compute-infrastructure

This document describes the actual security posture of the compute-infrastructure project as of
2026-03-23. It is descriptive of what exists, not aspirational. Gaps are documented plainly.

---

## 1. Active Credential Exposures (CRITICAL — Requires Immediate Rotation)

Three categories of live credentials are committed to this repository in plaintext. These are
not theoretical risks — they are currently in the git history and must be treated as
compromised until rotated and the history is addressed.

### 1.1 RKE2 Cluster Join Token

**Severity: Critical**

The RKE2 node join token is committed in plaintext in three separate files:

- `iac/ansible/inventory/hwc_prox.yaml` line 14 — `rke2_node_token` host variable
- `iac/ansible/roles/rke2-agent/defaults/main.yaml` line 2 — role default variable
- `iac/ansible/roles/rke2-agent/templates/config.yaml.j2` line 5 — rendered agent config template

Token value (compromised): `K105d9b5156d910c3b7c7e122803eab427c6699ca22973c911336c24d2522bed2b9::server:1e979178aa26ef4f379ce14fd7fd8007`

Anyone with read access to this repository can use this token to join rogue nodes to the cluster
at `192.168.1.105:9345` (hardcoded in the template). A rogue agent node has access to the cluster
network, pod-to-pod communication, and can pull secrets mounted in its scheduled pods.

**Required remediation:**
1. Rotate the token on the RKE2 server immediately (`rke2 token rotate`)
2. Remove the token from all three files and replace with Ansible Vault references
3. Rewrite git history to remove the token from all commits, or accept that the old token
   (not the new one) is permanently in history

### 1.2 JupyterHub Proxy Auth Token and Cookie Secret

**Severity: Critical**

The file `rke2/jupyter/jupyterhub_template.yaml` is a rendered Helm template (generated output,
not a template with variables) committed to the repo. It contains live Kubernetes Secret values
at lines 455-457:

- `hub.config.ConfigurableHTTPProxy.auth_token`: base64-encoded value committed in plaintext
- `hub.config.JupyterHub.cookie_secret`: base64-encoded value committed in plaintext
- `hub.config.CryptKeeper.keys`: base64-encoded value committed in plaintext

The proxy auth token controls hub-to-proxy communication. An attacker with this token can
inject arbitrary routes into the configurable-http-proxy, redirect user sessions, or intercept
JupyterHub traffic. The cookie secret, if known, enables session forgery for any JupyterHub user.

**Required remediation:**
1. Regenerate all three secrets immediately
2. The rendered template file should not exist in version control. It should be generated
   at deploy time and never committed. Either delete the file or add it to `.gitignore`.
3. Rewrite git history or accept historical exposure

### 1.3 GitHub OAuth Credentials (Partially Mitigated)

**Severity: High**

JupyterHub is configured to use GitHub OAuth via `rke2/jupyter/values.yaml`. The `client_id`
and `client_secret` fields use Ansible Jinja2 template markers (`{{ github_client_id }}`,
`{{ github_client_secret }}`), which means the raw values are NOT in this repo. They are
substituted at deploy time from an Ansible Vault file referenced as
`/home/k8s_user/compute-infrastructure/iac/ansible/roles/frontend/vars/vault.yaml`
on the control node.

This is the correct pattern — the values are not committed. However, the vault file path
implies it lives on a remote host's filesystem, not in a centrally managed secret store.
If the control node is compromised, the vault file is exposed.

---

## 2. Authentication and Authorization Architecture

### 2.1 JupyterHub — GitHub OAuth (Org-Gated)

JupyterHub (running at `https://jupyter.hwcopeland.net`) uses GitHub OAuth via
`jupyterhub-oauthenticator`. Access is restricted to members of the `Khemiatic-Energistics`
GitHub organization with `read:org` scope. This is the only user-facing authentication
boundary in the project.

Configuration: `rke2/jupyter/values.yaml`

```yaml
GitHubOAuthenticator:
  allowed_organizations:
    - Khemiatic-Energistics
  scope:
    - read:org
```

There is no MFA enforcement at the JupyterHub layer. MFA enforcement depends on GitHub
account settings.

### 2.2 Docking Controller HTTP API — NO AUTHENTICATION

**Severity: Critical**

The docking controller (`k8s-jobs/controller/`) exposes an HTTP REST API on port 8080 with
the following routes:

- `GET /api/v1/dockingjobs` — list all jobs
- `POST /api/v1/dockingjobs` — create and launch a docking job
- `GET /api/v1/dockingjobs/{name}` — get job status
- `DELETE /api/v1/dockingjobs/{name}` — delete all child jobs
- `GET /api/v1/dockingjobs/{name}/logs` — read pod logs
- `GET /health` and `GET /readyz` — liveness/readiness probes

There is zero authentication or authorization on any of these endpoints. The handler
registration in `main.go` (`startAPIServer()`) attaches no middleware for token validation,
mTLS, or even network policy restriction. The service is exposed as a ClusterIP
(`k8s-jobs/config/deployment.yaml`) but ClusterIP means every pod in the cluster can reach it
on `docking-controller.chem.svc.cluster.local:80` without any credential.

Consequences:
- Any compromised pod in the cluster (from any namespace, unless NetworkPolicy blocks it) can
  create docking jobs, consuming cluster compute
- Any pod can delete any running docking job (denial of service)
- Any pod can read pod logs, which may contain scientific data or operational details

The `chem` namespace has no NetworkPolicy defined (confirmed via codebase scan). The
controller service is therefore reachable cluster-wide.

### 2.3 Kubernetes RBAC — Controller Service Account

The controller's ServiceAccount (`docking-controller` in namespace `chem`) is defined in
`k8s-jobs/config/rbac.yaml`. The RBAC Role grants:

```
batch/jobs:        get, list, create, delete, watch, update, patch
core/pods:         get, list, watch
core/pods/log:     get, list, watch
core/configmaps:   get, list, watch
core/services:     get, list, create, delete
```

The service permissions (`create`, `delete` on Services) appear over-provisioned for the
controller's actual needs. The controller does not create Services at runtime — the manifest
files define the static Service. This `create`/`delete` on Services should be removed.

### 2.4 Airflow — No Authentication Configuration in Repo

The legacy Airflow deployment (`rke2/airflow/`) uses the official Airflow Helm chart. No
authentication configuration is present in the `rke2/airflow/values.yaml`. Airflow's
webserver is exposed as a LoadBalancer service on port 8080 with no explicit auth config
committed. This means it defaults to Airflow's built-in basic auth with the default admin
credentials, which is insecure for any internet-reachable service.

Airflow has been replaced by the K8s controller in the main migration. However, the Airflow
manifests still exist in the repo and may still be deployed on the cluster.

---

## 3. Secret Management

### 3.1 What Exists: external-secrets.io with Bitwarden

The project uses `external-secrets.io` with a `ClusterSecretStore` named `bitwarden-login` as
the intended secrets management pattern. One ExternalSecret is deployed:

**File:** `k8s-jobs/zot-pull-secret.yaml`

This ExternalSecret in namespace `chem` pulls Zot registry credentials from Bitwarden item
UUID `766ec5c7-6aa8-419d-bb27-e5982872bc5b` and materializes them as a
`kubernetes.io/dockerconfigjson` Secret named `zot-pull-secret`. It refreshes every 1 hour.

The controller deployment in `k8s-jobs/config/deployment.yaml` references this secret via
`imagePullSecrets`. This is the correct pattern and is the only use of it in this repository.

### 3.2 Ansible Vault (Partial, Undocumented)

GitHub OAuth credentials for JupyterHub are injected by Ansible using values from a vault
file at `/home/k8s_user/compute-infrastructure/iac/ansible/roles/frontend/vars/vault.yaml`.
The backend role references a vault file at
`/home/k8s_user/compute-infrastructure/iac/ansible/roles/backend/vars/r2-credentials.yml`
for Cloudflare R2 credentials (this section is currently commented out in `backend/tasks/main.yaml`).

Neither vault file is in the repository. The Ansible Vault password management process is
not documented anywhere in the codebase.

### 3.3 Secrets NOT Using a Secret Manager (Current Gaps)

| Secret | Location | Status |
|---|---|---|
| RKE2 join token | `iac/ansible/inventory/hwc_prox.yaml`, `roles/rke2-agent/defaults/main.yaml`, `templates/config.yaml.j2` | COMMITTED PLAINTEXT — rotate immediately |
| JupyterHub proxy auth token | `rke2/jupyter/jupyterhub_template.yaml` line 455 | COMMITTED PLAINTEXT — rotate immediately |
| JupyterHub cookie secret | `rke2/jupyter/jupyterhub_template.yaml` line 456 | COMMITTED PLAINTEXT — rotate immediately |
| JupyterHub CryptKeeper keys | `rke2/jupyter/jupyterhub_template.yaml` line 457 | COMMITTED PLAINTEXT — rotate immediately |
| GitHub OAuth client_id/secret | `rke2/jupyter/values.yaml` (templated) | NOT committed — managed via Ansible Vault |
| Cloudflare R2 credentials | Commented out in `iac/ansible/roles/backend/tasks/main.yaml` | Feature disabled; credentials file location undocumented |

---

## 4. Container Image Security

### 4.1 Unpinned Image Tags

All container images in the project use unpinned tags. This is a supply chain risk: a
compromised or malicious image update at the upstream registry can silently replace the
running image on the next pod restart.

| Image Reference | Where Used | Risk |
|---|---|---|
| `ubuntu:20.04` | `docker/autodock-vina/Dockerfile`, `docker/autodock4/Dockerfile` (both stages) | Minor — 20.04 tag is stable but receives unreviewed security patches |
| `golang:1.21-alpine` | `k8s-jobs/controller/Dockerfile` build stage | Supply chain risk on `1.21` minor track |
| `alpine:latest` | `k8s-jobs/controller/Dockerfile` final stage; copy-ligand-db job in `main.go` | CRITICAL — `latest` is mutable; changes silently |
| `hwcopeland/auto-docker:latest` | `main.go` constant `DefaultImage`, `rke2/airflow/dags/autodock4.py` | `latest` from Docker Hub — mutable tag on unverified public image |
| `hwcopeland/jupyter-chem:latest` | `rke2/jupyter/values.yaml` | `latest` from Docker Hub — mutable |
| `zot.hwcopeland.net/chem/docking-controller:latest` | `k8s-jobs/config/deployment.yaml` | Internal registry, but still `latest` |

The controller image (`zot.hwcopeland.net/chem/docking-controller:latest`) uses a private
registry with pull credentials managed via ExternalSecret — this is the correct approach for
internal images, but still needs a pinned tag.

### 4.2 Container User Context

The docking controller Dockerfile creates a non-root user (`appuser`, UID 1000) and runs the
binary as that user. This is correct.

The autodock compute containers (`docker/autodock4/`, `docker/autodock-vina/`) have no USER
directive and run as root. These containers execute user-controlled parameters inside shell
commands (see Section 5).

### 4.3 No Image Scanning

There is no CI/CD pipeline in this repository. No automated scanning (Trivy, Grype, Snyk) is
performed on any image build. Vulnerabilities in the base images and Python dependencies are
unknown.

---

## 5. Input Validation and Command Injection

### 5.1 Controller API — Unsanitized Inputs Flow into Shell Commands

**Severity: High**

The docking controller accepts `pdbid`, `ligand_db`, and `jupyter_user` as free-text JSON
fields from HTTP POST requests to `/api/v1/dockingjobs`. These values flow directly into shell
command arguments without any validation or sanitization.

Injection surface in `main.go`:

**`createCopyLigandDbJob`** — the `LigandDb` and `JupyterUser` values are interpolated directly
into a shell command string:

```go
fmt.Sprintf("cp %s/%s/%s.sdf %s/%s.sdf",
    job.Spec.MountPath, userPvcPath, job.Spec.LigandDb,
    job.Spec.MountPath, job.Spec.LigandDb)
```

This command runs via `/bin/sh -c`. A `LigandDb` value of `foo.sdf; rm -rf /data` would
execute as shell. The impact is bounded by what the container's filesystem access allows —
which includes the shared `pvc-autodock` PVC mounted at `/data` across all docking jobs.

**`createSplitSdfJob`** — `LigandDb` and `LigandsChunkSize` injected into a shell command:

```go
fmt.Sprintf("/autodock/scripts/split_sdf.sh %d %s",
    job.Spec.LigandsChunkSize, job.Spec.LigandDb)
```

**`createDockingJobExecution`** — `PDBID` and `batchLabel` (derived from `LigandDb`) injected:

```go
fmt.Sprintf("/autodock/scripts/dockingv2.sh %s %s",
    job.Spec.PDBID, batchLabel)
```

**`createPostProcessingJob`** — `PDBID` and `LigandDb` passed as direct container `Args`
(not via shell), which mitigates shell injection for this specific case. The subprocess
call still trusts these values as filesystem paths.

The Python compute scripts (`dockingv2.py`, `proteinprepv2.py`, `dockingv2.py` for autodock4)
all use `subprocess.run(command, shell=True, check=True)` with f-string interpolation.
`proteinprepv2.py`'s `download_protein()` passes `protein_id` directly to a shell command
via `wget`. The `pdbid` input controls which PDB file is downloaded from `rcsb.org` — a
path traversal value could attempt to write outside the working directory.

### 5.2 No Input Validation on Controller API

The `CreateJob` handler in `handlers.go` validates only that `ligand_db` is non-empty.
There is no validation of:
- `pdbid` (should match pattern `[a-zA-Z0-9]{4}`)
- `ligand_db` (should be an alphanumeric identifier)
- `jupyter_user` (should match known JupyterHub username format)
- `image` (should be from an allowlist — currently any image can be specified by the caller)

The `image` field is particularly notable: an unauthenticated caller to the API can specify
an arbitrary container image to run as a docking job on the cluster.

---

## 6. Network Security and Trust Boundaries

### 6.1 Network Topology

The cluster runs on a private LAN (`192.168.1.0/24`). Services are exposed externally via
MetalLB (L2 mode, pool `192.168.1.200-192.168.1.220`). There is no ingress controller
documented in the repo — services use LoadBalancer type directly.

### 6.2 NetworkPolicy

**JupyterHub namespace:** The rendered template (`jupyterhub_template.yaml`) includes
NetworkPolicies for the hub, proxy, and singleuser components. These are JupyterHub's
default chart-generated policies — they restrict inter-component communication appropriately.

**`chem` namespace (docking controller):** No NetworkPolicy exists for this namespace. The
docking controller Service is ClusterIP, making it accessible to any pod in the cluster
without restriction. Combined with the complete absence of authentication on the controller
API (Section 2.2), any pod in the cluster is a potential attack surface.

### 6.3 External Connectivity from Compute Containers

The `proteinprepv2.py` script makes outbound HTTP requests to `https://files.rcsb.org` to
download PDB files. The target URL is derived from the user-controlled `protein_id` parameter:

```python
url = f'https://files.rcsb.org/download/{filename}'
```

This is bounded to rcsb.org by hardcoding the base URL. However, if the container has broader
egress (no NetworkPolicy restricts it), a malicious `protein_id` containing path traversal or
a redirect manipulation could be attempted.

### 6.4 Trust Boundary Summary

| Boundary | Authentication | Authorization | Status |
|---|---|---|---|
| User → JupyterHub | GitHub OAuth (org-gated) | Organization membership | Adequate for a lab environment |
| User → Docking Controller API | None | None | CRITICAL GAP |
| Pod → Docking Controller API | None | None | CRITICAL GAP |
| Pod → Kubernetes API | ServiceAccount token (docking-controller SA) | RBAC Role in `chem` ns | Adequate but services permission over-provisioned |
| Agent node → Cluster | RKE2 join token | Token validated by server | COMPROMISED — token in git history |
| Controller → Zot Registry | dockerconfigjson via ExternalSecret | Registry credentials | Adequate |
| Compute containers → Internet | Unrestricted (no NetworkPolicy) | None | Uncontrolled |

---

## 7. Security-Relevant Dependencies

### 7.1 Kubernetes Controller Dependencies

From `k8s-jobs/controller/go.mod`: client-go and standard Kubernetes libraries. No unusual
dependencies. Go's compiler-verified type system limits some injection classes.

### 7.2 Python Scientific Stack

The autodock containers install `AutoDockTools_py3` directly from GitHub master:

```
RUN python3 -m pip install git+https://github.com/Valdes-Tresanco-MS/AutoDockTools_py3
```

This is an unpinned install from a VCS reference, not a PyPI release. There is no commit
hash pinning. Any push to that repository's default branch will be pulled on the next image
build. This is a supply chain risk.

Additional Python dependencies are in `requirements.txt` files (both autodock variants) —
these should be reviewed for pinned versions. The files exist at:
- `docker/autodock-vina/requirements.txt`
- `docker/autodock4/requirements.txt`

### 7.3 RKE2 and Helm Charts

RKE2 is installed via `curl -sfL https://get.rke2.io | sh -` in the Ansible tasks. No version
pin is specified — this installs whatever is current at run time. Similarly, Helm is installed
via `curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash`.

Neither installer is verified with a checksum or signature. This is a bootstrapping risk
acceptable for a lab environment but not for production.

---

## 8. What Does Not Exist (Security Gaps)

The following standard security controls are absent from this project:

| Control | Status |
|---|---|
| Authentication on the docking controller API | Not implemented |
| NetworkPolicy for `chem` namespace | Not defined |
| Input validation / allowlisting on controller API fields | Not implemented |
| Container image digest pinning | Not implemented — all images use mutable tags |
| CI/CD pipeline with secret scanning | No CI/CD exists at all |
| Automated vulnerability scanning (SAST, image scan) | None |
| Pod SecurityContext (non-root, read-only filesystem) for compute containers | Not set |
| Audit logging on the Kubernetes API server | Not configured in repo |
| Secret rotation policy | Not documented |
| `.gitignore` entries for rendered Helm templates and secrets | Not configured |
| Ansible Vault documentation | Not documented in repo |
| Resource limits on docking job templates (worker pods) | Not set — only controller has limits |

---

## 9. Remediation Priority

The following is ordered by impact, not effort.

1. **Rotate the RKE2 join token** — the token is live in git history. Until it is rotated,
   the cluster boundary is unenforceable.

2. **Regenerate JupyterHub secrets** (proxy auth token, cookie secret, CryptKeeper keys) —
   the current values are in git history. Users' session cookies signed with the compromised
   key can be forged.

3. **Add authentication to the docking controller API** — a bearer token middleware (verified
   against a Kubernetes Secret) is the minimal viable fix. NetworkPolicy restricting
   controller access to the JupyterHub namespace is a complementary defense.

4. **Add input validation to the controller API** — at minimum: pattern-match `pdbid` to
   `[a-zA-Z0-9]{4}`, restrict `ligand_db` to alphanumeric plus underscore/hyphen, validate
   `jupyter_user` against the known PVC naming scheme, and allowlist the `image` field.

5. **Add a NetworkPolicy for the `chem` namespace** — restrict ingress to the controller
   Service to only the JupyterHub namespace (or specific pods) until a proper auth layer is added.

6. **Pin all container image tags to digests** — especially `alpine:latest` and
   `hwcopeland/auto-docker:latest`.

7. **Remove `jupyterhub_template.yaml` from version control** or at minimum add it to
   `.gitignore` going forward. Consider using `helm template` output as an ephemeral deploy
   artifact only.

8. **Move the RKE2 token and JupyterHub secrets to Ansible Vault** or external-secrets.io,
   consistent with the Zot pull secret pattern already established.

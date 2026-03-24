---
project: "compute-infrastructure"
maturity: "draft"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "GitHub Actions CI pipeline to build and push 4 Docker images to zot.hwcopeland.net/chem/ from a GitHub-hosted runner that cannot reach the private registry directly, with Flux image automation for GitOps deployment"
owner: "@staff-engineer"
dependencies:
  - docs/spec/architecture.md
  - docs/spec/security.md
  - docs/spec/operations.md
---

# Technical Design: GitHub Actions CI — Docker Image Build and Push

## 1. Problem Statement

### 1.1 What and Why Now

The compute-infrastructure project currently has no CI/CD pipeline. All four Docker images are
built and pushed manually by the operator from a workstation with direct network access to
`zot.hwcopeland.net`. This approach has several compounding problems:

- **No rollback path**: The docking-controller image is always pushed as `:latest`. There is
  no image history to recover from a bad push (documented in `docs/spec/operations.md`).
- **No build gating**: `go vet`, linting, and tests (if they existed) are never run before
  the image lands in the registry.
- **Supply chain opacity**: No record of which git commit produced which image tag.
- **Operator dependency**: A human must be present and have network access to build and deploy.

The verified goal is a full GitOps pipeline where pushing to `main` automatically builds all
four images, pushes them to the private Zot OCI registry, and Flux handles image policy
detection and deployment. The `iac/ansible/` directory is removed from the repo as part of
this effort.

### 1.2 Critical Constraint

`zot.hwcopeland.net` is a self-hosted OCI registry on the private home network
(`192.168.1.x`). The DNS record for `zot.hwcopeland.net` resolves via Cloudflare with
`proxied: false`, meaning traffic goes directly to the home public IP without Cloudflare
proxying. GitHub Actions cloud-hosted runners run in Azure/GitHub infrastructure and
**cannot reach this registry** directly — no port-forwarding to the private registry is
assumed to be in place, and the registry is not intended to be permanently exposed on a
public port.

This network boundary is the central design problem this TDD must solve.

### 1.3 Acceptance Criteria

1. A push to the `main` branch of `Khemiatic-Energistics/compute-infrastructure` triggers a
   GitHub Actions workflow that builds all four images.
2. All four images are tagged with both a commit SHA short tag and a semver tag where
   applicable, in a format compatible with Flux `ImagePolicy` semver filtering.
3. All four images are pushed to `zot.hwcopeland.net/chem/` successfully.
4. The push can be performed without a human operator present.
5. GitHub Actions workflow secrets (registry credentials) are stored in GitHub Actions
   encrypted secrets, not in the repository.
6. Flux detects the new image tags via `ImageRepository` polling and updates the relevant
   deployment manifests in the repo via `ImageUpdateAutomation`.
7. The cluster applies the updated manifests and rolls out the new image.
8. `iac/ansible/` is removed from the repository.
9. The workflow completes successfully within 30 minutes for a cold build.

### 1.4 Scope Boundaries

**In scope:**
- GitHub Actions workflow definition for 4 image builds
- Network connectivity solution for CI runner to reach private registry
- Image tagging strategy compatible with Flux `ImagePolicy`
- GitHub Actions secrets setup for registry credentials
- Flux `ImageRepository`, `ImagePolicy`, and `ImageUpdateAutomation` resources for `chem` namespace
- Deletion of `iac/ansible/` directory

**Out of scope:**
- Implementing `go vet`, linting, or unit tests in the pipeline (no tests currently exist;
  this can be added in a follow-on issue)
- Fixing the controller's reconciliation loop or hardcoded batch count (separate work tracked
  in `docs/spec/architecture.md` §9.1)
- Flux GitRepository or Kustomization resources for the `chem` namespace (these need to be
  designed separately as part of the full Flux migration — this TDD focuses on the CI/push
  half of the GitOps pipeline)

---

## 2. Context and Prior Art

### 2.1 Existing Flux Pattern (tooling namespace)

The cluster already runs a proven Flux image automation pipeline for the `tooling` namespace.
The pattern at `/Users/hwcopeland/iac/rke2/tooling/flux/image-automation/` uses:

- `ImageRepository`: polls `zot.hwcopeland.net/<org>/<image>` every 1 minute using a
  `zot-pull-secret` (ExternalSecret from Bitwarden item `766ec5c7-6aa8-419d-bb27-e5982872bc5b`)
- `ImagePolicy`: filters tags matching `^sha-[a-f0-9]+` with alphabetical ascending order
- `ImageUpdateAutomation`: pushes `[ci skip]` commits to `main` updating the `{"$imagepolicy": "namespace:name"}` annotation in deployment YAML

The `u4u-engine` deployment shows the exact annotation format:
```yaml
image: zot.hwcopeland.net/florida-man-bioscience/u4u-engine:sha-e740625 # {"$imagepolicy": "tooling:u4u-engine"}
```

This is the exact same pattern this TDD builds on, applied to the `chem` namespace.

### 2.2 Existing Registry Auth

The Bitwarden item UUID `766ec5c7-6aa8-419d-bb27-e5982872bc5b` holds username/password
credentials that work for Docker registry `dockerconfigjson` authentication against
`zot.hwcopeland.net`. These same credentials are used in both the `chem` and `tooling`
namespaces for image pulls. The same credentials will be used for CI image pushes.

The Zot registry uses OIDC (Authentik at `auth.hwcopeland.net`) as its primary auth
mechanism but also supports username/password (htpasswd-style) authentication via the same
credential pair stored in Bitwarden.

### 2.3 Current Build Commands (Manual Process)

From `docs/spec/operations.md`:

| Image | Manual Build Command |
|---|---|
| docking-controller | `docker build -t zot.hwcopeland.net/chem/docking-controller:latest -f k8s-jobs/controller/Dockerfile k8s-jobs/` |
| autodock4 | `docker build -t hwcopeland/auto-docker:latest docker/autodock4/` (pushes to Docker Hub, not Zot) |
| autodock-vina | Not documented (Vina image is built but not deployed) |
| jupyter-chem | Not documented (currently pulled as `hwcopeland/jupyter-chem:latest` from Docker Hub) |

Note: autodock4 and jupyter-chem currently push to Docker Hub under `hwcopeland/`, not to
the private Zot registry. This TDD moves all four to `zot.hwcopeland.net/chem/`.

### 2.4 Existing Self-Hosted Runner Infrastructure

A scan of the `iac` repository reveals no existing GitHub Actions Runner Controller (ARC)
or self-hosted runner deployment on the cluster. The `arc-system` namespace referenced in
`k8s-jobs/zot-pull-secret.yaml` (in the comment "Same Bitwarden item UUID as
arc-system/zot-pull-secret.yaml") suggests a self-hosted runner was previously planned or
partially set up, but no runner deployment manifests exist in any scanned directory.

---

## 3. Alternatives Considered: Solving the Network Boundary

### Option A: GitHub Actions Runner Controller (ARC) — Self-Hosted Runner on Cluster

**How it works:** Deploy ARC (v2 `gha-runner-scale-set` Helm chart) onto the RKE2 cluster.
Runners run as Kubernetes pods inside the cluster's network — they can reach
`zot.hwcopeland.net` directly because the cluster nodes are on the same LAN as the registry.
GitHub Actions jobs are routed to these runners by assigning the workflow a matching label.

**Strengths:**
- Runners have direct, unrestricted network access to all cluster-internal services
- No firewall rule changes or network tunneling required
- Ephemeral runner pods (scale-set mode) — each job runs in a fresh pod, cleaned up after completion
- Runners can use the cluster's `KUBECONFIG` for optional future deployment automation
- The `arc-system` namespace reference in the existing `zot-pull-secret.yaml` comment
  suggests this was the intended approach
- Aligns with best practices for pushing to private registries (GitHub's own recommendation
  for on-prem registries)
- No additional network infrastructure dependencies (Tailscale, Cloudflare account config)

**Weaknesses:**
- Requires deploying and maintaining ARC on the cluster (an additional Helm release)
- Runner pods consume cluster resources (CPU/memory) while idle
- GitHub token for runner registration must be managed as a secret
- ARC scale-set controller requires a webhook or polling — adds cluster complexity

**Estimated effort:** M (ARC Helm chart + runner scale-set, ExternalSecret for PAT, one
workflow YAML)

---

### Option B: Tailscale Operator + GitHub Actions Tailscale Action

**How it works:** Deploy the Tailscale Kubernetes operator on the cluster (already not
present — requires new deployment). In the GitHub Actions workflow, use the
`tailscale/github-action` step to bring the runner into the Tailscale overlay network.
The runner can then reach the registry via Tailscale MagicDNS or a Tailscale Funnel.

**Strengths:**
- Cloud-hosted GitHub runners; no self-hosted runner management
- Tailscale handles NAT traversal automatically
- Works with any private service on the tailnet, not just the registry
- Tailscale action is well-maintained and widely used

**Weaknesses:**
- Requires a Tailscale account (third-party SaaS dependency) and Tailscale operator
  deployment on the cluster
- OAuth client secret (`TS_OAUTH_CLIENT_SECRET`) must be stored in GitHub secrets and a
  Tailscale OAuth app must be created — additional SaaS management overhead
- Increases the dependency surface (Tailscale terms of service, availability)
- Per-job Tailscale connection adds 5-15s overhead per workflow run
- The existing cluster infrastructure (Cilium BGP, Cloudflare operator) does not currently
  include Tailscale; adding it is a non-trivial infrastructure change

**Estimated effort:** L (Tailscale operator install, tailnet configuration, OAuth app,
GitHub secrets, workflow integration)

---

### Option C: Cloudflare Tunnel (cloudflared) — Expose Registry to GitHub Actions

**How it works:** The cluster already uses Cloudflare for DNS management. A `cloudflared`
tunnel could expose `zot.hwcopeland.net` through Cloudflare's Argo Tunnel network, making
it reachable from GitHub Actions without direct internet port-forwarding.

**Strengths:**
- No self-hosted runner infrastructure needed
- Cloudflare is already used for DNS in this cluster
- Works from any CI runner (cloud or self-hosted)

**Weaknesses:**
- The Zot registry is already DNS-accessible (`zot.hwcopeland.net` with `proxied: false`).
  The network constraint is not DNS resolution — it is that the home IP does not forward
  the Zot port to the internet. A Cloudflare Tunnel would need to be deployed separately.
- The Zot registry uses OIDC auth; permanent public exposure via Cloudflare Tunnel requires
  careful rate limiting and security review to prevent abuse.
- The current Zot DNS record has `proxied: false`, meaning Cloudflare is not in the path.
  Switching to a Tunnel with `proxied: true` changes the auth flow for OIDC callbacks.
- Cloudflare Tunnel is a persistent daemon that needs to be kept running and monitored.
- This approach permanently exposes the registry to the internet at the Cloudflare edge —
  a broader attack surface than needed for CI alone.

**Estimated effort:** L (cloudflared deployment, tunnel configuration, Cloudflare dashboard
config, security review)

---

### Option D: Workflow Dispatch with Local Runner (Manual-Trigger Only)

**How it works:** Workflows run on a self-hosted runner deployed on a cluster node
(not via ARC — just a single persistent runner process). Triggered manually via
`workflow_dispatch` for each push.

**Strengths:**
- Simplest possible setup — no Kubernetes controller, just a runner process on a node

**Weaknesses:**
- Does not automate on push to `main` — violates the goal of automated triggering
- Persistent runner process (not ephemeral) — state accumulates, requires manual restarts
- Not horizontally scalable — only one job can run at a time

**Verdict:** Rejected. Does not meet the acceptance criteria.

---

### Recommendation: Option A — GitHub Actions Runner Controller (ARC)

ARC (Actions Runner Controller v2, scale-set mode) is the recommended approach because:

1. **Network requirement is cleanly solved** at the cluster level — no new SaaS
   dependencies, no firewall changes, no tunnel management.
2. **Ephemeral runners** — each job gets a fresh pod, preventing state bleed between builds.
   This is a security property: a compromised build cannot contaminate future builds.
3. **Consistent with existing infrastructure** — the `arc-system` reference in the existing
   `zot-pull-secret.yaml` comment suggests this was the intended direction.
4. **Lowest ongoing operational overhead** — after initial setup, ARC self-manages runner
   pod lifecycle via Kubernetes.
5. **GitHub's documented recommendation** for private registry access is to use self-hosted
   runners co-located with the registry.

The primary cost — ARC Helm deployment — is a one-time setup of moderate complexity (M).

---

## 4. Architecture and System Design

### 4.1 Component Overview

```
GitHub (Khemiatic-Energistics/compute-infrastructure)
  |
  | push to main
  |
  v
GitHub Actions Workflow (.github/workflows/build-images.yaml)
  |
  | routes job to:
  v
ARC Scale-Set Runner (pod in arc-system namespace, cluster LAN)
  |
  | docker buildx build + push
  |
  v
zot.hwcopeland.net/chem/  (Zot OCI registry, LAN)
  |
  | Flux ImageRepository polls (1m interval)
  v
Flux image-reflector-controller (chem namespace)
  |
  | new tag detected, ImagePolicy match
  v
Flux image-automation-controller
  |
  | [ci skip] commit updating image tag in deployment YAML
  v
GitHub (iac repository, main branch)
  |
  | Flux GitRepository detects commit
  v
Flux kustomize-controller (chem namespace)
  |
  | kubectl apply
  v
Kubernetes Deployment (chem/docking-controller updated)
```

### 4.2 ARC Deployment (arc-system namespace)

ARC v2 (scale-set mode) consists of two Helm releases:
- `actions-runner-controller` — the controller manager (deployed to `arc-system` namespace)
- `chem-runners` — a `AutoScalingRunnerSet` that defines runner pods (deployed to
  `arc-system` namespace, labeled for the `Khemiatic-Energistics/compute-infrastructure` repo)

Runner pods need:
- Docker-in-Docker (DinD) sidecar OR Docker socket mount from the node for `docker build`
  and `docker push` — DinD is preferred for isolation; socket mount is simpler but
  shares the node daemon.
- Network access to `zot.hwcopeland.net` — satisfied by running on the cluster LAN.
- The Zot push credentials injected as environment variables via a Kubernetes Secret
  (sourced from Bitwarden via ExternalSecret — same pattern as `k8s-jobs/zot-pull-secret.yaml`).

**ARC registration secret:** ARC v2 requires a GitHub App (preferred over PAT) for runner
registration. A GitHub App with `Actions: Read & Write` and `Administration: Read & Write`
permissions on the repository is created once and its `APP_ID`, `INSTALLATION_ID`, and
private key stored as a Kubernetes Secret in `arc-system` namespace (via ExternalSecret
from Bitwarden).

### 4.3 GitHub Actions Workflow Design

**File:** `.github/workflows/build-images.yaml`

**Trigger:** `push` to `main` branch, path filter covering all four image contexts.

**Runner label:** `self-hosted` + a custom label matching the ARC scale-set name
(e.g., `chem-runner`).

**Jobs:** One matrix job or four parallel jobs (one per image). Matrix approach recommended
to reduce workflow YAML duplication. The matrix defines:

| matrix.name | matrix.context | matrix.dockerfile | matrix.image |
|---|---|---|---|
| docking-controller | k8s-jobs/ | k8s-jobs/controller/Dockerfile | zot.hwcopeland.net/chem/docking-controller |
| autodock4 | docker/autodock4/ | docker/autodock4/Dockerfile | zot.hwcopeland.net/chem/autodock4 |
| autodock-vina | docker/autodock-vina/ | docker/autodock-vina/Dockerfile | zot.hwcopeland.net/chem/autodock-vina |
| jupyter-chem | rke2/jupyter/docker/ | rke2/jupyter/docker/Dockerfile | zot.hwcopeland.net/chem/jupyter-chem |

**Steps per job:**
1. `actions/checkout@v4`
2. `docker/setup-buildx-action@v3` (enables layer caching)
3. `docker/login-action@v3` with `registry: zot.hwcopeland.net`, credentials from
   `${{ secrets.ZOT_USERNAME }}` / `${{ secrets.ZOT_PASSWORD }}`
4. Compute image tags (commit SHA + semver)
5. `docker/build-push-action@v5` with `push: true` and `cache-from: type=registry` /
   `cache-to: type=registry` using the registry for layer caching

**Conditional build:** Use `paths` filter so docking-controller only rebuilds when
`k8s-jobs/controller/**` changes, autodock4 rebuilds when `docker/autodock4/**` changes,
etc. This prevents unnecessary 20-minute autodock builds on every push.

### 4.4 Build Context Notes

| Image | Dockerfile Path | Build Context | Notes |
|---|---|---|---|
| docking-controller | `k8s-jobs/controller/Dockerfile` | `k8s-jobs/` | Dockerfile uses `COPY controller/go.mod ...` — context must be `k8s-jobs/`, NOT `k8s-jobs/controller/`. The existing manual build command confirms this. |
| autodock4 | `docker/autodock4/Dockerfile` | `docker/autodock4/` | Downloads binaries from scripps.edu via `ADD` — build will make outbound HTTP requests. |
| autodock-vina | `docker/autodock-vina/Dockerfile` | `docker/autodock-vina/` | Downloads Vina binary from vina.scripps.edu via `ADD`. Same pattern as autodock4. |
| jupyter-chem | `rke2/jupyter/docker/Dockerfile` | `rke2/jupyter/docker/` | Installs Miniconda via wget — requires outbound internet access from runner. |

**Warning:** The autodock4 and autodock-vina Dockerfiles use `ADD` with HTTPS URLs to
download large upstream binaries. These adds are not cached in the normal Docker layer
cache — they re-download on every cache-miss. Layer caching via the registry (`type=registry`)
will cache the resulting layers, but the first build after a cache eviction will make
outbound requests from the runner pod. Network policy for the `arc-system` namespace should
permit egress to `autodock.scripps.edu`, `vina.scripps.edu`, and `repo.anaconda.com`.

### 4.5 Flux Image Automation Resources (chem namespace)

Four `ImageRepository` + `ImagePolicy` pairs are needed, one per image. A single
`ImageUpdateAutomation` can update all four images in the `chem` namespace.

The `ImageUpdateAutomation` will target the path in the `iac` repository where the
`chem` namespace Flux manifests live. The exact path depends on where the Flux GitOps
manifests for `chem` are placed. Based on the `tooling` precedent:
- GitRepository: `https://github.com/hwcopeland/iac.git` (the root iac monorepo)
- Update path: `./rke2/chem/flux` (to be created, following the `tooling` pattern)

The `ImageUpdateAutomation` will push commits to `main` with `[ci skip]` in the message
to prevent triggering another build workflow cycle.

The deployment manifests that Flux updates must include the `# {"$imagepolicy": "chem:<name>"}`
annotation comment on the `image:` line, following the exact pattern from
`rke2/tooling/flux/theswamp/deployment.yaml`.

---

## 5. Image Tagging Strategy

### 5.1 Tag Format: `sha-<short-sha>`

The primary tag format follows the established precedent in the `tooling` namespace image
policies:

```
sha-<7-char-git-sha>
```

Example: `zot.hwcopeland.net/chem/docking-controller:sha-a1b2c3d`

**Why this format:**
- Deterministic from the git commit — reproducible, auditable
- Compatible with the existing `ImagePolicy` filter pattern `^sha-[a-f0-9]+` already in use
- Short enough to be readable in `kubectl get` output and `ImagePolicy` status
- The existing `tooling` deployments prove this format works end-to-end with Flux

**Secondary tag: `latest`** will also be pushed for compatibility with any tooling that
references `:latest` directly (including the current `k8s-jobs/config/deployment.yaml`
which hardcodes `:latest` as a fallback).

**Semver tags:** The `tooling` namespace policy uses `alphabetical` ordering on SHA tags
rather than `semver`. SHA ordering alphabetically is not chronological, which is a known
limitation called out in the existing `image-policy.yaml` comments. For the `chem` images,
we will follow the same pattern initially (SHA tags, alphabetical ordering). Semver tags
can be added in a future phase once a release tagging discipline is established.

The comment in `tooling`'s `image-policy.yaml` notes an alternative: `sha-<run_number>-<sha>`
where the run number enables numerical ordering. For `chem`, we adopt this improved format:

```
sha-<GITHUB_RUN_NUMBER>-<short-sha>
```

Example: `sha-142-a1b2c3d`

With `ImagePolicy` filter `^sha-[0-9]+-[a-f0-9]+` and `numerical` ordering on the run
number prefix. This guarantees chronological ordering — the highest run number is always
the most recent build.

**Implementation note:** The run number is available as `${{ github.run_number }}` and the
short SHA as `${{ github.sha }}` (first 7 chars via shell). The tag computation step:

```bash
SHORT_SHA="${GITHUB_SHA::7}"
echo "TAG=sha-${GITHUB_RUN_NUMBER}-${SHORT_SHA}" >> $GITHUB_ENV
```

### 5.2 Flux ImagePolicy Filter Pattern

```yaml
filterTags:
  pattern: '^sha-[0-9]+-[a-f0-9]+'
policy:
  numerical:
    order: asc
```

The `numerical` policy extracts the leading integer from the matched group — but Flux's
`numerical` policy works on the full tag string numerically. The better approach (matching
the tooling precedent comment about run numbers) is to use the `alphabetical: asc` policy
with the `sha-<run>-<sha>` format, since zero-padded run numbers would sort correctly but
run numbers are not zero-padded in GitHub.

Therefore: use `semver` policy with a `>=0.0.1` range constraint and push actual semver
tags (`v0.1.0`, `v0.1.1`, etc.) for a future phase. For the initial phase, use alphabetical
ordering on `sha-<sha>` format (identical to the `tooling` pattern) — it is not perfectly
chronological but is workable for a single-developer workflow where deployments are reviewed.

The transition plan: Phase 1 ships `sha-<sha>` tags with alphabetical policy (proven
pattern). Phase 3 (post-stabilization) adds semver release tags via a separate workflow
triggered on git tags, with `semver: ">=1.0.0"` Flux ImagePolicy for production promotion.

---

## 6. Secret Management

### 6.1 Registry Credentials in GitHub Actions

The Zot registry credentials (from Bitwarden item `766ec5c7-6aa8-419d-bb27-e5982872bc5b`)
must be available to the `docker/login-action` step in the GitHub Actions workflow.

**Approach: GitHub Actions Encrypted Repository Secrets**

Store as two repository secrets on `Khemiatic-Energistics/compute-infrastructure`:
- `ZOT_USERNAME` — the Bitwarden item's username field
- `ZOT_PASSWORD` — the Bitwarden item's password field

**Why not a Bitwarden Secret Manager GitHub Action:**
The Bitwarden GitHub Actions integration (`bitwarden/sm-action`) exists but requires a
Bitwarden Secrets Manager subscription and machine account setup. The simpler path is to
read the credentials from Bitwarden once, store them as GitHub encrypted secrets, and
rotate them manually when the Bitwarden item is rotated. This matches the existing Airflow
and JupyterHub credential management style.

**Rotation procedure:** When the Zot credentials in Bitwarden are rotated:
1. Read the new username/password from the Bitwarden UI
2. Update `ZOT_USERNAME` and `ZOT_PASSWORD` in GitHub repository Settings > Secrets

This is not automated — it is the same friction as the existing Ansible Vault pattern.

### 6.2 ARC Registration Secret

ARC v2 recommends a GitHub App for runner registration (PAT-based registration was
deprecated in ARC v2 scale-set mode). The GitHub App credentials (App ID, Installation ID,
private key) are stored in Bitwarden and materialized in the `arc-system` namespace via
ExternalSecret.

A new Bitwarden item will be created for the ARC GitHub App credentials. The ExternalSecret
template for ARC follows the same pattern as `k8s-jobs/zot-pull-secret.yaml`.

### 6.3 Secret Boundary Analysis

| Secret | Where Stored | Who Accesses It | How |
|---|---|---|---|
| Zot push credentials | GitHub Actions encrypted secrets | CI workflow (push) | `${{ secrets.ZOT_USERNAME }}` / `${{ secrets.ZOT_PASSWORD }}` |
| Zot pull credentials | Bitwarden → ExternalSecret → K8s Secret | Flux image-reflector, pods | `imagePullSecrets`, Flux `secretRef` |
| ARC GitHub App key | Bitwarden → ExternalSecret → K8s Secret | ARC controller | Helm values reference |
| ARC GitHub App key | Same Bitwarden item | Humans for setup | One-time read |

No new secrets are committed to the repository. All secrets follow the established
ExternalSecret + Bitwarden pattern for cluster-side access.

---

## 7. Workflow Design: Triggers, Jobs, Caching

### 7.1 Triggers

```yaml
on:
  push:
    branches: [main]
    paths:
      - 'k8s-jobs/controller/**'
      - 'docker/autodock4/**'
      - 'docker/autodock-vina/**'
      - 'rke2/jupyter/docker/**'
      - '.github/workflows/build-images.yaml'
  workflow_dispatch:  # manual trigger for initial testing
```

The `workflow_dispatch` trigger is critical for initial setup and debugging without
requiring a source code change.

### 7.2 Jobs Structure

**Preferred structure: Matrix job**

```yaml
jobs:
  build-and-push:
    runs-on: [self-hosted, chem-runner]
    strategy:
      matrix:
        include:
          - name: docking-controller
            context: k8s-jobs/
            dockerfile: k8s-jobs/controller/Dockerfile
            image: zot.hwcopeland.net/chem/docking-controller
            paths: k8s-jobs/controller
          - name: autodock4
            context: docker/autodock4/
            dockerfile: docker/autodock4/Dockerfile
            image: zot.hwcopeland.net/chem/autodock4
            paths: docker/autodock4
          - name: autodock-vina
            context: docker/autodock-vina/
            dockerfile: docker/autodock-vina/Dockerfile
            image: zot.hwcopeland.net/chem/autodock-vina
            paths: docker/autodock-vina
          - name: jupyter-chem
            context: rke2/jupyter/docker/
            dockerfile: rke2/jupyter/docker/Dockerfile
            image: zot.hwcopeland.net/chem/jupyter-chem
            paths: rke2/jupyter/docker
```

**Path-based conditional builds:** The matrix `paths` field is informational only —
GitHub Actions `paths` filter works at the workflow level, not the job level. To avoid
rebuilding all four images on every push (the autodock images take ~15 minutes each),
implement per-job path checking with the `dorny/paths-filter` action or a shell step
that uses `git diff --name-only` to skip the build step if no relevant files changed.

Alternatively, split into four separate jobs with individual `paths` filters — more
verbose but simpler logic.

**Recommendation:** Start with four separate jobs and `paths` filters at the workflow level.
If GitHub Actions triggers the whole workflow on any path change, use step-level skip
logic within a matrix job.

### 7.3 Caching Strategy

The autodock4 and autodock-vina images are large (`ubuntu:20.04` base + build tools) and
slow to build. Layer caching is critical for build time.

**Registry-based cache** (recommended for self-hosted runner with direct registry access):

```yaml
- uses: docker/build-push-action@v5
  with:
    cache-from: type=registry,ref=${{ matrix.image }}:cache
    cache-to: type=registry,ref=${{ matrix.image }}:cache,mode=max
```

Registry cache with `mode=max` caches all intermediate layers (not just the final stage).
This is the most effective cache for multi-stage builds like the autodock Dockerfiles.

**Alternative: GitHub Actions cache backend (`type=gha`)**
Does not work with self-hosted runners that are in Kubernetes pods unless the runner has
access to the GitHub cache service endpoint. Since the runner is on-cluster, `type=gha`
will fail. Use `type=registry` exclusively.

### 7.4 Build Platform

All four Dockerfiles are written for `linux/amd64` (confirmed: the controller Dockerfile
uses `golang:1.21-alpine` which defaults to the host platform, and the autodock binaries
are `x86_64Linux2`). Build with `--platform linux/amd64` explicitly.

The docking-controller binary in the repo (`k8s-jobs/controller/docking-controller`) is an
ARM64 Mach-O binary (macOS development artifact). The CI build produces the correct
`linux/amd64` binary; the committed binary should be added to `.gitignore`.

---

## 8. Flux Integration Design

### 8.1 GitOps Repository Layout

The existing `iac` repository (which Flux already watches at
`https://github.com/hwcopeland/iac.git`) should have the chem Flux resources at:

```
rke2/chem/flux/
├── kustomization.yaml          # Flux Kustomization source (created separately)
├── image-automation/
│   ├── kustomization.yaml
│   ├── zot-pull-secret.yaml    # ExternalSecret for Flux image-reflector
│   ├── image-repository-docking-controller.yaml
│   ├── image-repository-autodock4.yaml
│   ├── image-repository-autodock-vina.yaml
│   ├── image-repository-jupyter-chem.yaml
│   ├── image-policy-docking-controller.yaml
│   ├── image-policy-autodock4.yaml
│   ├── image-policy-autodock-vina.yaml
│   ├── image-policy-jupyter-chem.yaml
│   └── image-update-automation.yaml
└── workloads/
    └── ... (deployment manifests with $imagepolicy annotations)
```

**Note:** This TDD covers the image automation resources. The `workloads/` directory
(containing the migrated `k8s-jobs/` manifests with updated image references and
`$imagepolicy` annotations) is part of the broader Flux migration scope.

### 8.2 ImageRepository Resources (chem namespace)

All four `ImageRepository` resources follow the pattern from `tooling`:

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageRepository
metadata:
  name: docking-controller
  namespace: chem
spec:
  image: zot.hwcopeland.net/chem/docking-controller
  interval: 1m0s
  secretRef:
    name: zot-pull-secret
```

The `zot-pull-secret` in `chem` namespace already exists as an ExternalSecret
(`k8s-jobs/zot-pull-secret.yaml`). The Flux image-reflector-controller needs this secret
to authenticate to the registry to list tags.

### 8.3 ImagePolicy Resources (chem namespace)

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImagePolicy
metadata:
  name: docking-controller
  namespace: chem
spec:
  imageRepositoryRef:
    name: docking-controller
  filterTags:
    pattern: '^sha-[a-f0-9]+'
  policy:
    alphabetical:
      order: asc
```

Pattern `^sha-[a-f0-9]+` matches the `sha-<7-char-sha>` tag format. This is identical to
the `tooling` pattern and proven to work.

### 8.4 ImageUpdateAutomation Resource (chem namespace)

A single `ImageUpdateAutomation` for all four images:

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageUpdateAutomation
metadata:
  name: chem-images
  namespace: chem
spec:
  interval: 1m0s
  sourceRef:
    kind: GitRepository
    name: <chem-gitrepository-name>
    namespace: chem
  git:
    checkout:
      ref:
        branch: main
    commit:
      author:
        email: fluxcdbot@users.noreply.github.com
        name: fluxcdbot
      messageTemplate: |
        [ci skip] auto-update chem images
    push:
      branch: main
  update:
    path: ./rke2/chem/flux/workloads
    strategy: Setters
```

**Prerequisite:** A `GitRepository` resource for the `iac` repository must exist in the
`chem` namespace (or be shared from `tooling` namespace if the Flux architecture allows
cross-namespace source references). This is the responsibility of the broader Flux migration
TDD.

### 8.5 Deployment Manifest Annotation

The `k8s-jobs/config/deployment.yaml` image reference must be updated to include the Flux
setter annotation:

```yaml
image: zot.hwcopeland.net/chem/docking-controller:sha-<initial-sha> # {"$imagepolicy": "chem:docking-controller"}
```

The `ImageUpdateAutomation` will rewrite the tag portion whenever a new matching tag is
detected by the `ImagePolicy`.

---

## 9. Migration and Rollout

### 9.1 Current-to-Proposed Transition

| State | Before | After |
|---|---|---|
| Image build | Manual `docker build` from operator workstation | GitHub Actions on push to main |
| Image tags | `:latest` only | `sha-<sha>` + `:latest` |
| Image registry | autodock4/jupyter from Docker Hub; controller from Zot | All four from Zot `chem/` |
| Deployment | Manual `kubectl apply -k k8s-jobs/` | Flux GitOps (post full Flux migration) |
| `iac/ansible/` | Present | Deleted |

### 9.2 Phased Rollout

**Phase 1 (CI Foundation — S, ~1 day):**
1. Install ARC v2 on the cluster (Helm: `actions-runner-controller` + `chem-runners` scale-set)
2. Store ARC GitHub App credentials in Bitwarden, create ExternalSecret in `arc-system`
3. Add `ZOT_USERNAME` and `ZOT_PASSWORD` to GitHub repository secrets
4. Write `.github/workflows/build-images.yaml` with matrix build for all 4 images
5. Trigger `workflow_dispatch` to validate builds succeed
6. Verify all 4 images appear in `zot.hwcopeland.net/chem/` with `sha-*` tags
7. Delete `iac/ansible/` directory, commit to `main`

**Phase 2 (Flux Image Automation — S, ~0.5 day):**
1. Create `rke2/chem/flux/image-automation/` directory with 4 ImageRepository + ImagePolicy + 1 ImageUpdateAutomation
2. Add ExternalSecret for `zot-pull-secret` in image-automation namespace context (already exists in `chem` — verify Flux can use it)
3. Configure the ImageUpdateAutomation's `sourceRef` to point at the correct GitRepository
4. Update `k8s-jobs/config/deployment.yaml` with `$imagepolicy` annotation
5. Push a code change to `k8s-jobs/controller/` and verify Flux detects the new tag and commits a tag update

**Phase 3 (Semver Release Tags — M, future):**
- Add a separate workflow triggered on `push: tags: ['v*']` that builds and pushes both
  `sha-<sha>` and `v<semver>` tags
- Migrate Flux `ImagePolicy` for production images to `semver: ">=1.0.0"` filter
- This phase is deferred until the team has established a release cadence

### 9.3 Rollback Plan

**Phase 1 rollback:** If CI is broken, the operator can revert to manual builds using the
existing `docker build` commands from `docs/spec/operations.md`. The `:latest` tag still
exists in Zot for all images. ARC can be uninstalled via Helm without affecting workloads.

**Phase 2 rollback:** If Flux image automation causes unintended deployments, the
`ImageUpdateAutomation` can be suspended via `flux suspend image update chem-images -n chem`
without removing any resources. The `[ci skip]` commit prefix prevents CI re-triggering.

**Ansible deletion rollback:** The `iac/ansible/` directory will be removed via a git
commit. It can be recovered from git history if needed. The ansible playbooks are not part
of the ongoing automated operations after this migration; they served bare-metal provisioning
which is a one-time operation.

### 9.4 Breaking Changes

- The `k8s-jobs/config/deployment.yaml` will change from `image: ...docking-controller:latest`
  to `image: ...docking-controller:sha-<sha> # {"$imagepolicy": "chem:docking-controller"}`.
  This is backward-compatible — the `sha-<sha>` tag is a valid OCI image reference.
- Docker Hub images (`hwcopeland/auto-docker:latest`, `hwcopeland/jupyter-chem:latest`) are
  not removed immediately. The `DefaultImage` constant in `main.go` still references Docker
  Hub. Updating this constant is a separate code change that should be made alongside
  populating the new Zot images.
- `iac/ansible/` deletion: the `docs/iac/ansible.md` documentation file will become a
  stale reference. It should be updated or archived.

---

## 10. Risks and Open Questions

### 10.1 Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Autodock builds download from external URLs (`ADD https://`) and may be rate-limited or the URLs may change | Medium | High — build fails | Cache layers via registry cache; add a smoke-test to verify downloaded binaries work before push |
| ARC runner pod egress blocked by Cilium NetworkPolicy in `arc-system` | Medium | High — build fails | Create CiliumNetworkPolicy allowing egress from `arc-system` to zot-registry IP, autodock.scripps.edu, vina.scripps.edu, repo.anaconda.com |
| `AutoDockTools_py3` installed from GitHub `main` tip (no version pin) produces different results between builds | High | Medium — non-reproducible images | Pin to a specific commit SHA in the Dockerfile; flag as a separate issue |
| The Zot `zot-pull-secret` in `chem` namespace is an `imagePullSecrets` credential for pods — Flux image-reflector uses it differently (as a `secretRef` in ImageRepository) — may require the secret to exist in the same namespace as the ImageRepository | Low | Medium — Flux ImageRepository fails | Test: Flux image-reflector requires the secret in the namespace where `ImageRepository` lives. The `chem` namespace already has this secret. Verify it works before Phase 2 cutover. |
| `[ci skip]` commit from Flux ImageUpdateAutomation may not be recognized by all CI systems if the convention changes | Low | Low — triggers extra build | Confirmed: GitHub Actions respects `[ci skip]` and `[skip ci]` in commit messages |
| Zot OIDC primary auth — username/password credentials in Bitwarden item `766ec5c7` may expire or become invalid | Low | High — all pushes fail | Document rotation procedure; consider creating a dedicated machine account in Authentik for CI rather than using the OIDC-primary account |

### 10.2 Open Questions

1. **Machine account for CI push:** The Bitwarden item `766ec5c7-6aa8-419d-bb27-e5982872bc5b`
   is used for both Kubernetes pull secrets and now CI push. Is this a single shared service
   account or a personal account? If personal, a dedicated machine account should be created
   in Authentik for CI push operations to avoid coupling a human account to automated CI.
   *Needs operator decision before Phase 1 implementation.*

2. **DinD vs. host socket for runner builds:** ARC scale-set runners in DinD mode (Docker
   within Kubernetes pod) require the runner pod to run privileged or with specific security
   contexts. Host socket mount (`/var/run/docker.sock`) is simpler but shares the node
   daemon. For a lab environment with trusted workloads, host socket is acceptable; for
   production, DinD with a non-privileged mode (Kaniko, Buildkit, Podman) is preferable.
   *Recommend host socket mount for initial setup; revisit if security requirements harden.*

3. **Flux GitRepository for chem namespace:** The `ImageUpdateAutomation` requires a
   `GitRepository` source in the `chem` namespace. This requires a deploy key for the
   `iac` repository. The `tooling` Flux setup uses a secret named `flux-system` (see
   `tooling/gotk-sync.yaml` `secretRef: name: flux-system`). Does this secret exist
   cluster-wide, or is it namespace-scoped? *Check whether the existing `flux-system`
   secret can be referenced from `chem` or whether a new deploy key must be provisioned.*

4. **`iac/ansible/` deletion scope:** The `docs/iac/ansible.md` documentation references
   ansible playbooks. Should it be deleted, or updated to historical reference? What about
   `docs/spec/architecture.md` §7 which documents the Ansible provisioning path?
   *Recommend: update architecture.md §7 to note that Ansible provisioning was replaced by
   the GitOps pipeline, and archive rather than delete `docs/iac/ansible.md`.*

---

## 11. Testing Strategy

### 11.1 CI Workflow Tests

| Test | How | When |
|---|---|---|
| Workflow syntax valid | `actionlint` in a separate lint job | On every PR |
| All 4 images build successfully | End-to-end via `workflow_dispatch` | Before Phase 1 merge |
| Images pushed to Zot with correct tags | `docker manifest inspect zot.hwcopeland.net/chem/<name>:sha-<sha>` after workflow run | Phase 1 smoke test |
| Flux ImagePolicy detects new tag | `kubectl get imagepolicy -n chem` — `LATEST IMAGE` column updates | Phase 2 smoke test |
| Flux ImageUpdateAutomation commits tag change | Check `git log --oneline` on `iac` repo main branch for `[ci skip] auto-update chem images` | Phase 2 smoke test |

### 11.2 Build Matrix Verification

For the initial setup, all 4 images should be built and pushed end-to-end from a fresh
state:
1. Delete any existing `sha-*` tags from Zot for all 4 repos
2. Trigger `workflow_dispatch`
3. Confirm 4 jobs complete in the workflow run
4. Confirm 4 new `sha-*` tags appear in Zot
5. Confirm Flux detects and applies updates

### 11.3 Regression Tests (Future)

Once a Go test suite exists for the docking-controller, add a `go test ./...` step to the
`docking-controller` matrix job before the build step. This is deferred — see the code
quality spec.

---

## 12. Observability and Operational Readiness

### 12.1 Signals and Alerts

| Signal | Source | How to Observe |
|---|---|---|
| Build success/failure | GitHub Actions workflow run status | GitHub UI / email notification if enabled |
| Image push success | `docker/build-push-action` step output `digest` | GitHub Actions job log |
| Flux ImageRepository scan | `kubectl get imagerepository -n chem` | `READY` column + `LAST SCAN` timestamp |
| Flux ImagePolicy latest image | `kubectl get imagepolicy -n chem` | `LATEST IMAGE` column |
| Flux ImageUpdateAutomation | `kubectl get imageupdateautomation -n chem` | `READY` + `LAST RUN` |
| ARC runner availability | `kubectl get pods -n arc-system` | Runner pods in `Running` state |
| ARC scale-set status | `kubectl get autoscalingrunnerset -n arc-system` | Desired/assigned/running counts |

### 12.2 3am Diagnosability

**Build failed:**
1. Check GitHub Actions workflow run for the failing job and step
2. If `docker/login-action` fails: credentials rotated without updating GitHub secrets —
   update `ZOT_USERNAME`/`ZOT_PASSWORD` in GitHub settings
3. If `docker/build-push-action` fails with network error: ARC runner pod cannot reach
   registry or external URL — check `kubectl logs -n arc-system <runner-pod>` and Cilium
   network policy

**Flux not detecting new tags:**
1. `kubectl describe imagerepository -n chem docking-controller` — check `Status.Conditions`
2. If `401 Unauthorized`: `zot-pull-secret` may have expired — `kubectl describe externalsecret -n chem zot-pull-secret`
3. If connection refused: Zot registry down — check `kubectl get pods -n tooling -l app.kubernetes.io/name=zot`

**ARC runner not picking up jobs:**
1. `kubectl get pods -n arc-system` — ensure runner pods exist
2. `kubectl describe pod -n arc-system <runner>` — check for image pull failures or resource constraints
3. GitHub repository Settings > Actions > Runners — verify runner shows as online

### 12.3 Production Readiness Criteria for Phase 1 Go-Live

- [ ] ARC deployed and at least one runner pod in `Running` state
- [ ] `workflow_dispatch` test completes all 4 builds successfully
- [ ] All 4 `sha-*` tags visible in Zot UI at `https://zot.hwcopeland.net`
- [ ] `ZOT_USERNAME` and `ZOT_PASSWORD` stored in GitHub repository secrets
- [ ] `.github/workflows/build-images.yaml` merged to `main`
- [ ] `iac/ansible/` deleted from `main`
- [ ] `docs/spec/operations.md` updated to reflect CI-driven build process
- [ ] `docs/spec/architecture.md` §7 updated to note Ansible removal

---

## 13. Implementation Phases

### Phase 1 — ARC Deployment and CI Workflow (S)
**Owner:** @senior-engineer
**Deliverables:**
1. ARC controller Helm release in `arc-system` namespace (manifests in `rke2/arc-system/`)
2. ARC runner scale-set Helm release targeting `Khemiatic-Energistics/compute-infrastructure`
3. Bitwarden item for ARC GitHub App credentials + ExternalSecret in `arc-system`
4. GitHub repository secrets: `ZOT_USERNAME`, `ZOT_PASSWORD`
5. `.github/workflows/build-images.yaml` with 4-image matrix build
6. CiliumNetworkPolicy for `arc-system` egress to registry and external build URLs
7. Deletion of `iac/ansible/` directory
8. Updated `docs/spec/operations.md` and `docs/spec/architecture.md`

**Dependencies:** Bitwarden access (to read Zot credentials and create ARC App item),
GitHub App creation with appropriate permissions on the repository.

**Acceptance test:** `workflow_dispatch` on `main` builds and pushes all 4 images to Zot.

---

### Phase 2 — Flux Image Automation (S)
**Owner:** @senior-engineer
**Deliverables:**
1. `rke2/chem/flux/image-automation/` with 4 ImageRepository + 4 ImagePolicy + 1 ImageUpdateAutomation
2. ExternalSecret for `zot-pull-secret` in image-automation namespace (or confirm existing one works)
3. Updated `k8s-jobs/config/deployment.yaml` with `$imagepolicy` annotation for docking-controller
4. GitRepository source for `chem` namespace (or reuse from `tooling` — needs investigation)
5. Deploy key for `iac` repo if new GitRepository is needed

**Dependencies:** Phase 1 complete (images must exist in Zot with `sha-*` tags for Flux to detect).

**Acceptance test:** Push a trivial change to `k8s-jobs/controller/main.go`, wait for CI
to build and push, wait for Flux to detect and commit a tag update to `iac` repo on `main`.

---

### Phase 3 — Semver Release Tags (M, deferred)
**Owner:** @senior-engineer
**Deliverables:**
1. Additional workflow job triggered on `push: tags: ['v*']` that tags images with semver
2. Updated `ImagePolicy` resources with `semver: ">=1.0.0"` filter
3. Release documentation

**Dependencies:** Phase 2 stable for at least 2 weeks with no tag ordering issues.

---

## Appendix: File Reference

| File | Role | Status |
|---|---|---|
| `k8s-jobs/controller/Dockerfile` | docking-controller image definition | Exists |
| `docker/autodock4/Dockerfile` | autodock4 image definition | Exists |
| `docker/autodock-vina/Dockerfile` | autodock-vina image definition | Exists |
| `rke2/jupyter/docker/Dockerfile` | jupyter-chem image definition | Exists |
| `k8s-jobs/zot-pull-secret.yaml` | ExternalSecret for Zot credentials (chem ns) | Exists |
| `k8s-jobs/config/deployment.yaml` | docking-controller Deployment | Needs $imagepolicy annotation |
| `k8s-jobs/kustomization.yaml` | Kustomize root for chem workloads | Exists |
| `.github/workflows/build-images.yaml` | CI workflow | To be created (Phase 1) |
| `rke2/chem/flux/image-automation/` | Flux image automation resources | To be created (Phase 2) |
| `iac/ansible/` | Ansible provisioning (to be deleted) | Delete in Phase 1 |
| `docs/iac/ansible.md` | Ansible documentation | Archive/update in Phase 1 |

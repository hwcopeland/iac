---
project: "compute-infrastructure"
maturity: "draft"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "Full GitOps pipeline for the chem namespace: GitHub Actions CI for 4 Docker images, Flux image automation, Kustomization-managed deployment, and deletion of the iac/ansible/ subtree from this repo"
owner: "@staff-engineer"
dependencies:
  - docs/spec/architecture.md
  - docs/spec/operations.md
  - docs/spec/security.md
---

# TDD: Flux GitOps for the chem Namespace

## 1. Problem Statement

### 1.1 What

The `chem` namespace currently has no automated build or deployment pipeline. Images are built
and pushed manually. Kubernetes manifests are applied manually with `kubectl apply -k k8s-jobs/`.
The deployment state is not tracked in any authoritative Git repository that the cluster watches.
`iac/ansible/` exists in this repository but is the wrong level of abstraction going forward —
it was used for bare-metal provisioning and is now superseded by GitOps.

The goal is a complete GitOps pipeline in three layers:

1. **CI (GitHub Actions):** On every push to `main` in
   `github.com/Khemiatic-Energistics/compute-infrastructure`, four Docker images are built and
   pushed to `zot.hwcopeland.net/chem/` with deterministic, Flux-compatible tags.
2. **Image Automation (Flux):** Flux watches `zot.hwcopeland.net/chem/` for new image tags,
   selects the latest per image policy, and commits updated tag references back to the GitOps
   repository.
3. **Kustomization (Flux):** Flux applies the `k8s-jobs/` Kustomize tree from the
   `compute-infrastructure` repository to the `chem` namespace, reconciling automatically
   whenever the manifests change.

### 1.2 Why Now

- The `docking-controller` is always deployed as `:latest` — there is no rollback path, no
  audit trail of what code is running, and no automated update on push.
- The `iac/ansible/` directory is dead weight: its provisioning role is done, and it contains
  a committed RKE2 join token (a known critical security exposure). Deleting it reduces the
  attack surface and signals the team that Ansible is no longer the deployment mechanism.
- The `tooling` namespace already runs Flux with a working image automation loop
  (`tooling/flux/image-automation/`) for the `theswamp` application. This TDD extends the same
  proven pattern to the `chem` namespace, so the incremental work is small.

### 1.3 Constraints

- **Single cluster, single environment.** There is no staging — changes go directly to
  production. Rollback is via git revert followed by Flux reconcile.
- **Private registry at `zot.hwcopeland.net`.** All four images must be pushed to
  `zot.hwcopeland.net/chem/`. Flux's image-reflector-controller must authenticate to poll tags.
- **Registry credentials are already managed by Bitwarden + external-secrets.io.** The same
  Bitwarden item UUID (`766ec5c7-6aa8-419d-bb27-e5982872bc5b`) and pattern already used in
  `k8s-jobs/zot-pull-secret.yaml` and `tooling/flux/image-automation/zot-pull-secret.yaml` is
  the template.
- **Flux is already installed on the cluster** in the `tooling` namespace (evidenced by
  `tooling/flux/tooling/gotk-components.yaml` and the live `[ci skip] auto-update` commits in
  the `hwcopeland/iac` git log). Bootstrap is only needed if Flux CRDs/controllers are absent,
  which is unlikely. The bootstrap path is included for completeness.
- **The GitOps root (`~/iac/rke2`) is `github.com/hwcopeland/iac`.** It is already a git
  repository with a remote at `https://github.com/hwcopeland/iac.git`. Flux watches this repo
  via the `tooling` namespace `GitRepository` object. New `chem` Flux manifests live in a
  `rke2/chem/flux/` subtree of this same repo.
- **`compute-infrastructure` is a separate GitHub repository** at
  `github.com/Khemiatic-Energistics/compute-infrastructure`. It does not need to be a
  Flux-watched source for the deployment path: the manifests Flux applies can live in the
  `hwcopeland/iac` repo at `rke2/chem/flux/k8s-jobs/` (a copy or symlink-equivalent), OR Flux
  can watch `compute-infrastructure` directly as a second `GitRepository`. See §3 (Alternatives)
  for the trade-off analysis and the selected approach.
- **Four images to manage:**
  - `zot.hwcopeland.net/chem/docking-controller` — Go controller, Dockerfile at
    `k8s-jobs/controller/Dockerfile` (build context `k8s-jobs/`)
  - `zot.hwcopeland.net/chem/autodock4` — AutoDock4 compute worker, Dockerfile at
    `docker/autodock4/Dockerfile` (build context `docker/autodock4/`)
  - `zot.hwcopeland.net/chem/autodock-vina` — AutoDock Vina compute worker, Dockerfile at
    `docker/autodock-vina/Dockerfile` (build context `docker/autodock-vina/`)
  - `zot.hwcopeland.net/chem/jupyter-chem` — JupyterHub custom image, Dockerfile at
    `rke2/jupyter/docker/Dockerfile` (build context `rke2/jupyter/docker/`)

### 1.4 Acceptance Criteria

1. A push to `main` in `compute-infrastructure` triggers a GitHub Actions workflow that builds
   and pushes all four images to `zot.hwcopeland.net/chem/` with the tag format
   `sha-<short-sha>`.
2. Within 2 minutes of the push completing, Flux detects the new tags via ImageRepository
   polling and updates the relevant deployment manifests in the `hwcopeland/iac` repository
   with a `[ci skip] auto-update ...` commit.
3. Within 10 minutes of the commit landing, Flux reconciles the `chem` Kustomization and
   the cluster reflects the new image tags (pods restarted with updated images).
4. `kubectl get imagepolicies -n chem` shows Ready status for all four images.
5. `kubectl get imagerepositories -n chem` shows the correct last-scanned timestamp.
6. `kubectl get imageupdateautomation -n chem` shows the last update time.
7. `kubectl get kustomization chem -n chem` shows Ready.
8. The `iac/ansible/` directory is removed from the `compute-infrastructure` repository.

---

## 2. Context and Prior Art

### 2.1 Existing Flux Installation

Flux is already running on this cluster. Evidence:

- `rke2/tooling/flux/tooling/gotk-components.yaml` — full Flux component manifest (source,
  kustomize, helm, image-reflector, image-automation controllers), namespace `tooling`.
- `rke2/tooling/flux/tooling/gotk-sync.yaml` — `GitRepository` named `tooling` pointing to
  `https://github.com/hwcopeland/iac.git`, with `secretRef: flux-system`. Also a
  `Kustomization` that reconciles `./rke2/tooling/flux` from that source.
- `rke2/tooling/flux/tooling/apps.yaml` — two `Kustomization` objects: `theswamp` and
  `image-automation`, both in the `tooling` namespace.
- Recent git log entries (`[ci skip] auto-update u4u-engine`) confirm the ImageUpdateAutomation
  loop is live and committing.

### 2.2 Existing Image Automation Pattern (tooling)

The `tooling` namespace provides the authoritative working template:

- **ImageRepository** (`image-repository.yaml`): polls
  `zot.hwcopeland.net/florida-man-bioscience/u4u-engine` every 1m, authenticates via
  `secretRef: zot-pull-secret`.
- **ImagePolicy** (`image-policy.yaml`): filters tags matching `^sha-[a-f0-9]+`, orders
  alphabetically ascending.
- **ImageUpdateAutomation** (`image-update-automation.yaml`): references the `tooling`
  `GitRepository` source, checks out `main`, commits with author `fluxcdbot`, pushes to
  `main`, updates path `./rke2/tooling/flux/theswamp`, strategy `Setters`.
- **Setters marker** in deployment YAML: `# {"$imagepolicy": "tooling:u4u-engine"}` on the
  image line. Flux rewrites the entire `image:` value when the policy resolves to a new tag.
- **ExternalSecret** (`zot-pull-secret.yaml`): creates `kubernetes.io/dockerconfigjson` from
  Bitwarden item UUID `766ec5c7-6aa8-419d-bb27-e5982872bc5b` in namespace `tooling`.

The `chem` namespace design mirrors this exactly, substituting `tooling` → `chem` and the
`florida-man-bioscience` images → `chem` images.

### 2.3 Existing Registry Auth Pattern

The `chem` namespace already has `k8s-jobs/zot-pull-secret.yaml` — an ExternalSecret that
creates a `kubernetes.io/dockerconfigjson` Secret named `zot-pull-secret` in namespace `chem`
from the same Bitwarden item. This is the **pod pull secret** (for kubelet to pull images).

Flux's `image-reflector-controller` also needs a `kubernetes.io/dockerconfigjson` Secret in
its own namespace to poll the registry for tags. In the `tooling` pattern, this is the
`zot-pull-secret.yaml` in the `image-automation/` subdirectory (namespace `tooling`).

For the `chem` namespace pattern, Flux image automation objects live in namespace `chem`.
The `image-reflector-controller` accesses the secret referenced by each `ImageRepository`'s
`spec.secretRef` in the namespace where the `ImageRepository` is created. Since we create
`ImageRepository` objects in namespace `chem`, the secret must also be in namespace `chem`.
The existing `k8s-jobs/zot-pull-secret.yaml` already satisfies this requirement — no
additional secret is needed.

### 2.4 CI Precedent (theswamp)

`rke2/theswamp/u4u-engine/.github/workflows/build-and-push-frontend.yml` shows the existing
CI pattern:
- Runner: `ubuntu-latest` (GitHub-hosted)
- Auth: `docker/login-action@v3` with `secrets.ZOT_USERNAME` and `secrets.ZOT_API_KEY`
- Metadata: `docker/metadata-action@v5` generating `sha-<short>` tags (via `type=sha,format=short`)
- Build: `docker/build-push-action@v6` with registry cache

The `chem` CI follows this pattern for all four images. The critical difference: all four
images are built from the same `compute-infrastructure` repository, so a single workflow file
(with a matrix strategy or four parallel jobs) is the correct structure.

### 2.5 GitOps Root Structure

`~/iac/rke2` (`github.com/hwcopeland/iac`) organizes cluster state by namespace:

```
rke2/
  tooling/flux/         — Flux bootstrap + tooling namespace automation
  chem/                 — chem namespace YAMLs (jupyter, etc.)
  external-secrets/     — ExternalSecrets infrastructure
  kube-system/          — cert-manager, ingress
  ...
```

The `flux/` directory at `rke2/flux/` is currently empty (created, no content). The `chem`
Flux manifests belong at `rke2/chem/flux/` to keep them co-located with other chem namespace
resources. The top-level `rke2/flux/` directory can serve as the entry point for bootstrapping
Flux itself if it is ever re-installed (analogous to `rke2/tooling/flux/tooling/`).

---

## 3. Alternatives Considered

### Option A: Flux Watches `compute-infrastructure` Directly (Two GitRepository Sources)

Flux would have a second `GitRepository` pointing to
`github.com/Khemiatic-Energistics/compute-infrastructure` in addition to the existing
`tooling` one pointing to `hwcopeland/iac`. The Kustomization for `chem` would reference
`compute-infrastructure` as its source and apply `k8s-jobs/` directly.

**Strengths:**
- Manifest changes in `compute-infrastructure` go live automatically without any copy/sync step.
- Single source of truth for Kubernetes manifests (they live where the code lives).
- ImageUpdateAutomation writes updated tags back to `compute-infrastructure`, which is where
  the Dockerfiles also live — full loop in one repo.

**Weaknesses:**
- Requires a second `GitRepository` CRD with GitHub authentication credentials stored in a
  Kubernetes Secret (likely `flux-github` or similar) that can read
  `Khemiatic-Energistics/compute-infrastructure`. This is a new secret surface.
- ImageUpdateAutomation writes commits to `compute-infrastructure`, meaning the Flux bot needs
  write access to that repository too. This blurs the CI/GitOps boundary: CI pushes code to
  `compute-infrastructure`, and Flux also commits to it. Managing the commit author, branch,
  and avoiding CI re-trigger loops (`[ci skip]` on Flux commits) requires extra care.
- The `hwcopeland/iac` repo (`rke2/`) is the documented single source of truth for cluster
  state. Splitting cluster state across two repos (manifests in `compute-infrastructure`,
  everything else in `hwcopeland/iac`) violates this principle.

**Verdict:** Rejected. Two writeable Git sources create a split-brain risk and add operational
complexity without meaningful benefit for a single-person lab environment.

### Option B: Manifests Copied/Vendored into `hwcopeland/iac` (Selected)

The `k8s-jobs/` Kustomize tree from `compute-infrastructure` is mirrored into
`rke2/chem/flux/k8s-jobs/` in the `hwcopeland/iac` repo. The CI workflow in
`compute-infrastructure` commits any manifest changes to `hwcopeland/iac` as part of the build
pipeline (via `gh` CLI or `git push` to the iac repo). Alternatively, the manifests are simply
maintained in both repos at development time — they are nearly static (the only dynamic
content is the image tag, updated by Flux).

**Strengths:**
- `hwcopeland/iac` remains the single GitOps root. Flux watches exactly one repo.
- ImageUpdateAutomation writes commits to `hwcopeland/iac` only — the existing flux-system
  secret already has write access.
- Clean separation: `compute-infrastructure` owns code and Dockerfiles; `hwcopeland/iac`
  owns cluster state.
- No new GitHub credentials or `GitRepository` objects required.

**Weaknesses:**
- Manifest changes require a sync step. In practice, the manifests in `k8s-jobs/` are stable
  (CRD, RBAC, deployment structure); only the image tag changes, and that is handled by Flux
  automatically. Manual manifest edits require a deliberate copy step.
- Slight duplication of the kustomization.yaml and supporting YAMLs.

**Verdict:** Selected. Consistent with the existing `tooling` pattern. The duplication cost is
low because the manifests are nearly static. The image tag is the only value that changes, and
Flux owns that update.

### Option C: Manual Bootstrap + Managed Manifests (No GitOps for Manifests)

Keep using `kubectl apply -k k8s-jobs/` for structural changes (CRD, RBAC, deployment
resource definitions), and only automate the image tag update via Flux. The Kustomization
object is not used.

**Verdict:** Rejected. This is the current state — it leaves deployment of structural changes
manual and does not achieve GitOps for the full namespace.

---

## 4. Architecture and System Design

### 4.1 High-Level Data Flow

```
[developer pushes to main]
        |
        v
GitHub Actions (compute-infrastructure repo)
  - Build 4 Docker images
  - Tag: sha-<short-sha>
  - Push to zot.hwcopeland.net/chem/
        |
        v
zot.hwcopeland.net/chem/
  - docking-controller:<sha>
  - autodock4:<sha>
  - autodock-vina:<sha>
  - jupyter-chem:<sha>
        |
        v (polling, 1m interval)
Flux image-reflector-controller (tooling ns)
  4x ImageRepository objects (in chem ns)
  4x ImagePolicy objects (in chem ns)
        |
        v (new tag detected)
Flux image-automation-controller (tooling ns)
  4x ImageUpdateAutomation objects (in chem ns)
        |
        v (git commit)
hwcopeland/iac (github.com)
  rke2/chem/flux/k8s-jobs/config/deployment.yaml
    image: zot.hwcopeland.net/chem/docking-controller:sha-<new>
  (other manifests updated as needed)
        |
        v (source reconcile, 1m interval)
Flux source-controller detects commit
        |
        v
Flux kustomize-controller
  Kustomization: chem (in chem ns)
  Applies: rke2/chem/flux/k8s-jobs/
        |
        v
Kubernetes API Server
  chem namespace: CRD, RBAC, Deployment updated
  Pods restart with new image
```

### 4.2 Namespace Strategy

All Flux objects (`ImageRepository`, `ImagePolicy`, `ImageUpdateAutomation`, `Kustomization`,
`GitRepository`) are placed in namespace `chem`. This follows the principle used in the
`tooling` namespace: automation objects co-located with the workload namespace they govern.

The Flux controllers themselves (`image-reflector-controller`, `image-automation-controller`,
`kustomize-controller`, `source-controller`) run in namespace `tooling` (where `gotk-components`
installed them) and act cluster-wide via ClusterRoleBindings.

### 4.3 GitRepository Object

A `GitRepository` named `iac` in namespace `chem` points to `https://github.com/hwcopeland/iac.git`,
using the existing `flux-system` GitHub secret (already present from the tooling bootstrap).

The `tooling` namespace already has a `GitRepository` named `tooling` pointing to this same
repo. Rather than creating a second GitRepository for the same URL (which would result in two
independent polls of the same repo), the `chem` `Kustomization` can reference the `tooling`
namespace's `GitRepository` via a cross-namespace `sourceRef`. Flux supports this with an
explicit `namespace` field on the `sourceRef`.

Cross-namespace sourceRef requires the GitRepository's namespace to permit access. Flux permits
this by default when using the same cluster-scoped controllers.

**Decision:** The `chem` Kustomization references `sourceRef: kind: GitRepository, name: tooling,
namespace: tooling`. This avoids creating a duplicate GitRepository object.

### 4.4 Manifest Layout in `hwcopeland/iac`

```
rke2/chem/flux/
  kustomization.yaml          — Kustomize root: references all subdirs
  gitrepository.yaml          — (optional) GitRepository if cross-ns sourceRef not used
  apps.yaml                   — Kustomization CRD objects for kustomize-controller
  k8s-jobs/                   — Mirror of k8s-jobs/ from compute-infrastructure
    kustomization.yaml
    crd/
      dockingjob-crd.yaml
    config/
      deployment.yaml         — Contains $imagepolicy setter markers
      rbac.yaml
    zot-pull-secret.yaml      — ExternalSecret (same as in compute-infrastructure)
  image-automation/
    kustomization.yaml
    zot-pull-secret.yaml      — ExternalSecret for image-reflector-controller in chem ns
                                 (same Bitwarden UUID — already exists as chem ns secret)
    image-repository-docking-controller.yaml
    image-repository-autodock4.yaml
    image-repository-autodock-vina.yaml
    image-repository-jupyter-chem.yaml
    image-policy-docking-controller.yaml
    image-policy-autodock4.yaml
    image-policy-autodock-vina.yaml
    image-policy-jupyter-chem.yaml
    image-update-automation.yaml  — Single automation updating k8s-jobs/config/deployment.yaml
```

Note on the ExternalSecret for Flux: The existing `k8s-jobs/zot-pull-secret.yaml` creates a
`zot-pull-secret` in namespace `chem`. This is the same secret that `ImageRepository` objects
in namespace `chem` will reference for registry polling. No separate ExternalSecret is needed
in the `image-automation/` subdirectory — the existing one already satisfies the requirement.
The `image-automation/` subdirectory therefore does NOT need its own `zot-pull-secret.yaml`.

### 4.5 Kustomization Object for chem Namespace

```yaml
# apps.yaml — placed at rke2/chem/flux/
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: chem
  namespace: chem
spec:
  interval: 5m
  path: ./rke2/chem/flux/k8s-jobs
  prune: true
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: chem-image-automation
  namespace: chem
spec:
  interval: 5m
  path: ./rke2/chem/flux/image-automation
  prune: true
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
```

The `chem-image-automation` Kustomization applies the `ImageRepository`, `ImagePolicy`, and
`ImageUpdateAutomation` CRD objects from the `image-automation/` directory. The `chem`
Kustomization applies the namespace's Kubernetes workload resources from `k8s-jobs/`.

These two Kustomizations must be applied to the cluster. They live in `rke2/chem/flux/apps.yaml`.
That file must itself be referenced. The `tooling` namespace's `apps.yaml` currently only lists
`theswamp` and `image-automation` (the tooling-namespace versions). The `chem` Kustomizations
need to be added to the cluster. There are two sub-options:

1. Add them to `rke2/tooling/flux/tooling/apps.yaml` (expanding that file to manage cross-ns
   kustomizations).
2. Place a new entry in `rke2/chem/flux/` and add a Kustomization to the `tooling` sync that
   points at `rke2/chem/flux/`.

**Decision:** Add a new `Kustomization` entry to `rke2/tooling/flux/tooling/apps.yaml` for
`chem-bootstrap` pointing at `./rke2/chem/flux` (path in `hwcopeland/iac`). This follows the
same pattern as the existing `theswamp` and `image-automation` entries. The
`chem-bootstrap` Kustomization is in namespace `tooling` (because that is where the Flux
Kustomize controller runs its reconcile from). The resources it applies (the `chem` and
`chem-image-automation` Kustomization CRDs) are namespaced to `chem` via their manifest
metadata.

### 4.6 Bootstrap Kustomization in tooling/apps.yaml

The `tooling` namespace's `apps.yaml` file gets one new entry:

```yaml
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: chem-bootstrap
  namespace: tooling
spec:
  interval: 5m
  path: ./rke2/chem/flux
  prune: true
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
```

This causes the tooling Kustomize controller to apply everything in `rke2/chem/flux/`, which
includes `apps.yaml` (the two `chem` and `chem-image-automation` Kustomizations) and the
Kustomize root (`kustomization.yaml`).

---

## 5. Data Models and Storage

### 5.1 Image Tag Format

All four images use the same tag convention, matching the existing `tooling` pattern:

```
sha-<short-sha>
```

Where `<short-sha>` is the first 7 characters of the commit SHA, produced by
`docker/metadata-action@v5` with `type=sha,format=short`.

A push to `main` at commit `abc1234def` produces tags:
- `zot.hwcopeland.net/chem/docking-controller:sha-abc1234`
- `zot.hwcopeland.net/chem/autodock4:sha-abc1234`
- `zot.hwcopeland.net/chem/autodock-vina:sha-abc1234`
- `zot.hwcopeland.net/chem/jupyter-chem:sha-abc1234`

Additionally, the `latest` tag is pushed alongside (enabled when `is_default_branch` is true).
This preserves backward compatibility with any manual `imagePullPolicy: Always` references.

### 5.2 ImagePolicy Filter

```yaml
filterTags:
  pattern: '^sha-[a-f0-9]+'
policy:
  alphabetical:
    order: asc
```

This is identical to the tooling pattern. Alphabetical ascending on `sha-` prefixed tags
selects the lexicographically largest SHA as "latest". Note: this assumes SHAs are not
reused and the lexicographic order approximates chronological order well enough for this
workload. In practice, since all four images are built from the same commit, their tags are
always identical SHAs and the ordering is deterministic.

An alternative is `numerical` ordering using a run-number prefix (`sha-<run_number>-<sha>`),
as documented in the existing `image-policy.yaml` comment in the `tooling` namespace. This
guarantees strict chronological order. For the `chem` pipeline, the simpler alphabetical
approach is used first — it can be upgraded to numerical ordering if needed.

### 5.3 Setter Markers in deployment.yaml

Flux's `Setters` strategy rewrites image references annotated with `$imagepolicy` comments.

The `k8s-jobs/config/deployment.yaml` in `hwcopeland/iac`'s copy (`rke2/chem/flux/k8s-jobs/`)
must have the marker on the controller image line:

```yaml
image: zot.hwcopeland.net/chem/docking-controller:sha-<initial>  # {"$imagepolicy": "chem:docking-controller"}
```

The `autodock4`, `autodock-vina`, and `jupyter-chem` images are used by batch Jobs created
dynamically by the controller at runtime (hardcoded as the `DefaultImage` constant in
`main.go`). They are not referenced in any static deployment manifest. The ImagePolicy for
these images will still track available tags in the registry, but ImageUpdateAutomation
cannot update a manifest that does not exist.

**Two options for the worker images:**

**Option A (Setters only):** Update the `DefaultImage` constant in the controller's Go source
code to be sourced from an environment variable, then add an env var to the deployment YAML
with the `$imagepolicy` setter marker. This is the cleanest GitOps approach but requires a
small controller code change.

**Option B (Track only):** Create `ImageRepository` and `ImagePolicy` objects for the three
worker images but no `ImageUpdateAutomation` for them. The CI build tags and pushes them.
The controller continues using `:latest` or a manually-specified image. The ImagePolicy
objects provide visibility (operators can see the latest available tag) without requiring
code changes.

**Decision for Phase 1:** Option B (track only) for worker images. Phase 2 upgrades to
Option A after the controller code change. This keeps Phase 1 minimal and avoids blocking
the CI/GitOps pipeline on a controller refactor.

For Phase 1, the `ImageUpdateAutomation` only updates `deployment.yaml` with the
`docking-controller` image. The worker image policies exist for observability.

### 5.4 Kubernetes Resources Created

| Resource | Kind | Namespace | Count |
|---|---|---|---|
| `GitRepository` reference | (cross-ns, existing) | tooling | 0 new |
| `Kustomization` (chem workloads) | kustomize.toolkit.fluxcd.io/v1 | chem | 1 |
| `Kustomization` (chem image-automation) | kustomize.toolkit.fluxcd.io/v1 | chem | 1 |
| `Kustomization` (chem-bootstrap) | kustomize.toolkit.fluxcd.io/v1 | tooling | 1 |
| `ImageRepository` | image.toolkit.fluxcd.io/v1beta2 | chem | 4 |
| `ImagePolicy` | image.toolkit.fluxcd.io/v1beta2 | chem | 4 |
| `ImageUpdateAutomation` | image.toolkit.fluxcd.io/v1beta2 | chem | 1 |

---

## 6. API Contracts

### 6.1 ImageRepository Schema

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

The `secretRef` references the `zot-pull-secret` Secret in namespace `chem`, which is
materialized by the existing ExternalSecret in `k8s-jobs/zot-pull-secret.yaml`. This secret
is type `kubernetes.io/dockerconfigjson` and contains credentials for
`zot.hwcopeland.net`.

Identical structure for `autodock4`, `autodock-vina`, `jupyter-chem`.

### 6.2 ImagePolicy Schema

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

Identical structure for all four images.

### 6.3 ImageUpdateAutomation Schema

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageUpdateAutomation
metadata:
  name: chem
  namespace: chem
spec:
  interval: 1m0s
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
  git:
    checkout:
      ref:
        branch: main
    commit:
      author:
        email: fluxcdbot@users.noreply.github.com
        name: fluxcdbot
      messageTemplate: |
        [ci skip] auto-update chem
    push:
      branch: main
  update:
    path: ./rke2/chem/flux/k8s-jobs
    strategy: Setters
```

The `[ci skip]` prefix in the commit message prevents GitHub Actions from re-triggering on
the Flux commit (the same convention used by the existing `tooling` automation).

The cross-namespace `sourceRef` (referencing `tooling` namespace's `GitRepository`) requires
that the `image-automation-controller` has permission to read GitRepository objects across
namespaces. The existing RBAC from `gotk-components.yaml` grants
`image-automation-controller` a ClusterRole that includes this.

### 6.4 Kustomization Schema

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: chem
  namespace: chem
spec:
  interval: 5m
  path: ./rke2/chem/flux/k8s-jobs
  prune: true
  sourceRef:
    kind: GitRepository
    name: tooling
    namespace: tooling
```

`prune: true` means Flux will delete resources from the cluster that are removed from the
manifest. This is safe for the CRD, RBAC, and Deployment — it is intentional behavior for
a GitOps-managed namespace.

### 6.5 GitHub Actions Workflow Schema

The workflow file lives at `.github/workflows/build-and-push.yml` in the
`compute-infrastructure` repository.

```yaml
name: build and push chem images
on:
  push:
    branches: [main]

jobs:
  build-docking-controller:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - name: Log in to zot registry
        uses: docker/login-action@v3
        with:
          registry: zot.hwcopeland.net
          username: ${{ secrets.ZOT_USERNAME }}
          password: ${{ secrets.ZOT_API_KEY }}
      - name: Extract image metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: zot.hwcopeland.net/chem/docking-controller
          tags: |
            type=sha,format=short
            type=raw,value=latest,enable={{is_default_branch}}
      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: ./k8s-jobs
          file: ./k8s-jobs/controller/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=registry,ref=zot.hwcopeland.net/chem/docking-controller:cache
          cache-to: type=registry,ref=zot.hwcopeland.net/chem/docking-controller:cache,mode=max

  build-autodock4:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - name: Log in to zot registry
        uses: docker/login-action@v3
        with:
          registry: zot.hwcopeland.net
          username: ${{ secrets.ZOT_USERNAME }}
          password: ${{ secrets.ZOT_API_KEY }}
      - name: Extract image metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: zot.hwcopeland.net/chem/autodock4
          tags: |
            type=sha,format=short
            type=raw,value=latest,enable={{is_default_branch}}
      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: ./docker/autodock4
          file: ./docker/autodock4/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=registry,ref=zot.hwcopeland.net/chem/autodock4:cache
          cache-to: type=registry,ref=zot.hwcopeland.net/chem/autodock4:cache,mode=max

  # identical structure for autodock-vina and jupyter-chem jobs
```

The four jobs run in parallel (no `needs:` dependency). Each uses the same auth/metadata/build
pattern. GitHub repository secrets `ZOT_USERNAME` and `ZOT_API_KEY` must be configured in
`Khemiatic-Energistics/compute-infrastructure` (same keys as in the `theswamp/u4u-engine`
workflow, which already uses them — the Zot credentials are shared across repositories for
the same registry user).

**Dockerfile build contexts:**

| Image | Context | Dockerfile |
|---|---|---|
| `docking-controller` | `./k8s-jobs` | `./k8s-jobs/controller/Dockerfile` |
| `autodock4` | `./docker/autodock4` | `./docker/autodock4/Dockerfile` |
| `autodock-vina` | `./docker/autodock-vina` | `./docker/autodock-vina/Dockerfile` |
| `jupyter-chem` | `./rke2/jupyter/docker` | `./rke2/jupyter/docker/Dockerfile` |

Note: the `docking-controller` Dockerfile uses `COPY controller/ .` and `COPY controller/go.mod ...`
with paths relative to the build context. The build context must be `./k8s-jobs` (not
`./k8s-jobs/controller/`) because the Dockerfile copies from subdirectories of the context.
This matches the current manual build command documented in `docs/spec/operations.md`:
`docker build -t ... -f controller/Dockerfile k8s-jobs/`.

---

## 7. Migration and Rollout

### 7.1 Pre-Conditions

Before any Flux manifests are applied:

1. The `chem` namespace must exist in the cluster (it does — the controller is already deployed).
2. The `zot-pull-secret` ExternalSecret must be healthy in namespace `chem` (it is — the
   existing ExternalSecret is functional).
3. GitHub repository secrets `ZOT_USERNAME` and `ZOT_API_KEY` must be set in
   `Khemiatic-Energistics/compute-infrastructure`. These are the same credentials used by
   `theswamp/u4u-engine`. Verify via the GitHub UI or `gh secret list -R Khemiatic-Energistics/compute-infrastructure`.
4. The `flux-system` GitHub secret must exist in namespace `tooling` (it does — the tooling
   GitRepository already uses it and is working).

### 7.2 Rollout Phases

See §11 (Implementation Phases) for the full breakdown. The high-level sequence:

**Phase 1: CI Pipeline**
Add `.github/workflows/build-and-push.yml` to `compute-infrastructure`. Push to `main`.
Verify all four images appear in `zot.hwcopeland.net/chem/`.

**Phase 2: Flux Manifests in `hwcopeland/iac`**
Create `rke2/chem/flux/` with the ImageRepository, ImagePolicy, ImageUpdateAutomation,
and Kustomization objects. Add `chem-bootstrap` entry to `rke2/tooling/flux/tooling/apps.yaml`.
Push to `main` in `hwcopeland/iac`. Flux reconciles and creates the objects.

**Phase 3: Verify Automation Loop**
Trigger a new push to `compute-infrastructure/main`. Verify Flux detects the new image tag
and commits an update to `hwcopeland/iac`. Verify the `chem` Kustomization reconciles and
the controller pod restarts with the new image.

**Phase 4: Delete `iac/ansible/`**
After Phase 3 is verified, open a PR in `compute-infrastructure` that removes the `iac/ansible/`
directory. This eliminates the committed RKE2 join token. Note: deleting the directory from
the repository does not remove the token from git history — a `git filter-repo` or equivalent
history rewrite is required to fully remediate the exposure.

### 7.3 Rollback Plan

Rollback for a bad image: revert the Flux-committed `[ci skip] auto-update chem` commit in
`hwcopeland/iac`. Flux will reconcile back to the previous image tag on the next interval.

Rollback for a bad manifest change: revert the relevant commit in `rke2/chem/flux/k8s-jobs/`
and push. Flux reconciles.

Flux itself does not need to be rolled back in normal operation. If Flux is broken, apply
the last known-good `gotk-components.yaml` from `rke2/tooling/flux/tooling/`.

### 7.4 Breaking Changes

- **Image tag change:** The controller is currently deployed with tag `:latest`. After
  GitOps is active, the deployment will be updated to `sha-<first-sha>`. The pod will restart.
  This is safe — the controller handles normal pod restart correctly.
- **`iac/ansible/` deletion:** No Kubernetes resource depends on the Ansible directory.
  Deletion is a repository-only change. Bare-metal re-provisioning procedures will need to
  be referenced from historical git commits or separately documented.

---

## 8. Risks and Open Questions

### 8.1 Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `zot-pull-secret` in `chem` ns is not of type `dockerconfigjson` | Low | High | Verify via `kubectl get secret zot-pull-secret -n chem -o jsonpath='{.type}'` before applying Flux manifests |
| Flux `image-automation-controller` lacks RBAC to write cross-namespace GitRepository | Low | High | The existing `gotk-components.yaml` grants ClusterRole — check if it includes `gitrepositories` across namespaces |
| GitHub Actions workflow fails due to missing `ZOT_USERNAME`/`ZOT_API_KEY` secrets | Medium | Medium | Verify secrets are configured in `Khemiatic-Energistics/compute-infrastructure` repo before the first push |
| autodock4 image build is slow (large Ubuntu base + binary downloads) | High | Low | Layer caching via `cache-from/cache-to` to the registry mitigates repeat builds; first build may take 10-20 minutes |
| `jupyter-chem` Dockerfile may not exist | Medium | Medium | `rke2/jupyter/docker/Dockerfile` was found in the file tree — verify it is a valid, buildable Dockerfile |
| `[ci skip]` not honored by GitHub Actions | Low | Medium | GitHub Actions honors `[ci skip]` and `[skip ci]` natively since 2021; verify in the repo's Actions settings |
| Alphabetical SHA ordering is non-deterministic for rollback | Low | Low | Document that rollback is via git revert, not image policy reversal; upgrade to numerical ordering in Phase 2 if needed |
| `iac/ansible/` deletion removes operator's only documented provisioning path | Medium | Medium | Document the Ansible procedure in `docs/iac/ansible.md` or ensure the git history is accessible before deletion |

### 8.2 Open Questions

1. **Is the `flux-system` secret in namespace `tooling` a basic-auth GitHub token or an SSH
   deploy key?** This determines whether the `chem` Kustomizations can cross-reference it.
   If it is namespace-scoped, cross-namespace access from `chem` would need a copy. The
   `ImageUpdateAutomation` references it via the `GitRepository` object in `tooling` ns, which
   already has the `secretRef`. This is expected to work, but should be confirmed when
   `ImageUpdateAutomation` is first applied.

2. **Does the Flux image-automation-controller have a valid SSH or HTTPS push credential for
   `hwcopeland/iac`?** The current tooling automation is committing successfully (`[ci skip]`
   commits visible in git log), so this is expected to be already configured. Confirm by
   checking `rke2/tooling/flux/tooling/gotk-sync.yaml` for the `secretRef` name and verifying
   the referenced Secret has write permission.

3. **Should `jupyter-chem` builds be path-filtered?** The autodock4 and autodock-vina images
   only change when their respective `docker/` directories change. Building all four images on
   every push is correct for Phase 1 (simplicity) but adds ~15-20 minutes of unnecessary build
   time when only the controller changes. A path filter (`paths:`) on each job reduces CI time.
   Deferred to Phase 2.

4. **Should `iac/ansible/` deletion rewrite git history?** The committed RKE2 join token is
   the primary reason to delete the directory. Deleting the file does not remove it from history.
   A `git filter-repo --path iac/ansible/ --invert-paths` rewrite would remove it from all
   commits. This requires a force-push to `main`, which disrupts any open branches. Flag this
   decision for the operator: accepting historical exposure vs. rewriting history.

---

## 9. Testing Strategy

### 9.1 CI Pipeline Verification

After Phase 1 (CI workflow added):
- Trigger a push to `main` in `compute-infrastructure`.
- Verify all four jobs succeed in the GitHub Actions UI.
- Verify all four images appear in the Zot registry:
  ```bash
  curl -s -u $ZOT_USER:$ZOT_PASS https://zot.hwcopeland.net/v2/chem/docking-controller/tags/list
  curl -s -u $ZOT_USER:$ZOT_PASS https://zot.hwcopeland.net/v2/chem/autodock4/tags/list
  curl -s -u $ZOT_USER:$ZOT_PASS https://zot.hwcopeland.net/v2/chem/autodock-vina/tags/list
  curl -s -u $ZOT_USER:$ZOT_PASS https://zot.hwcopeland.net/v2/chem/jupyter-chem/tags/list
  ```

### 9.2 Flux Object Health Verification

After Phase 2 (Flux manifests applied):
```bash
# ImageRepositories
kubectl get imagerepositories -n chem
# Expected: READY=True for all four, LAST SCAN populated

# ImagePolicies
kubectl get imagepolicies -n chem
# Expected: READY=True, LATEST IMAGE shows sha-<commit>

# ImageUpdateAutomation
kubectl get imageupdateautomation -n chem
# Expected: READY=True

# Kustomizations
kubectl get kustomization -n chem
# Expected: chem and chem-image-automation READY=True

kubectl get kustomization -n tooling
# Expected: chem-bootstrap READY=True
```

### 9.3 End-to-End Automation Loop Verification

After Phase 3 (end-to-end):
1. Record the current SHA in `deployment.yaml` in `hwcopeland/iac`.
2. Push any change to `compute-infrastructure/main`.
3. Wait up to 5 minutes. Check `hwcopeland/iac` git log for a `[ci skip] auto-update chem`
   commit.
4. Verify the `deployment.yaml` image tag was updated.
5. Wait up to 10 minutes. Verify the `chem` Kustomization reconciled:
   ```bash
   kubectl describe kustomization chem -n chem | grep -E "Last Applied|Revision"
   kubectl get pods -n chem -l app.kubernetes.io/name=docking-controller
   # Verify pod has new image:
   kubectl get pod -n chem -l app.kubernetes.io/name=docking-controller \
     -o jsonpath='{.items[0].spec.containers[0].image}'
   ```

### 9.4 Controller Health After Tag Update

After the pod restarts:
- `GET /health` returns `{"status":"healthy"}` (liveness probe passes within 30s).
- `GET /readyz` returns `{"status":"ready"}` (readiness probe passes within 10s).
- No error-level log entries in `kubectl logs -n chem deployment/docking-controller`.

---

## 10. Observability and Operational Readiness

### 10.1 Flux Status Signals

| Signal | Command | Expected |
|---|---|---|
| GitRepository synced | `kubectl get gitrepository tooling -n tooling` | READY=True, REVISION=main/... |
| Kustomization reconciled | `kubectl get kustomization chem -n chem` | READY=True, no error |
| ImageRepository polled | `kubectl get imagerepo docking-controller -n chem` | READY=True, LAST SCAN < 2m ago |
| ImagePolicy resolved | `kubectl get imagepolicy docking-controller -n chem` | READY=True, LATEST IMAGE = sha-... |
| ImageUpdateAutomation ran | `kubectl get imageupdateautomation chem -n chem` | LAST RUN within expected interval |

### 10.2 Diagnosing Stalled Automation

If Flux stops committing tag updates:

```bash
# Check image-automation-controller logs
kubectl logs -n tooling deployment/image-automation-controller --tail=50

# Force reconcile the ImageUpdateAutomation
flux reconcile image update chem -n chem

# Check if Flux can authenticate to GitHub
kubectl describe gitrepository tooling -n tooling | grep -A5 "Conditions:"

# Check if image-reflector can reach zot
kubectl logs -n tooling deployment/image-reflector-controller --tail=50
```

If the Kustomization is not reconciling:

```bash
# Force reconcile
flux reconcile kustomization chem -n chem

# Check for errors in the kustomize-controller
kubectl logs -n tooling deployment/kustomize-controller --tail=50

# Check if the ExternalSecret is healthy (zot-pull-secret dependency)
kubectl describe externalsecret zot-pull-secret -n chem
```

### 10.3 Production Readiness Criteria

This design is production-ready when:
- All four ImageRepository objects show `READY=True` consistently.
- At least one full automation loop (push → tag update → manifest commit → kustomization
  reconcile → pod restart) has completed successfully without manual intervention.
- The `docking-controller` pod health probes pass after the automated restart.
- The `iac/ansible/` directory has been removed and the operator has confirmed the
  provisioning procedure is documented elsewhere (or accepted that historical context
  is in git log).

### 10.4 3am Diagnosability

If called at 3am because the controller is not responding after an automated update:

1. Check if the pod is running: `kubectl get pods -n chem`.
2. If `ImagePullBackOff`: the Zot pull secret has expired or the image tag is wrong.
   `kubectl describe pod <name> -n chem` will show the exact error.
3. If `CrashLoopBackOff`: the new image is broken. Check logs:
   `kubectl logs -n chem deployment/docking-controller --previous`.
4. Rollback: revert the latest `[ci skip] auto-update chem` commit in `hwcopeland/iac` and
   push. Flux will reconcile to the previous image within 5 minutes.
5. Pause automation: `kubectl patch imageupdateautomation chem -n chem \
   --type=merge -p '{"spec":{"suspend":true}}'` to stop further automated updates until
   the issue is resolved.

---

## 11. Implementation Phases

### Phase 1: CI Pipeline (Complexity: S)

**Owner:** @senior-engineer
**Deliverable:** `.github/workflows/build-and-push.yml` in `compute-infrastructure`

Tasks:
- Create `.github/workflows/build-and-push.yml` with four parallel jobs, one per image.
- Verify `ZOT_USERNAME` and `ZOT_API_KEY` repository secrets are set in
  `Khemiatic-Energistics/compute-infrastructure` (operator task).
- Push to `main`. Verify all four images appear in Zot.
- Confirm `sha-<sha>` tags are present alongside `latest`.

Dependencies: None (can start immediately).

### Phase 2: Flux Manifests in `hwcopeland/iac` (Complexity: S)

**Owner:** @senior-engineer
**Deliverable:** `rke2/chem/flux/` tree in `hwcopeland/iac`

Tasks:
- Create `rke2/chem/flux/` directory with `kustomization.yaml`, `apps.yaml`.
- Create `rke2/chem/flux/image-automation/` with 4x ImageRepository, 4x ImagePolicy, 1x
  ImageUpdateAutomation, and a `kustomization.yaml`.
- Copy `k8s-jobs/` structure into `rke2/chem/flux/k8s-jobs/`. Add `$imagepolicy` setter
  marker to `deployment.yaml` on the `docking-controller` image line.
- Add `chem-bootstrap` Kustomization entry to
  `rke2/tooling/flux/tooling/apps.yaml`.
- Push to `main` in `hwcopeland/iac`.

Dependencies: Phase 1 must be complete so there are valid tags in the registry for the
ImageRepository to resolve.

### Phase 3: Verification (Complexity: S)

**Owner:** Operator
**Deliverable:** Confirmed working end-to-end loop

Tasks:
- Verify all ImageRepository, ImagePolicy, Kustomization objects are READY.
- Trigger a push to `compute-infrastructure/main` (any trivial change).
- Confirm Flux commits an updated tag to `hwcopeland/iac` within 2 minutes.
- Confirm `chem` Kustomization reconciles and pod restarts within 10 minutes.
- Confirm controller health probes pass post-restart.

Dependencies: Phase 2 complete.

### Phase 4: Remove `iac/ansible/` (Complexity: S)

**Owner:** @senior-engineer
**Deliverable:** PR removing `iac/ansible/` from `compute-infrastructure`

Tasks:
- Open a PR that deletes the `iac/ansible/` directory.
- Ensure `docs/iac/ansible.md` is updated to reference git history as the record of the
  Ansible approach.
- Decide whether to rewrite git history to remove the committed RKE2 join token (operator
  decision — out of scope for this TDD but flagged as a prerequisite for full security
  remediation per `docs/spec/security.md` §1.1).
- Merge and verify no Kubernetes resources are affected.

Dependencies: Phase 3 verified.

### Phase 5: Worker Image Automation (Complexity: M) — Deferred

**Owner:** @senior-engineer
**Deliverable:** `docking-controller` reads worker image from env var; all four images
under `ImageUpdateAutomation`

Tasks:
- Modify `main.go` `DefaultImage` constant to be overridable via an environment variable
  (e.g., `AUTODOCK4_IMAGE`).
- Add `$imagepolicy` setter markers to the env var definition in `deployment.yaml`.
- Update `ImageUpdateAutomation` to cover the additional setter paths.
- Path-filter the CI workflow to skip worker image rebuilds when only the controller code changes.

Dependencies: Phase 3 verified. Controller code change required (new PR in `compute-infrastructure`).

---

## Appendix A: File Inventory — New Files Created

### In `compute-infrastructure` (github.com/Khemiatic-Energistics/compute-infrastructure)

```
.github/
  workflows/
    build-and-push.yml          — CI: build and push all 4 images to zot.hwcopeland.net/chem/
```

### In `hwcopeland/iac` (github.com/hwcopeland/iac)

```
rke2/chem/flux/
  kustomization.yaml            — Kustomize root listing: apps.yaml + image-automation/
  apps.yaml                     — Two Kustomization CRDs: chem and chem-image-automation

  k8s-jobs/
    kustomization.yaml          — Mirror of compute-infrastructure k8s-jobs/kustomization.yaml
    crd/
      dockingjob-crd.yaml       — Mirror
    config/
      deployment.yaml           — Mirror + $imagepolicy markers on docking-controller image
      rbac.yaml                 — Mirror
    zot-pull-secret.yaml        — Mirror (ExternalSecret for pod pull secret)

  image-automation/
    kustomization.yaml
    image-repository-docking-controller.yaml
    image-repository-autodock4.yaml
    image-repository-autodock-vina.yaml
    image-repository-jupyter-chem.yaml
    image-policy-docking-controller.yaml
    image-policy-autodock4.yaml
    image-policy-autodock-vina.yaml
    image-policy-jupyter-chem.yaml
    image-update-automation.yaml

rke2/tooling/flux/tooling/
  apps.yaml                     — MODIFIED: add chem-bootstrap Kustomization entry
```

### Deleted from `compute-infrastructure`

```
iac/ansible/                    — Entire directory (Phase 4)
```

---

## Appendix B: Tag Format Decision Log

The existing `tooling/flux/image-automation/image-policy.yaml` comment documents that the
tooling automation uses `^sha-[a-f0-9]+` with alphabetical ordering. This works for the
tooling workflow, and the same approach is used for `chem`. The comment in that file notes
that if a run-number prefix is desired (`sha-<run>-<sha>`), it enables numerical ordering.

For this TDD, the simpler format (`sha-<sha>`) is used. The trade-off: alphabetical SHA order
does not guarantee chronological order if two different commits produce SHAs where the later
SHA sorts lower. In practice, for a single-developer lab repo, this edge case is negligible.
The upgrade path to numerical ordering is documented in Phase 5 scope if it becomes an issue.

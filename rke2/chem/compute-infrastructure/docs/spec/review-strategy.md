---
project: "compute-infrastructure"
maturity: "experimental"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "Code and infrastructure review process, risk tiers, and manual quality gates for the AutoDock docking platform"
owner: "@staff-engineer"
dependencies:
  - security.md
  - operations.md
  - architecture.md
---

# Review Strategy

## Overview

This document establishes the review strategy for the `compute-infrastructure` project. It defines
what to review, how thoroughly, and why — calibrated to the actual risk profile of the codebase.

**What actually exists today:** No CI/CD pipeline. No PR templates. No automated linting, security
scanning, or test gates. No contribution guidelines. All review is manual and informal. This spec
creates a foundation from scratch, not an overlay on existing process.

The project is in active migration from Apache Airflow DAGs to a custom Go Kubernetes controller
(`k8s-jobs/`). Both paths coexist in production. There is a single environment — no staging, no
canary — meaning every change goes directly to a live cluster running real scientific workloads.

---

## Context: What Makes This Project Unusual

Before applying generic review heuristics, reviewers must internalize three properties of this
system that make ordinary review logic insufficient:

**1. Silent scientific incorrectness is the dominant failure mode.**
A crash or a pod that fails to start is recoverable. A docking run that completes successfully
but with wrong grid coordinates, wrong atom types, or a corrupted GPF file produces results that
look valid, appear in the output directory, and may be analyzed for days before the error is
noticed. There is no correctness assertion between "job exited 0" and "the science is right."
Reviewers of docking parameter logic — grid coords in `dockingv2.py`, atom type extraction in
`extract_atom_types()`, GPF/DPF generation in `prepare_gpf()`/`prepare_dpf()` — must treat these
as high-risk even when the code change is trivial.

**2. No CI gates exist. All quality enforcement is manual.**
There are no `.github/workflows` or equivalent. No linting, no test execution, no image scanning,
no secret scanning runs on push. This means the reviewer is the only line of defense before code
runs in production. Review must compensate for the absence of automated checks.

**3. Single production environment with bare-metal infrastructure.**
The cluster (RKE2, 5 nodes, bare-metal Ubuntu) has no equivalent staging environment. Ansible
playbooks that provision nodes or configure services are difficult to roll back and impossible to
test against a throwaway target. A misconfigured `rke2-server` role or a broken `kubernetes.yml`
playbook can require manual node recovery.

---

## Risk Tier Classification

Not all changes require the same scrutiny. The following tiers drive review depth and approval
requirements.

### Tier 1 — Critical (block on any blocker finding)

Changes where a mistake silently corrupts scientific output, exposes credentials, or makes the
cluster unrecoverable without manual intervention.

**Scope:**
- Controller job orchestration logic (`k8s-jobs/controller/main.go`): any change to
  `processDockingJob()`, the step sequencing (copy → receptor → split → prepare → dock → post),
  or the batch loop. A bug here affects every docking run.
- Docking parameter construction (`docker/autodock4/scripts/dockingv2.py`): GPF generation
  (`prepare_gpf`), DPF generation (`prepare_dpf`), atom type extraction (`extract_atom_types`),
  grid center reading (`read_grid_center`). Errors here produce wrong science without crashing.
- Shell command construction that incorporates user-supplied inputs. Specifically:
  - `createSplitSdfJob()` in `main.go` passes `job.Spec.LigandDb` directly into a shell
    command via `fmt.Sprintf`. Same pattern in `createCopyLigandDbJob()`.
  - `job-templates.yaml` template substitutions that land in `/bin/sh -c` `args` strings.
  - Any new code that passes CRD spec fields or API request fields into shell arguments.
- Kubernetes RBAC: any change to `k8s-jobs/config/rbac.yaml`. The current `Role` grants
  `create`/`delete` on `jobs`, which is the minimum viable scope — changes that widen this
  to `ClusterRole` or add additional resource verbs must be blocked pending architectural review.
- Ansible playbooks that touch bare-metal nodes: `iac/ansible/playbooks/kubernetes.yml`,
  `iac/ansible/roles/rke2-server/tasks/main.yaml`, `iac/ansible/roles/rke2-agent/tasks/main.yaml`,
  `iac/ansible/roles/rke2-common/tasks/main.yaml`. These run against production nodes and are
  hard to roll back.
- Any file that contains or handles secrets: `iac/ansible/roles/rke2-agent/defaults/main.yaml`,
  `iac/ansible/inventory/hwc_prox.yaml`, `k8s-jobs/zot-pull-secret.yaml`, vault-referenced files.

**Review requirement:** Two-person review when a second reviewer is available. Checklist
completion required (see Section: Checklists). No merge until all blockers are resolved.

### Tier 2 — High (blockers must be resolved; concerns require explicit justification)

Changes with meaningful operational risk or that affect workflow correctness in ways that are
observable (job fails, pod errors) but not silently wrong.

**Scope:**
- Controller API handlers (`k8s-jobs/controller/handlers.go`): input validation, job name
  generation, error handling, the `go h.controller.processDockingJob(job)` goroutine dispatch.
- CRD definition (`k8s-jobs/crd/dockingjob-crd.yaml`): schema changes, enum additions,
  required-field changes. The CRD is installed in the running cluster; incompatible changes
  require manual migration.
- Controller Deployment and Service manifests (`k8s-jobs/config/deployment.yaml`): resource
  limits, health probe configuration, image tags, `imagePullPolicy`.
- Protein and ligand preparation scripts (`docker/autodock4/scripts/proteinprepv2.py`,
  `docker/autodock4/scripts/ligandprepv2.py`): changes affect receptor preparation quality
  and can produce invalid PDBQT files that cause downstream silent failures.
- `split_sdf.sh`: the batch count returned by this script drives the loop count in
  `processDockingJob`. An off-by-one in the awk logic produces an incorrect number of batches
  (the current Airflow DAG has a `+ 1` in `get_batch_labels` that is marked with a `#maybe
  this will work?` comment — this is an unresolved correctness concern).
- Dockerfile changes (`docker/autodock4/Dockerfile`, `docker/autodock-vina/Dockerfile`,
  `k8s-jobs/controller/Dockerfile`): base image changes, new tool installations, run-as-user.
- Airflow DAG (`rke2/airflow/dags/autodock4.py`): any active changes during the migration
  period. The DAG and the controller must remain consistent in their parameter sets and step
  ordering.

**Review requirement:** Standard review with checklist completion. Concerns must be acknowledged
in the PR thread with justification or resolution before merge.

### Tier 3 — Medium (standard review; suggestions acceptable as follow-up)

Changes with limited blast radius, observable failure modes, and no silent incorrectness risk.

**Scope:**
- Storage and networking configs (`rke2/longhorn/`, `rke2/metallb/`): low change frequency,
  failures are visible.
- JupyterHub/Airflow Helm values (`rke2/jupyter/values.yaml`, `rke2/airflow/values.yaml`):
  affects research platform availability, not docking correctness.
- PVC definitions, non-RBAC Kubernetes manifests.
- Ansible roles for application deployment (backend, frontend) that do not touch cluster nodes.
- `go.mod`/`go.sum` changes: dependency additions should be assessed for security advisories
  and license compatibility.

**Review requirement:** Standard review. Suggestions may be deferred as follow-up issues.

### Tier 4 — Low (lightweight review; intent check sufficient)

Changes with minimal risk and easily verifiable correctness.

**Scope:**
- Documentation updates (`README.md`, `docs/`, `*.md` files) — verify accuracy, not policy.
- Comments and formatting.
- Non-functional kustomization changes (`k8s-jobs/kustomization.yaml`).

**Review requirement:** Intent check: does this match what the PR description says it does?
Approve if yes; flag inconsistencies.

---

## Review Dimensions

Applied by tier. Higher tiers activate more dimensions.

### Architecture (Tier 1, 2)

- Does this change fit the established pattern (controller orchestrates K8s Jobs via the Batch
  API; CRD is the declarative interface; API is the imperative interface)?
- Does it preserve the step ordering invariant: copy-ligand-db must precede prepare-receptor,
  which must precede split-sdf, which must precede all batch jobs?
- Does it avoid expanding the controller's scope beyond the `chem` namespace?
- For migration-period changes: are the Airflow DAG and the controller consistent in their
  parameter sets (pdbid, ligandDb, nativeLigand, ligandsChunkSize)?

### Security (Tier 1, 2, and any change touching secrets or shell invocation)

**Shell injection** is the highest-probability security issue in this codebase. Review any code
that passes user-supplied values into shell commands:

- In `main.go`, `LigandDb`, `PDBID`, `JupyterUser`, and other spec fields flow into
  `fmt.Sprintf(...)` strings that are passed as shell arguments. Verify inputs are validated
  in `handlers.go` before `processDockingJob()` is invoked. Currently, only `LigandDb` is
  validated as required; `PDBID` and others accept arbitrary strings.
- In `job-templates.yaml`, template substitutions land inside `/bin/sh -c` argument strings.
  Any change to how these values are populated must be assessed for injection.
- In docking scripts (`proteinprepv2.py`), `run_command(command)` executes shell strings
  directly with `shell=True`. Arguments come from `protein_id` and `ligand_id` parameters.

**Secret hygiene:**
- The RKE2 join token is committed in plaintext to `iac/ansible/roles/rke2-agent/defaults/main.yaml`.
  No new secrets may be committed in plaintext. This is a known existing debt, not a model
  to follow.
- Any PR that adds or modifies files containing API keys, tokens, passwords, or credentials must
  be blocked until secrets are moved to the Bitwarden/external-secrets pattern used by
  `k8s-jobs/zot-pull-secret.yaml`.

**Image pinning:**
- `alpine:latest` is used in the copy-ligand-db job container and in the controller Dockerfile's
  final stage. `golang:1.21-alpine` is used in the builder stage. These are unpinned.
  New container additions should use pinned digest tags; existing ones should be pinned as
  prioritized debt. Reviewers should flag any newly added `latest` tags as a blocker.

**RBAC least-privilege:**
- The controller's `Role` in `rbac.yaml` is namespace-scoped (`chem`) and covers the minimum
  verbs needed. Any expansion — new resource types, new verbs, ClusterRole elevation — is a
  Tier 1 concern requiring explicit justification.

### Operations (Tier 1, 2)

- **Rollback:** How is this change undone if it causes a production issue? For controller
  changes, is the previous image still available in Zot? For CRD changes, is the schema
  backward-compatible with existing DockingJob resources in the cluster?
- **Single-environment risk:** Because there is no staging, changes go directly to production.
  The reviewer must satisfy themselves that the change will not break running docking jobs.
  Prefer changes that are safe to deploy to a running controller without draining jobs.
- **Resource limits:** Docking job containers (prepare-ligands, docking, postprocessing)
  currently have no resource limits in job templates. New job types must specify `resources.requests`
  and `resources.limits`. Existing types without limits should be flagged as a concern.
- **TTL hygiene:** All jobs use `ttlSecondsAfterFinished: 300` (5 minutes). Changes that
  remove or reduce this to 0 should be flagged — orphaned jobs accumulate and consume etcd
  storage.
- **Reconcile loop stub:** `reconcileJobs()` in `main.go` returns `nil` immediately — it is
  a no-op. Any PR claiming to fix job lifecycle management must verify it actually implements
  this loop; the stub is not a partial implementation, it is a placeholder.
- **Health probes:** The controller deployment has liveness (`/health`) and readiness (`/readyz`)
  probes. Changes to probe paths or server routing must be verified against `deployment.yaml`.

### Domain Correctness (Tier 1, 2 — docking pipeline only)

This dimension has no equivalent in typical software review. It requires understanding of
the AutoDock4 workflow well enough to detect parameter errors that will not cause a crash.

Key invariants to verify when reviewing docking parameter changes:

- **Grid center derivation:** The grid center is calculated from the native ligand's center of
  mass (`calculate_grid_center()` in `proteinprepv2.py`). It is written to `grid_center.txt`
  and read back in `dockingv2.py`. Any change to this file path, format, or calculation is
  Tier 1 and requires verification that the coordinate system is preserved.
- **Atom type intersection:** `extract_atom_types()` intersects PDBQT atom types against a
  hardcoded `supported_atom_types` set. Changes to this set affect which atom types are
  included in the GPF/DPF files and can silently change docking behavior.
- **GPF npts:** Currently hardcoded as `60 60 60` in `prepare_gpf()`. This controls the grid
  box size. Any change must be explicitly flagged in the PR with the scientific rationale.
- **Batch count off-by-one:** The Airflow DAG's `get_batch_labels()` uses `range(batch_count + 1)`
  with a `#maybe this will work?` comment. The controller's `createSplitSdfJob()` hardcodes
  `return 5, nil` as a placeholder. These inconsistencies are known debt. Any PR touching
  batch counting logic must resolve the off-by-one question explicitly, not defer it.

When in doubt about a docking parameter change, flag it as a concern and ask the domain owner
to confirm the scientific intent. Do not approve docking parameter changes you cannot evaluate.

### Code Quality (Tier 2, 3)

- Go code in `k8s-jobs/controller/`: error handling patterns (are errors surfaced to callers?),
  goroutine leak risk (the `go h.controller.processDockingJob(job)` goroutine has no cancellation
  path), context propagation (are contexts passed to Kubernetes API calls?).
- Python scripts: use of `shell=True` in subprocess calls (flag as security concern),
  `sys.exit()` calls in library-style functions (should return errors, not exit), missing input
  validation on file paths derived from user inputs.
- Shell scripts: `set -e` usage (present in `split_sdf.sh` and `3_post_processing.sh` — verify
  it is preserved), quoting of variables, error handling.
- No linter configuration exists in the repo. Reviewer should not compensate for missing linter
  by performing style review — focus on logic, safety, and correctness.

### Testing (Tier 1, 2)

There are no automated tests in this repository. No unit tests, no integration tests, no
container tests (the `test/containers/` directory contains only a `.gitignore` and `setup.sh`).

Given the absence of tests, the reviewer's role is:

1. **Assess manual testability:** Can the change be manually verified before merge? If the change
   is to controller logic, can the reviewer or author deploy to the cluster and run a docking job
   to verify the behavior?
2. **Flag untestable changes:** Changes to logic that cannot be manually exercised without a full
   docking run (which may take hours) should be flagged. Consider requesting a minimal smoke-test
   path.
3. **Not prescribe specific tests:** The SDET is responsible for test implementation. The reviewer
   flags coverage gaps; they do not write or require specific test code.
4. **Watch for test debt accumulation:** New controller features, new job types, and new API
   endpoints should be flagged as creating test debt that is tracked for remediation.

---

## Checklists

Use these when reviewing Tier 1 and Tier 2 changes. They are not exhaustive — apply judgment.

### Controller Changes (main.go, handlers.go)

- [ ] Does `processDockingJob()` preserve step ordering (copy → receptor → split → prepare → dock → post)?
- [ ] Are all user-supplied CRD spec fields validated before they reach shell command construction?
- [ ] Does the goroutine in `CreateJob` (`go h.controller.processDockingJob`) have a mechanism
      for reporting failure back to the caller?
- [ ] Are Kubernetes API calls using the request context (not `context.TODO()` in new code)?
- [ ] Is `reconcileJobs()` still a stub? If a PR claims to implement it, verify the implementation.
- [ ] Are new K8s jobs created with resource limits defined?
- [ ] Are new K8s jobs using pinned image tags, not `latest`?

### RBAC Changes (rbac.yaml)

- [ ] Does the role remain namespace-scoped (Role, not ClusterRole)?
- [ ] Is every new verb and resource type explicitly justified in the PR?
- [ ] Does the service account binding remain within the `chem` namespace?

### Ansible Playbook Changes

- [ ] Does this change touch bare-metal node provisioning (rke2-server, rke2-agent, rke2-common)?
      If yes: has the playbook been reviewed for idempotency?
- [ ] Are there hardcoded IP addresses or tokens in the new code? (Flag as blocker.)
- [ ] Is the change safe to re-run against a running cluster?
- [ ] Is there a documented rollback step in the PR?

### Docking Script Changes (dockingv2.py, proteinprepv2.py, ligandprepv2.py)

- [ ] Does the change preserve grid center derivation and file path contract?
- [ ] Does the change preserve the atom type intersection against `supported_atom_types`?
- [ ] Are GPF/DPF parameters preserved or is the scientific rationale for changes documented?
- [ ] Are shell=True subprocess calls present? If yes, are inputs validated upstream?
- [ ] Has the domain owner confirmed the scientific correctness of parameter changes?

### Migration-Period Changes (any PR touching both Airflow and controller code)

- [ ] Are the Airflow DAG parameters and the controller DockingJobSpec consistent?
      (pdbid, ligandDb, jupyterUser, nativeLigand, ligandsChunkSize, image)
- [ ] Does the batch count logic agree between DAG and controller?
- [ ] If Airflow is being partially decommissioned, is the migration state documented?
- [ ] Are there running Airflow DAGs that would be disrupted by this change?

---

## What Constitutes a Blocker

The following are hard blockers — the PR must not merge until resolved:

1. **New plaintext secrets** committed to any file.
2. **Shell injection surface expanded** — new user input flowing into `fmt.Sprintf()` or
   `shell=True` subprocess calls without validation.
3. **RBAC scope widened** without explicit architectural review and justification.
4. **Step ordering violated** in `processDockingJob()` or equivalent orchestration logic.
5. **Newly introduced `latest` image tags** on new container definitions (existing ones are
   known debt).
6. **CRD schema breaking change** (removing required fields, changing enum values, narrowing
   types) without a migration plan.
7. **Ansible playbook that will fail on re-run** against a live node (non-idempotent task).
8. **Docking parameter change without domain owner confirmation** when the reviewer cannot
   evaluate scientific correctness.

---

## Active Migration Scrutiny

While the Airflow-to-controller migration is in progress, any PR that touches code on both
sides of the migration boundary requires elevated attention:

- The Airflow DAG (`rke2/airflow/dags/autodock4.py`) and the controller (`k8s-jobs/controller/`)
  must remain parameter-compatible. A user who knows both paths exist should be able to submit
  the same pdbid/ligandDb/nativeLigand values to either system and get scientifically equivalent
  results.
- The Airflow DAG has an unresolved batch count bug (the `+ 1` in `get_batch_labels`). The
  controller hardcodes `return 5, nil` in `createSplitSdfJob()`. These must converge before
  Airflow can be safely decommissioned. PRs should not deepen this divergence.
- Airflow infrastructure (`rke2/airflow/`) should not be decommissioned before the controller
  is verified to handle the same workloads end-to-end.

---

## Gaps and Known Debt

The following review infrastructure is absent and represents debt to address:

| Gap | Impact | Priority |
|-----|--------|----------|
| No CI/CD pipeline | No automated gates; reviewer is sole quality barrier | High |
| No secret scanning (e.g., gitleaks) | Plaintext token in repo went undetected | High |
| No automated tests | Manual testing only; docking correctness unverifiable at PR time | High |
| No container image scanning | Supply chain risk unquantified | Medium |
| No Go linting (golangci-lint) | Style/logic issues caught manually | Medium |
| No PR template | Review coverage is reviewer-dependent | Medium |
| No contribution guide | Onboarding friction; no documented workflow | Low |
| No OWNERS or CODEOWNERS file | No automatic reviewer assignment | Low |

Until a CI pipeline exists, each of the automated gate functions listed above is the manual
responsibility of the reviewer. This is not sustainable at scale and should be flagged as a
project risk in planning conversations.

---

## Approval Authority

There is currently no formal CODEOWNERS file or multi-approver requirement enforced by GitHub
branch protection. Until those controls are in place:

- Tier 1 changes: seek a second reviewer when one is available. Document single-reviewer merges
  as accepted risk in the PR thread.
- Tier 2 and below: single reviewer is sufficient.
- Self-merge: permitted only for Tier 4 (documentation, formatting). Not permitted for any
  code or infrastructure change.
- Emergency fixes (production down): permitted with single reviewer. Requires a follow-up PR
  within 48 hours adding the review commentary that was skipped.

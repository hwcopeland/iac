---
project: "compute-infrastructure"
maturity: "proof-of-concept"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "Code quality conventions, tooling gaps, and standards for Go controller and Python docking scripts"
owner: "@staff-engineer"
dependencies:
  - architecture.md
---

# Code Quality Specification

## Status Summary

**Zero code quality tooling is configured.** No linters, no formatters, no pre-commit hooks,
no CI pipeline, and no test suite exist anywhere in this repository. This document records what
conventions exist organically in the code, what gaps are present, and what the baseline should
be as the project matures beyond proof-of-concept.

---

## Languages and Source Boundaries

| Language | Location | Role |
|---|---|---|
| Go 1.21 | `k8s-jobs/controller/` | Active: controller, REST API, job orchestration |
| Python 3 | `docker/autodock4/scripts/`, `docker/autodock-vina/scripts/` | Active: scientific pipeline scripts inside Docker images |
| Python 3 | `rke2/airflow/dags/autodock4.py` | Legacy: Airflow DAG being replaced |
| Shell (POSIX sh) | `docker/*/scripts/*.sh` | Active: SDF splitting, post-processing |
| YAML | `k8s-jobs/`, `iac/ansible/`, `rke2/` | Infrastructure manifests |

---

## Go Code Quality — What Exists

### Module and Package Structure

The Go controller lives entirely in `k8s-jobs/controller/` as a single `package main` with
two source files:

- `main.go` — controller struct, all 6 job-creation methods, reconcile loop, `main()`
- `handlers.go` — HTTP API handlers and request/response types

**There are no internal packages, no sub-packages, and no interface definitions.** All
code shares package scope. The module name is `docking-controller` (`go.mod` line 1).

This is acceptable at the current scale (approximately 560 lines of Go), but creates
testability problems: `DockingJobController` embeds a concrete `*kubernetes.Clientset`
with no interface boundary, making unit testing impossible without a live cluster.

### Naming Conventions

Observed patterns in `main.go` and `handlers.go`:

- Exported types use PascalCase: `DockingJobController`, `DockingJob`, `DockingJobSpec`,
  `DockingJobStatus`, `DockingJobList`, `APIHandler`, `DockingJobRequest`, `DockingJobResponse`
- Unexported functions use camelCase: `getConfig`, `hasLogsSuffix`, `writeError`,
  `pvcVolume`, `pvcMount`, `ptrInt32`
- Receiver methods use camelCase: `createCopyLigandDbJob`, `createPrepareReceptorJob`,
  `createSplitSdfJob`, `processDockingJob`, `failJob`, `reconcileJobs`, `startAPIServer`,
  `waitForJobCompletion`
- Constants use PascalCase for exported values: `DefaultImage`, `DefaultAutodockPvc`,
  `DockingJobFinalizer`
- Kubernetes label domain: `docking.khemia.io` (consistent throughout `main.go` and `crd/`)

These naming patterns are idiomatic Go and should be maintained.

### Error Handling

The Go code follows Go's idiomatic error-return pattern consistently:

- Functions that can fail return `error` as the last return value
- Errors are wrapped with context using `fmt.Errorf("description: %v", err)`
- Callers check and propagate or log errors immediately — no silent drops
- `log.Fatalf` is used in `main()` for unrecoverable startup failures (correct usage)

One exception: `handlers.go:209` calls `Delete` in a loop and logs failures individually
rather than aggregating or returning an error to the caller. This is a pragmatic choice
for a delete workflow but means partial deletions are not surfaced to the API caller.

### Logging

All logging uses `log` from the Go standard library — `log.Println`, `log.Printf`,
`log.Fatalf`. No structured logging library (zap, zerolog, slog) is in use.

Consequences of unstructured logging:
- Log lines are plain text with no machine-parseable fields
- Job names and error details are embedded in format strings, not as discrete fields
- No log levels (debug/info/warn/error) — everything goes to the same stream
- Cannot filter or aggregate logs by job name, phase, or error type without grep

Current log statements are consistent in style: `log.Printf("verb %s: %v", subject, err)`.

### Comments and Documentation

Source files have a package-level doc comment (`// Package main provides ...`). Exported
types and key functions have doc comments following Go conventions. Private helper functions
(`pvcVolume`, `pvcMount`, `ptrInt32`) have concise doc comments. This level of documentation
is appropriate for the codebase size.

### Known Code Issues (Not Style — Functional)

These are documented here because they affect code quality in the broader sense:

| Location | Issue |
|---|---|
| `main.go:365` | `createSplitSdfJob` hardcodes `return 5, nil` — batch count is never read from job output |
| `main.go:516` | `reconcileJobs()` is a stub that returns `nil` immediately — no reconciliation occurs |
| `main.go:179-231` | Steps after split-sdf do not call `waitForJobCompletion` — race condition between pipeline steps |
| `handlers.go:117` | Job names use `time.Now().Unix()` — two POST requests in the same second produce a naming collision in Kubernetes |
| `main.go:175-177` | `hasLogsSuffix` uses a hard-coded length offset (`path[len(path)-5:] == "/logs"`) rather than `strings.HasSuffix` |
| `main.go:35` | `DockingJobFinalizer` constant is defined but never used in any finalizer logic |

---

## Python Code Quality — What Exists

### Naming Conventions

Observed patterns across `dockingv2.py`, `proteinprepv2.py`, `ligandprepv2.py` (both
autodock4 and autodock-vina variants):

- Module-level functions use `snake_case`: `parse_args`, `read_grid_center`, `docking`,
  `run_command`, `download_protein`, `remove_ligand_from_pdb`, `prepare_receptor`,
  `calculate_grid_center`, `run_prepare_ligand`, `convert_ligand_to_format`
- Constants use `UPPER_SNAKE_CASE` where present: `AUTODOCK`, `AUTOGRID`
- All scripts have a `main()` entry point guarded by `if __name__ == "__main__"`
- Function docstrings are present on most functions in Google/NumPy-adjacent style
  (description paragraph, then Parameters/Returns sections)

These patterns are idiomatic Python and should be maintained.

### `shell=True` Usage — Security and Style Concern

Multiple scripts use `subprocess.run(command, shell=True, ...)` where `command` is an
f-string constructed from function parameters:

`dockingv2.py:86`:
```
command = (
    f"{AUTODOCK} --receptor {receptor_pdbqt} --ligand {ligand_path} ..."
)
subprocess.run(command, shell=True, check=True)
```

`proteinprepv2.py` (`run_command` helper):
```
subprocess.run(command, shell=True, check=True)
```

`ligandprepv2.py:59`:
```
command = f"prepare_ligand4 -l {ligand_file} -o {output_pdbqt}"
subprocess.run(command, shell=True, check=True, ...)
```

In all these cases the parameters originate from:
1. Script arguments (file paths derived from the ligand database or batch label)
2. The `pdbid`, `ligand_db`, and `batch_label` values that flow from the controller
   API request body through to these scripts as Kubernetes Job arguments

`pdbid`, `ligand_db`, and `jupyter_user` from the REST API are passed unsanitized into
Job container arguments in `main.go`. If those values contain shell metacharacters, the
resulting commands constructed by these Python scripts will execute attacker-controlled
shell. **This is the same injection path flagged in the security posture section.**

The `shell=True` pattern is consistently used across all scripts — it is the established
convention in this codebase, though not a quality convention worth preserving.

### Error Handling

Python scripts use three different error-handling styles inconsistently:

1. `sys.exit(1)` with a `print` — used in `dockingv2.py`, `proteinprepv2.py`
2. `exit(1)` / `exit(2)` / `exit(3)` — used in `proteinprepv2.py` (builtin, not `sys.exit`)
3. `subprocess.CalledProcessError` catch + `continue` — used in the per-ligand loops
   in `dockingv2.py` and `docker/autodock4/scripts/dockingv2.py`

No script raises or propagates exceptions to callers; all errors terminate the process
or continue the loop. This is appropriate for containerized batch scripts.

### Missing Tooling

- No `pyproject.toml`, `setup.cfg`, or `tox.ini`
- No `ruff`, `flake8`, or `pylint` configuration
- No `mypy` or `pyright` type checking configuration
- No `black` or `isort` formatting configuration
- No `requirements-dev.txt` or test dependencies declared
- `requirements.txt` in both docker variants pin only package names without versions:
  `biopython` and `rdkit` — no version pinning means builds are not reproducible

### No `#!/usr/bin/env python3` Shebang in `ligandprepv2.py`

Both `docker/autodock4/scripts/ligandprepv2.py` and `docker/autodock-vina/scripts/ligandprepv2.py`
are missing the `#!/usr/bin/env python3` shebang line present in the other scripts.
This is a minor inconsistency that doesn't affect container execution (scripts are invoked
via explicit `python3` prefix) but diverges from the pattern.

---

## Shell Scripts — What Exists

Shell scripts (`split_sdf.sh`, `3_post_processing.sh`) use POSIX `sh` shebang
(`#!/bin/sh`) and include `set -e` for fail-fast behavior. This is correct practice.

`split_sdf.sh` uses positional argument validation (`[ $# != 2 ]`). The awk script uses
variable interpolation safely via `-v` option rather than shell expansion. These are
reasonable POSIX shell practices.

`test/containers/setup.sh` uses `#!/usr/bin/env bash` (bash-specific) and includes
some robust patterns (`${BASH_SOURCE[0]}`, double-quoting of variables) but also has
unquoted variables in one location (`cd $DIR_AIRFLOW`). This script is vestigial
(Airflow-era smoke test scaffold) and not on the critical path.

---

## YAML / Infrastructure Manifests — What Exists

### Kubernetes Manifests

Files in `k8s-jobs/` follow Kubernetes API conventions:
- `app.kubernetes.io/name` label on the deployment and pods (Kubernetes recommended label)
- Resource requests and limits are defined for the controller pod
- CRD includes `additionalPrinterColumns` for `kubectl get` output

**Label domain inconsistency:** `k8s-jobs/jobs/job-templates.yaml` and the README use
`docking.k8s.io/` as the label domain prefix, while `main.go` generates
`docking.khemia.io/` labels and the CRD is registered as `dockingjobs.docking.khemia.io`.
`docking.khemia.io` is correct; the templates and README are stale.

### Ansible

YAML files in `iac/ansible/` follow Ansible conventions. No `ansible-lint` configuration
exists. Hard-coded IP addresses in inventory files and unencrypted secrets (RKE2 join
token) are documented in the onboarding doc as tech debt.

---

## Compiled Binary in Version Control

`k8s-jobs/controller/docking-controller` is a compiled Mach-O 64-bit ARM64 binary committed
to the repository. It is listed as untracked in git status, meaning `.gitignore` does not
exclude it.

This binary:
- Is platform-specific (macOS/ARM64) and will not run in the Alpine Linux container
- Inflates repository size on each rebuild
- Creates confusion about which binary is current relative to the source
- Should be added to `.gitignore` immediately

---

## Documentation Quality

### README (`README.md`)

The root README describes the Airflow-era workflow and has not been updated for the
Kubernetes controller migration. It references `autodock.py` as the main DAG and the
`docker/` directory structure without mentioning `k8s-jobs/`.

### k8s-jobs README (`k8s-jobs/README.md`)

Contains multiple stale or incorrect references:
- File path `controller/api/handlers.go` — this file was deleted; handlers now live at
  `controller/handlers.go`
- API group `docking.k8s.io/v1` in the CRD example — the actual group is `docking.khemia.io/v1`
- Label domain `docking.k8s.io/` in the "Job Labels" section — should be `docking.khemia.io/`
- Environment variables `API_PORT` and `RECONCILE_INTERVAL` documented as supported — neither
  is read by the controller

### `docs/onboarding.md`

Accurate as of 2026-03-22 and explicitly documents known bugs and gaps. This is the most
reliable source of current project state.

---

## Tooling Inventory

The following tools are absent from the project. This is the baseline (not aspirational):

| Tool | Category | Status |
|---|---|---|
| `golangci-lint` | Go linting | Not configured, no `.golangci.yml` |
| `gofmt` / `goimports` | Go formatting | Not automated; mentioned in onboarding doc as "use manually" |
| `go vet` | Go static analysis | Not automated; no CI to run it |
| `ruff` / `flake8` | Python linting | Not configured |
| `mypy` / `pyright` | Python type checking | Not configured |
| `black` / `isort` | Python formatting | Not configured |
| `shellcheck` | Shell linting | Not configured |
| `yamllint` | YAML linting | Not configured |
| `ansible-lint` | Ansible linting | Not configured |
| `pre-commit` | Git hook framework | Not configured, no `.pre-commit-config.yaml` |
| CI/CD pipeline | Automated checks | Does not exist |
| `Makefile` | Build automation | Does not exist |

---

## Version Pinning

| Language | Dependency File | Version Pinning Status |
|---|---|---|
| Go | `k8s-jobs/controller/go.mod` | Direct dependencies pinned to minor versions (`v0.28.0`); `go.sum` present |
| Python (autodock4) | `docker/autodock4/requirements.txt` | No versions — `biopython` and `rdkit` only |
| Python (autodock-vina) | `docker/autodock-vina/requirements.txt` | No versions — `biopython` and `rdkit` only |
| Python (AutoDockTools) | `docker/autodock-vina/Dockerfile:48` | Installed from GitHub main branch tip — completely unpinned |

The Go module graph is reproducible. The Python Docker builds are not — identical
Dockerfiles may produce different results on successive builds.

---

## Conventions to Follow

These are extracted from the existing codebase and should be treated as the project's
current de facto standards:

### Go

- `package main` for the controller (acceptable at current scale; revisit if a second
  binary is added)
- Exported types: PascalCase
- Unexported functions: camelCase
- Kubernetes label domain: `docking.khemia.io` (not `docking.k8s.io`)
- Error wrapping: `fmt.Errorf("description: %v", err)` — use `%w` (wrapping) once
  minimum Go version allows callers to unwrap
- Logging: `log.Printf("verb %s ...", subject, ...)` format — subject first, error last
- Helper constructors with `New` prefix: `NewDockingJobController`, `NewAPIHandler`
- HTTP error helper: `writeError(w, message, statusCode)` — do not inline JSON error
  writes; use this function

### Python

- All scripts executable as `python3 script.py`, not as modules
- Functions use `snake_case`
- Module-level constants use `UPPER_SNAKE_CASE`
- Entry point always: `if __name__ == "__main__": main()`
- Docstrings present on all non-trivial functions (Parameters / Returns where applicable)
- Error exits via `sys.exit(1)` for fatal errors (prefer over bare `exit()`)

### YAML / Kubernetes

- Controller label: `app.kubernetes.io/name: docking-controller`
- Workflow job labels: `docking.khemia.io/workflow`, `docking.khemia.io/job-type`,
  `docking.khemia.io/batch`, `docking.khemia.io/parent-job`
- Namespace: `chem` for all active docking workloads
- Resource requests and limits required for all controller pods

---

## Gaps This Spec Does Not Cover (Out of Scope)

- Test strategy and coverage requirements — see `docs/spec/testing.md`
- Security review of `shell=True` injection paths — see `docs/spec/security.md`
- Observability and structured logging requirements — see `docs/spec/operations.md`
- Architecture of the controller and CRD contract — see `docs/spec/architecture.md`

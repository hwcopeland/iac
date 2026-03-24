---
project: "compute-infrastructure"
maturity: "proof-of-concept"
last_updated: "2026-03-23"
updated_by: "@staff-engineer"
scope: "Testing strategy, coverage gaps, and testability constraints for the compute-infrastructure project, including the Airflow-to-K8s controller migration"
owner: "@staff-engineer"
dependencies:
  - architecture.md
---

# Testing Specification

## Current State: Zero Tests

This document is honest about the starting point: **there are no tests in this codebase.**

No `*_test.go` files exist. No `test_*.py` or `*_test.py` files exist. No `pytest.ini`,
`pyproject.toml`, or `setup.cfg` with test configuration exists. No Makefile or CI workflow
defines a `go test` or `pytest` target. The `test/containers/setup.sh` file is not a test
suite — it is an Airflow local environment bootstrap script (vestigial from pre-migration)
that spins up Docker Compose, polls for health, and tears it down. It validates nothing and
asserts nothing.

This is a scientific HPC workflow. The absence of tests is not merely a quality concern — it
is a correctness risk. Wrong grid parameters, incorrect atom types, or a bad batch count
produce wrong science. No pipeline error surfaces. The jobs complete, the numbers are wrong,
and nobody knows.

---

## What the Codebase Actually Contains

### Go Controller (`k8s-jobs/controller/`)

Two source files, no test files:

- `main.go` — `DockingJobController` struct, all `createXxxJob` methods, `reconcileJobs`
  (stub returning `nil`), `waitForJobCompletion`, helper functions, and `main`.
- `handlers.go` — `APIHandler` struct, HTTP handler methods (`ListJobs`, `CreateJob`,
  `GetJob`, `DeleteJob`, `GetLogs`, `HealthCheck`, `ReadinessCheck`).

**Confirmed bug, no test catching it:** `createSplitSdfJob` (main.go:365) returns the
hardcoded integer `5` regardless of how many batches `split_sdf.sh` actually produced.
The controller will always attempt to process exactly 5 batches. If the real split produced
3 or 50, every downstream job is wrong. This is a silent correctness failure.

**Structural testability blocker:** `DockingJobController.client` is typed as
`*kubernetes.Clientset` (a concrete struct), not an interface. `DockingJobController.jobClient`
is `typed.JobInterface`, which is already an interface — that part is injectable. But `APIHandler`
also takes `*kubernetes.Clientset` directly. The Kubernetes fake client (`k8s.io/client-go/kubernetes/fake`)
implements the same `kubernetes.Interface` interface, but neither `DockingJobController` nor
`APIHandler` accept an interface — they require the concrete type. Injecting a fake client
requires refactoring both structs to accept `kubernetes.Interface`.

**What can be unit-tested without refactor:**

- `hasLogsSuffix` (main.go:175) — pure string function, no dependencies.
- `pvcVolume` / `pvcMount` / `ptrInt32` — pure builder functions.
- JSON serialization/deserialization of request/response types.
- `writeError` — with an `httptest.ResponseRecorder`.

**What requires refactor before it can be unit-tested:**

- All `createXxxJob` methods — require fake `JobInterface` injection (already partially in place
  via `jobClient typed.JobInterface`, but `client *kubernetes.Clientset` also used in handlers).
- `waitForJobCompletion` — requires fake `JobInterface`.
- All HTTP handler methods — require `kubernetes.Interface` (not the concrete `*Clientset`).

### Python Scripts (`docker/autodock4/scripts/`)

Four scripts, no test files, no test framework configuration:

- `proteinprepv2.py` — downloads PDB via `wget`, parses structure with BioPython, removes
  native ligand, calculates grid center (center-of-mass of ligand atoms), writes
  `grid_center.txt`, runs `prepare_receptor4` (MGLTools binary).
- `dockingv2.py` — reads `grid_center.txt`, extracts atom types from PDBQT files, generates
  GPF and DPF parameter files, runs `autogrid4` and `autodock4` binaries.
- `ligandprepv2.py` — reads SDF via RDKit, converts molecules to PDB/MOL2, runs
  `prepare_ligand4` (MGLTools binary).
- `3_post_processing.sh` — greps `.dlg` output files for RANKING lines, extracts best-energy
  result.
- `split_sdf.sh` — awk-based SDF splitter; outputs batch count to stdout.

**What can be unit-tested without Docker:**

- `extract_atom_types` (dockingv2.py) — pure file parser, testable with a fixture `.pdbqt` file.
- `read_grid_center` (dockingv2.py) — pure file reader, testable with a fixture file.
- `prepare_gpf` / `prepare_dpf` (dockingv2.py) — pure file writers; output can be asserted
  against a known-good GPF/DPF for a reference protein.
- `calculate_grid_center` (proteinprepv2.py) — BioPython PDB parser + arithmetic; testable
  with a small fixture PDB file.
- `remove_ligand_from_pdb` (proteinprepv2.py) — BioPython filter; testable with a fixture PDB.
- `convert_ligand_to_format` (ligandprepv2.py) — RDKit mol conversion; testable with a
  fixture SDF molecule.
- `split_sdf.sh` — pure awk script; testable with a small synthetic multi-molecule SDF.

**What requires the Docker image (external binaries):**

- `run_autogrid` — shells out to `/autodock/autogrid4`.
- `run_prepare_ligand` — shells out to `prepare_ligand4` (MGLTools).
- `prepare_receptor` — shells out to `prepare_receptor4` (MGLTools).
- `download_protein` — calls `wget` against RCSB.

### Airflow DAG (`rke2/airflow/dags/autodock4.py`)

The Airflow DAG is the pre-migration orchestrator. It is being superseded by the K8s
controller. Its own logic has a known off-by-one: `get_batch_labels` returns
`range(batch_count + 1)` which produces one extra batch index. The comment in the code reads
`#maybe this will work?` — confirming this was never validated.

### Test Infrastructure (`test/containers/setup.sh`)

This script bootstraps a local Airflow Docker Compose environment for the pre-migration
stack. It is not a test suite:

- It makes no assertions.
- It does not validate DAG behavior, only that the Airflow worker container reports
  `(healthy)` in Docker Desktop.
- It is implicitly scoped to the deprecated Airflow workflow.
- The `test/containers/.gitignore` excludes the generated `/airflow` directory, indicating
  this was a local-only developer tool.

---

## Domain Correctness Risk

This section exists because the project is a scientific computing pipeline. The risks are
different from typical application software.

**Grid center accuracy.** `calculate_grid_center` in `proteinprepv2.py` computes the
centroid of native ligand atoms. If `ligand_id` does not match any residue in the downloaded
PDB (wrong case, wrong residue name, insertion code mismatch), the function calls `exit(1)`.
If it matches the wrong residue, it silently computes a wrong center. A miscentered grid
produces all-positive binding energies; the post-processing step filters these out with
`grep '-'`, so a miscentered grid run produces an empty best-energy result with no error.

**Atom type completeness.** `extract_atom_types` intersects parsed types against a
hardcoded supported set (`{'A', 'C', 'HD', 'N', 'NA', 'OA', 'SA', 'S', 'H', 'F', 'Cl',
'Br', 'I', 'P', 'Mg', 'Mn', 'Zn', 'Ca', 'Fe'}`). Atoms not in this set are silently
dropped. A ligand with an unsupported atom type will be docked with an incomplete parameter
file. AutoDock4 will not necessarily fail; it may produce a result that is chemically wrong.

**Batch count correctness.** `createSplitSdfJob` ignores `split_sdf.sh` stdout entirely and
returns `5` (hardcoded). `split_sdf.sh` prints the actual batch count to stdout. The
controller never reads it. All runs use exactly 5 batches regardless of ligand database size.

**Post-processing fragility.** `3_post_processing.sh` globs `ligand_${DB_LABEL}_*.dlg`.
The `.dlg` naming convention must match what `dockingv2.py` produces. Any mismatch silently
produces an empty result.

---

## Testing Strategy

Given the current state of zero tests, the strategy is tiered by cost-to-implement and
risk-of-wrong-science. The highest-risk correctness bugs require the most immediate attention.

### Tier 1: Pure-Function Unit Tests (No External Dependencies)

**Target:** Functions with no subprocess calls, no file I/O beyond simple reads, and no
Kubernetes API calls. These can be run in any environment with no special setup.

**Go controller — testable without refactor:**

| Function | Test Focus |
|---|---|
| `hasLogsSuffix` | Boundary cases: empty string, exact `/logs`, `/logs` with prefix, strings shorter than 6 chars |
| `pvcVolume` | Verify PVC claim name is set correctly in returned Volume spec |
| `pvcMount` | Verify mount path and volume name are wired correctly |
| `ptrInt32` | Value round-trip check |
| `writeError` | Correct HTTP status code, Content-Type header, JSON body shape |
| Request/response JSON marshaling | Round-trip `DockingJobRequest` and `DockingJobResponse` through `encoding/json` |

**Python scripts — testable with fixture files:**

| Function | File | Test Focus |
|---|---|---|
| `extract_atom_types` | dockingv2.py | Known `.pdbqt` fixture: expected atom type set; fixture with unsupported types: verify intersection behavior |
| `read_grid_center` | dockingv2.py | Well-formed `grid_center.txt`; whitespace-only line; negative coordinates |
| `prepare_gpf` output | dockingv2.py | Assert generated GPF matches known-good reference for a fixed protein/ligand pair |
| `prepare_dpf` output | dockingv2.py | Assert generated DPF matches known-good reference; assert `ga_num_evals` and `ga_pop_size` are present |
| `calculate_grid_center` | proteinprepv2.py | Small synthetic PDB with one known ligand; verify centroid arithmetic; verify exit on missing ligand_id |
| `remove_ligand_from_pdb` | proteinprepv2.py | Verify ligand residue is absent in output PDB; verify other residues are preserved |
| `convert_ligand_to_format` | ligandprepv2.py | Small SDF molecule; verify output PDB file exists; verify `None` molecule is skipped |

**Shell scripts:**

| Script | Test Focus |
|---|---|
| `split_sdf.sh` | Synthetic 3-molecule SDF with `LigandsChunkSize=1`: verify 3 output files and stdout `2`; verify stdout matches actual batch count (directly addresses the hardcoded-5 bug) |

### Tier 2: Controller Unit Tests (Requires Interface Refactor)

**Prerequisite:** Refactor `DockingJobController.client` from `*kubernetes.Clientset` to
`kubernetes.Interface`, and `APIHandler.client` from `*kubernetes.Clientset` to
`kubernetes.Interface`. This unblocks use of `k8s.io/client-go/kubernetes/fake.NewSimpleClientset()`.

After refactor:

| Scenario | Test Focus |
|---|---|
| `createSplitSdfJob` batch count | After the hardcoded-5 fix: verify the returned count matches the `split_sdf.sh` stdout; fake JobInterface returns a completed Job |
| `createCopyLigandDbJob` | Verify Job name, labels, and volume mounts match expected spec |
| `createPrepareReceptorJob` | Verify `--protein_id` and `--ligand_id` args are passed from DockingJob spec |
| `createPrepareLigandsJob` | Verify `batchLabel` is correctly wired into args |
| `createDockingJobExecution` | Verify PDBID and batchLabel args |
| `waitForJobCompletion` timeout | Fake ticker advancing past 10m timeout returns error |
| `waitForJobCompletion` success | Fake Job returns `Status.Succeeded=1` |
| `CreateJob` HTTP handler | 202 on valid payload; 400 on missing `ligand_db`; default values applied |
| `GetJob` HTTP handler | Correct status derivation: all-succeeded=Completed, any-failed=Failed, mixed=Running |
| `DeleteJob` HTTP handler | 204 on success; partial delete failure logged but does not 500 |
| `ListJobs` HTTP handler | Correct `workflows` map construction from labeled jobs |

### Tier 3: Integration Tests (Requires Docker)

Tests in this tier require the autodock4 Docker image to be present. They validate the
scientific correctness of the full pipeline steps, not just structural properties.

**Python script integration — run inside Docker:**

| Scenario | Test Focus |
|---|---|
| `proteinprepv2.py` with PDB 7JRN | Download PDB, compute grid center, produce `.pdbqt`; assert `grid_center.txt` is non-empty; assert centroid is within expected bounds for TTT ligand |
| `ligandprepv2.py` with a small SDF (1-3 molecules) | Produce `.pdbqt` files; assert at least one `ligand_1.pdbqt` exists; assert it contains `ATOM` lines |
| `split_sdf.sh` with real ChEBI subset | Assert stdout batch count matches number of output files |
| Atom type completeness for known problematic ligands | Run `extract_atom_types` on a ligand with Fe or Mn; confirm it is in the supported set and appears in GPF |

These tests should run inside the container image via a `docker run` invocation and
require the image to be built locally or pulled from the registry.

### Tier 4: End-to-End (Requires Cluster)

E2E tests require a running Kubernetes cluster with the `chem` namespace, CRD applied,
controller deployed, and PVCs provisioned. These are smoke tests, not correctness validators.

| Scenario | Test Focus |
|---|---|
| POST /api/v1/dockingjobs with minimal payload | 202 response; Kubernetes Jobs are visible with correct labels within 30s |
| GET /api/v1/dockingjobs/{name} | Status transitions from Pending to Running |
| GET /health, GET /readyz | 200 with correct JSON body |
| DELETE /api/v1/dockingjobs/{name} | All labeled child Jobs are deleted |

E2E tests for a full docking run (complete workflow) are impractical for CI — a single run
against ChEBI_complete takes hours. Smoke tests with a minimal ligand fixture (1-10 molecules)
are the appropriate scope.

---

## What Not to Test

**Airflow DAG (`autodock4.py`):** The DAG is in the process of being superseded. Testing it
would validate a deprecated code path. The off-by-one in `get_batch_labels` should be
documented as a known defect in the migration notes, not fixed.

**Ansible playbooks (`iac/ansible/`):** Infrastructure provisioning is not a target for
automated testing in this project. Manual validation against test Proxmox VMs is the
current practice, which is appropriate for a research-lab-scale cluster.

**Kubernetes YAML manifests (CRD, RBAC, deployment):** Schema validation via
`kubectl apply --dry-run=client` is sufficient; full policy testing is out of scope.

---

## CI Integration

There is no CI pipeline. No `.github/workflows/` directory exists. No `Makefile` or
`Taskfile` defines test targets.

A minimum viable CI setup would gate on:

1. `go test ./...` — passes when Tier 1 Go tests exist.
2. `go vet ./...` — catches the `createSplitSdfJob` hardcoded-5 only if a linter rule flags
   it; `staticcheck` or `revive` with a `magic-number` rule would surface it.
3. `pytest` (or `python -m pytest`) — passes when Tier 1 Python unit tests exist.
4. `kubectl apply --dry-run=client -f k8s-jobs/crd/dockingjob-crd.yaml` — validates CRD schema.

Docker-based integration tests (Tier 3) should run on-demand or nightly, not on every push,
due to image pull and binary execution overhead.

---

## Coverage Targets

Given the project is proof-of-concept, coverage targets are advisory rather than gate
criteria. The intent is to establish a floor that grows as the codebase matures.

| Component | Current Coverage | Realistic Near-Term Target |
|---|---|---|
| Go controller (pure functions) | 0% | 60% (after Tier 1 only) |
| Go controller (handler/job methods) | 0% | 70% (after interface refactor + Tier 2) |
| Python pure functions | 0% | 70% (Tier 1 unit tests with fixtures) |
| Python subprocess-dependent | 0% | 20% (Tier 3, Docker-only) |
| Shell scripts | 0% | 80% (`split_sdf.sh` is the priority) |

Coverage tooling: `go test -coverprofile` for Go; `pytest --cov` with `pytest-cov` for
Python.

---

## Priority Order

The following order prioritizes by risk-to-scientific-correctness, not by ease:

1. **`split_sdf.sh` unit test** — Directly confirms the actual batch count returned by the
   script, which is the root cause of the hardcoded-5 bug in the controller.
2. **`createSplitSdfJob` fix + test** — After fixing the hardcoded return value to read
   script stdout, a unit test with a fake JobInterface guards against regression.
3. **`prepare_gpf` / `prepare_dpf` output tests** — Guard against silent parameter
   corruption in the scientific configuration files that drive AutoDock4.
4. **`calculate_grid_center` test** — Validates grid centering, which determines whether
   docking results are meaningful at all.
5. **HTTP handler tests** — `CreateJob` and `GetJob` are the primary user-facing surfaces;
   test these before the other handlers.
6. **Remaining pure-function unit tests** — Complete Tier 1 coverage for both Go and Python.
7. **Interface refactor + Tier 2 Go tests** — Unlocks full controller unit testing.
8. **Tier 3 Docker integration tests** — Scientific correctness validation end-to-end.

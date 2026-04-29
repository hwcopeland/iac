# WP-3: Docking and Pose Refinement

## Owner

TBD

## Scope

Replace the existing single-engine docking pipeline with a pluggable multi-engine docking
system, add ML-based pose prediction, and extend with post-docking pose refinement (short MD
relaxation, MM-GBSA rescoring, interaction analysis). The output is a ranked hit list with
scores, interaction fingerprints, and per-residue energy contributions.

### Current state

The existing docking pipeline is functional but limited:
- **Single engine**: AutoDock Vina via `containers/autodock-vina/` and
  `plugins/autodock-vina.yaml`
- **Parallel fan-out**: `parallel_docking.go` splits ligands into chunks, creates K8s jobs
  per chunk, waits for completion
- **Result collection**: `result-writer/` drains a staging table into `docking_results`
  (compound_id, affinity, docked_pdbqt)
- **Interaction analysis**: `handlers_pocket.go` computes receptor-ligand contacts from
  PDBQT coordinates; `handlers_prolif.go` proxies to the ProLIF sidecar for SVG interaction
  maps
- **No pose refinement**: Raw Vina scores are the final ranking. No MD relaxation, no
  MM-GBSA, no re-ranking
- **No consensus scoring**: Single engine, single score

This WP builds on the existing parallel docking infrastructure and adds engine pluggability,
consensus scoring, and a full refinement pipeline.

## Deliverables

### Docking engines

1. **AutoDock Vina 1.2 container** (`zot.hwcopeland.net/chem/vina:1.2`)
   - Modern AutoDock Vina from the Forli Lab (sometimes called "Vina 2") — successor to the
     legacy Vina 1.1.2 currently in `containers/autodock-vina/`. CPU-only, fast, well-tested.
     Replaces the existing Vina container as the default CPU engine.
   - Source: https://github.com/ccsb-scripps/AutoDock-Vina
   - Pin to a tagged release (currently `v1.2.5`); rebuild on each release.
   - **Container build** (`containers/vina-1.2/Dockerfile`):
     - Base: `python:3.11-slim` (Vina ships official Python bindings)
     - Install: `vina>=1.2.5` from PyPI, `meeko` for ligand prep, RDKit
     - No CUDA, no GPU runtime
     - Entrypoint: `vina` CLI; the prep wrapper script in `containers/vina-1.2/scripts/`
       handles input/output mapping consistent with the existing container's contract
   - **Scoring functions exposed**: `vina` (default), `vinardo`, `ad4`. Job spec selects
     which one.
   - Input: receptor PDBQT + ligand PDBQT + binding-site box
   - Output: docked poses (PDBQT), affinity scores (kcal/mol)
   - Compute class: `cpu` (16 threads recommended)

2. **Vina-GPU container** (`zot.hwcopeland.net/chem/vina-gpu:latest`)
   - GPU-accelerated AutoDock Vina (Vina-GPU 2.1 from the Sun Lab at Nanjing Tech).
     OpenCL-based, runs on the RTX 3070 in `nixos-gpu`. 10–50× speedup over CPU Vina at
     equivalent exhaustiveness on small ligand batches; bigger speedups on larger batches.
   - Source: https://github.com/DeltaGroupNJUPT/Vina-GPU-2.1
   - **Container build** (`containers/vina-gpu/Dockerfile`):
     - Base: `nvidia/opencl:ubuntu22.04` (OpenCL ICD loader + headers; the actual ICD
       comes from the host driver via the WP-9 NixOS mount path)
     - Install: Boost ≥1.77, OpenCL dev libs, then build Vina-GPU 2.1 from source against
       a pinned commit
     - Entrypoint: `Vina-GPU` binary with input/output flags wrapped by a Python adapter
       that matches the existing container's input/output contract
   - **Engine variants** (selectable via job spec):
     - `vina-gpu` — single ligand at high exhaustiveness (default)
     - `vina-gpu-batch` — multi-ligand batch mode (~100 ligands per launch); uses more VRAM
       but amortizes kernel startup
   - **Validation**: every release of the Vina-GPU container must pass a regression test
     that docks 10 ligands against `7jrn` and produces affinities within ±0.3 kcal/mol of
     the CPU Vina 1.2 result (same scoring function). If the regression drifts, hold the
     image.
   - Input: receptor PDBQT + ligand PDBQT(s) + binding-site box
   - Output: docked poses (PDBQT), affinity scores
   - Compute class: `gpu` (uses the WP-9 NixOS host-path mounts for OpenCL ICD via
     `/run/opengl-driver/lib`)

3. **Smina container** (`zot.hwcopeland.net/chem/smina:latest`)
   - Smina (Vina fork with custom scoring and flexible residue support)
   - Input: receptor PDBQT + ligand PDBQT + binding-site box
   - Output: docked poses (PDBQT), affinity scores (kcal/mol)
   - CPU-only. Use when flexible side-chain support is needed (Smina-specific feature).

4. **Gnina container** (`zot.hwcopeland.net/chem/gnina:latest`)
   - Gnina (CNN-scored docking)
   - Input: receptor PDBQT + ligand SDF/PDBQT + binding-site box
   - Output: docked poses, CNN score, CNN affinity, Vina affinity
   - Compute class: `gpu`

5. **DiffDock container** (`zot.hwcopeland.net/chem/diffdock:latest`)
   - DiffDock-L (diffusion-based blind docking)
   - Input: receptor PDB + ligand SMILES or SDF
   - Output: predicted poses (SDF) with confidence scores
   - Compute class: `gpu`

6. **Engine selection guide** (informs UI defaults and consensus presets):

   | Scenario | Recommended engine | Why |
   |---|---|---|
   | Default fast CPU dock | `vina-1.2` | Modern, well-tested, free CPU baseline |
   | Need GPU throughput at Vina-equivalent scoring | `vina-gpu` | Same scoring as Vina, 10–50× faster on RTX 3070 |
   | Large library batch (>1k ligands) on GPU | `vina-gpu-batch` | Amortizes OpenCL kernel startup |
   | Flexible side chains | `smina` | Only Smina supports it natively |
   | CNN-rescored hits | `gnina` | Adds neural-net scoring on top of physics |
   | Blind docking (no known pocket) | `diffdock` | Works without a binding-site box |
   | Consensus mode | `vina-1.2` + `gnina` (or `vina-gpu` + `gnina`) | Cheap physics + CNN re-rank |

7. **Consensus scoring module** (library within the API or a sidecar):
   - Run 2+ engines on the same receptor-ligand pair
   - Normalize scores to a common scale (rank-based percentile normalization)
   - Consensus score = mean of normalized per-engine ranks
   - Flag compounds where engines disagree by more than 2 standard deviations
   - Configurable: user selects which engines to run via job spec
   - Vina-GPU and Vina 1.2 share a scoring function — running both in consensus is
     redundant. The consensus validator rejects this pairing with a 400.

### Parallel fan-out

8. **Refactored parallel orchestration:**
   - Generalize `parallel_docking.go` to support multiple engines
   - Per-engine: fan out ligand chunks to N worker pods (CPU engines: more pods, smaller
     chunks; `vina-gpu-batch`: fewer pods, larger chunks to amortize kernel startup)
   - Cross-engine: launch engine pods in parallel (e.g. `vina-gpu` + `gnina` consensus)
   - Result aggregation: collect all engine results, compute consensus, write to DB
   - Compute-class routing per engine (matches WP-9):
     - `vina-1.2`, `smina` → `cpu`
     - `vina-gpu`, `gnina`, `diffdock` → `gpu`
     - consensus scoring → `cpu`
   - Single-GPU constraint: only one `gpu`-class pod can run at a time on `nixos-gpu`. The
     orchestrator queues GPU engine pods serially within a job rather than fanning them out
     in parallel until additional GPU hardware exists.

### Pose refinement

9. **Short MD relaxation container** (`zot.hwcopeland.net/chem/gromacs-refine:latest`)
   - **Engine: GROMACS** (no OpenMM)
   - Input: protein-ligand complex (PDB from docked pose)
   - Protocol: energy minimization (steepest descent, 5000 steps) followed by 1 ns NVT MD
     with position restraints on protein backbone
   - Force field: AMBER ff14SB (protein) + GAFF2 (ligand, parameterized via AmberTools
     antechamber/parmchk2)
   - Output: relaxed complex PDB, trajectory (XTC), energy log

10. **MM-GBSA rescoring container** (`zot.hwcopeland.net/chem/mmgbsa:latest`)
    - **Engine: GROMACS + gmx_MMPBSA** (AmberTools-based, runs on GROMACS trajectories)
    - Input: MD trajectory from step 9
    - Protocol: extract last 200 ps of trajectory, compute MM-GBSA binding free energy
      with per-residue decomposition
    - Output: delta-G (kcal/mol), per-residue energy contributions (JSON), standard error

11. **PLIP interaction analysis:**
    - Run PLIP (Protein-Ligand Interaction Profiler) on the refined complex
    - Output: interaction types (H-bond, hydrophobic, pi-stacking, salt bridge, water bridge),
      interacting residues, distances, angles

12. **ProLIF interaction fingerprint:**
    - Refactor existing ProLIF sidecar integration (`handlers_prolif.go`) to run on the
      refined pose (not just the raw docked pose)
    - Generate binary interaction fingerprint (bit vector) for downstream ML/SAR

### CRDs and output

13. **DockJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `DockJob`)
    - Spec: `targetRef` (reference to TargetPrep), `libraryRef` (reference to LibraryPrep),
      `engines` (array of engine configs — each with `engine` enum and optional
      `scoringFunction`/`exhaustiveness`/`batchSize`), `consensus` (boolean), `topN`
      (number of poses to refine), `chunkSize`
    - Engine enum: `vina-1.2`, `vina-gpu`, `vina-gpu-batch`, `smina`, `gnina`, `diffdock`
    - Status: `phase`, `enginesCompleted`, `totalLigands`, `dockedLigands`, `bestAffinity`,
      `consensusComputed`, `startTime`, `completionTime`, `provenance`

14. **RefineJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `RefineJob`)
    - Spec: `dockJobRef` (reference to DockJob), `topN`, `mdLength` (ns),
      `mmgbsaFrames` (number of frames for MM-GBSA)
    - Status: `phase`, `refinedCount`, `startTime`, `completionTime`, `provenance`

15. **Output per compound** (stored in DB + Garage):
    - Best pose coordinates (PDB/SDF)
    - Score breakdown: per-engine affinity, consensus rank, MM-GBSA delta-G
    - Interaction fingerprint (ProLIF bit vector)
    - Per-residue energy contributions from MM-GBSA decomposition (JSON)
    - PLIP interaction summary (JSON)

16. **API endpoints:**
    - `POST /api/v1/docking/submit` -- submit a dock job (replaces existing endpoint with
      richer spec)
    - `GET /api/v1/docking/jobs/{name}` -- get dock job status (extend existing)
    - `GET /api/v1/docking/jobs/{name}/results` -- paginated ranked results (extend existing)
    - `POST /api/v1/docking/jobs/{name}/refine` -- submit a refinement job for top N poses
    - `GET /api/v1/docking/jobs/{name}/refine/{compoundId}` -- get refinement results

17. **Benchmark validation:**
    - Run against DUD-E or LIT-PCBA benchmark set (operator to specify which)
    - Report enrichment factor (EF1%, EF5%), AUC-ROC, and BEDROC
    - Benchmark must run as a reproducible script (`benchmark/sbdd-benchmark.sh`)
    - Store benchmark results with provenance for regression tracking
    - Benchmark suite runs Vina-GPU vs Vina 1.2 vs Smina against a 1k-compound subset and
      reports speedup + correlation. Used to validate that Vina-GPU stays within
      ±0.3 kcal/mol of Vina 1.2 on the same scoring function.

18. **Tests:**
    - Unit tests for consensus scoring normalization
    - Unit tests for MM-GBSA energy parsing
    - Unit tests for engine selector validation (e.g. consensus with `vina-1.2 + vina-gpu`
      → 400; valid combos pass)
    - Integration test: dock 10 ligands against 7jrn with `vina-1.2`, verify ranked results
    - Integration test: dock the same 10 ligands with `vina-gpu` on `nixos-gpu`, verify
      affinities within ±0.3 kcal/mol of `vina-1.2` and ≥10× wall-time speedup
    - Integration test: refine top 3 poses with GROMACS MD, verify trajectory output
    - E2E smoke test: submit dock job via API, poll until Succeeded, submit refine job,
      verify per-residue energies exist

## Acceptance Criteria

1. Submitting a DockJob with `engines: ["vina-1.2"]` against target 7jrn and a 100-compound
   library produces ranked results sorted by affinity within 30 minutes.
2. Submitting a DockJob with `engines: ["vina-gpu"]` against the same target and library
   completes in at least 10× less wall-time than the equivalent `vina-1.2` job, with
   affinities correlated at r ≥ 0.95 against the `vina-1.2` baseline.
3. Submitting a DockJob with `engines: ["vina-gpu", "gnina"], consensus: true` produces a
   consensus-scored result set where the `consensus_rank` column differs from either
   single-engine ranking for at least some compounds.
4. RefineJob on the top 5 poses produces: a GROMACS trajectory file (XTC), an MM-GBSA
   delta-G value with standard error, and per-residue energy contributions for each of the
   5 compounds.
5. The PLIP analysis output for each refined pose includes at least one identified interaction
   type (H-bond, hydrophobic, etc.) with residue ID and distance.
6. The ProLIF interaction fingerprint is a binary bit vector of length >= 50 that can be
   used for Tanimoto similarity comparison between compounds.
7. Benchmark validation against DUD-E (or LIT-PCBA) produces an EF1% value and AUC-ROC.
   The specific numeric thresholds are informational (not pass/fail) for the initial release.
8. The existing docking API endpoints (`/api/v1/docking/submit`,
   `/api/v1/docking/jobs/{name}`) continue to work for backward compatibility with the
   current Vina-only workflow. The new multi-engine spec is additive.
9. All GROMACS containers use GROMACS (not OpenMM). The Dockerfile must not install or
   import OpenMM.
10. The Vina-GPU container builds and runs against the `nixos-gpu` node's RTX 3070 with
    only the WP-9 NixOS host-path mounts (no other custom plumbing).

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-1 | Consumes prepared receptor and binding-site box |
| Blocked by | WP-2 | Consumes prepared compound library |
| Blocked by | WP-9 | Garage storage, CRD framework, `cpu` and `gpu` compute classes (GPU scheduling on `nixos-gpu`) |
| Blocks | WP-5 | Generative SAR loop-back requires re-docking |
| Blocks | WP-6 | FEP consumes refined poses as starting structures |
| Blocks | WP-8 | Reporting consumes docking/refinement results |
| Integrates with | WP-4 | ADMET triage runs on the same compound set |
| Integrates with | WP-7 | UI displays docking results, interaction maps, energy decomposition |

## Out of Scope

- Covalent docking
- Ensemble docking (multiple receptor conformations)
- Water-mediated docking (explicit water placement)
- Free energy perturbation (WP-6)
- ADMET prediction (WP-4)
- Flexible receptor docking beyond Smina's flexible side-chain support

## Open Questions

1. **DiffDock version**: DiffDock-L vs DiffDock-PP? DiffDock-L handles protein-ligand;
   DiffDock-PP handles protein-protein. Confirm scope is ligand docking only.

2. **GAFF2 parameterization**: antechamber + parmchk2 can fail on unusual chemistries.
   Should the refinement pipeline have a fallback (e.g., skip refinement for compounds
   that fail parameterization) or fail the entire compound?

3. **MD trajectory storage**: 1 ns MD trajectory per compound at reasonable write frequency
   can be 10-50 MB per compound. For a 1000-compound refinement, that is 10-50 GB. Store
   full trajectories or only the last frame + energies?

4. **Existing docking results**: The current `docking_results` table has ~millions of rows
   from previous runs. Should the schema migration preserve these, or start fresh with a
   new table?

5. **Gnina GPU requirements**: Gnina requires CUDA. The `nixos-gpu` node has an RTX 3070
   (8 GB VRAM). Is 8 GB enough for the typical Gnina batch size we want, or do we need to
   shrink batches and trade for wall-time?

6. **Vina-GPU upstream maintenance**: Vina-GPU is academic code with sporadic releases.
   Pin to a known-good commit, and decide a cadence for evaluating new releases vs the
   ±0.3 kcal/mol regression bound.

7. **OpenCL ICD on NixOS GPU node**: Vina-GPU uses OpenCL. Does the NixOS host-path
   `/run/opengl-driver` provide a usable OpenCL ICD that Vina-GPU can pick up at runtime,
   or do we need to install an OpenCL ICD loader inside the container that points to the
   host driver explicitly? Validate during initial container build.

## Technical Constraints

- **No OpenMM**: All MD and refinement work uses GROMACS. This includes the short MD
  relaxation and any trajectory analysis. GROMACS 2024.x or later.
- **GPU scheduling**: `vina-gpu`, `gnina`, and `diffdock` require `nvidia.com/gpu`
  resource requests, the `gpu=true:NoSchedule` toleration, and the WP-9 NixOS host-path
  mounts (`/run/opengl-driver`, `/nix/store`,
  `LD_LIBRARY_PATH=/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu`,
  `OCL_ICD_VENDORS=/run/opengl-driver/etc/OpenCL/vendors`).
  All injected by the `gpu` compute class.
- **Single-GPU constraint**: only one GPU pod scheduled at a time on `nixos-gpu`. The
  parallel orchestrator queues GPU engine pods serially within a job.
- **AmberTools**: Required for GAFF2 parameterization (antechamber, parmchk2, tleap).
  AmberTools is free and open-source. Install in the GROMACS refinement container.
- **gmx_MMPBSA**: Python package that wraps AmberTools MMPBSA.py for GROMACS trajectories.
  Install in the MM-GBSA container.
- **Backward compatibility**: The existing `POST /api/v1/docking/submit` endpoint and
  `docking` plugin YAML must continue to work. New multi-engine functionality is additive.
  The legacy Vina 1.1 container can stay until Vina 1.2 ships and is validated.
- **Parallel docking pattern**: Reuse and generalize the existing `parallel_docking.go`
  fan-out pattern. Do not build a separate orchestration mechanism.
- **Result writer**: The existing `result-writer` service uses a staging table pattern.
  New engine results should feed through the same staging mechanism for consistency.
- **Vina-GPU pin**: pin to a specific upstream commit (not `latest`); rebuild requires
  the regression test (±0.3 kcal/mol vs `vina-1.2` on a 10-ligand smoke set) to pass.

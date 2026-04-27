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

1. **Smina container** (`zot.hwcopeland.net/chem/smina:latest`)
   - Smina (Vina fork with custom scoring and flexible residue support)
   - Input: receptor PDBQT + ligand PDBQT + binding-site box
   - Output: docked poses (PDBQT), affinity scores (kcal/mol)
   - Replaces AutoDock Vina as the primary physics-based engine (Smina is a strict superset)

2. **Gnina container** (`zot.hwcopeland.net/chem/gnina:latest`)
   - Gnina (CNN-scored docking)
   - Input: receptor PDBQT + ligand SDF/PDBQT + binding-site box
   - Output: docked poses, CNN score, CNN affinity, Vina affinity
   - Requires GPU scheduling (WP-9 compute classes)

3. **DiffDock container** (`zot.hwcopeland.net/chem/diffdock:latest`)
   - DiffDock-L (diffusion-based blind docking)
   - Input: receptor PDB + ligand SMILES or SDF
   - Output: predicted poses (SDF) with confidence scores
   - Requires GPU scheduling

4. **Consensus scoring module** (library within the API or a sidecar):
   - Run 2+ engines on the same receptor-ligand pair
   - Normalize scores to a common scale (rank-based percentile normalization)
   - Consensus score = mean of normalized per-engine ranks
   - Flag compounds where engines disagree by more than 2 standard deviations
   - Configurable: user selects which engines to run via job spec

### Parallel fan-out

5. **Refactored parallel orchestration:**
   - Generalize `parallel_docking.go` to support multiple engines
   - Per-engine: fan out ligand chunks to N worker pods
   - Cross-engine: launch engine pods in parallel (Smina batch + DiffDock batch)
   - Result aggregation: collect all engine results, compute consensus, write to DB
   - Respect WP-9 compute classes: Smina/Gnina on GPU nodes, DiffDock on GPU nodes,
     consensus scoring on CPU

### Pose refinement

6. **Short MD relaxation container** (`zot.hwcopeland.net/chem/gromacs-refine:latest`)
   - **Engine: GROMACS** (no OpenMM)
   - Input: protein-ligand complex (PDB from docked pose)
   - Protocol: energy minimization (steepest descent, 5000 steps) followed by 1 ns NVT MD
     with position restraints on protein backbone
   - Force field: AMBER ff14SB (protein) + GAFF2 (ligand, parameterized via AmberTools
     antechamber/parmchk2)
   - Output: relaxed complex PDB, trajectory (XTC), energy log

7. **MM-GBSA rescoring container** (`zot.hwcopeland.net/chem/mmgbsa:latest`)
   - **Engine: GROMACS + gmx_MMPBSA** (AmberTools-based, runs on GROMACS trajectories)
   - Input: MD trajectory from step 6
   - Protocol: extract last 200 ps of trajectory, compute MM-GBSA binding free energy
     with per-residue decomposition
   - Output: delta-G (kcal/mol), per-residue energy contributions (JSON), standard error

8. **PLIP interaction analysis:**
   - Run PLIP (Protein-Ligand Interaction Profiler) on the refined complex
   - Output: interaction types (H-bond, hydrophobic, pi-stacking, salt bridge, water bridge),
     interacting residues, distances, angles

9. **ProLIF interaction fingerprint:**
   - Refactor existing ProLIF sidecar integration (`handlers_prolif.go`) to run on the
     refined pose (not just the raw docked pose)
   - Generate binary interaction fingerprint (bit vector) for downstream ML/SAR

### CRDs and output

10. **DockJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `DockJob`)
    - Spec: `targetRef` (reference to TargetPrep), `libraryRef` (reference to LibraryPrep),
      `engines` (array of engine configs), `consensus` (boolean), `topN` (number of poses
      to refine), `chunkSize`
    - Status: `phase`, `enginesCompleted`, `totalLigands`, `dockedLigands`, `bestAffinity`,
      `consensusComputed`, `startTime`, `completionTime`, `provenance`

11. **RefineJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `RefineJob`)
    - Spec: `dockJobRef` (reference to DockJob), `topN`, `mdLength` (ns),
      `mmgbsaFrames` (number of frames for MM-GBSA)
    - Status: `phase`, `refinedCount`, `startTime`, `completionTime`, `provenance`

12. **Output per compound** (stored in DB + S3):
    - Best pose coordinates (PDB/SDF)
    - Score breakdown: per-engine affinity, consensus rank, MM-GBSA delta-G
    - Interaction fingerprint (ProLIF bit vector)
    - Per-residue energy contributions from MM-GBSA decomposition (JSON)
    - PLIP interaction summary (JSON)

13. **API endpoints:**
    - `POST /api/v1/docking/submit` -- submit a dock job (replaces existing endpoint with
      richer spec)
    - `GET /api/v1/docking/jobs/{name}` -- get dock job status (extend existing)
    - `GET /api/v1/docking/jobs/{name}/results` -- paginated ranked results (extend existing)
    - `POST /api/v1/docking/jobs/{name}/refine` -- submit a refinement job for top N poses
    - `GET /api/v1/docking/jobs/{name}/refine/{compoundId}` -- get refinement results

14. **Benchmark validation:**
    - Run against DUD-E or LIT-PCBA benchmark set (operator to specify which)
    - Report enrichment factor (EF1%, EF5%), AUC-ROC, and BEDROC
    - Benchmark must run as a reproducible script (`benchmark/sbdd-benchmark.sh`)
    - Store benchmark results with provenance for regression tracking

15. **Tests:**
    - Unit tests for consensus scoring normalization
    - Unit tests for MM-GBSA energy parsing
    - Integration test: dock 10 ligands against 7jrn with Smina, verify ranked results
    - Integration test: refine top 3 poses with GROMACS MD, verify trajectory output
    - E2E smoke test: submit dock job via API, poll until Succeeded, submit refine job,
      verify per-residue energies exist

## Acceptance Criteria

1. Submitting a DockJob with `engines: ["smina"]` against target 7jrn and a 100-compound
   library produces ranked results sorted by affinity within 30 minutes.
2. Submitting a DockJob with `engines: ["smina", "diffdock"], consensus: true` produces a
   consensus-scored result set where the `consensus_rank` column differs from either
   single-engine ranking for at least some compounds.
3. RefineJob on the top 5 poses produces: a GROMACS trajectory file (XTC), an MM-GBSA
   delta-G value with standard error, and per-residue energy contributions for each of the
   5 compounds.
4. The PLIP analysis output for each refined pose includes at least one identified interaction
   type (H-bond, hydrophobic, etc.) with residue ID and distance.
5. The ProLIF interaction fingerprint is a binary bit vector of length >= 50 that can be
   used for Tanimoto similarity comparison between compounds.
6. Benchmark validation against DUD-E (or LIT-PCBA) produces an EF1% value and AUC-ROC.
   The specific numeric thresholds are informational (not pass/fail) for the initial release.
7. The existing docking API endpoints (`/api/v1/docking/submit`,
   `/api/v1/docking/jobs/{name}`) continue to work for backward compatibility with the
   current Vina-only workflow. The new multi-engine spec is additive.
8. All GROMACS containers use GROMACS (not OpenMM). The Dockerfile must not install or
   import OpenMM.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-1 | Consumes prepared receptor and binding-site box |
| Blocked by | WP-2 | Consumes prepared compound library |
| Blocked by | WP-9 | S3 storage, CRD framework, compute classes (GPU scheduling) |
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

5. **Gnina GPU requirements**: Gnina requires CUDA. What GPU type is available on the
   cluster? Is there a GPU node with sufficient memory for Gnina batches?

## Technical Constraints

- **No OpenMM**: All MD and refinement work uses GROMACS. This includes the short MD
  relaxation and any trajectory analysis. GROMACS 2024.x or later.
- **GPU scheduling**: DiffDock and Gnina require `nvidia.com/gpu` resource requests. WP-9
  must define GPU compute classes before these engines can be deployed.
- **AmberTools**: Required for GAFF2 parameterization (antechamber, parmchk2, tleap).
  AmberTools is free and open-source. Install in the GROMACS refinement container.
- **gmx_MMPBSA**: Python package that wraps AmberTools MMPBSA.py for GROMACS trajectories.
  Install in the MM-GBSA container.
- **Backward compatibility**: The existing `POST /api/v1/docking/submit` endpoint and
  `docking` plugin YAML must continue to work. New multi-engine functionality is additive.
- **Parallel docking pattern**: Reuse and generalize the existing `parallel_docking.go`
  fan-out pattern. Do not build a separate orchestration mechanism.
- **Result writer**: The existing `result-writer` service uses a staging table pattern.
  New engine results should feed through the same staging mechanism for consistency.

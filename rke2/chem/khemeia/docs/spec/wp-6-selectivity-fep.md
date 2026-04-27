# WP-6: Selectivity and Free-Energy Refinement

## Owner

TBD

## Scope

Build the selectivity profiling and free-energy perturbation (FEP) stage. Given refined poses
from WP-3, this WP predicts off-target binding risk through panel docking against related
protein families, then computes rigorous relative and absolute binding free energies for
lead-series compounds. The output is a selectivity heatmap and per-compound free-energy
estimates that replace empirical scoring with physics-based ranking.

### Current state

No selectivity analysis or FEP capability exists in the platform. Compounds are ranked by
Vina affinity scores against a single target. There is no way to assess off-target risk, and
no thermodynamic integration or free-energy perturbation pipeline.

The docking infrastructure (WP-3) produces refined poses with MM-GBSA scores, which serve as
the starting structures for this WP's FEP calculations.

## Deliverables

### Selectivity panels

1. **Curated selectivity panel definitions:**

   | Panel | Approximate Size | Source |
   |-------|-----------------|--------|
   | Kinome | ~520 human kinase catalytic domains | KLIFS (curated structures) or PDB-mined |
   | Aminergic GPCR | ~30 receptors (5-HT, dopamine, adrenergic, muscarinic, histamine) | GPCRdb curated structures |
   | Nuclear receptor | ~48 receptors (steroid, thyroid, retinoid, orphan) | PDB-mined, one structure per receptor |

   - Each panel entry: receptor name, UniProt ID, PDB ID (representative structure),
     binding-site box coordinates (from co-crystallized ligand or pre-computed)
   - Panels stored as JSON definitions in S3 (WP-9) and versioned

2. **Panel docking pipeline:**
   - Input: compound set (from WP-3 hit list) + panel name
   - For each compound x panel-member pair: dock using Smina (WP-3 engine) with the
     panel member's binding site
   - Reuse the WP-3 parallel fan-out: fan out across compounds and panel members
   - Output: affinity matrix (compounds x panel members) stored as a structured JSON document
   - Scale: for 50 compounds against the kinome panel (~520 targets), that is ~26,000 docking
     runs. Use the WP-3 chunk-based parallelism with appropriate chunk sizing.

3. **Selectivity heatmap data:**
   - Compute selectivity index per compound: ratio of primary-target affinity to mean
     off-target affinity
   - Flag compounds with strong off-target hits (affinity within 1.5 kcal/mol of primary
     target)
   - Output: JSON structure suitable for rendering as a heatmap in WP-7 UI (compound rows x
     panel-member columns, cells contain affinity values, color-coded by selectivity)

### Free-energy perturbation

4. **RBFE pipeline container** (`zot.hwcopeland.net/chem/gromacs-rbfe:latest`)
   - **Engine: GROMACS + pmx** (no OpenMM)
   - pmx (https://github.com/deGrootLab/pmx) for hybrid topology generation
   - Protocol:
     a. Take two ligands (A and B) with refined poses from WP-3 RefineJob
     b. pmx generates the hybrid topology (dual-topology approach) and mutation files
     c. GROMACS runs the alchemical transformation: energy minimization, NVT equilibration
        (100 ps), NPT equilibration (100 ps), production alchemical MD at 12 lambda windows
        (default; configurable), 2 ns per lambda window
     d. Free energy computed via MBAR (multistate Bennett acceptance ratio, via alchemlyb)
   - Input: two ligand PDB/SDF files (from WP-3 refined poses), receptor PDB, binding-site
     definition
   - Output: relative binding free energy (delta-delta-G in kcal/mol), statistical error,
     per-lambda-window overlap metrics, convergence plot data
   - Force field: AMBER ff14SB (protein) + GAFF2 (ligands via AmberTools antechamber)
   - Compute: this is the most expensive calculation in the platform. A single edge (A->B)
     at 12 lambda windows x 2 ns = 24 ns total MD. Budget ~2-4 hours per edge on a modern
     GPU.

5. **ABFE pipeline container** (`zot.hwcopeland.net/chem/gromacs-abfe:latest`)
   - **Engine: GROMACS + pmx** (no OpenMM)
   - Absolute binding free energy: decouples a single ligand from the protein
   - Protocol: Boresch restraints, dual decoupling (electrostatics then van der Waals),
     20 lambda windows (default), 5 ns per window
   - Input: single ligand PDB/SDF (from WP-3 refined pose), receptor PDB
   - Output: absolute binding free energy (delta-G in kcal/mol), statistical error
   - Use case: when there is no congeneric series for RBFE (e.g., scaffold-hopping hits from
     WP-5), or to anchor the RBFE cycle

6. **FEP network planner** (library or sidecar, not a separate container):
   - Given a set of N ligands, compute pairwise similarity (Tanimoto on Morgan fingerprints)
   - Build a minimum spanning tree of perturbation edges where each edge connects similar
     ligands (Tanimoto > 0.4)
   - Add cycle-closure edges (~20% additional edges) for thermodynamic cycle consistency
     checks
   - Output: perturbation map (graph of edges to run) with estimated computational cost
   - Reject ligand pairs with Tanimoto < 0.3 as RBFE perturbations (too dissimilar; route
     through ABFE instead)

### CRDs

7. **SelectivityJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `SelectivityJob`)
   - Spec: `compoundIds` (array) or `dockJobRef` + `topN`, `panel` (enum: `kinome`,
     `aminergic-gpcr`, `nuclear-receptor`, or custom panel S3 reference),
     `affinityThreshold` (kcal/mol, for off-target flagging), `chunkSize`
   - Status: `phase`, `totalPairs`, `completedPairs`, `offTargetHits`,
     `startTime`, `completionTime`, `provenance`

8. **RBFEJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `RBFEJob`)
   - Spec: `refineJobRef` (reference to WP-3 RefineJob for starting poses), `ligandPairs`
     (array of [compoundIdA, compoundIdB] or `auto` for network-planned edges),
     `lambdaWindows` (default 12), `productionLength` (ns per window, default 2),
     `forceField` (default `ff14sb-gaff2`)
   - Status: `phase`, `edgesTotal`, `edgesCompleted`, `convergedEdges`,
     `meanError` (kcal/mol), `startTime`, `completionTime`, `provenance`

9. **ABFEJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `ABFEJob`)
   - Spec: `refineJobRef`, `compoundIds` (array), `lambdaWindows` (default 20),
     `productionLength` (ns per window, default 5), `restraintType` (enum: `boresch`,
     default `boresch`)
   - Status: `phase`, `compoundsTotal`, `compoundsCompleted`,
     `startTime`, `completionTime`, `provenance`

10. **API endpoints:**
    - `POST /api/v1/selectivity/submit` -- submit a selectivity panel docking job
    - `GET /api/v1/selectivity/jobs/{name}` -- get SelectivityJob status
    - `GET /api/v1/selectivity/jobs/{name}/heatmap` -- get affinity matrix as heatmap data
    - `POST /api/v1/fep/rbfe` -- submit an RBFE calculation
    - `GET /api/v1/fep/rbfe/{name}` -- get RBFEJob status and results
    - `POST /api/v1/fep/abfe` -- submit an ABFE calculation
    - `GET /api/v1/fep/abfe/{name}` -- get ABFEJob status and results
    - `GET /api/v1/fep/rbfe/{name}/network` -- get the perturbation network graph

11. **Benchmark validation (Wang 2015):**
    - Run the RBFE pipeline against the Wang et al. 2015 FEP benchmark set (8 protein targets,
      ~200 ligand pairs)
    - Success criterion: mean unsigned error (MUE) within 1 kcal/mol of experimental values
      across the benchmark set
    - Reproduce as a scripted benchmark (`benchmark/fep-benchmark.sh`) with provenance
    - Store per-edge results for regression tracking across pipeline updates

12. **Tests:**
    - Unit tests for FEP network planner (verify MST construction, cycle closure, similarity
      thresholds)
    - Unit tests for selectivity index calculation
    - Integration test: dock 5 compounds against a 3-member mini-panel, verify affinity
      matrix output
    - Integration test: run RBFE on a single edge (two similar ligands) with 2 lambda windows
      and 100 ps production (fast test, not converged), verify delta-delta-G output
    - E2E smoke test: submit SelectivityJob via API, poll until Succeeded, verify heatmap
      data JSON

## Acceptance Criteria

1. Panel docking: submitting a SelectivityJob for 10 compounds against the aminergic GPCR
   panel (~30 targets) produces a 10x30 affinity matrix within 4 hours. Each cell contains
   a docking affinity value (kcal/mol).
2. Selectivity heatmap data: the JSON output includes per-compound selectivity index values
   and an `off_target_hits` array listing panel members where the compound binds within 1.5
   kcal/mol of its primary-target affinity.
3. RBFE: a single edge (two congeneric ligands from the Wang 2015 set, e.g., JNK1 series)
   produces a delta-delta-G with statistical error. The pmx hybrid topology is generated
   without manual intervention.
4. RBFE with `auto` network planning: given 10 ligands, the planner produces a perturbation
   network with at least 9 edges (MST) plus cycle-closure edges, and rejects pairs with
   Tanimoto < 0.3.
5. ABFE: a single ligand decoupling calculation produces a delta-G value. The Boresch
   restraints are automatically selected from the binding pose geometry.
6. Wang 2015 benchmark: the full benchmark run completes and reports per-target MUE values.
   The specific numeric threshold (within 1 kcal/mol) is a target, not a hard pass/fail
   for the initial release.
7. All GROMACS containers use GROMACS and pmx. No OpenMM imports or installations exist in
   any container.
8. RBFE and ABFE provenance records include: input pose references, force field version, pmx
   version, GROMACS version, lambda schedule, and simulation parameters.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-3 | Consumes refined poses (RefineJob output) as starting structures |
| Blocked by | WP-1 | Selectivity panels require prepared receptor structures |
| Blocked by | WP-9 | CRD framework, GPU compute classes, S3 storage, provenance |
| Integrates with | WP-7 | UI displays selectivity heatmap and FEP results |
| Consumed by | WP-8 | Reporting includes selectivity profiles and FEP data |
| Soft dependency | WP-5 | Generated compounds from SAR expansion can be FEP endpoints |

## Out of Scope

- Absolute protein stability calculations (protein-only FEP)
- Covalent binding free energy calculations
- Enhanced sampling methods beyond standard alchemical FEP (metadynamics, REST2, etc.)
- Water-mediated free energy contributions (Grand Canonical MC water placement)
- Force field development or parameterization beyond GAFF2
- Crystal structure determination or homology modeling for panel members (use existing PDB
  structures)
- Kinome-wide profiling from experimental assays (this is computational prediction only)

## Open Questions

1. **Panel structure curation**: The kinome panel has ~520 members, but not all have
   high-quality crystal structures with co-crystallized ligands. Should the panel be limited
   to structures with resolution < 2.5 A and a bound ligand, or include apo structures with
   predicted binding sites (WP-1 pocket detection)?

2. **RBFE convergence criteria**: 2 ns per lambda window is a starting point. For
   production use, 5-10 ns may be needed. Should the pipeline support adaptive window
   length based on overlap metrics, or use a fixed length with a convergence warning?

3. **pmx compatibility**: pmx was originally developed for GROMACS 2021.x. Verify
   compatibility with GROMACS 2024.x (the version specified in WP-3). If incompatible,
   either pin an older GROMACS version for FEP containers or contribute upstream patches.

4. **GPU requirements for FEP**: RBFE with 12 lambda windows running in parallel requires
   12 GPU slots (or serial execution on fewer GPUs). What is the cluster's GPU capacity?
   Should lambda windows run in parallel (fast but GPU-hungry) or serial (slow but
   GPU-efficient)?

5. **ABFE restraint selection**: Boresch restraint selection requires stable protein-ligand
   contacts. For weak binders or flexible binding modes, automatic restraint selection may
   fail. What is the fallback: manual restraint specification or skipping ABFE for that
   compound?

## Technical Constraints

- **No OpenMM**: All MD and FEP work uses GROMACS. This includes RBFE alchemical
  transformations and ABFE decoupling. GROMACS 2024.x or later.
- **pmx**: Required for hybrid topology generation. Install in the GROMACS FEP containers.
  pmx is Python-based and integrates with GROMACS topology files.
- **alchemlyb**: Python package for free energy analysis (MBAR, BAR, TI). Install in the
  FEP containers for post-processing.
- **AmberTools**: Required for GAFF2 ligand parameterization (same as WP-3 refinement).
  Consistent force field assignment across WP-3 and WP-6 is mandatory.
- **Compute cost**: FEP is the most expensive calculation in the platform. A single RBFE
  edge takes 2-4 hours on a GPU. A 10-compound network with 15 edges = 30-60 GPU-hours.
  Budget GPU resources accordingly and make cost estimates visible in the UI (WP-7).
- **Lambda window scheduling**: Each lambda window is an independent MD simulation. Use
  K8s Job arrays (one Job per window) with the WP-9 compute class `gpu`. If GPUs are
  scarce, serialize windows within a single pod.
- **Panel storage**: Panel definitions (receptor structures + binding sites) are large (PDB
  files for ~520 kinases). Store in S3 (WP-9 MinIO), not in the database. Cache on local
  `emptyDir` volumes during docking runs.
- **Backward compatibility**: This WP introduces new API endpoints. No existing endpoints
  are modified.

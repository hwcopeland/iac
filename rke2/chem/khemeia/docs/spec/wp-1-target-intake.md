# WP-1: Target Intake and Binding-Site Definition

## Owner

TBD

## Scope

Build the receptor preparation and binding-site definition stage of the SBDD pipeline. Given
a protein target (PDB ID or user-uploaded structure), this work package prepares a
docking-ready receptor and defines the binding site using one of three modes. The output is a
`TargetPrep` CRD instance containing the cleaned receptor coordinates and a binding-site box
specification that downstream stages (WP-3 docking, WP-6 selectivity) consume directly.

### Current state

The existing prototype has a single hard-coded receptor prep path in the docking plugin:
`containers/autodock-vina/scripts/proteinprepv2.py` runs `pdb_fetch`, extracts the native
ligand, and computes a grid center. Grid coordinates are stored in `docking_workflows` table
columns (`grid_center_x/y/z`). There is no PDBFixer step, no pocket detection, no custom box
mode, and no standalone receptor prep separate from docking.

This WP replaces that embedded prep with a standalone, reusable target preparation stage.

## Deliverables

1. **PDBFixer-based receptor preparation container** (`zot.hwcopeland.net/chem/target-prep:latest`)
   - Fetch PDB/mmCIF from RCSB (or accept user upload)
   - Remove water, heteroatoms (except specified co-factors)
   - Add missing heavy atoms and hydrogens (PDBFixer)
   - Assign protonation states at physiological pH (7.4)
   - Output: cleaned PDB file, preparation log, provenance metadata

2. **Three binding-site definition modes:**
   - **Native-ligand mode**: Extract co-crystallized ligand coordinates, compute centroid,
     define box with configurable padding (default: 10 A per side). This is the current
     behavior, refactored into the new pipeline.
   - **Custom-box mode**: User specifies center (x, y, z) and dimensions (sx, sy, sz)
     directly. No computation required; validate that the box intersects the receptor.
   - **Pocket detection mode**: Run fpocket and P2Rank independently on the cleaned
     receptor. Rank pockets by consensus score (average of normalized fpocket druggability
     score and P2Rank probability). Present the top N pockets (default: 5) to the user for
     selection. If running unattended, select the top-ranked pocket automatically.

3. **fpocket container** (`zot.hwcopeland.net/chem/fpocket:latest`)
   - fpocket 4.x
   - Input: cleaned PDB
   - Output: pocket PDB files, druggability scores (JSON)

4. **P2Rank container** (`zot.hwcopeland.net/chem/p2rank:latest`)
   - P2Rank 2.4+
   - Input: cleaned PDB
   - Output: predicted pockets with probabilities (CSV/JSON)

5. **Consensus pocket ranker** (library, not a separate container)
   - Normalize fpocket druggability score to [0, 1]
   - Normalize P2Rank probability to [0, 1]
   - Consensus score = mean of both normalized scores
   - Output: ranked pocket list with coordinates, dimensions, and scores

6. **TargetPrep CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `TargetPrep`)
   - Spec fields: `pdbId`, `uploadRef` (S3 key for user uploads), `bindingSiteMode`
     (enum: `native-ligand`, `custom-box`, `pocket-detection`), `nativeLigandId`,
     `customBox` (center + dimensions), `pocketCount`, `padding`, `pH`
   - Status fields: `phase` (Pending/Running/Succeeded/Failed), `receptor` (S3 key to
     cleaned PDB), `bindingSite` (center + dimensions), `pockets` (array of detected
     pockets with scores), `startTime`, `completionTime`, `provenance`

7. **API endpoints:**
   - `POST /api/v1/targets/prepare` -- submit a target prep job
   - `GET /api/v1/targets/{name}` -- get target prep status and results
   - `GET /api/v1/targets/{name}/pockets` -- list detected pockets (pocket-detection mode)
   - `POST /api/v1/targets/{name}/pockets/{index}/select` -- select a pocket for downstream use

8. **Plugin YAML** (`plugins/target-prep.yaml`) following the existing plugin schema in
   `api/plugin.go`

9. **Tests:**
   - Unit tests for PDBFixer prep logic (mock PDB input, verify output has no waters, has
     hydrogens)
   - Unit tests for consensus pocket ranking (verify score normalization and ranking)
   - Integration test: submit PDB ID `7jrn` (existing benchmark target), verify receptor
     output and binding-site coordinates within tolerance of current known-good values
   - E2E smoke test: submit target prep via API, poll until Succeeded, verify receptor S3
     object exists

## Acceptance Criteria

1. `POST /api/v1/targets/prepare` with `{"pdbId": "7jrn", "bindingSiteMode": "native-ligand", "nativeLigandId": "TTT"}` returns 202 and the job reaches `Succeeded` within 5 minutes.
2. The cleaned receptor PDB contains no water molecules (residue name `HOH`), has hydrogens added, and parses without errors in RDKit or BioPython.
3. In native-ligand mode, the computed binding-site center is within 2.0 A RMSD of the ligand centroid in the original PDB structure.
4. In custom-box mode, submitting a box that does not intersect the receptor returns a 422 validation error.
5. In pocket-detection mode, both fpocket and P2Rank containers run to completion, and the consensus-ranked pocket list contains at least one pocket with a score above 0.5 for a known druggable target (e.g., 7jrn).
6. The `TargetPrep` CRD status includes a `provenance` field that records: source PDB ID or upload key, PDBFixer version, pocket detection tool versions, and timestamps.
7. All output files (cleaned PDB, pocket predictions) are written to the S3 object store (WP-9) under a deterministic key derived from the job name.
8. The target-prep plugin YAML loads successfully and generates routes at API startup.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-9 | S3 object store for receptor/pocket output storage |
| Blocked by | WP-9 | CRD framework for TargetPrep resource definition |
| Blocked by | WP-9 | Provenance system for metadata recording |
| Blocks | WP-3 | Docking consumes the prepared receptor and binding-site box |
| Blocks | WP-6 | Selectivity panel docking uses the same receptor prep output |
| Soft dependency | WP-7 | UI for pocket selection in pocket-detection mode |

## Out of Scope

- Docking (WP-3)
- Homology modeling or AlphaFold structure prediction
- Covalent warhead definition
- Allosteric site prediction (beyond what fpocket/P2Rank detect as general pockets)
- Multi-chain / multi-receptor ensemble prep
- Receptor flexibility (flexible side chains are a WP-3 concern)

## Open Questions

1. **Co-factor handling**: Which co-factors should be retained during receptor cleaning? A
   blanket "remove all heteroatoms" policy loses metal ions (Zn, Mg) and co-factors (FAD,
   NAD) that are structurally important. Define a default keep-list or make it user-configurable?

2. **Protonation state engine**: PDBFixer adds hydrogens but does not perform pKa-aware
   protonation. Should we integrate PropKa or H++ for pH-dependent protonation, or accept
   PDBFixer defaults for the initial release?

3. **User uploads**: What file formats beyond PDB/mmCIF should be supported? AlphaFold
   models are PDB-format but lack experimental metadata -- should we flag them differently?

4. **Pocket detection timeout**: fpocket is fast (seconds); P2Rank can take minutes on large
   structures. What is the acceptable timeout for pocket detection before falling back to
   a single-tool result?

## Technical Constraints

- **Container registry**: All images pushed to `zot.hwcopeland.net/chem/`. Pull credentials
  via `zot-pull-secret` ExternalSecret (already deployed).
- **Namespace**: All jobs run in the `chem` namespace.
- **RBAC**: The `khemeia-controller` ServiceAccount has `batch/jobs` CRUD and `pods/log`
  read in `chem`. New CRDs require a ClusterRole or namespace-scoped Role extension.
- **Storage**: Receptor files are currently stored as BLOBs in MySQL
  (`docking_workflows.receptor_pdbqt`). WP-9 provides S3 (MinIO) for file storage. During
  transition, support both paths with a feature flag.
- **Base images**: Prefer `python:3.11-slim` for Python containers (consistent with existing
  `prolif-runner`). Use multi-stage builds for Go components.
- **No OpenMM**: Not applicable to this WP, but noted for consistency.

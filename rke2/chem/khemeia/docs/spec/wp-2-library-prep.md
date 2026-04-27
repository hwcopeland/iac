# WP-2: Library Intake and Preparation

## Owner

TBD

## Scope

Build the compound library ingestion, standardization, filtering, and 3D conformer generation
stage. This is the second stage of the SBDD hot path. Given one or more compound sources, this
WP produces a docking-ready library of 3D molecular structures with stable identifiers,
provenance tracking, and pre-filter annotations.

### Current state

The existing prototype has two compound intake paths:
1. **SDF file import**: Ligands are loaded from an SDF file on a shared PVC. The
   `prep_ligands.py` script converts SMILES to PDBQT via Meeko/RDKit. No standardization,
   no filtering, no provenance.
2. **ChEMBL search/import**: `handlers_chembl.go` queries a read-only ChEMBL MySQL database,
   computes RDKit descriptors (MW, LogP, HBA, HBD, PSA, QED), and imports selected compounds
   into the `ligands` table. Basic Lipinski/Veber/lead-like flags are computed at import time
   (`handlers_analysis.go`, `ADMETFlags` struct).

Both paths are tightly coupled to the docking workflow. This WP creates a standalone library
preparation stage that any downstream consumer (docking, ADMET, generative) can use.

## Deliverables

1. **Multi-source ingestion handlers:**
   - **SDF file upload**: Accept SDF/SDF.gz via multipart upload or S3 reference. Parse with
     RDKit `SDMolSupplier`. Extract molecule name, SMILES, and any SD properties.
   - **SMILES file/list**: Accept a text file (one SMILES per line) or JSON array of SMILES
     strings.
   - **ChEMBL query**: Refactor existing `handlers_chembl.go` into the library prep pipeline.
     User specifies ChEMBL filters (target, activity type, pChEMBL threshold, max_phase).
     The handler queries the ChEMBL database and feeds results into the standardization step.
   - **Enamine REAL subset**: Accept an Enamine REAL Space subset file (SMILES + Enamine IDs).
     Tag provenance as `enamine-real`. Do not implement on-demand enumeration of the full
     REAL Space in this WP (that is a future enhancement).

2. **Standardization pipeline container** (`zot.hwcopeland.net/chem/library-prep:latest`)
   - RDKit `MolStandardize`: sanitize, normalize, uncharge, choose canonical tautomer
   - ChEMBL Structure Pipeline: `standardiser.standardise()` for additional salt stripping
     and standardization
   - Generate canonical SMILES and InChIKey from the standardized molecule
   - Reject molecules that fail sanitization (log reason, do not silently drop)

3. **Pre-filter suite** (each independently toggleable via job spec):

   | Filter | Default | Rule |
   |--------|---------|------|
   | Lipinski Ro5 | ON | MW <= 500, LogP <= 5, HBA <= 10, HBD <= 5; pass if <= 1 violation |
   | Veber | ON | PSA <= 140 A^2, rotatable bonds <= 10 |
   | PAINS | ON | Reject PAINS-A, PAINS-B, PAINS-C substructure matches (RDKit FilterCatalog) |
   | Brenk | OFF | Reject Brenk unwanted substructures (RDKit FilterCatalog) |
   | REOS | OFF | Rapid Elimination of Swill: MW 200-500, LogP -5 to 5, HBD 0-5, HBA 0-10, formal charge -2 to 2, rotatable bonds 0-8, heavy atoms 15-50 |

   Each filter produces a boolean pass/fail annotation on the compound record. Compounds that
   fail enabled filters are marked `filtered=true` but not deleted -- downstream stages can
   choose to include or exclude filtered compounds.

4. **3D conformer generation:**
   - ETKDG (RDKit `EmbedMolecule` with `ETKDGv3`) for initial 3D coordinates
   - MMFF94 force field optimization (max 200 iterations)
   - Generate one low-energy conformer per compound (not an ensemble)
   - Output: SDF with 3D coordinates, or PDBQT via Meeko for direct Vina consumption
   - Compounds that fail embedding (e.g., impossible ring systems): mark as
     `conformer_failed=true`, log the error, continue processing

5. **Stable compound IDs with provenance:**
   - Primary key: auto-increment integer (internal, not exposed)
   - Stable ID: `KHM-{InChIKey_first14}` (e.g., `KHM-BSYNRYMUTXBXSQ`)
   - If duplicate InChIKey detected: reuse existing stable ID, append new provenance record
   - Provenance record: source type (sdf, smiles, chembl, enamine), source identifier
     (filename, ChEMBL ID, Enamine ID), import timestamp, import job name

6. **LibraryPrep CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `LibraryPrep`)
   - Spec fields: `sources` (array of source definitions), `filters` (map of filter name to
     enabled boolean), `conformerMethod` (default: `etkdg`), `outputFormat` (enum: `sdf`,
     `pdbqt`), `chunkSize` (for parallel processing)
   - Status fields: `phase`, `totalCompounds`, `passedFilter`, `failedFilter`,
     `conformersFailed`, `duplicatesFound`, `startTime`, `completionTime`, `provenance`

7. **API endpoints:**
   - `POST /api/v1/libraries/prepare` -- submit a library prep job
   - `GET /api/v1/libraries/{name}` -- get library prep status and summary statistics
   - `GET /api/v1/libraries/{name}/compounds` -- paginated compound list with filter/sort
   - `GET /api/v1/libraries/{name}/compounds/{id}` -- single compound detail with provenance

8. **Plugin YAML** (`plugins/library-prep.yaml`)

9. **Tests:**
   - Unit tests for standardization (known edge cases: salts, tautomers, stereochemistry)
   - Unit tests for each filter (known PAINS hits should fail, known drug-like molecules
     should pass)
   - Unit tests for stable ID generation and deduplication
   - Integration test: import 100 compounds from test SDF, verify counts after filtering
   - E2E smoke test: submit library prep via API, poll until Succeeded, verify compound
     records exist with 3D coordinates

## Acceptance Criteria

1. Submitting an SDF file with 100 molecules via `POST /api/v1/libraries/prepare` produces a
   library with exactly 100 compound records (some may be `filtered=true`), each with a
   `KHM-*` stable ID, canonical SMILES, and InChIKey.
2. With all filters enabled (Lipinski, Veber, PAINS, Brenk, REOS), aspirin (SMILES:
   `CC(=O)Oc1ccccc1C(=O)O`) passes Lipinski and Veber, and is not flagged by PAINS, Brenk,
   or REOS.
3. A known PAINS compound (e.g., rhodanine, SMILES: `O=C1CSC(=S)N1`) is flagged by the PAINS
   filter.
4. Duplicate SMILES submitted in two separate SDF files produce a single compound record with
   two provenance entries.
5. 3D conformers have non-zero coordinates and pass RDKit `Chem.Mol.GetConformer()` without
   error.
6. Compounds that fail ETKDG embedding are marked `conformer_failed=true` and do not block
   the rest of the library.
7. Disabling a filter (e.g., `"filters": {"pains": false}`) results in no PAINS-flagged
   compounds in the output, and the PAINS column is not present in the filter annotation.
8. The ChEMBL source path queries the existing ChEMBL database
   (`chemblDB` connection in `api/main.go`) and feeds results through the same
   standardization pipeline as SDF imports.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-9 | S3 object store for conformer SDF/PDBQT storage |
| Blocked by | WP-9 | CRD framework for LibraryPrep resource definition |
| Blocked by | WP-9 | Provenance system for source tracking |
| Blocks | WP-3 | Docking consumes the prepared library |
| Blocks | WP-4 | ADMET triage consumes compound SMILES from the library |
| Blocks | WP-5 | Generative SAR uses the library for deduplication checks |
| Soft dependency | WP-1 | Target prep is independent but typically precedes library prep |

## Out of Scope

- Enamine REAL Space on-demand enumeration (billions of compounds; future work)
- Reaction-based library enumeration (combinatorial chemistry)
- Tautomer ensemble generation (one canonical tautomer per compound)
- Multi-conformer ensemble generation (one low-energy conformer per compound)
- Fragment library preparation for FBDD (WP-5 handles fragment-specific logic)
- Descriptor calculation beyond filter requirements (ADMET descriptors are WP-4)

## Open Questions

1. **ChEMBL database version**: The current deployment has a ChEMBL MySQL mirror. What
   version? Should library prep auto-detect the version and include it in provenance?

2. **Enamine REAL format**: Enamine distributes subsets in various formats (SMILES, SDF,
   custom TSV). Which format should be the primary import target? Is there a local copy
   or does this require download from Enamine?

3. **Stereochemistry handling**: Should the standardization pipeline enumerate undefined
   stereocenters as separate compounds, or treat them as a single compound with
   undefined stereochemistry? Enumerating stereocenters can multiply library size
   exponentially.

4. **Chunk size for parallel processing**: The current docking pipeline uses 10,000 ligands
   per chunk. Should library prep use the same default, or is a different chunk size
   appropriate for the lighter-weight standardization/filtering work?

5. **Filter configuration presets**: Should there be named presets (e.g., "drug-like",
   "lead-like", "fragment-like") that set multiple filters at once, or is per-filter
   toggle sufficient?

## Technical Constraints

- **RDKit version**: Pin a specific RDKit release in the container (e.g., `2024.03.x`).
  RDKit standardization behavior can change between releases.
- **ChEMBL Structure Pipeline**: Requires `pip install chembl_structure_pipeline`. Verify
  compatibility with the pinned RDKit version.
- **Memory**: Large SDF files (100K+ molecules) require streaming parsing
  (`ForwardSDMolSupplier`), not loading the entire file into memory.
- **Parallel processing**: For libraries over 10K compounds, split into chunks and process
  in parallel K8s jobs. Reuse the fan-out pattern from `parallel_docking.go`.
- **MySQL schema**: The existing `ligands` table (`docking` database) has columns:
  `id`, `compound_id`, `smiles`, `pdbqt`, `mw`, `logp`, `hba`, `hbd`, `psa`,
  `ro5_violations`, `qed`. Extend this schema or create a new `compounds` table with the
  richer data model.
- **Container base**: `python:3.11-slim` with RDKit installed via conda or pip
  (`rdkit` package).

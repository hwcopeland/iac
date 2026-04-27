# WP-5: Generative SAR Expansion

## Owner

TBD

## Scope

Build a generative chemistry stage that expands the hit set from docking/ADMET triage using
scaffold-conditioned molecular generation, synthesizability filtering, and fragment-based drug
design (FBDD) linking. Generated compounds loop back to Stage 1 (WP-1/WP-2/WP-3) with full
provenance, creating a closed-loop optimization cycle.

### Current state

There is no generative chemistry capability in the existing prototype. The only compound
sources are SDF file import and ChEMBL database search (WP-2 current state).

The FBDD generative pipeline has been discussed as a project goal (see memory reference:
`project_fbdd_pipeline.md`) but no implementation exists.

## Deliverables

1. **ChemMamba serving pod** (`zot.hwcopeland.net/chem/chemmamba:latest`)
   - ChemMamba model for conditional molecular generation
   - Deployment type: persistent in-cluster pod (not a per-job Job), similar to admet_ai
     in WP-4
   - Input: scaffold SMILES (conditioning), generation parameters (temperature, top-k,
     number of samples)
   - Output: batch of generated SMILES with generation metadata
   - Liveness/readiness probes (model load may take 60-120s)
   - GPU scheduling required (WP-9 compute classes)

2. **Scaffold-conditioned generation pipeline:**
   - Input: one or more hit compounds from WP-3 docking results (SMILES + score + ADMET
     profile)
   - Extract Murcko scaffold from each hit
   - Condition ChemMamba on the scaffold to generate analogs
   - Configurable parameters: number of analogs per scaffold (default 100), temperature
     (default 0.8), diversity threshold (minimum Tanimoto distance between generated
     molecules, default 0.3)
   - Post-generation validation: reject invalid SMILES, reject molecules that fail RDKit
     sanitization, reject duplicates (by InChIKey)

3. **Synthesizability filter:**
   - **SAScore**: Synthetic Accessibility Score (Ertl & Schuffenhauer, built into RDKit).
     Threshold: reject if SAScore > 5.0 (configurable).
   - **AiZynthFinder**: Retrosynthetic analysis to verify that a multi-step synthesis route
     exists. Run AiZynthFinder on each generated molecule. Mark as `synthesizable=true` if
     at least one route with <= 6 steps is found.
   - Compounds that pass SAScore but fail AiZynthFinder are flagged
     `synthesizability_uncertain=true` (not rejected outright).

4. **AiZynthFinder container** (`zot.hwcopeland.net/chem/aizynthfinder:latest`)
   - AiZynthFinder with pre-trained USPTO policy network
   - Input: SMILES
   - Output: retrosynthetic routes (JSON), number of steps, route score
   - CPU-only (no GPU required for inference)
   - Per-compound timeout: 60 seconds (configurable). Timeout = `route_not_found`.

5. **De-duplication against reference databases:**
   - Check generated compounds against:
     - The current project library (WP-2 compounds table, by InChIKey)
     - ChEMBL (existing ChEMBL database connection, by InChIKey)
     - ZINC (by InChIKey, if ZINC subset is available locally; otherwise skip)
   - Tag matches: `known_compound=true` with source database and ID
   - Known compounds are not rejected -- they are flagged for the scientist to decide

6. **Fragment linking via DiffLinker** (FBDD support):
   - **DiffLinker container** (`zot.hwcopeland.net/chem/difflinker:latest`)
   - Input: two fragment SMILES + protein structure (PDB) + binding pocket coordinates
   - Output: linked molecules (SMILES) with 3D poses
   - GPU scheduling required
   - Use case: given two fragments that bind in adjacent pockets (identified by WP-1 pocket
     detection or WP-3 fragment docking), generate linker molecules that connect them

7. **Loop-back to Stage 1 with provenance:**
   - Generated compounds feed back into the pipeline as a new library
   - Provenance chain: `source=generated`, `parent_compounds=[KHM-xxx, KHM-yyy]`,
     `generation_method=chemmamba|difflinker`, `generation_parameters={...}`,
     `generation_job=gen-job-abc123`
   - The looped-back library enters WP-2 standardization (same filters, same ID scheme)
   - Provenance must be traversable: given any compound, trace back through all generations
     to the original source

8. **GenerateJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `GenerateJob`)
   - Spec: `sourceCompounds` (array of compound IDs or dockJobRef + topN),
     `scaffoldExtraction` (enum: `murcko`, `generic-murcko`, `custom-smarts`),
     `generationParams` (temperature, topK, samplesPerScaffold, diversityThreshold),
     `synthesizabilityFilter` (SAScore threshold, AiZynthFinder enabled boolean,
     maxSynthSteps), `deduplicateSources` (array of database names)
   - Status: `phase`, `scaffoldsExtracted`, `generated`, `validSmiles`, `uniqueCompounds`,
     `synthesizable`, `knownCompounds`, `startTime`, `completionTime`, `provenance`

9. **LinkJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `LinkJob`)
   - Spec: `fragment1` (SMILES or compound ID), `fragment2` (SMILES or compound ID),
     `targetRef` (TargetPrep reference for protein context), `numLinkers` (default 50),
     `maxLinkerLength` (heavy atoms, default 12)
   - Status: `phase`, `linkersGenerated`, `validLinkers`, `startTime`, `completionTime`,
     `provenance`

10. **API endpoints:**
    - `POST /api/v1/generate/scaffold` -- submit a scaffold-conditioned generation job
    - `GET /api/v1/generate/jobs/{name}` -- get generation job status
    - `GET /api/v1/generate/jobs/{name}/compounds` -- paginated generated compounds
    - `POST /api/v1/generate/link` -- submit a fragment linking job
    - `GET /api/v1/generate/link/{name}` -- get link job status and results

11. **Tests:**
    - Unit tests for Murcko scaffold extraction (known molecules, expected scaffolds)
    - Unit tests for SAScore filtering (known easy-to-synthesize molecules pass, known
      impossible molecules fail)
    - Unit tests for deduplication logic (same InChIKey detected across sources)
    - Integration test: generate 50 analogs of aspirin scaffold via ChemMamba, verify >80%
      valid SMILES, verify deduplication against ChEMBL detects aspirin itself
    - E2E smoke test: submit GenerateJob via API, poll until Succeeded, verify generated
      compounds have provenance linking back to source compounds

## Acceptance Criteria

1. ChemMamba serving pod starts, loads model, and responds to generation requests within
   120 seconds of pod creation. Generating 100 molecules conditioned on a simple scaffold
   (e.g., benzene) completes within 60 seconds.
2. At least 80% of generated SMILES pass RDKit sanitization (are chemically valid).
3. With diversity threshold 0.3, no two generated molecules have Tanimoto similarity > 0.7
   (using Morgan fingerprints, radius 2).
4. SAScore correctly classifies aspirin (SAScore ~2.2) as synthesizable and a known
   hard-to-synthesize molecule (e.g., palytoxin, SAScore >8) as not synthesizable.
5. AiZynthFinder finds at least one synthesis route for aspirin with <= 3 steps. The
   container returns a JSON route with starting materials.
6. Deduplication against ChEMBL: generating aspirin-scaffold analogs, at least one generated
   compound matches an existing ChEMBL compound (tagged `known_compound=true`).
7. DiffLinker generates at least 10 valid linked molecules from two simple fragments (e.g.,
   phenol + pyridine) with a linker length <= 12 heavy atoms.
8. Loop-back provenance: compounds generated by a GenerateJob, when re-submitted to WP-2
   library prep, carry a `source=generated` provenance record with a reference to the
   originating GenerateJob.
9. All generated compounds receive `KHM-*` stable IDs through the WP-2 standardization
   pipeline.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-2 | Standardization pipeline for generated compounds |
| Blocked by | WP-3 | Source hit compounds come from docking results |
| Blocked by | WP-9 | GPU compute classes, CRD framework, provenance system |
| Soft dependency | WP-4 | ADMET scores used to prioritize which scaffolds to expand |
| Loops back to | WP-1 | Generated compounds re-enter the full pipeline |
| Integrates with | WP-7 | UI for generation parameter configuration and results browsing |

## Out of Scope

- Reinforcement learning optimization loops (multi-objective optimization is future work)
- Protein structure-conditioned generation (pocket-based generation without scaffold input)
- Reaction-based enumeration (virtual combinatorial chemistry)
- Custom model training or fine-tuning ChemMamba on proprietary data
- Full retrosynthetic route planning and costing (AiZynthFinder provides feasibility check
  only, not a procurement-ready synthesis plan)
- 3D conformer generation for generated molecules (handled by WP-2 on loop-back)

## Open Questions

1. **ChemMamba model availability**: Is a pre-trained ChemMamba checkpoint publicly available
   for self-hosted deployment? If not, what is the fallback generative model? Options:
   REINVENT 4, MolGPT, or a simpler SMILES-LSTM.

2. **AiZynthFinder stock database**: AiZynthFinder needs a "stock" database of commercially
   available starting materials. Options: eMolecules, Sigma-Aldrich, or a curated subset.
   Which is available for self-hosted deployment?

3. **Fragment sourcing for FBDD**: Where do the input fragments come from? Options:
   (a) Fragment screening hits imported via WP-2, (b) computational fragment decomposition
   of known actives (BRICS/Recap), (c) curated fragment libraries (e.g., Enamine fragment
   collection). Define the primary input path.

4. **Generation budget**: How many analogs per scaffold is reasonable before the pipeline
   becomes compute-bound? 100? 1000? Should there be a hard cap per GenerateJob?

5. **Intellectual property check**: Should generated compounds be checked against patent
   databases? This is non-trivial and may be out of scope, but worth discussing.

## Technical Constraints

- **GPU scheduling**: ChemMamba and DiffLinker require GPU. ChemMamba should be a persistent
  deployment (not spun up per job) to amortize model loading time. DiffLinker can be a
  per-job pod if linking requests are infrequent.
- **AiZynthFinder memory**: AiZynthFinder with the USPTO policy can use 4-8 GB RAM. Size
  the container accordingly. CPU-only.
- **Provenance chain**: The provenance system (WP-9) must support multi-hop traversal.
  A generated compound's provenance chain is: original source -> WP-2 prep -> WP-3 docking
  -> WP-5 generation -> WP-2 re-prep -> WP-3 re-docking. Each hop must be recorded and
  queryable.
- **De-duplication performance**: InChIKey lookup against ChEMBL (~2M compounds) must be
  indexed. The existing ChEMBL MySQL database has `compound_structures.standard_inchi_key`.
  Verify this column is indexed.
- **RDKit version**: Must match the version pinned in the WP-2 library-prep container to
  ensure consistent standardization and fingerprint computation.

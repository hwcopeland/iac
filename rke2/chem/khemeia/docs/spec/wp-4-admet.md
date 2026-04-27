# WP-4: ADMET Triage (Expanded)

## Owner

TBD

## Scope

Build a first-class ADMET (Absorption, Distribution, Metabolism, Excretion, Toxicity)
prediction stage that replaces the current property-flag approximation with deep-learning
models covering the full ADMET endpoint spectrum. This is not a quick filter -- it is a
comprehensive characterization of each compound's drug-likeness and pharmacokinetic profile.

### Current state

The existing prototype computes six binary flags from molecular properties
(`handlers_analysis.go`, `ADMETFlags` struct):
- Lipinski (MW, LogP, HBA, HBD thresholds)
- Veber (PSA threshold)
- Lead-like (MW + LogP + HBA + HBD ranges)
- QED >= 0.5
- P450 risk (LogP > 3)
- High PSA (PSA > 140)

These are descriptor-based heuristics, not predictive models. They cannot predict Caco-2
permeability, CYP inhibition profiles, hERG liability, hepatotoxicity, or any endpoint that
requires training data. This WP replaces them with ML-based predictions while retaining the
existing flags as a fast pre-screen.

### Integration contract with WP-7

ADMET results are displayed inline in the Hit/Compound tab of the UI, not in a separate
report. The JSON schema for per-compound ADMET blocks must be agreed with the WP-7 team
before implementation begins. See the output schema in Deliverable 6.

## Deliverables

1. **admet_ai container** (`zot.hwcopeland.net/chem/admet-ai:latest`)
   - Stanford Chemprop-D model serving (admet_ai package)
   - Input: SMILES string or batch of SMILES
   - Output: per-endpoint prediction with confidence interval
   - Persistent serving: runs as a Deployment (not a Job), accepts HTTP requests
   - Liveness/readiness probes (model load takes 30-60s)

2. **ADMETlab 3.0 container** (`zot.hwcopeland.net/chem/admetlab3:latest`)
   - ADMETlab 3.0 model serving
   - Input: SMILES
   - Output: per-endpoint predictions
   - Used for cross-validation against admet_ai, not as the primary prediction engine

3. **Full endpoint coverage** (all predicted by admet_ai, cross-validated where ADMETlab 3.0
   covers the same endpoint):

   **Absorption:**
   | Endpoint | Unit/Scale | Description |
   |----------|-----------|-------------|
   | Aqueous solubility | log S (mol/L) | Thermodynamic solubility |
   | Caco-2 permeability | log Papp (cm/s) | Intestinal permeability proxy |
   | HIA | % or class | Human intestinal absorption |
   | P-glycoprotein substrate | binary | Efflux transporter substrate |
   | Oral bioavailability (%F) | % | Fraction absorbed and reaching systemic circulation |

   **Distribution:**
   | Endpoint | Unit/Scale | Description |
   |----------|-----------|-------------|
   | Plasma protein binding (PPB) | % bound | Free fraction available for activity |
   | Volume of distribution (Vd) | L/kg | Extent of tissue distribution |
   | Blood-brain barrier (BBB) | binary + log BB | CNS penetration |

   **Metabolism:**
   | Endpoint | Unit/Scale | Description |
   |----------|-----------|-------------|
   | CYP1A2 substrate | binary | Metabolized by CYP1A2 |
   | CYP1A2 inhibitor | binary | Inhibits CYP1A2 |
   | CYP2C9 substrate | binary | Metabolized by CYP2C9 |
   | CYP2C9 inhibitor | binary | Inhibits CYP2C9 |
   | CYP2C19 substrate | binary | Metabolized by CYP2C19 |
   | CYP2C19 inhibitor | binary | Inhibits CYP2C19 |
   | CYP2D6 substrate | binary | Metabolized by CYP2D6 |
   | CYP2D6 inhibitor | binary | Inhibits CYP2D6 |
   | CYP3A4 substrate | binary | Metabolized by CYP3A4 |
   | CYP3A4 inhibitor | binary | Inhibits CYP3A4 |
   | Clearance (CL) | mL/min/kg | Total body clearance |
   | Half-life (t1/2) | hours | Elimination half-life |

   **Toxicity:**
   | Endpoint | Unit/Scale | Description |
   |----------|-----------|-------------|
   | hERG inhibition | binary + pIC50 | Cardiac ion channel liability |
   | AMES mutagenicity | binary | Bacterial reverse mutation test |
   | DILI | binary | Drug-induced liver injury risk |
   | Hepatotoxicity | binary | Liver toxicity (broader than DILI) |
   | Skin sensitization | binary | Allergic contact dermatitis risk |
   | LD50 | mg/kg (log scale) | Acute oral toxicity |
   | Carcinogenicity | binary | Long-term cancer risk |
   | Acute oral toxicity class | EPA class (I-IV) | GHS classification |

4. **Multi-model agreement check:**
   - For each endpoint that both admet_ai and ADMETlab 3.0 predict, compute agreement
   - Agreement = both models predict the same class (binary endpoints) or predictions are
     within 1 standard deviation (continuous endpoints)
   - Flag disagreements in the per-compound output with `agreement: false` and both
     model values

5. **MPO scoring with indication-aware presets:**
   - Multi-Parameter Optimization (MPO) score: weighted composite of ADMET endpoints
   - Presets:

   | Preset | Key Weights | Description |
   |--------|------------|-------------|
   | `oral` | HIA, Caco-2, %F, solubility, CYP panel, clearance | Standard oral drug profile |
   | `cns` | BBB, P-gp (penalize substrate), PPB, HIA, hERG | CNS-penetrant profile |
   | `oncology` | Relaxed oral, tolerate higher toxicity, emphasize potency/selectivity | Oncology-specific tolerances |
   | `antimicrobial` | Solubility, permeability, clearance, AMES (strict) | Antimicrobial profile |

   - Users can also define custom weights via the job spec
   - MPO score normalized to 0-100

6. **Per-compound ADMET output schema** (JSON, the WP-4/WP-7 integration contract):

   ```json
   {
     "compound_id": "KHM-BSYNRYMUTXBXSQ",
     "mpo_score": 72.5,
     "mpo_preset": "oral",
     "applicability_domain": {
       "in_domain": true,
       "distance": 0.23,
       "warning": null
     },
     "endpoints": {
       "solubility": {
         "value": -3.2,
         "unit": "log S",
         "confidence": 0.85,
         "category": "moderate",
         "models_agree": true
       },
       "caco2": { "..." },
       "cyp3a4_inhibitor": {
         "value": true,
         "confidence": 0.92,
         "models_agree": true
       }
     },
     "flags": {
       "herg_alert": false,
       "ames_positive": false,
       "dili_risk": false,
       "cyp_liability_count": 1,
       "low_bioavailability": false
     },
     "provenance": {
       "primary_model": "admet_ai v1.x",
       "secondary_model": "admetlab3 v3.x",
       "prediction_time": "2026-04-19T12:00:00Z",
       "job_name": "admet-job-abc123"
     }
   }
   ```

7. **Applicability-domain (AD) warnings:**
   - Compute Tanimoto distance from query compound to the nearest neighbor in the training
     set
   - If distance exceeds threshold (default 0.7): flag `in_domain: false` with warning
     message explaining that predictions may be unreliable
   - Display AD warnings prominently in the UI (WP-7 responsibility)

8. **ADMETJob CRD** (apiVersion: `khemeia.io/v1alpha1`, kind: `ADMETJob`)
   - Spec: `libraryRef` or `compoundIds` (array), `preset` (enum + custom),
     `customWeights` (map of endpoint to weight), `crossValidate` (boolean, default true),
     `batchSize`
   - Status: `phase`, `totalCompounds`, `predictedCompounds`, `outOfDomain`,
     `startTime`, `completionTime`, `provenance`

9. **API endpoints:**
   - `POST /api/v1/admet/predict` -- submit an ADMET prediction job
   - `GET /api/v1/admet/jobs/{name}` -- get ADMET job status
   - `GET /api/v1/admet/jobs/{name}/results` -- paginated compound ADMET results
   - `GET /api/v1/admet/jobs/{name}/results/{compoundId}` -- single compound ADMET detail
   - `GET /api/v1/admet/presets` -- list available MPO presets with their weights

10. **Tests:**
    - Unit tests for MPO score calculation (known inputs, expected score)
    - Unit tests for multi-model agreement logic
    - Unit tests for applicability domain distance calculation
    - Integration test: submit 10 known drug molecules (aspirin, ibuprofen, metformin, etc.),
      verify all endpoints return values within published ranges
    - E2E smoke test: submit ADMET job via API, poll until Succeeded, verify per-compound
      JSON matches output schema

## Acceptance Criteria

1. The admet_ai container starts, loads models, and responds to health checks within 90
   seconds. A SMILES query returns predictions for all listed endpoints within 5 seconds per
   compound.
2. For aspirin (`CC(=O)Oc1ccccc1C(=O)O`): solubility is predicted as moderate to high
   (log S > -4), HIA is predicted as high (>80%), and AMES is predicted as negative.
   These are known experimental values -- predictions must agree directionally.
3. The full CYP panel (1A2, 2C9, 2C19, 2D6, 3A4, each substrate + inhibitor) returns
   predictions for every compound. No endpoint is silently skipped.
4. Multi-model agreement: for endpoints covered by both admet_ai and ADMETlab 3.0, the
   `models_agree` field is populated and is `true` for >70% of a diverse 100-compound set.
5. MPO score with preset `oral` produces a value between 0 and 100 for every compound.
   Compounds with known good oral profiles (e.g., metformin) score higher than compounds
   with known poor oral profiles (e.g., cyclosporine).
6. Applicability domain: a synthetic SMILES far from drug-like space (e.g., a large
   inorganic coordination complex) triggers `in_domain: false`.
7. The per-compound ADMET JSON matches the schema defined in Deliverable 6. WP-7 can
   parse and render it without transformation.
8. The ADMETJob CRD status includes provenance with model versions and timestamps.

## Dependencies

| Relationship | WP | Detail |
|---|---|---|
| Blocked by | WP-2 | Consumes compound SMILES from the prepared library |
| Blocked by | WP-9 | CRD framework, event bus for job status, S3 for result storage |
| Integrates with | WP-7 | ADMET results displayed inline in Hit/Compound tab |
| Integrates with | WP-3 | ADMET runs on the same compound set as docking; combined in results view |
| Consumed by | WP-5 | Generative SAR uses ADMET scores to filter/rank generated compounds |
| Consumed by | WP-8 | Reporting includes ADMET profiles in compound reports |

## Out of Scope

- Experimental ADMET assay data ingestion (this WP is prediction-only)
- Custom model training or fine-tuning on proprietary data
- PK/PD modeling or dose-response prediction
- Metabolite identification or metabolic pathway prediction
- Formulation-dependent bioavailability prediction
- In vitro assay result management

## Open Questions

1. **admet_ai model size and GPU**: Does admet_ai require GPU for inference, or is CPU
   sufficient? The Chemprop-D models are relatively small, but batch inference on 10K+
   compounds may benefit from GPU.

2. **ADMETlab 3.0 availability**: ADMETlab 3.0 is primarily available as a web service. Is
   there a downloadable model for self-hosted deployment, or does this require API calls to
   an external service? If external, this introduces a network dependency and rate limits.

3. **Training set for applicability domain**: admet_ai's training set may not be publicly
   enumerable. How should we compute the AD distance? Options: use a reference set of known
   drugs (e.g., DrugBank approved), or use the Morgan fingerprint centroid of the training
   set if admet_ai exposes it.

4. **ADMET endpoint prioritization**: The full endpoint list is extensive. For the initial
   release, should all endpoints be mandatory, or can some be phased in (e.g., skin
   sensitization, carcinogenicity are less critical for early-stage SBDD)?

5. **MPO weight calibration**: The preset weights need calibration against real drug discovery
   campaigns. Should the initial weights come from published literature (e.g., Wager et al.
   2016 for CNS MPO) or be set empirically?

## Technical Constraints

- **Container serving**: admet_ai runs as a persistent Deployment (not a per-job Job) to
  avoid model loading overhead on every request. Deploy with 2 replicas for availability.
- **ADMETlab 3.0**: If only available as a web API, wrap in a sidecar that handles
  rate limiting, retries, and caching. Cache predictions by InChIKey to avoid redundant
  external calls.
- **Batch size**: For large libraries (10K+ compounds), the API should accept batch
  submissions. Process in chunks of 100-500 SMILES per request to the model service.
- **Existing flags**: The current `ADMETFlags` struct in `handlers_analysis.go` is computed
  from descriptors and is fast. Retain it as a "fast pre-screen" (no model inference needed)
  alongside the full ML-based predictions. Do not remove the existing flags.
- **Schema coordination**: The JSON schema in Deliverable 6 is the interface contract with
  WP-7. Changes to this schema require agreement from both WP-4 and WP-7 owners.
- **Memory**: Chemprop models can use 2-4 GB RAM. Size the container accordingly.
  ADMETlab 3.0 models (if self-hosted) have similar requirements.

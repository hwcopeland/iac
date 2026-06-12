# u4u → Khemeia Genomics Integration Guide (GEN-13)

> **Audience:** the **u4u-engine** maintainer (Noah / the Florida Man Bioscience team) wiring
> u4u's structural-biophysics step (`step 8i`) against Khemeia, and the Khemeia operator who
> supports that integration.
>
> **What this doc is:** the end-to-end **call set** — the exact HTTP calls, request bodies, and
> response shapes — that take **one missense variant** and return
> **{ΔΔG stability, pocket proximity, PGx drug-response}** via one documented flow. This is the
> US-6 acceptance criterion in the [genomics PRD](../../../../docs/prd/khemeia-genomics.md).
>
> **What this doc is NOT:** it does **not** cover how u4u obtains its API token. Token
> provisioning, custody, rotation, and the trust model live in
> [`u4u-api-token.md`](./u4u-api-token.md) (GEN-03). That doc covers **WHO / auth**; this doc
> covers the **call FLOW**. The authoritative REST/field contract is the
> [core genomics TDD](./tdd/khemeia-genomics-core.md) (§5.4, §6) — where this doc and the TDD
> disagree, the TDD wins.

> **Contract source note.** At the time of writing, `api/handlers_genome.go` (GEN-11) has not
> landed. Every request/response body below is transcribed from the **core TDD §5.4 / §6**, which
> is the frozen contract. Re-verify against the handler once it exists; if it diverges, the TDD
> is the source of truth and this doc gets corrected, not the other way around.

---

## 1. Overview

### u4u is an external HTTP consumer

u4u (`Florida-Man-Bioscience/u4u-engine`) is **not deployed in the cluster**. It is an ordinary
HTTP client that calls Khemeia to turn a genomic variant into quantitative biophysical scores,
then folds those scores into its own variant prioritization. Khemeia is the platform; u4u is only
a consumer. (See PRD §4 — "No u4u deployment in-cluster.")

### Trust / auth model (one Bearer token)

Every call below requires `Authorization: Bearer <token>`. u4u carries exactly **one** secret —
a single environment variable — and presents it as a bearer header on every request:

```bash
export KHEMEIA_API_TOKEN="<the 64-hex secret handed to you out-of-band>"
```

Because u4u reaches Khemeia through the ingress, its requests carry `X-Forwarded-For`, which
disables the internal-CIDR auth bypass — so the token is the **only** thing authorizing the call.
There is no OAuth flow, no token refresh, no client registration.

**Provisioning, custody, rotation, and the revoke kill-switch are out of scope here** — see
[`u4u-api-token.md`](./u4u-api-token.md). If a call returns `401`, the token is missing, expired,
or revoked; refer to that doc, do not retry blindly.

### Base URL

```
https://khemeia.hwcopeland.net
```

All genomics endpoints live under `/api/v1/genome/*`. A quick auth smoke test:

```bash
curl -sf -H "Authorization: Bearer ${KHEMEIA_API_TOKEN}" \
  https://khemeia.hwcopeland.net/health
# -> {"status":"healthy", ...}
```

### The async model in one sentence

Khemeia genomics is **submit → poll → fetch**: you `POST` a batch and get a `group_name` back
(`202`), you `GET` the job status until it reaches `Completed`, then you `GET` the results. There
is no synchronous "give me the answer now" call — this matches u4u's existing
`ThreadPoolExecutor` batch fan-out.

---

## 2. The variant → enrichment flow (end to end)

This section walks a single missense variant through the full call set with concrete `curl` and
JSON. The worked example is a **CYP2D6** missense substitution `p.Arg117His` — a pharmacogene, so
all three signals (ΔΔG stability, pocket proximity, PGx docking) are meaningful.

> **One submit covers the whole call set.** You do **not** issue one request per calculation. A
> single `submit` takes the **cartesian product** of `variants[] × calculations[]`, so one
> variant × three calculations = three calc-jobs under one `group_name`. `esmfold` / `resolve`
> are **not** in your `calculations[]` list — structure resolution (UniProt → AlphaFold DB, with
> an ESMFold fallback on an AlphaFold miss) happens **implicitly** as a controller-internal stage
> before any calc runs. You only ask for the biophysical signals you want.

### Step 1 — Submit the variant + calculation set

`POST /api/v1/genome/variant/submit` → `202 Accepted`

```bash
curl -s -X POST https://khemeia.hwcopeland.net/api/v1/genome/variant/submit \
  -H "Authorization: Bearer ${KHEMEIA_API_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{
    "variants": [
      { "id": "u4u-var-001", "mode": "protein_change", "gene": "CYP2D6",
        "transcript": "ENST00000360608", "protein_change": "p.Arg117His" }
    ],
    "calculations": ["ddg_stability", "pocket_proximity", "pgx_docking"],
    "params": {
      "ddg_stability": { "engine": "foldx" },
      "pgx_docking":   { "drugs": ["codeine"], "engines": ["gnina"] }
    },
    "priority": "normal"
  }'
```

Response (`202`):

```json
{
  "group_name": "genome-1749600000000000000",
  "status": "Pending",
  "variant_count": 1,
  "calc_count": 3,
  "accepted": ["u4u-var-001"],
  "rejected": []
}
```

- `group_name` is your handle for everything that follows — capture it.
- `calc_count` = `variant_count × len(calculations)` (1 × 3 = 3 here).
- Variants that can't be resolved at submit time land in `rejected[]` with a typed `E_*` code
  (see §4); they do **not** fail the whole batch. `accepted[]` is what actually queued.

The optional `callback_url` field is **accepted but not invoked** in this phase — poll is the
contract (TDD §7.4). Don't rely on a webhook.

### Step 2 — Poll for status

`GET /api/v1/genome/jobs/{group_name}` → `200`

```bash
curl -s https://khemeia.hwcopeland.net/api/v1/genome/jobs/genome-1749600000000000000 \
  -H "Authorization: Bearer ${KHEMEIA_API_TOKEN}"
```

While work is in flight (`Running`):

```json
{
  "group_name": "genome-1749600000000000000",
  "status": "Running",
  "calculations": ["ddg_stability", "pocket_proximity", "pgx_docking"],
  "totals": { "calc_jobs": 3, "completed": 1, "failed": 0, "skipped": 0 },
  "per_calc": [
    { "calculation": "ddg_stability",    "completed": 1, "total": 1 },
    { "calculation": "pocket_proximity", "completed": 0, "total": 1, "status": "Running" },
    { "calculation": "pgx_docking",      "completed": 0, "total": 1, "status": "Running" }
  ],
  "submitted_by": "u4u-engine",
  "created_at": "2026-06-12T00:00:00Z",
  "started_at": "2026-06-12T00:00:05Z",
  "completed_at": null
}
```

When every calc-job is terminal (`status: "Completed"`):

```json
{
  "group_name": "genome-1749600000000000000",
  "status": "Completed",
  "calculations": ["ddg_stability", "pocket_proximity", "pgx_docking"],
  "totals": { "calc_jobs": 3, "completed": 3, "failed": 0, "skipped": 0 },
  "per_calc": [
    { "calculation": "ddg_stability",    "completed": 1, "total": 1 },
    { "calculation": "pocket_proximity", "completed": 1, "total": 1 },
    { "calculation": "pgx_docking",      "completed": 1, "total": 1 }
  ],
  "submitted_by": "u4u-engine",
  "created_at": "2026-06-12T00:00:00Z",
  "started_at": "2026-06-12T00:00:05Z",
  "completed_at": "2026-06-12T00:18:42Z"
}
```

`status` is one of `Pending | Running | Completed | Failed`. Poll on a backoff (see §5 for
cadence) rather than tight-looping. `ddg_stability` and `pocket_proximity` typically finish well
before `pgx_docking`, which composes the most work (docking WT and mutant) — you can pull partial
results as they land (next step) instead of waiting for the whole group.

### Step 3 — Fetch results

`GET /api/v1/genome/jobs/{group_name}/results` → `200` (paginated)

```bash
curl -s "https://khemeia.hwcopeland.net/api/v1/genome/jobs/genome-1749600000000000000/results?page=1&per_page=100" \
  -H "Authorization: Bearer ${KHEMEIA_API_TOKEN}"
```

Optionally filter to one calc with `&calculation=ddg_stability`. The envelope:

```json
{
  "group_name": "genome-1749600000000000000",
  "total_results": 3,
  "page": 1,
  "per_page": 100,
  "complete": true,
  "results": [ /* one object per (variant × calc); shapes below */ ],
  "failures": []
}
```

`complete` is `true` only when all calc-jobs are terminal. Any per-(variant,calc) failure appears
in `failures[]` with a typed `E_*` code (see §4) — never as a silent drop.

Each entry in `results[]` shares a **common envelope** (TDD §6) and a per-calc `payload`:

```json
{
  "variant_key": "u4u-var-001",
  "resolution_id": "rv-9f3c1a72b0e45d18",
  "calculation": "ddg_stability",
  "structure_source": "alphafold",
  "uniprot_acc": "P10635",
  "residue_index": 117,
  "wild_type_aa": "R",
  "mutant_aa": "H",
  "confidence": 0.81,
  "status": "Completed",
  "payload": { "...": "per-calc, below" },
  "artifact_keys": { "...": "S3 keys for large artifacts" }
}
```

The four typed **headline fields** u4u folds into prioritization are surfaced both in the per-calc
`payload` and (per TDD §5.3) as typed result columns. The three signals for this variant:

**`ddg_stability`** — destabilization of the fold:

```json
{
  "engine": "foldx",
  "ddg_fold_kcal_mol": 2.7,
  "stability_class": "destabilizing",
  "per_term": { "vdw": 0.9, "electrostatic": 0.4, "solvation": 1.1, "hbond": 0.3 },
  "n_runs": 5,
  "stdev_kcal_mol": 0.4,
  "report_key": "ddg_stability/rv-9f3c1a72b0e45d18/foldx_report.json",
  "interpretation": "Destabilizing (ddG=2.7 kcal/mol) — likely disrupts fold"
}
```
→ headline field: **`ddg_fold_kcal_mol`** (positive = destabilizing).

**`pocket_proximity`** — adjacency to a druggable pocket / functional site:

```json
{
  "detector": "p2rank",
  "nearest_pocket": { "rank": 1, "druggability_score": 0.74,
                      "distance_ang": 4.2, "pocket_residues": ["GLU117","PHE208","ASP150"] },
  "within_cutoff": true,
  "in_pocket": true,
  "n_pockets": 3,
  "pockets_key": "pocket_proximity/rv-9f3c1a72b0e45d18/pockets.json",
  "interpretation": "Mutated residue lines the top druggable pocket (4.2 A) — likely functional"
}
```
→ headline fields: **`pocket_proximity_flag`** (= `within_cutoff`) and **`pocket_distance_ang`**.

**`pgx_docking`** — WT-vs-mutant drug-response shift:

```json
{
  "per_drug": [{
    "drug": "codeine",
    "wt_affinity_kcal_mol": -7.8,
    "mut_affinity_kcal_mol": -6.1,
    "ddg_bind_kcal_mol": 1.7,
    "fingerprint_delta": {
      "tanimoto": 0.62,
      "lost_contacts":   ["HBOND:ASP150","HYDROPHOBIC:PHE208"],
      "gained_contacts": ["HBOND:GLU117"]
    },
    "engine": "gnina",
    "wt_pose_key":  "pgx_docking/rv-9f3c1a72b0e45d18/codeine/wt_pose.pdbqt",
    "mut_pose_key": "pgx_docking/rv-9f3c1a72b0e45d18/codeine/mut_pose.pdbqt"
  }],
  "interpretation": "Mutant reduces codeine binding by 1.7 kcal/mol with loss of 2 key contacts"
}
```
→ headline fields: **`ddg_bind_kcal_mol`** (max `|ddg_bind|` across drugs, `mut − wt`; positive =
mutant binds weaker) and **`fp_delta_tanimoto`** (min tanimoto across drugs; `1.0` = identical
interaction profile).

### How u4u folds these in (additive, never overwriting ClinVar)

u4u's annotation-only scoring parks a missense VUS at roughly **50–120** (VUS `+50`, missense
`+50`, ultra-rare `+20`) with no way to separate a benign substitution from a destabilizing or
binding-altering one. u4u plugs Khemeia in as **step 8i** and adds the four headline numbers to
its existing `reasons` / `score` as **bounded, additive evidence** — exactly the discipline u4u
already applies to `frequency_derived_label`:

- The signals **add weight**; they **never overwrite a ClinVar verdict**. A ClinVar `pathogenic`
  short-circuit (score 1000) or `benign` (score 1) stays authoritative regardless of biophysics.
- The exact weighting is **u4u's decision**. Khemeia only guarantees the signal contract; it does
  not tell u4u how much each number is worth.
- Khemeia returns biophysical **numbers**, not clinical calls. Tiering, VUS policy, and human
  sign-out stay in u4u (PRD §4 — "No clinical interpretation").

### Step 4 (optional) — Fetch the predicted structure

`GET /api/v1/genome/variant/{resolution_id}/structure` → presigned S3 URL (`302` or JSON)

When u4u needs the actual resolved structure (e.g. to render it or run its own analysis), fetch it
by the `resolution_id` from any result envelope:

```bash
curl -s https://khemeia.hwcopeland.net/api/v1/genome/variant/rv-9f3c1a72b0e45d18/structure \
  -H "Authorization: Bearer ${KHEMEIA_API_TOKEN}"
```

This returns a presigned Garage S3 URL for the resolved WT structure (`.pdb`). The
`structure_source` field on the result (`alphafold | esmfold | pdb`) tells you whether the
structure was experimental/AlphaFold or an ESMFold fallback — weigh confidence accordingly
(an ESMFold model carries `plddt`; treat low-pLDDT structures with caution).

---

## 3. Interpreting each signal

Each number answers a different question and unblocks a different action for u4u. All three are
**tiebreakers / evidence**, not verdicts.

| Signal | Headline field(s) | What the number means | What it unblocks for u4u |
|---|---|---|---|
| **ΔΔG stability** | `ddg_fold_kcal_mol` | Change in folding free energy for the substitution. **Positive = destabilizing**; larger = more disruptive. `stability_class` buckets it (`stabilizing` / `neutral` / `destabilizing`). | **Break VUS-missense ties.** A strongly destabilizing ΔΔG (e.g. `> 2.0`) promotes a missense VUS out of the undifferentiated MEDIUM pile with a mechanistic rationale; a near-zero ΔΔG argues benign-tolerated. |
| **Pocket proximity** | `pocket_proximity_flag`, `pocket_distance_ang` | Whether the mutated residue sits at/near a druggable pocket, catalytic site, or interface, and how far (Å) to the nearest. `in_pocket=true` means the residue lines the pocket. | **Flag mechanistically plausible LoF/GoF.** A variant *in* or adjacent to a functional pocket is a plausible loss/gain-of-function candidate that annotation alone would miss — raises priority for human review. |
| **PGx docking** | `ddg_bind_kcal_mol`, `fp_delta_tanimoto` | Change in drug binding (WT vs mutant): `ddg_bind` is the affinity shift (positive = mutant binds weaker); `fp_delta_tanimoto` is how much the interaction fingerprint changed (lower = bigger change). | **Quantify PGx drug-response.** For pharmacogene variants, turns "this variant is in CYP2D6" into a quantitative drug-response signal (e.g. "reduces codeine binding by 1.7 kcal/mol, loses 2 key contacts") — actionable for dosing review. |

`confidence` (0..1) on each result and `structure_source` together tell u4u how much to trust a
given signal — downweight low-confidence ESMFold-derived results.

---

## 4. Error handling

Errors are **typed `E_*` codes**, never silent drops (TDD §8, Appendix A). Where they appear:

- At **submit**, an unresolvable variant comes back in the `202` body's `rejected[]` — the rest of
  the batch still queues.
- At **results**, a per-(variant,calc) failure comes back in the results `failures[]` array.

```json
"rejected": [ { "id": "u4u-var-002", "error": "E_VARIANT_NOT_MISSENSE" } ]
```
```json
"failures": [ { "variant_key": "u4u-var-002", "calculation": "pgx_docking",
                "error": "E_NO_LIGAND_STRUCTURE" } ]
```

| Code | Meaning | What u4u should do |
|---|---|---|
| `E_VARIANT_NOT_MISSENSE` | The variant isn't a single-residue missense substitution (e.g. rsID resolves to non-coding or multi-gene). These four calcs are **missense-scoped by design.** | **Skip — do not retry.** Proceed with annotation-only scoring for that variant; this is expected, not an outage. |
| `E_WT_MISMATCH` | The submitted wild-type residue disagrees with the UniProt canonical sequence at that position (transcript/build drift). | **Skip and report.** A retry with the same input fails identically. Surface it for review — likely a transcript/build mismatch in the input. Pin the transcript or correct the protein change, then resubmit. |
| `E_RESOLVE_UPSTREAM` | An external dependency (UniProt / AlphaFold DB / ESMFold weights) was unreachable. Transient. | **Retry later** with backoff. The in-cluster ESMFold fallback and resolution cache mean this should be rare and self-clearing. Proceed without the signal if it persists. |
| `E_UNIPROT_NOT_FOUND` | The gene couldn't be mapped to a UniProt accession. | **Skip and report.** Not retryable as-is. Check the gene symbol (HGNC) or pass an explicit `uniprotAcc` override on the variant, then resubmit. |
| `E_PARAMS_INVALID` | Per-calc `params` failed validation at submit (bad engine name, malformed drug list, etc.). | **Fix and resubmit.** A client-side bug — correct the `params` block; do not retry unchanged. |

Two additional calc-specific codes you may see in `failures[]`: `E_NO_LIGAND_STRUCTURE`
(PGx — a requested drug couldn't be resolved to a ligand structure; drop that drug or fix the
name) and `E_STRUCTURE_FOLD_FAILED` (ESMFold could not produce a usable model; proceed without the
structural signal for that variant).

**Golden rule:** one failing calc never blocks the others. If `pgx_docking` fails but
`ddg_stability` and `pocket_proximity` succeed, u4u still gets two of three signals — fold in what
returned (PRD non-functional req: "one calculation failing does not block the others").

---

## 5. Operational notes

### Async / polling cadence

The model is async submit → poll → fetch; there is no synchronous result. Suggested cadence:
poll `GET /jobs/{group_name}` on a **backoff** — e.g. every ~5 s for the first minute, then every
15–30 s. `ddg_stability` and `pocket_proximity` are quick (minutes); `pgx_docking` is the long
pole (GPU docking of WT and mutant, up to a 24 h timeout for large jobs). Pull results
incrementally with the `&calculation=` filter as each calc completes rather than blocking on the
whole group.

### Batch sizing

`submit` is batch-native — send many variants in one request and poll one `group_name`. This maps
cleanly onto u4u's existing `ThreadPoolExecutor(max_workers=8)` fan-out (TDD Decision C1). Prefer
**one batch per genome's VUS list** over per-variant calls (which would be hundreds of chatty
requests). For `pgx_docking`, always pass an **explicit `drugs[]`** — there is no implicit
full-panel docking, to keep GPU cost bounded.

### Idempotency / caching (re-submits are cheap)

Resolutions and results are **content-addressed**: the `resolution_id` is a deterministic hash of
the canonical residue substitution (UniProt acc + isoform + residue index + WT>mut + structure
source). Two different inputs that land on the same substitution — e.g. an rsID and a
`protein_change` for the same residue — **share a resolution and structure**. Re-submitting the
same variant×calc reuses the cached resolution and (where the result is cached) the prior result,
so **re-running a genome is fast and cheap**. You don't need to dedup on your side; submit and let
the cache absorb repeats.

### Open item to confirm with Noah

- [ ] **Single API-key env var.** This guide assumes u4u carries exactly **one** secret —
  `KHEMEIA_API_TOKEN` — set once and sent as `Authorization: Bearer`. Confirm that matches u4u's
  fail-open, zero-config posture before the token is issued. If u4u wants separate staging vs prod
  tokens, the operator provisions one labeled row each so rotation/revoke are independent. (This
  is the same open item tracked in [`u4u-api-token.md`](./u4u-api-token.md) §7.)

---

## See also

- [`u4u-api-token.md`](./u4u-api-token.md) — token provisioning, custody, rotation, revoke (auth / WHO).
- [core genomics TDD](./tdd/khemeia-genomics-core.md) — authoritative REST/field contract (§5.4) and per-calc result contracts (§6).
- [genomics PRD](../../../../docs/prd/khemeia-genomics.md) — US-6 and the per-calc "what u4u can DO with each result" framing.

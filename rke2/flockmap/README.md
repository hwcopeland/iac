# flockmap

flockmap is a **deterministic, ML-free** data pipeline that attributes a
**funding source** to **Flock Safety ALPR cameras** with **court-grade
provenance**. It answers: *which government money paid for this license-plate
reader, and where is the receipt?*

Every datum on the ledger traces back to a stored raw byte copy (with a SHA-256)
of the exact API response or document it came from. No machine-learning model
ever reads, transforms, or emits a dollar figure, a camera record, or a join
that lands on the ledger. The map is a *floor, not a census*: OSM is
crowdsourced, so coverage figures are a lower bound, never "% of all Flock
cameras."

---

## The firewall (non-negotiable)

See `CONVENTIONS.md §7` for the binding text. In short:

1. **No ML on the data path.** Every ledger datum is produced by deterministic
   code: HTTP fetch + parse, SQL, PostGIS point-in-polygon, fixed-threshold
   string match. No LLM/embedding/learned weight ever touches a number that
   enters the ledger.
2. **Provenance is mandatory.** Every data row carries `source_doc_id` →
   `raw_documents` (verbatim bytes + sha256). A row without it is a breach.
   Connectors call `store.SaveRawDocument` **first**, then thread the returned
   id into every row they write.
3. **`funding_records.extraction`** is one of `deterministic` /
   `human_verified` / `ml_unreviewed`. `ml_unreviewed` **requires** a non-nil
   `confidence` and is **excluded from headline coverage** until a human flips
   it to `human_verified`.
4. **`source_registry` has no money column** — discovery agents emit only
   metadata about *where* data lives. A money-bearing discovery row is a breach.
5. **Match confidence is a fixed lookup, never learned:**
   `exact_agency_keyword` 0.95 / `agency_only` 0.70 / `jurisdiction_keyword`
   0.55 / `jurisdiction_only` 0.30. Headline match threshold is **≥ 0.70**.
6. **ODbL:** OSM-derived `raw_documents` rows record `license` + `attribution`.

The data path is enforced structurally: `internal/store` ships only
`ConnectPostgres` / `EnsureSchema` / `NewUUIDv7` / `SaveRawDocument`, and the
schema makes `source_doc_id` `NOT NULL` on every data table.

---

## Components present (P0–P4)

The pipeline is a chain of `cmd/<name>` stages, all sharing one Go module
(`github.com/hwcopeland/flockmap`) and the frozen foundation (`go.mod`,
`internal/store`, `db/`). Each stage is built into its own image and deployed as
a Job or CronJob.

| Stage | `cmd/` dir | Plan | Image | Workload | What it does |
|---|---|---|---|---|---|
| Camera spine | `camera-spine` | P0 | `flockmap/camera-spine` | weekly CronJob (Mon 04:00) | Pulls the Flock ALPR spine from the OSM **Overpass** API (3 queries: canonical manufacturer tag, manufacturer wikidata `Q108485435`, bad-tag sweep). Stores each verbatim response (channel `overpass`, ODbL), upserts `cameras` `ON CONFLICT (osm_type, osm_id)`. |
| Geocode | `geocode` | P1 | `flockmap/geocode` (a.k.a. `flockmap/tiger-load`) | one-shot Job (`tiger-load`) | Downloads U.S. Census **TIGER/Line** shapefiles (state/county/county-subdivision/place), stores each zip verbatim (channel `census_tiger`, public domain), loads via `ogr2ogr` into staging, promotes into `jurisdictions`, then runs a deterministic PostGIS `ST_Contains` point-in-polygon pass stamping every camera's `state_fips` / `county_geoid` / `place_geoid`. |
| Agency resolve | `agency-resolve` | P2 | `flockmap/agency-resolve` | one-shot Job | Resolves `cameras.agency_id` by a strict priority ladder: (1) `operator` tag → agency + alias; (2) crosswalk: place GEOID → municipal PD / unincorporated county GEOID → sheriff (LEAIC-seeded `agencies`); (3) **state-scoped** fixed-threshold (0.88) token-sort + Levenshtein fuzzy match; (4) unresolved. Confidence is a fixed lookup. |
| Funding (federal) | `funding-federal` | P3 | `flockmap/funding-federal` | weekly CronJob (Mon 05:00) | Sweeps **USAspending** `spending_by_award` (grants + contracts) across fixed ALPR keywords, stores each page verbatim (channel `usaspending`), upserts `funding_records` (`extraction='deterministic'`) `ON CONFLICT (channel, external_id)`. Also probes **grants.gov** for ALN/CFDA discovery (channel `grants_gov`, raw stored, **no money rows minted**). `cameras_funded_count` is extracted by one fixed regex or left NULL. |
| Match | `match` | P4/P5 | `flockmap/match` | one-shot Job | A single deterministic SQL pass populating `camera_funding_matches` by joining `cameras` ↔ `funding_records`, strongest-basis-first, with the fixed confidence lookup. Refreshes coverage views (no-op for the current plain VIEWs). Performs **no external fetch** and writes no new evidence. |

Foundation (frozen — do not edit): `db/schema.sql`, `db/views.sql`,
`internal/store/store.go` (+ `schema_embed/` build-synced copies), `go.mod`,
`go.sum`. Connectors own only their `cmd/<name>/` package and the shared
`deploy/` manifests.

### Coverage views (`db/views.sql`)

- `camera_funding` — flattened camera → match → funding for the map UI, with
  `is_trusted` (extraction ∈ deterministic/human_verified).
- `coverage_by_channel` — distinct cameras with a **trusted** funder at ≥ 0.70,
  by funding channel.
- `jurisdiction_funding_gap` — the FOIA-queue engine:
  `cameras_observed − cameras_accounted`, ranked by gap descending.

---

## The spine denominator (~79,201)

The verified live OSM/Overpass spine pull returns roughly **79,201** Flock ALPR
cameras (the canonical-tag query alone yields ~79k; the wikidata query ~78k;
they overlap and dedupe on `(osm_type, osm_id)`). This is the **denominator**
every coverage percentage is computed against — and it is a **floor**, not a
census: OSM is crowdsourced, so the true camera count is higher. Never present a
coverage figure as "% of all Flock cameras"; always caveat that it is "% of the
~79,201 OSM-observed spine."

---

## Build

Images are built on the self-hosted **`arc-chem`** ARC runner (on the cluster
LAN, so it can reach the private `zot.hwcopeland.net` registry) and pushed to
`zot.hwcopeland.net/flockmap/<name>`. **Never build on a Mac.**

CI: `.github/workflows/build-flockmap.yml` (triggered on `rke2/flockmap/**`
changes). A matrix builds all five stages with `provenance:false sbom:false` for
zot compatibility, tagging each `sha-<7>` + `latest`:

- The four pure-Go stages (`camera-spine`, `agency-resolve`, `funding-federal`,
  `match`) build from the **shared** `Dockerfile`, parameterised by the
  `CMD=<name>` build-arg (distroless static, nonroot).
- **`geocode`** is the exception: it shells out to `ogr2ogr` (GDAL) at runtime,
  so it builds from `cmd/geocode/Dockerfile` (a GDAL runtime image) and is also
  published under the **`tiger-load`** image name, which the TIGER load Job pulls.

The `deploy` job in the workflow does a `rollout restart` for any stage deployed
as a `Deployment` (none today — all stages are Jobs/CronJobs — so it is a no-op
that skips absent workloads).

Local `go build ./cmd/...` produces bare binaries at the module root; these are
`.gitignore`d and must never be committed.

---

## Deploy

GitOps via **Flux**. `deploy/flux-kustomization.yaml` points the cluster at
`./rke2/flockmap/deploy` (source: the shared `tooling` GitRepository, branch
`main`). `deploy/kustomization.yaml` reconciles:

- `namespace.yaml` — the `flockmap` namespace.
- `zot-pull-secret.yaml` — Bitwarden-backed ExternalSecret for registry pulls.
- `postgres-secret.yaml` — Bitwarden-backed ExternalSecret (`username=flockmap`,
  password from a **new** Bitwarden item — see punch-list).
- `postgres.yaml` — `postgis/postgis:16-3.4` Deployment + 20Gi Longhorn PVC +
  ClusterIP Service `flockmap-postgres`.
- `camera-spine-cronjob.yaml`, `funding-federal-cronjob.yaml` — weekly ingests.
- `tiger-load-job.yaml`, `agency-resolve-job.yaml`, `match-job.yaml` — one-shot
  pipeline Jobs (Job pod templates are immutable; re-run by `kubectl delete job`
  then letting Flux reconcile).

All workloads connect to Postgres via `POSTGRES_*` env (host
`flockmap-postgres.flockmap.svc.cluster.local`, db `flockmap`, user/password
from `flockmap-postgres-secret`) and pull with `imagePullSecrets:
[zot-pull-secret]`.

**First-run order:** `tiger-load` (P1, needs cameras to assign but loads
jurisdictions regardless) → after `camera-spine` has populated cameras, run the
geocode assignment → `agency-resolve` (P2) → `funding-federal` (P3) → `match`
(P4). The CronJobs (`camera-spine`, `funding-federal`) refresh weekly thereafter.

---

## What is stubbed / deferred (later phases)

- **Garage S3 blob storage.** `raw_documents.raw_bytes` is stored **inline** as
  `bytea` today. The `flockmap-raw` / `flockmap-foia` Garage buckets and the
  `storage_uri` migration (`store.SaveRawDocument` TODO) are not wired yet.
- **LEAIC crosswalk load.** `agency-resolve` *reads* the `agencies` / `aliases`
  crosswalk but does not load it. The DOJ/BJS LEAIC (ICPSR study 35158) loader
  (a future `cmd/crosswalk-load` or a `psql \copy` runbook) is not built — see
  `cmd/agency-resolve/crosswalk.go`. Until it runs, the crosswalk path resolves
  nothing and resolution falls back to operator-tag + fuzzy only.
- **TIGER vintage** is pinned to **2023** (`TIGER_YEAR`, overridable). No
  vintage-selection / refresh automation.
- **Census Geocoder / FCC Area API** are documented **spot-check only** tools
  (`cmd/geocode/sql/SPOTCHECK.md`); they are deliberately **off the hot path**.
- **Map UI** and the **FOIA** drafting/tracking workflow (P6/P8 — `foia_requests`
  table and `source_registry` exist in the schema, fed by
  `jurisdiction_funding_gap`) are later phases; no connector writes them yet.
- **`funding_records.agency_id` / `jurisdiction_id`** are left NULL by
  `funding-federal`; wiring federal awards to agencies/jurisdictions (so the
  agency-join match basis fires) is a downstream task.

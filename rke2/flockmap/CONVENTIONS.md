# flockmap — The Contract

This file is the binding contract for every agent building a flockmap component.
Read it before writing any code. The foundation (`go.mod`, `internal/store/`,
`db/`) is **already built and frozen** — you consume it, you do not change it.

flockmap is a **deterministic, ML-free** data pipeline that attributes a funding
source to Flock ALPR cameras with court-grade provenance. See the plan at
`~/.claude/plans/fan-out-a-team-fancy-parasol.md`.

---

## 1. Module & layout

- **Go module path:** `github.com/hwcopeland/flockmap`
- **Go version:** 1.22 (`go.mod` pins it; do not bump)
- **Shared store import:** `github.com/hwcopeland/flockmap/internal/store`
- **Only dependency pre-declared:** `github.com/lib/pq`. It is already in
  `go.mod`. If you need it, just import it — **do not run `go get` or edit
  `go.mod`.** If you genuinely need another dependency, STOP and ask the
  foundation owner; do not add it yourself.

```
flockmap/
├── go.mod                      # FROZEN — never edit
├── go.sum                      # FROZEN — never edit
├── CONVENTIONS.md              # this file
├── db/
│   ├── schema.sql              # FROZEN — all table DDL (idempotent)
│   └── views.sql               # FROZEN — coverage views
├── internal/
│   └── store/                  # FROZEN — shared persistence layer
│       ├── store.go
│       └── schema_embed/       # build-synced copies of db/*.sql (go:embed)
├── cmd/
│   └── <your-component>/       # ← YOU build here, and only here
│       ├── main.go
│       └── *.go                # your upsert helpers live here
└── deploy/                     # ← your K8s manifests for your component
```

## 2. The ownership rule (prevents merge conflicts)

> **Only add files under your own `cmd/<name>/` and `deploy/`. Never edit
> `go.mod`, `go.sum`, `internal/`, or `db/`.**

If the shared layer is missing something you need, do not work around it by
forking — request the change from the foundation owner so all components stay in
sync. The store ships intentionally minimal: you write your own upsert helpers
in your own `cmd/<name>/` package (signatures listed in §5).

## 3. The store package — exact functions you call

Import: `import "github.com/hwcopeland/flockmap/internal/store"`

```go
// Open the pooled Postgres connection (sslmode=disable, in-cluster).
func store.ConnectPostgres() (*sql.DB, error)

// Run db/schema.sql then db/views.sql (embedded, idempotent). Call once at boot.
func store.EnsureSchema(db *sql.DB) error

// Time-ordered UUIDv7 — use for EVERY primary key you insert.
func store.NewUUIDv7() string

// Store the verbatim evidence bytes, dedupe on sha256, return raw_documents.id.
// CALL THIS FIRST, then thread the returned id into your rows as source_doc_id.
func store.SaveRawDocument(db *sql.DB, doc store.RawDocument) (id string, err error)

type store.RawDocument struct {
    SourceURL    string // fetched URL/endpoint
    HTTPMethod   string // "GET"/"POST"; defaults "GET"
    RequestBody  string // e.g. Overpass 'data=...' body; "" if none
    ContentType  string // response Content-Type; "" if unknown
    RawBytes     []byte // verbatim response bytes (the evidence)
    Channel      string // "overpass" | "usaspending" | "foia" | ...
    FetchedByJob string // your ingest-job identity (allowlist)
    License      string // optional, e.g. "ODbL" for OSM data
    Attribution  string // optional ODbL attribution string
}
```

`SaveRawDocument` computes the sha256, byte size, and id for you — do not set
them. It is idempotent: re-saving identical bytes returns the existing id.

## 4. Environment variables (set these in your `deploy/` manifest)

The store reads Postgres config from the environment:

| Var | Required | Default | Value in-cluster |
|---|---|---|---|
| `POSTGRES_HOST` | yes | — | `flockmap-postgres.flockmap.svc.cluster.local` |
| `POSTGRES_PORT` | no | `5432` | `5432` |
| `POSTGRES_DB` | no | `flockmap` | `flockmap` |
| `POSTGRES_USER` | yes | — | from `flockmap-postgres-secret` key `username` |
| `POSTGRES_PASSWORD` | yes | — | from `flockmap-postgres-secret` key `password` |

Inject `POSTGRES_USER` / `POSTGRES_PASSWORD` via `secretKeyRef` from the
`flockmap-postgres-secret` ExternalSecret (Bitwarden-backed). Mirror the
khemeia pattern.

## 5. Upsert helpers YOU implement (in your own cmd/<name>/ files)

The store does not ship these — write the one(s) your component needs, in your
own package. **Every helper must accept a non-empty `sourceDocID` (from
`SaveRawDocument`) and persist it as `source_doc_id`.** Generate PKs with
`store.NewUUIDv7()`, use `$N` placeholders, lean on the schema's `ON CONFLICT`
keys for idempotency.

```go
// camera-spine (P0)
UpsertCamera(db, c Camera, sourceDocID string) (id string, err error)            // ON CONFLICT (osm_type, osm_id)

// geocode (P1)
UpsertJurisdiction(db, j Jurisdiction, sourceDocID string) (id string, err error) // ON CONFLICT (geoid, level)
SetCameraGeoids(db, cameraID, placeGeoid, countyGeoid, stateFips string) error

// agency-resolve (P2)
UpsertAgency(db, a Agency, sourceDocID string) (id string, err error)
UpsertAlias(db, alias, agencyID, sourceDocID string) (id string, err error)
SetCameraAgency(db, cameraID, agencyID string, confidence float64) error

// funding connectors (P3/P7)
UpsertFundingRecord(db, f FundingRecord, sourceDocID string) (id string, err error) // ON CONFLICT (channel, external_id)

// match (P5)
UpsertCameraFundingMatch(db, cameraID, fundingRecordID, matchBasis string, confidence float64) (id string, err error)

// FOIA / discovery (P6/P8)
UpsertSourceRegistry(db, r SourceRegistryRow, sourceDocID string) (id string, err error) // NEVER a money field
UpsertFOIARequest(db, fr FOIARequest, sourceDocID string) (id string, err error)
```

## 6. Image naming & build

- **Images:** `zot.hwcopeland.net/flockmap/<cmd>` (e.g. `.../flockmap/camera-spine`).
- **Build on the self-hosted `arc-chem` ARC runner** (mirror `build-blog.yml` /
  `compute-infrastructure/.github/workflows/build-and-push.yml`). Use
  `provenance: false, sbom: false` for zot compatibility.
- **No Mac builds.** Ever. (Docker-on-Mac filled the disk before.)
- **Namespace:** `flockmap`. **`imagePullSecrets: [{name: zot-pull-secret}]`**.
- Secrets via **External Secrets → Bitwarden** (new item per secret).

## 7. THE FIREWALL — non-negotiable

1. **No ML on the data path.** No LLM may read, transcribe, transform, or emit a
   dollar figure, camera record, or join that enters the ledger. Every ledger
   datum is produced by deterministic code (HTTP fetch + parse, SQL, PostGIS
   point-in-polygon, fixed-threshold string match).
2. **Provenance is mandatory.** Every data row carries `source_doc_id` →
   `raw_documents` (raw byte copy + sha256). A row without it is a breach.
3. **`funding_records.extraction` is mandatory** and is one of:
   - `deterministic` — API/tabular parse; trusted immediately; `confidence` NULL.
   - `human_verified` — human spot-checked; trusted.
   - `ml_unreviewed` — ML-extracted; **REQUIRES a non-nil `confidence`**;
     provisional; **excluded from headline coverage** until a human flips it to
     `human_verified`.
4. **`source_registry` has NO money column.** Discovery agents emit only
   *metadata about where data lives*. A money-bearing discovery row is a breach.
5. **Match confidence is a fixed lookup**, never learned:
   `exact_agency_keyword` 0.95 / `agency_only` 0.70 / `jurisdiction_keyword`
   0.55 / `jurisdiction_only` 0.30. Headline match threshold is `>= 0.70`.
6. **ODbL:** OSM-derived data is ODbL — record `license`/`attribution` on the
   `raw_documents` row.

Audit invariant: any `funding_records` row lacking a raw copy, off the
ingest-job allowlist, or with no `extraction` provenance is a **firewall
breach**.

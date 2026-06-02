-- flockmap schema — Postgres 16 + PostGIS, provenance-first.
--
-- Every data table carries source_doc_id -> raw_documents(id): the court-grade
-- provenance link to a stored raw byte copy + sha256. This DDL is idempotent
-- (IF NOT EXISTS); EnsureSchema() runs it on every boot.
--
-- THE FIREWALL (see plan §0): no ML on the data path. funding_records.extraction
-- records HOW each number was produced (deterministic | human_verified |
-- ml_unreviewed). Headline coverage counts only deterministic + human_verified.
-- source_registry has NO money column by design — a money-bearing discovery row
-- is a firewall breach.

CREATE EXTENSION IF NOT EXISTS postgis;

-- ---------------------------------------------------------------------------
-- raw_documents: the evidence vault. Every datum on the ledger traces here.
-- sha256 is UNIQUE for dedupe-on-content. raw_bytes is stored inline as bytea
-- for now; TODO migrate to Garage S3 and populate storage_uri (see store.go).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS raw_documents (
    id              UUID PRIMARY KEY,
    source_url      TEXT NOT NULL,
    http_method     TEXT NOT NULL DEFAULT 'GET',
    request_body    TEXT NULL,                 -- e.g. Overpass 'data=' POST body
    content_type    TEXT NULL,
    checksum_sha256 CHAR(64) NOT NULL UNIQUE,  -- dedupe on content
    raw_bytes       BYTEA NULL,                -- inline copy (TODO -> Garage S3)
    storage_uri     TEXT NULL,                 -- s3://flockmap-raw/... once migrated
    byte_size       BIGINT NULL,
    channel         TEXT NOT NULL,             -- 'overpass' | 'usaspending' | 'foia' | ...
    license         TEXT NULL,                 -- e.g. 'ODbL' for OSM-derived
    attribution     TEXT NULL,                 -- ODbL attribution string
    fetched_by_job  TEXT NOT NULL,             -- ingest-job allowlist identity
    retrieved_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_raw_documents_channel ON raw_documents (channel);
CREATE INDEX IF NOT EXISTS idx_raw_documents_retrieved_at ON raw_documents (retrieved_at);
CREATE INDEX IF NOT EXISTS idx_raw_documents_fetched_by_job ON raw_documents (fetched_by_job);

-- ---------------------------------------------------------------------------
-- jurisdictions: TIGER geographies (places, county subdivisions, counties).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS jurisdictions (
    id            UUID PRIMARY KEY,
    geoid         TEXT NOT NULL,               -- Census GEOID
    level         TEXT NOT NULL CHECK (level IN ('place','county_subdivision','county','state')),
    name          TEXT NOT NULL,
    state_fips    CHAR(2) NULL,
    geom          geometry(MultiPolygon, 4326) NOT NULL,
    source_doc_id UUID NOT NULL REFERENCES raw_documents(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (geoid, level)
);
CREATE INDEX IF NOT EXISTS idx_jurisdictions_geom ON jurisdictions USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_jurisdictions_state_fips ON jurisdictions (state_fips);

-- ---------------------------------------------------------------------------
-- agencies: the entity that operates / is funded for cameras.
-- type includes governmental + private_hoa / business buckets.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agencies (
    id              UUID PRIMARY KEY,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN (
                        'police','sheriff','state_police','other_government',
                        'private_hoa','business','unknown')),
    leaic_ori       TEXT NULL,                 -- DOJ/BJS LEAIC ORI code
    state_fips      CHAR(2) NULL,
    jurisdiction_id UUID NULL REFERENCES jurisdictions(id),
    source_doc_id   UUID NOT NULL REFERENCES raw_documents(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agencies_leaic_ori ON agencies (leaic_ori);
CREATE INDEX IF NOT EXISTS idx_agencies_state_fips ON agencies (state_fips);
CREATE INDEX IF NOT EXISTS idx_agencies_jurisdiction ON agencies (jurisdiction_id);

-- ---------------------------------------------------------------------------
-- aliases: static, human-curated name crosswalk feeding agency resolution.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS aliases (
    id            UUID PRIMARY KEY,
    alias         TEXT NOT NULL,               -- raw operator/recipient string
    agency_id     UUID NOT NULL REFERENCES agencies(id),
    state_fips    CHAR(2) NULL,
    note          TEXT NULL,
    source_doc_id UUID NULL REFERENCES raw_documents(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (alias, agency_id)
);
CREATE INDEX IF NOT EXISTS idx_aliases_alias ON aliases (alias);
CREATE INDEX IF NOT EXISTS idx_aliases_agency ON aliases (agency_id);

-- ---------------------------------------------------------------------------
-- cameras: the spine. Idempotent upsert on (osm_type, osm_id).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cameras (
    id                UUID PRIMARY KEY,
    osm_type          TEXT NOT NULL CHECK (osm_type IN ('node','way','relation')),
    osm_id            BIGINT NOT NULL,
    geom              geometry(Point, 4326) NOT NULL,
    operator          TEXT NULL,               -- OSM operator= tag
    manufacturer      TEXT NULL,               -- OSM manufacturer= tag
    tags              JSONB NULL,               -- full OSM tag bag
    place_geoid       TEXT NULL,
    county_geoid      TEXT NULL,
    state_fips        CHAR(2) NULL,
    agency_id         UUID NULL REFERENCES agencies(id),
    agency_confidence NUMERIC NULL,            -- fixed-lookup match score
    source_doc_id     UUID NOT NULL REFERENCES raw_documents(id),
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (osm_type, osm_id)
);
CREATE INDEX IF NOT EXISTS idx_cameras_geom ON cameras USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_cameras_place_geoid ON cameras (place_geoid);
CREATE INDEX IF NOT EXISTS idx_cameras_county_geoid ON cameras (county_geoid);
CREATE INDEX IF NOT EXISTS idx_cameras_agency ON cameras (agency_id);

-- ---------------------------------------------------------------------------
-- source_registry: discovery output (off-path LLM agents). NO MONEY COLUMN.
-- A money-bearing row here is a firewall breach. status proposed -> verified.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS source_registry (
    id            UUID PRIMARY KEY,
    state_fips    CHAR(2) NULL,
    channel       TEXT NOT NULL,               -- funding channel this source covers
    access_tier   TEXT NOT NULL CHECK (access_tier IN ('A','B','C')),  -- api / bulk / foia
    endpoint_url  TEXT NULL,
    dataset_id    TEXT NULL,
    foia_target   TEXT NULL,
    statute_notes TEXT NULL,
    status        TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed','verified','rejected')),
    produced_by   TEXT NOT NULL,               -- e.g. 'llm-discovery-agent'
    verified_by   TEXT NULL,                   -- human who flipped to verified
    verified_at   TIMESTAMPTZ NULL,
    source_doc_id UUID NULL REFERENCES raw_documents(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_source_registry_state ON source_registry (state_fips);
CREATE INDEX IF NOT EXISTS idx_source_registry_status ON source_registry (status);
CREATE INDEX IF NOT EXISTS idx_source_registry_channel ON source_registry (channel);

-- ---------------------------------------------------------------------------
-- funding_records: the ledger. extraction records HOW the number was produced.
-- ml_unreviewed rows are provisional and excluded from headline coverage until
-- a human spot-check flips them to human_verified.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS funding_records (
    id                   UUID PRIMARY KEY,
    agency_id            UUID NULL REFERENCES agencies(id),
    jurisdiction_id      UUID NULL REFERENCES jurisdictions(id),
    amount_usd           NUMERIC NULL,
    cameras_funded_count INTEGER NULL,         -- quantity the doc accounts for
    channel              TEXT NOT NULL,         -- 'usaspending' | 'grants_gov' | 'socrata' | 'foia' | ...
    recipient_name       TEXT NULL,
    keyword_hit          TEXT[] NULL,           -- which keywords matched
    description          TEXT NULL,             -- verbatim award/line description
    term_start           DATE NULL,
    term_end             DATE NULL,
    fund_source          TEXT NULL,             -- federal grant / forfeiture / local budget / ...
    external_id          TEXT NULL,             -- award id / PO number — feeds idempotent UNIQUE
    extraction           TEXT NOT NULL CHECK (extraction IN ('deterministic','human_verified','ml_unreviewed')),
    confidence           NUMERIC NULL,          -- required for ml_unreviewed; null for deterministic
    verified_by          TEXT NULL,
    verified_at          TIMESTAMPTZ NULL,
    source_doc_id        UUID NOT NULL REFERENCES raw_documents(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Idempotent: a given external award/line in a given channel commits once.
    UNIQUE (channel, external_id)
);
CREATE INDEX IF NOT EXISTS idx_funding_records_agency ON funding_records (agency_id);
CREATE INDEX IF NOT EXISTS idx_funding_records_jurisdiction ON funding_records (jurisdiction_id);
CREATE INDEX IF NOT EXISTS idx_funding_records_channel ON funding_records (channel);
CREATE INDEX IF NOT EXISTS idx_funding_records_extraction ON funding_records (extraction);

-- ---------------------------------------------------------------------------
-- camera_funding_matches: M:N camera <-> funding_record. confidence from a
-- fixed lookup (exact_agency_keyword 0.95 / agency_only 0.70 /
-- jurisdiction_keyword 0.55 / jurisdiction_only 0.30). No ML.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS camera_funding_matches (
    id                 UUID PRIMARY KEY,
    camera_id          UUID NOT NULL REFERENCES cameras(id),
    funding_record_id  UUID NOT NULL REFERENCES funding_records(id),
    match_basis        TEXT NOT NULL CHECK (match_basis IN (
                           'exact_agency_keyword','agency_only',
                           'jurisdiction_keyword','jurisdiction_only')),
    confidence         NUMERIC NOT NULL,
    source_doc_id      UUID NULL REFERENCES raw_documents(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (camera_id, funding_record_id)
);
CREATE INDEX IF NOT EXISTS idx_cfm_camera ON camera_funding_matches (camera_id);
CREATE INDEX IF NOT EXISTS idx_cfm_funding ON camera_funding_matches (funding_record_id);

-- ---------------------------------------------------------------------------
-- foia_requests: gap-driven FOIA tracking. response_doc_ids[] -> raw_documents.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS foia_requests (
    id               UUID PRIMARY KEY,
    agency_id        UUID NULL REFERENCES agencies(id),
    jurisdiction_id  UUID NULL REFERENCES jurisdictions(id),
    channel          TEXT NOT NULL CHECK (channel IN ('foia_gov','muckrock','manual')),
    external_id      TEXT NULL,                -- MuckRock / FOIA.gov tracking id
    status           TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
                         'draft','queued','sent','acknowledged','responsive',
                         'completed','rejected','overdue')),
    gap_at_filing    INTEGER NULL,             -- camera gap that justified the request
    request_body     TEXT NULL,                -- the drafted letter (off-ledger)
    statutory_window INTEGER NULL,             -- days
    sent_at          TIMESTAMPTZ NULL,
    due_at           TIMESTAMPTZ NULL,
    response_doc_ids UUID[] NULL,              -- raw_documents ingested in response
    source_doc_id    UUID NULL REFERENCES raw_documents(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_foia_requests_agency ON foia_requests (agency_id);
CREATE INDEX IF NOT EXISTS idx_foia_requests_jurisdiction ON foia_requests (jurisdiction_id);
CREATE INDEX IF NOT EXISTS idx_foia_requests_status ON foia_requests (status);

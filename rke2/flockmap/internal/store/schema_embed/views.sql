-- flockmap coverage views — pure SQL, no ML, idempotent (CREATE OR REPLACE).
--
-- THE HEADLINE RULE: trusted coverage counts only funding_records with
-- extraction IN ('deterministic','human_verified'). 'ml_unreviewed' rows are
-- provisional and surfaced separately, never folded into the headline number.

-- ---------------------------------------------------------------------------
-- camera_funding: camera -> matches -> funding, flattened for the map UI.
-- One row per (camera, matched funding_record). is_trusted marks rows that
-- count toward headline coverage.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE VIEW camera_funding AS
SELECT
    c.id                       AS camera_id,
    c.osm_type,
    c.osm_id,
    c.geom,
    c.operator,
    c.state_fips,
    c.place_geoid,
    c.county_geoid,
    c.agency_id,
    m.id                       AS match_id,
    m.match_basis,
    m.confidence               AS match_confidence,
    f.id                       AS funding_record_id,
    f.amount_usd,
    f.cameras_funded_count,
    f.channel                  AS funding_channel,
    f.fund_source,
    f.extraction,
    f.confidence               AS extraction_confidence,
    f.source_doc_id            AS funding_source_doc_id,
    rd.storage_uri             AS funding_doc_uri,
    rd.source_url              AS funding_doc_source_url,
    (f.extraction IN ('deterministic','human_verified')) AS is_trusted
FROM cameras c
JOIN camera_funding_matches m ON m.camera_id = c.id
JOIN funding_records f        ON f.id = m.funding_record_id
JOIN raw_documents rd         ON rd.id = f.source_doc_id;

-- ---------------------------------------------------------------------------
-- coverage_by_channel: how many distinct cameras have a TRUSTED funder at the
-- headline match threshold (>= 0.70), broken out by funding channel.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE VIEW coverage_by_channel AS
SELECT
    f.channel                            AS funding_channel,
    count(DISTINCT m.camera_id)          AS cameras_covered,
    count(DISTINCT f.id)                 AS funding_records,
    coalesce(sum(f.amount_usd), 0)       AS total_amount_usd,
    coalesce(sum(f.cameras_funded_count), 0) AS total_cameras_funded_count
FROM camera_funding_matches m
JOIN funding_records f ON f.id = m.funding_record_id
WHERE f.extraction IN ('deterministic','human_verified')
  AND m.confidence >= 0.70
GROUP BY f.channel
ORDER BY cameras_covered DESC;

-- ---------------------------------------------------------------------------
-- jurisdiction_funding_gap: the engine that feeds the FOIA queue.
--   gap = cameras_observed (OSM, per jurisdiction/agency)
--         - cameras_accounted (Σ cameras_funded_count of matched TRUSTED funding)
-- Ranked by gap size descending — largest unaccounted gaps first.
-- Grouped by (state_fips, county_geoid, place_geoid, agency_id) so the FOIA
-- targeter can resolve a concrete jurisdiction/agency to file against.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE VIEW jurisdiction_funding_gap AS
WITH observed AS (
    SELECT
        c.state_fips,
        c.county_geoid,
        c.place_geoid,
        c.agency_id,
        count(*) AS cameras_observed
    FROM cameras c
    GROUP BY c.state_fips, c.county_geoid, c.place_geoid, c.agency_id
),
accounted AS (
    -- Sum cameras_funded_count of TRUSTED funding matched to cameras in each
    -- jurisdiction/agency bucket. Each funding_record is counted ONCE per
    -- bucket (the inner DISTINCT collapses the M:N fan-out before summing).
    SELECT
        b.state_fips,
        b.county_geoid,
        b.place_geoid,
        b.agency_id,
        coalesce(sum(b.cameras_funded_count), 0) AS cameras_accounted
    FROM (
        SELECT DISTINCT
            c.state_fips,
            c.county_geoid,
            c.place_geoid,
            c.agency_id,
            f.id                   AS funding_record_id,
            f.cameras_funded_count AS cameras_funded_count
        FROM cameras c
        JOIN camera_funding_matches m ON m.camera_id = c.id
        JOIN funding_records f        ON f.id = m.funding_record_id
        WHERE m.confidence >= 0.70
          AND f.extraction IN ('deterministic','human_verified')
    ) b
    GROUP BY b.state_fips, b.county_geoid, b.place_geoid, b.agency_id
)
SELECT
    o.state_fips,
    o.county_geoid,
    o.place_geoid,
    o.agency_id,
    a.name                                   AS agency_name,
    o.cameras_observed,
    coalesce(acc.cameras_accounted, 0)       AS cameras_accounted,
    o.cameras_observed - coalesce(acc.cameras_accounted, 0) AS gap
FROM observed o
LEFT JOIN accounted acc
       ON acc.state_fips IS NOT DISTINCT FROM o.state_fips
      AND acc.county_geoid IS NOT DISTINCT FROM o.county_geoid
      AND acc.place_geoid IS NOT DISTINCT FROM o.place_geoid
      AND acc.agency_id IS NOT DISTINCT FROM o.agency_id
LEFT JOIN agencies a ON a.id = o.agency_id
ORDER BY gap DESC;

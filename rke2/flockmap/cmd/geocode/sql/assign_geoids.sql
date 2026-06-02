-- flockmap geocode — deterministic point-in-polygon assignment (P1)
--
-- This is the authoritative, ML-free, API-free assignment that stamps every
-- camera with its containing Census geographies. It is the SQL the geocode
-- binary runs in assignCameraGeoids(); it is duplicated here as a standalone,
-- runnable artifact for audit and manual re-runs (psql -f).
--
-- Inputs:  cameras.geom (Point, 4326), jurisdictions.geom (MultiPolygon, 4326)
-- Outputs: cameras.state_fips, cameras.county_geoid, cameras.place_geoid
--
-- Both geom columns carry source_doc_id provenance, so the derived geoids
-- inherit a fully-traceable lineage. ST_Contains uses the GiST indexes
-- (idx_cameras_geom, idx_jurisdictions_geom). Idempotent: re-running recomputes.
--
-- A camera in an unincorporated area legitimately gets place_geoid = NULL.
-- A camera outside all US state polygons (bad coordinate / foreign) gets all
-- three NULL — surface those as a data-quality signal, do not guess.

-- State FIPS (broadest; present for any valid US camera).
UPDATE cameras c
   SET state_fips = j.state_fips
  FROM jurisdictions j
 WHERE j.level = 'state'
   AND ST_Contains(j.geom, c.geom);

-- County GEOID (5-digit: state FIPS + county FIPS).
UPDATE cameras c
   SET county_geoid = j.geoid
  FROM jurisdictions j
 WHERE j.level = 'county'
   AND ST_Contains(j.geom, c.geom);

-- Place GEOID (7-digit incorporated/census place). NULL when unincorporated.
UPDATE cameras c
   SET place_geoid = j.geoid
  FROM jurisdictions j
 WHERE j.level = 'place'
   AND ST_Contains(j.geom, c.geom);

-- Audit / coverage check (run after assignment):
--   SELECT
--     count(*)                                            AS cameras,
--     count(*) FILTER (WHERE state_fips  IS NOT NULL)     AS with_state,
--     count(*) FILTER (WHERE county_geoid IS NOT NULL)    AS with_county,
--     count(*) FILTER (WHERE place_geoid  IS NOT NULL)    AS with_place,
--     count(*) FILTER (WHERE state_fips  IS NULL)         AS no_state_review
--   FROM cameras;

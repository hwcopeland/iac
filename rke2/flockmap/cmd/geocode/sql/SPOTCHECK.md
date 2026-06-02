# geocode spot-check tools (NOT on the hot path)

The authoritative camera→geography assignment is the local TIGER/Line
point-in-polygon join in `assign_geoids.sql` (run by the `geocode` binary).
It is deterministic, reproducible, provenanced, and uses **no external API**.

The two services below are **spot-check only** — to hand-verify a handful of
assignments during QA. They are deliberately **not** wired into the pipeline:
external geocoders rate-limit, change behavior between calls, and would put an
un-provenanced black box on the ledger path. Never let their output write to
`cameras`, `jurisdictions`, or `funding_records`.

## 1. Census Geocoder (coordinates → geographies)

Reverse-geocode a lon/lat to its Census state/county/place. Use to confirm a
sampled camera's computed geoids.

```
GET https://geocoding.geo.census.gov/geocoder/geographies/coordinates
    ?x=<lon>&y=<lat>
    &benchmark=Public_AR_Current
    &vintage=Current_Current
    &layers=all
    &format=json
```

Compare the returned `Counties[0].GEOID` / `Incorporated Places[0].GEOID`
against `cameras.county_geoid` / `cameras.place_geoid` for that camera.

## 2. FCC Census Block / Area API (coordinates → state/county FIPS)

Lighter-weight cross-check for state + county FIPS only:

```
GET https://geo.fcc.gov/api/census/area?lat=<lat>&lon=<lon>&format=json
```

Returns `results[0].state_fips`, `results[0].county_fips`,
`results[0].county_name`. Use to sanity-check `cameras.state_fips` /
`cameras.county_geoid` (county_geoid = state_fips || county_fips).

## When a spot-check disagrees with the local join

Treat it as a data-quality signal, not an override. Check: TIGER vintage
mismatch, a camera coordinate on a boundary, or a stale/duplicate jurisdiction
polygon. Fix the input (re-pull TIGER, correct the camera spine) and re-run
`assign_geoids.sql`. Do not patch `cameras` from the API.

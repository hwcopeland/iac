package main

import (
	"database/sql"
	"fmt"

	"github.com/hwcopeland/flockmap/internal/store"
)

// Jurisdiction is a single Census TIGER geography destined for the
// jurisdictions table. geom is supplied as EWKT (SRID=4326;MULTIPOLYGON(...))
// and cast in SQL via ST_GeomFromEWKT so we never round-trip binary geometry.
type Jurisdiction struct {
	GeoID     string // Census GEOID
	Level     string // 'place' | 'county_subdivision' | 'county' | 'state'
	Name      string
	StateFIPS string // 2-char FIPS, "" -> NULL
	GeomEWKT  string // 'SRID=4326;MULTIPOLYGON(((...)))'
}

// UpsertJurisdiction inserts (or refreshes) one TIGER geography. The firewall
// (CONVENTIONS.md §7) requires a non-empty sourceDocID returned from
// store.SaveRawDocument; it is persisted as source_doc_id. Idempotent on the
// schema's UNIQUE (geoid, level).
//
// geom is forced to MultiPolygon (TIGER ships MultiPolygon already, but
// ST_Multi is a deterministic no-op guard) and validated as SRID 4326.
func UpsertJurisdiction(db *sql.DB, j Jurisdiction, sourceDocID string) (string, error) {
	if sourceDocID == "" {
		return "", fmt.Errorf("UpsertJurisdiction: sourceDocID is required (firewall: source_doc_id)")
	}
	if j.GeomEWKT == "" {
		return "", fmt.Errorf("UpsertJurisdiction: empty geom for geoid=%s level=%s", j.GeoID, j.Level)
	}

	id := store.NewUUIDv7()
	var stateFIPS interface{}
	if j.StateFIPS != "" {
		stateFIPS = j.StateFIPS
	}

	err := db.QueryRow(
		`INSERT INTO jurisdictions
		     (id, geoid, level, name, state_fips, geom, source_doc_id)
		 VALUES ($1,$2,$3,$4,$5, ST_Multi(ST_GeomFromEWKT($6)), $7)
		 ON CONFLICT (geoid, level) DO UPDATE SET
		     name          = EXCLUDED.name,
		     state_fips    = EXCLUDED.state_fips,
		     geom          = EXCLUDED.geom,
		     source_doc_id = EXCLUDED.source_doc_id
		 RETURNING id`,
		id, j.GeoID, j.Level, j.Name, stateFIPS, j.GeomEWKT, sourceDocID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("UpsertJurisdiction(geoid=%s level=%s): %w", j.GeoID, j.Level, err)
	}
	return id, nil
}

// SetCameraGeoids assigns the spatial-join results for one camera. Empty
// strings map to NULL (no containing geography found at that level). This is
// the deterministic point-in-polygon outcome; no provenance row is created
// because the geoids are derived purely from already-provenanced cameras.geom
// and jurisdictions.geom (both carry source_doc_id).
func SetCameraGeoids(db *sql.DB, cameraID, placeGeoid, countyGeoid, stateFips string) error {
	_, err := db.Exec(
		`UPDATE cameras
		    SET place_geoid  = $2,
		        county_geoid = $3,
		        state_fips   = $4
		  WHERE id = $1`,
		cameraID, nullStr(placeGeoid), nullStr(countyGeoid), nullStr(stateFips),
	)
	if err != nil {
		return fmt.Errorf("SetCameraGeoids(camera=%s): %w", cameraID, err)
	}
	return nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

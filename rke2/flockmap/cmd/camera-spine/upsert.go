package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/hwcopeland/flockmap/internal/store"
)

// Camera is the parsed, deterministic projection of one Overpass element ready
// to UPSERT into the cameras table.
//
// NOTE on schema mapping: the frozen db/schema.sql cameras table has columns
// `operator`, `manufacturer`, and `tags` (jsonb) — there is no dedicated
// direction column. We persist Operator->operator, Manufacturer->manufacturer,
// and the full OSM tag bag (which already contains any direction/camera:direction
// tag) into `tags`. Direction is surfaced as a struct field for clarity and is
// preserved losslessly inside the tags jsonb.
type Camera struct {
	OSMType      string // "node" | "way" | "relation"
	OSMID        int64
	Lat          float64
	Lon          float64
	Operator     string            // OSM operator= tag
	Manufacturer string            // OSM manufacturer= tag
	Direction    string            // OSM direction/camera:direction (also in Tags)
	Tags         map[string]string // full OSM tag bag -> tags jsonb
}

// UpsertCamera idempotently inserts or refreshes a camera keyed on
// (osm_type, osm_id). On first insert it generates a UUIDv7 PK and records the
// source_doc_id; on conflict it refreshes geom/operator/manufacturer/tags,
// re-points source_doc_id at the latest evidence, and bumps last_seen_at.
// first_seen_at is preserved across updates.
//
// FIREWALL: sourceDocID is mandatory and is written as source_doc_id (NOT NULL
// in the schema). A camera row without it is a provenance breach.
func UpsertCamera(db *sql.DB, c Camera, sourceDocID string) (string, error) {
	if sourceDocID == "" {
		return "", fmt.Errorf("UpsertCamera: sourceDocID is required (firewall)")
	}
	if c.OSMType != "node" && c.OSMType != "way" && c.OSMType != "relation" {
		return "", fmt.Errorf("UpsertCamera: invalid osm_type %q", c.OSMType)
	}

	tagsJSON, err := json.Marshal(c.Tags)
	if err != nil {
		return "", fmt.Errorf("UpsertCamera: marshal tags: %w", err)
	}

	id := store.NewUUIDv7()

	// ST_SetSRID(ST_MakePoint(lon, lat), 4326) — PostGIS takes (x=lon, y=lat).
	// NULLIF('','') maps empty tag strings to SQL NULL for the text columns.
	const q = `
INSERT INTO cameras
    (id, osm_type, osm_id, geom, operator, manufacturer, tags,
     source_doc_id, first_seen_at, last_seen_at)
VALUES
    ($1, $2, $3,
     ST_SetSRID(ST_MakePoint($4, $5), 4326),
     NULLIF($6, ''), NULLIF($7, ''), $8::jsonb,
     $9, now(), now())
ON CONFLICT (osm_type, osm_id) DO UPDATE SET
    geom          = EXCLUDED.geom,
    operator      = EXCLUDED.operator,
    manufacturer  = EXCLUDED.manufacturer,
    tags          = EXCLUDED.tags,
    source_doc_id = EXCLUDED.source_doc_id,
    last_seen_at  = now()
RETURNING id`

	var outID string
	if err := db.QueryRow(q,
		id, c.OSMType, c.OSMID,
		c.Lon, c.Lat,
		c.Operator, c.Manufacturer, string(tagsJSON),
		sourceDocID,
	).Scan(&outID); err != nil {
		return "", fmt.Errorf("UpsertCamera: %s/%d: %w", c.OSMType, c.OSMID, err)
	}
	return outID, nil
}

// Thin, component-owned upsert helpers for agency-resolve.
//
// CONVENTIONS.md §5: the shared store ships ONLY ConnectPostgres / EnsureSchema
// / NewUUIDv7 / SaveRawDocument. Each connector writes its own upsert helpers in
// its OWN cmd/<name>/ package. Every write here threads a non-empty sourceDocID
// (returned by store.SaveRawDocument) into source_doc_id, satisfying the
// provenance firewall (§7.2).
package main

import (
	"database/sql"
	"fmt"

	"github.com/hwcopeland/flockmap/internal/store"
)

// Agency mirrors the agencies table columns this component writes.
type Agency struct {
	Name           string
	Type           string // police|sheriff|state_police|other_government|private_hoa|business|unknown
	LEAICOri       string // DOJ/BJS LEAIC ORI code; "" -> NULL
	StateFIPS      string // 2-char; "" -> NULL
	JurisdictionID string // UUID; "" -> NULL
}

// UpsertAgency inserts an agencies row (or returns the existing id for an
// identical name+type+state_fips). The agencies table has no natural UNIQUE
// key in the frozen schema, so we dedupe with a guarded SELECT-then-INSERT:
// idempotent for repeated resolver runs over the same crosswalk input.
func UpsertAgency(db *sql.DB, a Agency, sourceDocID string) (string, error) {
	if sourceDocID == "" {
		return "", fmt.Errorf("UpsertAgency: sourceDocID is required (provenance firewall)")
	}

	// Dedupe on the human-meaningful identity tuple.
	var existing string
	err := db.QueryRow(
		`SELECT id FROM agencies
		   WHERE name = $1 AND type = $2
		     AND state_fips IS NOT DISTINCT FROM $3`,
		a.Name, a.Type, nullStr(a.StateFIPS),
	).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("UpsertAgency: dedupe lookup (%s): %w", a.Name, err)
	}

	id := store.NewUUIDv7()
	_, err = db.Exec(
		`INSERT INTO agencies
		    (id, name, type, leaic_ori, state_fips, jurisdiction_id, source_doc_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		id, a.Name, a.Type, nullStr(a.LEAICOri), nullStr(a.StateFIPS),
		nullStr(a.JurisdictionID), sourceDocID,
	)
	if err != nil {
		return "", fmt.Errorf("UpsertAgency: insert (%s): %w", a.Name, err)
	}
	return id, nil
}

// UpsertAlias records a raw operator/recipient string -> agency mapping. The
// schema's UNIQUE (alias, agency_id) makes this idempotent.
func UpsertAlias(db *sql.DB, alias, agencyID, stateFIPS, sourceDocID string) (string, error) {
	if sourceDocID == "" {
		return "", fmt.Errorf("UpsertAlias: sourceDocID is required (provenance firewall)")
	}
	id := store.NewUUIDv7()
	err := db.QueryRow(
		`INSERT INTO aliases (id, alias, agency_id, state_fips, source_doc_id)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (alias, agency_id) DO NOTHING
		 RETURNING id`,
		id, alias, agencyID, nullStr(stateFIPS), sourceDocID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		// Already present — fetch the existing id (idempotent).
		if err := db.QueryRow(
			`SELECT id FROM aliases WHERE alias = $1 AND agency_id = $2`,
			alias, agencyID,
		).Scan(&id); err != nil {
			return "", fmt.Errorf("UpsertAlias: resolving existing (%s): %w", alias, err)
		}
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("UpsertAlias: insert (%s): %w", alias, err)
	}
	return id, nil
}

// SetCameraAgency links a camera to a resolved agency with the fixed-lookup
// confidence score. The matchLabel is recorded in tags-free form on the
// confidence numeric only; the label semantics ('operator_tag'|'crosswalk'|
// 'fuzzy'|'unresolved') map to confidenceFor() scores below.
func SetCameraAgency(db *sql.DB, cameraID, agencyID string, confidence float64) error {
	res, err := db.Exec(
		`UPDATE cameras
		    SET agency_id = $1, agency_confidence = $2, last_seen_at = now()
		  WHERE id = $3`,
		agencyID, confidence, cameraID,
	)
	if err != nil {
		return fmt.Errorf("SetCameraAgency: update camera %s: %w", cameraID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("SetCameraAgency: camera %s not found", cameraID)
	}
	return nil
}

// nullStr maps "" to a SQL NULL so empty optionals store as NULL not "".
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

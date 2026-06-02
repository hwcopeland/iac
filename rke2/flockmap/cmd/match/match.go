package main

import (
	"database/sql"
	"fmt"

	"github.com/hwcopeland/flockmap/internal/store"
)

// Fixed confidence lookup (CONVENTIONS.md §7.5 — never learned).
const (
	basisExactAgencyKeyword  = "exact_agency_keyword"
	basisAgencyOnly          = "agency_only"
	basisJurisdictionKeyword = "jurisdiction_keyword"
	basisJurisdictionOnly    = "jurisdiction_only"

	confExactAgencyKeyword  = 0.95
	confAgencyOnly          = 0.70
	confJurisdictionKeyword = 0.55
	confJurisdictionOnly    = 0.30
)

// matchResult counts the new rows inserted per basis in a single pass.
type matchResult struct {
	exactAgencyKeyword  int64
	agencyOnly          int64
	jurisdictionKeyword int64
	jurisdictionOnly    int64
}

func (r matchResult) total() int64 {
	return r.exactAgencyKeyword + r.agencyOnly + r.jurisdictionKeyword + r.jurisdictionOnly
}

// runMatchPass executes the four deterministic match queries in
// strongest-basis-first order inside one transaction. The ON CONFLICT
// (camera_id, funding_record_id) DO NOTHING guard means that once a stronger
// basis has claimed a pair, weaker bases cannot overwrite it — so ordering the
// inserts high→low confidence yields the strongest applicable basis per pair.
func runMatchPass(db *sql.DB) (matchResult, error) {
	var res matchResult

	tx, err := db.Begin()
	if err != nil {
		return res, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rolled back unless Commit succeeds

	steps := []struct {
		name  string
		basis string
		conf  float64
		query string
		count *int64
	}{
		// 1. Agency match + keyword hit on the funding record → 0.95.
		{
			name:  basisExactAgencyKeyword,
			basis: basisExactAgencyKeyword,
			conf:  confExactAgencyKeyword,
			query: agencyMatchSQL(true),
			count: &res.exactAgencyKeyword,
		},
		// 2. Agency match, no keyword hit → 0.70.
		{
			name:  basisAgencyOnly,
			basis: basisAgencyOnly,
			conf:  confAgencyOnly,
			query: agencyMatchSQL(false),
			count: &res.agencyOnly,
		},
		// 3. Jurisdiction (place/county GEOID) match + keyword hit → 0.55.
		{
			name:  basisJurisdictionKeyword,
			basis: basisJurisdictionKeyword,
			conf:  confJurisdictionKeyword,
			query: jurisdictionMatchSQL(true),
			count: &res.jurisdictionKeyword,
		},
		// 4. Jurisdiction match, no keyword hit → 0.30.
		{
			name:  basisJurisdictionOnly,
			basis: basisJurisdictionOnly,
			conf:  confJurisdictionOnly,
			query: jurisdictionMatchSQL(false),
			count: &res.jurisdictionOnly,
		},
	}

	for _, s := range steps {
		n, err := insertMatches(tx, s.query, s.basis, s.conf)
		if err != nil {
			return res, fmt.Errorf("%s pass: %w", s.name, err)
		}
		*s.count = n
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("commit tx: %w", err)
	}
	return res, nil
}

// insertMatches runs one set-based INSERT ... SELECT over a candidate
// (camera_id, funding_record_id) set, tagging every row with a fixed match_basis
// and its fixed confidence.
//
// The PK is generated in SQL via gen_random_uuid() (PG13+ core) rather than
// store.NewUUIDv7(), because a Go-side per-row UUID cannot be threaded into a
// single set-based INSERT. The PK only needs to be unique; UUIDv7 time-ordering
// is a nicety for the row-at-a-time Go connectors (see UpsertCameraFundingMatch
// below, which does use store.NewUUIDv7()), not for this bulk sweep. The
// (camera_id, funding_record_id) UNIQUE constraint is what enforces idempotency.
func insertMatches(tx *sql.Tx, candidateSQL, basis string, conf float64) (int64, error) {
	full := fmt.Sprintf(`
INSERT INTO camera_funding_matches (id, camera_id, funding_record_id, match_basis, confidence)
SELECT gen_random_uuid(), cand.camera_id, cand.funding_record_id, $1, $2
FROM ( %s ) AS cand
ON CONFLICT (camera_id, funding_record_id) DO NOTHING`, candidateSQL)

	r, err := tx.Exec(full, basis, conf)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	return n, nil
}

// agencyMatchSQL returns the candidate (camera_id, funding_record_id) set for an
// agency-equality join. When withKeyword is true it selects only funding records
// whose keyword_hit array is non-empty; when false, only those with an
// empty/NULL keyword_hit (the agency_only bucket), so the two passes partition
// cleanly and the ON CONFLICT guard is a belt-and-suspenders idempotency net.
func agencyMatchSQL(withKeyword bool) string {
	keywordPred := "(f.keyword_hit IS NULL OR cardinality(f.keyword_hit) = 0)"
	if withKeyword {
		keywordPred = "(f.keyword_hit IS NOT NULL AND cardinality(f.keyword_hit) > 0)"
	}
	// Join cameras to funding_records on the shared agency. Only cameras that
	// have been resolved to an agency (cameras.agency_id NOT NULL) can match.
	return fmt.Sprintf(`
SELECT c.id AS camera_id, f.id AS funding_record_id
FROM cameras c
JOIN funding_records f ON f.agency_id = c.agency_id
WHERE c.agency_id IS NOT NULL
  AND f.agency_id IS NOT NULL
  AND %s`, keywordPred)
}

// jurisdictionMatchSQL returns the candidate set for the jurisdiction fallback,
// joining cameras to funding_records that share a place_geoid or county_geoid
// (resolved via the funding record's jurisdiction_id -> jurisdictions.geoid).
func jurisdictionMatchSQL(withKeyword bool) string {
	keywordPred := "(f.keyword_hit IS NULL OR cardinality(f.keyword_hit) = 0)"
	if withKeyword {
		keywordPred = "(f.keyword_hit IS NOT NULL AND cardinality(f.keyword_hit) > 0)"
	}
	return jurisdictionCandidateSQL(keywordPred)
}

// jurisdictionCandidateSQL builds the jurisdiction-fallback candidate set.
//
// A funding record's jurisdiction is resolved through
// funding_records.jurisdiction_id -> jurisdictions.geoid. A camera carries its
// place_geoid and county_geoid directly. They match when the funding record's
// jurisdiction GEOID equals the camera's place_geoid (a 'place' jurisdiction) or
// county_geoid (a 'county'/'county_subdivision' jurisdiction).
//
// This fallback only fires where the agency join did NOT already claim the pair
// (the strongest-basis-first ordering + ON CONFLICT guard enforce that), and it
// is intentionally below the 0.70 headline threshold: a shared jurisdiction is
// a lead, not proof, that a funding line paid for a given camera.
func jurisdictionCandidateSQL(keywordPred string) string {
	return fmt.Sprintf(`
SELECT DISTINCT c.id AS camera_id, f.id AS funding_record_id
FROM cameras c
JOIN funding_records f ON f.jurisdiction_id IS NOT NULL
JOIN jurisdictions j   ON j.id = f.jurisdiction_id
WHERE (
        (c.place_geoid IS NOT NULL AND c.place_geoid = j.geoid)
     OR (c.county_geoid IS NOT NULL AND c.county_geoid = j.geoid)
      )
  AND %s`, keywordPred)
}

// UpsertCameraFundingMatch inserts a single (camera, funding_record) match with
// a fixed-lookup confidence, per the contract signature in CONVENTIONS.md §5.
// Generates the PK with store.NewUUIDv7(); idempotent via ON CONFLICT
// (camera_id, funding_record_id) DO NOTHING. This is the row-at-a-time helper;
// the bulk pass in runMatchPass uses set-based INSERTs for the full sweep.
//
// match_basis must be one of the four fixed bases; confidence must be the fixed
// value for that basis (the caller owns the lookup — this helper does not learn
// or derive a score).
func UpsertCameraFundingMatch(db *sql.DB, cameraID, fundingRecordID, matchBasis string, confidence float64) (string, error) {
	id := store.NewUUIDv7()
	err := db.QueryRow(
		`INSERT INTO camera_funding_matches
		    (id, camera_id, funding_record_id, match_basis, confidence)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (camera_id, funding_record_id) DO NOTHING
		 RETURNING id`,
		id, cameraID, fundingRecordID, matchBasis, confidence,
	).Scan(&id)
	if err == sql.ErrNoRows {
		// Pair already matched — idempotent: return the existing row id.
		if err := db.QueryRow(
			`SELECT id FROM camera_funding_matches
			 WHERE camera_id = $1 AND funding_record_id = $2`,
			cameraID, fundingRecordID,
		).Scan(&id); err != nil {
			return "", fmt.Errorf("UpsertCameraFundingMatch: resolving existing id: %w", err)
		}
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("UpsertCameraFundingMatch: insert: %w", err)
	}
	return id, nil
}

// refreshCoverage refreshes any materialized coverage state. The current
// coverage objects in db/views.sql are plain (non-materialized) VIEWs, which
// always reflect live data and need no refresh. This is a forward-compatible
// hook: if a MATERIALIZED VIEW named coverage_by_channel_mat (or similar) is
// later added to db/views.sql, refresh it here. We detect materialized views
// dynamically so this component never has to edit db/.
func refreshCoverage(db *sql.DB) error {
	rows, err := db.Query(`SELECT schemaname, matviewname FROM pg_matviews WHERE schemaname = 'public'`)
	if err != nil {
		return fmt.Errorf("listing materialized views: %w", err)
	}
	defer rows.Close()

	var matviews []string
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return fmt.Errorf("scanning matview row: %w", err)
		}
		matviews = append(matviews, fmt.Sprintf("%s.%s", schema, name))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, mv := range matviews {
		// Identifiers come from the catalog, not user input — safe to inline.
		if _, err := db.Exec(fmt.Sprintf(`REFRESH MATERIALIZED VIEW %s`, mv)); err != nil {
			return fmt.Errorf("refreshing %s: %w", mv, err)
		}
	}
	return nil
}

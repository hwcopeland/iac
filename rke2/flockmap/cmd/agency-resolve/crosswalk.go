// Crosswalk + alias loading for agency-resolve.
//
// The crosswalk is the deterministic backbone of jurisdiction->agency mapping.
// It is derived from the DOJ/BJS Law Enforcement Agency Identifiers Crosswalk
// (LEAIC) — a public file that links each U.S. local law-enforcement agency to
// the Census place/county it polices, with its ORI code. We do NOT bundle the
// LEAIC in this repo (it is large and licensed for redistribution under ICPSR
// terms); instead the operator loads it once into the agencies + aliases tables
// (see "Loading the LEAIC crosswalk" below). This file reads those tables.
//
// FIREWALL: every crosswalk/alias row read here was itself written with a
// source_doc_id, and every camera link this resolver writes carries the
// fixed-lookup confidence — no ML, no learned weights.
package main

import (
	"database/sql"
	"fmt"
)

// ----------------------------------------------------------------------------
// Loading the LEAIC crosswalk (operator runbook — NOT executed by this code)
// ----------------------------------------------------------------------------
//
// The agencies table is the crosswalk target. To populate it from the DOJ/BJS
// LEAIC (ICPSR study 35158, "Law Enforcement Agency Identifiers Crosswalk,
// United States"):
//
//  1. Download the LEAIC tabular extract (CSV) from ICPSR. It has, per agency:
//     ORI9 (ORI code), agency NAME, AGENCYTYPE, the Census PLACE FIPS and
//     COUNTY FIPS it covers, and STATE FIPS.
//  2. Run a one-shot loader Job (a future cmd/crosswalk-load, or a psql \copy
//     into a staging table) that, for each LEAIC row, calls SaveRawDocument on
//     the CSV bytes ONCE (channel="leaic", license/attribution = ICPSR cite),
//     then UpsertAgency{Name, Type, LEAICOri, StateFIPS, JurisdictionID} where
//     JurisdictionID is the jurisdictions row matched on (geoid, level) for the
//     place/county FIPS. AGENCYTYPE maps: municipal police -> 'police';
//     sheriff -> 'sheriff'; state police/highway patrol -> 'state_police';
//     everything else governmental -> 'other_government'.
//  3. Also seed the aliases table with the curated name variants (UpsertAlias)
//     so exact-alias hits short-circuit fuzzy matching.
//
// This resolver assumes that load has happened. It maps each camera's
// jurisdiction (place GEOID -> municipal PD; unincorporated county GEOID ->
// sheriff) to the crosswalk agency, falling back to STATE-SCOPED fuzzy match.

// crosswalkAgency is a candidate row pulled from the agencies table for a given
// state, used as the deterministic fuzzy-match candidate set.
type crosswalkAgency struct {
	ID             string
	Name           string
	Type           string
	JurisdictionID sql.NullString
}

// loadStateAgencies returns all agencies in a state (the fuzzy candidate set).
// State-scoping is mandatory: a fuzzy match may only consider agencies in the
// camera's own state (CONVENTIONS: STATE-SCOPED fuzzy match at a fixed
// threshold). Ordered by id for stable, deterministic candidate ordering.
func loadStateAgencies(db *sql.DB, stateFIPS string) ([]crosswalkAgency, error) {
	rows, err := db.Query(
		`SELECT id, name, type, jurisdiction_id
		   FROM agencies
		  WHERE state_fips = $1
		  ORDER BY id ASC`,
		stateFIPS,
	)
	if err != nil {
		return nil, fmt.Errorf("loadStateAgencies(%s): %w", stateFIPS, err)
	}
	defer rows.Close()

	var out []crosswalkAgency
	for rows.Next() {
		var a crosswalkAgency
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.JurisdictionID); err != nil {
			return nil, fmt.Errorf("loadStateAgencies scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// lookupAgencyByJurisdiction returns the agency whose jurisdiction_id matches
// the camera's place (preferred type 'police') or county ('sheriff') GEOID.
// This is the primary, exact crosswalk path: a TIGER GEOID resolved by the
// geocode component links directly to a LEAIC-seeded agency. Returns ("",nil)
// when no crosswalk agency covers the jurisdiction.
//
// preferType lets the caller bias selection: a place jurisdiction prefers
// 'police'; an unincorporated county prefers 'sheriff'.
func lookupAgencyByJurisdiction(db *sql.DB, jurisdictionID, preferType string) (string, error) {
	if jurisdictionID == "" {
		return "", nil
	}
	var id string
	err := db.QueryRow(
		`SELECT id FROM agencies
		   WHERE jurisdiction_id = $1
		   ORDER BY (type = $2) DESC, id ASC
		   LIMIT 1`,
		jurisdictionID, preferType,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookupAgencyByJurisdiction(%s): %w", jurisdictionID, err)
	}
	return id, nil
}

// lookupJurisdictionIDByGeoid resolves a jurisdictions.id from a (geoid, level)
// pair written by the geocode component. Returns ("",nil) if absent.
func lookupJurisdictionIDByGeoid(db *sql.DB, geoid, level string) (string, error) {
	if geoid == "" {
		return "", nil
	}
	var id string
	err := db.QueryRow(
		`SELECT id FROM jurisdictions WHERE geoid = $1 AND level = $2`,
		geoid, level,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookupJurisdictionIDByGeoid(%s,%s): %w", geoid, level, err)
	}
	return id, nil
}

// lookupAgencyByAlias resolves an exact (case-insensitive) alias hit, scoped to
// state. The curated aliases table short-circuits fuzzy matching for known
// operator-string variants. Returns ("",nil) on no hit.
func lookupAgencyByAlias(db *sql.DB, alias, stateFIPS string) (string, error) {
	if alias == "" {
		return "", nil
	}
	var id string
	err := db.QueryRow(
		`SELECT a.agency_id
		   FROM aliases a
		   JOIN agencies g ON g.id = a.agency_id
		  WHERE lower(a.alias) = lower($1)
		    AND g.state_fips IS NOT DISTINCT FROM $2
		  ORDER BY a.id ASC
		  LIMIT 1`,
		alias, nullStr(stateFIPS),
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookupAgencyByAlias(%s): %w", alias, err)
	}
	return id, nil
}

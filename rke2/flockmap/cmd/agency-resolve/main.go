// Command agency-resolve deterministically resolves cameras.agency_id.
//
// Pipeline position: P2. Runs after camera-spine (P0, populates cameras) and
// geocode (P1, populates cameras.place_geoid/county_geoid/state_fips and the
// jurisdictions table). For each camera with no agency yet, it resolves an
// agency by, in strict priority order:
//
//  1. operator_tag  — the OSM operator= tag names the agency directly. Upsert an
//     agencies row from it, record the operator string as an
//     alias, link the camera. confidence label 'operator_tag'.
//  2. crosswalk     — the camera's place GEOID (-> municipal PD) or, for an
//     unincorporated county, county GEOID (-> sheriff) maps to a
//     LEAIC-seeded agency via jurisdiction_id. label 'crosswalk'.
//  3. fuzzy         — STATE-SCOPED token-sort/Levenshtein match of the camera's
//     jurisdiction name against agencies in the same state, at a
//     FIXED threshold (match.go). label 'fuzzy'.
//  4. unresolved    — nothing matched; camera left without an agency. 'unresolved'.
//
// THE FIREWALL (CONVENTIONS.md §7): no ML anywhere. Every external byte stream
// goes through store.SaveRawDocument first; every agency/alias write threads the
// returned source_doc_id. Match confidence is a fixed lookup, never learned.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hwcopeland/flockmap/internal/store"
)

// fetchedByJob is this component's ingest-job identity for the raw_documents
// allowlist (CONVENTIONS §7 audit invariant).
const fetchedByJob = "agency-resolve"

// confidenceFor maps a resolution label to the camera's agency_confidence
// numeric. These are FIXED scores, never learned. operator_tag is highest
// (the source named the operator), then crosswalk, then fuzzy.
func confidenceFor(label string) float64 {
	switch label {
	case "operator_tag":
		return 0.95
	case "crosswalk":
		return 0.90
	case "fuzzy":
		return 0.75
	default: // unresolved
		return 0.0
	}
}

// cameraRow is the subset of cameras this resolver reads.
type cameraRow struct {
	ID          string
	Operator    sql.NullString
	Tags        []byte // JSONB operator/recipient bag
	PlaceGeoid  sql.NullString
	CountyGeoid sql.NullString
	StateFIPS   sql.NullString
}

func main() {
	limit := flag.Int("limit", 0, "max cameras to process (0 = all unresolved)")
	dryRun := flag.Bool("dry-run", false, "log decisions without writing camera links")
	flag.Parse()

	log.Printf("[agency-resolve] starting (dry-run=%v)", *dryRun)

	db, err := store.ConnectPostgres()
	if err != nil {
		log.Fatalf("[agency-resolve] connect postgres: %v", err)
	}
	defer db.Close()

	if err := store.EnsureSchema(db); err != nil {
		log.Fatalf("[agency-resolve] ensure schema: %v", err)
	}

	// One provenance row covers this resolver run: the "evidence" is the
	// deterministic decision log (the input camera+jurisdiction state and the
	// fixed-threshold rule). Every agency/alias/camera write this run makes
	// references it as source_doc_id, satisfying the firewall for derived rows.
	runDoc := buildRunDoc(*dryRun)
	sourceDocID, err := store.SaveRawDocument(db, runDoc)
	if err != nil {
		log.Fatalf("[agency-resolve] save run provenance: %v", err)
	}
	log.Printf("[agency-resolve] run provenance source_doc_id=%s", sourceDocID)

	cams, err := loadUnresolvedCameras(db, *limit)
	if err != nil {
		log.Fatalf("[agency-resolve] load cameras: %v", err)
	}
	log.Printf("[agency-resolve] %d unresolved cameras to process", len(cams))

	// Cache state agency candidate sets so we load each state once.
	stateCache := map[string][]crosswalkAgency{}

	var counts = map[string]int{}
	for _, c := range cams {
		label, agencyID, err := resolveCamera(db, c, sourceDocID, stateCache)
		if err != nil {
			log.Printf("[agency-resolve] camera %s: %v", c.ID, err)
			continue
		}
		counts[label]++

		if label == "unresolved" || agencyID == "" {
			continue
		}
		if *dryRun {
			log.Printf("[agency-resolve] DRY camera=%s -> agency=%s (%s)", c.ID, agencyID, label)
			continue
		}
		if err := SetCameraAgency(db, c.ID, agencyID, confidenceFor(label)); err != nil {
			log.Printf("[agency-resolve] link camera %s: %v", c.ID, err)
		}
	}

	log.Printf("[agency-resolve] done: operator_tag=%d crosswalk=%d fuzzy=%d unresolved=%d",
		counts["operator_tag"], counts["crosswalk"], counts["fuzzy"], counts["unresolved"])
}

// resolveCamera applies the priority ladder and returns the chosen label and
// agency id ("" + "unresolved" when nothing matched).
func resolveCamera(db *sql.DB, c cameraRow, sourceDocID string, stateCache map[string][]crosswalkAgency) (string, string, error) {
	state := c.StateFIPS.String
	operator := operatorString(c)

	// --- 1. operator_tag ---------------------------------------------------
	if operator != "" {
		// An exact curated alias for this operator string wins first.
		if aid, err := lookupAgencyByAlias(db, operator, state); err != nil {
			return "", "", err
		} else if aid != "" {
			return "operator_tag", aid, nil
		}

		// Otherwise materialize the operator-named agency and link it.
		jurID := preferredJurisdictionID(db, c)
		aid, err := UpsertAgency(db, Agency{
			Name:           operator,
			Type:           classifyOperatorType(operator),
			StateFIPS:      state,
			JurisdictionID: jurID,
		}, sourceDocID)
		if err != nil {
			return "", "", err
		}
		// Record the operator string as an alias for future exact hits.
		if _, err := UpsertAlias(db, operator, aid, state, sourceDocID); err != nil {
			return "", "", err
		}
		return "operator_tag", aid, nil
	}

	// --- 2. crosswalk (jurisdiction -> LEAIC agency) -----------------------
	// place GEOID -> municipal PD.
	if c.PlaceGeoid.Valid && c.PlaceGeoid.String != "" {
		jurID, err := lookupJurisdictionIDByGeoid(db, c.PlaceGeoid.String, "place")
		if err != nil {
			return "", "", err
		}
		if aid, err := lookupAgencyByJurisdiction(db, jurID, "police"); err != nil {
			return "", "", err
		} else if aid != "" {
			return "crosswalk", aid, nil
		}
	}
	// unincorporated county (no place) -> sheriff.
	if c.CountyGeoid.Valid && c.CountyGeoid.String != "" {
		jurID, err := lookupJurisdictionIDByGeoid(db, c.CountyGeoid.String, "county")
		if err != nil {
			return "", "", err
		}
		if aid, err := lookupAgencyByJurisdiction(db, jurID, "sheriff"); err != nil {
			return "", "", err
		} else if aid != "" {
			return "crosswalk", aid, nil
		}
	}

	// --- 3. fuzzy (STATE-SCOPED, fixed threshold) --------------------------
	if state != "" {
		jurName, err := jurisdictionName(db, c)
		if err != nil {
			return "", "", err
		}
		if jurName != "" {
			cands, ok := stateCache[state]
			if !ok {
				cands, err = loadStateAgencies(db, state)
				if err != nil {
					return "", "", err
				}
				stateCache[state] = cands
			}
			names := make([]string, len(cands))
			for i, ca := range cands {
				names[i] = ca.Name
			}
			if idx, score := bestMatch(jurName, names); idx >= 0 && score >= fuzzyThreshold {
				return "fuzzy", cands[idx].ID, nil
			}
		}
	}

	// --- 4. unresolved -----------------------------------------------------
	return "unresolved", "", nil
}

// operatorString extracts the operator name from the column or the tag bag.
func operatorString(c cameraRow) string {
	if c.Operator.Valid && c.Operator.String != "" {
		return c.Operator.String
	}
	if len(c.Tags) > 0 {
		var tags map[string]string
		if err := json.Unmarshal(c.Tags, &tags); err == nil {
			if v := tags["operator"]; v != "" {
				return v
			}
		}
	}
	return ""
}

// classifyOperatorType makes a deterministic, keyword-only type bucket from the
// operator string. No ML — a fixed substring table.
func classifyOperatorType(op string) string {
	s := normalize(op)
	switch {
	case containsTok(s, "sheriff"):
		return "sheriff"
	case containsTok(s, "state police"), containsTok(s, "highway patrol"),
		containsTok(s, "state patrol"):
		return "state_police"
	case containsTok(s, "police"), containsTok(s, "police department"):
		return "police"
	case containsTok(s, "hoa"), containsTok(s, "homeowners"),
		containsTok(s, "neighborhood association"):
		return "private_hoa"
	default:
		return "other_government"
	}
}

func containsTok(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// preferredJurisdictionID resolves the camera's jurisdiction id (place over
// county) for stamping onto a freshly-created agency. Best-effort; "" on miss.
func preferredJurisdictionID(db *sql.DB, c cameraRow) string {
	if c.PlaceGeoid.Valid && c.PlaceGeoid.String != "" {
		if id, _ := lookupJurisdictionIDByGeoid(db, c.PlaceGeoid.String, "place"); id != "" {
			return id
		}
	}
	if c.CountyGeoid.Valid && c.CountyGeoid.String != "" {
		if id, _ := lookupJurisdictionIDByGeoid(db, c.CountyGeoid.String, "county"); id != "" {
			return id
		}
	}
	return ""
}

// jurisdictionName returns the place name (preferred) or county name for the
// camera, used as the fuzzy-match query string.
func jurisdictionName(db *sql.DB, c cameraRow) (string, error) {
	for _, q := range []struct {
		geoid sql.NullString
		level string
	}{
		{c.PlaceGeoid, "place"},
		{c.CountyGeoid, "county"},
	} {
		if !q.geoid.Valid || q.geoid.String == "" {
			continue
		}
		var name string
		err := db.QueryRow(
			`SELECT name FROM jurisdictions WHERE geoid = $1 AND level = $2`,
			q.geoid.String, q.level,
		).Scan(&name)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("jurisdictionName(%s,%s): %w", q.geoid.String, q.level, err)
		}
		if name != "" {
			return name, nil
		}
	}
	return "", nil
}

// loadUnresolvedCameras returns cameras with no agency yet. limit<=0 = all.
func loadUnresolvedCameras(db *sql.DB, limit int) ([]cameraRow, error) {
	q := `SELECT id, operator, tags, place_geoid, county_geoid, state_fips
	        FROM cameras
	       WHERE agency_id IS NULL
	       ORDER BY id ASC`
	args := []interface{}{}
	if limit > 0 {
		q += " LIMIT $1"
		args = append(args, limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query unresolved cameras: %w", err)
	}
	defer rows.Close()

	var out []cameraRow
	for rows.Next() {
		var c cameraRow
		if err := rows.Scan(&c.ID, &c.Operator, &c.Tags, &c.PlaceGeoid, &c.CountyGeoid, &c.StateFIPS); err != nil {
			return nil, fmt.Errorf("scan camera: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// buildRunDoc constructs the provenance record describing this resolver run.
// The bytes are a deterministic JSON manifest of the rule set (threshold,
// confidence lookup, priority ladder) so the evidence vault records exactly how
// every link in this run was decided. SaveRawDocument dedupes on content, so an
// identical rule set + day collapses to one row.
func buildRunDoc(dryRun bool) store.RawDocument {
	manifest := map[string]interface{}{
		"component":       fetchedByJob,
		"date":            time.Now().UTC().Format("2006-01-02"),
		"dry_run":         dryRun,
		"fuzzy_threshold": fuzzyThreshold,
		"priority":        []string{"operator_tag", "crosswalk", "fuzzy", "unresolved"},
		"confidence": map[string]float64{
			"operator_tag": confidenceFor("operator_tag"),
			"crosswalk":    confidenceFor("crosswalk"),
			"fuzzy":        confidenceFor("fuzzy"),
		},
		"matcher": "token-sort + levenshtein (deterministic, no ML)",
	}
	raw, _ := json.Marshal(manifest)
	return store.RawDocument{
		SourceURL:    "flockmap://agency-resolve/run-manifest",
		HTTPMethod:   "GET",
		ContentType:  "application/json",
		RawBytes:     raw,
		Channel:      "agency-resolve",
		FetchedByJob: fetchedByJob,
	}
}

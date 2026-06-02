// Command match is the flockmap P5 matcher: a single, deterministic SQL pass
// that populates the M:N camera_funding_matches table by joining cameras to
// funding_records, then refreshes the coverage views.
//
// THE FIREWALL (CONVENTIONS.md §7): no ML anywhere. Every match is produced by
// fixed-threshold SQL — there is no learned scoring. The confidence for each
// match_basis is a constant lookup, never trained:
//
//	exact_agency_keyword  0.95   agency_id == agency_id AND funding.keyword_hit non-empty
//	agency_only           0.70   agency_id == agency_id
//	jurisdiction_keyword  0.55   shared place/county GEOID AND funding.keyword_hit non-empty
//	jurisdiction_only      0.30   shared place/county GEOID
//
// Headline coverage threshold is >= 0.70 (see db/views.sql); the two
// jurisdiction bases are deliberately below it (fallback / lead only).
//
// Provenance: this pass creates NO new external evidence — it only joins rows
// that already carry their own source_doc_id. The match itself is a derived,
// deterministic SQL artifact, so camera_funding_matches.source_doc_id is left
// NULL (the schema allows it). The funding_record on the other side of every
// match still carries its mandatory source_doc_id, so the ledger stays traced.
// Because this component performs NO external fetch, it does not (and must not)
// call store.SaveRawDocument.
//
// Idempotent: ON CONFLICT (camera_id, funding_record_id) DO NOTHING. Inserts run
// strongest-basis-first so the highest applicable confidence wins for a given
// pair, and re-running the Job is a no-op once the table is populated.
package main

import (
	"flag"
	"log"

	"github.com/hwcopeland/flockmap/internal/store"
)

func main() {
	skipSchema := flag.Bool("skip-schema", false, "skip EnsureSchema (migration Job already ran)")
	flag.Parse()

	log.Println("flockmap match: starting deterministic match pass")

	db, err := store.ConnectPostgres()
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	if !*skipSchema {
		if err := store.EnsureSchema(db); err != nil {
			log.Fatalf("ensure schema: %v", err)
		}
	}

	res, err := runMatchPass(db)
	if err != nil {
		log.Fatalf("match pass: %v", err)
	}
	log.Printf("flockmap match: inserted matches by basis: exact_agency_keyword=%d agency_only=%d jurisdiction_keyword=%d jurisdiction_only=%d (total new=%d)",
		res.exactAgencyKeyword, res.agencyOnly, res.jurisdictionKeyword, res.jurisdictionOnly, res.total())

	if err := refreshCoverage(db); err != nil {
		// Views are CREATE OR REPLACE (no materialized views exist yet); a
		// failure here is non-fatal for the match data already committed, but
		// we surface it loudly.
		log.Printf("flockmap match: WARNING coverage refresh: %v", err)
	}

	log.Println("flockmap match: done")
}

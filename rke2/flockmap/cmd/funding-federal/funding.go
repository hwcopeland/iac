package main

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hwcopeland/flockmap/internal/store"
	"github.com/lib/pq"
)

// FundingRecord is the deterministic ledger row this connector emits. It maps
// 1:1 onto the funding_records table columns this component writes. PK, created_at,
// and the agency/jurisdiction joins are left to downstream P2/P5 components — this
// connector only writes the raw federal money fact + its provenance.
type FundingRecord struct {
	RecipientName      string   // recipient_name
	AmountUSD          *float64 // amount_usd (nullable)
	AwardDate          *string  // term_start (ISO date) — best available award date
	Channel            string   // 'federal_grant' | 'federal_contract'
	KeywordHit         []string // keyword_hit[]
	Description        string   // verbatim award description
	ExternalID         string   // award id — feeds the idempotent UNIQUE (channel, external_id)
	CamerasFundedCount *int     // parsed deterministically from description; nil if absent
	FundSource         string   // human-readable funding source label
}

// cameraCountRe deterministically extracts a camera quantity from a verbatim
// description. ML-free: a single fixed regex, leftmost match wins, null if absent.
// Matches forms like "12 cameras", "8 LPR", "30 license plate readers".
var cameraCountRe = regexp.MustCompile(`(?i)\b(\d{1,5})\s+(?:cameras?|lpr|license[\s-]plate(?:\s+readers?)?)\b`)

// parseCamerasFunded returns the camera quantity stated in description, or nil if
// none is found. Deterministic: no inference, no ML — just the fixed regex above.
func parseCamerasFunded(description string) *int {
	m := cameraCountRe.FindStringSubmatch(description)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}
	return &n
}

// UpsertFundingRecord inserts a deterministic federal funding fact, threading the
// provenance source_doc_id. Idempotent on the schema's UNIQUE (channel, external_id):
// a re-seen award commits once. extraction is always 'deterministic' (API parse),
// so confidence stays NULL per the firewall.
func UpsertFundingRecord(db *sql.DB, f FundingRecord, sourceDocID string) (string, error) {
	if sourceDocID == "" {
		return "", fmt.Errorf("UpsertFundingRecord: sourceDocID is required (provenance firewall)")
	}
	if f.Channel == "" {
		return "", fmt.Errorf("UpsertFundingRecord: channel is required")
	}
	if f.ExternalID == "" {
		return "", fmt.Errorf("UpsertFundingRecord: external_id is required for idempotency")
	}

	id := store.NewUUIDv7()

	var keywordHit interface{}
	if len(f.KeywordHit) > 0 {
		keywordHit = pq.Array(f.KeywordHit)
	}

	err := db.QueryRow(
		`INSERT INTO funding_records
		    (id, amount_usd, cameras_funded_count, channel, recipient_name,
		     keyword_hit, description, term_start, fund_source, external_id,
		     extraction, source_doc_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'deterministic',$11)
		 ON CONFLICT (channel, external_id) DO NOTHING
		 RETURNING id`,
		id,
		nullableFloat(f.AmountUSD),
		nullableInt(f.CamerasFundedCount),
		f.Channel,
		nullableStr(f.RecipientName),
		keywordHit,
		nullableStr(f.Description),
		nullableStr(derefStr(f.AwardDate)),
		nullableStr(f.FundSource),
		f.ExternalID,
		sourceDocID,
	).Scan(&id)

	if err == sql.ErrNoRows {
		// Already committed for this (channel, external_id) — idempotent.
		if err := db.QueryRow(
			`SELECT id FROM funding_records WHERE channel = $1 AND external_id = $2`,
			f.Channel, f.ExternalID,
		).Scan(&id); err != nil {
			return "", fmt.Errorf("UpsertFundingRecord: resolving existing id (%s/%s): %w", f.Channel, f.ExternalID, err)
		}
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("UpsertFundingRecord: inserting (%s/%s): %w", f.Channel, f.ExternalID, err)
	}
	return id, nil
}

// matchedKeywords returns the subset of searchKeywords (case-insensitive) that
// appear verbatim in the haystack. Deterministic substring match — no fuzzing.
func matchedKeywords(haystack string, searchKeywords []string) []string {
	lower := strings.ToLower(haystack)
	var hits []string
	for _, kw := range searchKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hits = append(hits, kw)
		}
	}
	return hits
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableFloat(f *float64) interface{} {
	if f == nil {
		return nil
	}
	return *f
}

func nullableInt(i *int) interface{} {
	if i == nil {
		return nil
	}
	return *i
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Command funding-federal is a deterministic, ML-free flockmap connector that
// pulls federal funding facts relevant to Flock/ALPR cameras from two sources:
//
//   - USAspending (api.usaspending.gov spending_by_award): grants + contracts.
//     Every raw page is stored verbatim via store.SaveRawDocument (channel
//     "usaspending") BEFORE any parse, then each award becomes a funding_records
//     row (channel 'federal_grant'/'federal_contract', extraction 'deterministic').
//   - grants.gov (api.grants.gov search2): ALN/CFDA discovery only. Raw responses
//     are stored (channel "grants_gov") so discovered assistance-listing numbers
//     are court-traceable; this connector does NOT mint money rows from grants.gov
//     (USAspending is the authoritative award ledger).
//
// Firewall (CONVENTIONS.md §7): no ML on the data path. cameras_funded_count is
// extracted from the verbatim award description by ONE fixed regex, else left null.
// Every funding_records row carries a source_doc_id and extraction='deterministic'.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hwcopeland/flockmap/internal/store"
)

const (
	jobName            = "funding-federal"
	usaspendingURL     = "https://api.usaspending.gov/api/v2/search/spending_by_award/"
	grantsGovSearchURL = "https://api.grants.gov/v1/api/search2"
	pageLimit          = 100 // USAspending max page size
	maxPages           = 200 // safety ceiling per (keyword, award-group) combo
	httpTimeout        = 120 * time.Second
)

// keywords are the verbatim search terms; a matched term is recorded in keyword_hit[].
var keywords = []string{
	"license plate reader",
	"automated license plate",
	"ALPR",
	"Flock",
	"Flock Group",
}

// awardGroup pairs a USAspending award_type_codes set with the funding_records
// channel it maps to. Grants and contracts are queried separately because the
// USAspending API requires homogeneous award_type_codes per request.
type awardGroup struct {
	label      string   // for logging
	channel    string   // funding_records.channel
	fundSource string   // funding_records.fund_source label
	typeCodes  []string // USAspending award_type_codes
	fields     []string // requested fields (differ for grants vs contracts)
}

var awardGroups = []awardGroup{
	{
		label:      "grants",
		channel:    "federal_grant",
		fundSource: "federal grant (USAspending)",
		typeCodes:  []string{"02", "03", "04", "05"},
		fields: []string{
			"Award ID", "Recipient Name", "Award Amount",
			"Start Date", "End Date", "Awarding Agency", "Award Type",
		},
	},
	{
		label:      "contracts",
		channel:    "federal_contract",
		fundSource: "federal contract (USAspending)",
		typeCodes:  []string{"A", "B", "C", "D"},
		fields: []string{
			"Award ID", "Recipient Name", "Award Amount",
			"Start Date", "End Date", "Awarding Agency", "Award Type",
			"Description",
		},
	},
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("%s starting (deterministic federal funding connector)", jobName)

	db, err := store.ConnectPostgres()
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()
	log.Println("postgres connection established")

	if err := store.EnsureSchema(db); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}
	log.Println("schema ensured")

	client := &http.Client{Timeout: httpTimeout}

	// 1) grants.gov ALN discovery (raw stored; metadata only, no money rows).
	if err := discoverGrantsGovALNs(db, client); err != nil {
		// Non-fatal: discovery feeds future ALN-scoped queries, but the USAspending
		// keyword sweep below is the primary ledger source.
		log.Printf("grants.gov ALN discovery failed (continuing): %v", err)
	}

	// 2) USAspending keyword sweep across grants + contracts (the ledger).
	var totalInserted int
	for _, g := range awardGroups {
		for _, kw := range keywords {
			n, err := sweepUSAspending(db, client, g, kw)
			if err != nil {
				log.Printf("usaspending sweep %s/%q failed: %v", g.label, kw, err)
				continue
			}
			totalInserted += n
			log.Printf("usaspending %s keyword=%q: %d records upserted", g.label, kw, n)
		}
	}

	log.Printf("%s done: %d funding_records upserted total", jobName, totalInserted)
}

// --- USAspending ---

// usaRequest is the spending_by_award POST body.
type usaRequest struct {
	Filters    usaFilters `json:"filters"`
	Fields     []string   `json:"fields"`
	Page       int        `json:"page"`
	Limit      int        `json:"limit"`
	Sort       string     `json:"sort"`
	Order      string     `json:"order"`
	SubawardID *string    `json:"subawards,omitempty"`
}

type usaFilters struct {
	Keywords       []string `json:"keywords"`
	AwardTypeCodes []string `json:"award_type_codes"`
}

// usaResponse is the relevant slice of the spending_by_award response. Fields use
// the human "Award ID"/"Recipient Name" keys requested above, so results decode
// into a generic map per row.
type usaResponse struct {
	Results      []map[string]interface{} `json:"results"`
	PageMetadata struct {
		Page    int  `json:"page"`
		HasNext bool `json:"hasNext"`
		Next    *int `json:"next"`
	} `json:"page_metadata"`
}

// sweepUSAspending pages through all awards matching (keyword, awardGroup), storing
// every raw page first, then upserting each award as a deterministic funding_record.
// Returns the number of funding_records upserted.
func sweepUSAspending(db *sql.DB, client *http.Client, g awardGroup, keyword string) (int, error) {
	inserted := 0
	for page := 1; page <= maxPages; page++ {
		reqBody := usaRequest{
			Filters: usaFilters{
				Keywords:       []string{keyword},
				AwardTypeCodes: g.typeCodes,
			},
			Fields: g.fields,
			Page:   page,
			Limit:  pageLimit,
			Sort:   "Award Amount",
			Order:  "desc",
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return inserted, fmt.Errorf("marshal request: %w", err)
		}

		raw, contentType, err := postJSON(client, usaspendingURL, bodyBytes)
		if err != nil {
			return inserted, fmt.Errorf("page %d: %w", page, err)
		}

		// Provenance FIRST: store the verbatim page before parsing.
		sourceDocID, err := store.SaveRawDocument(db, store.RawDocument{
			SourceURL:    usaspendingURL,
			HTTPMethod:   http.MethodPost,
			RequestBody:  string(bodyBytes),
			ContentType:  contentType,
			RawBytes:     raw,
			Channel:      "usaspending",
			FetchedByJob: jobName,
		})
		if err != nil {
			return inserted, fmt.Errorf("page %d save raw: %w", page, err)
		}

		var resp usaResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return inserted, fmt.Errorf("page %d unmarshal: %w", page, err)
		}

		for _, row := range resp.Results {
			f, ok := buildFundingRecord(row, g, keyword)
			if !ok {
				continue
			}
			if _, err := UpsertFundingRecord(db, f, sourceDocID); err != nil {
				log.Printf("upsert %s/%s: %v", f.Channel, f.ExternalID, err)
				continue
			}
			inserted++
		}

		if len(resp.Results) == 0 || !resp.PageMetadata.HasNext {
			break
		}
		// Be polite to the public API.
		time.Sleep(250 * time.Millisecond)
	}
	return inserted, nil
}

// buildFundingRecord maps a USAspending result row into a deterministic FundingRecord.
// Returns ok=false if there is no usable award id (the idempotency key).
func buildFundingRecord(row map[string]interface{}, g awardGroup, keyword string) (FundingRecord, bool) {
	awardID := asString(row["Award ID"])
	if awardID == "" {
		// Fall back to generated_internal_id if present (USAspending sometimes
		// returns it as the canonical link key).
		awardID = asString(row["generated_internal_id"])
	}
	if awardID == "" {
		return FundingRecord{}, false
	}

	recipient := asString(row["Recipient Name"])
	description := asString(row["Description"])

	// The description is the only free-text field; match keywords against it plus
	// the recipient + award type so keyword_hit reflects what actually matched.
	haystack := strings.Join([]string{description, recipient, asString(row["Award Type"])}, " ")
	hits := matchedKeywords(haystack, keywords)
	// The query keyword itself always counts as a hit (it produced this result).
	hits = ensureContains(hits, keyword)

	startDate := normalizeDate(asString(row["Start Date"]))

	f := FundingRecord{
		RecipientName:      recipient,
		AmountUSD:          asFloatPtr(row["Award Amount"]),
		AwardDate:          startDate,
		Channel:            g.channel,
		KeywordHit:         hits,
		Description:        description,
		ExternalID:         awardID,
		CamerasFundedCount: parseCamerasFunded(description),
		FundSource:         g.fundSource,
	}
	return f, true
}

// --- grants.gov ALN discovery ---

type grantsGovRequest struct {
	Keyword        string `json:"keyword"`
	OppStatuses    string `json:"oppStatuses"`
	Rows           int    `json:"rows"`
	StartRecordNum int    `json:"startRecordNum"`
}

type grantsGovResponse struct {
	Data struct {
		HitCount int `json:"hitCount"`
		OppHits  []struct {
			Number     string   `json:"number"`
			Title      string   `json:"title"`
			AgencyCode string   `json:"agencyCode"`
			ALNList    []string `json:"alnist"` // assistance listing numbers (CFDA)
		} `json:"oppHits"`
	} `json:"data"`
}

// discoverGrantsGovALNs hits grants.gov search2 for each keyword and stores each
// raw response (channel "grants_gov") for provenance. It logs the distinct ALN
// (CFDA) numbers discovered so a later component can scope USAspending by program.
// It does NOT write funding_records (grants.gov is discovery metadata, not the ledger).
func discoverGrantsGovALNs(db *sql.DB, client *http.Client) error {
	alnSet := map[string]struct{}{}
	for _, kw := range keywords {
		reqBody := grantsGovRequest{
			Keyword:        kw,
			OppStatuses:    "forecasted|posted|closed|archived",
			Rows:           100,
			StartRecordNum: 0,
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal grants.gov request: %w", err)
		}
		raw, contentType, err := postJSON(client, grantsGovSearchURL, bodyBytes)
		if err != nil {
			log.Printf("grants.gov %q: %v", kw, err)
			continue
		}
		// Provenance FIRST.
		if _, err := store.SaveRawDocument(db, store.RawDocument{
			SourceURL:    grantsGovSearchURL,
			HTTPMethod:   http.MethodPost,
			RequestBody:  string(bodyBytes),
			ContentType:  contentType,
			RawBytes:     raw,
			Channel:      "grants_gov",
			FetchedByJob: jobName,
		}); err != nil {
			return fmt.Errorf("grants.gov %q save raw: %w", kw, err)
		}

		var resp grantsGovResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			log.Printf("grants.gov %q unmarshal (raw stored): %v", kw, err)
			continue
		}
		for _, h := range resp.Data.OppHits {
			for _, aln := range h.ALNList {
				if aln != "" {
					alnSet[aln] = struct{}{}
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}

	if len(alnSet) > 0 {
		alns := make([]string, 0, len(alnSet))
		for a := range alnSet {
			alns = append(alns, a)
		}
		log.Printf("grants.gov discovered %d distinct ALN/CFDA numbers: %s",
			len(alns), strings.Join(alns, ", "))
	} else {
		log.Println("grants.gov: no ALN/CFDA numbers discovered")
	}
	return nil
}

// --- HTTP + JSON helpers ---

// postJSON POSTs a JSON body and returns the raw response bytes + Content-Type.
// Non-2xx responses return an error including a truncated body for diagnosis;
// the verbatim bytes are still returned so the caller can store them as evidence.
func postJSON(client *http.Client, url string, body []byte) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "flockmap-"+jobName+"/1.0 (+https://github.com/hwcopeland/flockmap)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return raw, resp.Header.Get("Content-Type"), fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(raw)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return raw, resp.Header.Get("Content-Type"),
			fmt.Errorf("http %d: %s", resp.StatusCode, snippet)
	}
	return raw, resp.Header.Get("Content-Type"), nil
}

// asString coerces an interface{} JSON value to a trimmed string ("" if nil/empty).
func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

// asFloatPtr coerces a JSON number/string amount to *float64, nil if absent/invalid.
func asFloatPtr(v interface{}) *float64 {
	switch t := v.(type) {
	case float64:
		f := t
		return &f
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return &f
		}
	case string:
		s := strings.TrimSpace(strings.NewReplacer("$", "", ",", "").Replace(t))
		if s == "" {
			return nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return &f
		}
	}
	return nil
}

// normalizeDate returns an ISO YYYY-MM-DD string pointer, or nil if unparseable.
// USAspending returns dates already as YYYY-MM-DD, so this mostly validates.
func normalizeDate(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len(s) >= 10 {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			d := s[:10]
			return &d
		}
	}
	return nil
}

// ensureContains appends want to hits if not already present (case-insensitive).
func ensureContains(hits []string, want string) []string {
	for _, h := range hits {
		if strings.EqualFold(h, want) {
			return hits
		}
	}
	return append(hits, want)
}

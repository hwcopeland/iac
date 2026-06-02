// Command camera-spine is the P0 connector for flockmap: it pulls the Flock
// Safety ALPR camera spine from the OpenStreetMap Overpass API, stores each
// verbatim response in the raw-document evidence vault (channel "overpass"),
// parses the elements deterministically, and idempotently UPSERTs them into the
// cameras table keyed on (osm_type, osm_id).
//
// THE FIREWALL (see CONVENTIONS.md §7): no ML on the data path. Every camera
// row carries a source_doc_id pointing at the raw_documents row that holds the
// verbatim Overpass bytes + sha256. Every external fetch goes through
// store.SaveRawDocument FIRST, then the parsed rows thread that id through.
//
// OSM-derived data is ODbL: the raw_documents rows record license="ODbL" and
// the attribution string below.
//
// Queries run (sequentially, politely, with backoff):
//  1. surveillance:type=ALPR + manufacturer="Flock Safety"            (~79k)
//  2. surveillance:type=ALPR + manufacturer:wikidata=Q108485435       (~78k)
//  3. bad-tag sweep: brand="Flock Safety" OR camera:type=ALPR variants
//
// Across all three the same physical node/way/relation can recur; the UPSERT on
// (osm_type, osm_id) dedupes idempotently.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hwcopeland/flockmap/internal/store"
)

const (
	jobName     = "camera-spine"
	channel     = "overpass"
	license     = "ODbL"
	attribution = "© OpenStreetMap contributors, ODbL (https://www.openstreetmap.org/copyright)"

	overpassURL = "https://overpass-api.de/api/interpreter"

	// Overpass server-side timeout (seconds). Must be <= our HTTP client timeout.
	overpassTimeoutSecs = 240
)

// queries are the Overpass spine pulls. Each runs as its own POST so each raw
// response is stored as a distinct, independently-verifiable evidence document.
// "out center tags" gives way/relation a synthesized center lat/lon plus the
// full tag bag.
var queries = []struct {
	name string
	// body is the Overpass QL (without the leading settings line, which is
	// prepended in buildBody).
	body string
}{
	{
		name: "manufacturer-flock-safety",
		body: `nwr["surveillance:type"="ALPR"]["manufacturer"="Flock Safety"];`,
	},
	{
		name: "manufacturer-wikidata",
		body: `nwr["surveillance:type"="ALPR"]["manufacturer:wikidata"="Q108485435"];`,
	},
	{
		// Bad-tag sweep: cameras tagged with the common mis-taggings instead of
		// the canonical manufacturer/surveillance:type pair.
		name: "bad-tag-sweep",
		body: `(
  nwr["brand"="Flock Safety"];
  nwr["brand:wikidata"="Q108485435"];
  nwr["camera:type"="ALPR"]["operator"~"Flock",i];
  nwr["man_made"="surveillance"]["manufacturer"="Flock Safety"];
);`,
	},
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("%s: starting", jobName)

	db, err := store.ConnectPostgres()
	if err != nil {
		log.Fatalf("%s: connect postgres: %v", jobName, err)
	}
	defer db.Close()

	if err := store.EnsureSchema(db); err != nil {
		log.Fatalf("%s: ensure schema: %v", jobName, err)
	}

	client := &http.Client{Timeout: time.Duration(overpassTimeoutSecs+60) * time.Second}

	var totalUpserts, totalElements int
	for i, q := range queries {
		if i > 0 {
			// Be polite to the shared Overpass instance between queries.
			time.Sleep(15 * time.Second)
		}
		up, els, err := runQuery(db, client, q.name, q.body)
		if err != nil {
			// One query failing should not lose the work of the others; log and
			// continue, but make the job fail overall so the CronJob surfaces it.
			log.Printf("%s: query %q FAILED: %v", jobName, q.name, err)
			defer os.Exit(1)
			continue
		}
		totalElements += els
		totalUpserts += up
		log.Printf("%s: query %q -> %d elements, %d cameras upserted", jobName, q.name, els, up)
	}

	log.Printf("%s: done — %d elements parsed, %d camera upserts across %d queries",
		jobName, totalElements, totalUpserts, len(queries))
}

// runQuery fetches one Overpass query, stores the raw response FIRST, then
// parses and upserts the elements. Returns (upserts, elements parsed).
func runQuery(db *sql.DB, client *http.Client, name, qlBody string) (int, int, error) {
	body := buildBody(qlBody)

	raw, contentType, err := fetchOverpass(client, body)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch: %w", err)
	}

	// FIREWALL: store the verbatim evidence FIRST, get the source_doc_id.
	sourceDocID, err := store.SaveRawDocument(db, store.RawDocument{
		SourceURL:    overpassURL,
		HTTPMethod:   http.MethodPost,
		RequestBody:  body,
		ContentType:  contentType,
		RawBytes:     raw,
		Channel:      channel,
		FetchedByJob: jobName,
		License:      license,
		Attribution:  attribution,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("save raw document: %w", err)
	}
	if sourceDocID == "" {
		return 0, 0, fmt.Errorf("save raw document returned empty id")
	}

	resp, err := parseOverpass(raw)
	if err != nil {
		return 0, 0, fmt.Errorf("parse overpass json: %w", err)
	}

	var upserts int
	for _, el := range resp.Elements {
		cam, ok := el.toCamera()
		if !ok {
			// Element without a usable point (e.g. way/relation with no center,
			// or a bare relation member) — skip deterministically.
			continue
		}
		if _, err := UpsertCamera(db, cam, sourceDocID); err != nil {
			return upserts, len(resp.Elements), fmt.Errorf("upsert camera %s/%d: %w", cam.OSMType, cam.OSMID, err)
		}
		upserts++
	}
	return upserts, len(resp.Elements), nil
}

// buildBody prepends the JSON/timeout settings line and wraps the body with the
// "out center tags" finisher, then URL-encodes it as the Overpass 'data=' POST
// body.
func buildBody(qlBody string) string {
	ql := fmt.Sprintf("[out:json][timeout:%d];\n%s\nout center tags;", overpassTimeoutSecs, qlBody)
	return "data=" + url.QueryEscape(ql)
}

// fetchOverpass POSTs the form-encoded query with bounded retry/backoff. Overpass
// returns 429 (too many requests) / 504 (gateway timeout) under load; we back
// off and retry those. Returns the verbatim response bytes and Content-Type.
func fetchOverpass(client *http.Client, body string) ([]byte, string, error) {
	const maxAttempts = 4
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			backoff := time.Duration(attempt*attempt) * 10 * time.Second
			log.Printf("%s: retry %d/%d after %s (%v)", jobName, attempt, maxAttempts, backoff, lastErr)
			time.Sleep(backoff)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(overpassTimeoutSecs+50)*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, overpassURL, strings.NewReader(body))
		if err != nil {
			cancel()
			return nil, "", fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "flockmap-camera-spine/1.0 (+https://github.com/hwcopeland/flockmap)")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("http do: %w", err)
			continue
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		contentType := resp.Header.Get("Content-Type")

		if readErr != nil {
			lastErr = fmt.Errorf("read body: %w", readErr)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			// Overpass sometimes returns 200 with an HTML/error remark when the
			// query is rejected; guard against silently storing garbage.
			if !looksLikeJSON(data) {
				lastErr = fmt.Errorf("status 200 but body is not JSON (%d bytes, ct=%q): %.200s",
					len(data), contentType, data)
				continue
			}
			return data, contentType, nil
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode == http.StatusGatewayTimeout,
			resp.StatusCode == http.StatusServiceUnavailable, resp.StatusCode == http.StatusBadGateway:
			lastErr = fmt.Errorf("retryable status %d: %.200s", resp.StatusCode, data)
			continue
		default:
			return nil, "", fmt.Errorf("non-retryable status %d: %.200s", resp.StatusCode, data)
		}
	}
	return nil, "", fmt.Errorf("exhausted %d attempts: %w", maxAttempts, lastErr)
}

func looksLikeJSON(b []byte) bool {
	t := bytes.TrimSpace(b)
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

// --- Overpass response model ---

type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

type overpassElement struct {
	Type   string            `json:"type"` // "node" | "way" | "relation"
	ID     int64             `json:"id"`
	Lat    *float64          `json:"lat"`    // present on nodes
	Lon    *float64          `json:"lon"`    // present on nodes
	Center *overpassCenter   `json:"center"` // present on way/relation via "out center"
	Tags   map[string]string `json:"tags"`
}

type overpassCenter struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func parseOverpass(raw []byte) (*overpassResponse, error) {
	var r overpassResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// toCamera projects an Overpass element into a Camera. Returns ok=false if the
// element has no usable point (so it cannot become a geometry(Point) row).
func (e overpassElement) toCamera() (Camera, bool) {
	if e.Type != "node" && e.Type != "way" && e.Type != "relation" {
		return Camera{}, false
	}

	var lat, lon float64
	switch {
	case e.Lat != nil && e.Lon != nil:
		lat, lon = *e.Lat, *e.Lon
	case e.Center != nil:
		lat, lon = e.Center.Lat, e.Center.Lon
	default:
		return Camera{}, false
	}

	tags := e.Tags
	if tags == nil {
		tags = map[string]string{}
	}

	return Camera{
		OSMType:      e.Type,
		OSMID:        e.ID,
		Lat:          lat,
		Lon:          lon,
		Operator:     tags["operator"],
		Manufacturer: tags["manufacturer"],
		Direction:    firstNonEmpty(tags["direction"], tags["camera:direction"], tags["camera:mount"]),
		Tags:         tags,
	}, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

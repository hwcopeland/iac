// Package store is the shared persistence layer for flockmap. It owns the
// Postgres connection, schema bootstrap, UUIDv7 generation, and the raw-document
// evidence vault. Every flockmap connector imports this package; it is the
// single contract between components.
//
// Provenance firewall (plan §0): every datum on the ledger traces to a
// raw_documents row holding a raw byte copy + sha256. Connectors MUST call
// SaveRawDocument first and thread the returned id into their data rows as
// source_doc_id. No row without a source_doc_id; no ML on the data path.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	_ "embed"

	_ "github.com/lib/pq"
)

// The canonical SQL lives in db/schema.sql and db/views.sql (the migration Job
// reads those). go:embed cannot traverse "..", so build-synced copies live in
// schema_embed/. Regenerate the copies whenever db/*.sql changes:
//
//go:generate sh -c "cp ../../db/schema.sql schema_embed/schema.sql && cp ../../db/views.sql schema_embed/views.sql"

//go:embed schema_embed/schema.sql
var schemaSQL string

//go:embed schema_embed/views.sql
var viewsSQL string

// --- Connection ---

// ConnectPostgres builds a DSN from POSTGRES_* env vars and opens a connection
// pool. Mirrors the khemeia result-writer: sslmode=disable, in-cluster Postgres.
//
// Env vars (the contract — connectors set these via the deployment manifest):
//
//	POSTGRES_HOST     (required)  e.g. flockmap-postgres.flockmap.svc.cluster.local
//	POSTGRES_PORT     (default 5432)
//	POSTGRES_DB       (default flockmap)
//	POSTGRES_USER     (required)
//	POSTGRES_PASSWORD (required)
func ConnectPostgres() (*sql.DB, error) {
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbname := os.Getenv("POSTGRES_DB")
	if port == "" {
		port = "5432"
	}
	if dbname == "" {
		dbname = "flockmap"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return db, nil
}

// --- Schema ---

// EnsureSchema runs db/schema.sql then db/views.sql (both embedded). Idempotent;
// safe to call on every boot. Splitting on ';' is intentional — the embedded SQL
// uses no procedural blocks, so statement-level splitting is sufficient.
func EnsureSchema(db *sql.DB) error {
	for _, blob := range []struct {
		name string
		sql  string
	}{
		{"schema.sql", schemaSQL},
		{"views.sql", viewsSQL},
	} {
		for _, stmt := range splitStatements(blob.sql) {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("ensuring %s (stmt %.60q...): %w", blob.name, stmt, err)
			}
		}
	}
	return nil
}

// splitStatements splits a SQL blob into individual statements on ';',
// dropping blank statements and full-line '--' comments.
func splitStatements(blob string) []string {
	var out []string
	for _, raw := range strings.Split(blob, ";") {
		var lines []string
		for _, ln := range strings.Split(raw, "\n") {
			if strings.HasPrefix(strings.TrimSpace(ln), "--") {
				continue
			}
			lines = append(lines, ln)
		}
		stmt := strings.TrimSpace(strings.Join(lines, "\n"))
		if stmt != "" {
			out = append(out, stmt)
		}
	}
	return out
}

// --- UUID v7 (lifted from khemeia api/provenance.go) ---

// NewUUIDv7 generates a UUID v7 (RFC 9562): time-ordered, 48-bit ms timestamp,
// version 7, variant 10, the rest cryptographically random. No dependency.
func NewUUIDv7() string {
	var u [16]byte

	ms := uint64(time.Now().UnixMilli())
	binary.BigEndian.PutUint16(u[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(u[2:6], uint32(ms))

	if _, err := rand.Read(u[6:]); err != nil {
		panic(fmt.Sprintf("store: failed to read random bytes: %v", err))
	}

	u[6] = (u[6] & 0x0F) | 0x70 // version 7
	u[8] = (u[8] & 0x3F) | 0x80 // variant 10

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(u[0:4]),
		binary.BigEndian.Uint16(u[4:6]),
		binary.BigEndian.Uint16(u[6:8]),
		binary.BigEndian.Uint16(u[8:10]),
		u[10:16],
	)
}

// --- Raw-document evidence vault ---

// RawDocument is the input to SaveRawDocument. SHA-256, id, byte size, and
// retrieved_at are computed by the store — callers do not set them.
type RawDocument struct {
	SourceURL    string // the fetched URL (or endpoint)
	HTTPMethod   string // "GET" / "POST"; defaults to "GET" if empty
	RequestBody  string // e.g. Overpass 'data=...' POST body; "" if none
	ContentType  string // response Content-Type; "" if unknown
	RawBytes     []byte // the verbatim response bytes (the evidence)
	Channel      string // "overpass" | "usaspending" | "foia" | ...
	FetchedByJob string // ingest-job identity (allowlist)
	License      string // optional, e.g. "ODbL" for OSM-derived
	Attribution  string // optional ODbL attribution string
}

// SaveRawDocument computes the SHA-256 of doc.RawBytes, dedupes on the sha256
// UNIQUE constraint, and stores the bytes inline (bytea) for now.
//
// TODO(garage): once Garage S3 (flockmap-raw / flockmap-foia) is wired, upload
// RawBytes there, set storage_uri, and stop storing raw_bytes inline. The
// sha256 dedupe and the returned id contract stay the same.
//
// Returns the raw_documents.id (a UUIDv7). On a content-hash collision (the
// same bytes already stored) it is idempotent: returns the existing row's id.
func SaveRawDocument(db *sql.DB, doc RawDocument) (string, error) {
	if doc.Channel == "" {
		return "", fmt.Errorf("SaveRawDocument: channel is required")
	}
	if doc.FetchedByJob == "" {
		return "", fmt.Errorf("SaveRawDocument: fetched_by_job is required")
	}
	method := doc.HTTPMethod
	if method == "" {
		method = "GET"
	}

	sum := sha256.Sum256(doc.RawBytes)
	checksum := hex.EncodeToString(sum[:])

	id := NewUUIDv7()
	err := db.QueryRow(
		`INSERT INTO raw_documents
		    (id, source_url, http_method, request_body, content_type,
		     checksum_sha256, raw_bytes, byte_size, channel, license,
		     attribution, fetched_by_job)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT (checksum_sha256) DO NOTHING
		 RETURNING id`,
		id, doc.SourceURL, method, nullable(doc.RequestBody), nullable(doc.ContentType),
		checksum, doc.RawBytes, len(doc.RawBytes), doc.Channel, nullable(doc.License),
		nullable(doc.Attribution), doc.FetchedByJob,
	).Scan(&id)

	if err == sql.ErrNoRows {
		// Content already stored — fetch the existing id (idempotent).
		if err := db.QueryRow(
			`SELECT id FROM raw_documents WHERE checksum_sha256 = $1`, checksum,
		).Scan(&id); err != nil {
			return "", fmt.Errorf("SaveRawDocument: resolving existing id for sha256 %s: %w", checksum, err)
		}
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("SaveRawDocument: inserting raw_document (channel=%s): %w", doc.Channel, err)
	}
	return id, nil
}

// nullable maps "" to a NULL bytes value so empty optionals are stored as NULL.
func nullable(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// --- Upsert helper contract (extended by connectors in their OWN files) ---
//
// The store deliberately ships ONLY ConnectPostgres / EnsureSchema / NewUUIDv7 /
// SaveRawDocument. Each connector adds its own thin upsert helper IN ITS OWN
// cmd/<name>/ package (NOT in this file). Every helper MUST take a non-empty
// sourceDocID returned by SaveRawDocument and write it as source_doc_id.
//
// Recommended signatures for downstream agents to implement themselves:
//
//	// camera-spine (P0)
//	UpsertCamera(db *sql.DB, c Camera, sourceDocID string) (id string, err error)
//	    // ON CONFLICT (osm_type, osm_id) DO UPDATE last_seen_at, tags, geom.
//
//	// geocode (P1)
//	UpsertJurisdiction(db *sql.DB, j Jurisdiction, sourceDocID string) (id string, err error)
//	    // ON CONFLICT (geoid, level) DO UPDATE geom/name.
//	SetCameraGeoids(db *sql.DB, cameraID, placeGeoid, countyGeoid, stateFips string) error
//
//	// agency-resolve (P2)
//	UpsertAgency(db *sql.DB, a Agency, sourceDocID string) (id string, err error)
//	UpsertAlias(db *sql.DB, alias string, agencyID, sourceDocID string) (id string, err error)
//	SetCameraAgency(db *sql.DB, cameraID, agencyID string, confidence float64) error
//
//	// funding connectors (P3/P7) — extraction MUST be one of
//	// 'deterministic' | 'human_verified' | 'ml_unreviewed'; ml_unreviewed
//	// REQUIRES a non-nil confidence. ON CONFLICT (channel, external_id) DO NOTHING.
//	UpsertFundingRecord(db *sql.DB, f FundingRecord, sourceDocID string) (id string, err error)
//
//	// match (P5)
//	UpsertCameraFundingMatch(db *sql.DB, cameraID, fundingRecordID, matchBasis string, confidence float64) (id string, err error)
//
//	// FOIA (P8)
//	UpsertSourceRegistry(db *sql.DB, r SourceRegistryRow, sourceDocID string) (id string, err error)  // NEVER a money field
//	UpsertFOIARequest(db *sql.DB, fr FOIARequest, sourceDocID string) (id string, err error)
//
// All helpers: generate the PK with store.NewUUIDv7(), use $N placeholders,
// and never touch go.mod / internal/ / db/.

// Package main provides the provenance system for tracking artifact lineage.
// Every computational artifact (receptor, library, docked pose, etc.) gets a
// provenance record when created. Parent-child edges form a DAG that supports
// ancestor/descendant traversal via recursive CTEs.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// --- UUID v7 generation ---

// newUUIDv7 generates a UUID v7 (RFC 9562) — a time-ordered UUID with 48-bit
// millisecond timestamp, 4-bit version, 12-bit random, 2-bit variant, and
// 62-bit random. No external dependency required.
func newUUIDv7() string {
	var u [16]byte

	// 48-bit Unix timestamp in milliseconds (big-endian).
	ms := uint64(time.Now().UnixMilli())
	binary.BigEndian.PutUint16(u[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(u[2:6], uint32(ms))

	// Fill the remaining 10 bytes with cryptographic randomness.
	if _, err := rand.Read(u[6:]); err != nil {
		panic(fmt.Sprintf("provenance: failed to read random bytes: %v", err))
	}

	// Set version 7 (0b0111 in bits 48-51).
	u[6] = (u[6] & 0x0F) | 0x70

	// Set variant 10 (RFC 9562) in bits 64-65.
	u[8] = (u[8] & 0x3F) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(u[0:4]),
		binary.BigEndian.Uint16(u[4:6]),
		binary.BigEndian.Uint16(u[6:8]),
		binary.BigEndian.Uint16(u[8:10]),
		u[10:16],
	)
}

// --- Schema ---

// EnsureProvenanceSchema creates the provenance and provenance_edges tables.
// Called once during startup on the shared database, following the same
// pattern as EnsureAPITokenSchema and EnsureBasisSetSchema.
func EnsureProvenanceSchema(db *sql.DB) error {
	provenanceDDL := `CREATE TABLE IF NOT EXISTS provenance (
		id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		artifact_id     CHAR(36) NOT NULL,
		artifact_type   ENUM(
			'receptor', 'library', 'compound', 'docked_pose', 'refined_pose',
			'admet_result', 'generated_compound', 'linked_compound', 'report',
			'selectivity_matrix', 'fep_result'
		) NOT NULL,
		s3_bucket       VARCHAR(64) NULL,
		s3_key          VARCHAR(512) NULL,
		checksum_sha256 CHAR(64) NULL,
		created_by_job  VARCHAR(255) NOT NULL,
		job_kind        VARCHAR(64) NULL,
		job_namespace   VARCHAR(64) NOT NULL DEFAULT 'chem',
		parameters      JSON NULL,
		tool_versions   JSON NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

		UNIQUE KEY uq_artifact_id (artifact_id),
		INDEX idx_artifact_type (artifact_type),
		INDEX idx_created_by_job (created_by_job),
		INDEX idx_created_at (created_at),
		INDEX idx_s3_key (s3_bucket, s3_key)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(provenanceDDL); err != nil {
		return fmt.Errorf("creating provenance table: %w", err)
	}

	edgesDDL := `CREATE TABLE IF NOT EXISTS provenance_edges (
		id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		parent_id       CHAR(36) NOT NULL,
		child_id        CHAR(36) NOT NULL,

		UNIQUE KEY uq_edge (parent_id, child_id),
		INDEX idx_parent (parent_id),
		INDEX idx_child (child_id),
		CONSTRAINT fk_parent FOREIGN KEY (parent_id) REFERENCES provenance(artifact_id),
		CONSTRAINT fk_child FOREIGN KEY (child_id) REFERENCES provenance(artifact_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(edgesDDL); err != nil {
		return fmt.Errorf("creating provenance_edges table: %w", err)
	}

	return nil
}

// --- Go types ---

// ProvenanceRecord represents a single provenance entry.
type ProvenanceRecord struct {
	ID             uint64          `json:"id"`
	ArtifactID     string          `json:"artifact_id"`
	ArtifactType   string          `json:"artifact_type"`
	S3Bucket       *string         `json:"s3_bucket,omitempty"`
	S3Key          *string         `json:"s3_key,omitempty"`
	ChecksumSHA256 *string         `json:"checksum_sha256,omitempty"`
	CreatedByJob   string          `json:"created_by_job"`
	JobKind        *string         `json:"job_kind,omitempty"`
	JobNamespace   string          `json:"job_namespace"`
	Parameters     json.RawMessage `json:"parameters,omitempty"`
	ToolVersions   json.RawMessage `json:"tool_versions,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	Depth          *int            `json:"depth,omitempty"` // populated by ancestor/descendant queries
}

// ProvenanceEdge represents a parent-child relationship between artifacts.
type ProvenanceEdge struct {
	ID       uint64 `json:"id"`
	ParentID string `json:"parent_id"`
	ChildID  string `json:"child_id"`
}

// --- Data functions ---

// RecordProvenance inserts a provenance record and optional parent edges in a
// single transaction. If record.ArtifactID is empty, a UUID v7 is generated
// and assigned to the record.
func RecordProvenance(ctx context.Context, db *sql.DB, record *ProvenanceRecord, parentIDs []string) error {
	if record.ArtifactID == "" {
		record.ArtifactID = newUUIDv7()
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning provenance transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Insert the provenance record.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO provenance
			(artifact_id, artifact_type, s3_bucket, s3_key, checksum_sha256,
			 created_by_job, job_kind, job_namespace, parameters, tool_versions)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ArtifactID, record.ArtifactType, record.S3Bucket, record.S3Key,
		record.ChecksumSHA256, record.CreatedByJob, record.JobKind,
		record.JobNamespace, record.Parameters, record.ToolVersions,
	)
	if err != nil {
		return fmt.Errorf("inserting provenance record: %w", err)
	}

	// Insert parent edges.
	if len(parentIDs) > 0 {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO provenance_edges (parent_id, child_id) VALUES (?, ?)`)
		if err != nil {
			return fmt.Errorf("preparing edge insert: %w", err)
		}
		defer stmt.Close()

		for _, parentID := range parentIDs {
			if _, err := stmt.ExecContext(ctx, parentID, record.ArtifactID); err != nil {
				return fmt.Errorf("inserting edge from %s to %s: %w", parentID, record.ArtifactID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing provenance transaction: %w", err)
	}
	return nil
}

// GetProvenance retrieves a single provenance record by artifact ID.
func GetProvenance(ctx context.Context, db *sql.DB, artifactID string) (*ProvenanceRecord, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, artifact_id, artifact_type, s3_bucket, s3_key, checksum_sha256,
		        created_by_job, job_kind, job_namespace, parameters, tool_versions, created_at
		 FROM provenance WHERE artifact_id = ?`, artifactID)

	var r ProvenanceRecord
	err := row.Scan(
		&r.ID, &r.ArtifactID, &r.ArtifactType, &r.S3Bucket, &r.S3Key,
		&r.ChecksumSHA256, &r.CreatedByJob, &r.JobKind, &r.JobNamespace,
		&r.Parameters, &r.ToolVersions, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying provenance for %s: %w", artifactID, err)
	}
	return &r, nil
}

// GetAncestors returns all ancestor artifacts of the given artifact ID using a
// recursive CTE that walks parent edges upstream. maxDepth limits traversal
// depth (capped at 50).
func GetAncestors(ctx context.Context, db *sql.DB, artifactID string, maxDepth int) ([]ProvenanceRecord, error) {
	if maxDepth <= 0 || maxDepth > 50 {
		maxDepth = 50
	}

	query := `
		WITH RECURSIVE ancestors AS (
			SELECT p.id, p.artifact_id, p.artifact_type, p.s3_bucket, p.s3_key,
			       p.checksum_sha256, p.created_by_job, p.job_kind, p.job_namespace,
			       p.parameters, p.tool_versions, p.created_at, 0 AS depth
			FROM provenance p
			WHERE p.artifact_id = ?

			UNION ALL

			SELECT p.id, p.artifact_id, p.artifact_type, p.s3_bucket, p.s3_key,
			       p.checksum_sha256, p.created_by_job, p.job_kind, p.job_namespace,
			       p.parameters, p.tool_versions, p.created_at, a.depth + 1
			FROM ancestors a
			JOIN provenance_edges e ON e.child_id = a.artifact_id
			JOIN provenance p ON p.artifact_id = e.parent_id
			WHERE a.depth < ?
		)
		SELECT id, artifact_id, artifact_type, s3_bucket, s3_key, checksum_sha256,
		       created_by_job, job_kind, job_namespace, parameters, tool_versions,
		       created_at, depth
		FROM ancestors
		ORDER BY depth ASC`

	return queryProvenanceWithDepth(ctx, db, query, artifactID, maxDepth)
}

// GetDescendants returns all descendant artifacts of the given artifact ID
// using a recursive CTE that walks child edges downstream. maxDepth limits
// traversal depth (capped at 50).
func GetDescendants(ctx context.Context, db *sql.DB, artifactID string, maxDepth int) ([]ProvenanceRecord, error) {
	if maxDepth <= 0 || maxDepth > 50 {
		maxDepth = 50
	}

	query := `
		WITH RECURSIVE descendants AS (
			SELECT p.id, p.artifact_id, p.artifact_type, p.s3_bucket, p.s3_key,
			       p.checksum_sha256, p.created_by_job, p.job_kind, p.job_namespace,
			       p.parameters, p.tool_versions, p.created_at, 0 AS depth
			FROM provenance p
			WHERE p.artifact_id = ?

			UNION ALL

			SELECT p.id, p.artifact_id, p.artifact_type, p.s3_bucket, p.s3_key,
			       p.checksum_sha256, p.created_by_job, p.job_kind, p.job_namespace,
			       p.parameters, p.tool_versions, p.created_at, d.depth + 1
			FROM descendants d
			JOIN provenance_edges e ON e.parent_id = d.artifact_id
			JOIN provenance p ON p.artifact_id = e.child_id
			WHERE d.depth < ?
		)
		SELECT id, artifact_id, artifact_type, s3_bucket, s3_key, checksum_sha256,
		       created_by_job, job_kind, job_namespace, parameters, tool_versions,
		       created_at, depth
		FROM descendants
		ORDER BY depth ASC`

	return queryProvenanceWithDepth(ctx, db, query, artifactID, maxDepth)
}

// GetJobArtifacts returns all provenance records created by a specific job.
func GetJobArtifacts(ctx context.Context, db *sql.DB, jobName string) ([]ProvenanceRecord, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, artifact_id, artifact_type, s3_bucket, s3_key, checksum_sha256,
		        created_by_job, job_kind, job_namespace, parameters, tool_versions, created_at
		 FROM provenance WHERE created_by_job = ? ORDER BY created_at ASC`, jobName)
	if err != nil {
		return nil, fmt.Errorf("querying provenance for job %s: %w", jobName, err)
	}
	defer rows.Close()

	var records []ProvenanceRecord
	for rows.Next() {
		var r ProvenanceRecord
		if err := rows.Scan(
			&r.ID, &r.ArtifactID, &r.ArtifactType, &r.S3Bucket, &r.S3Key,
			&r.ChecksumSHA256, &r.CreatedByJob, &r.JobKind, &r.JobNamespace,
			&r.Parameters, &r.ToolVersions, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning provenance row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating provenance rows: %w", err)
	}
	if records == nil {
		records = []ProvenanceRecord{}
	}
	return records, nil
}

// queryProvenanceWithDepth executes a recursive CTE query that returns
// provenance records with a depth column.
func queryProvenanceWithDepth(ctx context.Context, db *sql.DB, query, artifactID string, maxDepth int) ([]ProvenanceRecord, error) {
	rows, err := db.QueryContext(ctx, query, artifactID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("querying provenance graph for %s: %w", artifactID, err)
	}
	defer rows.Close()

	var records []ProvenanceRecord
	for rows.Next() {
		var r ProvenanceRecord
		var depth int
		if err := rows.Scan(
			&r.ID, &r.ArtifactID, &r.ArtifactType, &r.S3Bucket, &r.S3Key,
			&r.ChecksumSHA256, &r.CreatedByJob, &r.JobKind, &r.JobNamespace,
			&r.Parameters, &r.ToolVersions, &r.CreatedAt, &depth,
		); err != nil {
			return nil, fmt.Errorf("scanning provenance row: %w", err)
		}
		r.Depth = &depth
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating provenance rows: %w", err)
	}
	if records == nil {
		records = []ProvenanceRecord{}
	}
	return records, nil
}

// --- HTTP handlers ---

// provenanceCreateRequest is the JSON body for POST /api/v1/provenance/record.
type provenanceCreateRequest struct {
	ArtifactType   string          `json:"artifact_type"`
	S3Bucket       *string         `json:"s3_bucket,omitempty"`
	S3Key          *string         `json:"s3_key,omitempty"`
	ChecksumSHA256 *string         `json:"checksum_sha256,omitempty"`
	CreatedByJob   string          `json:"created_by_job"`
	JobKind        *string         `json:"job_kind,omitempty"`
	JobNamespace   string          `json:"job_namespace,omitempty"`
	Parameters     json.RawMessage `json:"parameters,omitempty"`
	ToolVersions   json.RawMessage `json:"tool_versions,omitempty"`
	ParentIDs      []string        `json:"parent_ids,omitempty"`
}

// validArtifactTypes enumerates the allowed artifact_type values.
var validArtifactTypes = map[string]bool{
	"receptor":           true,
	"library":            true,
	"compound":           true,
	"docked_pose":        true,
	"refined_pose":       true,
	"admet_result":       true,
	"generated_compound": true,
	"linked_compound":    true,
	"report":             true,
	"selectivity_matrix": true,
	"fep_result":         true,
}

// HandleGetProvenance handles GET /api/v1/provenance/{artifactId}.
func (h *APIHandler) HandleGetProvenance(w http.ResponseWriter, r *http.Request) {
	artifactID := strings.TrimPrefix(r.URL.Path, "/api/v1/provenance/")
	artifactID = strings.TrimRight(artifactID, "/")
	if artifactID == "" {
		writeError(w, "artifact_id is required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	record, err := GetProvenance(r.Context(), db, artifactID)
	if err != nil {
		log.Printf("[provenance] GetProvenance error: %v", err)
		writeError(w, "failed to query provenance", http.StatusInternalServerError)
		return
	}
	if record == nil {
		writeError(w, "artifact not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}

// HandleGetAncestors handles GET /api/v1/provenance/{artifactId}/ancestors.
func (h *APIHandler) HandleGetAncestors(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/provenance/{id}/ancestors
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/provenance/")
	path = strings.TrimRight(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, "artifact_id is required", http.StatusBadRequest)
		return
	}
	artifactID := parts[0]

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	records, err := GetAncestors(r.Context(), db, artifactID, 50)
	if err != nil {
		log.Printf("[provenance] GetAncestors error: %v", err)
		writeError(w, "failed to query ancestors", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"artifact_id": artifactID,
		"ancestors":   records,
		"count":       len(records),
	})
}

// HandleGetDescendants handles GET /api/v1/provenance/{artifactId}/descendants.
func (h *APIHandler) HandleGetDescendants(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/provenance/")
	path = strings.TrimRight(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, "artifact_id is required", http.StatusBadRequest)
		return
	}
	artifactID := parts[0]

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	records, err := GetDescendants(r.Context(), db, artifactID, 50)
	if err != nil {
		log.Printf("[provenance] GetDescendants error: %v", err)
		writeError(w, "failed to query descendants", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"artifact_id": artifactID,
		"descendants": records,
		"count":       len(records),
	})
}

// HandleGetJobArtifacts handles GET /api/v1/provenance/job/{jobName}.
func (h *APIHandler) HandleGetJobArtifacts(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/provenance/job/")
	jobName = strings.TrimRight(jobName, "/")
	if jobName == "" {
		writeError(w, "job name is required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	records, err := GetJobArtifacts(r.Context(), db, jobName)
	if err != nil {
		log.Printf("[provenance] GetJobArtifacts error: %v", err)
		writeError(w, "failed to query job artifacts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_name":  jobName,
		"artifacts": records,
		"count":     len(records),
	})
}

// HandleCreateProvenance handles POST /api/v1/provenance/record.
func (h *APIHandler) HandleCreateProvenance(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var req provenanceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields.
	if req.ArtifactType == "" {
		writeError(w, "artifact_type is required", http.StatusBadRequest)
		return
	}
	if !validArtifactTypes[req.ArtifactType] {
		writeError(w, fmt.Sprintf("invalid artifact_type: %s", req.ArtifactType), http.StatusBadRequest)
		return
	}
	if req.CreatedByJob == "" {
		writeError(w, "created_by_job is required", http.StatusBadRequest)
		return
	}

	// Default job namespace to "chem".
	jobNamespace := req.JobNamespace
	if jobNamespace == "" {
		jobNamespace = "chem"
	}

	record := &ProvenanceRecord{
		ArtifactType:   req.ArtifactType,
		S3Bucket:       req.S3Bucket,
		S3Key:          req.S3Key,
		ChecksumSHA256: req.ChecksumSHA256,
		CreatedByJob:   req.CreatedByJob,
		JobKind:        req.JobKind,
		JobNamespace:   jobNamespace,
		Parameters:     req.Parameters,
		ToolVersions:   req.ToolVersions,
	}

	if err := RecordProvenance(r.Context(), db, record, req.ParentIDs); err != nil {
		log.Printf("[provenance] RecordProvenance error: %v", err)
		writeError(w, fmt.Sprintf("failed to record provenance: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"artifact_id": record.ArtifactID,
		"created_at":  record.CreatedAt,
	})
}

// provenanceDispatch routes /api/v1/provenance/ requests based on path structure.
// It distinguishes between:
//   - /api/v1/provenance/record         -> POST: create
//   - /api/v1/provenance/job/{name}     -> GET: list by job
//   - /api/v1/provenance/{id}           -> GET: single record
//   - /api/v1/provenance/{id}/ancestors -> GET: ancestor chain
//   - /api/v1/provenance/{id}/descendants -> GET: descendant chain
func (h *APIHandler) provenanceDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/provenance/")
	path = strings.TrimRight(path, "/")

	// POST /api/v1/provenance/record
	if r.Method == http.MethodPost && path == "record" {
		h.HandleCreateProvenance(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// GET /api/v1/provenance/job/{name}
	if strings.HasPrefix(path, "job/") {
		h.HandleGetJobArtifacts(w, r)
		return
	}

	// Check for sub-resources: {id}/ancestors or {id}/descendants
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 2 {
		switch parts[1] {
		case "ancestors":
			h.HandleGetAncestors(w, r)
			return
		case "descendants":
			h.HandleGetDescendants(w, r)
			return
		default:
			writeError(w, "not found", http.StatusNotFound)
			return
		}
	}

	// GET /api/v1/provenance/{id}
	h.HandleGetProvenance(w, r)
}

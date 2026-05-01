// Package main provides HTTP handlers for WP-2 library preparation endpoints.
// These handlers manage the lifecycle of compound library ingestion,
// standardization, filtering, and 3D conformer generation. Multiple input
// sources are supported: SMILES lists, SDF data, ChEMBL queries, and Enamine
// REAL subset files.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// --- Service URL for library-prep sidecar ---

// libraryPrepServiceURL is the base URL of the library-prep sidecar.
var libraryPrepServiceURL = getLibraryPrepURL()

func getLibraryPrepURL() string {
	if url := os.Getenv("LIBRARY_PREP_SERVICE_URL"); url != "" {
		return url
	}
	return "http://library-prep.chem.svc.cluster.local"
}

// --- Request/response types ---

// LibraryPrepFilters controls which pre-filters are applied during library prep.
// Each filter is independently toggleable; nil means use the server default.
type LibraryPrepFilters struct {
	Lipinski *bool `json:"lipinski,omitempty"`
	Veber    *bool `json:"veber,omitempty"`
	PAINS    *bool `json:"pains,omitempty"`
	Brenk    *bool `json:"brenk,omitempty"`
	REOS     *bool `json:"reos,omitempty"`
}

// resolveDefaults fills in default values for unset filters.
// Defaults: Lipinski=ON, Veber=ON, PAINS=ON, Brenk=OFF, REOS=OFF.
func (f *LibraryPrepFilters) resolveDefaults() {
	t := true
	fa := false
	if f.Lipinski == nil {
		f.Lipinski = &t
	}
	if f.Veber == nil {
		f.Veber = &t
	}
	if f.PAINS == nil {
		f.PAINS = &t
	}
	if f.Brenk == nil {
		f.Brenk = &fa
	}
	if f.REOS == nil {
		f.REOS = &fa
	}
}

// ChEMBLSourceParams holds ChEMBL-specific query parameters for library prep.
type ChEMBLSourceParams struct {
	Q        string   `json:"q,omitempty"`
	MWMin    *float64 `json:"mw_min,omitempty"`
	MWMax    *float64 `json:"mw_max,omitempty"`
	LogPMin  *float64 `json:"logp_min,omitempty"`
	LogPMax  *float64 `json:"logp_max,omitempty"`
	HBAMax   *int     `json:"hba_max,omitempty"`
	HBDMax   *int     `json:"hbd_max,omitempty"`
	MaxPhase *float64 `json:"max_phase,omitempty"`
	Ro5      bool     `json:"ro5,omitempty"`
}

// toParamMap converts ChEMBLSourceParams into the flat string map consumed by
// buildChEMBLFilterClauses (defined in handlers_chembl.go).
func (p *ChEMBLSourceParams) toParamMap() map[string]string {
	params := make(map[string]string)
	if p.Q != "" {
		params["q"] = p.Q
	}
	if p.MWMin != nil {
		params["mw_min"] = strconv.FormatFloat(*p.MWMin, 'f', -1, 64)
	}
	if p.MWMax != nil {
		params["mw_max"] = strconv.FormatFloat(*p.MWMax, 'f', -1, 64)
	}
	if p.LogPMin != nil {
		params["logp_min"] = strconv.FormatFloat(*p.LogPMin, 'f', -1, 64)
	}
	if p.LogPMax != nil {
		params["logp_max"] = strconv.FormatFloat(*p.LogPMax, 'f', -1, 64)
	}
	if p.HBAMax != nil {
		params["hba_max"] = strconv.Itoa(*p.HBAMax)
	}
	if p.HBDMax != nil {
		params["hbd_max"] = strconv.Itoa(*p.HBDMax)
	}
	if p.MaxPhase != nil {
		params["max_phase"] = strconv.FormatFloat(*p.MaxPhase, 'f', -1, 64)
	}
	if p.Ro5 {
		params["ro5"] = "true"
	}
	return params
}

// LibraryPrepRequest is the JSON body for POST /api/v1/libraries/prepare.
type LibraryPrepRequest struct {
	Source     string              `json:"source"`               // smiles, sdf, chembl, enamine
	Name       string             `json:"name"`                 // human-readable library name
	SMILESList []string           `json:"smiles_list,omitempty"` // source=smiles
	SDFData    string             `json:"sdf_data,omitempty"`   // source=sdf (inline SDF)
	S3Ref      string             `json:"s3_ref,omitempty"`     // source=sdf (S3 key)
	ChEMBL     *ChEMBLSourceParams `json:"chembl,omitempty"`    // source=chembl
	Filters    LibraryPrepFilters `json:"filters"`
}

// LibraryPrepResponse is the 202 Accepted response for a new library prep job.
type LibraryPrepResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// LibraryPrepStatus is the response for GET /api/v1/libraries/{name}.
type LibraryPrepStatus struct {
	Name           string  `json:"name"`
	Source         string  `json:"source"`
	Phase          string  `json:"phase"`
	CompoundCount  int     `json:"compound_count"`
	FilteredCount  int     `json:"filtered_count"`
	S3Key          *string `json:"s3_key,omitempty"`
	CreatedAt      string  `json:"created_at"`
	CompletedAt    *string `json:"completed_at,omitempty"`
	ErrorOutput    *string `json:"error_output,omitempty"`
}

// LibraryCompound represents a single compound in a prepared library.
type LibraryCompound struct {
	CompoundID     string   `json:"compound_id"`
	CanonicalSMILES string  `json:"canonical_smiles"`
	InChIKey       string   `json:"inchikey"`
	MW             *float64 `json:"mw,omitempty"`
	LogP           *float64 `json:"logp,omitempty"`
	HBA            *int     `json:"hba,omitempty"`
	HBD            *int     `json:"hbd,omitempty"`
	PSA            *float64 `json:"psa,omitempty"`
	RotatableBonds *int     `json:"rotatable_bonds,omitempty"`
	QED            *float64 `json:"qed,omitempty"`
	LipinskiPass   *bool   `json:"lipinski_pass,omitempty"`
	VeberPass      *bool   `json:"veber_pass,omitempty"`
	PAINSPass      *bool   `json:"pains_pass,omitempty"`
	BrenkPass      *bool   `json:"brenk_pass,omitempty"`
	REOSPass       *bool   `json:"reos_pass,omitempty"`
	Filtered       bool     `json:"filtered"`
	S3ConformerKey *string  `json:"s3_conformer_key,omitempty"`
}

// --- Validation ---

// validLibrarySources enumerates the allowed source values.
var validLibrarySources = map[string]bool{
	"smiles":  true,
	"sdf":     true,
	"chembl":  true,
	"enamine": true,
}

// --- MySQL schema ---

// EnsureLibraryPrepSchema creates the library_prep_results and library_compounds
// tables if they do not exist. Called during startup on the shared database,
// following the same pattern as EnsureTargetPrepSchema.
func EnsureLibraryPrepSchema(db *sql.DB) error {
	resultsDDL := `CREATE TABLE IF NOT EXISTS library_prep_results (
		id              INT AUTO_INCREMENT PRIMARY KEY,
		name            VARCHAR(255) NOT NULL UNIQUE,
		source          ENUM('smiles', 'sdf', 'chembl', 'enamine') NOT NULL,
		phase           ENUM('Pending', 'Running', 'Succeeded', 'Failed') NOT NULL DEFAULT 'Pending',
		compound_count  INT NOT NULL DEFAULT 0,
		filtered_count  INT NOT NULL DEFAULT 0,
		s3_key          VARCHAR(512) NULL,
		filters         JSON NULL,
		request_params  JSON NULL,
		error_output    TEXT NULL,
		start_time      TIMESTAMP NULL,
		completion_time TIMESTAMP NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_phase (phase),
		INDEX idx_source (source),
		INDEX idx_created_at (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(resultsDDL); err != nil {
		return fmt.Errorf("creating library_prep_results table: %w", err)
	}

	compoundsDDL := `CREATE TABLE IF NOT EXISTS library_compounds (
		id               INT AUTO_INCREMENT PRIMARY KEY,
		library_id       INT NOT NULL,
		compound_id      VARCHAR(64) NOT NULL,
		canonical_smiles TEXT NOT NULL,
		inchikey         VARCHAR(27) NOT NULL,
		mw               FLOAT NULL,
		logp             FLOAT NULL,
		hba              INT NULL,
		hbd              INT NULL,
		psa              FLOAT NULL,
		rotatable_bonds  INT NULL,
		qed              FLOAT NULL,
		lipinski_pass    TINYINT(1) NULL,
		veber_pass       TINYINT(1) NULL,
		pains_pass       TINYINT(1) NULL,
		brenk_pass       TINYINT(1) NULL,
		reos_pass        TINYINT(1) NULL,
		filtered         TINYINT(1) NOT NULL DEFAULT 0,
		s3_conformer_key VARCHAR(512) NULL,
		created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_library_id (library_id),
		INDEX idx_compound_id (compound_id),
		INDEX idx_inchikey (inchikey),
		INDEX idx_filtered (filtered),
		CONSTRAINT fk_library FOREIGN KEY (library_id) REFERENCES library_prep_results(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(compoundsDDL); err != nil {
		return fmt.Errorf("creating library_compounds table: %w", err)
	}

	return nil
}

// --- HTTP handlers ---

// LibraryPrepareHandler handles POST /api/v1/libraries/prepare.
// Validates input, creates a library prep record in MySQL, and starts an async
// goroutine to orchestrate the preparation pipeline.
func (h *APIHandler) LibraryPrepareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LibraryPrepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate source type.
	if !validLibrarySources[req.Source] {
		writeError(w, fmt.Sprintf("invalid source: %q (must be one of: smiles, sdf, chembl, enamine)", req.Source), http.StatusBadRequest)
		return
	}

	// Validate name.
	if req.Name == "" {
		writeError(w, "name is required", http.StatusBadRequest)
		return
	}

	// Source-specific validation.
	switch req.Source {
	case "smiles":
		if len(req.SMILESList) == 0 {
			writeError(w, "smiles_list is required for source=smiles", http.StatusBadRequest)
			return
		}
	case "sdf":
		if req.SDFData == "" && req.S3Ref == "" {
			writeError(w, "sdf_data or s3_ref is required for source=sdf", http.StatusBadRequest)
			return
		}
	case "chembl":
		if req.ChEMBL == nil {
			writeError(w, "chembl parameters are required for source=chembl", http.StatusBadRequest)
			return
		}
		if h.chemblDB == nil {
			writeError(w, "ChEMBL database not available", http.StatusServiceUnavailable)
			return
		}
	case "enamine":
		if len(req.SMILESList) == 0 && req.S3Ref == "" {
			writeError(w, "smiles_list or s3_ref is required for source=enamine", http.StatusBadRequest)
			return
		}
	}

	// Resolve filter defaults.
	req.Filters.resolveDefaults()

	// Generate job name.
	jobName := fmt.Sprintf("libprep-%s-%d", sanitizeName(req.Name), time.Now().UnixNano())

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Serialize JSON fields.
	filtersJSON, _ := json.Marshal(req.Filters)
	reqParamsJSON, _ := json.Marshal(req)

	// Insert the library prep record.
	_, err := db.ExecContext(r.Context(),
		`INSERT INTO library_prep_results
			(name, source, filters, request_params)
		 VALUES (?, ?, ?, ?)`,
		jobName, req.Source, string(filtersJSON), string(reqParamsJSON))
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create library prep job: %v", err), http.StatusInternalServerError)
		return
	}

	// Return 202 Accepted.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(LibraryPrepResponse{
		Name:   jobName,
		Status: "Pending",
	})

	// Start async pipeline.
	go h.runLibraryPrepPipeline(jobName, req)
}

// LibraryGetHandler handles GET /api/v1/libraries/{name}.
// Returns the library prep status, compound count, and filter stats.
func (h *APIHandler) LibraryGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := extractLibraryName(r.URL.Path)
	if name == "" || name == "prepare" {
		writeError(w, "library name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var status LibraryPrepStatus
	var s3Key, errorOutput sql.NullString
	var completionTime sql.NullTime
	var createdAt time.Time

	err := db.QueryRowContext(r.Context(),
		`SELECT name, source, phase, compound_count, filtered_count,
			s3_key, error_output, completion_time, created_at
		 FROM library_prep_results WHERE name = ?`, name).Scan(
		&status.Name, &status.Source, &status.Phase,
		&status.CompoundCount, &status.FilteredCount,
		&s3Key, &errorOutput, &completionTime, &createdAt)

	if err == sql.ErrNoRows {
		writeError(w, "library not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get library: %v", err), http.StatusInternalServerError)
		return
	}

	status.CreatedAt = createdAt.Format(time.RFC3339)
	if s3Key.Valid {
		status.S3Key = &s3Key.String
	}
	if errorOutput.Valid {
		status.ErrorOutput = &errorOutput.String
	}
	if completionTime.Valid {
		ts := completionTime.Time.Format(time.RFC3339)
		status.CompletedAt = &ts
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// LibraryCompoundsHandler handles GET /api/v1/libraries/{name}/compounds.
// Returns a paginated compound list with all annotations.
func (h *APIHandler) LibraryCompoundsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := extractLibraryNameForCompounds(r.URL.Path)
	if name == "" {
		writeError(w, "library name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Resolve library ID.
	var libraryID int
	err := db.QueryRowContext(r.Context(),
		`SELECT id FROM library_prep_results WHERE name = ?`, name).Scan(&libraryID)
	if err == sql.ErrNoRows {
		writeError(w, "library not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get library: %v", err), http.StatusInternalServerError)
		return
	}

	// Pagination.
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			page = n
		}
	}
	perPage := 50
	if v := q.Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 500 {
			perPage = n
		}
	}
	offset := (page - 1) * perPage

	// Count total.
	var total int
	if err := db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM library_compounds WHERE library_id = ?`, libraryID).Scan(&total); err != nil {
		writeError(w, fmt.Sprintf("failed to count compounds: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch compounds.
	rows, err := db.QueryContext(r.Context(),
		`SELECT compound_id, canonical_smiles, inchikey,
			mw, logp, hba, hbd, psa, rotatable_bonds, qed,
			lipinski_pass, veber_pass, pains_pass, brenk_pass, reos_pass,
			filtered, s3_conformer_key
		 FROM library_compounds
		 WHERE library_id = ?
		 ORDER BY id
		 LIMIT ? OFFSET ?`, libraryID, perPage, offset)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query compounds: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var compounds []LibraryCompound
	for rows.Next() {
		var c LibraryCompound
		var lipinskiPass, veberPass, painsPass, brenkPass, reosPass sql.NullBool
		var s3Key sql.NullString

		if err := rows.Scan(
			&c.CompoundID, &c.CanonicalSMILES, &c.InChIKey,
			&c.MW, &c.LogP, &c.HBA, &c.HBD, &c.PSA, &c.RotatableBonds, &c.QED,
			&lipinskiPass, &veberPass, &painsPass, &brenkPass, &reosPass,
			&c.Filtered, &s3Key,
		); err != nil {
			continue
		}

		if lipinskiPass.Valid {
			c.LipinskiPass = &lipinskiPass.Bool
		}
		if veberPass.Valid {
			c.VeberPass = &veberPass.Bool
		}
		if painsPass.Valid {
			c.PAINSPass = &painsPass.Bool
		}
		if brenkPass.Valid {
			c.BrenkPass = &brenkPass.Bool
		}
		if reosPass.Valid {
			c.REOSPass = &reosPass.Bool
		}
		if s3Key.Valid {
			c.S3ConformerKey = &s3Key.String
		}

		compounds = append(compounds, c)
	}
	if compounds == nil {
		compounds = []LibraryCompound{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"compounds": compounds,
		"total":     total,
		"page":      page,
		"per_page":  perPage,
	})
}

// --- Route dispatcher ---

// LibraryDispatch routes /api/v1/libraries/ requests based on path structure.
// It distinguishes between:
//   - POST /api/v1/libraries/prepare                   -> LibraryPrepareHandler
//   - GET  /api/v1/libraries/{name}                    -> LibraryGetHandler
//   - GET  /api/v1/libraries/{name}/compounds          -> LibraryCompoundsHandler
func (h *APIHandler) LibraryDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/libraries/")
	path = strings.TrimRight(path, "/")

	// POST /api/v1/libraries/prepare
	if r.Method == http.MethodPost && path == "prepare" {
		h.LibraryPrepareHandler(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(path, "/")

	// GET /api/v1/libraries/{name}/compounds
	if len(parts) == 2 && parts[1] == "compounds" && parts[0] != "" {
		h.LibraryCompoundsHandler(w, r)
		return
	}

	// GET /api/v1/libraries/{name}
	if len(parts) == 1 && parts[0] != "" {
		h.LibraryGetHandler(w, r)
		return
	}

	writeError(w, "not found", http.StatusNotFound)
}

// --- Async pipeline ---

// runLibraryPrepPipeline orchestrates the full library preparation workflow:
// 1. Update status to Running
// 2. Resolve input compounds from source (SMILES, SDF, ChEMBL, Enamine)
// 3. Call the library-prep sidecar for standardization + filtering + conformers
// 4. Store results in the database and Garage
// 5. Record provenance
// 6. Optionally create a LibraryPrep CRD instance
// 7. Update status to Succeeded or Failed
func (h *APIHandler) runLibraryPrepPipeline(jobName string, req LibraryPrepRequest) {
	ctx := context.Background()

	db := h.controller.firstDB()
	if db == nil {
		log.Printf("[library-prep] %s: CRITICAL: no database available", jobName)
		return
	}

	// Mark as Running.
	if _, err := db.ExecContext(ctx,
		`UPDATE library_prep_results SET phase = 'Running', start_time = NOW() WHERE name = ?`, jobName); err != nil {
		log.Printf("[library-prep] %s: failed to update status to Running: %v", jobName, err)
	}

	// Step 1: Resolve input SMILES from source.
	smilesList, err := h.resolveLibrarySource(ctx, req)
	if err != nil {
		h.failLibraryPrep(ctx, db, jobName, fmt.Sprintf("source resolution failed: %v", err))
		return
	}

	if len(smilesList) == 0 {
		h.failLibraryPrep(ctx, db, jobName, "no compounds found from source")
		return
	}

	log.Printf("[library-prep] %s: resolved %d input compound(s) from source=%s", jobName, len(smilesList), req.Source)

	// Write resolved count immediately so the UI can display progress during the sidecar call.
	db.ExecContext(ctx, `UPDATE library_prep_results SET compound_count = ? WHERE name = ?`, len(smilesList), jobName)

	// Step 2: Call library-prep sidecar for standardization + filtering + conformers.
	sidecarReq := libraryPrepSidecarRequest{
		SMILES:  smilesList,
		Filters: req.Filters,
		JobName: jobName,
	}

	sidecarResp, err := h.callLibraryPrepSidecar(ctx, sidecarReq)
	if err != nil {
		h.failLibraryPrep(ctx, db, jobName, fmt.Sprintf("sidecar processing failed: %v", err))
		return
	}

	// Step 3: Insert compound records.
	var libraryID int
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM library_prep_results WHERE name = ?`, jobName).Scan(&libraryID); err != nil {
		h.failLibraryPrep(ctx, db, jobName, fmt.Sprintf("failed to look up library ID: %v", err))
		return
	}

	compoundCount, filteredCount, err := h.insertLibraryCompounds(ctx, db, libraryID, jobName, sidecarResp)
	if err != nil {
		h.failLibraryPrep(ctx, db, jobName, fmt.Sprintf("failed to insert compounds: %v", err))
		return
	}

	// Step 4: Store library summary artifact in Garage.
	s3Key := ArtifactKey("LibraryPrep", jobName, "library", "json")
	summaryJSON, _ := json.Marshal(map[string]interface{}{
		"name":           jobName,
		"source":         req.Source,
		"compound_count": compoundCount,
		"filtered_count": filteredCount,
		"filters":        req.Filters,
	})
	if err := h.s3Client.PutArtifact(ctx, BucketLibraries, s3Key,
		strings.NewReader(string(summaryJSON)), "application/json"); err != nil {
		log.Printf("[library-prep] %s: warning: failed to store library summary in S3: %v", jobName, err)
	}

	// Step 5: Record provenance.
	bucket := BucketLibraries
	jobKind := "LibraryPrep"
	params, _ := json.Marshal(map[string]interface{}{
		"source":  req.Source,
		"name":    req.Name,
		"filters": req.Filters,
	})

	provRecord := &ProvenanceRecord{
		ArtifactType: "library",
		S3Bucket:     &bucket,
		S3Key:        &s3Key,
		CreatedByJob: jobName,
		JobKind:      &jobKind,
		JobNamespace: "chem",
		Parameters:   params,
	}
	if err := RecordProvenance(ctx, db, provRecord, nil); err != nil {
		log.Printf("[library-prep] %s: warning: failed to record provenance: %v", jobName, err)
	}

	// Step 6: Optionally create a LibraryPrep CRD instance.
	if os.Getenv("CRD_ENABLED") == "true" {
		h.createLibraryPrepCRD(ctx, jobName, req, s3Key, compoundCount, filteredCount)
	}

	// Step 7: Mark as Succeeded.
	if _, err := db.ExecContext(ctx,
		`UPDATE library_prep_results
		 SET phase = 'Succeeded', compound_count = ?, filtered_count = ?,
		     s3_key = ?, completion_time = NOW()
		 WHERE name = ?`,
		compoundCount, filteredCount, s3Key, jobName); err != nil {
		log.Printf("[library-prep] %s: failed to update status to Succeeded: %v", jobName, err)
	}

	log.Printf("[library-prep] %s: completed successfully (%d compounds, %d filtered)",
		jobName, compoundCount, filteredCount)
}

// --- Source resolution ---

// resolveLibrarySource resolves the input SMILES list from the request source.
func (h *APIHandler) resolveLibrarySource(ctx context.Context, req LibraryPrepRequest) ([]string, error) {
	switch req.Source {
	case "smiles":
		return req.SMILESList, nil

	case "sdf":
		// SDF data is passed directly to the sidecar for parsing.
		// Return a single-element list with the raw SDF to signal sidecar to parse.
		if req.SDFData != "" {
			return []string{"__SDF__:" + req.SDFData}, nil
		}
		if req.S3Ref != "" {
			return []string{"__S3_SDF__:" + req.S3Ref}, nil
		}
		return nil, fmt.Errorf("no SDF data or S3 reference provided")

	case "chembl":
		return h.resolveChEMBLSource(ctx, req.ChEMBL)

	case "enamine":
		// Enamine REAL subset: SMILES list or S3 reference.
		if len(req.SMILESList) > 0 {
			return req.SMILESList, nil
		}
		if req.S3Ref != "" {
			return []string{"__S3_ENAMINE__:" + req.S3Ref}, nil
		}
		return nil, fmt.Errorf("no Enamine data provided")

	default:
		return nil, fmt.Errorf("unknown source: %s", req.Source)
	}
}

// resolveChEMBLSource queries the ChEMBL database and returns canonical SMILES
// for all matching compounds. Reuses the buildChEMBLFilterClauses helper from
// handlers_chembl.go.
func (h *APIHandler) resolveChEMBLSource(ctx context.Context, params *ChEMBLSourceParams) ([]string, error) {
	if h.chemblDB == nil {
		return nil, fmt.Errorf("ChEMBL database not available")
	}

	conditions, args := buildChEMBLFilterClauses(params.toParamMap())

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`SELECT cs.canonical_smiles
		FROM molecule_dictionary md
		JOIN compound_structures cs ON md.molregno = cs.molregno
		LEFT JOIN compound_properties cp ON md.molregno = cp.molregno
		%s
		ORDER BY md.chembl_id
		LIMIT 50000`, whereClause)

	rows, err := h.chemblDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ChEMBL query failed: %w", err)
	}
	defer rows.Close()

	var smiles []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			continue
		}
		smiles = append(smiles, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading ChEMBL results: %w", err)
	}

	return smiles, nil
}

// --- Sidecar communication ---

// libraryPrepSidecarRequest is the request body sent to the library-prep sidecar.
type libraryPrepSidecarRequest struct {
	SMILES  []string           `json:"smiles_list"`
	Filters LibraryPrepFilters `json:"filters"`
	JobName string             `json:"job_name"`
}

// libraryPrepSidecarCompound is a single compound result from the sidecar.
type libraryPrepSidecarCompound struct {
	CanonicalSMILES string   `json:"canonical_smiles"`
	InChIKey        string   `json:"inchikey"`
	MW              *float64 `json:"mw,omitempty"`
	LogP            *float64 `json:"logp,omitempty"`
	HBA             *int     `json:"hba,omitempty"`
	HBD             *int     `json:"hbd,omitempty"`
	PSA             *float64 `json:"psa,omitempty"`
	RotatableBonds  *int     `json:"rotatable_bonds,omitempty"`
	QED             *float64 `json:"qed,omitempty"`
	LipinskiPass    *bool    `json:"lipinski_pass,omitempty"`
	VeberPass       *bool    `json:"veber_pass,omitempty"`
	PAINSPass       *bool    `json:"pains_pass,omitempty"`
	BrenkPass       *bool    `json:"brenk_pass,omitempty"`
	REOSPass        *bool    `json:"reos_pass,omitempty"`
	Filtered        bool     `json:"filtered"`
	ConformerS3Key  string   `json:"conformer_s3_key,omitempty"` // set by handler after S3 upload
	PDBQTData       string   `json:"pdbqt_data,omitempty"`       // raw PDBQT from sidecar (pre-upload)
	Error           string   `json:"error,omitempty"`
}

// libraryPrepSidecarResponse is the response from the library-prep sidecar.
type libraryPrepSidecarResponse struct {
	Compounds []libraryPrepSidecarCompound `json:"compounds"`
	Error     string                       `json:"error,omitempty"`
}

// callLibraryPrepSidecar sends compounds to the library-prep sidecar for
// standardization, filtering, and conformer generation.
func (h *APIHandler) callLibraryPrepSidecar(ctx context.Context, req libraryPrepSidecarRequest) (*libraryPrepSidecarResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sidecar request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Minute}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		libraryPrepServiceURL+"/standardize", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create sidecar request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to contact library-prep sidecar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("library-prep sidecar returned %d: %s", resp.StatusCode, errResp.Error)
	}

	var sidecarResp libraryPrepSidecarResponse
	if err := json.NewDecoder(resp.Body).Decode(&sidecarResp); err != nil {
		return nil, fmt.Errorf("failed to decode sidecar response: %w", err)
	}

	if sidecarResp.Error != "" {
		return nil, fmt.Errorf("sidecar error: %s", sidecarResp.Error)
	}

	return &sidecarResp, nil
}

// --- Compound insertion ---

// insertLibraryCompounds inserts processed compounds into the library_compounds
// table and returns (total_count, filtered_count).
// For each compound that has PDBQTData, it uploads the PDBQT to S3 and stores
// the resulting key in s3_conformer_key. The sidecar returns raw PDBQT bytes;
// the handler is responsible for the S3 upload.
func (h *APIHandler) insertLibraryCompounds(ctx context.Context, db *sql.DB, libraryID int, jobName string, resp *libraryPrepSidecarResponse) (int, int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("beginning compound insert transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO library_compounds
			(library_id, compound_id, canonical_smiles, inchikey,
			 mw, logp, hba, hbd, psa, rotatable_bonds, qed,
			 lipinski_pass, veber_pass, pains_pass, brenk_pass, reos_pass,
			 filtered, s3_conformer_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, 0, fmt.Errorf("preparing compound insert: %w", err)
	}
	defer stmt.Close()

	compoundCount := 0
	filteredCount := 0

	for _, c := range resp.Compounds {
		if c.Error != "" {
			log.Printf("[library-prep] %s: compound error (skipped): %s", jobName, c.Error)
			continue
		}

		// Generate stable compound ID: KHM-{first 8 chars of InChIKey}.
		compoundID := generateStableCompoundID(c.InChIKey)

		// Upload PDBQT conformer to S3 if the sidecar returned one.
		// The sidecar returns raw PDBQT bytes in PDBQTData; we upload and store the key.
		var conformerKey sql.NullString
		if c.PDBQTData != "" && h.s3Client != nil {
			s3Key := fmt.Sprintf("LibraryPrep/%s/%s.pdbqt", jobName, compoundID)
			if err := h.s3Client.PutArtifact(ctx, BucketLibraries, s3Key,
				strings.NewReader(c.PDBQTData), "chemical/x-pdbqt"); err != nil {
				log.Printf("[library-prep] %s: warning: failed to upload conformer for %s: %v", jobName, compoundID, err)
			} else {
				conformerKey = sql.NullString{String: s3Key, Valid: true}
			}
		} else if c.ConformerS3Key != "" {
			conformerKey = sql.NullString{String: c.ConformerS3Key, Valid: true}
		}

		if _, err := stmt.ExecContext(ctx,
			libraryID, compoundID, c.CanonicalSMILES, c.InChIKey,
			c.MW, c.LogP, c.HBA, c.HBD, c.PSA, c.RotatableBonds, c.QED,
			c.LipinskiPass, c.VeberPass, c.PAINSPass, c.BrenkPass, c.REOSPass,
			c.Filtered, conformerKey,
		); err != nil {
			log.Printf("[library-prep] %s: warning: failed to insert compound %s: %v", jobName, compoundID, err)
			continue
		}

		compoundCount++
		if c.Filtered {
			filteredCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("committing compound insert: %w", err)
	}

	return compoundCount, filteredCount, nil
}

// --- CRD creation ---

// createLibraryPrepCRD creates a LibraryPrep CRD instance to track the job
// in the Kubernetes API.
func (h *APIHandler) createLibraryPrepCRD(ctx context.Context, jobName string, req LibraryPrepRequest, s3Key string, compoundCount, filteredCount int) {
	if h.controller.crdController == nil || h.controller.crdController.dynamicClient == nil {
		return
	}

	filtersMap := map[string]interface{}{
		"lipinski": derefBool(req.Filters.Lipinski),
		"veber":    derefBool(req.Filters.Veber),
		"pains":    derefBool(req.Filters.PAINS),
		"brenk":    derefBool(req.Filters.Brenk),
		"reos":     derefBool(req.Filters.REOS),
	}

	crd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "khemeia.io/v1alpha1",
			"kind":       "LibraryPrep",
			"metadata": map[string]interface{}{
				"name":      jobName,
				"namespace": h.namespace,
			},
			"spec": map[string]interface{}{
				"source":  req.Source,
				"name":    req.Name,
				"filters": filtersMap,
			},
			"status": map[string]interface{}{
				"phase":          "Succeeded",
				"totalCompounds": int64(compoundCount),
				"passedFilter":   int64(compoundCount - filteredCount),
				"failedFilter":   int64(filteredCount),
				"completionTime": time.Now().Format(time.RFC3339),
			},
		},
	}

	gvr := registeredCRDs["LibraryPrep"]
	_, err := h.controller.crdController.dynamicClient.Resource(gvr).
		Namespace(h.namespace).Create(ctx, crd, metav1.CreateOptions{})
	if err != nil {
		log.Printf("[library-prep] %s: warning: failed to create LibraryPrep CRD: %v", jobName, err)
	}
}

// --- Helpers ---

// failLibraryPrep marks a library prep job as Failed in MySQL with the given error message.
func (h *APIHandler) failLibraryPrep(ctx context.Context, db *sql.DB, jobName string, errMsg string) {
	log.Printf("[library-prep] %s: FAILED: %s", jobName, errMsg)
	if _, err := db.ExecContext(ctx,
		`UPDATE library_prep_results SET phase = 'Failed', error_output = ?, completion_time = NOW() WHERE name = ?`,
		errMsg, jobName); err != nil {
		log.Printf("[library-prep] %s: failed to update status to Failed: %v", jobName, err)
	}
}

// generateStableCompoundID produces a stable ID in the form KHM-{first14ofInChIKey}.
// The first 14 characters of an InChIKey are the connectivity layer hash,
// which is unique per molecular skeleton (e.g., KHM-BSYNRYMUTXBXSQ).
// If the InChIKey is shorter than 14 characters, the full key is used.
func generateStableCompoundID(inchiKey string) string {
	key := inchiKey
	if len(key) > 14 {
		key = key[:14]
	}
	return "KHM-" + strings.ToUpper(key)
}

// sanitizeName replaces characters unsuitable for job names with hyphens.
func sanitizeName(name string) string {
	replacer := strings.NewReplacer(
		" ", "-", "_", "-", "/", "-", "\\", "-",
		".", "-", "(", "", ")", "",
	)
	s := replacer.Replace(strings.ToLower(name))
	// Collapse multiple hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// extractLibraryName extracts the library name from a URL path like
// /api/v1/libraries/{name} or /api/v1/libraries/{name}/.
func extractLibraryName(path string) string {
	s := strings.TrimPrefix(path, "/api/v1/libraries/")
	s = strings.TrimRight(s, "/")
	if strings.Contains(s, "/") {
		return ""
	}
	return s
}

// extractLibraryNameForCompounds extracts the library name from a URL path
// like /api/v1/libraries/{name}/compounds.
func extractLibraryNameForCompounds(path string) string {
	s := strings.TrimPrefix(path, "/api/v1/libraries/")
	s = strings.TrimRight(s, "/")
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[1] != "compounds" {
		return ""
	}
	return parts[0]
}

// derefBool safely dereferences a bool pointer, returning false if nil.
func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

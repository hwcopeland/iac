// Package main provides HTTP handlers for WP-4 ADMET prediction endpoints.
// These handlers manage the lifecycle of ADMET (Absorption, Distribution,
// Metabolism, Excretion, Toxicity) prediction jobs. Predictions are generated
// by the admet-ai sidecar service using Stanford Chemprop-D models, with
// results stored in MySQL and Garage.
//
// Endpoints:
//   POST /api/v1/admet/predict                       - submit an ADMET prediction job
//   GET  /api/v1/admet/jobs/{name}                   - get ADMET job status
//   GET  /api/v1/admet/jobs/{name}/results            - paginated per-compound ADMET results
//   GET  /api/v1/admet/compound/{compoundId}          - single compound ADMET profile
//   GET  /api/v1/admet/presets                        - list available MPO presets
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
)

// --- Service URL for ADMET sidecar ---

// admetServiceURL is the base URL of the ADMET prediction sidecar.
var admetServiceURL = getAdmetURL()

func getAdmetURL() string {
	if url := os.Getenv("ADMET_SERVICE_URL"); url != "" {
		return url
	}
	return "http://admet.chem.svc.cluster.local"
}

// --- Request/response types ---

// ADMETSubmitRequest is the JSON body for POST /api/v1/admet/predict.
type ADMETSubmitRequest struct {
	CompoundRefs []string `json:"compound_refs"`          // compound IDs from library_compounds
	LibraryRef   string   `json:"library_ref"`            // library-prep job name
	Engines      []string `json:"engines,omitempty"`       // default: ["admet_ai"]
	MPOProfile   string   `json:"mpo_profile,omitempty"`   // default: "oral"
}

// ADMETSubmitResponse is the 202 Accepted response for a new ADMET prediction job.
type ADMETSubmitResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ADMETJobStatus is the response for GET /api/v1/admet/jobs/{name}.
type ADMETJobStatus struct {
	Name             string  `json:"name"`
	Phase            string  `json:"phase"`
	LibraryRef       string  `json:"library_ref"`
	MPOProfile       string  `json:"mpo_profile"`
	TotalCompounds   int     `json:"total_compounds"`
	PredictedCount   int     `json:"predicted_count"`
	FailedCount      int     `json:"failed_count"`
	AvgMPOScore      float64 `json:"avg_mpo_score"`
	CreatedAt        string  `json:"created_at"`
	StartedAt        *string `json:"started_at,omitempty"`
	CompletedAt      *string `json:"completed_at,omitempty"`
	ErrorOutput      *string `json:"error_output,omitempty"`
}

// ADMETCompoundResult represents a single compound's full ADMET prediction.
type ADMETCompoundResult struct {
	CompoundID  string                       `json:"compound_id"`
	SMILES      string                       `json:"smiles"`
	MPOScore    float64                      `json:"mpo_score"`
	MPOProfile  string                       `json:"mpo_profile"`
	Endpoints   map[string]json.RawMessage   `json:"endpoints"`
	Flags       json.RawMessage              `json:"flags"`
	Engine      string                       `json:"engine"`
	PredictedAt string                       `json:"predicted_at"`
}

// --- Sidecar request/response types ---

type admetSidecarPredictRequest struct {
	SMILESList []string `json:"smiles_list"`
	Endpoints  string   `json:"endpoints"`
}

type admetSidecarPredictResponse struct {
	Predictions []admetSidecarPrediction `json:"predictions"`
	Count       int                      `json:"count"`
	Engine      string                   `json:"engine"`
}

type admetSidecarPrediction struct {
	SMILES    string                     `json:"smiles"`
	Endpoints map[string]json.RawMessage `json:"endpoints"`
	Flags     json.RawMessage            `json:"flags"`
	Engine    string                     `json:"engine"`
	Error     string                     `json:"error,omitempty"`
}

type admetSidecarMPORequest struct {
	Predictions []admetSidecarPrediction `json:"predictions"`
	Profile     string                   `json:"profile"`
}

type admetSidecarMPOResponse struct {
	Scores []admetSidecarMPOScore `json:"scores"`
}

type admetSidecarMPOScore struct {
	SMILES     string  `json:"smiles"`
	MPOScore   float64 `json:"mpo_score"`
	MPOProfile string  `json:"mpo_profile"`
}

// --- MySQL schema ---

// EnsureADMETSchema creates the ADMET tables if they do not exist.
// Called during startup on the shared database, following the same pattern
// as EnsureTargetPrepSchema and EnsureLibraryPrepSchema.
func EnsureADMETSchema(db *sql.DB) error {
	jobsDDL := `CREATE TABLE IF NOT EXISTS admet_jobs (
		id              INT AUTO_INCREMENT PRIMARY KEY,
		name            VARCHAR(255) NOT NULL UNIQUE,
		phase           ENUM('Pending', 'Running', 'Succeeded', 'Failed') NOT NULL DEFAULT 'Pending',
		library_ref     VARCHAR(255) NOT NULL,
		mpo_profile     VARCHAR(32) NOT NULL DEFAULT 'oral',
		engines         JSON NULL,
		total_compounds INT NOT NULL DEFAULT 0,
		predicted_count INT NOT NULL DEFAULT 0,
		failed_count    INT NOT NULL DEFAULT 0,
		avg_mpo_score   FLOAT NOT NULL DEFAULT 0,
		s3_key          VARCHAR(512) NULL,
		error_output    TEXT NULL,
		request_params  JSON NULL,
		start_time      TIMESTAMP NULL,
		completion_time TIMESTAMP NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_phase (phase),
		INDEX idx_library_ref (library_ref),
		INDEX idx_created_at (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(jobsDDL); err != nil {
		return fmt.Errorf("creating admet_jobs table: %w", err)
	}

	resultsDDL := `CREATE TABLE IF NOT EXISTS admet_results (
		id              INT AUTO_INCREMENT PRIMARY KEY,
		job_name        VARCHAR(255) NOT NULL,
		compound_id     VARCHAR(64) NOT NULL,
		smiles          TEXT NOT NULL,
		mpo_score       FLOAT NOT NULL DEFAULT 0,
		mpo_profile     VARCHAR(32) NOT NULL DEFAULT 'oral',
		endpoints       JSON NOT NULL,
		flags           JSON NOT NULL,
		engine          VARCHAR(32) NOT NULL DEFAULT 'admet_ai',
		predicted_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_job_name (job_name),
		INDEX idx_compound_id (compound_id),
		INDEX idx_mpo_score (mpo_score),
		UNIQUE INDEX idx_job_compound (job_name, compound_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(resultsDDL); err != nil {
		return fmt.Errorf("creating admet_results table: %w", err)
	}

	return nil
}

// --- HTTP handlers ---

// ADMETPredictHandler handles POST /api/v1/admet/predict.
// Validates input, resolves compound SMILES from the library, creates the job
// record, and launches the ADMET prediction pipeline in a background goroutine.
func (h *APIHandler) ADMETPredictHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ADMETSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields.
	if req.LibraryRef == "" && len(req.CompoundRefs) == 0 {
		writeError(w, "library_ref or compound_refs is required", http.StatusBadRequest)
		return
	}

	// Apply defaults.
	if len(req.Engines) == 0 {
		req.Engines = []string{"admet_ai"}
	}
	if req.MPOProfile == "" {
		req.MPOProfile = "oral"
	}

	// Validate MPO profile.
	validProfiles := map[string]bool{
		"oral": true, "cns": true, "oncology": true, "antimicrobial": true,
	}
	if !validProfiles[req.MPOProfile] {
		writeError(w, fmt.Sprintf("invalid mpo_profile: %q (must be one of: oral, cns, oncology, antimicrobial)", req.MPOProfile), http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Generate job name.
	jobName := fmt.Sprintf("admet-%d", time.Now().UnixNano())

	// Serialize JSON fields.
	enginesJSON, _ := json.Marshal(req.Engines)
	reqParamsJSON, _ := json.Marshal(req)

	// Insert the ADMET job record.
	_, err := db.ExecContext(r.Context(),
		`INSERT INTO admet_jobs
			(name, library_ref, mpo_profile, engines, request_params)
		 VALUES (?, ?, ?, ?, ?)`,
		jobName, req.LibraryRef, req.MPOProfile, string(enginesJSON), string(reqParamsJSON))
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create ADMET job: %v", err), http.StatusInternalServerError)
		return
	}

	// Return 202 Accepted.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(ADMETSubmitResponse{
		Name:   jobName,
		Status: "Pending",
	})

	// Start async pipeline.
	go h.runADMETPipeline(jobName, req)
}

// ADMETJobStatusHandler handles GET /api/v1/admet/jobs/{name}.
// Returns the ADMET job status and prediction counts.
func (h *APIHandler) ADMETJobStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := extractADMETJobName(r.URL.Path)
	if name == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var status ADMETJobStatus
	var errorOutput sql.NullString
	var startTime, completionTime sql.NullTime
	var createdAt time.Time

	err := db.QueryRowContext(r.Context(),
		`SELECT name, phase, library_ref, mpo_profile,
			total_compounds, predicted_count, failed_count, avg_mpo_score,
			error_output, start_time, completion_time, created_at
		 FROM admet_jobs WHERE name = ?`, name).Scan(
		&status.Name, &status.Phase, &status.LibraryRef, &status.MPOProfile,
		&status.TotalCompounds, &status.PredictedCount, &status.FailedCount,
		&status.AvgMPOScore, &errorOutput, &startTime, &completionTime, &createdAt)

	if err == sql.ErrNoRows {
		writeError(w, "ADMET job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get ADMET job: %v", err), http.StatusInternalServerError)
		return
	}

	status.CreatedAt = createdAt.Format(time.RFC3339)
	if errorOutput.Valid {
		status.ErrorOutput = &errorOutput.String
	}
	if startTime.Valid {
		ts := startTime.Time.Format(time.RFC3339)
		status.StartedAt = &ts
	}
	if completionTime.Valid {
		ts := completionTime.Time.Format(time.RFC3339)
		status.CompletedAt = &ts
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// ADMETJobResultsHandler handles GET /api/v1/admet/jobs/{name}/results.
// Returns paginated per-compound ADMET results with all endpoints and MPO score.
func (h *APIHandler) ADMETJobResultsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := extractADMETJobNameForResults(r.URL.Path)
	if name == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
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
		`SELECT COUNT(*) FROM admet_results WHERE job_name = ?`, name).Scan(&total); err != nil {
		writeError(w, fmt.Sprintf("failed to count results: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch results.
	rows, err := db.QueryContext(r.Context(),
		`SELECT compound_id, smiles, mpo_score, mpo_profile,
			endpoints, flags, engine, predicted_at
		 FROM admet_results
		 WHERE job_name = ?
		 ORDER BY mpo_score DESC
		 LIMIT ? OFFSET ?`, name, perPage, offset)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query results: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []ADMETCompoundResult
	for rows.Next() {
		var c ADMETCompoundResult
		var endpointsJSON, flagsJSON []byte
		var predictedAt time.Time

		if err := rows.Scan(
			&c.CompoundID, &c.SMILES, &c.MPOScore, &c.MPOProfile,
			&endpointsJSON, &flagsJSON, &c.Engine, &predictedAt,
		); err != nil {
			continue
		}

		json.Unmarshal(endpointsJSON, &c.Endpoints)
		c.Flags = json.RawMessage(flagsJSON)
		c.PredictedAt = predictedAt.Format(time.RFC3339)
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("error reading results: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_name": name,
		"results":  results,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// ADMETCompoundHandler handles GET /api/v1/admet/compound/{compoundId}.
// Returns the full ADMET profile for a single compound (most recent prediction).
func (h *APIHandler) ADMETCompoundHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	compoundID := strings.TrimPrefix(r.URL.Path, "/api/v1/admet/compound/")
	compoundID = strings.TrimRight(compoundID, "/")

	if compoundID == "" {
		writeError(w, "compound ID required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var c ADMETCompoundResult
	var endpointsJSON, flagsJSON []byte
	var predictedAt time.Time

	err := db.QueryRowContext(r.Context(),
		`SELECT compound_id, smiles, mpo_score, mpo_profile,
			endpoints, flags, engine, predicted_at
		 FROM admet_results
		 WHERE compound_id = ?
		 ORDER BY predicted_at DESC
		 LIMIT 1`, compoundID).Scan(
		&c.CompoundID, &c.SMILES, &c.MPOScore, &c.MPOProfile,
		&endpointsJSON, &flagsJSON, &c.Engine, &predictedAt)

	if err == sql.ErrNoRows {
		writeError(w, "no ADMET predictions found for compound", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get ADMET prediction: %v", err), http.StatusInternalServerError)
		return
	}

	json.Unmarshal(endpointsJSON, &c.Endpoints)
	c.Flags = json.RawMessage(flagsJSON)
	c.PredictedAt = predictedAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c)
}

// ADMETPresetsHandler handles GET /api/v1/admet/presets.
// Returns available MPO presets and their descriptions.
func (h *APIHandler) ADMETPresetsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	presets := []map[string]interface{}{
		{
			"name":        "oral",
			"description": "Standard oral drug profile: HIA, Caco-2, bioavailability, solubility, CYP panel, clearance",
		},
		{
			"name":        "cns",
			"description": "CNS-penetrant profile: BBB, P-gp (penalize substrate), PPB, HIA, hERG",
		},
		{
			"name":        "oncology",
			"description": "Oncology-specific tolerances: relaxed oral, higher toxicity tolerance, emphasize potency",
		},
		{
			"name":        "antimicrobial",
			"description": "Antimicrobial profile: solubility, permeability, clearance, AMES (strict)",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"presets": presets,
		"count":   len(presets),
	})
}

// ADMETDispatch routes /api/v1/admet/ requests based on path structure.
func (h *APIHandler) ADMETDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admet/")
	path = strings.TrimRight(path, "/")

	// POST /api/v1/admet/predict
	if r.Method == http.MethodPost && path == "predict" {
		h.ADMETPredictHandler(w, r)
		return
	}

	// GET /api/v1/admet/presets
	if r.Method == http.MethodGet && path == "presets" {
		h.ADMETPresetsHandler(w, r)
		return
	}

	// GET /api/v1/admet/compound/{compoundId}
	if r.Method == http.MethodGet && strings.HasPrefix(path, "compound/") {
		h.ADMETCompoundHandler(w, r)
		return
	}

	// GET /api/v1/admet/jobs/{name}/results
	if r.Method == http.MethodGet && strings.HasPrefix(path, "jobs/") && strings.HasSuffix(path, "/results") {
		h.ADMETJobResultsHandler(w, r)
		return
	}

	// GET /api/v1/admet/jobs/{name}
	if r.Method == http.MethodGet && strings.HasPrefix(path, "jobs/") {
		h.ADMETJobStatusHandler(w, r)
		return
	}

	writeError(w, "not found", http.StatusNotFound)
}

// --- Path extraction helpers ---

// extractADMETJobName extracts the job name from paths like /api/v1/admet/jobs/{name}.
func extractADMETJobName(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/v1/admet/jobs/")
	trimmed = strings.TrimRight(trimmed, "/")
	// Strip any sub-paths (e.g., /results).
	if idx := strings.Index(trimmed, "/"); idx != -1 {
		trimmed = trimmed[:idx]
	}
	return trimmed
}

// extractADMETJobNameForResults extracts the job name from paths like
// /api/v1/admet/jobs/{name}/results.
func extractADMETJobNameForResults(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/v1/admet/jobs/")
	trimmed = strings.TrimSuffix(trimmed, "/results")
	trimmed = strings.TrimRight(trimmed, "/")
	return trimmed
}

// --- Async pipeline ---

// runADMETPipeline orchestrates the full ADMET prediction workflow:
// 1. Update status to Running
// 2. Resolve compound SMILES from library
// 3. Call ADMET sidecar for batch prediction
// 4. Call ADMET sidecar for MPO scoring
// 5. Store results in MySQL and Garage
// 6. Record provenance
// 7. Update status to Succeeded or Failed
func (h *APIHandler) runADMETPipeline(jobName string, req ADMETSubmitRequest) {
	ctx := context.Background()

	db := h.controller.firstDB()
	if db == nil {
		log.Printf("[admet] %s: CRITICAL: no database available", jobName)
		return
	}

	// Mark as Running.
	if _, err := db.ExecContext(ctx,
		`UPDATE admet_jobs SET phase = 'Running', start_time = NOW() WHERE name = ?`, jobName); err != nil {
		log.Printf("[admet] %s: failed to update status to Running: %v", jobName, err)
	}

	// Step 1: Resolve compound SMILES.
	type compoundInfo struct {
		ID     string
		SMILES string
	}

	var compounds []compoundInfo

	if req.LibraryRef != "" {
		// Resolve from library.
		rows, err := db.QueryContext(ctx,
			`SELECT lc.compound_id, lc.canonical_smiles
			 FROM library_compounds lc
			 JOIN library_prep_results lpr ON lc.library_id = lpr.id
			 WHERE lpr.name = ? AND lc.filtered = 0`,
			req.LibraryRef)
		if err != nil {
			h.failADMET(ctx, db, jobName, fmt.Sprintf("failed to query library compounds: %v", err))
			return
		}
		defer rows.Close()

		for rows.Next() {
			var c compoundInfo
			if err := rows.Scan(&c.ID, &c.SMILES); err != nil {
				continue
			}
			compounds = append(compounds, c)
		}
		if err := rows.Err(); err != nil {
			h.failADMET(ctx, db, jobName, fmt.Sprintf("error reading library compounds: %v", err))
			return
		}
	}

	// If specific compound refs were provided, filter to those.
	if len(req.CompoundRefs) > 0 && len(compounds) > 0 {
		refSet := make(map[string]bool, len(req.CompoundRefs))
		for _, ref := range req.CompoundRefs {
			refSet[ref] = true
		}
		filtered := make([]compoundInfo, 0, len(req.CompoundRefs))
		for _, c := range compounds {
			if refSet[c.ID] {
				filtered = append(filtered, c)
			}
		}
		compounds = filtered
	}

	if len(compounds) == 0 {
		h.failADMET(ctx, db, jobName, "no compounds found to predict")
		return
	}

	// Update total_compounds.
	if _, err := db.ExecContext(ctx,
		`UPDATE admet_jobs SET total_compounds = ? WHERE name = ?`,
		len(compounds), jobName); err != nil {
		log.Printf("[admet] %s: failed to update total_compounds: %v", jobName, err)
	}

	log.Printf("[admet] %s: resolved %d compounds from library=%s", jobName, len(compounds), req.LibraryRef)

	// Step 2: Call ADMET sidecar in batches.
	batchSize := 500
	var allPredictions []admetSidecarPrediction
	predictedCount := 0
	failedCount := 0

	for i := 0; i < len(compounds); i += batchSize {
		end := i + batchSize
		if end > len(compounds) {
			end = len(compounds)
		}
		batch := compounds[i:end]

		smilesList := make([]string, len(batch))
		for j, c := range batch {
			smilesList[j] = c.SMILES
		}

		sidecarResp, err := h.callADMETSidecar(ctx, admetSidecarPredictRequest{
			SMILESList: smilesList,
			Endpoints:  "all",
		})
		if err != nil {
			log.Printf("[admet] %s: sidecar batch %d-%d failed: %v", jobName, i, end, err)
			failedCount += len(batch)
			continue
		}

		for _, pred := range sidecarResp.Predictions {
			if pred.Error != "" {
				failedCount++
			} else {
				predictedCount++
			}
		}

		allPredictions = append(allPredictions, sidecarResp.Predictions...)
	}

	// Step 3: Call ADMET sidecar for MPO scoring.
	mpoResp, err := h.callADMETMPOSidecar(ctx, admetSidecarMPORequest{
		Predictions: allPredictions,
		Profile:     req.MPOProfile,
	})
	if err != nil {
		log.Printf("[admet] %s: MPO scoring failed: %v; using default scores", jobName, err)
	}

	// Build SMILES->MPO score map.
	mpoScores := make(map[string]float64)
	if mpoResp != nil {
		for _, s := range mpoResp.Scores {
			mpoScores[s.SMILES] = s.MPOScore
		}
	}

	// Step 4: Store results in MySQL.
	var totalMPO float64
	for i, pred := range allPredictions {
		if pred.Error != "" {
			continue
		}

		// Match prediction back to compound.
		var compoundID string
		if i < len(compounds) {
			compoundID = compounds[i].ID
		} else {
			compoundID = fmt.Sprintf("unknown-%d", i)
		}

		mpoScore := mpoScores[pred.SMILES]
		totalMPO += mpoScore

		endpointsJSON, _ := json.Marshal(pred.Endpoints)
		flagsJSON := pred.Flags
		if flagsJSON == nil {
			flagsJSON = json.RawMessage("{}")
		}

		if _, err := db.ExecContext(ctx,
			`INSERT INTO admet_results
				(job_name, compound_id, smiles, mpo_score, mpo_profile, endpoints, flags, engine)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE
				mpo_score = VALUES(mpo_score),
				endpoints = VALUES(endpoints),
				flags = VALUES(flags),
				engine = VALUES(engine),
				predicted_at = NOW()`,
			jobName, compoundID, pred.SMILES, mpoScore, req.MPOProfile,
			string(endpointsJSON), string(flagsJSON), pred.Engine); err != nil {
			log.Printf("[admet] %s: failed to insert result for %s: %v", jobName, compoundID, err)
		}
	}

	// Step 5: Store summary artifact in Garage.
	avgMPO := 0.0
	if predictedCount > 0 {
		avgMPO = totalMPO / float64(predictedCount)
	}

	s3Key := ArtifactKey("ADMETJob", jobName, "admet-summary", "json")
	summaryJSON, _ := json.Marshal(map[string]interface{}{
		"name":             jobName,
		"library_ref":      req.LibraryRef,
		"mpo_profile":      req.MPOProfile,
		"total_compounds":  len(compounds),
		"predicted_count":  predictedCount,
		"failed_count":     failedCount,
		"avg_mpo_score":    avgMPO,
	})
	if err := h.s3Client.PutArtifact(ctx, BucketReports, s3Key,
		strings.NewReader(string(summaryJSON)), "application/json"); err != nil {
		log.Printf("[admet] %s: warning: failed to store summary in S3: %v", jobName, err)
	}

	// Step 6: Record provenance.
	bucket := BucketReports
	jobKind := "ADMETJob"
	params, _ := json.Marshal(map[string]interface{}{
		"library_ref": req.LibraryRef,
		"engines":     req.Engines,
		"mpo_profile": req.MPOProfile,
	})

	provRecord := &ProvenanceRecord{
		ArtifactType: "admet_result",
		S3Bucket:     &bucket,
		S3Key:        &s3Key,
		CreatedByJob: jobName,
		JobKind:      &jobKind,
		JobNamespace: "chem",
		Parameters:   params,
	}
	if err := RecordProvenance(ctx, db, provRecord, nil); err != nil {
		log.Printf("[admet] %s: warning: failed to record provenance: %v", jobName, err)
	}

	// Step 7: Mark as Succeeded.
	if _, err := db.ExecContext(ctx,
		`UPDATE admet_jobs
		 SET phase = 'Succeeded', predicted_count = ?, failed_count = ?,
		     avg_mpo_score = ?, s3_key = ?, completion_time = NOW()
		 WHERE name = ?`,
		predictedCount, failedCount, avgMPO, s3Key, jobName); err != nil {
		log.Printf("[admet] %s: failed to update status to Succeeded: %v", jobName, err)
	}

	log.Printf("[admet] %s: completed (%d predicted, %d failed, avg MPO=%.1f)",
		jobName, predictedCount, failedCount, avgMPO)
}

// --- Sidecar calls ---

// callADMETSidecar calls the ADMET sidecar's /predict endpoint.
func (h *APIHandler) callADMETSidecar(ctx context.Context, req admetSidecarPredictRequest) (*admetSidecarPredictResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sidecar request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Minute}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		admetServiceURL+"/predict", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create sidecar request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to contact ADMET sidecar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("ADMET sidecar returned %d: %s", resp.StatusCode, errResp.Error)
	}

	var sidecarResp admetSidecarPredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&sidecarResp); err != nil {
		return nil, fmt.Errorf("failed to decode sidecar response: %w", err)
	}

	return &sidecarResp, nil
}

// callADMETMPOSidecar calls the ADMET sidecar's /mpo endpoint.
func (h *APIHandler) callADMETMPOSidecar(ctx context.Context, req admetSidecarMPORequest) (*admetSidecarMPOResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MPO request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		admetServiceURL+"/mpo", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create MPO request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to contact ADMET sidecar for MPO: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("ADMET MPO sidecar returned %d: %s", resp.StatusCode, errResp.Error)
	}

	var mpoResp admetSidecarMPOResponse
	if err := json.NewDecoder(resp.Body).Decode(&mpoResp); err != nil {
		return nil, fmt.Errorf("failed to decode MPO response: %w", err)
	}

	return &mpoResp, nil
}

// failADMET marks an ADMET job as failed.
func (h *APIHandler) failADMET(ctx context.Context, db *sql.DB, jobName string, errMsg string) {
	log.Printf("[admet] %s: FAILED: %s", jobName, errMsg)
	if _, err := db.ExecContext(ctx,
		`UPDATE admet_jobs SET phase = 'Failed', error_output = ?, completion_time = NOW() WHERE name = ?`,
		errMsg, jobName); err != nil {
		log.Printf("[admet] %s: failed to update status to Failed: %v", jobName, err)
	}
}

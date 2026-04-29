// Package main provides HTTP handlers for WP-3 multi-engine docking (v2 API).
// These endpoints coexist with the original v1 docking API — the v1 handlers in
// handlers_generic.go continue to serve /api/v1/docking/{submit,jobs} for
// backward compatibility. The v2 API adds engine selection, consensus scoring,
// and cross-engine result aggregation.
//
// Endpoints:
//   POST /api/v1/docking/v2/submit         — submit a multi-engine docking job
//   GET  /api/v1/docking/v2/jobs/{name}    — get job status with per-engine progress
//   GET  /api/v1/docking/v2/jobs/{name}/results — paginated consensus-ranked results
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Engine routing ---

// engineServiceURLs maps engine names to their in-cluster sidecar service URLs.
// Each sidecar exposes POST /dock for receiving docking work.
var engineServiceURLs = map[string]string{
	"vina-1.2":       getEngineURL("VINA_1_2_SERVICE_URL", "http://vina-1-2.chem.svc.cluster.local"),
	"vina-gpu":       getEngineURL("VINA_GPU_SERVICE_URL", "http://vina-gpu.chem.svc.cluster.local"),
	"vina-gpu-batch": getEngineURL("VINA_GPU_BATCH_SERVICE_URL", "http://vina-gpu.chem.svc.cluster.local"),
	"gnina":          getEngineURL("GNINA_SERVICE_URL", "http://gnina.chem.svc.cluster.local"),
	"diffdock":       getEngineURL("DIFFDOCK_SERVICE_URL", "http://diffdock.chem.svc.cluster.local"),
}

// engineContainerImages maps engine names to their container images.
// Used when creating K8s Jobs for engine workers.
var engineContainerImages = map[string]string{
	"vina-1.2":       "zot.hwcopeland.net/chem/vina:1.2",
	"vina-gpu":       "zot.hwcopeland.net/chem/vina-gpu:2.1",
	"vina-gpu-batch": "zot.hwcopeland.net/chem/vina-gpu:2.1",
	"gnina":          "zot.hwcopeland.net/chem/gnina:latest",
	"diffdock":       "zot.hwcopeland.net/chem/diffdock:latest",
}

func getEngineURL(envVar, defaultURL string) string {
	if url := os.Getenv(envVar); url != "" {
		return url
	}
	return defaultURL
}

// --- Request/response types ---

// DockingV2SubmitRequest is the JSON body for POST /api/v1/docking/v2/submit.
type DockingV2SubmitRequest struct {
	ReceptorRef    string   `json:"receptor_ref"`              // target-prep job name
	LibraryRef     string   `json:"library_ref"`               // library-prep job name
	Engines        []string `json:"engines"`                   // e.g. ["vina-1.2", "gnina"]
	Exhaustiveness int      `json:"exhaustiveness,omitempty"`  // default 32
	Scoring        string   `json:"scoring,omitempty"`         // default "vina"
	Consensus      bool     `json:"consensus"`                 // compute consensus scores
	TopNRefine     int      `json:"top_n_refine,omitempty"`    // top N poses for downstream refinement
	ChunkSize      int      `json:"chunk_size,omitempty"`      // ligands per worker chunk
}

// DockingV2SubmitResponse is the 202 Accepted response.
type DockingV2SubmitResponse struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Engines []string `json:"engines"`
}

// DockingV2JobStatus is the response for GET /api/v1/docking/v2/jobs/{name}.
type DockingV2JobStatus struct {
	Name            string                `json:"name"`
	Status          string                `json:"status"`
	Engines         []string              `json:"engines"`
	Consensus       bool                  `json:"consensus"`
	EngineProgress  []EngineProgressEntry `json:"engine_progress"`
	TotalLigands    int                   `json:"total_ligands"`
	DockedLigands   int                   `json:"docked_ligands"`
	BestAffinity    *float64              `json:"best_affinity,omitempty"`
	ConsensusReady  bool                  `json:"consensus_ready"`
	SubmittedBy     *string               `json:"submitted_by,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	StartedAt       *time.Time            `json:"started_at,omitempty"`
	CompletedAt     *time.Time            `json:"completed_at,omitempty"`
	Error           *string               `json:"error,omitempty"`
}

// EngineProgressEntry tracks progress for a single engine within a v2 docking job.
type EngineProgressEntry struct {
	Engine       string  `json:"engine"`
	Status       string  `json:"status"` // Pending, Running, Completed, Failed
	ResultCount  int     `json:"result_count"`
	BestAffinity *float64 `json:"best_affinity,omitempty"`
}

// DockingV2ResultsResponse is the response for GET /api/v1/docking/v2/jobs/{name}/results.
type DockingV2ResultsResponse struct {
	JobName        string            `json:"job_name"`
	TotalResults   int               `json:"total_results"`
	Page           int               `json:"page"`
	PerPage        int               `json:"per_page"`
	Consensus      bool              `json:"consensus"`
	Results        []ConsensusResult `json:"results"`
	Disagreements  []string          `json:"disagreements,omitempty"`
}

// --- MySQL schema for v2 docking ---

// EnsureDockingV2Schema creates the v2 docking tables.
// Called during startup alongside other schema initialization.
func EnsureDockingV2Schema(db *sql.DB) error {
	// Master job table for v2 docking jobs.
	jobsDDL := `CREATE TABLE IF NOT EXISTS docking_v2_jobs (
		id              INT AUTO_INCREMENT PRIMARY KEY,
		name            VARCHAR(255) NOT NULL UNIQUE,
		status          ENUM('Pending', 'Running', 'Completed', 'Failed') NOT NULL DEFAULT 'Pending',
		receptor_ref    VARCHAR(255) NOT NULL,
		library_ref     VARCHAR(255) NOT NULL,
		engines         JSON NOT NULL,
		exhaustiveness  INT NOT NULL DEFAULT 32,
		scoring         VARCHAR(32) NOT NULL DEFAULT 'vina',
		consensus       TINYINT(1) NOT NULL DEFAULT 0,
		top_n_refine    INT NOT NULL DEFAULT 100,
		chunk_size      INT NOT NULL DEFAULT 10000,
		submitted_by    VARCHAR(255) NULL,
		error_output    TEXT NULL,
		input_data      JSON NULL,
		output_data     JSON NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at      TIMESTAMP NULL,
		completed_at    TIMESTAMP NULL,
		INDEX idx_status (status),
		INDEX idx_receptor_ref (receptor_ref),
		INDEX idx_created_at (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(jobsDDL); err != nil {
		return fmt.Errorf("creating docking_v2_jobs table: %w", err)
	}

	// Per-engine progress tracking.
	enginesDDL := `CREATE TABLE IF NOT EXISTS docking_v2_engine_status (
		id              INT AUTO_INCREMENT PRIMARY KEY,
		job_name        VARCHAR(255) NOT NULL,
		engine          VARCHAR(32)  NOT NULL,
		status          ENUM('Pending', 'Running', 'Completed', 'Failed') NOT NULL DEFAULT 'Pending',
		result_count    INT NOT NULL DEFAULT 0,
		best_affinity   FLOAT NULL,
		error_output    TEXT NULL,
		started_at      TIMESTAMP NULL,
		completed_at    TIMESTAMP NULL,
		UNIQUE KEY uq_job_engine (job_name, engine),
		INDEX idx_job_name (job_name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(enginesDDL); err != nil {
		return fmt.Errorf("creating docking_v2_engine_status table: %w", err)
	}

	// Per-engine docking results. Extends the v1 docking_results pattern with
	// an engine column to support multi-engine aggregation.
	resultsDDL := `CREATE TABLE IF NOT EXISTS docking_v2_results (
		id                INT AUTO_INCREMENT PRIMARY KEY,
		job_name          VARCHAR(255) NOT NULL,
		engine            VARCHAR(32)  NOT NULL,
		compound_id       VARCHAR(255) NOT NULL,
		ligand_id         INT          NOT NULL,
		affinity_kcal_mol FLOAT        NOT NULL,
		docked_pdbqt      MEDIUMBLOB   NULL,
		created_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_job_name (job_name),
		INDEX idx_engine (engine),
		INDEX idx_compound (compound_id),
		INDEX idx_affinity (affinity_kcal_mol),
		INDEX idx_job_engine (job_name, engine)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(resultsDDL); err != nil {
		return fmt.Errorf("creating docking_v2_results table: %w", err)
	}

	return nil
}

// --- HTTP handlers ---

// DockingV2Dispatch routes /api/v1/docking/v2/ requests.
func (h *APIHandler) DockingV2Dispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docking/v2/")
	path = strings.TrimRight(path, "/")

	// POST /api/v1/docking/v2/submit
	if r.Method == http.MethodPost && path == "submit" {
		h.DockingV2Submit(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// GET /api/v1/docking/v2/jobs/{name}/results
	if strings.HasPrefix(path, "jobs/") && strings.HasSuffix(path, "/results") {
		h.DockingV2Results(w, r)
		return
	}

	// GET /api/v1/docking/v2/jobs/{name}
	if strings.HasPrefix(path, "jobs/") {
		h.DockingV2JobStatus(w, r)
		return
	}

	writeError(w, "not found", http.StatusNotFound)
}

// DockingV2Submit handles POST /api/v1/docking/v2/submit.
// Validates input, resolves receptor and library references, creates the job
// record, and launches the multi-engine orchestration in a background goroutine.
func (h *APIHandler) DockingV2Submit(w http.ResponseWriter, r *http.Request) {
	var req DockingV2SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Apply defaults.
	if req.Exhaustiveness == 0 {
		req.Exhaustiveness = 32
	}
	if req.Scoring == "" {
		req.Scoring = "vina"
	}
	if req.TopNRefine == 0 {
		req.TopNRefine = 100
	}
	if req.ChunkSize == 0 {
		req.ChunkSize = 10000
	}

	// Validate required fields.
	if req.ReceptorRef == "" {
		writeError(w, "receptor_ref is required (target-prep job name)", http.StatusBadRequest)
		return
	}
	if req.LibraryRef == "" {
		writeError(w, "library_ref is required (library-prep job name)", http.StatusBadRequest)
		return
	}

	// Validate engine selection.
	if err := ValidateEngineSelection(req.Engines); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Consensus requires 2+ engines.
	if req.Consensus && len(req.Engines) < 2 {
		writeError(w, "consensus scoring requires at least 2 engines", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Verify receptor_ref points to a Succeeded target prep job.
	var receptorPhase string
	var receptorS3Key sql.NullString
	err := db.QueryRowContext(r.Context(),
		`SELECT phase, receptor_s3_key FROM target_prep_results WHERE name = ?`,
		req.ReceptorRef).Scan(&receptorPhase, &receptorS3Key)
	if err == sql.ErrNoRows {
		writeError(w, fmt.Sprintf("receptor_ref %q not found in target_prep_results", req.ReceptorRef), http.StatusBadRequest)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query receptor: %v", err), http.StatusInternalServerError)
		return
	}
	if receptorPhase != "Succeeded" {
		writeError(w, fmt.Sprintf("receptor_ref %q is not ready (phase=%s)", req.ReceptorRef, receptorPhase), http.StatusBadRequest)
		return
	}

	// Verify library_ref points to a Succeeded library prep job.
	var libraryPhase string
	var compoundCount int
	err = db.QueryRowContext(r.Context(),
		`SELECT phase, compound_count FROM library_prep_results WHERE name = ?`,
		req.LibraryRef).Scan(&libraryPhase, &compoundCount)
	if err == sql.ErrNoRows {
		writeError(w, fmt.Sprintf("library_ref %q not found in library_prep_results", req.LibraryRef), http.StatusBadRequest)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query library: %v", err), http.StatusInternalServerError)
		return
	}
	if libraryPhase != "Succeeded" {
		writeError(w, fmt.Sprintf("library_ref %q is not ready (phase=%s)", req.LibraryRef, libraryPhase), http.StatusBadRequest)
		return
	}

	// Generate job name.
	jobName := fmt.Sprintf("dockv2-%d", time.Now().UnixNano())
	submittedBy := UserFromContext(r)

	enginesJSON, _ := json.Marshal(req.Engines)
	inputJSON, _ := json.Marshal(req)

	_, err = db.ExecContext(r.Context(),
		`INSERT INTO docking_v2_jobs
			(name, status, receptor_ref, library_ref, engines, exhaustiveness,
			 scoring, consensus, top_n_refine, chunk_size, submitted_by, input_data)
		 VALUES (?, 'Pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		jobName, req.ReceptorRef, req.LibraryRef, string(enginesJSON),
		req.Exhaustiveness, req.Scoring, req.Consensus,
		req.TopNRefine, req.ChunkSize, submittedBy, string(inputJSON))
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create job: %v", err), http.StatusInternalServerError)
		return
	}

	// Create per-engine status rows.
	for _, engine := range req.Engines {
		if _, err := db.ExecContext(r.Context(),
			`INSERT INTO docking_v2_engine_status (job_name, engine, status) VALUES (?, ?, 'Pending')`,
			jobName, engine); err != nil {
			log.Printf("[docking-v2] Warning: failed to create engine status for %s/%s: %v",
				jobName, engine, err)
		}
	}

	// Return 202 Accepted immediately.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(DockingV2SubmitResponse{
		Name:    jobName,
		Status:  "Pending",
		Engines: req.Engines,
	})

	// Launch multi-engine orchestration in background.
	go h.controller.RunDockingV2Job(jobName, req)
}

// DockingV2JobStatus handles GET /api/v1/docking/v2/jobs/{name}.
func (h *APIHandler) DockingV2JobStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docking/v2/jobs/")
	jobName := strings.TrimRight(path, "/")
	// Remove /results suffix if accidentally routed here.
	jobName = strings.TrimSuffix(jobName, "/results")

	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var status DockingV2JobStatus
	var enginesJSON string
	var submittedBy sql.NullString
	var errorOutput sql.NullString
	var startedAt, completedAt sql.NullTime

	err := db.QueryRowContext(r.Context(),
		`SELECT name, status, engines, consensus, submitted_by, error_output,
		        created_at, started_at, completed_at
		 FROM docking_v2_jobs WHERE name = ?`, jobName).Scan(
		&status.Name, &status.Status, &enginesJSON, &status.Consensus,
		&submittedBy, &errorOutput, &status.CreatedAt, &startedAt, &completedAt)
	if err == sql.ErrNoRows {
		writeError(w, "job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query job: %v", err), http.StatusInternalServerError)
		return
	}

	json.Unmarshal([]byte(enginesJSON), &status.Engines)
	if submittedBy.Valid {
		status.SubmittedBy = &submittedBy.String
	}
	if errorOutput.Valid {
		status.Error = &errorOutput.String
	}
	if startedAt.Valid {
		status.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		status.CompletedAt = &completedAt.Time
	}

	// Fetch per-engine progress.
	rows, err := db.QueryContext(r.Context(),
		`SELECT engine, status, result_count, best_affinity
		 FROM docking_v2_engine_status WHERE job_name = ?`, jobName)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ep EngineProgressEntry
			var bestAff sql.NullFloat64
			if err := rows.Scan(&ep.Engine, &ep.Status, &ep.ResultCount, &bestAff); err != nil {
				continue
			}
			if bestAff.Valid {
				ep.BestAffinity = &bestAff.Float64
			}
			status.EngineProgress = append(status.EngineProgress, ep)
			status.DockedLigands += ep.ResultCount
		}
	}
	if status.EngineProgress == nil {
		status.EngineProgress = []EngineProgressEntry{}
	}

	// Total ligands from library.
	var totalLigands int
	db.QueryRowContext(r.Context(),
		`SELECT compound_count FROM library_prep_results lp
		 JOIN docking_v2_jobs dj ON dj.library_ref = lp.name
		 WHERE dj.name = ?`, jobName).Scan(&totalLigands)
	status.TotalLigands = totalLigands

	// Best affinity across all engines.
	var bestAff sql.NullFloat64
	db.QueryRowContext(r.Context(),
		`SELECT MIN(affinity_kcal_mol) FROM docking_v2_results WHERE job_name = ?`,
		jobName).Scan(&bestAff)
	if bestAff.Valid {
		status.BestAffinity = &bestAff.Float64
	}

	// Consensus is ready when all engines have completed.
	allDone := true
	for _, ep := range status.EngineProgress {
		if ep.Status != "Completed" {
			allDone = false
			break
		}
	}
	status.ConsensusReady = status.Consensus && allDone && len(status.EngineProgress) > 0

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// DockingV2Results handles GET /api/v1/docking/v2/jobs/{name}/results.
// Returns paginated results with per-engine scores and consensus rank.
func (h *APIHandler) DockingV2Results(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docking/v2/jobs/")
	jobName := strings.TrimSuffix(path, "/results")
	jobName = strings.TrimRight(jobName, "/")

	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Pagination parameters.
	page := 1
	perPage := 50
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			perPage = n
		}
	}

	// Check if this is a consensus job.
	var consensus bool
	err := db.QueryRowContext(r.Context(),
		`SELECT consensus FROM docking_v2_jobs WHERE name = ?`, jobName).Scan(&consensus)
	if err == sql.ErrNoRows {
		writeError(w, "job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query job: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch all engine scores for this job.
	rows, err := db.QueryContext(r.Context(),
		`SELECT compound_id, engine, affinity_kcal_mol
		 FROM docking_v2_results
		 WHERE job_name = ?
		 ORDER BY compound_id, engine`, jobName)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query results: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allScores []EngineScore
	for rows.Next() {
		var es EngineScore
		if err := rows.Scan(&es.CompoundID, &es.Engine, &es.Affinity); err != nil {
			continue
		}
		allScores = append(allScores, es)
	}

	// Compute consensus (works for single or multi-engine).
	consensusResults, err := ComputeConsensus(allScores)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to compute consensus: %v", err), http.StatusInternalServerError)
		return
	}

	totalResults := len(consensusResults)

	// Apply pagination.
	offset := (page - 1) * perPage
	end := offset + perPage
	if offset > totalResults {
		offset = totalResults
	}
	if end > totalResults {
		end = totalResults
	}
	pageResults := consensusResults[offset:end]

	resp := DockingV2ResultsResponse{
		JobName:      jobName,
		TotalResults: totalResults,
		Page:         page,
		PerPage:      perPage,
		Consensus:    consensus,
		Results:      pageResults,
	}

	// Flag disagreements only for consensus jobs.
	if consensus && len(consensusResults) > 0 {
		resp.Disagreements = FlagDisagreements(consensusResults)
	}
	if resp.Results == nil {
		resp.Results = []ConsensusResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Multi-engine orchestration ---

// RunDockingV2Job orchestrates a multi-engine docking job. It runs each engine's
// docking phase, collects results, and finalizes the job. CPU engines run in
// parallel; GPU engines run serially (single-GPU constraint).
//
// This follows the same pattern as RunParallelDockingJob in parallel_docking.go
// but extended for multi-engine support.
func (c *Controller) RunDockingV2Job(jobName string, req DockingV2SubmitRequest) {
	log.Printf("[docking-v2] %s: starting multi-engine docking (%v)", jobName, req.Engines)

	db := c.firstDB()
	if db == nil {
		log.Printf("[docking-v2] %s: CRITICAL: no database available", jobName)
		return
	}

	// Mark job as Running.
	if _, err := db.Exec(
		`UPDATE docking_v2_jobs SET status='Running', started_at=NOW() WHERE name=?`,
		jobName); err != nil {
		log.Printf("[docking-v2] %s: failed to update status to Running: %v", jobName, err)
		return
	}

	// Separate CPU and GPU engines for scheduling.
	cpuEngines, gpuEngines := SortEnginesForScheduling(req.Engines)

	// Phase 1: Run CPU engines in parallel.
	var wg sync.WaitGroup
	engineErrors := make(map[string]error)
	var mu sync.Mutex

	for _, engine := range cpuEngines {
		wg.Add(1)
		go func(eng string) {
			defer wg.Done()
			if err := c.runSingleEngineDocking(jobName, eng, req); err != nil {
				mu.Lock()
				engineErrors[eng] = err
				mu.Unlock()
				log.Printf("[docking-v2] %s: CPU engine %s failed: %v", jobName, eng, err)
			}
		}(engine)
	}
	wg.Wait()

	// Phase 2: Run GPU engines serially (single-GPU constraint on nixos-gpu).
	for _, engine := range gpuEngines {
		if err := c.runSingleEngineDocking(jobName, engine, req); err != nil {
			mu.Lock()
			engineErrors[engine] = err
			mu.Unlock()
			log.Printf("[docking-v2] %s: GPU engine %s failed: %v", jobName, engine, err)
		}
	}

	// Finalize: check for failures.
	failed := len(engineErrors)
	if failed == len(req.Engines) {
		// All engines failed — mark the job as Failed.
		errMsgs := make([]string, 0, failed)
		for eng, err := range engineErrors {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", eng, err))
		}
		failMsg := strings.Join(errMsgs, "; ")
		if _, err := db.Exec(
			`UPDATE docking_v2_jobs SET status='Failed', error_output=?, completed_at=NOW() WHERE name=?`,
			failMsg, jobName); err != nil {
			log.Printf("[docking-v2] %s: failed to mark job as Failed: %v", jobName, err)
		}
		log.Printf("[docking-v2] %s: all engines failed", jobName)
		return
	}

	// At least one engine succeeded — finalize.
	var totalResults int
	var bestAffinity sql.NullFloat64
	db.QueryRow(`SELECT COUNT(*) FROM docking_v2_results WHERE job_name = ?`, jobName).Scan(&totalResults)
	db.QueryRow(`SELECT MIN(affinity_kcal_mol) FROM docking_v2_results WHERE job_name = ?`, jobName).Scan(&bestAffinity)

	outputJSON, _ := json.Marshal(map[string]interface{}{
		"result_count":    totalResults,
		"best_affinity":   bestAffinity.Float64,
		"engines_total":   len(req.Engines),
		"engines_failed":  failed,
		"consensus":       req.Consensus,
	})

	if _, err := db.Exec(
		`UPDATE docking_v2_jobs SET status='Completed', output_data=?, completed_at=NOW() WHERE name=?`,
		string(outputJSON), jobName); err != nil {
		log.Printf("[docking-v2] %s: failed to mark job as Completed: %v", jobName, err)
	}

	if failed > 0 {
		log.Printf("[docking-v2] %s: completed with %d/%d engine failures, %d results",
			jobName, failed, len(req.Engines), totalResults)
	} else {
		log.Printf("[docking-v2] %s: completed successfully, %d results from %d engines",
			jobName, totalResults, len(req.Engines))
	}
}

// runSingleEngineDocking runs the docking phase for a single engine. It chunks
// the library, fans out worker pods per the parallel_docking.go pattern, and
// collects results into docking_v2_results.
func (c *Controller) runSingleEngineDocking(jobName, engine string, req DockingV2SubmitRequest) error {
	log.Printf("[docking-v2] %s/%s: starting engine docking", jobName, engine)

	db := c.firstDB()
	if db == nil {
		return fmt.Errorf("no database available")
	}

	// Mark engine as Running.
	db.Exec(
		`UPDATE docking_v2_engine_status SET status='Running', started_at=NOW() WHERE job_name=? AND engine=?`,
		jobName, engine)

	// Resolve the library compound count.
	var compoundCount int
	if err := db.QueryRow(
		`SELECT compound_count FROM library_prep_results WHERE name = ?`,
		req.LibraryRef).Scan(&compoundCount); err != nil {
		return c.failEngine(db, jobName, engine, fmt.Sprintf("library ref resolution failed: %v", err))
	}

	if compoundCount == 0 {
		return c.failEngine(db, jobName, engine, "library has 0 compounds")
	}

	// Chunk and fan out worker pods.
	chunkSize := req.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 10000
	}
	workerCount := int(math.Ceil(float64(compoundCount) / float64(chunkSize)))

	image, ok := engineContainerImages[engine]
	if !ok {
		return c.failEngine(db, jobName, engine, fmt.Sprintf("no container image for engine %q", engine))
	}

	computeClass := EngineComputeClass(engine)
	log.Printf("[docking-v2] %s/%s: launching %d workers (compute=%s, image=%s)",
		jobName, engine, workerCount, computeClass, image)

	// Use the existing runParallelWorkers pattern from parallel_docking.go.
	err := c.runParallelWorkers(jobName, workerCount, chunkSize, compoundCount,
		fmt.Sprintf("dock-%s", strings.ReplaceAll(engine, ".", "-")),
		func(workerName string, offset, limit int) error {
			return c.createDockingV2Worker(jobName, workerName, engine, image, req, offset, limit, computeClass)
		},
		4*time.Hour) // timeout per engine

	if err != nil {
		return c.failEngine(db, jobName, engine, fmt.Sprintf("docking workers failed: %v", err))
	}

	// Update engine status with result count.
	var resultCount int
	var bestAff sql.NullFloat64
	db.QueryRow(`SELECT COUNT(*) FROM docking_v2_results WHERE job_name=? AND engine=?`,
		jobName, engine).Scan(&resultCount)
	db.QueryRow(`SELECT MIN(affinity_kcal_mol) FROM docking_v2_results WHERE job_name=? AND engine=?`,
		jobName, engine).Scan(&bestAff)

	db.Exec(
		`UPDATE docking_v2_engine_status
		 SET status='Completed', result_count=?, best_affinity=?, completed_at=NOW()
		 WHERE job_name=? AND engine=?`,
		resultCount, bestAff, jobName, engine)

	log.Printf("[docking-v2] %s/%s: completed, %d results", jobName, engine, resultCount)
	return nil
}

// failEngine marks an engine as failed and returns an error for the caller.
func (c *Controller) failEngine(db *sql.DB, jobName, engine, msg string) error {
	db.Exec(
		`UPDATE docking_v2_engine_status
		 SET status='Failed', error_output=?, completed_at=NOW()
		 WHERE job_name=? AND engine=?`,
		msg, jobName, engine)
	return fmt.Errorf("%s", msg)
}

// createDockingV2Worker creates a K8s Job for a single docking worker chunk.
// Mirrors createDockingWorker from parallel_docking.go but parameterized for
// the v2 multi-engine model.
func (c *Controller) createDockingV2Worker(
	jobName, workerName, engine, image string,
	req DockingV2SubmitRequest,
	offset, limit int,
	computeClass string,
) error {
	ctx := context.Background()

	env := c.buildDockingV2WorkerEnv(jobName, workerName, engine, req, offset, limit)

	backoffLimit := int32(0)
	ttl := int32(600)

	job := buildV2WorkerJob(workerName, jobName, engine, image, env, &backoffLimit, &ttl)

	// Apply compute class if available.
	c.crdController.mu.Lock()
	cc, classFound := c.crdController.computeClasses[computeClass]
	c.crdController.mu.Unlock()

	if classFound {
		if err := c.crdController.applyComputeClass(&job.Spec.Template.Spec, cc); err != nil {
			log.Printf("[docking-v2] Warning: failed to apply compute class %s: %v", computeClass, err)
		}
	}

	if _, err := c.jobClient.Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create worker job %s: %v", workerName, err)
	}

	return nil
}

// buildDockingV2WorkerEnv creates environment variables for a v2 docking worker.
func (c *Controller) buildDockingV2WorkerEnv(
	jobName, workerName, engine string,
	req DockingV2SubmitRequest,
	offset, limit int,
) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "JOB_NAME", Value: jobName},
		{Name: "WORKER_NAME", Value: workerName},
		{Name: "ENGINE", Value: engine},
		{Name: "RECEPTOR_REF", Value: req.ReceptorRef},
		{Name: "LIBRARY_REF", Value: req.LibraryRef},
		{Name: "EXHAUSTIVENESS", Value: fmt.Sprintf("%d", req.Exhaustiveness)},
		{Name: "SCORING", Value: req.Scoring},
		{Name: "BATCH_OFFSET", Value: fmt.Sprintf("%d", offset)},
		{Name: "BATCH_LIMIT", Value: fmt.Sprintf("%d", limit)},
		{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
		{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
		{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
		{Name: "MYSQL_PASSWORD", Value: os.Getenv("MYSQL_PASSWORD")},
		{Name: "NAMESPACE", Value: c.namespace},
	}

}

// buildV2WorkerJob constructs a batchv1.Job for a v2 docking worker.
// Separated for testability.
func buildV2WorkerJob(
	workerName, jobName, engine, image string,
	env []corev1.EnvVar,
	backoffLimit *int32,
	ttl *int32,
) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: workerName,
			Labels: map[string]string{
				"khemeia.io/job-name":   jobName,
				"khemeia.io/role":       "dock-v2-worker",
				"khemeia.io/engine":     engine,
				"khemeia.io/managed-by": "docking-v2",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ttl,
			BackoffLimit:            backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"khemeia.io/job-name": jobName,
						"khemeia.io/engine":   engine,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "dock",
							Image:           image,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"/bin/sh", "-c", "python3 /scripts/dock_batch.py"},
							Env:             env,
							VolumeMounts: []corev1.VolumeMount{
								emptyDirMount("scratch", "/scratch"),
								emptyDirMount("data", "/data"),
							},
						},
					},
					Volumes: []corev1.Volume{
						emptyDirVolume("scratch"),
						emptyDirVolume("data"),
					},
				},
			},
		},
	}
}

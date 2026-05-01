// Package main provides HTTP handlers for WP-5 molecular dynamics (GROMACS GPU).
//
// Each MD job selects the top-N compounds from a completed docking job,
// then launches one GROMACS K8s Job per compound on the nixos-gpu node.
// Workers run md_batch.py which performs the full EM→NVT→NPT→MD protocol
// and writes results to md_results and trajectories to the khemeia-trajectories
// S3 bucket. The handler polls for completion and surfaces per-compound status.
//
// Endpoints:
//   POST /api/v1/md/submit                                — submit an MD job
//   GET  /api/v1/md/jobs/{name}                           — job status + per-compound progress
//   GET  /api/v1/md/jobs/{name}/results                   — list completed compound results
//   GET  /api/v1/md/jobs/{name}/trajectory/{compoundId}   — proxy PDB frames from S3
//   GET  /api/v1/md/jobs/{name}/energy/{compoundId}       — proxy energy JSON from S3
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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Schema ---

func EnsureMDSchema(db *sql.DB) error {
	ddl := `CREATE TABLE IF NOT EXISTS md_jobs (
		id             INT AUTO_INCREMENT PRIMARY KEY,
		name           VARCHAR(255) NOT NULL UNIQUE,
		status         ENUM('Pending','Running','Completed','Failed') NOT NULL DEFAULT 'Pending',
		dock_job_name  VARCHAR(255) NOT NULL,
		receptor_ref   VARCHAR(255) NOT NULL,
		top_n          INT NOT NULL DEFAULT 5,
		md_nsteps      INT NOT NULL DEFAULT 500000,
		force_field    VARCHAR(64) NOT NULL DEFAULT 'amber99sb-ildn',
		ligand_ff      VARCHAR(32) NOT NULL DEFAULT 'gaff2',
		use_resp       TINYINT(1) NOT NULL DEFAULT 0,
		submitted_by   VARCHAR(255) NULL,
		error_output   TEXT NULL,
		input_data     JSON NULL,
		output_data    JSON NULL,
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at     TIMESTAMP NULL,
		completed_at   TIMESTAMP NULL,
		INDEX idx_status (status),
		INDEX idx_dock_job (dock_job_name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("creating md_jobs table: %w", err)
	}

	// md_results is created by the GROMACS worker directly; ensure it exists here
	// so the controller can query it without depending on a prior worker run.
	resDDL := `CREATE TABLE IF NOT EXISTS md_results (
		id                      BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		job_name                VARCHAR(128) NOT NULL,
		compound_id             VARCHAR(64)  NOT NULL,
		dock_engine             VARCHAR(32)  NOT NULL,
		dock_affinity_kcal_mol  DOUBLE       NULL,
		duration_s              INT          NULL,
		trajectory_s3_key       VARCHAR(512) NULL,
		energy_s3_key           VARCHAR(512) NULL,
		frames_s3_key           VARCHAR(512) NULL,
		energy_json_s3_key      VARCHAR(512) NULL,
		created_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_job_name (job_name),
		INDEX idx_compound (compound_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	if _, err := db.Exec(resDDL); err != nil {
		return fmt.Errorf("creating md_results table: %w", err)
	}

	// Add post-processing columns to existing deployments.
	for _, col := range []struct{ name, def string }{
		{"frames_s3_key", "VARCHAR(512) NULL"},
		{"energy_json_s3_key", "VARCHAR(512) NULL"},
	} {
		db.Exec(fmt.Sprintf(
			"ALTER TABLE md_results ADD COLUMN IF NOT EXISTS %s %s", col.name, col.def))
	}

	return nil
}

// --- Request / response types ---

type MDSubmitRequest struct {
	DockJobName     string  `json:"dock_job_name"`      // dockv2-* job name
	ReceptorRef     string  `json:"receptor_ref"`       // target-prep job name
	TopN            int     `json:"top_n"`              // number of top hits to simulate
	AffinityCutoff  float64 `json:"affinity_cutoff"`    // only consider compounds scoring ≤ this (e.g. -7.0); 0 = no cutoff
	MDNSteps        int     `json:"md_nsteps"`          // production MD steps (default 500000 = 1ns)
	ForceField      string  `json:"force_field"`        // amber99sb-ildn | amber14sb | charmm36m
	LigandFF        string  `json:"ligand_ff"`          // gaff2 | gaff
	UseRESP         bool    `json:"use_resp"`           // load CHELPG charges from khemeia-resp bucket
}

type MDSubmitResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	TopN   int    `json:"top_n"`
}

type MDCompoundProgress struct {
	CompoundID  string   `json:"compound_id"`
	Affinity    float64  `json:"dock_affinity_kcal_mol"`
	Status      string   `json:"status"` // Pending | Running | Completed | Failed
	DurationS   *int     `json:"duration_s,omitempty"`
	TrajectoryKey *string `json:"trajectory_s3_key,omitempty"`
	EnergyKey   *string  `json:"energy_s3_key,omitempty"`
}

type MDJobStatus struct {
	Name        string               `json:"name"`
	Status      string               `json:"status"`
	DockJobName string               `json:"dock_job_name"`
	ReceptorRef string               `json:"receptor_ref"`
	TopN        int                  `json:"top_n"`
	MDNSteps    int                  `json:"md_nsteps"`
	ForceField  string               `json:"force_field"`
	LigandFF    string               `json:"ligand_ff"`
	UseRESP     bool                 `json:"use_resp"`
	Compounds   []MDCompoundProgress `json:"compounds"`
	Completed   int                  `json:"completed"`
	Failed      int                  `json:"failed"`
	Progress    map[string]any       `json:"progress,omitempty"` // live phase/step from worker
	CreatedAt   time.Time            `json:"created_at"`
	StartedAt   *time.Time           `json:"started_at,omitempty"`
	CompletedAt *time.Time           `json:"completed_at,omitempty"`
	Error       *string              `json:"error,omitempty"`
}

type MDResultEntry struct {
	CompoundID      string  `json:"compound_id"`
	Affinity        float64 `json:"dock_affinity_kcal_mol"`
	DurationS       int     `json:"duration_s"`
	TrajectoryKey   string  `json:"trajectory_s3_key"`
	EnergyKey       string  `json:"energy_s3_key"`
	FramesKey       string  `json:"frames_s3_key,omitempty"`
	EnergyJSONKey   string  `json:"energy_json_s3_key,omitempty"`
	HasTrajectory   bool    `json:"has_trajectory"`
	HasEnergy       bool    `json:"has_energy"`
	CreatedAt       string  `json:"created_at"`
}

type MDResultsResponse struct {
	JobName string          `json:"job_name"`
	Total   int             `json:"total"`
	Results []MDResultEntry `json:"results"`
}

// --- HTTP dispatcher ---

func (h *APIHandler) MDDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/md/")
	path = strings.TrimRight(path, "/")

	if r.Method == http.MethodPost && path == "submit" {
		h.MDSubmit(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(path, "jobs/") && strings.HasSuffix(path, "/results") {
		h.MDResults(w, r)
		return
	}
	// GET /api/v1/md/jobs/{name}/trajectory/{compoundId}
	if strings.HasPrefix(path, "jobs/") && strings.Contains(path, "/trajectory/") {
		h.MDTrajectory(w, r)
		return
	}
	// GET /api/v1/md/jobs/{name}/energy/{compoundId}
	if strings.HasPrefix(path, "jobs/") && strings.Contains(path, "/energy/") {
		h.MDEnergy(w, r)
		return
	}
	if strings.HasPrefix(path, "jobs/") {
		h.MDJobStatus(w, r)
		return
	}
	writeError(w, "not found", http.StatusNotFound)
}

// MDSubmit handles POST /api/v1/md/submit.
func (h *APIHandler) MDSubmit(w http.ResponseWriter, r *http.Request) {
	var req MDSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.DockJobName == "" {
		writeError(w, "dock_job_name is required", http.StatusBadRequest)
		return
	}
	if req.ReceptorRef == "" {
		writeError(w, "receptor_ref is required", http.StatusBadRequest)
		return
	}
	if req.TopN <= 0 {
		req.TopN = 5
	}
	if req.MDNSteps <= 0 {
		req.MDNSteps = 500000
	}
	if req.ForceField == "" {
		req.ForceField = "amber99sb-ildn"
	}
	if req.LigandFF == "" {
		req.LigandFF = "gaff2"
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Verify docking job exists and is complete.
	var dockStatus string
	if err := db.QueryRowContext(r.Context(),
		`SELECT status FROM docking_v2_jobs WHERE name = ?`, req.DockJobName,
	).Scan(&dockStatus); err == sql.ErrNoRows {
		writeError(w, fmt.Sprintf("dock_job_name %q not found", req.DockJobName), http.StatusBadRequest)
		return
	} else if err != nil {
		writeError(w, fmt.Sprintf("failed to query docking job: %v", err), http.StatusInternalServerError)
		return
	}
	if dockStatus != "Completed" {
		writeError(w, fmt.Sprintf("docking job %q is not completed (status=%s)", req.DockJobName, dockStatus), http.StatusBadRequest)
		return
	}

	jobName := fmt.Sprintf("md-%d", time.Now().UnixNano())
	submittedBy := UserFromContext(r)
	inputJSON, _ := json.Marshal(req)

	_, err := db.ExecContext(r.Context(),
		`INSERT INTO md_jobs
			(name, status, dock_job_name, receptor_ref, top_n, md_nsteps,
			 force_field, ligand_ff, use_resp, submitted_by, input_data)
		 VALUES (?, 'Pending', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		jobName, req.DockJobName, req.ReceptorRef, req.TopN, req.MDNSteps,
		req.ForceField, req.LigandFF, req.UseRESP, submittedBy, string(inputJSON))
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create MD job: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(MDSubmitResponse{Name: jobName, Status: "Pending", TopN: req.TopN})

	go h.controller.RunMDJob(jobName, req)
}

// MDJobStatus handles GET /api/v1/md/jobs/{name}.
func (h *APIHandler) MDJobStatus(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/md/jobs/")
	jobName = strings.TrimSuffix(strings.TrimRight(jobName, "/"), "/results")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var s MDJobStatus
	var submittedBy, errOut, outputData sql.NullString
	var startedAt, completedAt sql.NullTime

	err := db.QueryRowContext(r.Context(),
		`SELECT name, status, dock_job_name, receptor_ref, top_n, md_nsteps,
		        force_field, ligand_ff, use_resp, error_output, output_data,
		        created_at, started_at, completed_at
		 FROM md_jobs WHERE name = ?`, jobName,
	).Scan(&s.Name, &s.Status, &s.DockJobName, &s.ReceptorRef, &s.TopN, &s.MDNSteps,
		&s.ForceField, &s.LigandFF, &s.UseRESP, &errOut, &outputData,
		&s.CreatedAt, &startedAt, &completedAt)
	if err == sql.ErrNoRows {
		writeError(w, "job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query MD job: %v", err), http.StatusInternalServerError)
		return
	}
	if submittedBy.Valid {
		_ = submittedBy
	}
	if errOut.Valid {
		s.Error = &errOut.String
	}
	if startedAt.Valid {
		s.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		s.CompletedAt = &completedAt.Time
	}
	if outputData.Valid && outputData.String != "" && s.Status == "Running" {
		var prog map[string]any
		if json.Unmarshal([]byte(outputData.String), &prog) == nil {
			s.Progress = prog
		}
	}

	// Fetch top-N compounds from docking results (best affinity per compound).
	topRows, err := db.QueryContext(r.Context(),
		`SELECT compound_id, MIN(affinity_kcal_mol) AS affinity_kcal_mol
		 FROM docking_v2_results
		 WHERE job_name = ?
		 GROUP BY compound_id
		 ORDER BY MIN(affinity_kcal_mol) ASC LIMIT ?`,
		s.DockJobName, s.TopN)
	if err == nil {
		defer topRows.Close()
		for topRows.Next() {
			var cp MDCompoundProgress
			topRows.Scan(&cp.CompoundID, &cp.Affinity)
			cp.Status = "Pending"
			s.Compounds = append(s.Compounds, cp)
		}
	}

	// Overlay with actual md_results.
	mdRows, err := db.QueryContext(r.Context(),
		`SELECT compound_id, duration_s, trajectory_s3_key, energy_s3_key
		 FROM md_results WHERE job_name = ?`, jobName)
	if err == nil {
		defer mdRows.Close()
		done := map[string]MDResultEntry{}
		for mdRows.Next() {
			var cid string
			var dur sql.NullInt64
			var traj, edr sql.NullString
			mdRows.Scan(&cid, &dur, &traj, &edr)
			e := MDResultEntry{}
			if dur.Valid {
				e.DurationS = int(dur.Int64)
			}
			if traj.Valid {
				e.TrajectoryKey = traj.String
			}
			done[cid] = e
		}
		for i, cp := range s.Compounds {
			if e, ok := done[cp.CompoundID]; ok {
				s.Compounds[i].Status = "Completed"
				s.Completed++
				d := e.DurationS
				s.Compounds[i].DurationS = &d
				if e.TrajectoryKey != "" {
					s.Compounds[i].TrajectoryKey = &e.TrajectoryKey
				}
			}
		}
	}

	if s.Compounds == nil {
		s.Compounds = []MDCompoundProgress{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// MDResults handles GET /api/v1/md/jobs/{name}/results.
func (h *APIHandler) MDResults(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/md/jobs/")
	jobName := strings.TrimSuffix(strings.TrimRight(path, "/"), "/results")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	rows, err := db.QueryContext(r.Context(),
		`SELECT compound_id, dock_affinity_kcal_mol, duration_s,
		        trajectory_s3_key, energy_s3_key,
		        COALESCE(frames_s3_key, ''), COALESCE(energy_json_s3_key, ''),
		        created_at
		 FROM md_results WHERE job_name = ?
		 ORDER BY dock_affinity_kcal_mol ASC`, jobName)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query results: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []MDResultEntry
	for rows.Next() {
		var e MDResultEntry
		var aff sql.NullFloat64
		var dur sql.NullInt64
		var traj, edr sql.NullString
		rows.Scan(&e.CompoundID, &aff, &dur, &traj, &edr, &e.FramesKey, &e.EnergyJSONKey, &e.CreatedAt)
		if aff.Valid {
			e.Affinity = aff.Float64
		}
		if dur.Valid {
			e.DurationS = int(dur.Int64)
		}
		if traj.Valid {
			e.TrajectoryKey = traj.String
		}
		if edr.Valid {
			e.EnergyKey = edr.String
		}
		e.HasTrajectory = e.FramesKey != ""
		e.HasEnergy = e.EnergyJSONKey != ""
		results = append(results, e)
	}
	if results == nil {
		results = []MDResultEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MDResultsResponse{JobName: jobName, Total: len(results), Results: results})
}

// MDTrajectory handles GET /api/v1/md/jobs/{name}/trajectory/{compoundId}.
// Looks up frames_s3_key in md_results and proxies the PDB content from S3.
func (h *APIHandler) MDTrajectory(w http.ResponseWriter, r *http.Request) {
	// path: jobs/{name}/trajectory/{compoundId}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/md/jobs/")
	parts := strings.SplitN(path, "/trajectory/", 2)
	if len(parts) != 2 {
		writeError(w, "invalid path", http.StatusBadRequest)
		return
	}
	jobName, compoundID := parts[0], parts[1]
	h.proxyMDS3(w, r, jobName, compoundID, "frames_s3_key", "application/octet-stream", "frames.pdb")
}

// MDEnergy handles GET /api/v1/md/jobs/{name}/energy/{compoundId}.
// Looks up energy_json_s3_key in md_results and proxies the JSON from S3.
func (h *APIHandler) MDEnergy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/md/jobs/")
	parts := strings.SplitN(path, "/energy/", 2)
	if len(parts) != 2 {
		writeError(w, "invalid path", http.StatusBadRequest)
		return
	}
	jobName, compoundID := parts[0], parts[1]
	h.proxyMDS3(w, r, jobName, compoundID, "energy_json_s3_key", "application/json", "energy.json")
}

// proxyMDS3 fetches an S3 key from md_results and streams it to the response.
func (h *APIHandler) proxyMDS3(w http.ResponseWriter, r *http.Request, jobName, compoundID, col, contentType, fallbackName string) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var s3Key sql.NullString
	err := db.QueryRowContext(r.Context(),
		fmt.Sprintf("SELECT %s FROM md_results WHERE job_name = ? AND compound_id = ? LIMIT 1", col),
		jobName, compoundID,
	).Scan(&s3Key)
	if err == sql.ErrNoRows {
		writeError(w, "result not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("db error: %v", err), http.StatusInternalServerError)
		return
	}
	if !s3Key.Valid || s3Key.String == "" {
		writeError(w, "file not available (post-processing may be pending)", http.StatusNotFound)
		return
	}

	rc, err := h.s3Client.GetArtifact(r.Context(), BucketTrajectories, s3Key.String)
	if err != nil {
		writeError(w, fmt.Sprintf("s3 fetch failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fallbackName))
	// io.Copy without import — use http.ServeContent workaround via ResponseWriter
	buf := make([]byte, 32*1024)
	for {
		n, readErr := rc.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
		}
		if readErr != nil {
			break
		}
	}
}

// --- Orchestration ---

func (c *Controller) RunMDJob(jobName string, req MDSubmitRequest) {
	log.Printf("[md] %s: starting MD orchestration (topN=%d, ff=%s/%s)", jobName, req.TopN, req.ForceField, req.LigandFF)

	db := c.firstDB()
	if db == nil {
		log.Printf("[md] %s: CRITICAL: no database", jobName)
		return
	}

	db.Exec(`UPDATE md_jobs SET status='Running', started_at=NOW() WHERE name=?`, jobName)

	// Fetch top-N compounds: best affinity per compound, with the engine that achieved it.
	// If AffinityCutoff is set (negative value), restrict to compounds scoring at or below it.
	var rows *sql.Rows
	var err error
	if req.AffinityCutoff < 0 {
		rows, err = db.Query(
			`SELECT r.compound_id, r.affinity_kcal_mol, r.engine
			 FROM docking_v2_results r
			 INNER JOIN (
			   SELECT compound_id, MIN(affinity_kcal_mol) AS min_aff
			   FROM docking_v2_results WHERE job_name = ?
			   GROUP BY compound_id
			   HAVING MIN(affinity_kcal_mol) <= ?
			 ) m ON r.compound_id = m.compound_id AND r.affinity_kcal_mol = m.min_aff
			 WHERE r.job_name = ?
			 ORDER BY r.affinity_kcal_mol ASC LIMIT ?`,
			req.DockJobName, req.AffinityCutoff, req.DockJobName, req.TopN)
	} else {
		rows, err = db.Query(
			`SELECT r.compound_id, r.affinity_kcal_mol, r.engine
			 FROM docking_v2_results r
			 INNER JOIN (
			   SELECT compound_id, MIN(affinity_kcal_mol) AS min_aff
			   FROM docking_v2_results WHERE job_name = ?
			   GROUP BY compound_id
			 ) m ON r.compound_id = m.compound_id AND r.affinity_kcal_mol = m.min_aff
			 WHERE r.job_name = ?
			 ORDER BY r.affinity_kcal_mol ASC LIMIT ?`,
			req.DockJobName, req.DockJobName, req.TopN)
	}
	if err != nil {
		c.failMDJob(db, jobName, fmt.Sprintf("failed to query top compounds: %v", err))
		return
	}

	type Candidate struct {
		CompoundID string
		Affinity   float64
		Engine     string
	}
	var candidates []Candidate
	for rows.Next() {
		var ca Candidate
		rows.Scan(&ca.CompoundID, &ca.Affinity, &ca.Engine)
		candidates = append(candidates, ca)
	}
	rows.Close()

	if len(candidates) == 0 {
		c.failMDJob(db, jobName, "docking job has no results")
		return
	}

	// Launch one GROMACS job per compound.
	ctx := context.Background()
	var launched []string
	for i, ca := range candidates {
		workerName := fmt.Sprintf("%s-%04d", jobName, i)
		if err := c.createMDWorker(ctx, workerName, jobName, ca.CompoundID, ca.Engine, req); err != nil {
			log.Printf("[md] %s: failed to launch worker for %s: %v", jobName, ca.CompoundID, err)
		} else {
			launched = append(launched, workerName)
		}
	}

	if len(launched) == 0 {
		c.failMDJob(db, jobName, "failed to launch any GROMACS workers")
		return
	}

	// Poll until all launched jobs finish (timeout: 4h per compound).
	timeout := time.Now().Add(time.Duration(len(launched)) * 4 * time.Hour)
	for {
		if time.Now().After(timeout) {
			c.failMDJob(db, jobName, "MD orchestration timed out")
			return
		}
		allDone := true
		for _, wn := range launched {
			job, err := c.jobClient.Get(ctx, wn, metav1.GetOptions{})
			if err != nil {
				continue
			}
			if job.Status.Active > 0 {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		time.Sleep(30 * time.Second)
	}

	// Count results.
	var resultCount int
	db.QueryRow(`SELECT COUNT(*) FROM md_results WHERE job_name = ?`, jobName).Scan(&resultCount)

	outputJSON, _ := json.Marshal(map[string]interface{}{
		"compounds_launched": len(launched),
		"results_written":    resultCount,
	})
	db.Exec(
		`UPDATE md_jobs SET status='Completed', output_data=?, completed_at=NOW() WHERE name=?`,
		string(outputJSON), jobName)

	log.Printf("[md] %s: completed, %d results", jobName, resultCount)
}

func (c *Controller) failMDJob(db *sql.DB, jobName, msg string) {
	db.Exec(`UPDATE md_jobs SET status='Failed', error_output=?, completed_at=NOW() WHERE name=?`, msg, jobName)
	log.Printf("[md] %s: FAILED: %s", jobName, msg)
}

func (c *Controller) createMDWorker(
	ctx context.Context,
	workerName, jobName, compoundID, dockEngine string,
	req MDSubmitRequest,
) error {
	backoffLimit := int32(0)
	ttl := int32(86400)

	gpuLimit := resource.MustParse("1")
	memLimit := resource.MustParse("16Gi")
	cpuReq := resource.MustParse("4")

	useRESP := "false"
	if req.UseRESP {
		useRESP = "true"
	}

	env := []corev1.EnvVar{
		{Name: "JOB_NAME", Value: jobName},
		{Name: "WORKER_NAME", Value: workerName},
		{Name: "COMPOUND_ID", Value: compoundID},
		{Name: "RECEPTOR_REF", Value: req.ReceptorRef},
		{Name: "DOCK_JOB_NAME", Value: req.DockJobName},
		{Name: "DOCK_ENGINE", Value: dockEngine},
		{Name: "MD_NSTEPS", Value: strconv.Itoa(req.MDNSteps)},
		{Name: "FORCE_FIELD", Value: req.ForceField},
		{Name: "LIGAND_FF", Value: req.LigandFF},
		{Name: "USE_RESP", Value: useRESP},
		{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
		{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
		{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
		{Name: "MYSQL_DATABASE", Value: "docking"},
		{Name: "MYSQL_PASSWORD", Value: os.Getenv("MYSQL_PASSWORD")},
		{Name: "GARAGE_ENABLED", Value: "true"},
		{Name: "GARAGE_ENDPOINT", Value: os.Getenv("GARAGE_ENDPOINT")},
		{Name: "GARAGE_ACCESS_KEY", Value: os.Getenv("GARAGE_ACCESS_KEY")},
		{Name: "GARAGE_SECRET_KEY", Value: os.Getenv("GARAGE_SECRET_KEY")},
		{Name: "GARAGE_REGION", Value: os.Getenv("GARAGE_REGION")},
		{Name: "LD_LIBRARY_PATH", Value: "/run/opengl-driver/lib:/usr/local/cuda/lib64"},
	}

	hostPathType := corev1.HostPathDirectory
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerName,
			Namespace: c.namespace,
			Labels: map[string]string{
				"khemeia.io/job-name":   jobName,
				"khemeia.io/role":       "md-worker",
				"khemeia.io/compound":   compoundID,
				"khemeia.io/managed-by": "md",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					NodeSelector:     map[string]string{"gpu": "rtx3070"},
					Tolerations: []corev1.Toleration{
						{Key: "gpu", Value: "true", Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{
						{
							Name:            "gromacs",
							Image:           "zot.hwcopeland.net/chem/gromacs:latest",
							ImagePullPolicy: corev1.PullAlways,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    cpuReq,
									corev1.ResourceMemory: memLimit,
									"nvidia.com/gpu":       gpuLimit,
								},
								Limits: corev1.ResourceList{
									"nvidia.com/gpu": gpuLimit,
								},
							},
							Env: env,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "nvidia-driver", MountPath: "/run/opengl-driver", ReadOnly: true},
								{Name: "nix-store", MountPath: "/nix/store", ReadOnly: true},
								{Name: "dri", MountPath: "/dev/dri"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "nvidia-driver",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/run/opengl-driver", Type: &hostPathType},
							},
						},
						{
							Name: "nix-store",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/nix/store", Type: &hostPathType},
							},
						},
						{
							Name: "dri",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/dev/dri", Type: &hostPathType},
							},
						},
					},
				},
			},
		},
	}

	_, err := c.jobClient.Create(ctx, job, metav1.CreateOptions{})
	return err
}

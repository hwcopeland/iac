// Package main provides HTTP handlers for the docking job API
package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// APIHandler handles HTTP requests for docking jobs
type APIHandler struct {
	client     *kubernetes.Clientset
	namespace  string
	controller *DockingJobController
	db         *sql.DB   // docking database
	qeDb       *sql.DB   // qe database
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(client *kubernetes.Clientset, namespace string, controller *DockingJobController, db, qeDb *sql.DB) *APIHandler {
	return &APIHandler{
		client:     client,
		namespace:  namespace,
		controller: controller,
		qeDb:       qeDb,
		db:         db,
	}
}

// DockingJobRequest represents a request to create a new docking job
type DockingJobRequest struct {
	PDBID            string `json:"pdbid"`
	LigandDb         string `json:"ligand_db"`
	NativeLigand     string `json:"native_ligand"`
	LigandsChunkSize int    `json:"ligands_chunk_size"`
	Image            string `json:"image"`
}

// DockingJobResponse represents a response containing docking job information
type DockingJobResponse struct {
	Name             string     `json:"name"`
	PDBID            string     `json:"pdbid"`
	LigandDb         string     `json:"ligand_db"`
	Status           string     `json:"status"`
	BatchCount       int        `json:"batch_count"`
	CompletedBatches int        `json:"completed_batches"`
	Message          string     `json:"message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartTime        *time.Time `json:"start_time,omitempty"`
	CompletionTime   *time.Time `json:"completion_time,omitempty"`
}

// WorkflowListItem represents a single workflow in the list response
type WorkflowListItem struct {
	Name             string    `json:"name"`
	Phase            string    `json:"phase"`
	PDBID            string    `json:"pdbid"`
	SourceDb         string    `json:"source_db"`
	BatchCount       int       `json:"batch_count"`
	CompletedBatches int       `json:"completed_batches"`
	CreatedAt        time.Time `json:"created_at"`
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ListJobs handles GET /api/v1/dockingjobs
func (h *APIHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT name, phase, pdbid, source_db, batch_count, completed_batches, created_at
		   FROM docking_workflows ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to list workflows: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var workflows []WorkflowListItem
	for rows.Next() {
		var wf WorkflowListItem
		if err := rows.Scan(&wf.Name, &wf.Phase, &wf.PDBID, &wf.SourceDb,
			&wf.BatchCount, &wf.CompletedBatches, &wf.CreatedAt); err != nil {
			writeError(w, fmt.Sprintf("failed to scan workflow: %v", err), http.StatusInternalServerError)
			return
		}
		workflows = append(workflows, wf)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("failed to iterate workflows: %v", err), http.StatusInternalServerError)
		return
	}

	// Return empty array instead of null when no workflows exist.
	if workflows == nil {
		workflows = []WorkflowListItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"workflows": workflows,
		"count":     len(workflows),
	})
}

// CreateJob handles POST /api/v1/dockingjobs
func (h *APIHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req DockingJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.LigandDb == "" {
		writeError(w, "ligand_db is required", http.StatusBadRequest)
		return
	}
	if req.PDBID == "" {
		req.PDBID = DefaultPDBID
	}
	if req.NativeLigand == "" {
		req.NativeLigand = DefaultNativeLigand
	}
	if req.Image == "" {
		req.Image = DefaultImage
	}
	if req.LigandsChunkSize == 0 {
		req.LigandsChunkSize = DefaultLigandsChunkSize
	}

	// Validate that ligands exist for the given source_db.
	var ligandCount int
	err := h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM ligands WHERE source_db = ?`, req.LigandDb).Scan(&ligandCount)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to check ligands: %v", err), http.StatusInternalServerError)
		return
	}
	if ligandCount == 0 {
		writeError(w, fmt.Sprintf("no ligands found for source_db '%s'", req.LigandDb), http.StatusBadRequest)
		return
	}

	jobName := fmt.Sprintf("docking-%d", time.Now().UnixNano())
	submittedBy := UserFromContext(r)

	// INSERT workflow row with phase='Running'.
	_, err = h.db.ExecContext(r.Context(),
		`INSERT INTO docking_workflows (name, phase, pdbid, source_db, native_ligand, chunk_size, image, submitted_by)
		 VALUES (?, 'Running', ?, ?, ?, ?, ?, ?)`,
		jobName, req.PDBID, req.LigandDb, req.NativeLigand, req.LigandsChunkSize, req.Image, submittedBy)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create workflow: %v", err), http.StatusInternalServerError)
		return
	}

	job := DockingJob{
		Name: jobName,
		Spec: DockingJobSpec{
			PDBID:            req.PDBID,
			LigandDb:         req.LigandDb,
			NativeLigand:     req.NativeLigand,
			LigandsChunkSize: req.LigandsChunkSize,
			Image:            req.Image,
		},
		Status: DockingJobStatus{Phase: "Running"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(DockingJobResponse{
		Name:      jobName,
		PDBID:     req.PDBID,
		LigandDb:  req.LigandDb,
		Status:    "Running",
		CreatedAt: time.Now(),
	})

	go h.controller.processDockingJob(job)
}

// GetJob handles GET /api/v1/dockingjobs/{name}
func (h *APIHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/dockingjobs/")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	var resp DockingJobResponse
	var startTime, completionTime sql.NullTime
	var message sql.NullString

	err := h.db.QueryRowContext(r.Context(),
		`SELECT name, phase, pdbid, source_db, batch_count, completed_batches,
		        message, created_at, started_at, completed_at
		   FROM docking_workflows WHERE name = ?`, jobName).Scan(
		&resp.Name, &resp.Status, &resp.PDBID, &resp.LigandDb,
		&resp.BatchCount, &resp.CompletedBatches,
		&message, &resp.CreatedAt, &startTime, &completionTime)
	if err == sql.ErrNoRows {
		writeError(w, "workflow not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get workflow: %v", err), http.StatusInternalServerError)
		return
	}

	if message.Valid {
		resp.Message = message.String
	}
	if startTime.Valid {
		resp.StartTime = &startTime.Time
	}
	if completionTime.Valid {
		resp.CompletionTime = &completionTime.Time
	}

	log.Printf("[GetJob] %s: status=%s batch=%d completed=%d",
		jobName, resp.Status, resp.BatchCount, resp.CompletedBatches)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// DeleteJob handles DELETE /api/v1/dockingjobs/{name}
func (h *APIHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/dockingjobs/")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	// Delete associated K8s Jobs by label (cleanup running/completed pods).
	jobs, err := h.client.BatchV1().Jobs(h.namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("docking.khemia.io/parent-job=%s", jobName),
	})
	if err != nil {
		writeError(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
		return
	}

	for _, job := range jobs.Items {
		propagation := metav1.DeletePropagationBackground
		if err := h.client.BatchV1().Jobs(h.namespace).Delete(r.Context(), job.Name, metav1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			log.Printf("Failed to delete job %s: %v", job.Name, err)
		}
	}

	// Delete from MySQL tables (staging first, then results, then workflow).
	if _, err := h.db.ExecContext(r.Context(),
		`DELETE FROM staging WHERE job_type = 'dock' AND JSON_EXTRACT(payload, '$.workflow_name') = ?`, jobName); err != nil {
		log.Printf("Failed to delete staging rows for workflow %s: %v", jobName, err)
	}

	if _, err := h.db.ExecContext(r.Context(),
		`DELETE FROM docking_results WHERE workflow_name = ?`, jobName); err != nil {
		log.Printf("Failed to delete results for workflow %s: %v", jobName, err)
	}

	if _, err := h.db.ExecContext(r.Context(),
		`DELETE FROM docking_workflows WHERE name = ?`, jobName); err != nil {
		log.Printf("Failed to delete workflow %s: %v", jobName, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetLogs handles GET /api/v1/dockingjobs/{name}/logs
func (h *APIHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/dockingjobs/")
	jobName := strings.TrimSuffix(trimmed, "/logs")
	taskType := r.URL.Query().Get("task")

	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	labelSelector := fmt.Sprintf("docking.khemia.io/parent-job=%s", jobName)
	if taskType != "" {
		labelSelector = fmt.Sprintf("docking.khemia.io/parent-job=%s,docking.khemia.io/job-type=%s", jobName, taskType)
	}

	jobs, err := h.client.BatchV1().Jobs(h.namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil || len(jobs.Items) == 0 {
		writeError(w, "job not found", http.StatusNotFound)
		return
	}

	pods, err := h.client.CoreV1().Pods(h.namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobs.Items[0].Name),
	})
	if err != nil || len(pods.Items) == 0 {
		writeError(w, "pods not found", http.StatusNotFound)
		return
	}

	logs, err := h.client.CoreV1().Pods(h.namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).
		Do(r.Context()).Raw()
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(logs)
}

// DockingResultsResponse holds aggregated MySQL stats for a workflow
type DockingResultsResponse struct {
	WorkflowName         string  `json:"workflow_name"`
	ResultCount          int     `json:"result_count"`
	BestAffinityKcalMol  float64 `json:"best_affinity_kcal_mol,omitempty"`
	WorstAffinityKcalMol float64 `json:"worst_affinity_kcal_mol,omitempty"`
	AvgAffinityKcalMol   float64 `json:"avg_affinity_kcal_mol,omitempty"`
}

// GetResults handles GET /api/v1/dockingjobs/{name}/results
// Returns aggregated docking affinity stats from MySQL for the given workflow.
func (h *APIHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/dockingjobs/")
	jobName := strings.TrimSuffix(trimmed, "/results")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	var count int
	var best, worst, avg sql.NullFloat64
	row := h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*), MIN(affinity_kcal_mol), MAX(affinity_kcal_mol), AVG(affinity_kcal_mol)
		   FROM docking_results WHERE workflow_name = ?`, jobName)
	if err := row.Scan(&count, &best, &worst, &avg); err != nil {
		writeError(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
		return
	}

	resp := DockingResultsResponse{
		WorkflowName: jobName,
		ResultCount:  count,
	}
	if best.Valid {
		resp.BestAffinityKcalMol = best.Float64
		resp.WorstAffinityKcalMol = worst.Float64
		resp.AvgAffinityKcalMol = avg.Float64
	}

	log.Printf("[GetResults] %s: count=%d best=%.2f worst=%.2f avg=%.2f",
		jobName, count, resp.BestAffinityKcalMol, resp.WorstAffinityKcalMol, resp.AvgAffinityKcalMol)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HealthCheck handles GET /health
func (h *APIHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// LigandImportRequest represents a single ligand to import.
type LigandImportRequest struct {
	CompoundID string `json:"compound_id"`
	Smiles     string `json:"smiles"`
	PDBQTB64   string `json:"pdbqt_b64,omitempty"` // base64-encoded PDBQT, optional
	SourceDb   string `json:"source_db"`
}

// ImportLigands handles POST /api/v1/ligands
// Accepts a JSON array of ligands and upserts them into the ligands table.
func (h *APIHandler) ImportLigands(w http.ResponseWriter, r *http.Request) {
	var ligands []LigandImportRequest
	if err := json.NewDecoder(r.Body).Decode(&ligands); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if len(ligands) == 0 {
		writeError(w, "empty ligand list", http.StatusBadRequest)
		return
	}

	imported := 0
	for _, lig := range ligands {
		if lig.CompoundID == "" || lig.Smiles == "" || lig.SourceDb == "" {
			continue
		}

		var pdbqt []byte
		if lig.PDBQTB64 != "" {
			var err error
			pdbqt, err = base64Decode(lig.PDBQTB64)
			if err != nil {
				log.Printf("[ImportLigands] bad base64 for %s: %v", lig.CompoundID, err)
				continue
			}
		}

		_, err := h.db.ExecContext(r.Context(),
			`INSERT INTO ligands (compound_id, smiles, pdbqt, source_db)
			 VALUES (?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE smiles = VALUES(smiles), pdbqt = VALUES(pdbqt)`,
			lig.CompoundID, lig.Smiles, pdbqt, lig.SourceDb)
		if err != nil {
			log.Printf("[ImportLigands] failed to upsert %s: %v", lig.CompoundID, err)
			continue
		}
		imported++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"imported": imported,
		"total":    len(ligands),
	})
}

// base64Decode decodes a base64 string.
func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// --- Quantum ESPRESSO handlers ---

// QESubmitRequest represents a request to submit a QE calculation.
type QESubmitRequest struct {
	Executable string `json:"executable,omitempty"`
	InputFile  string `json:"input_file"`
	NumCPUs    int    `json:"num_cpus,omitempty"`
	MemoryMB   int    `json:"memory_mb,omitempty"`
	Image      string `json:"image,omitempty"`
}

// QEJobSummary is the list-level view (omits large text fields).
type QEJobSummary struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Executable  string     `json:"executable"`
	TotalEnergy *float64   `json:"total_energy,omitempty"`
	WallTimeSec *float32   `json:"wall_time_sec,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// QEJobDetail is the full view returned by GetQEJob.
type QEJobDetail struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Executable  string     `json:"executable"`
	InputFile   string     `json:"input_file"`
	OutputFile  *string    `json:"output_file,omitempty"`
	ErrorOutput *string    `json:"error_output,omitempty"`
	TotalEnergy *float64   `json:"total_energy,omitempty"`
	WallTimeSec *float32   `json:"wall_time_sec,omitempty"`
	NumCPUs     int        `json:"num_cpus"`
	MemoryMB    int        `json:"memory_mb"`
	Image       string     `json:"image"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// SubmitQEJob handles POST /api/v1/qe/submit
func (h *APIHandler) SubmitQEJob(w http.ResponseWriter, r *http.Request) {
	var req QESubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.InputFile) == "" {
		writeError(w, "input_file is required", http.StatusBadRequest)
		return
	}

	// Apply defaults.
	if req.Executable == "" {
		req.Executable = DefaultQEExecutable
	}
	if req.NumCPUs == 0 {
		req.NumCPUs = DefaultQENumCPUs
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = DefaultQEMemoryMB
	}
	if req.Image == "" {
		req.Image = DefaultQEImage
	}

	jobName := fmt.Sprintf("qe-%d", time.Now().UnixNano())
	submittedBy := UserFromContext(r)

	_, err := h.qeDb.ExecContext(r.Context(),
		`INSERT INTO qe_jobs (name, executable, input_file, num_cpus, memory_mb, image, submitted_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		jobName, req.Executable, req.InputFile, req.NumCPUs, req.MemoryMB, req.Image, submittedBy)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create QE job: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":   jobName,
		"status": "Pending",
	})

	go h.controller.processQEJob(jobName, req.Executable, req.InputFile, req.Image, req.NumCPUs, req.MemoryMB)
}

// ListQEJobs handles GET /api/v1/qe/jobs
func (h *APIHandler) ListQEJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.qeDb.QueryContext(r.Context(),
		`SELECT id, name, status, executable, total_energy, wall_time_sec, created_at, completed_at
		   FROM qe_jobs ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to list QE jobs: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var jobs []QEJobSummary
	for rows.Next() {
		var j QEJobSummary
		var totalEnergy sql.NullFloat64
		var wallTime sql.NullFloat64
		var completedAt sql.NullTime

		if err := rows.Scan(&j.ID, &j.Name, &j.Status, &j.Executable,
			&totalEnergy, &wallTime, &j.CreatedAt, &completedAt); err != nil {
			writeError(w, fmt.Sprintf("failed to scan QE job: %v", err), http.StatusInternalServerError)
			return
		}
		if totalEnergy.Valid {
			j.TotalEnergy = &totalEnergy.Float64
		}
		if wallTime.Valid {
			wt := float32(wallTime.Float64)
			j.WallTimeSec = &wt
		}
		if completedAt.Valid {
			j.CompletedAt = &completedAt.Time
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("failed to iterate QE jobs: %v", err), http.StatusInternalServerError)
		return
	}

	if jobs == nil {
		jobs = []QEJobSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobs":  jobs,
		"count": len(jobs),
	})
}

// GetQEJob handles GET /api/v1/qe/jobs/{name}
func (h *APIHandler) GetQEJob(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/qe/jobs/")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	var j QEJobDetail
	var outputFile, errorOutput sql.NullString
	var totalEnergy sql.NullFloat64
	var wallTime sql.NullFloat64
	var startedAt, completedAt sql.NullTime

	err := h.qeDb.QueryRowContext(r.Context(),
		`SELECT id, name, status, executable, input_file, output_file, error_output,
		        total_energy, wall_time_sec, num_cpus, memory_mb, image,
		        created_at, started_at, completed_at
		   FROM qe_jobs WHERE name = ?`, jobName).Scan(
		&j.ID, &j.Name, &j.Status, &j.Executable, &j.InputFile, &outputFile, &errorOutput,
		&totalEnergy, &wallTime, &j.NumCPUs, &j.MemoryMB, &j.Image,
		&j.CreatedAt, &startedAt, &completedAt)
	if err == sql.ErrNoRows {
		writeError(w, "QE job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get QE job: %v", err), http.StatusInternalServerError)
		return
	}

	if outputFile.Valid {
		j.OutputFile = &outputFile.String
	}
	if errorOutput.Valid {
		j.ErrorOutput = &errorOutput.String
	}
	if totalEnergy.Valid {
		j.TotalEnergy = &totalEnergy.Float64
	}
	if wallTime.Valid {
		wt := float32(wallTime.Float64)
		j.WallTimeSec = &wt
	}
	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}

// DeleteQEJob handles DELETE /api/v1/qe/jobs/{name}
func (h *APIHandler) DeleteQEJob(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/qe/jobs/")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	// Delete the K8s Job if it still exists.
	propagation := metav1.DeletePropagationBackground
	if err := h.client.BatchV1().Jobs(h.namespace).Delete(r.Context(), jobName, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	}); err != nil && !isNotFound(err) {
		log.Printf("[DeleteQEJob] Failed to delete K8s job %s: %v", jobName, err)
	}

	// Delete the input ConfigMap if it still exists.
	cmName := fmt.Sprintf("qe-input-%s", jobName)
	if err := h.client.CoreV1().ConfigMaps(h.namespace).Delete(r.Context(), cmName, metav1.DeleteOptions{}); err != nil && !isNotFound(err) {
		log.Printf("[DeleteQEJob] Failed to delete ConfigMap %s: %v", cmName, err)
	}

	// Delete from MySQL.
	if _, err := h.qeDb.ExecContext(r.Context(),
		`DELETE FROM qe_jobs WHERE name = ?`, jobName); err != nil {
		log.Printf("[DeleteQEJob] Failed to delete QE job %s from DB: %v", jobName, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// isNotFound checks if a Kubernetes API error is a NotFound error.
func isNotFound(err error) bool {
	return k8serrors.IsNotFound(err)
}

// UploadPseudopotential handles POST /api/v1/qe/pseudopotentials
// Accepts a JSON object with filename, content (base64), element, functional.
func (h *APIHandler) UploadPseudopotential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename   string `json:"filename"`
		ContentB64 string `json:"content_b64"`
		Element    string `json:"element"`
		Functional string `json:"functional,omitempty"`
		SourceURL  string `json:"source_url,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if req.Filename == "" || req.ContentB64 == "" || req.Element == "" {
		writeError(w, "filename, content_b64, and element are required", http.StatusBadRequest)
		return
	}

	content, err := base64Decode(req.ContentB64)
	if err != nil {
		writeError(w, fmt.Sprintf("invalid base64: %v", err), http.StatusBadRequest)
		return
	}

	_, err = h.qeDb.ExecContext(r.Context(),
		`INSERT INTO pseudopotentials (filename, content, element, functional, source_url)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE content = VALUES(content), functional = VALUES(functional)`,
		req.Filename, content, req.Element, req.Functional, req.SourceURL)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to store pseudopotential: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filename": req.Filename,
		"size":     len(content),
	})
}

// ListPseudopotentials handles GET /api/v1/qe/pseudopotentials
func (h *APIHandler) ListPseudopotentials(w http.ResponseWriter, r *http.Request) {
	rows, err := h.qeDb.QueryContext(r.Context(),
		`SELECT filename, element, functional, LENGTH(content) as size, created_at FROM pseudopotentials ORDER BY element, filename`)
	if err != nil {
		writeError(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ppEntry struct {
		Filename   string    `json:"filename"`
		Element    string    `json:"element"`
		Functional *string   `json:"functional,omitempty"`
		Size       int       `json:"size_bytes"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var pps []ppEntry
	for rows.Next() {
		var p ppEntry
		if err := rows.Scan(&p.Filename, &p.Element, &p.Functional, &p.Size, &p.CreatedAt); err != nil {
			continue
		}
		pps = append(pps, p)
	}
	if pps == nil {
		pps = []ppEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pseudopotentials": pps,
		"count":            len(pps),
	})
}

// PrepRequest represents a request to prep ligands for docking.
type PrepRequest struct {
	SourceDb  string `json:"source_db"`
	ChunkSize int    `json:"chunk_size,omitempty"` // defaults to 500
	Image     string `json:"image,omitempty"`
}

// StartPrep handles POST /api/v1/prep
// Counts unprepared ligands for the given source_db, then launches batch prep jobs.
func (h *APIHandler) StartPrep(w http.ResponseWriter, r *http.Request) {
	var req PrepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.SourceDb == "" {
		writeError(w, "source_db is required", http.StatusBadRequest)
		return
	}

	// Count unprepared ligands (those without PDBQT data).
	var unpreparedCount int
	err := h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NULL`,
		req.SourceDb).Scan(&unpreparedCount)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to count unprepared ligands: %v", err), http.StatusInternalServerError)
		return
	}

	if unpreparedCount == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "all ligands already prepped",
			"count":   0,
		})
		return
	}

	// Apply defaults.
	if req.ChunkSize == 0 {
		req.ChunkSize = 500
	}
	if req.Image == "" {
		req.Image = DefaultImage
	}

	batchCount := int(math.Ceil(float64(unpreparedCount) / float64(req.ChunkSize)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "prep started",
		"source_db":  req.SourceDb,
		"unprepared": unpreparedCount,
		"batches":    batchCount,
	})

	go h.controller.processLigandPrep(req)
}

// CreateAPIToken handles POST /api/v1/tokens — generates a new API token.
func (h *APIHandler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username      string `json:"username"`
		ExpiresInHours int   `json:"expires_in_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Username == "" {
		writeError(w, "username is required", http.StatusBadRequest)
		return
	}
	if req.ExpiresInHours <= 0 {
		req.ExpiresInHours = 72 // default 72 hours
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(time.Duration(req.ExpiresInHours) * time.Hour)
	_, err = h.db.Exec(
		"INSERT INTO api_tokens (token, username, expires_at) VALUES (?, ?, ?)",
		token, req.Username, expiresAt,
	)
	if err != nil {
		log.Printf("[CreateAPIToken] DB error: %v", err)
		writeError(w, "failed to store token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"username":   req.Username,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

// ListAPITokens handles GET /api/v1/tokens — lists active tokens (token value redacted).
func (h *APIHandler) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		"SELECT id, username, created_at, expires_at FROM api_tokens WHERE expires_at > NOW() ORDER BY created_at DESC",
	)
	if err != nil {
		writeError(w, "failed to list tokens", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type tokenEntry struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		CreatedAt string `json:"created_at"`
		ExpiresAt string `json:"expires_at"`
	}
	var tokens []tokenEntry
	for rows.Next() {
		var t tokenEntry
		var createdAt, expiresAt time.Time
		if err := rows.Scan(&t.ID, &t.Username, &createdAt, &expiresAt); err != nil {
			continue
		}
		t.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		t.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
		tokens = append(tokens, t)
	}
	if tokens == nil {
		tokens = []tokenEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"tokens": tokens})
}

// RevokeAPIToken handles DELETE /api/v1/tokens/{id} — revokes a token by ID.
func (h *APIHandler) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	// Extract token ID from path: /api/v1/tokens/123
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/")
	if path == "" {
		writeError(w, "token id required", http.StatusBadRequest)
		return
	}

	result, err := h.db.Exec("DELETE FROM api_tokens WHERE id = ?", path)
	if err != nil {
		writeError(w, "failed to revoke token", http.StatusInternalServerError)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, "token not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

// ReadinessCheck handles GET /readyz
func (h *APIHandler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	if err := h.db.PingContext(r.Context()); err != nil {
		log.Printf("[ReadinessCheck] MySQL ping failed: %v", err)
		writeError(w, "database not reachable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

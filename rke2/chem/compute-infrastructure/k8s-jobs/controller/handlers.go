// Package main provides HTTP handlers for the docking job API
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// APIHandler handles HTTP requests for docking jobs
type APIHandler struct {
	client     *kubernetes.Clientset
	namespace  string
	controller *DockingJobController
	db         *sql.DB
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(client *kubernetes.Clientset, namespace string, controller *DockingJobController, db *sql.DB) *APIHandler {
	return &APIHandler{
		client:     client,
		namespace:  namespace,
		controller: controller,
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

	jobName := fmt.Sprintf("docking-%d", time.Now().Unix())

	// INSERT workflow row with phase='Running'.
	_, err = h.db.ExecContext(r.Context(),
		`INSERT INTO docking_workflows (name, phase, pdbid, source_db, native_ligand, chunk_size, image)
		 VALUES (?, 'Running', ?, ?, ?, ?, ?)`,
		jobName, req.PDBID, req.LigandDb, req.NativeLigand, req.LigandsChunkSize, req.Image)
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

// Package main provides HTTP handlers for the docking job API
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// APIHandler handles HTTP requests for docking jobs
type APIHandler struct {
	client     *kubernetes.Clientset
	namespace  string
	controller *DockingJobController
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(client *kubernetes.Clientset, namespace string, controller *DockingJobController) *APIHandler {
	return &APIHandler{
		client:     client,
		namespace:  namespace,
		controller: controller,
	}
}

// DockingJobRequest represents a request to create a new docking job
type DockingJobRequest struct {
	PDBID            string `json:"pdbid"`
	LigandDb         string `json:"ligand_db"`
	JupyterUser      string `json:"jupyter_user"`
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

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ListJobs handles GET /api/v1/dockingjobs
func (h *APIHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.client.BatchV1().Jobs(h.namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: "docking.khemia.io/parent-job",
	})
	if err != nil {
		writeError(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
		return
	}

	workflows := make(map[string][]string)
	for _, job := range jobs.Items {
		parentJob := job.Labels["docking.khemia.io/parent-job"]
		if parentJob != "" {
			workflows[parentJob] = append(workflows[parentJob], job.Name)
		}
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
	if req.JupyterUser == "" {
		req.JupyterUser = DefaultJupyterUser
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

	jobName := fmt.Sprintf("docking-%d", time.Now().Unix())

	job := DockingJob{
		ObjectMeta: metav1.ObjectMeta{Name: jobName},
		Spec: DockingJobSpec{
			PDBID:            req.PDBID,
			LigandDb:         req.LigandDb,
			JupyterUser:      req.JupyterUser,
			NativeLigand:     req.NativeLigand,
			LigandsChunkSize: req.LigandsChunkSize,
			Image:            req.Image,
			AutodockPvc:      DefaultAutodockPvc,
			UserPvcPrefix:    DefaultUserPvcPrefix,
			MountPath:        DefaultMountPath,
		},
		Status: DockingJobStatus{Phase: "Pending"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(DockingJobResponse{
		Name:      jobName,
		PDBID:     req.PDBID,
		LigandDb:  req.LigandDb,
		Status:    "Pending",
		CreatedAt: time.Now(),
	})

	go h.controller.processDockingJob(job)
}

// GetJob handles GET /api/v1/dockingjobs/{name}
//
// Status is derived from child K8s jobs:
//   - "Pending"   – no child jobs exist yet
//   - "Running"   – child jobs exist but postprocessing has not succeeded
//   - "Failed"    – any child job has a failed pod
//   - "Completed" – the postprocessing job has succeeded (terminal step)
//
// The message field on Completed contains the best-energy result from
// postprocessing logs (e.g. "Best energy: -8.45").
func (h *APIHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/dockingjobs/")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	jobs, err := h.client.BatchV1().Jobs(h.namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("docking.khemia.io/parent-job=%s", jobName),
	})
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
		return
	}

	total := len(jobs.Items)
	if total == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DockingJobResponse{Name: jobName, Status: "Pending"})
		return
	}

	completed := 0
	status := "Running"
	message := ""

	for _, job := range jobs.Items {
		if job.Status.Failed > 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DockingJobResponse{
				Name:             jobName,
				Status:           "Failed",
				CompletedBatches: completed,
				BatchCount:       total,
				Message:          fmt.Sprintf("job %s failed", job.Name),
			})
			return
		}
		if job.Status.Succeeded > 0 {
			completed++
			if job.Labels["docking.khemia.io/job-type"] == "postprocessing" {
				status = "Completed"
				// Prefer the in-memory cache populated at pipeline completion time
				// (avoids races with the job's 5-minute TTL cleanup).
				if cached, ok := h.controller.results.Load(jobName); ok {
					message = cached.(string)
					log.Printf("[GetJob] %s: using cached result: %q", jobName, message)
				} else {
					log.Printf("[GetJob] %s: no cached result, fetching from pod logs", jobName)
					message = h.postprocessingResult(r.Context(), job.Name)
				}
			}
		}
	}

	log.Printf("[GetJob] %s: status=%s total_k8s_jobs=%d completed=%d message=%q",
		jobName, status, total, completed, message)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DockingJobResponse{
		Name:             jobName,
		Status:           status,
		CompletedBatches: completed,
		BatchCount:       total,
		Message:          message,
	})
}

// postprocessingResult fetches the "Best energy: ..." line from postprocessing pod logs.
// This is the fallback path used when the in-memory cache is unavailable (e.g. after
// a controller restart). Prefer the cache when possible.
func (h *APIHandler) postprocessingResult(ctx context.Context, jobName string) string {
	log.Printf("[postprocessingResult] fetching logs from job %s", jobName)
	pods, err := h.client.CoreV1().Pods(h.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil || len(pods.Items) == 0 {
		log.Printf("[postprocessingResult] no pods found for job %s: err=%v", jobName, err)
		return ""
	}
	podName := pods.Items[0].Name
	log.Printf("[postprocessingResult] reading logs from pod %s", podName)
	raw, err := h.client.CoreV1().Pods(h.namespace).GetLogs(podName, &corev1.PodLogOptions{}).
		Do(ctx).Raw()
	if err != nil {
		log.Printf("[postprocessingResult] GetLogs error for pod %s: %v", podName, err)
		return ""
	}
	log.Printf("[postprocessingResult] pod %s logs (%d bytes):\n%s", podName, len(raw), strings.TrimSpace(string(raw)))
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.HasPrefix(line, "Best energy:") {
			log.Printf("[postprocessingResult] found result: %s", line)
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(string(raw))
}

// DeleteJob handles DELETE /api/v1/dockingjobs/{name}
func (h *APIHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	jobName := strings.TrimPrefix(r.URL.Path, "/api/v1/dockingjobs/")
	if jobName == "" {
		writeError(w, "job name required", http.StatusBadRequest)
		return
	}

	jobs, err := h.client.BatchV1().Jobs(h.namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("docking.khemia.io/parent-job=%s", jobName),
	})
	if err != nil {
		writeError(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
		return
	}

	for _, job := range jobs.Items {
		if err := h.client.BatchV1().Jobs(h.namespace).Delete(r.Context(), job.Name, metav1.DeleteOptions{}); err != nil {
			log.Printf("Failed to delete job %s: %v", job.Name, err)
		}
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
	WorkflowName        string  `json:"workflow_name"`
	ResultCount         int     `json:"result_count"`
	BestAffinityKcalMol float64 `json:"best_affinity_kcal_mol,omitempty"`
	WorstAffinityKcalMol float64 `json:"worst_affinity_kcal_mol,omitempty"`
	AvgAffinityKcalMol  float64 `json:"avg_affinity_kcal_mol,omitempty"`
}

// mysqlDSN builds a DSN from the standard MYSQL_* env vars injected into the controller pod.
func mysqlDSN() string {
	host := os.Getenv("MYSQL_HOST")
	if host == "" {
		host = "docking-mysql.chem.svc.cluster.local"
	}
	port := os.Getenv("MYSQL_PORT")
	if port == "" {
		port = "3306"
	}
	user := os.Getenv("MYSQL_USER")
	if user == "" {
		user = "root"
	}
	password := os.Getenv("MYSQL_PASSWORD")
	dbName := os.Getenv("MYSQL_DATABASE")
	if dbName == "" {
		dbName = "docking"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, dbName)
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

	db, err := sql.Open("mysql", mysqlDSN())
	if err != nil {
		writeError(w, fmt.Sprintf("db open error: %v", err), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	var count int
	var best, worst, avg sql.NullFloat64
	row := db.QueryRowContext(r.Context(),
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

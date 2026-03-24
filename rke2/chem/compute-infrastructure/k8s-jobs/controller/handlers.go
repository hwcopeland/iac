// Package main provides HTTP handlers for the docking job API
package main

import (
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

	status := "Pending"
	completed := 0
	total := len(jobs.Items)

	for _, job := range jobs.Items {
		if job.Status.Succeeded > 0 {
			completed++
		} else if job.Status.Failed > 0 {
			status = "Failed"
			break
		}
	}

	if total > 0 && completed == total {
		status = "Completed"
	} else if completed > 0 {
		status = "Running"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DockingJobResponse{
		Name:             jobName,
		Status:           status,
		CompletedBatches: completed,
		BatchCount:       total,
	})
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

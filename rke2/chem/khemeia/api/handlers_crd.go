// Package main provides HTTP handlers for CRD job management:
// advance (POST /api/v1/jobs/{kind}/{name}/advance) and
// status  (GET  /api/v1/jobs/{kind}/{name}/status).
//
// These handlers use the dynamic K8s client to interact with Khemeia CRD
// instances and the provenance database for artifact validation and lineage.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// AdvanceRequest is the request body for POST /api/v1/jobs/{kind}/{name}/advance.
type AdvanceRequest struct {
	DownstreamKind      string                 `json:"downstream_kind"`
	SelectedArtifactIDs []string               `json:"selected_artifact_ids"`
	DownstreamParams    map[string]interface{} `json:"downstream_params"`
}

// AdvanceResponse is returned on successful advance.
type AdvanceResponse struct {
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace"`
	ParentJob map[string]string `json:"parentJob"`
}

// JobStatusResponse is returned by GET /api/v1/jobs/{kind}/{name}/status.
type JobStatusResponse struct {
	Phase              string        `json:"phase"`
	StartTime          string        `json:"startTime,omitempty"`
	CompletionTime     string        `json:"completionTime,omitempty"`
	RetryCount         int64         `json:"retryCount"`
	ProvenanceRef      string        `json:"provenanceRef,omitempty"`
	Conditions         []interface{} `json:"conditions,omitempty"`
	Events             []interface{} `json:"events,omitempty"`
	ProducedArtifactIDs []string     `json:"producedArtifactIds,omitempty"`
}

// CRDHandlers holds the dependencies needed by CRD-related HTTP handlers.
type CRDHandlers struct {
	dynamicClient dynamic.Interface
	db            *sql.DB
	namespace     string
}

// NewCRDHandlers creates a handler set for CRD endpoints.
func NewCRDHandlers(dynamicClient dynamic.Interface, db *sql.DB, namespace string) *CRDHandlers {
	return &CRDHandlers{
		dynamicClient: dynamicClient,
		db:            db,
		namespace:     namespace,
	}
}

// HandleAdvance handles POST /api/v1/jobs/{kind}/{name}/advance.
// It validates the source job is Succeeded, validates artifact ownership,
// creates a downstream CRD instance, and records provenance edges.
func (h *CRDHandlers) HandleAdvance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/jobs/{kind}/{name}/advance
	kind, name, err := parseJobAdvancePath(r.URL.Path)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate source CRD exists and is Succeeded.
	gvr := gvrForKind(kind)
	if gvr.Resource == "" {
		writeError(w, fmt.Sprintf("unknown CRD kind: %s", kind), http.StatusBadRequest)
		return
	}

	sourceCRD, err := h.dynamicClient.Resource(gvr).Namespace(h.namespace).Get(
		r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeError(w, fmt.Sprintf("source job not found: %v", err), http.StatusNotFound)
		return
	}

	phase := getPhase(sourceCRD)
	if phase != "Succeeded" {
		writeError(w, fmt.Sprintf("source job must be in Succeeded phase, currently %s", phase),
			http.StatusConflict)
		return
	}

	// Parse request body.
	var req AdvanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.DownstreamKind == "" {
		writeError(w, "downstream_kind is required", http.StatusBadRequest)
		return
	}

	downstreamGVR := gvrForKind(req.DownstreamKind)
	if downstreamGVR.Resource == "" {
		writeError(w, fmt.Sprintf("unknown downstream CRD kind: %s", req.DownstreamKind),
			http.StatusBadRequest)
		return
	}

	// Validate selected artifact IDs belong to source job.
	if len(req.SelectedArtifactIDs) > 0 {
		if err := h.validateArtifactOwnership(name, req.SelectedArtifactIDs); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Generate downstream CRD name.
	downstreamName := fmt.Sprintf("%s-%d",
		strings.ToLower(req.DownstreamKind), time.Now().Unix())

	// Build downstream CRD spec.
	downstreamSpec := make(map[string]interface{})
	for k, v := range req.DownstreamParams {
		downstreamSpec[k] = v
	}
	downstreamSpec["parentJob"] = map[string]interface{}{
		"kind":                kind,
		"name":                name,
		"selectedArtifactIds": toInterfaceSlice(req.SelectedArtifactIDs),
	}

	// Create the downstream CRD instance.
	downstream := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", khemeiaGroup, khemeiaVersion),
			"kind":       req.DownstreamKind,
			"metadata": map[string]interface{}{
				"name":      downstreamName,
				"namespace": h.namespace,
			},
			"spec": downstreamSpec,
		},
	}

	created, err := h.dynamicClient.Resource(downstreamGVR).Namespace(h.namespace).Create(
		r.Context(), downstream, metav1.CreateOptions{})
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create downstream CRD: %v", err),
			http.StatusInternalServerError)
		return
	}

	log.Printf("[advance] Created %s/%s from %s/%s with %d selected artifacts",
		req.DownstreamKind, created.GetName(), kind, name, len(req.SelectedArtifactIDs))

	// Record provenance edges (source artifacts -> downstream job).
	// Per TDD revised approach: edges are created from selected parent artifacts
	// to downstream job outputs when those outputs are produced. The parentJob
	// field on the CRD serves as the advance record in the K8s layer.
	h.recordAdvanceProvenance(name, created.GetName(), req.DownstreamKind, req.SelectedArtifactIDs)

	// Return response.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(AdvanceResponse{
		Name:      created.GetName(),
		Kind:      req.DownstreamKind,
		Namespace: h.namespace,
		ParentJob: map[string]string{"kind": kind, "name": name},
	})
}

// HandleJobStatus handles GET /api/v1/jobs/{kind}/{name}/status.
// Returns the CRD status subresource plus produced artifact IDs from provenance.
func (h *CRDHandlers) HandleJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/jobs/{kind}/{name}/status
	kind, name, err := parseJobStatusPath(r.URL.Path)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	gvr := gvrForKind(kind)
	if gvr.Resource == "" {
		writeError(w, fmt.Sprintf("unknown CRD kind: %s", kind), http.StatusBadRequest)
		return
	}

	crd, err := h.dynamicClient.Resource(gvr).Namespace(h.namespace).Get(
		r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeError(w, fmt.Sprintf("job not found: %v", err), http.StatusNotFound)
		return
	}

	// Extract status fields.
	status := getStatusMap(crd)

	resp := JobStatusResponse{
		Phase: getPhase(crd),
	}

	if st, ok := status["startTime"].(string); ok {
		resp.StartTime = st
	}
	if ct, ok := status["completionTime"].(string); ok {
		resp.CompletionTime = ct
	}
	if rc, ok := status["retryCount"].(int64); ok {
		resp.RetryCount = rc
	} else if rc, ok := status["retryCount"].(float64); ok {
		resp.RetryCount = int64(rc)
	}
	if pr, ok := status["provenanceRef"].(string); ok {
		resp.ProvenanceRef = pr
	}
	if conds, ok := status["conditions"].([]interface{}); ok {
		resp.Conditions = conds
	}
	if evts, ok := status["events"].([]interface{}); ok {
		resp.Events = evts
	}

	// Look up produced artifact IDs from provenance.
	resp.ProducedArtifactIDs = h.getProducedArtifactIDs(name)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// validateArtifactOwnership checks that all provided artifact IDs were produced
// by the given job (created_by_job matches).
func (h *CRDHandlers) validateArtifactOwnership(jobName string, artifactIDs []string) error {
	if h.db == nil || len(artifactIDs) == 0 {
		return nil
	}

	// Build placeholders for IN clause.
	placeholders := make([]string, len(artifactIDs))
	args := make([]interface{}, 0, len(artifactIDs)+1)
	for i, id := range artifactIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, jobName)

	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM provenance WHERE artifact_id IN (%s) AND created_by_job = ?",
		strings.Join(placeholders, ","))

	var count int
	if err := h.db.QueryRow(query, args...).Scan(&count); err != nil {
		return fmt.Errorf("failed to validate artifact ownership: %v", err)
	}

	if count != len(artifactIDs) {
		return fmt.Errorf("some artifact IDs do not belong to job %s: expected %d matches, got %d",
			jobName, len(artifactIDs), count)
	}

	return nil
}

// recordAdvanceProvenance records the advance action in provenance.
// Per TDD: no separate advance_action type. The parentJob on the CRD serves as
// the advance record. We log the advance for auditing but the actual provenance
// edges (source -> output) are created when the downstream job produces outputs.
func (h *CRDHandlers) recordAdvanceProvenance(sourceName, downstreamName, downstreamKind string, artifactIDs []string) {
	if h.db == nil {
		return
	}

	// Record as a log entry for now. The actual provenance edges from source
	// artifacts to downstream outputs will be created by the downstream job's
	// completion handler (or provenance.go RecordProvenance function).
	log.Printf("[provenance] Advance: %s -> %s/%s, artifacts: %v",
		sourceName, downstreamKind, downstreamName, artifactIDs)
}

// getProducedArtifactIDs queries provenance for artifacts produced by a job.
func (h *CRDHandlers) getProducedArtifactIDs(jobName string) []string {
	if h.db == nil {
		return nil
	}

	rows, err := h.db.Query(
		"SELECT artifact_id FROM provenance WHERE created_by_job = ? ORDER BY created_at",
		jobName)
	if err != nil {
		log.Printf("[provenance] Failed to query produced artifacts for %s: %v", jobName, err)
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// --- Path parsing helpers ---

// parseJobAdvancePath extracts kind and name from /api/v1/jobs/{kind}/{name}/advance.
func parseJobAdvancePath(path string) (kind, name string, err error) {
	// Expected: /api/v1/jobs/{kind}/{name}/advance
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// parts: [api, v1, jobs, {kind}, {name}, advance]
	if len(parts) < 6 || parts[len(parts)-1] != "advance" {
		return "", "", fmt.Errorf("invalid advance path: %s", path)
	}
	return parts[len(parts)-3], parts[len(parts)-2], nil
}

// parseJobStatusPath extracts kind and name from /api/v1/jobs/{kind}/{name}/status.
func parseJobStatusPath(path string) (kind, name string, err error) {
	// Expected: /api/v1/jobs/{kind}/{name}/status
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// parts: [api, v1, jobs, {kind}, {name}, status]
	if len(parts) < 6 || parts[len(parts)-1] != "status" {
		return "", "", fmt.Errorf("invalid status path: %s", path)
	}
	return parts[len(parts)-3], parts[len(parts)-2], nil
}

// toInterfaceSlice converts a []string to []interface{} for unstructured objects.
func toInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

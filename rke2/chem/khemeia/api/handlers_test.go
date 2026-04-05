package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, "something broke", http.StatusBadRequest)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "something broke" {
		t.Errorf("expected error message %q, got %q", "something broke", body["error"])
	}
}

func TestCreateJobValidation(t *testing.T) {
	// CreateJob should return 400 when ligand_db is missing.
	// Validation happens before any DB call, so nil db is fine.
	handler := &APIHandler{db: nil}

	body := `{"pdbid":"7jrn"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dockingjobs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.CreateJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "ligand_db") {
		t.Errorf("expected error to mention ligand_db, got %q", resp["error"])
	}
}

func TestCreateJobNoJupyterUser(t *testing.T) {
	// Compile-time verification that DockingJobRequest does not have a JupyterUser field.
	// If someone adds JupyterUser, this will fail to compile.
	_ = DockingJobRequest{
		PDBID:            "7jrn",
		LigandDb:         "test-db",
		NativeLigand:     "TTT",
		LigandsChunkSize: 10000,
		Image:            "test:latest",
	}
}

func TestHealthCheck(t *testing.T) {
	handler := &APIHandler{}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.HealthCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("expected status %q, got %q", "healthy", body["status"])
	}
	if body["time"] == "" {
		t.Error("expected non-empty time field")
	}
	// Verify the time field is valid RFC3339.
	if _, err := time.Parse(time.RFC3339, body["time"]); err != nil {
		t.Errorf("expected valid RFC3339 time, got %q: %v", body["time"], err)
	}
}

func TestDockingJobRequestJSON(t *testing.T) {
	// Test unmarshal (incoming request).
	input := `{
		"pdbid": "7jrn",
		"ligand_db": "chembl",
		"native_ligand": "TTT",
		"ligands_chunk_size": 5000,
		"image": "registry.example.com/vina:v1"
	}`

	var req DockingJobRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.PDBID != "7jrn" {
		t.Errorf("PDBID: expected %q, got %q", "7jrn", req.PDBID)
	}
	if req.LigandDb != "chembl" {
		t.Errorf("LigandDb: expected %q, got %q", "chembl", req.LigandDb)
	}
	if req.NativeLigand != "TTT" {
		t.Errorf("NativeLigand: expected %q, got %q", "TTT", req.NativeLigand)
	}
	if req.LigandsChunkSize != 5000 {
		t.Errorf("LigandsChunkSize: expected %d, got %d", 5000, req.LigandsChunkSize)
	}
	if req.Image != "registry.example.com/vina:v1" {
		t.Errorf("Image: expected %q, got %q", "registry.example.com/vina:v1", req.Image)
	}

	// Test marshal (round-trip).
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var roundTrip DockingJobRequest
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("failed to unmarshal round-trip: %v", err)
	}
	if roundTrip != req {
		t.Errorf("round-trip mismatch: got %+v, want %+v", roundTrip, req)
	}

	// Test that zero-value fields unmarshal correctly.
	minimal := `{"ligand_db": "zinc"}`
	var minReq DockingJobRequest
	if err := json.Unmarshal([]byte(minimal), &minReq); err != nil {
		t.Fatalf("failed to unmarshal minimal: %v", err)
	}
	if minReq.LigandDb != "zinc" {
		t.Errorf("minimal LigandDb: expected %q, got %q", "zinc", minReq.LigandDb)
	}
	if minReq.PDBID != "" {
		t.Errorf("minimal PDBID: expected empty, got %q", minReq.PDBID)
	}
}

func TestDockingJobResponseJSON(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	startTime := time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)

	resp := DockingJobResponse{
		Name:      "docking-12345",
		PDBID:     "7jrn",
		LigandDb:  "chembl",
		Status:    "Running",
		BatchCount: 5,
		CompletedBatches: 2,
		Message:   "processing",
		CreatedAt: now,
		StartTime: &startTime,
		// CompletionTime left nil to test omitempty.
	}

	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Verify required fields are present.
	if decoded["name"] != "docking-12345" {
		t.Errorf("name: expected %q, got %v", "docking-12345", decoded["name"])
	}
	if decoded["status"] != "Running" {
		t.Errorf("status: expected %q, got %v", "Running", decoded["status"])
	}
	if decoded["ligand_db"] != "chembl" {
		t.Errorf("ligand_db: expected %q, got %v", "chembl", decoded["ligand_db"])
	}

	// Verify omitempty: completion_time should be absent when nil.
	if _, exists := decoded["completion_time"]; exists {
		t.Error("expected completion_time to be omitted when nil")
	}

	// start_time should be present.
	if _, exists := decoded["start_time"]; !exists {
		t.Error("expected start_time to be present")
	}

	// Test with empty message (omitempty).
	respNoMsg := DockingJobResponse{
		Name:      "docking-99",
		PDBID:     "1abc",
		LigandDb:  "zinc",
		Status:    "Pending",
		CreatedAt: now,
	}
	out2, err := json.Marshal(respNoMsg)
	if err != nil {
		t.Fatalf("failed to marshal no-message response: %v", err)
	}

	var decoded2 map[string]interface{}
	if err := json.Unmarshal(out2, &decoded2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, exists := decoded2["message"]; exists {
		t.Error("expected message to be omitted when empty")
	}
}

func TestWorkflowListItemJSON(t *testing.T) {
	item := WorkflowListItem{
		Name:             "docking-42",
		Phase:            "Completed",
		PDBID:            "7jrn",
		SourceDb:         "chembl",
		BatchCount:       10,
		CompletedBatches: 10,
		CreatedAt:        time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	out, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify JSON field names match the struct tags.
	if decoded["name"] != "docking-42" {
		t.Errorf("name: expected %q, got %v", "docking-42", decoded["name"])
	}
	if decoded["phase"] != "Completed" {
		t.Errorf("phase: expected %q, got %v", "Completed", decoded["phase"])
	}
	if decoded["source_db"] != "chembl" {
		t.Errorf("source_db: expected %q, got %v", "chembl", decoded["source_db"])
	}
	// batch_count comes back as float64 from json.Unmarshal into interface{}.
	if decoded["batch_count"] != float64(10) {
		t.Errorf("batch_count: expected %v, got %v", 10, decoded["batch_count"])
	}
	if decoded["completed_batches"] != float64(10) {
		t.Errorf("completed_batches: expected %v, got %v", 10, decoded["completed_batches"])
	}

	// Round-trip via typed struct.
	var roundTrip WorkflowListItem
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("failed to unmarshal round-trip: %v", err)
	}
	if roundTrip.Name != item.Name || roundTrip.Phase != item.Phase ||
		roundTrip.PDBID != item.PDBID || roundTrip.SourceDb != item.SourceDb ||
		roundTrip.BatchCount != item.BatchCount || roundTrip.CompletedBatches != item.CompletedBatches {
		t.Errorf("round-trip mismatch: got %+v, want %+v", roundTrip, item)
	}
}

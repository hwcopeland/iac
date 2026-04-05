package main

import (
	"database/sql"
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
	if _, err := time.Parse(time.RFC3339, body["time"]); err != nil {
		t.Errorf("expected valid RFC3339 time, got %q: %v", body["time"], err)
	}
}

func TestPluginSubmitValidation(t *testing.T) {
	// PluginSubmit should return 400 when a required field is missing.
	plugin := Plugin{
		Name:     "test-plugin",
		Slug:     "test",
		Database: "test",
		Input: []PluginInput{
			{Name: "input_file", Type: "text", Required: true},
		},
	}

	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
		pluginDBs: map[string]*sql.DB{},
	}

	// Missing required field.
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.PluginSubmit(plugin)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "input_file") {
		t.Errorf("expected error to mention input_file, got %q", resp["error"])
	}
}

func TestPluginSubmitInvalidJSON(t *testing.T) {
	plugin := Plugin{
		Name:     "test-plugin",
		Slug:     "test",
		Database: "test",
	}

	handler := &APIHandler{}

	body := `not valid json`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.PluginSubmit(plugin)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPluginListNoDB(t *testing.T) {
	plugin := Plugin{
		Name:     "test-plugin",
		Slug:     "test",
		Database: "test",
	}

	handler := &APIHandler{
		pluginDBs: map[string]*sql.DB{}, // no "test" database
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/jobs", nil)
	rr := httptest.NewRecorder()

	handler.PluginList(plugin)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestPluginGetNoDB(t *testing.T) {
	plugin := Plugin{
		Name:     "test-plugin",
		Slug:     "test",
		Database: "test",
	}

	handler := &APIHandler{
		pluginDBs: map[string]*sql.DB{}, // no "test" database
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/jobs/test-123", nil)
	rr := httptest.NewRecorder()

	handler.PluginGet(plugin)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestPluginDeleteNoDB(t *testing.T) {
	plugin := Plugin{
		Name:     "test-plugin",
		Slug:     "test",
		Database: "test",
	}

	handler := &APIHandler{
		pluginDBs: map[string]*sql.DB{}, // no "test" database
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/test/jobs/test-123", nil)
	rr := httptest.NewRecorder()

	handler.PluginDelete(plugin)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestListPlugins(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			plugins: []Plugin{
				{
					Name:    "quantum-espresso",
					Slug:    "qe",
					Version: "1.0",
					Type:    "job",
					Input: []PluginInput{
						{Name: "input_file", Type: "text", Required: true},
					},
					Output: []PluginOutput{
						{Name: "total_energy", Type: "float"},
					},
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	rr := httptest.NewRecorder()

	handler.ListPlugins(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["count"] != float64(1) {
		t.Errorf("expected count 1, got %v", body["count"])
	}

	plugins, ok := body["plugins"].([]interface{})
	if !ok {
		t.Fatal("expected plugins to be an array")
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	p := plugins[0].(map[string]interface{})
	if p["slug"] != "qe" {
		t.Errorf("expected slug %q, got %v", "qe", p["slug"])
	}
	if p["name"] != "quantum-espresso" {
		t.Errorf("expected name %q, got %v", "quantum-espresso", p["name"])
	}
}

func TestListPluginsEmpty(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			plugins: []Plugin{},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	rr := httptest.NewRecorder()

	handler.ListPlugins(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["count"] != float64(0) {
		t.Errorf("expected count 0, got %v", body["count"])
	}
}

func TestPluginJobSummaryJSON(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	startTime := time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)
	user := "testuser"

	summary := PluginJobSummary{
		ID:          1,
		Name:        "qe-12345",
		Status:      "Running",
		SubmittedBy: &user,
		CreatedAt:   now,
		StartedAt:   &startTime,
		// CompletedAt left nil to test omitempty.
	}

	out, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["name"] != "qe-12345" {
		t.Errorf("name: expected %q, got %v", "qe-12345", decoded["name"])
	}
	if decoded["status"] != "Running" {
		t.Errorf("status: expected %q, got %v", "Running", decoded["status"])
	}
	if _, exists := decoded["completed_at"]; exists {
		t.Error("expected completed_at to be omitted when nil")
	}
	if _, exists := decoded["started_at"]; !exists {
		t.Error("expected started_at to be present")
	}
}

func TestPluginJobDetailJSON(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	errMsg := "test error"

	detail := PluginJobDetail{
		ID:     1,
		Name:   "qe-99",
		Status: "Failed",
		InputData: map[string]interface{}{
			"input_file": "test content",
			"num_cpus":   float64(4),
		},
		ErrorOutput: &errMsg,
		CreatedAt:   now,
	}

	out, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["name"] != "qe-99" {
		t.Errorf("name: expected %q, got %v", "qe-99", decoded["name"])
	}
	if decoded["status"] != "Failed" {
		t.Errorf("status: expected %q, got %v", "Failed", decoded["status"])
	}
	if decoded["error_output"] != "test error" {
		t.Errorf("error_output: expected %q, got %v", "test error", decoded["error_output"])
	}

	inputData, ok := decoded["input_data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected input_data to be a map")
	}
	if inputData["input_file"] != "test content" {
		t.Errorf("input_data.input_file: expected %q, got %v", "test content", inputData["input_file"])
	}
}

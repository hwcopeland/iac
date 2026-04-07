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

func TestBasisSetSummaryJSON(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	summary := BasisSetSummary{
		ID:        1,
		Name:      "cc-pVDZ",
		Elements:  "C,N,O,H",
		Format:    "nwchem",
		Source:    "bse",
		CreatedAt: now,
	}

	out, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["name"] != "cc-pVDZ" {
		t.Errorf("name: expected %q, got %v", "cc-pVDZ", decoded["name"])
	}
	if decoded["elements"] != "C,N,O,H" {
		t.Errorf("elements: expected %q, got %v", "C,N,O,H", decoded["elements"])
	}
	if decoded["format"] != "nwchem" {
		t.Errorf("format: expected %q, got %v", "nwchem", decoded["format"])
	}
	if decoded["source"] != "bse" {
		t.Errorf("source: expected %q, got %v", "bse", decoded["source"])
	}
}

func TestBasisSetDetailJSON(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	desc := "Dunning correlation-consistent basis"
	detail := BasisSetDetail{
		ID:          1,
		Name:        "cc-pVDZ",
		Elements:    "C,N,O,H",
		Format:      "nwchem",
		Source:      "bse",
		Description: &desc,
		Content:     "BASIS \"ao basis\" PRINT\n#BASIS SET: ...",
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

	if decoded["name"] != "cc-pVDZ" {
		t.Errorf("name: expected %q, got %v", "cc-pVDZ", decoded["name"])
	}
	if decoded["content"] != detail.Content {
		t.Errorf("content: expected %q, got %v", detail.Content, decoded["content"])
	}
	if decoded["description"] != desc {
		t.Errorf("description: expected %q, got %v", desc, decoded["description"])
	}
}

func TestBasisSetDetailJSON_NilDescription(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	detail := BasisSetDetail{
		ID:        1,
		Name:      "cc-pVDZ",
		Elements:  "C,N,O,H",
		Format:    "nwchem",
		Source:    "bse",
		Content:   "some content",
		CreatedAt: now,
	}

	out, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, exists := decoded["description"]; exists {
		t.Error("expected description to be omitted when nil")
	}
}

func TestListBasisSetsNoDB(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/basis-sets", nil)
	rr := httptest.NewRecorder()

	handler.ListBasisSets(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestGetBasisSetNoDB(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/basis-sets/1", nil)
	rr := httptest.NewRecorder()

	handler.GetBasisSet(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestGetBasisSetEmptyID(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/basis-sets/", nil)
	rr := httptest.NewRecorder()

	handler.GetBasisSet(rr, req)

	// With no DB, returns 500 before reaching ID validation.
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestSearchBasisSetsNoDB(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/basis-sets/search?name=cc-pVDZ", nil)
	rr := httptest.NewRecorder()

	handler.SearchBasisSets(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestUploadBasisSetNoDB(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
	}

	body := `{"name":"test","elements":"C,H","format":"nwchem","content":"data"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/basis-sets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.UploadBasisSet(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestImportBasisSetNoDB(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
	}

	body := `{"name":"cc-pVDZ","elements":["C","H"],"format":"nwchem"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/basis-sets/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ImportBasisSet(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestDeleteBasisSetNoDB(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{
			pluginDBs: map[string]*sql.DB{},
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/basis-sets/1", nil)
	rr := httptest.NewRecorder()

	handler.DeleteBasisSet(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestElementToAtomicNumberCompleteness(t *testing.T) {
	// Verify common elements are present and mapped correctly.
	commonElements := map[string]int{
		"H": 1, "C": 6, "N": 7, "O": 8, "F": 9, "S": 16, "Cl": 17,
		"Fe": 26, "Cu": 29, "Zn": 30, "Br": 35, "I": 53, "Au": 79,
	}
	for sym, expected := range commonElements {
		got, ok := elementToAtomicNumber[sym]
		if !ok {
			t.Errorf("element %s not found in lookup table", sym)
			continue
		}
		if got != expected {
			t.Errorf("element %s: expected Z=%d, got Z=%d", sym, expected, got)
		}
	}

	// Verify table has 118 elements (H through Og).
	if len(elementToAtomicNumber) != 118 {
		t.Errorf("expected 118 elements, got %d", len(elementToAtomicNumber))
	}
}

func TestElementToAtomicNumberNoDuplicates(t *testing.T) {
	// Verify no two symbols map to the same atomic number.
	seen := make(map[int]string)
	for sym, num := range elementToAtomicNumber {
		if prev, exists := seen[num]; exists {
			t.Errorf("duplicate atomic number %d: %q and %q", num, prev, sym)
		}
		seen[num] = sym
	}
}

func TestBasisSetUploadRequestJSON(t *testing.T) {
	body := `{"name":"my-basis","elements":"C,N","format":"gaussian94","description":"test","content":"data"}`
	var req BasisSetUploadRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != "my-basis" {
		t.Errorf("name: expected %q, got %q", "my-basis", req.Name)
	}
	if req.Elements != "C,N" {
		t.Errorf("elements: expected %q, got %q", "C,N", req.Elements)
	}
	if req.Format != "gaussian94" {
		t.Errorf("format: expected %q, got %q", "gaussian94", req.Format)
	}
	if req.Description != "test" {
		t.Errorf("description: expected %q, got %q", "test", req.Description)
	}
	if req.Content != "data" {
		t.Errorf("content: expected %q, got %q", "data", req.Content)
	}
}

func TestBasisSetImportRequestJSON(t *testing.T) {
	body := `{"name":"cc-pVDZ","elements":["C","N","O","H"],"format":"nwchem"}`
	var req BasisSetImportRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != "cc-pVDZ" {
		t.Errorf("name: expected %q, got %q", "cc-pVDZ", req.Name)
	}
	if len(req.Elements) != 4 {
		t.Errorf("elements: expected 4 elements, got %d", len(req.Elements))
	}
	if req.Format != "nwchem" {
		t.Errorf("format: expected %q, got %q", "nwchem", req.Format)
	}

	expected := []string{"C", "N", "O", "H"}
	for i, el := range expected {
		if req.Elements[i] != el {
			t.Errorf("elements[%d]: expected %q, got %q", i, el, req.Elements[i])
		}
	}
}

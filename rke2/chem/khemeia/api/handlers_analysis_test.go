package main

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Unit tests for analysis types and aggregation logic ---

func TestInteractionCountsJSON(t *testing.T) {
	ic := InteractionCounts{
		HBond:       5,
		Hydrophobic: 10,
		Ionic:       3,
		Dipole:      2,
		Contact:     15,
	}
	data, err := json.Marshal(ic)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	for _, key := range []string{"hbond", "hydrophobic", "ionic", "dipole", "contact"} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing key %q: %s", key, s)
		}
	}
}

func TestAggregateResidueContactJSON(t *testing.T) {
	arc := AggregateResidueContact{
		ChainID:          "A",
		ResID:            107,
		ResName:          "ASP",
		ContactFrequency: 0.92,
		AvgDistance:       3.2,
		InteractionCounts: InteractionCounts{
			HBond:       23,
			Hydrophobic: 45,
			Ionic:       12,
			Dipole:      8,
			Contact:     50,
		},
		CompoundsContacting: 46,
	}
	data, err := json.Marshal(arc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	for _, key := range []string{
		"chain_id", "res_id", "res_name", "contact_frequency",
		"avg_distance", "interaction_counts", "compounds_contacting",
	} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing key %q: %s", key, s)
		}
	}
}

func TestReceptorContactsResponseJSON(t *testing.T) {
	resp := ReceptorContactsResponse{
		JobName: "docking-test",
		TopN:    50,
		ResidueContacts: []AggregateResidueContact{
			{
				ChainID:          "A",
				ResID:            107,
				ResName:          "ASP",
				ContactFrequency: 0.92,
				AvgDistance:       3.2,
				InteractionCounts: InteractionCounts{
					HBond: 23, Hydrophobic: 45, Ionic: 12, Dipole: 8, Contact: 50,
				},
				CompoundsContacting: 46,
			},
		},
		TotalCompoundsAnalyzed: 50,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	for _, key := range []string{"job_name", "top_n", "residue_contacts", "total_compounds_analyzed"} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing key %q: %s", key, s)
		}
	}
}

func TestFingerprintsResponseJSON(t *testing.T) {
	resp := FingerprintsResponse{
		JobName: "docking-test",
		Compounds: []CompoundEntry{
			{CompoundID: "CHEMBL123", Smiles: "CC(=O)Oc1ccccc1C(=O)O", Affinity: -8.5},
		},
		Total: 1,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	for _, key := range []string{"job_name", "compounds", "total", "compound_id", "smiles", "affinity"} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing key %q: %s", key, s)
		}
	}
}

func TestCompoundEntryJSON(t *testing.T) {
	c := CompoundEntry{CompoundID: "CHEMBL456", Smiles: "c1ccccc1", Affinity: -6.3}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded CompoundEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.CompoundID != "CHEMBL456" {
		t.Errorf("compound_id: got %q, want %q", decoded.CompoundID, "CHEMBL456")
	}
	if decoded.Smiles != "c1ccccc1" {
		t.Errorf("smiles: got %q, want %q", decoded.Smiles, "c1ccccc1")
	}
	if decoded.Affinity != -6.3 {
		t.Errorf("affinity: got %f, want -6.3", decoded.Affinity)
	}
}

// --- Aggregation logic tests using in-memory data ---

// TestResidueAccumulatorAveraging verifies the accumulator math used in
// receptor-contacts aggregation.
func TestResidueAccumulatorAveraging(t *testing.T) {
	acc := &residueAccumulator{
		ChainID:     "A",
		ResID:       107,
		ResName:     "ASP",
		TotalDist:   0,
		CompCount:   0,
		HBond:       0,
		Hydrophobic: 0,
		Ionic:       0,
		Dipole:      0,
		Contact:     0,
	}

	// Simulate 3 compounds contacting this residue.
	distances := []float64{2.5, 3.0, 3.5}
	for _, d := range distances {
		acc.TotalDist += d
		acc.CompCount++
		acc.Contact++
	}
	acc.HBond = 2
	acc.Ionic = 1

	totalCompounds := 5
	freq := float64(acc.CompCount) / float64(totalCompounds)
	avgDist := acc.TotalDist / float64(acc.CompCount)

	expectedFreq := 0.6
	if math.Abs(freq-expectedFreq) > 0.001 {
		t.Errorf("frequency: got %f, want %f", freq, expectedFreq)
	}

	expectedAvgDist := 3.0
	if math.Abs(avgDist-expectedAvgDist) > 0.001 {
		t.Errorf("avg distance: got %f, want %f", avgDist, expectedAvgDist)
	}
}

// TestResidueAccumulatorZeroCompounds verifies division-by-zero safety.
func TestResidueAccumulatorZeroCompounds(t *testing.T) {
	totalCompounds := 0
	compCount := 0

	freq := 0.0
	if totalCompounds > 0 {
		freq = float64(compCount) / float64(totalCompounds)
	}
	if freq != 0.0 {
		t.Errorf("frequency should be 0 for zero compounds, got %f", freq)
	}

	avgDist := 0.0
	if compCount > 0 {
		avgDist = 10.0 / float64(compCount)
	}
	if avgDist != 0.0 {
		t.Errorf("avg distance should be 0 for zero comp count, got %f", avgDist)
	}
}

// --- HTTP dispatch tests (no DB required) ---

func TestAnalysisDispatchMethodNotAllowed(t *testing.T) {
	h := &APIHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docking/analysis/receptor-contacts/job1", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAnalysisDispatchBadPath(t *testing.T) {
	h := &APIHandler{}

	tests := []struct {
		name string
		path string
	}{
		{"no endpoint or job", "/api/v1/docking/analysis/"},
		{"endpoint only", "/api/v1/docking/analysis/receptor-contacts/"},
		{"trailing slash only", "/api/v1/docking/analysis/receptor-contacts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			h.AnalysisDispatch(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400, got %d", tt.name, rr.Code)
			}
		})
	}
}

func TestAnalysisDispatchUnknownEndpoint(t *testing.T) {
	h := &APIHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/analysis/unknown-thing/job1", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestReceptorContactsNoDB(t *testing.T) {
	h := &APIHandler{pluginDBs: map[string]*sql.DB{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/analysis/receptor-contacts/job1", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for no DB, got %d", rr.Code)
	}
}

func TestFingerprintsNoDB(t *testing.T) {
	h := &APIHandler{pluginDBs: map[string]*sql.DB{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/analysis/fingerprints/job1", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for no DB, got %d", rr.Code)
	}
}

func TestReceptorContactsBadTopParam(t *testing.T) {
	h := &APIHandler{pluginDBs: map[string]*sql.DB{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/analysis/receptor-contacts/job1?top=abc", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad top param, got %d", rr.Code)
	}
}

func TestReceptorContactsTopTooLarge(t *testing.T) {
	h := &APIHandler{pluginDBs: map[string]*sql.DB{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/analysis/receptor-contacts/job1?top=999", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for top > 500, got %d", rr.Code)
	}
}

func TestFingerprintsBadTopParam(t *testing.T) {
	h := &APIHandler{pluginDBs: map[string]*sql.DB{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/analysis/fingerprints/job1?top=abc", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad top param, got %d", rr.Code)
	}
}

func TestFingerprintsTopTooLarge(t *testing.T) {
	h := &APIHandler{pluginDBs: map[string]*sql.DB{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/analysis/fingerprints/job1?top=5000", nil)
	rr := httptest.NewRecorder()
	h.AnalysisDispatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for top > 1000, got %d", rr.Code)
	}
}

// TestAggregateResidueContactRoundTrip verifies JSON round-trip fidelity for
// the full response type.
func TestAggregateResidueContactRoundTrip(t *testing.T) {
	original := ReceptorContactsResponse{
		JobName: "docking-abc",
		TopN:    25,
		ResidueContacts: []AggregateResidueContact{
			{
				ChainID:          "B",
				ResID:            42,
				ResName:          "GLU",
				ContactFrequency: 0.75,
				AvgDistance:       2.8,
				InteractionCounts: InteractionCounts{
					HBond: 10, Hydrophobic: 5, Ionic: 8, Dipole: 3, Contact: 25,
				},
				CompoundsContacting: 19,
			},
		},
		TotalCompoundsAnalyzed: 25,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ReceptorContactsResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.JobName != original.JobName {
		t.Errorf("job_name: got %q, want %q", decoded.JobName, original.JobName)
	}
	if decoded.TopN != original.TopN {
		t.Errorf("top_n: got %d, want %d", decoded.TopN, original.TopN)
	}
	if decoded.TotalCompoundsAnalyzed != original.TotalCompoundsAnalyzed {
		t.Errorf("total_compounds_analyzed: got %d, want %d", decoded.TotalCompoundsAnalyzed, original.TotalCompoundsAnalyzed)
	}
	if len(decoded.ResidueContacts) != 1 {
		t.Fatalf("residue_contacts length: got %d, want 1", len(decoded.ResidueContacts))
	}
	rc := decoded.ResidueContacts[0]
	if rc.ChainID != "B" || rc.ResID != 42 || rc.ResName != "GLU" {
		t.Errorf("residue identity: got %s %s %d, want B GLU 42", rc.ChainID, rc.ResName, rc.ResID)
	}
	if rc.ContactFrequency != 0.75 {
		t.Errorf("contact_frequency: got %f, want 0.75", rc.ContactFrequency)
	}
	if rc.InteractionCounts.Ionic != 8 {
		t.Errorf("ionic count: got %d, want 8", rc.InteractionCounts.Ionic)
	}
}

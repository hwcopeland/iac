package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Health handler tests
// ---------------------------------------------------------------------------

func TestHealthHandler_ReturnsOK(t *testing.T) {
	// The /health handler is defined inline in startHealthServer, so we
	// reproduce it here. If the handler is ever extracted, this test should
	// call that function directly.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %q", body["status"])
	}
}

// ---------------------------------------------------------------------------
// prepPayload JSON tests
// ---------------------------------------------------------------------------

func TestPrepPayload_UnmarshalValid(t *testing.T) {
	raw := `{"ligand_id": 42, "pdbqt_b64": "aGVsbG8="}`
	var p prepPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if p.LigandID != 42 {
		t.Errorf("LigandID: expected 42, got %d", p.LigandID)
	}
	if p.PDBQTB64 != "aGVsbG8=" {
		t.Errorf("PDBQTB64: expected 'aGVsbG8=', got %q", p.PDBQTB64)
	}
}

func TestPrepPayload_Base64Decode(t *testing.T) {
	content := "ATOM      1  C1  LIG     1       0.000   0.000   0.000"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	raw, _ := json.Marshal(prepPayload{LigandID: 1, PDBQTB64: encoded})
	var p prepPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(p.PDBQTB64)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if string(decoded) != content {
		t.Errorf("decoded content mismatch:\n  got:  %q\n  want: %q", string(decoded), content)
	}
}

func TestPrepPayload_InvalidBase64(t *testing.T) {
	p := prepPayload{LigandID: 1, PDBQTB64: "not-valid-base64!!!"}
	_, err := base64.StdEncoding.DecodeString(p.PDBQTB64)
	if err == nil {
		t.Error("expected base64 decode error for invalid input, got nil")
	}
}

func TestPrepPayload_MarshalRoundTrip(t *testing.T) {
	original := prepPayload{LigandID: 99, PDBQTB64: "dGVzdA=="}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded prepPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestPrepPayload_UnmarshalMissingFields(t *testing.T) {
	// JSON with no fields should zero-value everything without error.
	raw := `{}`
	var p prepPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal of empty object failed: %v", err)
	}
	if p.LigandID != 0 {
		t.Errorf("expected zero LigandID, got %d", p.LigandID)
	}
	if p.PDBQTB64 != "" {
		t.Errorf("expected empty PDBQTB64, got %q", p.PDBQTB64)
	}
}

// ---------------------------------------------------------------------------
// dockPayload JSON tests
// ---------------------------------------------------------------------------

func TestDockPayload_UnmarshalValid(t *testing.T) {
	raw := `{
		"workflow_name": "vina-standard",
		"pdb_id": "1ABC",
		"ligand_id": 7,
		"compound_id": "ZINC000000000042",
		"affinity_kcal_mol": -8.3
	}`
	var d dockPayload
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if d.WorkflowName != "vina-standard" {
		t.Errorf("WorkflowName: expected 'vina-standard', got %q", d.WorkflowName)
	}
	if d.PDBID != "1ABC" {
		t.Errorf("PDBID: expected '1ABC', got %q", d.PDBID)
	}
	if d.LigandID != 7 {
		t.Errorf("LigandID: expected 7, got %d", d.LigandID)
	}
	if d.CompoundID != "ZINC000000000042" {
		t.Errorf("CompoundID: expected 'ZINC000000000042', got %q", d.CompoundID)
	}
	if d.AffinityKcalMol != -8.3 {
		t.Errorf("AffinityKcalMol: expected -8.3, got %f", d.AffinityKcalMol)
	}
}

func TestDockPayload_MarshalRoundTrip(t *testing.T) {
	original := dockPayload{
		WorkflowName:   "vina-standard",
		PDBID:          "2XYZ",
		LigandID:       100,
		CompoundID:     "CID12345",
		AffinityKcalMol: -12.5,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded dockPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestDockPayload_UnmarshalMissingFields(t *testing.T) {
	raw := `{}`
	var d dockPayload
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal of empty object failed: %v", err)
	}
	if d.WorkflowName != "" {
		t.Errorf("expected empty WorkflowName, got %q", d.WorkflowName)
	}
	if d.AffinityKcalMol != 0 {
		t.Errorf("expected zero AffinityKcalMol, got %f", d.AffinityKcalMol)
	}
}

func TestDockPayload_NegativeAffinity(t *testing.T) {
	// Docking affinities are typically negative; ensure sign is preserved.
	raw := `{"affinity_kcal_mol": -0.001}`
	var d dockPayload
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if d.AffinityKcalMol >= 0 {
		t.Errorf("expected negative affinity, got %f", d.AffinityKcalMol)
	}
}

// ---------------------------------------------------------------------------
// JSON tag verification
// ---------------------------------------------------------------------------

func TestPrepPayload_JSONTags(t *testing.T) {
	p := prepPayload{LigandID: 5, PDBQTB64: "abc="}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify the serialized keys match the expected JSON tags.
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	for _, key := range []string{"ligand_id", "pdbqt_b64"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected JSON key %q not found in marshaled output", key)
		}
	}
}

func TestDockPayload_JSONTags(t *testing.T) {
	d := dockPayload{
		WorkflowName:   "w",
		PDBID:          "p",
		LigandID:       1,
		CompoundID:     "c",
		AffinityKcalMol: -1.0,
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	expected := []string{"workflow_name", "pdb_id", "ligand_id", "compound_id", "affinity_kcal_mol"}
	for _, key := range expected {
		if _, ok := m[key]; !ok {
			t.Errorf("expected JSON key %q not found in marshaled output", key)
		}
	}
}

// ---------------------------------------------------------------------------
// stagingRow payload dispatch tests
// ---------------------------------------------------------------------------

func TestStagingRow_PayloadPreservesRawJSON(t *testing.T) {
	// Verify json.RawMessage in stagingRow preserves the raw bytes without
	// early parsing, which is important for the two-phase unmarshal pattern
	// used in processPrep/processDock.
	payload := json.RawMessage(`{"ligand_id":1,"pdbqt_b64":"dGVzdA=="}`)
	row := stagingRow{ID: 1, JobType: "prep", Payload: payload}

	// Re-marshal and check raw bytes are intact.
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal stagingRow failed: %v", err)
	}

	var decoded stagingRow
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal stagingRow failed: %v", err)
	}

	// The payload should still parse as a valid prepPayload.
	var p prepPayload
	if err := json.Unmarshal(decoded.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload from stagingRow failed: %v", err)
	}
	if p.LigandID != 1 {
		t.Errorf("LigandID: expected 1, got %d", p.LigandID)
	}
}

func TestStagingRow_InvalidPayloadDetected(t *testing.T) {
	// If the payload is not valid JSON for the expected type, unmarshal
	// into the specific payload struct should fail.
	row := stagingRow{
		ID:      1,
		JobType: "prep",
		Payload: json.RawMessage(`{"ligand_id": "not_a_number"}`),
	}

	var p prepPayload
	err := json.Unmarshal(row.Payload, &p)
	if err == nil {
		t.Error("expected unmarshal error for string in int field, got nil")
	}
}

// ---------------------------------------------------------------------------
// Constants sanity checks
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	if batchSize <= 0 {
		t.Errorf("batchSize should be positive, got %d", batchSize)
	}
	if pollInterval <= 0 {
		t.Errorf("pollInterval should be positive, got %v", pollInterval)
	}
	if statsInterval <= 0 {
		t.Errorf("statsInterval should be positive, got %v", statsInterval)
	}
	if httpPort <= 0 || httpPort > 65535 {
		t.Errorf("httpPort should be a valid port number, got %d", httpPort)
	}
}

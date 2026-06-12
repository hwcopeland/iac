package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newGenomeHandler builds an APIHandler with a nil DB. These tests exercise the
// dispatch + offline validation paths that return before any DB access, mirroring
// the DB-free assertions in handlers_test.go (TestPluginSubmitValidation et al.).
func newGenomeHandler() *APIHandler {
	return &APIHandler{controller: &Controller{}}
}

func TestGenomeDispatchMethodNotAllowed(t *testing.T) {
	h := newGenomeHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/genome/jobs/genome-1", nil)
	rr := httptest.NewRecorder()
	h.GenomeDispatch(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestGenomeDispatchNotFound(t *testing.T) {
	h := newGenomeHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/genome/bogus", nil)
	rr := httptest.NewRecorder()
	h.GenomeDispatch(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestGenomeSubmitInvalidJSON(t *testing.T) {
	h := newGenomeHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genome/variant/submit", strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	h.GenomeSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGenomeSubmitEmptyVariants(t *testing.T) {
	h := newGenomeHandler()
	body := `{"variants":[],"calculations":["ddg_stability"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genome/variant/submit", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.GenomeSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty variants, got %d", rr.Code)
	}
}

func TestGenomeSubmitEmptyCalculations(t *testing.T) {
	h := newGenomeHandler()
	body := `{"variants":[{"id":"v1","mode":"rsid","rsid":"rs123"}],"calculations":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genome/variant/submit", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.GenomeSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty calculations, got %d", rr.Code)
	}
}

func TestGenomeSubmitUnknownCalculation(t *testing.T) {
	h := newGenomeHandler()
	body := `{"variants":[{"id":"v1","mode":"rsid","rsid":"rs123"}],"calculations":["resolve"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genome/variant/submit", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.GenomeSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-submittable calculation 'resolve', got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "resolve") {
		t.Errorf("expected error to mention the rejected calculation, got %q", resp["error"])
	}
}

func TestGenomeSubmitBadParamsShape(t *testing.T) {
	h := newGenomeHandler()
	// params.ddg_stability is an array, not an object → 400.
	body := `{"variants":[{"id":"v1","mode":"rsid","rsid":"rs123"}],
	          "calculations":["ddg_stability"],
	          "params":{"ddg_stability":["foldx"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genome/variant/submit", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.GenomeSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed params, got %d", rr.Code)
	}
}

func TestGenomeSubmitParamsForUnrequestedCalc(t *testing.T) {
	h := newGenomeHandler()
	body := `{"variants":[{"id":"v1","mode":"rsid","rsid":"rs123"}],
	          "calculations":["ddg_stability"],
	          "params":{"pgx_docking":{"drugs":["x"]}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genome/variant/submit", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.GenomeSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for params referencing a non-requested calc, got %d", rr.Code)
	}
}

// TestValidateVariantInput covers the offline per-variant validation partitioning.
func TestValidateVariantInput(t *testing.T) {
	cases := []struct {
		name    string
		in      VariantInput
		wantOK  bool
		wantErr string
	}{
		{"protein_change ok", VariantInput{Mode: "protein_change", Gene: "TPMT", ProteinChange: "p.R117H"}, true, ""},
		{"protein_change via uniprot", VariantInput{Mode: "protein_change", UniProtAcc: "P51580", ProteinChange: "p.R117H"}, true, ""},
		{"protein_change missing gene", VariantInput{Mode: "protein_change", ProteinChange: "p.R117H"}, false, ECodeParamsInvalid},
		{"protein_change missing change", VariantInput{Mode: "protein_change", Gene: "TPMT"}, false, ECodeParamsInvalid},
		{"hgvs ok", VariantInput{Mode: "hgvs", HGVS: "NP_000358.1:p.Arg117His"}, true, ""},
		{"hgvs missing", VariantInput{Mode: "hgvs"}, false, ECodeParamsInvalid},
		{"rsid ok", VariantInput{Mode: "rsid", RSID: "rs1142345"}, true, ""},
		{"rsid missing", VariantInput{Mode: "rsid"}, false, ECodeParamsInvalid},
		{"missing mode", VariantInput{}, false, ECodeParamsInvalid},
		{"unknown mode", VariantInput{Mode: "genomic"}, false, ECodeParamsInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, ok := validateVariantInput(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok && code != tc.wantErr {
				t.Errorf("code = %q, want %q", code, tc.wantErr)
			}
		})
	}
}

// TestVariantKey verifies the stable per-variant key derivation.
func TestVariantKey(t *testing.T) {
	cases := []struct {
		name string
		in   VariantInput
		idx  int
		want string
	}{
		{"prefers id", VariantInput{ID: "u4u-var-001", Mode: "rsid", RSID: "rs1"}, 0, "u4u-var-001"},
		{"protein_change canonical", VariantInput{Mode: "protein_change", Gene: "TPMT", ProteinChange: "p.R117H"}, 0, "TPMT:p.R117H"},
		{"rsid canonical", VariantInput{Mode: "rsid", RSID: "rs1142345"}, 0, "rs1142345"},
		{"hgvs canonical", VariantInput{Mode: "hgvs", HGVS: "x:p.R1H"}, 0, "x:p.R1H"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := variantKey(tc.in, tc.idx); got != tc.want {
				t.Errorf("variantKey = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenomeStructureBadResolutionID(t *testing.T) {
	h := newGenomeHandler()
	// Empty resolution id segment.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/genome/variant//structure", nil)
	rr := httptest.NewRecorder()
	h.GenomeDispatch(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty resolution_id, got %d", rr.Code)
	}
}

func TestPaginationParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?page=3&per_page=50", nil)
	page, perPage := paginationParams(req, 100, 200)
	if page != 3 || perPage != 50 {
		t.Errorf("got page=%d per_page=%d, want 3/50", page, perPage)
	}
	// per_page over cap falls back to default.
	req2 := httptest.NewRequest(http.MethodGet, "/x?per_page=9999", nil)
	_, perPage2 := paginationParams(req2, 100, 200)
	if perPage2 != 100 {
		t.Errorf("got per_page=%d, want default 100 when over cap", perPage2)
	}
}

// --- B1/VULN-3 dispatcher + guard unit tests (DB-free, pure functions + the
// pre-DB cap path in GenomeSubmit). ---

// TestGenomeCalcJobCRNameDeterministic verifies the dispatcher's CR name is a
// stable, idempotent function of (group, variant_key, calculation) so a retried
// submit converges on the same CR (AlreadyExists-tolerant minting).
func TestGenomeCalcJobCRNameDeterministic(t *testing.T) {
	a := genomeCalcJobCRName("genome-1", "BRAF:p.V600E", "ddg_stability")
	b := genomeCalcJobCRName("genome-1", "BRAF:p.V600E", "ddg_stability")
	if a != b {
		t.Fatalf("CR name not deterministic: %q != %q", a, b)
	}
	if a == genomeCalcJobCRName("genome-1", "BRAF:p.V600E", "pgx_docking") {
		t.Errorf("different calculation must yield a different CR name")
	}
	if a == genomeCalcJobCRName("genome-2", "BRAF:p.V600E", "ddg_stability") {
		t.Errorf("different group must yield a different CR name")
	}
	if !strings.HasPrefix(a, "gj-") {
		t.Errorf("expected gj- prefix (DNS-1123 safe, distinct from esmfold children), got %q", a)
	}
	// "gj-" + 16 hex chars.
	if len(a) != 3+16 {
		t.Errorf("expected stable length 19, got %d (%q)", len(a), a)
	}
}

// TestGenomeVariantSpecBlock verifies the inverse of variantInputFromCR: camelCase
// keys, empty fields omitted, mode always present.
func TestGenomeVariantSpecBlock(t *testing.T) {
	block := genomeVariantSpecBlock(VariantInput{
		Mode:          VariantModeProteinChange,
		Gene:          "BRAF",
		ProteinChange: "p.V600E",
		UniProtAcc:    "P15056",
	})
	if block["mode"] != "protein_change" {
		t.Errorf("mode missing/wrong: %v", block["mode"])
	}
	if block["gene"] != "BRAF" || block["proteinChange"] != "p.V600E" || block["uniprotAcc"] != "P15056" {
		t.Errorf("camelCase fields not mapped: %#v", block)
	}
	if _, ok := block["hgvs"]; ok {
		t.Errorf("empty fields must be omitted, got hgvs in %#v", block)
	}
}

// TestGenomeSubmitBatchCap verifies VULN-3: an over-cap variants×calculations
// product is rejected with 413 before any DB write (the handler has a nil DB).
func TestGenomeSubmitBatchCap(t *testing.T) {
	t.Setenv("GENOME_MAX_BATCH", "4")
	h := newGenomeHandler()
	// 3 variants × 2 calcs = 6 units > cap 4.
	body := `{"variants":[
	            {"id":"v1","mode":"rsid","rsid":"rs1"},
	            {"id":"v2","mode":"rsid","rsid":"rs2"},
	            {"id":"v3","mode":"rsid","rsid":"rs3"}],
	          "calculations":["ddg_stability","pocket_proximity"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genome/variant/submit", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.GenomeSubmit(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 over-cap, got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "batch too large") {
		t.Errorf("expected a clear cap message, got %q", resp["error"])
	}
}

// TestGenomeMaxBatchDefault verifies a bad/absent GENOME_MAX_BATCH falls back to
// the default rather than uncapping.
func TestGenomeMaxBatchDefault(t *testing.T) {
	t.Setenv("GENOME_MAX_BATCH", "")
	if got := genomeMaxBatch(); got != genomeDefaultMaxBatch {
		t.Errorf("empty env: expected default %d, got %d", genomeDefaultMaxBatch, got)
	}
	t.Setenv("GENOME_MAX_BATCH", "-7")
	if got := genomeMaxBatch(); got != genomeDefaultMaxBatch {
		t.Errorf("negative env must not uncap: expected %d, got %d", genomeDefaultMaxBatch, got)
	}
	t.Setenv("GENOME_MAX_BATCH", "garbage")
	if got := genomeMaxBatch(); got != genomeDefaultMaxBatch {
		t.Errorf("non-numeric env must not uncap: expected %d, got %d", genomeDefaultMaxBatch, got)
	}
	t.Setenv("GENOME_MAX_BATCH", "1000")
	if got := genomeMaxBatch(); got != 1000 {
		t.Errorf("valid override: expected 1000, got %d", got)
	}
}

// TestClampExhaustiveness verifies VULN-3 second clause: an over-cap docking
// exhaustiveness is clamped in place; in-range and non-numeric values are left.
func TestClampExhaustiveness(t *testing.T) {
	// Over-cap -> clamped and rewritten.
	calcs := []string{"pgx_docking"}
	params := map[string]json.RawMessage{
		"pgx_docking": json.RawMessage(`{"drugs":["x"],"exhaustiveness":1000000}`),
	}
	if err := validateGenomeParams(calcs, params); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(params["pgx_docking"], &got); err != nil {
		t.Fatalf("clamped params not valid JSON: %v", err)
	}
	var ex int
	json.Unmarshal(got["exhaustiveness"], &ex)
	if ex != genomeMaxExhaustiveness {
		t.Errorf("expected exhaustiveness clamped to %d, got %d", genomeMaxExhaustiveness, ex)
	}

	// In-range -> untouched.
	params2 := map[string]json.RawMessage{
		"pgx_docking": json.RawMessage(`{"exhaustiveness":16}`),
	}
	if err := validateGenomeParams(calcs, params2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got2 map[string]json.RawMessage
	json.Unmarshal(params2["pgx_docking"], &got2)
	var ex2 int
	json.Unmarshal(got2["exhaustiveness"], &ex2)
	if ex2 != 16 {
		t.Errorf("in-range value must be preserved, got %d", ex2)
	}
}

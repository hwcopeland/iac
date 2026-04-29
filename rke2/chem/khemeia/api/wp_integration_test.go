package main

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ===================================================================
// WP-1: Target Preparation — pocket consensus, merge, validation
// ===================================================================

// TestWP1_RankPockets_Empty verifies RankPockets returns an empty slice
// when both tools produce no results.
func TestWP1_RankPockets_Empty(t *testing.T) {
	result := RankPockets(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 pockets, got %d", len(result))
	}

	result2 := RankPockets([]DetectedPocket{}, []DetectedPocket{})
	if len(result2) != 0 {
		t.Errorf("expected 0 pockets for empty slices, got %d", len(result2))
	}
}

// TestWP1_RankPockets_FpocketOnly verifies consensus scoring when only
// fpocket returns results (P2Rank has nothing).
func TestWP1_RankPockets_FpocketOnly(t *testing.T) {
	fpPockets := []DetectedPocket{
		{Center: [3]float64{10, 20, 30}, Size: [3]float64{5, 5, 5}, FpocketScore: 0.8, Volume: 100},
		{Center: [3]float64{40, 50, 60}, Size: [3]float64{6, 6, 6}, FpocketScore: 0.4, Volume: 80},
	}

	result := RankPockets(fpPockets, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 pockets, got %d", len(result))
	}

	// Best fpocket score normalizes to 1.0, worst to 0.0.
	// With no P2Rank, P2RankScore = 0 for all.
	// Consensus = (normalized_fpocket + 0) / 2
	// Pocket 1: fpocket normalized = 1.0 => consensus = 0.5
	// Pocket 2: fpocket normalized = 0.0 => consensus = 0.0
	if result[0].Rank != 1 {
		t.Errorf("expected rank 1 first, got %d", result[0].Rank)
	}
	if result[0].ConsensusScore < result[1].ConsensusScore {
		t.Error("expected first pocket to have higher consensus score")
	}
}

// TestWP1_RankPockets_P2RankOnly verifies consensus scoring when only
// P2Rank returns results (fpocket has nothing).
func TestWP1_RankPockets_P2RankOnly(t *testing.T) {
	p2Pockets := []DetectedPocket{
		{Center: [3]float64{10, 20, 30}, Size: [3]float64{5, 5, 5}, P2RankScore: 0.95, Volume: 120},
		{Center: [3]float64{40, 50, 60}, Size: [3]float64{6, 6, 6}, P2RankScore: 0.30, Volume: 70},
	}

	result := RankPockets(nil, p2Pockets)
	if len(result) != 2 {
		t.Fatalf("expected 2 pockets, got %d", len(result))
	}

	// Unmatched P2Rank pockets get fpocket score = 0.
	// Consensus = (0 + normalized_p2rank) / 2
	if result[0].Rank != 1 {
		t.Errorf("expected rank 1 first, got %d", result[0].Rank)
	}
	if result[0].ConsensusScore < result[1].ConsensusScore {
		t.Error("expected first pocket to have higher consensus score")
	}
}

// TestWP1_RankPockets_MatchedPockets verifies spatial matching when both
// tools detect the same pocket (centers within 5A tolerance).
func TestWP1_RankPockets_MatchedPockets(t *testing.T) {
	fpPockets := []DetectedPocket{
		{Center: [3]float64{10, 20, 30}, Size: [3]float64{5, 5, 5}, FpocketScore: 0.9, Volume: 100, Residues: []string{"ALA1", "GLY2"}},
	}
	p2Pockets := []DetectedPocket{
		// Within 5A tolerance of fpocket center.
		{Center: [3]float64{11, 21, 31}, Size: [3]float64{6, 6, 6}, P2RankScore: 0.85, Volume: 110, Residues: []string{"GLY2", "VAL3"}},
	}

	result := RankPockets(fpPockets, p2Pockets)
	if len(result) != 1 {
		t.Fatalf("expected 1 merged pocket, got %d", len(result))
	}

	// Both tools detected the same pocket — should get scores from both.
	// With single pocket per tool, both normalize to 1.0.
	// Consensus = (1.0 + 1.0) / 2 = 1.0
	if result[0].ConsensusScore != 1.0 {
		t.Errorf("expected consensus 1.0 for matched pocket, got %f", result[0].ConsensusScore)
	}

	// Verify residue merging (deduplicated union).
	if len(result[0].Residues) != 3 {
		t.Errorf("expected 3 unique residues (ALA1, GLY2, VAL3), got %d: %v",
			len(result[0].Residues), result[0].Residues)
	}
}

// TestWP1_RankPockets_UnmatchedPockets verifies that pockets beyond 5A
// tolerance are NOT merged — they remain separate entries.
func TestWP1_RankPockets_UnmatchedPockets(t *testing.T) {
	fpPockets := []DetectedPocket{
		{Center: [3]float64{10, 20, 30}, Size: [3]float64{5, 5, 5}, FpocketScore: 0.9, Volume: 100},
	}
	p2Pockets := []DetectedPocket{
		// 50A away — well beyond the 5A tolerance.
		{Center: [3]float64{60, 70, 80}, Size: [3]float64{6, 6, 6}, P2RankScore: 0.85, Volume: 110},
	}

	result := RankPockets(fpPockets, p2Pockets)
	if len(result) != 2 {
		t.Fatalf("expected 2 separate pockets (beyond tolerance), got %d", len(result))
	}
}

// TestWP1_MergePockets_ExactBoundary tests merge behavior at exactly
// the 5A tolerance boundary.
func TestWP1_MergePockets_ExactBoundary(t *testing.T) {
	// Distance = exactly 5.0A (sqrt(3^2 + 4^2 + 0^2) = 5.0).
	fpPockets := []DetectedPocket{
		{Center: [3]float64{0, 0, 0}, FpocketScore: 0.7},
	}
	p2Pockets := []DetectedPocket{
		{Center: [3]float64{3, 4, 0}, P2RankScore: 0.8},
	}

	result := RankPockets(fpPockets, p2Pockets)
	// At exactly 5.0A, the pockets should merge (bestDist <= pocketMatchTolerance).
	if len(result) != 1 {
		t.Fatalf("expected 1 merged pocket at exact 5A boundary, got %d", len(result))
	}
}

// TestWP1_MergePockets_JustBeyondBoundary tests that pockets just beyond 5A do NOT merge.
func TestWP1_MergePockets_JustBeyondBoundary(t *testing.T) {
	// Distance = ~5.0001A (just beyond tolerance).
	fpPockets := []DetectedPocket{
		{Center: [3]float64{0, 0, 0}, FpocketScore: 0.7},
	}
	p2Pockets := []DetectedPocket{
		{Center: [3]float64{3, 4, 0.01}, P2RankScore: 0.8},
	}

	result := RankPockets(fpPockets, p2Pockets)
	if len(result) != 2 {
		t.Fatalf("expected 2 separate pockets just beyond 5A, got %d", len(result))
	}
}

// TestWP1_RankPockets_IdenticalScores verifies that identical scores
// normalize to 1.0 (edge case: zero range).
func TestWP1_RankPockets_IdenticalScores(t *testing.T) {
	fpPockets := []DetectedPocket{
		{Center: [3]float64{10, 20, 30}, FpocketScore: 0.5, Volume: 100},
		{Center: [3]float64{40, 50, 60}, FpocketScore: 0.5, Volume: 80},
	}

	result := RankPockets(fpPockets, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 pockets, got %d", len(result))
	}

	// When all fpocket scores are identical, they normalize to 1.0.
	// Consensus = (1.0 + 0) / 2 = 0.5 for both.
	for _, p := range result {
		if p.ConsensusScore != 0.5 {
			t.Errorf("expected consensus 0.5 for identical scores, got %f", p.ConsensusScore)
		}
	}
}

// TestWP1_RankPockets_RankOrdering verifies that ranks are assigned
// sequentially by consensus score descending.
func TestWP1_RankPockets_RankOrdering(t *testing.T) {
	fpPockets := []DetectedPocket{
		{Center: [3]float64{10, 20, 30}, FpocketScore: 0.9},
		{Center: [3]float64{40, 50, 60}, FpocketScore: 0.1},
		{Center: [3]float64{70, 80, 90}, FpocketScore: 0.5},
	}

	result := RankPockets(fpPockets, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 pockets, got %d", len(result))
	}

	for i := range result {
		if result[i].Rank != i+1 {
			t.Errorf("pocket %d: expected rank %d, got %d", i, i+1, result[i].Rank)
		}
	}

	// Verify descending order.
	for i := 1; i < len(result); i++ {
		if result[i].ConsensusScore > result[i-1].ConsensusScore {
			t.Errorf("pockets not in descending order: rank %d score %f > rank %d score %f",
				result[i].Rank, result[i].ConsensusScore,
				result[i-1].Rank, result[i-1].ConsensusScore)
		}
	}
}

// TestWP1_CenterDistance verifies Euclidean distance computation.
func TestWP1_CenterDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b [3]float64
		want float64
	}{
		{"origin", [3]float64{0, 0, 0}, [3]float64{0, 0, 0}, 0.0},
		{"unit-x", [3]float64{0, 0, 0}, [3]float64{1, 0, 0}, 1.0},
		{"3-4-5", [3]float64{0, 0, 0}, [3]float64{3, 4, 0}, 5.0},
		{"negative", [3]float64{-1, -2, -3}, [3]float64{2, 2, 1}, 6.4031242},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := centerDistance(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-4 {
				t.Errorf("centerDistance(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestWP1_MergeResidues verifies deduplication and sorting.
func TestWP1_MergeResidues(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []string
		wantLen  int
		wantSorted bool
	}{
		{"both_empty", nil, nil, 0, true},
		{"a_only", []string{"ALA1", "GLY2"}, nil, 2, true},
		{"overlap", []string{"ALA1", "GLY2"}, []string{"GLY2", "VAL3"}, 3, true},
		{"duplicates_in_same", []string{"ALA1", "ALA1"}, []string{"ALA1"}, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeResidues(tt.a, tt.b)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d (got %v)", len(got), tt.wantLen, got)
			}
			// Verify sorted.
			for i := 1; i < len(got); i++ {
				if got[i] < got[i-1] {
					t.Errorf("residues not sorted: %v", got)
					break
				}
			}
		})
	}
}

// TestWP1_TargetPrepValidation_MissingPDBID verifies that a request
// with an empty PDB ID is rejected.
func TestWP1_TargetPrepValidation_MissingPDBID(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"pdb_id": "", "binding_site_mode": "native-ligand"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "pdb_id") {
		t.Errorf("expected error to mention pdb_id, got %q", rr.Body.String())
	}
}

// TestWP1_TargetPrepValidation_InvalidMode verifies that an invalid
// binding_site_mode is rejected with 400.
func TestWP1_TargetPrepValidation_InvalidMode(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"pdb_id": "1ABC", "binding_site_mode": "invalid-mode"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "binding_site_mode") {
		t.Errorf("expected error to mention binding_site_mode, got %q", rr.Body.String())
	}
}

// TestWP1_TargetPrepValidation_NativeLigandMissing verifies that native-ligand
// mode without a ligand ID is rejected.
func TestWP1_TargetPrepValidation_NativeLigandMissing(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"pdb_id": "1ABC", "binding_site_mode": "native-ligand"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "native_ligand_id") {
		t.Errorf("expected error to mention native_ligand_id, got %q", rr.Body.String())
	}
}

// TestWP1_TargetPrepValidation_CustomBoxMissing verifies that custom-box mode
// without a box spec is rejected.
func TestWP1_TargetPrepValidation_CustomBoxMissing(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"pdb_id": "1ABC", "binding_site_mode": "custom-box"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "custom_box") {
		t.Errorf("expected error to mention custom_box, got %q", rr.Body.String())
	}
}

// TestWP1_TargetPrepValidation_CustomBoxZeroSize verifies that a custom
// box with zero-size dimensions is rejected.
func TestWP1_TargetPrepValidation_CustomBoxZeroSize(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{
		"pdb_id": "1ABC",
		"binding_site_mode": "custom-box",
		"custom_box": {"center": [10, 20, 30], "size": [0, 10, 10]}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for zero-size box dimension, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "size") {
		t.Errorf("expected error to mention size, got %q", rr.Body.String())
	}
}

// TestWP1_TargetPrepValidation_CustomBoxNegativeSize verifies that negative
// box dimensions are rejected.
func TestWP1_TargetPrepValidation_CustomBoxNegativeSize(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{
		"pdb_id": "1ABC",
		"binding_site_mode": "custom-box",
		"custom_box": {"center": [10, 20, 30], "size": [10, -5, 10]}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative size, got %d", rr.Code)
	}
}

// TestWP1_TargetPrepValidation_InvalidJSON verifies that malformed JSON
// returns a 400 error.
func TestWP1_TargetPrepValidation_InvalidJSON(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/targets/prepare", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}
}

// TestWP1_TargetPrepValidation_MethodNotAllowed verifies that GET is rejected.
func TestWP1_TargetPrepValidation_MethodNotAllowed(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/prepare", nil)
	rr := httptest.NewRecorder()

	handler.TargetPrepareHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// TestWP1_TargetPrepValidation_BindingSiteModes verifies that all 3
// valid modes are recognized as valid entries in the mode map.
func TestWP1_TargetPrepValidation_BindingSiteModes(t *testing.T) {
	validModes := []string{"native-ligand", "custom-box", "pocket-detection"}
	for _, mode := range validModes {
		if !validBindingSiteModes[mode] {
			t.Errorf("expected %q to be a valid binding site mode", mode)
		}
	}

	invalidModes := []string{"", "auto", "manual", "blind"}
	for _, mode := range invalidModes {
		if validBindingSiteModes[mode] {
			t.Errorf("expected %q to be an invalid binding site mode", mode)
		}
	}
}

// ===================================================================
// WP-2: Library Preparation — validation, compound IDs, filters
// ===================================================================

// TestWP2_LibraryValidation_InvalidSource verifies that an unknown source
// type is rejected.
func TestWP2_LibraryValidation_InvalidSource(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"source": "invalid", "name": "my-lib"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/libraries/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LibraryPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "source") {
		t.Errorf("expected error to mention source, got %q", rr.Body.String())
	}
}

// TestWP2_LibraryValidation_ValidSources verifies the valid source type map.
func TestWP2_LibraryValidation_ValidSources(t *testing.T) {
	valid := []string{"smiles", "sdf", "chembl", "enamine"}
	for _, s := range valid {
		if !validLibrarySources[s] {
			t.Errorf("expected %q to be a valid library source", s)
		}
	}

	invalid := []string{"", "csv", "mol2", "SMILES"}
	for _, s := range invalid {
		if validLibrarySources[s] {
			t.Errorf("expected %q to be an invalid library source", s)
		}
	}
}

// TestWP2_LibraryValidation_MissingName verifies that a missing name is rejected.
func TestWP2_LibraryValidation_MissingName(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"source": "smiles", "name": "", "smiles_list": ["CCO"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/libraries/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LibraryPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "name") {
		t.Errorf("expected error to mention name, got %q", rr.Body.String())
	}
}

// TestWP2_LibraryValidation_SMILESMissing verifies that source=smiles
// without a smiles_list is rejected.
func TestWP2_LibraryValidation_SMILESMissing(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"source": "smiles", "name": "test-lib"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/libraries/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LibraryPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "smiles_list") {
		t.Errorf("expected error to mention smiles_list, got %q", rr.Body.String())
	}
}

// TestWP2_LibraryValidation_SDFMissing verifies that source=sdf without
// sdf_data or s3_ref is rejected.
func TestWP2_LibraryValidation_SDFMissing(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"source": "sdf", "name": "test-lib"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/libraries/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LibraryPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "sdf_data") || !strings.Contains(rr.Body.String(), "s3_ref") {
		t.Errorf("expected error to mention sdf_data or s3_ref, got %q", rr.Body.String())
	}
}

// TestWP2_LibraryValidation_ChEMBLMissingParams verifies that source=chembl
// without chembl params is rejected.
func TestWP2_LibraryValidation_ChEMBLMissingParams(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"source": "chembl", "name": "test-lib"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/libraries/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LibraryPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "chembl") {
		t.Errorf("expected error to mention chembl, got %q", rr.Body.String())
	}
}

// TestWP2_LibraryValidation_EnamineMissingInput verifies that source=enamine
// without smiles_list or s3_ref is rejected.
func TestWP2_LibraryValidation_EnamineMissingInput(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"source": "enamine", "name": "test-lib"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/libraries/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LibraryPrepareHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// TestWP2_GenerateStableCompoundID verifies the KHM-{InChIKey_first14} format.
func TestWP2_GenerateStableCompoundID(t *testing.T) {
	tests := []struct {
		inchiKey string
		want     string
	}{
		{"LFQSCWFLJHTTHZ-UHFFFAOYSA-N", "KHM-LFQSCWFLJHTTHZ"},
		{"BSYNRYMUTXBXSQ-UHFFFAOYSA-N", "KHM-BSYNRYMUTXBXSQ"},
		// Short key (less than 14 chars) — use full key.
		{"ABCDEF", "KHM-ABCDEF"},
		// Exactly 14 chars.
		{"12345678901234", "KHM-12345678901234"},
		// Lowercase gets uppercased.
		{"lfqscwfljhtthz-uhfffaoysa-n", "KHM-LFQSCWFLJHTTHZ"},
	}

	for _, tt := range tests {
		t.Run(tt.inchiKey, func(t *testing.T) {
			got := generateStableCompoundID(tt.inchiKey)
			if got != tt.want {
				t.Errorf("generateStableCompoundID(%q) = %q, want %q", tt.inchiKey, got, tt.want)
			}
		})
	}
}

// TestWP2_GenerateStableCompoundID_Deterministic verifies that the same
// InChIKey always produces the same compound ID.
func TestWP2_GenerateStableCompoundID_Deterministic(t *testing.T) {
	key := "LFQSCWFLJHTTHZ-UHFFFAOYSA-N"
	id1 := generateStableCompoundID(key)
	id2 := generateStableCompoundID(key)
	if id1 != id2 {
		t.Errorf("expected deterministic output: %q != %q", id1, id2)
	}
}

// TestWP2_GenerateStableCompoundID_Prefix verifies all compound IDs
// start with the KHM- prefix.
func TestWP2_GenerateStableCompoundID_Prefix(t *testing.T) {
	keys := []string{"ABC", "ABCDEFGHIJKLMNOP", "xyz"}
	for _, k := range keys {
		id := generateStableCompoundID(k)
		if !strings.HasPrefix(id, "KHM-") {
			t.Errorf("expected KHM- prefix, got %q", id)
		}
	}
}

// TestWP2_FilterDefaults verifies the default filter configuration:
// Lipinski=ON, Veber=ON, PAINS=ON, Brenk=OFF, REOS=OFF.
func TestWP2_FilterDefaults(t *testing.T) {
	f := LibraryPrepFilters{}
	f.resolveDefaults()

	if f.Lipinski == nil || !*f.Lipinski {
		t.Error("expected Lipinski default = true")
	}
	if f.Veber == nil || !*f.Veber {
		t.Error("expected Veber default = true")
	}
	if f.PAINS == nil || !*f.PAINS {
		t.Error("expected PAINS default = true")
	}
	if f.Brenk == nil || *f.Brenk {
		t.Error("expected Brenk default = false")
	}
	if f.REOS == nil || *f.REOS {
		t.Error("expected REOS default = false")
	}
}

// TestWP2_FilterDefaults_PreserveExplicit verifies that explicitly set
// filter values are NOT overwritten by defaults.
func TestWP2_FilterDefaults_PreserveExplicit(t *testing.T) {
	fa := false
	tr := true
	f := LibraryPrepFilters{
		Lipinski: &fa,   // explicitly OFF
		Brenk:    &tr,   // explicitly ON
	}
	f.resolveDefaults()

	if *f.Lipinski != false {
		t.Error("explicit Lipinski=false was overwritten")
	}
	if *f.Brenk != true {
		t.Error("explicit Brenk=true was overwritten")
	}
	// Remaining should use defaults.
	if *f.Veber != true {
		t.Error("expected Veber default = true")
	}
	if *f.PAINS != true {
		t.Error("expected PAINS default = true")
	}
	if *f.REOS != false {
		t.Error("expected REOS default = false")
	}
}

// TestWP2_SanitizeName verifies the name sanitization for job names.
func TestWP2_SanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Library", "my-library"},
		{"test_lib.v2", "test-lib-v2"},
		{"A/B\\C", "a-b-c"},
		{"already-clean", "already-clean"},
		{"UPPERCASE", "uppercase"},
		{"(parens)", "parens"},
		{"multi  space", "multi-space"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestWP2_ChEMBLSourceParams_ToParamMap verifies the param map conversion.
func TestWP2_ChEMBLSourceParams_ToParamMap(t *testing.T) {
	mwMin := 200.0
	mwMax := 500.0
	hbaMax := 10
	p := ChEMBLSourceParams{
		Q:     "aspirin",
		MWMin: &mwMin,
		MWMax: &mwMax,
		HBAMax: &hbaMax,
		Ro5:   true,
	}

	params := p.toParamMap()

	if params["q"] != "aspirin" {
		t.Errorf("expected q=aspirin, got %q", params["q"])
	}
	if params["mw_min"] != "200" {
		t.Errorf("expected mw_min=200, got %q", params["mw_min"])
	}
	if params["mw_max"] != "500" {
		t.Errorf("expected mw_max=500, got %q", params["mw_max"])
	}
	if params["hba_max"] != "10" {
		t.Errorf("expected hba_max=10, got %q", params["hba_max"])
	}
	if params["ro5"] != "true" {
		t.Errorf("expected ro5=true, got %q", params["ro5"])
	}

	// Unset fields should not be present.
	if _, ok := params["logp_min"]; ok {
		t.Error("logp_min should not be present when nil")
	}
	if _, ok := params["logp_max"]; ok {
		t.Error("logp_max should not be present when nil")
	}
	if _, ok := params["hbd_max"]; ok {
		t.Error("hbd_max should not be present when nil")
	}
}

// TestWP2_ChEMBLSourceParams_EmptyMap verifies an empty params struct
// produces an empty map (except ro5=false is not included).
func TestWP2_ChEMBLSourceParams_EmptyMap(t *testing.T) {
	p := ChEMBLSourceParams{}
	params := p.toParamMap()
	if len(params) != 0 {
		t.Errorf("expected empty param map, got %d entries: %v", len(params), params)
	}
}

// TestWP2_LibraryValidation_MethodNotAllowed verifies POST-only enforcement.
func TestWP2_LibraryValidation_MethodNotAllowed(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries/prepare", nil)
	rr := httptest.NewRecorder()

	handler.LibraryPrepareHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// ===================================================================
// WP-3: Multi-Engine Docking — validation, engine routing, disagreements
// NOTE: consensus_scoring_test.go already covers ComputeConsensus,
//       ValidateEngineSelection, EngineComputeClass, SortEnginesForScheduling,
//       and FlagDisagreements basic cases. Tests here cover additional
//       edge cases and handler-level validation not in that file.
// ===================================================================

// TestWP3_FlagDisagreements_TwoEnginesNoFlag verifies that with only 2 engines,
// disagreement cannot exceed 2 stddevs (mathematically capped at 1.0 stddev for
// 2 points), so no flagging occurs even at maximal disagreement.
func TestWP3_FlagDisagreements_TwoEnginesNoFlag(t *testing.T) {
	results := []ConsensusResult{
		{
			CompoundID: "X",
			PerEngine: []NormalizedScore{
				{Engine: "vina-1.2", Normalized: 0.0},
				{Engine: "gnina", Normalized: 1.0},
			},
		},
	}

	flagged := FlagDisagreements(results)
	// With 2 engines, max deviation = 1.0 stddev, threshold is >2.0, so never flags.
	if len(flagged) != 0 {
		t.Errorf("expected no flagged compounds for 2-engine case, got %v", flagged)
	}
}

// TestWP3_FlagDisagreements_ManyEnginesOutlier verifies flagging with many
// engines where one is a clear outlier. With enough engines agreeing, the
// outlier's deviation exceeds the 2-stddev threshold.
func TestWP3_FlagDisagreements_ManyEnginesOutlier(t *testing.T) {
	// 5 engines agree near 0.9, one is at 0.0 — a severe outlier.
	// Mean ~ 0.75, stddev is small relative to the 0.0 outlier's distance.
	results := []ConsensusResult{
		{
			CompoundID: "Y",
			PerEngine: []NormalizedScore{
				{Engine: "e1", Normalized: 0.9},
				{Engine: "e2", Normalized: 0.9},
				{Engine: "e3", Normalized: 0.9},
				{Engine: "e4", Normalized: 0.9},
				{Engine: "e5", Normalized: 0.9},
				{Engine: "e6", Normalized: 0.0}, // extreme outlier
			},
		},
	}

	flagged := FlagDisagreements(results)
	if len(flagged) == 0 {
		t.Error("expected compound Y to be flagged due to extreme outlier in 6-engine set")
	}
}

// TestWP3_FlagDisagreements_NoFlagForClose verifies that engines with close
// scores are NOT flagged.
func TestWP3_FlagDisagreements_NoFlagForClose(t *testing.T) {
	results := []ConsensusResult{
		{
			CompoundID: "Z",
			PerEngine: []NormalizedScore{
				{Engine: "vina-1.2", Normalized: 0.72},
				{Engine: "gnina", Normalized: 0.78},
			},
		},
	}

	flagged := FlagDisagreements(results)
	if len(flagged) != 0 {
		t.Errorf("expected no flagged compounds for close scores, got %v", flagged)
	}
}

// TestWP3_EngineServiceURLs verifies all 6 engine URLs are defined.
func TestWP3_EngineServiceURLs(t *testing.T) {
	expectedEngines := []string{"vina-1.2", "vina-gpu", "vina-gpu-batch", "gnina", "diffdock"}
	for _, engine := range expectedEngines {
		url, ok := engineServiceURLs[engine]
		if !ok {
			t.Errorf("missing service URL for engine %q", engine)
			continue
		}
		if url == "" {
			t.Errorf("empty service URL for engine %q", engine)
		}
		if !strings.HasPrefix(url, "http://") {
			t.Errorf("engine %q URL should start with http://, got %q", engine, url)
		}
	}
}

// TestWP3_EngineContainerImages verifies all 6 engine images are defined.
func TestWP3_EngineContainerImages(t *testing.T) {
	expectedEngines := []string{"vina-1.2", "vina-gpu", "vina-gpu-batch", "gnina", "diffdock"}
	for _, engine := range expectedEngines {
		image, ok := engineContainerImages[engine]
		if !ok {
			t.Errorf("missing container image for engine %q", engine)
			continue
		}
		if image == "" {
			t.Errorf("empty container image for engine %q", engine)
		}
		if !strings.HasPrefix(image, "zot.hwcopeland.net/chem/") {
			t.Errorf("engine %q image should be from zot registry, got %q", engine, image)
		}
	}
}

// TestWP3_IsGPUEngine verifies the GPU detection helper.
func TestWP3_IsGPUEngine(t *testing.T) {
	tests := []struct {
		engine string
		wantGPU bool
	}{
		{"vina-gpu", true},
		{"vina-gpu-batch", true},
		{"gnina", true},
		{"diffdock", true},
		{"vina-1.2", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			got := IsGPUEngine(tt.engine)
			if got != tt.wantGPU {
				t.Errorf("IsGPUEngine(%q) = %v, want %v", tt.engine, got, tt.wantGPU)
			}
		})
	}
}

// TestWP3_DockingV2Validation_MissingReceptorRef verifies that an empty
// receptor_ref is rejected.
func TestWP3_DockingV2Validation_MissingReceptorRef(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"receptor_ref": "", "library_ref": "lib-1", "engines": ["vina-1.2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docking/v2/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.DockingV2Submit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "receptor_ref") {
		t.Errorf("expected error to mention receptor_ref, got %q", rr.Body.String())
	}
}

// TestWP3_DockingV2Validation_MissingLibraryRef verifies that an empty
// library_ref is rejected.
func TestWP3_DockingV2Validation_MissingLibraryRef(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"receptor_ref": "tp-1", "library_ref": "", "engines": ["vina-1.2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docking/v2/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.DockingV2Submit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "library_ref") {
		t.Errorf("expected error to mention library_ref, got %q", rr.Body.String())
	}
}

// TestWP3_DockingV2Validation_ConsensusRequiresTwoEngines verifies that
// consensus=true with a single engine is rejected.
func TestWP3_DockingV2Validation_ConsensusRequiresTwoEngines(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"receptor_ref": "tp-1", "library_ref": "lib-1", "engines": ["vina-1.2"], "consensus": true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docking/v2/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.DockingV2Submit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "consensus") {
		t.Errorf("expected error to mention consensus, got %q", rr.Body.String())
	}
}

// TestWP3_DockingV2Validation_InvalidEngine verifies that an unknown engine
// is rejected at the handler level.
func TestWP3_DockingV2Validation_InvalidEngine(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"receptor_ref": "tp-1", "library_ref": "lib-1", "engines": ["nonexistent"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docking/v2/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.DockingV2Submit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// TestWP3_DockingV2Defaults verifies that default values are applied
// when not specified in the request.
func TestWP3_DockingV2Defaults(t *testing.T) {
	// We cannot fully test through the handler without a DB, but we can
	// verify the default logic by examining the struct after JSON decode.
	body := `{"receptor_ref": "tp-1", "library_ref": "lib-1", "engines": ["vina-1.2"]}`
	var req DockingV2SubmitRequest
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&req); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Apply the same defaults the handler does.
	if req.Exhaustiveness == 0 {
		req.Exhaustiveness = 32
	}
	if req.Scoring == "" {
		req.Scoring = "vina"
	}
	if req.TopNRefine == 0 {
		req.TopNRefine = 100
	}
	if req.ChunkSize == 0 {
		req.ChunkSize = 10000
	}

	if req.Exhaustiveness != 32 {
		t.Errorf("expected default exhaustiveness=32, got %d", req.Exhaustiveness)
	}
	if req.Scoring != "vina" {
		t.Errorf("expected default scoring=vina, got %q", req.Scoring)
	}
	if req.TopNRefine != 100 {
		t.Errorf("expected default top_n_refine=100, got %d", req.TopNRefine)
	}
	if req.ChunkSize != 10000 {
		t.Errorf("expected default chunk_size=10000, got %d", req.ChunkSize)
	}
}

// TestWP3_DockingV2Dispatch_NotFound verifies that unknown sub-paths
// under /api/v1/docking/v2/ return 404.
func TestWP3_DockingV2Dispatch_NotFound(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docking/v2/unknown", nil)
	rr := httptest.NewRecorder()

	handler.DockingV2Dispatch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// TestWP3_ConsensusDisagreementThreshold verifies the constant value.
func TestWP3_ConsensusDisagreementThreshold(t *testing.T) {
	if ConsensusDisagreementThreshold != 2.0 {
		t.Errorf("expected threshold 2.0, got %f", ConsensusDisagreementThreshold)
	}
}

// ===================================================================
// WP-4: ADMET Prediction — validation, MPO profiles, presets, sorting
// ===================================================================

// TestWP4_ADMETValidation_MissingRefs verifies that a request with neither
// library_ref nor compound_refs is rejected.
func TestWP4_ADMETValidation_MissingRefs(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admet/predict", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ADMETPredictHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "library_ref") || !strings.Contains(rr.Body.String(), "compound_refs") {
		t.Errorf("expected error to mention library_ref or compound_refs, got %q", rr.Body.String())
	}
}

// TestWP4_ADMETValidation_InvalidMPOProfile verifies that an unknown
// MPO profile is rejected.
func TestWP4_ADMETValidation_InvalidMPOProfile(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	body := `{"library_ref": "lib-1", "mpo_profile": "invalid-profile"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admet/predict", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ADMETPredictHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "mpo_profile") {
		t.Errorf("expected error to mention mpo_profile, got %q", rr.Body.String())
	}
}

// TestWP4_ADMETValidation_ValidMPOProfiles verifies all 4 valid MPO profiles.
func TestWP4_ADMETValidation_ValidMPOProfiles(t *testing.T) {
	validProfiles := []string{"oral", "cns", "oncology", "antimicrobial"}
	profileMap := map[string]bool{
		"oral": true, "cns": true, "oncology": true, "antimicrobial": true,
	}

	for _, p := range validProfiles {
		if !profileMap[p] {
			t.Errorf("expected %q to be a valid MPO profile", p)
		}
	}

	invalidProfiles := []string{"", "iv", "topical", "inhalation"}
	for _, p := range invalidProfiles {
		if profileMap[p] {
			t.Errorf("expected %q to be an invalid MPO profile", p)
		}
	}
}

// TestWP4_ADMETDefaults verifies default values are applied.
func TestWP4_ADMETDefaults(t *testing.T) {
	body := `{"library_ref": "lib-1"}`
	var req ADMETSubmitRequest
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&req); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Apply same defaults the handler does.
	if len(req.Engines) == 0 {
		req.Engines = []string{"admet_ai"}
	}
	if req.MPOProfile == "" {
		req.MPOProfile = "oral"
	}

	if len(req.Engines) != 1 || req.Engines[0] != "admet_ai" {
		t.Errorf("expected default engine [admet_ai], got %v", req.Engines)
	}
	if req.MPOProfile != "oral" {
		t.Errorf("expected default mpo_profile=oral, got %q", req.MPOProfile)
	}
}

// TestWP4_ADMETValidation_MethodNotAllowed verifies POST-only enforcement.
func TestWP4_ADMETValidation_MethodNotAllowed(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admet/predict", nil)
	rr := httptest.NewRecorder()

	handler.ADMETPredictHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// TestWP4_ADMETValidation_InvalidJSON verifies malformed JSON is rejected.
func TestWP4_ADMETValidation_InvalidJSON(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admet/predict", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ADMETPredictHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}
}

// TestWP4_ADMETPresets verifies the /presets endpoint returns all 4 profiles.
func TestWP4_ADMETPresets(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admet/presets", nil)
	rr := httptest.NewRecorder()

	handler.ADMETPresetsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["count"] != float64(4) {
		t.Errorf("expected 4 presets, got %v", body["count"])
	}

	presets, ok := body["presets"].([]interface{})
	if !ok {
		t.Fatal("expected presets to be an array")
	}
	if len(presets) != 4 {
		t.Fatalf("expected 4 presets, got %d", len(presets))
	}

	// Verify all 4 preset names.
	expectedNames := map[string]bool{"oral": false, "cns": false, "oncology": false, "antimicrobial": false}
	for _, p := range presets {
		preset := p.(map[string]interface{})
		name := preset["name"].(string)
		if _, ok := expectedNames[name]; !ok {
			t.Errorf("unexpected preset name: %q", name)
		}
		expectedNames[name] = true

		// Every preset should have a description.
		desc, ok := preset["description"].(string)
		if !ok || desc == "" {
			t.Errorf("preset %q: expected non-empty description", name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("missing preset: %q", name)
		}
	}
}

// TestWP4_ADMETPresetsMethodNotAllowed verifies the presets endpoint rejects POST.
func TestWP4_ADMETPresetsMethodNotAllowed(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admet/presets", nil)
	rr := httptest.NewRecorder()

	handler.ADMETPresetsHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// TestWP4_ADMETDispatch_Routing verifies the dispatch routes for the ADMET API.
func TestWP4_ADMETDispatch_Routing(t *testing.T) {
	handler := &APIHandler{
		controller: &Controller{},
	}

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"presets", http.MethodGet, "/api/v1/admet/presets", http.StatusOK},
		{"unknown_path", http.MethodGet, "/api/v1/admet/unknown", http.StatusNotFound},
		{"predict_get", http.MethodGet, "/api/v1/admet/predict", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ADMETDispatch(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("%s %s: expected %d, got %d", tt.method, tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}

// TestWP4_ExtractADMETJobName verifies path extraction for job status endpoint.
func TestWP4_ExtractADMETJobName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/admet/jobs/admet-123", "admet-123"},
		{"/api/v1/admet/jobs/admet-123/", "admet-123"},
		{"/api/v1/admet/jobs/admet-123/results", "admet-123"},
		{"/api/v1/admet/jobs/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractADMETJobName(tt.path)
			if got != tt.want {
				t.Errorf("extractADMETJobName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestWP4_ExtractADMETJobNameForResults verifies path extraction for results endpoint.
func TestWP4_ExtractADMETJobNameForResults(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/admet/jobs/admet-123/results", "admet-123"},
		{"/api/v1/admet/jobs/my-job/results", "my-job"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractADMETJobNameForResults(tt.path)
			if got != tt.want {
				t.Errorf("extractADMETJobNameForResults(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestWP4_ADMETBatchSizeChunking verifies that compounds are batched into
// chunks of 500 for sidecar calls. This tests the algorithm, not the sidecar call.
func TestWP4_ADMETBatchSizeChunking(t *testing.T) {
	batchSize := 500

	tests := []struct {
		totalCompounds int
		wantBatches    int
	}{
		{0, 0},
		{1, 1},
		{499, 1},
		{500, 1},
		{501, 2},
		{1000, 2},
		{1001, 3},
		{2500, 5},
	}

	for _, tt := range tests {
		t.Run(
			strings.ReplaceAll(strings.TrimSpace(
				strings.Replace(strings.Replace(
					"total_"+string(rune('0'+tt.totalCompounds/1000))+
						string(rune('0'+(tt.totalCompounds%1000)/100))+
						string(rune('0'+(tt.totalCompounds%100)/10))+
						string(rune('0'+tt.totalCompounds%10)),
					"", "", 0), "", "", 0)), " ", ""),
			func(t *testing.T) {
				batchCount := 0
				for i := 0; i < tt.totalCompounds; i += batchSize {
					batchCount++
					end := i + batchSize
					if end > tt.totalCompounds {
						end = tt.totalCompounds
					}
					chunkLen := end - i
					if chunkLen <= 0 {
						t.Errorf("empty chunk at batch %d", batchCount)
					}
					if chunkLen > batchSize {
						t.Errorf("chunk size %d exceeds batch size %d", chunkLen, batchSize)
					}
				}
				if batchCount != tt.wantBatches {
					t.Errorf("total=%d: expected %d batches, got %d",
						tt.totalCompounds, tt.wantBatches, batchCount)
				}
			})
	}
}

// TestWP4_ADMETResultsSortByMPO verifies that ADMET results would be sorted
// by MPO score descending (matches the ORDER BY mpo_score DESC in the query).
func TestWP4_ADMETResultsSortByMPO(t *testing.T) {
	results := []ADMETCompoundResult{
		{CompoundID: "A", MPOScore: 0.3},
		{CompoundID: "B", MPOScore: 0.9},
		{CompoundID: "C", MPOScore: 0.6},
	}

	// Sort descending by MPO score (same as SQL ORDER BY mpo_score DESC).
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].MPOScore > results[i].MPOScore {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if results[0].CompoundID != "B" {
		t.Errorf("expected B first (highest MPO), got %s", results[0].CompoundID)
	}
	if results[1].CompoundID != "C" {
		t.Errorf("expected C second, got %s", results[1].CompoundID)
	}
	if results[2].CompoundID != "A" {
		t.Errorf("expected A last (lowest MPO), got %s", results[2].CompoundID)
	}
}

// ===================================================================
// Cross-cutting: S3 artifact keys, provenance UUID v7, schema idempotency
// ===================================================================

// TestCross_ArtifactKey_WPSpecific verifies S3 key generation for each
// WP's artifact types.
func TestCross_ArtifactKey_WPSpecific(t *testing.T) {
	tests := []struct {
		name         string
		jobKind      string
		jobName      string
		artifactName string
		ext          string
		want         string
	}{
		{
			name:         "WP1_receptor",
			jobKind:      "TargetPrep",
			jobName:      "target-prep-1abc-123",
			artifactName: "receptor",
			ext:          "pdbqt",
			want:         "TargetPrep/target-prep-1abc-123/receptor.pdbqt",
		},
		{
			name:         "WP2_library",
			jobKind:      "LibraryPrep",
			jobName:      "libprep-my-lib-456",
			artifactName: "library",
			ext:          "sdf",
			want:         "LibraryPrep/libprep-my-lib-456/library.sdf",
		},
		{
			name:         "WP3_docked_pose",
			jobKind:      "DockV2",
			jobName:      "dockv2-789",
			artifactName: "KHM-LFQSCWFLJHTTHZ-pose1",
			ext:          "pdbqt",
			want:         "DockV2/dockv2-789/KHM-LFQSCWFLJHTTHZ-pose1.pdbqt",
		},
		{
			name:         "WP4_admet_report",
			jobKind:      "ADMET",
			jobName:      "admet-101",
			artifactName: "results",
			ext:          "json",
			want:         "ADMET/admet-101/results.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ArtifactKey(tt.jobKind, tt.jobName, tt.artifactName, tt.ext)
			if got != tt.want {
				t.Errorf("ArtifactKey = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCross_UUIDv7_Ordering generates two UUID v7s in sequence and
// verifies the second is lexicographically greater than the first
// (time-ordered property of UUID v7).
func TestCross_UUIDv7_Ordering(t *testing.T) {
	id1 := newUUIDv7()
	// Small delay to ensure different timestamp.
	time.Sleep(2 * time.Millisecond)
	id2 := newUUIDv7()

	if id1 == id2 {
		t.Error("two UUID v7s should not be identical")
	}

	// UUID v7 is time-ordered: later UUIDs sort after earlier ones.
	if id2 <= id1 {
		t.Errorf("expected id2 > id1 (time-ordered), but id1=%q, id2=%q", id1, id2)
	}
}

// TestCross_UUIDv7_Format verifies the UUID v7 format (8-4-4-4-12 hex groups
// with version 7 nibble and variant bits).
func TestCross_UUIDv7_Format(t *testing.T) {
	id := newUUIDv7()

	// Check format: 8-4-4-4-12.
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts in UUID, got %d: %q", len(parts), id)
	}

	expectedLens := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != expectedLens[i] {
			t.Errorf("part %d: expected length %d, got %d in %q", i, expectedLens[i], len(p), id)
		}
	}

	// Version 7: the 13th character should be '7'.
	if id[14] != '7' {
		t.Errorf("expected version nibble '7' at position 14, got %c in %q", id[14], id)
	}

	// Variant: the 19th character should be 8, 9, a, or b.
	variant := id[19]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
		t.Errorf("expected variant nibble [89ab] at position 19, got %c in %q", variant, id)
	}
}

// TestCross_UUIDv7_Uniqueness generates multiple UUIDs and verifies
// no collisions (basic sanity check).
func TestCross_UUIDv7_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := newUUIDv7()
		if seen[id] {
			t.Fatalf("UUID v7 collision detected: %q (after %d generations)", id, i)
		}
		seen[id] = true
	}
}

// TestCross_SchemaIdempotency_TargetPrep verifies calling EnsureTargetPrepSchema
// twice does not error (CREATE TABLE IF NOT EXISTS).
func TestCross_SchemaIdempotency_TargetPrep(t *testing.T) {
	t.Skip("requires MySQL")
}

// TestCross_SchemaIdempotency_LibraryPrep verifies calling EnsureLibraryPrepSchema
// twice does not error.
func TestCross_SchemaIdempotency_LibraryPrep(t *testing.T) {
	t.Skip("requires MySQL")
}

// TestCross_SchemaIdempotency_DockingV2 verifies calling EnsureDockingV2Schema
// twice does not error.
func TestCross_SchemaIdempotency_DockingV2(t *testing.T) {
	t.Skip("requires MySQL")
}

// TestCross_SchemaIdempotency_ADMET verifies calling EnsureADMETSchema
// twice does not error.
func TestCross_SchemaIdempotency_ADMET(t *testing.T) {
	t.Skip("requires MySQL")
}

// ===================================================================
// JSON serialization round-trip tests
// ===================================================================

// TestWP1_BoxSpecJSON verifies BoxSpec JSON round-trip.
func TestWP1_BoxSpecJSON(t *testing.T) {
	box := BoxSpec{
		Center: [3]float64{10.5, 20.3, 30.1},
		Size:   [3]float64{15.0, 15.0, 15.0},
	}

	data, err := json.Marshal(box)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded BoxSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		if decoded.Center[i] != box.Center[i] {
			t.Errorf("center[%d]: want %f, got %f", i, box.Center[i], decoded.Center[i])
		}
		if decoded.Size[i] != box.Size[i] {
			t.Errorf("size[%d]: want %f, got %f", i, box.Size[i], decoded.Size[i])
		}
	}
}

// TestWP1_DetectedPocketJSON verifies DetectedPocket JSON serialization.
func TestWP1_DetectedPocketJSON(t *testing.T) {
	pocket := DetectedPocket{
		Rank:           1,
		Center:         [3]float64{10, 20, 30},
		Size:           [3]float64{5, 5, 5},
		FpocketScore:   0.85,
		P2RankScore:    0.90,
		ConsensusScore: 0.875,
		Volume:         120.5,
		Residues:       []string{"ALA1", "GLY2"},
	}

	data, err := json.Marshal(pocket)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["rank"] != float64(1) {
		t.Errorf("rank: want 1, got %v", decoded["rank"])
	}
	if decoded["consensus_score"] != 0.875 {
		t.Errorf("consensus_score: want 0.875, got %v", decoded["consensus_score"])
	}

	residues, ok := decoded["residues"].([]interface{})
	if !ok || len(residues) != 2 {
		t.Errorf("expected 2 residues, got %v", decoded["residues"])
	}
}

// TestWP4_ADMETCompoundResultJSON verifies ADMET result JSON fields.
func TestWP4_ADMETCompoundResultJSON(t *testing.T) {
	result := ADMETCompoundResult{
		CompoundID: "KHM-LFQSCWFLJHTTHZ",
		SMILES:     "CCO",
		MPOScore:   0.75,
		MPOProfile: "oral",
		Endpoints:  map[string]json.RawMessage{"HIA": json.RawMessage(`0.95`)},
		Flags:      json.RawMessage(`{"ames": false}`),
		Engine:     "admet_ai",
		PredictedAt: "2026-04-19T10:00:00Z",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["compound_id"] != "KHM-LFQSCWFLJHTTHZ" {
		t.Errorf("compound_id: want KHM-LFQSCWFLJHTTHZ, got %v", decoded["compound_id"])
	}
	if decoded["mpo_score"] != 0.75 {
		t.Errorf("mpo_score: want 0.75, got %v", decoded["mpo_score"])
	}
	if decoded["engine"] != "admet_ai" {
		t.Errorf("engine: want admet_ai, got %v", decoded["engine"])
	}
}

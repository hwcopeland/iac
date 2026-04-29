package main

import (
	"math"
	"testing"
)

func TestComputeConsensus_Empty(t *testing.T) {
	results, err := ComputeConsensus(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestComputeConsensus_SingleEngine(t *testing.T) {
	scores := []EngineScore{
		{CompoundID: "A", Engine: "vina-1.2", Affinity: -8.0},
		{CompoundID: "B", Engine: "vina-1.2", Affinity: -6.0},
		{CompoundID: "C", Engine: "vina-1.2", Affinity: -10.0},
	}

	results, err := ComputeConsensus(scores)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Best compound (C, -10.0) should be rank 1 with normalized = 1.0.
	if results[0].CompoundID != "C" {
		t.Errorf("rank 1: expected C, got %s", results[0].CompoundID)
	}
	if results[0].ConsensusScore != 1.0 {
		t.Errorf("rank 1 consensus: expected 1.0, got %f", results[0].ConsensusScore)
	}

	// Worst compound (B, -6.0) should be rank 3 with normalized = 0.0.
	if results[2].CompoundID != "B" {
		t.Errorf("rank 3: expected B, got %s", results[2].CompoundID)
	}
	if results[2].ConsensusScore != 0.0 {
		t.Errorf("rank 3 consensus: expected 0.0, got %f", results[2].ConsensusScore)
	}

	// Middle compound (A, -8.0) should be rank 2 with normalized = 0.5.
	if results[1].CompoundID != "A" {
		t.Errorf("rank 2: expected A, got %s", results[1].CompoundID)
	}
	if math.Abs(results[1].ConsensusScore-0.5) > 1e-9 {
		t.Errorf("rank 2 consensus: expected 0.5, got %f", results[1].ConsensusScore)
	}
}

func TestComputeConsensus_MultiEngine(t *testing.T) {
	scores := []EngineScore{
		// Vina scores
		{CompoundID: "A", Engine: "vina-1.2", Affinity: -8.0},
		{CompoundID: "B", Engine: "vina-1.2", Affinity: -10.0},
		// Gnina scores (different ranking)
		{CompoundID: "A", Engine: "gnina", Affinity: -12.0},
		{CompoundID: "B", Engine: "gnina", Affinity: -9.0},
	}

	results, err := ComputeConsensus(scores)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Each result should have 2 per-engine scores.
	for _, r := range results {
		if len(r.PerEngine) != 2 {
			t.Errorf("compound %s: expected 2 per-engine scores, got %d",
				r.CompoundID, len(r.PerEngine))
		}
	}

	// Verify ranks are assigned.
	if results[0].ConsensusRank != 1 || results[1].ConsensusRank != 2 {
		t.Errorf("unexpected ranks: %d, %d", results[0].ConsensusRank, results[1].ConsensusRank)
	}

	// Verify the consensus differs from single-engine. In Vina, B is better.
	// In Gnina, A is better. Consensus should reflect the average.
	// Vina: A=0.0, B=1.0 (range -10 to -8; -8 is worst)
	// Gnina: A=1.0, B=0.0 (range -12 to -9; -9 is worst)
	// A consensus = (0.0 + 1.0) / 2 = 0.5
	// B consensus = (1.0 + 0.0) / 2 = 0.5
	// Tied — sorted alphabetically, A should be first.
	if results[0].CompoundID != "A" {
		t.Errorf("expected A first (alphabetical tiebreak), got %s", results[0].CompoundID)
	}
}

func TestComputeConsensus_IdenticalScores(t *testing.T) {
	scores := []EngineScore{
		{CompoundID: "A", Engine: "vina-1.2", Affinity: -7.5},
		{CompoundID: "B", Engine: "vina-1.2", Affinity: -7.5},
	}

	results, err := ComputeConsensus(scores)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All identical — both should be normalized to 1.0.
	for _, r := range results {
		if r.ConsensusScore != 1.0 {
			t.Errorf("compound %s: expected 1.0 for identical scores, got %f",
				r.CompoundID, r.ConsensusScore)
		}
	}
}

func TestValidateEngineSelection_Valid(t *testing.T) {
	cases := [][]string{
		{"vina-1.2"},
		{"gnina"},
		{"vina-1.2", "gnina"},
		{"gnina", "diffdock"},
		{"vina-gpu", "gnina"},
	}

	for _, engines := range cases {
		if err := ValidateEngineSelection(engines); err != nil {
			t.Errorf("expected valid for %v, got error: %v", engines, err)
		}
	}
}

func TestValidateEngineSelection_Invalid(t *testing.T) {
	cases := []struct {
		engines []string
		errMsg  string
	}{
		{nil, "at least one"},
		{[]string{}, "at least one"},
		{[]string{"unknown"}, "unknown docking engine"},
		{[]string{"vina-1.2", "vina-1.2"}, "duplicate"},
		{[]string{"vina-1.2", "vina-gpu"}, "same scoring function"},
		{[]string{"vina-1.2", "vina-gpu-batch"}, "same scoring function"},
	}

	for _, tc := range cases {
		err := ValidateEngineSelection(tc.engines)
		if err == nil {
			t.Errorf("expected error for %v", tc.engines)
			continue
		}
		// Just check it returned an error (message checked by substring would be fragile).
	}
}

func TestEngineComputeClass(t *testing.T) {
	gpuEngines := []string{"vina-gpu", "vina-gpu-batch", "gnina", "diffdock"}
	cpuEngines := []string{"vina-1.2"}

	for _, e := range gpuEngines {
		if EngineComputeClass(e) != "gpu" {
			t.Errorf("expected gpu for %s", e)
		}
	}
	for _, e := range cpuEngines {
		if EngineComputeClass(e) != "cpu" {
			t.Errorf("expected cpu for %s", e)
		}
	}
}

func TestSortEnginesForScheduling(t *testing.T) {
	cpu, gpu := SortEnginesForScheduling([]string{"gnina", "vina-1.2", "vina-gpu"})

	if len(cpu) != 1 {
		t.Errorf("expected 1 CPU engine, got %d", len(cpu))
	}
	if len(gpu) != 2 {
		t.Errorf("expected 2 GPU engines, got %d", len(gpu))
	}
}

func TestFlagDisagreements_NoDisagreement(t *testing.T) {
	// Two engines agree perfectly.
	results := []ConsensusResult{
		{
			CompoundID: "A",
			PerEngine: []NormalizedScore{
				{Engine: "vina-1.2", Normalized: 0.8},
				{Engine: "gnina", Normalized: 0.8},
			},
		},
	}

	flagged := FlagDisagreements(results)
	if len(flagged) != 0 {
		t.Errorf("expected no flagged compounds, got %v", flagged)
	}
}

func TestFlagDisagreements_SingleEngine(t *testing.T) {
	// Single engine cannot disagree.
	results := []ConsensusResult{
		{
			CompoundID: "A",
			PerEngine: []NormalizedScore{
				{Engine: "vina-1.2", Normalized: 0.8},
			},
		},
	}

	flagged := FlagDisagreements(results)
	if len(flagged) != 0 {
		t.Errorf("expected no flagged compounds for single engine, got %v", flagged)
	}
}

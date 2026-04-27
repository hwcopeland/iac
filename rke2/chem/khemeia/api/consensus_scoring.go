// Package main provides consensus scoring for multi-engine docking results.
// When multiple docking engines are run against the same receptor-ligand set,
// this module normalizes per-engine scores to a common [0,1] scale and computes
// a consensus rank as the mean of normalized scores across engines.
//
// Normalization method: min-max per engine, where the best (most negative)
// affinity maps to 1.0 and the worst maps to 0.0. This follows the WP-3 spec:
// "Normalize scores per engine to [0,1] (best = 1.0)".
package main

import (
	"fmt"
	"math"
	"sort"
)

// EngineScore holds a single compound's score from one docking engine.
type EngineScore struct {
	CompoundID string  `json:"compound_id"`
	Engine     string  `json:"engine"`
	Affinity   float64 `json:"affinity_kcal_mol"` // lower (more negative) is better
}

// NormalizedScore holds a compound's normalized score from one engine.
type NormalizedScore struct {
	CompoundID string  `json:"compound_id"`
	Engine     string  `json:"engine"`
	RawScore   float64 `json:"raw_score"`
	Normalized float64 `json:"normalized"` // 0.0 = worst, 1.0 = best
}

// ConsensusResult holds the final consensus-scored result for one compound.
type ConsensusResult struct {
	CompoundID     string            `json:"compound_id"`
	ConsensusRank  int               `json:"consensus_rank"`
	ConsensusScore float64           `json:"consensus_score"` // mean of normalized scores
	PerEngine      []NormalizedScore `json:"per_engine"`
}

// engineMinMax tracks the range of affinities for a single engine.
type engineMinMax struct {
	min float64 // best (most negative) affinity
	max float64 // worst (least negative) affinity
}

// ComputeConsensus takes per-engine scores for a set of compounds and returns
// consensus-ranked results. The input is a flat slice of EngineScores; multiple
// engines may contribute scores for the same compound.
//
// Algorithm:
//  1. Group scores by engine.
//  2. Compute min/max affinity per engine.
//  3. Normalize each score to [0,1]: normalized = (max - score) / (max - min).
//     When max == min (single compound or all identical), normalized = 1.0.
//  4. For each compound, consensus = mean(normalized scores across engines).
//  5. Sort by consensus score descending (best first).
func ComputeConsensus(scores []EngineScore) ([]ConsensusResult, error) {
	if len(scores) == 0 {
		return []ConsensusResult{}, nil
	}

	// Step 1: Group by engine and compute min/max.
	byEngine := make(map[string][]EngineScore)
	ranges := make(map[string]*engineMinMax)

	for _, s := range scores {
		byEngine[s.Engine] = append(byEngine[s.Engine], s)

		r, ok := ranges[s.Engine]
		if !ok {
			r = &engineMinMax{min: s.Affinity, max: s.Affinity}
			ranges[s.Engine] = r
		}
		if s.Affinity < r.min {
			r.min = s.Affinity
		}
		if s.Affinity > r.max {
			r.max = s.Affinity
		}
	}

	// Step 2: Normalize each score and group by compound.
	type compoundScores struct {
		normalized []NormalizedScore
		total      float64
	}
	byCompound := make(map[string]*compoundScores)

	for engine, engineScores := range byEngine {
		r := ranges[engine]
		span := r.max - r.min

		for _, s := range engineScores {
			var norm float64
			if span == 0 {
				// All scores identical or single compound — assign 1.0.
				norm = 1.0
			} else {
				// Lower affinity (more negative) = better = higher normalized.
				norm = (r.max - s.Affinity) / span
			}

			ns := NormalizedScore{
				CompoundID: s.CompoundID,
				Engine:     engine,
				RawScore:   s.Affinity,
				Normalized: norm,
			}

			cs, ok := byCompound[s.CompoundID]
			if !ok {
				cs = &compoundScores{}
				byCompound[s.CompoundID] = cs
			}
			cs.normalized = append(cs.normalized, ns)
			cs.total += norm
		}
	}

	// Step 3: Compute consensus score = mean of normalized scores.
	results := make([]ConsensusResult, 0, len(byCompound))
	for compoundID, cs := range byCompound {
		mean := cs.total / float64(len(cs.normalized))

		// Sort per-engine scores by engine name for deterministic output.
		sort.Slice(cs.normalized, func(i, j int) bool {
			return cs.normalized[i].Engine < cs.normalized[j].Engine
		})

		results = append(results, ConsensusResult{
			CompoundID:     compoundID,
			ConsensusScore: mean,
			PerEngine:      cs.normalized,
		})
	}

	// Step 4: Sort by consensus score descending (best first).
	sort.Slice(results, func(i, j int) bool {
		if results[i].ConsensusScore != results[j].ConsensusScore {
			return results[i].ConsensusScore > results[j].ConsensusScore
		}
		return results[i].CompoundID < results[j].CompoundID
	})

	// Step 5: Assign ranks.
	for i := range results {
		results[i].ConsensusRank = i + 1
	}

	return results, nil
}

// ValidateEngineSelection validates the engine list for a consensus docking job.
// Returns an error if:
//   - engines is empty
//   - an unknown engine name is used
//   - vina-1.2 and vina-gpu are both selected (redundant scoring functions)
func ValidateEngineSelection(engines []string) error {
	if len(engines) == 0 {
		return fmt.Errorf("at least one docking engine must be specified")
	}

	known := map[string]bool{
		"vina-1.2":       true,
		"vina-gpu":       true,
		"vina-gpu-batch": true,
		"smina":          true,
		"gnina":          true,
		"diffdock":       true,
	}

	seen := make(map[string]bool, len(engines))
	for _, e := range engines {
		if !known[e] {
			return fmt.Errorf("unknown docking engine: %q", e)
		}
		if seen[e] {
			return fmt.Errorf("duplicate engine: %q", e)
		}
		seen[e] = true
	}

	// WP-3 spec: "Vina-GPU and Vina 1.2 share a scoring function — running
	// both in consensus is redundant. The consensus validator rejects this
	// pairing with a 400."
	if seen["vina-1.2"] && (seen["vina-gpu"] || seen["vina-gpu-batch"]) {
		return fmt.Errorf("vina-1.2 and vina-gpu share the same scoring function; " +
			"running both in consensus is redundant")
	}

	return nil
}

// EngineComputeClass returns the compute class for a given engine.
// GPU engines use the "gpu" class; CPU engines use "cpu".
func EngineComputeClass(engine string) string {
	switch engine {
	case "vina-gpu", "vina-gpu-batch", "gnina", "diffdock":
		return "gpu"
	default:
		return "cpu"
	}
}

// IsGPUEngine returns true if the engine requires GPU scheduling.
func IsGPUEngine(engine string) bool {
	return EngineComputeClass(engine) == "gpu"
}

// SortEnginesForScheduling orders engines with CPU engines first and GPU engines
// second. This enables the orchestrator to launch CPU docking pods in parallel
// while queuing GPU pods serially (single-GPU constraint on nixos-gpu).
func SortEnginesForScheduling(engines []string) (cpuEngines, gpuEngines []string) {
	for _, e := range engines {
		if IsGPUEngine(e) {
			gpuEngines = append(gpuEngines, e)
		} else {
			cpuEngines = append(cpuEngines, e)
		}
	}
	return cpuEngines, gpuEngines
}

// ConsensusDisagreementThreshold is the number of standard deviations beyond
// which engines are considered to disagree on a compound's ranking.
const ConsensusDisagreementThreshold = 2.0

// FlagDisagreements returns compound IDs where engines disagree by more than
// ConsensusDisagreementThreshold standard deviations. Per WP-3 spec:
// "Flag compounds where engines disagree by more than 2 standard deviations."
func FlagDisagreements(results []ConsensusResult) []string {
	var flagged []string

	for _, r := range results {
		if len(r.PerEngine) < 2 {
			continue
		}

		// Compute standard deviation of normalized scores.
		var sum, sumSq float64
		n := float64(len(r.PerEngine))
		for _, ns := range r.PerEngine {
			sum += ns.Normalized
			sumSq += ns.Normalized * ns.Normalized
		}
		mean := sum / n
		variance := sumSq/n - mean*mean
		if variance < 0 {
			variance = 0 // numerical guard
		}
		stddev := math.Sqrt(variance)

		// Check if any engine's score is more than threshold stddevs from mean.
		for _, ns := range r.PerEngine {
			if stddev > 0 && math.Abs(ns.Normalized-mean)/stddev > ConsensusDisagreementThreshold {
				flagged = append(flagged, r.CompoundID)
				break
			}
		}
	}

	return flagged
}

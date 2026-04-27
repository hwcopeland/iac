// Package main provides the consensus pocket ranker for WP-1 target preparation.
// Given pocket predictions from fpocket and P2Rank, this module normalizes their
// scores to [0,1], computes a consensus score as the mean of both, and returns a
// ranked list sorted by consensus score descending.
//
// Design: The ranker handles mismatched pocket sets (one tool may find pockets the
// other doesn't) by assigning 0 for the missing tool's score. Pockets are matched
// by spatial proximity of their centers (within a 5A tolerance).
package main

import (
	"math"
	"sort"
)

// DetectedPocket represents a single detected binding pocket with scores from
// both fpocket and P2Rank, plus a consensus score.
type DetectedPocket struct {
	Rank           int        `json:"rank"`
	Center         [3]float64 `json:"center"`
	Size           [3]float64 `json:"size"`
	FpocketScore   float64    `json:"fpocket_score"`    // raw druggability
	P2RankScore    float64    `json:"p2rank_score"`     // raw probability
	ConsensusScore float64    `json:"consensus_score"`  // normalized average
	Volume         float64    `json:"volume"`
	Residues       []string   `json:"residues"`
}

// pocketMatchTolerance is the maximum distance (Angstroms) between two pocket
// centers for them to be considered the same pocket from different tools.
const pocketMatchTolerance = 5.0

// RankPockets takes pocket predictions from fpocket and P2Rank, normalizes
// their scores, computes a consensus score, and returns a merged, ranked list.
//
// Normalization:
//   - fpocket druggability scores are normalized to [0,1] using min-max scaling
//     across all fpocket results. If all scores are equal, they normalize to 1.0.
//   - P2Rank probabilities are also min-max normalized for consistency.
//
// Matching: Pockets from different tools are matched by spatial proximity of
// their centers. Unmatched pockets receive 0 for the missing tool's score.
//
// The returned slice is sorted by consensus score descending and assigned
// sequential ranks starting from 1.
func RankPockets(fpocketResults, p2rankResults []DetectedPocket) []DetectedPocket {
	if len(fpocketResults) == 0 && len(p2rankResults) == 0 {
		return []DetectedPocket{}
	}

	// Normalize each tool's scores independently to [0,1].
	fpNorm := normalizeFpocketScores(fpocketResults)
	p2Norm := normalizeP2RankScores(p2rankResults)

	// Build merged pocket list by matching spatially close pockets.
	merged := mergePockets(fpNorm, p2Norm)

	// Compute consensus score = mean of both normalized scores.
	for i := range merged {
		merged[i].ConsensusScore = (merged[i].FpocketScore + merged[i].P2RankScore) / 2.0
		// Round to 4 decimal places for clean output.
		merged[i].ConsensusScore = math.Round(merged[i].ConsensusScore*10000) / 10000
	}

	// Sort by consensus score descending.
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].ConsensusScore > merged[j].ConsensusScore
	})

	// Assign sequential ranks.
	for i := range merged {
		merged[i].Rank = i + 1
	}

	return merged
}

// normalizeFpocketScores normalizes fpocket druggability scores to [0,1]
// using min-max scaling. Returns copies with normalized FpocketScore values.
func normalizeFpocketScores(pockets []DetectedPocket) []DetectedPocket {
	if len(pockets) == 0 {
		return nil
	}

	result := make([]DetectedPocket, len(pockets))
	copy(result, pockets)

	minScore := math.Inf(1)
	maxScore := math.Inf(-1)
	for _, p := range result {
		if p.FpocketScore < minScore {
			minScore = p.FpocketScore
		}
		if p.FpocketScore > maxScore {
			maxScore = p.FpocketScore
		}
	}

	scoreRange := maxScore - minScore
	for i := range result {
		if scoreRange == 0 {
			result[i].FpocketScore = 1.0
		} else {
			result[i].FpocketScore = (result[i].FpocketScore - minScore) / scoreRange
		}
	}

	return result
}

// normalizeP2RankScores normalizes P2Rank probabilities to [0,1] using
// min-max scaling. Returns copies with normalized P2RankScore values.
func normalizeP2RankScores(pockets []DetectedPocket) []DetectedPocket {
	if len(pockets) == 0 {
		return nil
	}

	result := make([]DetectedPocket, len(pockets))
	copy(result, pockets)

	minScore := math.Inf(1)
	maxScore := math.Inf(-1)
	for _, p := range result {
		if p.P2RankScore < minScore {
			minScore = p.P2RankScore
		}
		if p.P2RankScore > maxScore {
			maxScore = p.P2RankScore
		}
	}

	scoreRange := maxScore - minScore
	for i := range result {
		if scoreRange == 0 {
			result[i].P2RankScore = 1.0
		} else {
			result[i].P2RankScore = (result[i].P2RankScore - minScore) / scoreRange
		}
	}

	return result
}

// mergePockets matches pockets from fpocket and P2Rank by spatial proximity
// and merges them into a single list. Unmatched pockets get 0 for the
// missing tool's score.
func mergePockets(fpPockets, p2Pockets []DetectedPocket) []DetectedPocket {
	// Track which P2Rank pockets have been matched.
	p2Matched := make([]bool, len(p2Pockets))

	var merged []DetectedPocket

	// For each fpocket result, find the closest P2Rank pocket.
	for _, fp := range fpPockets {
		bestIdx := -1
		bestDist := math.Inf(1)

		for j, p2 := range p2Pockets {
			if p2Matched[j] {
				continue
			}
			d := centerDistance(fp.Center, p2.Center)
			if d < bestDist {
				bestDist = d
				bestIdx = j
			}
		}

		if bestIdx >= 0 && bestDist <= pocketMatchTolerance {
			// Matched — merge scores, prefer fpocket geometry (it computes volume).
			p2Matched[bestIdx] = true
			merged = append(merged, DetectedPocket{
				Center:       fp.Center,
				Size:         fp.Size,
				FpocketScore: fp.FpocketScore,
				P2RankScore:  p2Pockets[bestIdx].P2RankScore,
				Volume:       fp.Volume,
				Residues:     mergeResidues(fp.Residues, p2Pockets[bestIdx].Residues),
			})
		} else {
			// No match — fpocket only, P2Rank score = 0.
			merged = append(merged, DetectedPocket{
				Center:       fp.Center,
				Size:         fp.Size,
				FpocketScore: fp.FpocketScore,
				P2RankScore:  0,
				Volume:       fp.Volume,
				Residues:     fp.Residues,
			})
		}
	}

	// Add unmatched P2Rank pockets (fpocket score = 0).
	for j, p2 := range p2Pockets {
		if p2Matched[j] {
			continue
		}
		merged = append(merged, DetectedPocket{
			Center:       p2.Center,
			Size:         p2.Size,
			FpocketScore: 0,
			P2RankScore:  p2.P2RankScore,
			Volume:       p2.Volume,
			Residues:     p2.Residues,
		})
	}

	return merged
}

// centerDistance computes the Euclidean distance between two 3D centers.
func centerDistance(a, b [3]float64) float64 {
	dx := a[0] - b[0]
	dy := a[1] - b[1]
	dz := a[2] - b[2]
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// mergeResidues combines two residue lists, deduplicating by value.
func mergeResidues(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	var result []string

	for _, r := range a {
		if _, ok := seen[r]; !ok {
			seen[r] = struct{}{}
			result = append(result, r)
		}
	}
	for _, r := range b {
		if _, ok := seen[r]; !ok {
			seen[r] = struct{}{}
			result = append(result, r)
		}
	}

	sort.Strings(result)
	return result
}

// Deterministic, in-package string matching for agency resolution.
//
// THE FIREWALL (CONVENTIONS.md §7): no ML anywhere. Agency name matching is a
// fixed-threshold token-sort + Levenshtein similarity computed by the code in
// this file. There is no learned model, no embedding, no LLM call. The
// threshold is a compile-time constant so the matcher is fully reproducible:
// the same input strings always produce the same score and the same decision.
package main

import (
	"sort"
	"strings"
	"unicode"
)

// fuzzyThreshold is the FIXED similarity score (0..1) at or above which a
// state-scoped fuzzy match is accepted as confidence='fuzzy'. Below it, the
// camera is left 'unresolved'. This is a constant by design — never tuned by
// data, never learned.
const fuzzyThreshold = 0.88

// normalize lower-cases, strips punctuation, collapses whitespace, and removes
// generic governmental boilerplate words so that "City of Springfield Police
// Department" and "Springfield PD" normalize toward a comparable core. This is
// a deterministic transform — purely mechanical, no inference.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	// Replace any non-alphanumeric rune with a space.
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	s = b.String()

	// Expand a few common abbreviations to their canonical token so token-sort
	// alignment is stable. Deterministic, fixed table.
	repl := strings.NewReplacer(
		" pd ", " police department ",
		" so ", " sheriffs office ",
		" sd ", " sheriffs department ",
		" dept ", " department ",
		" co ", " county ",
		" twp ", " township ",
		" vil ", " village ",
		" bor ", " borough ",
	)
	s = repl.Replace(" " + s + " ")

	return strings.TrimSpace(s)
}

// stopwords are generic tokens dropped before token-sort comparison so that the
// distinctive part of an agency name dominates the score.
var stopwords = map[string]bool{
	"of": true, "the": true, "and": true, "city": true,
	"town": true, "village": true, "borough": true, "township": true,
}

// tokenSortKey normalizes, drops stopwords, sorts the remaining tokens, and
// joins them. "Springfield Police Department" and "Department Police
// Springfield" produce the same key. Deterministic.
func tokenSortKey(s string) string {
	fields := strings.Fields(normalize(s))
	kept := fields[:0]
	for _, f := range fields {
		if !stopwords[f] {
			kept = append(kept, f)
		}
	}
	sort.Strings(kept)
	return strings.Join(kept, " ")
}

// levenshtein computes the classic edit distance between two strings. Pure,
// O(len(a)*len(b)) dynamic programming. No dependency.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// similarity returns a deterministic 0..1 score combining token-sort alignment
// with normalized Levenshtein distance. 1.0 means identical token-sort keys.
func similarity(a, b string) float64 {
	ka, kb := tokenSortKey(a), tokenSortKey(b)
	if ka == "" || kb == "" {
		return 0
	}
	if ka == kb {
		return 1.0
	}
	dist := levenshtein(ka, kb)
	maxLen := len(ka)
	if len(kb) > maxLen {
		maxLen = len(kb)
	}
	if maxLen == 0 {
		return 0
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// bestMatch scans candidates and returns the index of the highest-similarity
// candidate and its score. Ties resolve to the earliest index (stable), so the
// result is fully deterministic for a fixed candidate ordering. Returns
// (-1, 0) when candidates is empty.
func bestMatch(query string, candidates []string) (int, float64) {
	best := -1
	bestScore := 0.0
	for i, c := range candidates {
		s := similarity(query, c)
		if s > bestScore {
			bestScore = s
			best = i
		}
	}
	return best, bestScore
}

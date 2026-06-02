package main

import "testing"

// TestSimilarityDeterministic asserts the matcher is reproducible and ordering-
// invariant under token reordering — the property that lets us call it
// deterministic (no ML, no learned weights).
func TestSimilarityDeterministic(t *testing.T) {
	a := "Springfield Police Department"
	b := "Department of Police, Springfield"
	s1 := similarity(a, b)
	s2 := similarity(a, b)
	if s1 != s2 {
		t.Fatalf("similarity not deterministic: %v vs %v", s1, s2)
	}
	if s1 < fuzzyThreshold {
		t.Fatalf("token-sort should align reordered tokens, got %v < %v", s1, fuzzyThreshold)
	}
}

func TestSimilarityIdentical(t *testing.T) {
	if got := similarity("Metro PD", "Metro Police Department"); got < fuzzyThreshold {
		t.Fatalf("abbrev expansion should match, got %v", got)
	}
}

func TestSimilarityDistinct(t *testing.T) {
	if got := similarity("Springfield Police Department", "Gotham City Sheriffs Office"); got >= fuzzyThreshold {
		t.Fatalf("distinct agencies should not match, got %v", got)
	}
}

func TestBestMatchStable(t *testing.T) {
	cands := []string{"Aurora Police", "Boulder Police", "Aurora Police Department"}
	idx, score := bestMatch("Aurora Police Dept", cands)
	if idx < 0 || score < fuzzyThreshold {
		t.Fatalf("expected a match >= threshold, got idx=%d score=%v", idx, score)
	}
	// Re-run: must be identical (deterministic, stable tie-break).
	idx2, score2 := bestMatch("Aurora Police Dept", cands)
	if idx != idx2 || score != score2 {
		t.Fatalf("bestMatch not stable: (%d,%v) vs (%d,%v)", idx, score, idx2, score2)
	}
}

func TestClassifyOperatorType(t *testing.T) {
	cases := map[string]string{
		"Springfield Police Department": "police",
		"Cook County Sheriff's Office":  "sheriff",
		"Texas Highway Patrol":          "state_police",
		"Lakeside HOA":                  "private_hoa",
		"County Public Works":           "other_government",
	}
	for in, want := range cases {
		if got := classifyOperatorType(in); got != want {
			t.Errorf("classifyOperatorType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	if d := levenshtein("kitten", "sitting"); d != 3 {
		t.Fatalf("levenshtein(kitten,sitting) = %d, want 3", d)
	}
	if d := levenshtein("", "abc"); d != 3 {
		t.Fatalf("levenshtein empty = %d, want 3", d)
	}
}

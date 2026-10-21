package domain

import (
	"slices"
	"testing"
)

// Faces are scored against the mean of the most common recognizer's vectors; a lone face and a vector of another
// recognizer get no score.
func TestFaceAppearanceSimilarities(t *testing.T) {
	got := FaceAppearanceSimilarities(map[int64][]float32{1: {1, 0}, 2: {1, 0}, 3: {0, 1}, 4: {1, 0, 0}})
	if len(got) != 3 || got[1] <= got[3] || got[1] != got[2] {
		t.Errorf("scores = %v, want 1 and 2 alike above 3, and none for the other recognizer", got)
	}
	if lone := FaceAppearanceSimilarities(map[int64][]float32{1: {1, 0}}); len(lone) != 0 {
		t.Errorf("lone face scores = %v, want none", lone)
	}
}

// Equal scores and unscored faces fall back to the newest video, then the higher id, so every order is total.
func TestComparePersonAppearancesBreaksTies(t *testing.T) {
	score := func(v float64) *float64 { return &v }
	items := []PersonAppearance{
		{ID: 1, AddedAt: 10, Similarity: score(0.5)},
		{ID: 2, AddedAt: 20, Similarity: score(0.5)},
		{ID: 3, AddedAt: 20, Similarity: score(0.5)},
		{ID: 4, AddedAt: 30},
		{ID: 5, AddedAt: 5, Similarity: score(0.9)},
	}
	for _, tc := range []struct {
		sort PersonAppearanceSort
		want []int64
	}{
		{PersonAppearancesNewest, []int64{4, 3, 2, 1, 5}},
		{PersonAppearancesLeastSimilar, []int64{3, 2, 1, 5, 4}},
	} {
		sorted := slices.Clone(items)
		slices.SortFunc(sorted, ComparePersonAppearances(tc.sort))
		got := make([]int64, 0, len(sorted))
		for _, item := range sorted {
			got = append(got, item.ID)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s order = %v, want %v", tc.sort, got, tc.want)
		}
	}
}

// The candidate threshold follows the recognizer that makes vectors of the length; no recognizer makes others.
func TestFaceCandidateThreshold(t *testing.T) {
	for _, tc := range []struct {
		dimensions int
		want       float64
		known      bool
	}{{128, 0.363, true}, {512, 0.205, true}, {3, 0, false}} {
		if got, known := FaceCandidateThreshold(tc.dimensions); got != tc.want || known != tc.known {
			t.Errorf("threshold of %d dimensions = %v, %v; want %v, %v", tc.dimensions, got, known, tc.want, tc.known)
		}
	}
}

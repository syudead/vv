package domain

import (
	"math"
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
		{PersonAppearancesMostSimilar, []int64{5, 3, 2, 1, 4}},
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

// The mean stands for the vectors of the most common length, normalized; there is none without vectors.
func TestMeanFaceVector(t *testing.T) {
	got := MeanFaceVector([][]float32{{1, 0}, {0, 1}, {1, 0, 0}})
	if len(got) != 2 || math.Abs(float64(got[0])-math.Sqrt2/2) > 1e-6 || got[0] != got[1] {
		t.Errorf("mean = %v, want the normalized mean of the two 2-vectors", got)
	}
	if got := MeanFaceVector(nil); got != nil {
		t.Errorf("mean of nothing = %v, want nil", got)
	}
}

// Each person ranks by their closest reference of the query's length, most similar first, ties by id, up to limit.
func TestRankFacePeople(t *testing.T) {
	query := []float32{1, 0}
	got := RankFacePeople(query, map[int64][][]float32{
		1: {{0, 1}, {1, 0}},
		2: {{0.6, 0.8}},
		3: {{1, 0}},
		4: {{1, 0, 0}},
	}, 2)
	want := []FaceCandidate{{PersonID: 1, Similarity: 1}, {PersonID: 3, Similarity: 1}}
	if !slices.Equal(got, want) {
		t.Errorf("ranking = %v, want %v", got, want)
	}
}

// The focus is the middle of the longest track, the first of equal ones, at its centre and width; none without tracks.
func TestFaceFocusOf(t *testing.T) {
	got := FaceFocusOf([]FaceSpan{
		{Start: 0, End: 4, X: 0.1, Y: 0.1, W: 0.1},
		{Start: 10, End: 20, X: 0.5, Y: 0.3, W: 0.2},
		{Start: 30, End: 40, X: 0.9, Y: 0.9, W: 0.3},
	})
	if want := (FaceFocus{Time: 15, X: 0.5, Y: 0.3, W: 0.2}); got == nil || *got != want {
		t.Errorf("focus = %+v, want %+v", got, want)
	}
	if got := FaceFocusOf(nil); got != nil {
		t.Errorf("focus without tracks = %+v, want nil", got)
	}
}

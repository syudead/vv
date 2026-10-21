package domain

import "testing"

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

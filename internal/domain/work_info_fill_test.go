package domain

import "testing"

func TestMatchWorkInfoCandidateTakesOnlyAnEqualOrSoleCandidate(t *testing.T) {
	other := WorkInfoCandidate{ID: "x-9", Title: "Another work"}
	byTitle := WorkInfoCandidate{ID: "w-2", Title: "ＴＨＥ Work: Part 1"}
	byID := WorkInfoCandidate{ID: "ABC-123", Title: "Unrelated title"}
	for _, tc := range []struct {
		name       string
		candidates []WorkInfoCandidate
		query      string
		want       int
		ok         bool
	}{
		{"title ignoring case, width and punctuation", []WorkInfoCandidate{other, byTitle}, "the work part 1", 1, true},
		{"id", []WorkInfoCandidate{other, byID}, "abc123", 1, true},
		{"sole candidate", []WorkInfoCandidate{other}, "something else", 0, true},
		{"several without an equal one", []WorkInfoCandidate{other, byID}, "the work", 0, false},
		{"none", nil, "the work", 0, false},
	} {
		got, ok := MatchWorkInfoCandidate(tc.candidates, tc.query)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: got %d, %v; want %d, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

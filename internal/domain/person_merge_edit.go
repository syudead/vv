package domain

import (
	"cmp"
	"fmt"
	"slices"
)

// Limits of the merges one bulk edit carries: a target per performer the edit can add, and a source per video it
// can select.
const (
	PersonMergesMaxCount       = 100
	PersonMergeSourcesMaxCount = 20000
)

// PersonMerge merges provisional people into one person.
type PersonMerge struct {
	Into      int64
	SourceIDs []int64
}

// PersonMerged is one merge a bulk edit applied; UndoActivityID is 0 when the merge cannot be undone.
type PersonMerged struct {
	PersonID, Into int64
	UndoActivityID int64
}

// ValidatePersonMerges checks the counts and that every id names one role: each into once, each source once and
// never an into.
func ValidatePersonMerges(merges []PersonMerge) error {
	if len(merges) > PersonMergesMaxCount {
		return invalidWorkInfo("merges", WorkInfoTooMany, PersonMergesMaxCount)
	}
	into := make(map[int64]bool, len(merges))
	for i, merge := range merges {
		if into[merge.Into] {
			return invalidWorkInfo(fmt.Sprintf("merges[%d].into", i), WorkInfoDuplicate, 0)
		}
		into[merge.Into] = true
	}
	seen := map[int64]bool{}
	for i, merge := range merges {
		field := fmt.Sprintf("merges[%d].sourceIds", i)
		switch {
		case len(merge.SourceIDs) == 0:
			return invalidWorkInfo(field, WorkInfoEmpty, 0)
		case len(merge.SourceIDs) > PersonMergeSourcesMaxCount:
			return invalidWorkInfo(field, WorkInfoTooMany, PersonMergeSourcesMaxCount)
		}
		for j, id := range merge.SourceIDs {
			if seen[id] || into[id] {
				return invalidWorkInfo(fmt.Sprintf("%s[%d]", field, j), WorkInfoDuplicate, 0)
			}
			seen[id] = true
		}
	}
	return nil
}

// ProvisionalMatch tells how one provisional performer of a selection relates to the people being added.
type ProvisionalMatch struct {
	PersonID int64
	// Alone is true when the person is the only provisional performer of at least one selected video.
	Alone        bool
	CoPerformers []int64
	// SimilarTo lists the added people whose faces reach the merge threshold, most similar first.
	SimilarTo []int64
}

// ProvisionalPerformers groups the provisional performers of each video into matches, without similarity.
func ProvisionalPerformers(byVideo [][]int64) []ProvisionalMatch {
	alone := map[int64]bool{}
	co := map[int64]map[int64]bool{}
	for _, ids := range byVideo {
		for _, id := range ids {
			if co[id] == nil {
				co[id] = map[int64]bool{}
			}
			if len(ids) == 1 {
				alone[id] = true
			}
			for _, other := range ids {
				if other != id {
					co[id][other] = true
				}
			}
		}
	}
	out := make([]ProvisionalMatch, 0, len(co))
	for id, others := range co {
		m := ProvisionalMatch{PersonID: id, Alone: alone[id], CoPerformers: []int64{}, SimilarTo: []int64{}}
		for other := range others {
			m.CoPerformers = append(m.CoPerformers, other)
		}
		slices.Sort(m.CoPerformers)
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b ProvisionalMatch) int { return cmp.Compare(a.PersonID, b.PersonID) })
	return out
}

// FaceMergeThreshold is the Merge threshold of the recognizer whose vectors have the given length, and false for a
// length no recognizer makes.
func FaceMergeThreshold(dimensions int) (float64, bool) {
	for _, spec := range faceRecognizers {
		if spec.Dimensions == dimensions {
			return spec.Thresholds.Merge, true
		}
	}
	return 0, false
}

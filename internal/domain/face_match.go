package domain

import (
	"cmp"
	"errors"
	"slices"
)

// FacePersonVector is the vector of a person's face image, the one face a person is matched by.
type FacePersonVector struct {
	PersonID int64
	Vector   []float32
}

// FaceCandidate is a person close enough to offer to the owner, but not to link or merge.
type FaceCandidate struct {
	PersonID   int64
	Similarity float64
}

// FaceMatchKind is what a main person's vector does among the stored people.
type FaceMatchKind int

const (
	// FaceMatchNew creates a provisional person, which Candidates are then offered with.
	FaceMatchNew FaceMatchKind = iota
	// FaceMatchLink links the appearance to PersonID.
	FaceMatchLink
)

// FaceMatch is the result of comparing one vector with every stored person.
type FaceMatch struct {
	Kind       FaceMatchKind
	PersonID   int64
	Similarity float64
	// Candidates are, for a new person, every person at or above Candidate; for a link, the other people at or above
	// Link, who are likely the linked person stored twice. Most similar first.
	Candidates []FaceCandidate
}

// MatchFace links v to the most similar person at or above Link, offering the others at or above Link as that
// person's candidates, or creates a provisional person offered with every person at or above Candidate.
func MatchFace(th FaceThresholds, v []float32, vectors []FacePersonVector) FaceMatch {
	best := bestFaceSimilarities(v, vectors)
	if len(best) > 0 && best[0].Similarity >= th.Link {
		return FaceMatch{Kind: FaceMatchLink, PersonID: best[0].PersonID, Similarity: best[0].Similarity,
			Candidates: atLeast(best[1:], th.Link)}
	}
	return FaceMatch{Kind: FaceMatchNew, Candidates: atLeast(best, th.Candidate)}
}

// FaceSiblingPair is two provisional people created by one analysis whose faces may be one person.
type FaceSiblingPair struct {
	ProvisionalID, PersonID int64
	Similarity              float64
}

// SiblingFacePairs returns the pairs of the people whose vectors reach Candidate; the later person in the input
// is offered with the earlier one.
func SiblingFacePairs(th FaceThresholds, people []FacePersonVector) []FaceSiblingPair {
	var out []FaceSiblingPair
	for i, a := range people {
		for _, b := range people[:i] {
			if s := FaceSimilarity(a.Vector, b.Vector); s >= th.Candidate {
				out = append(out, FaceSiblingPair{ProvisionalID: a.PersonID, PersonID: b.PersonID, Similarity: s})
			}
		}
	}
	return out
}

// bestFaceSimilarities returns each person's highest similarity to v, most similar first.
func bestFaceSimilarities(v []float32, vectors []FacePersonVector) []FaceCandidate {
	byPerson := map[int64]float64{}
	for _, pv := range vectors {
		if pv.PersonID == 0 || len(pv.Vector) != len(v) {
			continue
		}
		s := FaceSimilarity(v, pv.Vector)
		if old, ok := byPerson[pv.PersonID]; !ok || s > old {
			byPerson[pv.PersonID] = s
		}
	}
	out := make([]FaceCandidate, 0, len(byPerson))
	for id, s := range byPerson {
		out = append(out, FaceCandidate{PersonID: id, Similarity: s})
	}
	slices.SortFunc(out, func(a, b FaceCandidate) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		return cmp.Compare(a.PersonID, b.PersonID)
	})
	return out
}

func atLeast(sorted []FaceCandidate, floor float64) []FaceCandidate {
	n := 0
	for n < len(sorted) && sorted[n].Similarity >= floor {
		n++
	}
	if n == 0 {
		return nil
	}
	return sorted[:n]
}

// StoredFaceAppearance is an appearance of a video as stored before a re-analysis.
type StoredFaceAppearance struct {
	ID       int64
	PersonID int64
	Dropped  bool
	Spans    []FaceSpan
}

// FaceAppearancePlan is what a re-analysis does with one new main person.
type FaceAppearancePlan struct {
	// CarriedFrom is the stored appearance this one replaces, keeping its id, person and dropped mark; 0 when new.
	CarriedFrom int64
	// Match is the vector match of a new appearance; unset when carried over.
	Match FaceMatch
}

// PlanFaceAppearances carries stored appearances over to the new main people of the same video and matches
// the rest by vector. A carried-over appearance keeps its person and is not matched again, so a dropped one is
// neither linked nor made a new provisional person.
func PlanFaceAppearances(th FaceThresholds, stored []StoredFaceAppearance, people []MainPerson, vectors []FacePersonVector) []FaceAppearancePlan {
	spans := make([][]FaceSpan, len(people))
	for i, p := range people {
		spans[i] = p.Spans()
	}
	carried := CarryOverFaceAppearances(stored, spans)
	plans := make([]FaceAppearancePlan, len(people))
	for i, p := range people {
		if carried[i] != 0 {
			plans[i].CarriedFrom = carried[i]
			continue
		}
		plans[i].Match = MatchFace(th, p.Centroid, vectors)
	}
	return plans
}

// CarryOverFaceAppearances returns, for each new appearance, the id of the stored appearance it carries over or
// 0. A stored appearance goes to the one new appearance holding more than half of its matched track seconds,
// and only when no other stored appearance goes to the same new one.
func CarryOverFaceAppearances(stored []StoredFaceAppearance, fresh [][]FaceSpan) []int64 {
	claims := make([][]int, len(fresh))
	for oi, old := range stored {
		if ni := majorityAppearance(old.Spans, fresh); ni >= 0 {
			claims[ni] = append(claims[ni], oi)
		}
	}
	out := make([]int64, len(fresh))
	for ni, c := range claims {
		if len(c) == 1 {
			out[ni] = stored[c[0]].ID
		}
	}
	return out
}

func majorityAppearance(old []FaceSpan, fresh [][]FaceSpan) int {
	seconds := make([]float64, len(fresh))
	var total float64
	for ni, spans := range fresh {
		for _, o := range old {
			for _, n := range spans {
				s := matchedSeconds(o, n)
				seconds[ni] += s
				total += s
			}
		}
	}
	for ni, s := range seconds {
		if total > 0 && s > total/2 {
			return ni
		}
	}
	return -1
}

// matchedSeconds is the time two tracks share when their centres lie no farther apart than the smaller of their
// widths; each sampled frame stands for 1/FaceSampleFPS seconds.
func matchedSeconds(a, b FaceSpan) float64 {
	start, end := max(a.Start, b.Start), min(a.End, b.End)
	if start > end {
		return 0
	}
	dx, dy, limit := a.X-b.X, a.Y-b.Y, min(a.W, b.W)
	if dx*dx+dy*dy > limit*limit {
		return 0
	}
	return end - start + 1/FaceSampleFPS
}

// FaceImageDecision is what a person's new face image does to provisional people.
type FaceImageDecision struct {
	// MergeID is the provisional person to merge into the person; 0 for none.
	MergeID int64
	// Candidates are the other provisional people at or above the candidate threshold, most similar first.
	Candidates []FaceCandidate
}

// DecideFaceImageMerge merges the provisional person whose vectors reach Merge when exactly one does and the
// face belongs to a named person; otherwise those people are only candidates. provisional holds the vectors of
// provisional people; the person's own are ignored.
func DecideFaceImageMerge(th FaceThresholds, personID int64, personProvisional bool, face []float32, provisional []FacePersonVector) FaceImageDecision {
	var others []FacePersonVector
	for _, pv := range provisional {
		if pv.PersonID != personID {
			others = append(others, pv)
		}
	}
	near := atLeast(bestFaceSimilarities(face, others), th.Candidate)
	merging := atLeast(near, th.Merge)
	if personProvisional || len(merging) != 1 {
		return FaceImageDecision{Candidates: near}
	}
	return FaceImageDecision{MergeID: merging[0].PersonID, Candidates: near[1:]}
}

// FacePersonPair is a provisional person and the person it may be merged into.
type FacePersonPair struct {
	ProvisionalID int64
	PersonID      int64
}

// FaceCastPerformer is a performer of a video.
type FaceCastPerformer struct {
	PersonID    int64
	Provisional bool
	// FaceLinked is true when one of the video's appearances that is not dropped is linked to this person.
	FaceLinked bool
}

// DecideCastMerge merges the video's only provisional performer linked by face into its only performer without
// a face, when that performer is named and the owner has not dismissed the pair.
func DecideCastMerge(performers []FaceCastPerformer, dismissed map[FacePersonPair]bool) (FacePersonPair, bool) {
	var faceless, linked []FaceCastPerformer
	for _, p := range performers {
		switch {
		case !p.FaceLinked:
			faceless = append(faceless, p)
		case p.Provisional:
			linked = append(linked, p)
		}
	}
	if len(faceless) != 1 || len(linked) != 1 || faceless[0].Provisional {
		return FacePersonPair{}, false
	}
	pair := FacePersonPair{ProvisionalID: linked[0].PersonID, PersonID: faceless[0].PersonID}
	if dismissed[pair] {
		return FacePersonPair{}, false
	}
	return pair, true
}

// ErrFaceCandidateNotFound reports that two people have no open (not dismissed) candidate pair.
var ErrFaceCandidateNotFound = errors.New("no open face candidate pair of these people")

// ErrFaceAppearanceNotFound reports that the person has no appearance with the id that is still linked to them.
var ErrFaceAppearanceNotFound = errors.New("no face appearance of this person")

// FacePair is an open candidate pair seen from one of its people: the other person and their similarity.
type FacePair struct {
	Person CreditRef
	// ImageRevision is zero unless the other person has a face.
	ImageRevision int64
	Similarity    float64
}

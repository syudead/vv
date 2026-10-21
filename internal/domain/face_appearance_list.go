package domain

import (
	"cmp"
	"slices"
)

// PersonAppearance is one not-dropped appearance of a person, as the person page lists it.
type PersonAppearance struct {
	ID int64
	// VideoID is zero when no video has the appearance's content key any more.
	VideoID int64
	Title   string
	Spans   []FaceTimeSpan
	HasCrop bool
	// AddedAt is the video's added time, or -1 without a video, so such appearances come last.
	AddedAt int64
	// Similarity is how close the appearance is to the person's other faces, nil when it cannot be compared.
	Similarity *float64
	// Unlike is true when Similarity is below its recognizer's candidate threshold, as unlike as a stranger usually is.
	Unlike bool
}

// PersonAppearanceQuery selects a page of a person's appearances; a nonpositive Limit returns every remaining one.
type PersonAppearanceQuery struct {
	Sort   PersonAppearanceSort
	Cursor string
	Limit  int
}

// PersonAppearanceSort orders a person's appearances.
type PersonAppearanceSort string

const (
	// PersonAppearancesNewest orders by the video's added time, newest first.
	PersonAppearancesNewest PersonAppearanceSort = "newest"
	// PersonAppearancesLeastSimilar puts the faces least like the person first, so a wrong link is found early.
	PersonAppearancesLeastSimilar PersonAppearanceSort = "leastSimilar"
)

// ComparePersonAppearances is the order of sort; ties and unscored appearances fall back to newest first, then by
// descending id, so every order is total and a cursor can resume after any appearance.
func ComparePersonAppearances(sort PersonAppearanceSort) func(a, b PersonAppearance) int {
	newest := func(a, b PersonAppearance) int {
		if c := cmp.Compare(b.AddedAt, a.AddedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	}
	if sort != PersonAppearancesLeastSimilar {
		return newest
	}
	return func(a, b PersonAppearance) int {
		switch {
		case a.Similarity != nil && b.Similarity != nil:
			if c := cmp.Compare(*a.Similarity, *b.Similarity); c != 0 {
				return c
			}
		case a.Similarity != nil:
			return -1
		case b.Similarity != nil:
			return 1
		}
		return newest(a, b)
	}
}

// MeanFaceVector is the normalized mean of the vectors of the most common length, which stands for the person or
// the faces they come from; vectors of another length, from another recognizer, are left out. It is nil without any.
func MeanFaceVector(vectors [][]float32) []float32 {
	counts := map[int]int{}
	for _, v := range vectors {
		counts[len(v)]++
	}
	size, best := 0, 0
	for n, c := range counts {
		if n > 0 && (c > best || c == best && n > size) {
			size, best = n, c
		}
	}
	if size == 0 {
		return nil
	}
	mean := make([]float32, size)
	for _, v := range vectors {
		if len(v) != size {
			continue
		}
		for i, x := range v {
			mean[i] += x
		}
	}
	return NormalizeFaceVector(mean)
}

// FaceAppearanceSimilarities scores each vector by its similarity to MeanFaceVector of them all; a vector of another
// length than the mean gets no score.
func FaceAppearanceSimilarities(vectors map[int64][]float32) map[int64]float64 {
	all := make([][]float32, 0, len(vectors))
	for _, v := range vectors {
		all = append(all, v)
	}
	mean := MeanFaceVector(all)
	out := make(map[int64]float64, len(vectors))
	for id, v := range vectors {
		if mean != nil && len(v) == len(mean) {
			out[id] = FaceSimilarity(v, mean)
		}
	}
	return out
}

// RankFacePeople orders people by how close query is to the nearest of each one's reference vectors, such as their
// face image's vector and the mean of their faces in videos, most similar first and then by id, and keeps limit.
func RankFacePeople(query []float32, references map[int64][][]float32, limit int) []FaceCandidate {
	out := make([]FaceCandidate, 0, len(references))
	for personID, refs := range references {
		best, found := 0.0, false
		for _, ref := range refs {
			if len(ref) == len(query) && len(ref) > 0 {
				if s := FaceSimilarity(query, ref); !found || s > best {
					best, found = s, true
				}
			}
		}
		if found {
			out = append(out, FaceCandidate{PersonID: personID, Similarity: best})
		}
	}
	slices.SortFunc(out, func(a, b FaceCandidate) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		return cmp.Compare(a.PersonID, b.PersonID)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SimilarPerson is a person whose faces look like a set of faces, offered as where those faces belong.
type SimilarPerson struct {
	Person     Credit
	Similarity float64
	// FaceCount counts the person's appearances that are not dropped.
	FaceCount int
	// SampleAppearanceIDs are a few of their appearances with a crop, latest first.
	SampleAppearanceIDs []int64
	// FirstSeenTitle is the title of the video of their first appearance, which tells unnamed people apart.
	FirstSeenTitle string
}

// PersonAppearancePage is one page of a person's appearances; NextCursor is empty on the last page.
type PersonAppearancePage struct {
	Items      []PersonAppearance
	NextCursor string
	// Total counts every appearance of the person that is not dropped, across all pages.
	Total int
}

// FaceTimeSpan is a time range in seconds.
type FaceTimeSpan struct {
	Start, End float64
}

// FaceTimeSpans returns when the spans' face is on screen, ordered by start, with overlapping ranges joined, since
// the tracks of one main person can overlap in time.
func FaceTimeSpans(spans []FaceSpan) []FaceTimeSpan {
	out := make([]FaceTimeSpan, 0, len(spans))
	for _, s := range spans {
		out = append(out, FaceTimeSpan{Start: s.Start, End: s.End})
	}
	slices.SortFunc(out, func(a, b FaceTimeSpan) int { return cmp.Compare(a.Start, b.Start) })
	joined := out[:0]
	for _, s := range out {
		if n := len(joined); n > 0 && s.Start <= joined[n-1].End {
			joined[n-1].End = max(joined[n-1].End, s.End)
			continue
		}
		joined = append(joined, s)
	}
	return joined
}

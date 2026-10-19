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
}

// PersonAppearancePage is one page of a person's appearances; NextCursor is empty on the last page.
type PersonAppearancePage struct {
	Items      []PersonAppearance
	NextCursor string
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

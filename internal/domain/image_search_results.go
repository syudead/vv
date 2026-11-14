package domain

import (
	"cmp"
	"math"
	"math/bits"
	"slices"
)

const (
	// ImageSearchStrongMinPercent splits the rates a search lists into two bands for presentation; it makes no claim
	// about accuracy, and an accuracy decision may move it.
	ImageSearchStrongMinPercent = 90
	// ImageSearchMaxMatchFrames bounds the frames one result carries; FrameCount keeps the number found.
	ImageSearchMaxMatchFrames = 12
	// imageSearchVideoMinPercent is the share of matching entries a video query lists from.
	imageSearchVideoMinPercent = 50
)

type ImageSearchBandID string

const (
	ImageSearchBandStrong  ImageSearchBandID = "strong"
	ImageSearchBandSimilar ImageSearchBandID = "similar"
)

// ImageSearchBand holds the whole-percent rates from MinPercent to MaxPercent, both included.
type ImageSearchBand struct {
	ID         ImageSearchBandID
	MinPercent int
	MaxPercent int
}

// ImageSearchMatchedFrame is one matched frame on the matched video's timeline.
type ImageSearchMatchedFrame struct {
	PositionMs int64
	Rate       float64
}

// ImageSearchRatePercent rounds a match rate to the nearest whole percent, halves up, on the same float product the
// screen rounds to show the rate.
func ImageSearchRatePercent(rate float64) int {
	return int(math.Floor(rate*100 + 0.5))
}

// ImageSearchBands lists every band of the query's kind, strongest first; the lowest band starts at the kind's
// listing floor, so no listed rate falls outside a band.
func ImageSearchBands(kind ImageSearchQueryKind) []ImageSearchBand {
	floor := ImageSearchRatePercent(ImageMatchRate(FingerprintMatchMaxDistance))
	if kind == ImageSearchQueryVideo {
		floor = imageSearchVideoMinPercent
	}
	return []ImageSearchBand{
		{ID: ImageSearchBandStrong, MinPercent: ImageSearchStrongMinPercent, MaxPercent: 100},
		{ID: ImageSearchBandSimilar, MinPercent: floor, MaxPercent: ImageSearchStrongMinPercent - 1},
	}
}

// ImageSearchBandOf decides a match's band on its rounded rate, so a rate shown as 90% is never under the lower band.
func ImageSearchBandOf(rate float64) ImageSearchBandID {
	if ImageSearchRatePercent(rate) >= ImageSearchStrongMinPercent {
		return ImageSearchBandStrong
	}
	return ImageSearchBandSimilar
}

// ImageMatchFrames returns the matched frames of an image query's match in content, the match's own video, best
// first and at most ImageSearchMaxMatchFrames, with the number found. Each run of consecutive entries within
// FingerprintMatchMaxDistance gives its closest entry, the earliest on a tie, so a held shot is one frame. A refined
// match, whose content has no such entry, has its one refined frame; a video query's match has none.
func ImageMatchFrames(query uint64, content ImageSearchContent, match ImageSearchMatch) ([]ImageSearchMatchedFrame, int) {
	if match.Kind != ImageSearchMatchFrame {
		return []ImageSearchMatchedFrame{}, 0
	}
	var frames []ImageSearchMatchedFrame
	best, bestDistance := -1, 0
	closeRun := func() {
		if best >= 0 {
			frames = append(frames, ImageSearchMatchedFrame{
				PositionMs: content.Frames[best].TimeMs, Rate: ImageMatchRate(bestDistance),
			})
		}
		best = -1
	}
	for i, frame := range content.Frames {
		distance := bits.OnesCount64(query ^ frame.Hash)
		switch {
		case distance > FingerprintMatchMaxDistance:
			closeRun()
		case best < 0 || distance < bestDistance:
			best, bestDistance = i, distance
		}
	}
	closeRun()
	if len(frames) == 0 {
		return []ImageSearchMatchedFrame{{PositionMs: match.PositionMs, Rate: match.Rate}}, 1
	}
	slices.SortStableFunc(frames, func(a, b ImageSearchMatchedFrame) int {
		if c := cmp.Compare(b.Rate, a.Rate); c != 0 {
			return c
		}
		return cmp.Compare(a.PositionMs, b.PositionMs)
	})
	return frames[:min(len(frames), ImageSearchMaxMatchFrames)], len(frames)
}

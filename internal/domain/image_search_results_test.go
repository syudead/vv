package domain

import (
	"slices"
	"testing"
)

// matchRun returns entries at the given times, each the given distance from query.
func matchRun(query uint64, timesMs []int64, distances []int) []ImageSearchFrame {
	frames := make([]ImageSearchFrame, len(timesMs))
	for i, t := range timesMs {
		frames[i] = ImageSearchFrame{TimeMs: t, Hash: flipBits(query, distances[i])}
	}
	return frames
}

// Each run of consecutive entries within 12 bits is one frame at its closest entry, the earliest on a tie; frames
// come best first, then earlier.
func TestImageMatchFramesOnePerRun(t *testing.T) {
	query := randomHashes(11, 1)[0]
	content := ImageSearchContent{VideoID: 1, DurationMs: 60_000, Frames: matchRun(query,
		[]int64{1_000, 2_000, 3_000, 4_000, 5_000, 6_000, 7_000, 8_000},
		[]int{9, 4, 4, 30, 12, 13, 4, 2})}
	match := ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchFrame, Rate: ImageMatchRate(2), PositionMs: 8_000}

	frames, count := ImageMatchFrames(query, content, match)

	want := []ImageSearchMatchedFrame{
		{PositionMs: 8_000, Rate: ImageMatchRate(2)},
		{PositionMs: 2_000, Rate: ImageMatchRate(4)},
		{PositionMs: 5_000, Rate: ImageMatchRate(12)},
	}
	if count != 3 || !slices.Equal(frames, want) {
		t.Fatalf("frames = %+v (%d), want %+v (3)", frames, count, want)
	}
}

func TestImageMatchFramesKeepsTheTwelveBest(t *testing.T) {
	query := randomHashes(12, 1)[0]
	var times []int64
	var distances []int
	// 15 runs of one entry each, separated by an unrelated entry; run k is k mod 5 bits away.
	for k := range 15 {
		times = append(times, int64(2*k)*1_000, int64(2*k+1)*1_000)
		distances = append(distances, k%5, 40)
	}
	content := ImageSearchContent{VideoID: 1, DurationMs: 30_000, Frames: matchRun(query, times, distances)}
	match := ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchFrame, Rate: 1, PositionMs: 0}

	frames, count := ImageMatchFrames(query, content, match)

	if count != 15 || len(frames) != ImageSearchMaxMatchFrames {
		t.Fatalf("%d frames of %d, want %d of 15", len(frames), count, ImageSearchMaxMatchFrames)
	}
	// Distances 0 to 3 have three runs each and fill the twelve; the three runs at 4 bits are cut.
	if last := frames[len(frames)-1]; last.Rate != ImageMatchRate(3) || last.PositionMs != 26_000 {
		t.Fatalf("last frame = %+v, want the last run at 3 bits", last)
	}
}

// A refined match has only its decoded frame, and a video query's match has none.
func TestImageMatchFramesOfRefinedAndVideoMatches(t *testing.T) {
	query := randomHashes(13, 1)[0]
	content := ImageSearchContent{VideoID: 1, DurationMs: 20_000, Frames: matchRun(query,
		[]int64{0, 10_000}, []int{15, 30})}
	refined := ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchFrame, Rate: ImageMatchRate(6), PositionMs: 1_250}

	frames, count := ImageMatchFrames(query, content, refined)
	if want := []ImageSearchMatchedFrame{{PositionMs: 1_250, Rate: ImageMatchRate(6)}}; count != 1 ||
		!slices.Equal(frames, want) {
		t.Fatalf("refined: frames = %+v (%d), want %+v (1)", frames, count, want)
	}

	video := ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchWhole, Rate: 1, PositionMs: 0}
	if frames, count := ImageMatchFrames(query, content, video); count != 0 || frames == nil || len(frames) != 0 {
		t.Fatalf("video: frames = %#v (%d), want empty", frames, count)
	}
}

func TestImageSearchBands(t *testing.T) {
	for _, tc := range []struct {
		kind ImageSearchQueryKind
		want []ImageSearchBand
	}{
		{ImageSearchQueryImage, []ImageSearchBand{{ImageSearchBandStrong, 90, 100}, {ImageSearchBandSimilar, 81, 89}}},
		{ImageSearchQueryVideo, []ImageSearchBand{{ImageSearchBandStrong, 90, 100}, {ImageSearchBandSimilar, 50, 89}}},
	} {
		if got := ImageSearchBands(tc.kind); !slices.Equal(got, tc.want) {
			t.Errorf("%s: bands = %+v, want %+v", tc.kind, got, tc.want)
		}
	}
}

// The band follows the rate rounded to a whole percent, halves up.
func TestImageSearchBandOf(t *testing.T) {
	for _, tc := range []struct {
		rate float64
		want ImageSearchBandID
	}{
		{179.0 / 200, ImageSearchBandStrong},    // 89.5% is shown as 90%
		{1789.0 / 2000, ImageSearchBandSimilar}, // 89.45%
		{ImageMatchRate(6), ImageSearchBandStrong},
		{ImageMatchRate(7), ImageSearchBandSimilar},
		{ImageMatchRate(FingerprintMatchMaxDistance), ImageSearchBandSimilar},
		{0.5, ImageSearchBandSimilar},
		{1, ImageSearchBandStrong},
	} {
		if got := ImageSearchBandOf(tc.rate); got != tc.want {
			t.Errorf("rate %v: band = %s, want %s", tc.rate, got, tc.want)
		}
	}
}

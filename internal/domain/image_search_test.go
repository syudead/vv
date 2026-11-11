package domain

import (
	"math/rand/v2"
	"testing"
	"time"
)

// randomHashes returns unrelated hashes; distinct random 64-bit values are about 32 bits apart.
func randomHashes(seed uint64, n int) []uint64 {
	random := rand.New(rand.NewPCG(seed, seed*7+1))
	out := make([]uint64, n)
	for i := range out {
		out[i] = random.Uint64()
	}
	return out
}

// flipBits returns hash at exactly distance bits from h.
func flipBits(h uint64, distance int) uint64 {
	return h ^ (1<<uint(distance) - 1)
}

func secondQuery(hashes []uint64) ImageSearchVideoQuery {
	frames := make([]FrameHash, len(hashes))
	for i, h := range hashes {
		frames[i] = FrameHash{Hash: h}
	}
	return ImageSearchVideoQuery{DurationMs: int64(len(hashes)) * 1000, IntervalMs: 1000, Frames: frames}
}

// contentAt indexes the scene shown at each second listed, at that second plus shiftMs.
func contentAt(id int64, durationMs int64, scenes []uint64, seconds []int, shiftMs int64) ImageSearchContent {
	frames := make([]ImageSearchFrame, len(seconds))
	for i, s := range seconds {
		frames[i] = ImageSearchFrame{TimeMs: int64(s)*1000 + shiftMs, Hash: scenes[s]}
	}
	return ImageSearchContent{VideoID: id, DurationMs: durationMs, Frames: frames}
}

func every(step, until int) []int {
	var out []int
	for s := 0; s < until; s += step {
		out = append(out, s)
	}
	return out
}

func TestMatchImageQueryListsRefinesOrDrops(t *testing.T) {
	scenes := randomHashes(1, 3)
	query := scenes[1]
	content := func(id int64, distance int) ImageSearchContent {
		return ImageSearchContent{VideoID: id, DurationMs: 40_000, Frames: []ImageSearchFrame{
			{TimeMs: 0, Hash: scenes[0]},
			{TimeMs: 8_000, Hash: flipBits(query, distance)},
			{TimeMs: 30_000, Hash: scenes[2]},
		}}
	}
	matches, refinements := MatchImageQuery(query, []ImageSearchContent{
		content(1, 12), content(2, 13), content(3, 18), content(4, 19),
	})

	if len(matches) != 1 {
		t.Fatalf("一致 %+v、距離 12 の 1 件だけのはず", matches)
	}
	want := ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchFrame, Rate: 51.0 / 63, PositionMs: 8_000}
	if matches[0] != want {
		t.Errorf("一致 = %+v、want %+v", matches[0], want)
	}
	wantRefine := []ImageSearchRefinement{
		{VideoID: 2, Distance: 13, StartMs: 8_000, EndMs: 18_000},
		{VideoID: 3, Distance: 18, StartMs: 8_000, EndMs: 18_000},
	}
	if len(refinements) != len(wantRefine) {
		t.Fatalf("絞り込み %+v、want %+v", refinements, wantRefine)
	}
	for i := range wantRefine {
		if refinements[i] != wantRefine[i] {
			t.Errorf("絞り込み[%d] = %+v、want %+v", i, refinements[i], wantRefine[i])
		}
	}
}

func TestMatchImageQueryRefinesTheTenClosest(t *testing.T) {
	query := randomHashes(2, 1)[0]
	var contents []ImageSearchContent
	// Distances 18, 17, ..., 13, then 18 again: the six 18s compete for the last places.
	for i, distance := range []int{18, 17, 16, 15, 14, 13, 18, 18, 18, 18, 18, 18} {
		contents = append(contents, ImageSearchContent{
			VideoID: int64(i + 1), DurationMs: 5_000,
			Frames: []ImageSearchFrame{{TimeMs: 1_000, Hash: flipBits(query, distance)}},
		})
	}
	_, refinements := MatchImageQuery(query, contents)

	if len(refinements) != ImageSearchRefineLimit {
		t.Fatalf("絞り込み %d 件、want %d", len(refinements), ImageSearchRefineLimit)
	}
	for i, wantDistance := range []int{13, 14, 15, 16, 17, 18, 18, 18, 18, 18} {
		if refinements[i].Distance != wantDistance {
			t.Errorf("絞り込み[%d] の距離 %d、want %d", i, refinements[i].Distance, wantDistance)
		}
	}
	// The last entry's gap runs to the video's end.
	if got := refinements[0]; got.StartMs != 1_000 || got.EndMs != 5_000 {
		t.Errorf("最後のエントリの区間 %d〜%d、want 1000〜5000", got.StartMs, got.EndMs)
	}
}

// A refined content is listed at its nearest non-flat frame within 12 bits, the earliest of equals; a gap whose frames
// are all flat or over 12 bits away is not listed.
func TestMatchRefinedFramesListsTheNearestFrame(t *testing.T) {
	query := randomHashes(3, 1)[0]
	refinement := ImageSearchRefinement{VideoID: 7, AddedAt: time.Unix(7, 0), Distance: 15, StartMs: 8_000, EndMs: 16_000}
	frame := func(timeMs int64, distance int, flat bool) KeyframeHash {
		return KeyframeHash{TimeMs: timeMs, Frame: FrameHash{Hash: flipBits(query, distance), Flat: flat}}
	}
	match, ok := MatchRefinedFrames(query, refinement, []KeyframeHash{
		frame(8_000, 0, true), frame(8_250, 13, false), frame(8_500, 5, false), frame(8_750, 5, false),
	})
	want := ImageSearchMatch{VideoID: 7, AddedAt: time.Unix(7, 0), Kind: ImageSearchMatchFrame, Rate: 58.0 / 63,
		PositionMs: 8_500}
	if !ok || match != want {
		t.Errorf("match = %+v, %v, want %+v", match, ok, want)
	}
	if match, ok := MatchRefinedFrames(query, refinement, []KeyframeHash{frame(8_000, 0, true), frame(8_250, 13, false)}); ok {
		t.Errorf("matched %+v with only a flat frame and one 13 bits away", match)
	}
}

func TestMatchVideoQuerySections(t *testing.T) {
	scenes := randomHashes(3, 90)
	longScenes := randomHashes(6, 900)
	for _, tc := range []struct {
		name    string
		query   ImageSearchVideoQuery
		content ImageSearchContent
		want    ImageSearchMatch
	}{
		{
			name:    "キーフレームの時刻だけずれた同じ動画",
			query:   secondQuery(scenes[:60]),
			content: contentAt(1, 60_000, scenes, every(3, 60), 400),
			// The query section is the result section moved by the 400 ms offset, clipped to the query.
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchWhole, Rate: 1, PositionMs: 400,
				StartMs: 0, EndMs: 60_000, QueryStartMs: 0, QueryEndMs: 59_600},
		},
		{
			name:    "8 秒ごとのキーフレームの 30 秒の動画",
			query:   secondQuery(scenes[:30]),
			content: contentAt(1, 30_000, scenes, []int{0, 8, 16, 24}, 0),
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchWhole, Rate: 1, PositionMs: 0,
				StartMs: 0, EndMs: 30_000, QueryStartMs: 0, QueryEndMs: 30_000},
		},
		{
			name:    "問い合わせがインデックスの中央 3 分の 1",
			query:   secondQuery(scenes[30:60]),
			content: contentAt(1, 90_000, scenes, every(3, 90), 0),
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchPartial, Rate: 1, PositionMs: 30_000,
				StartMs: 30_000, EndMs: 60_000, QueryStartMs: 0, QueryEndMs: 30_000},
		},
		{
			name:  "インデックスが長い問い合わせの 3 分の 1",
			query: secondQuery(scenes),
			content: func() ImageSearchContent {
				c := contentAt(1, 30_000, scenes, every(3, 30), 0)
				for i := range c.Frames {
					c.Frames[i].Hash = scenes[30+3*i]
				}
				return c
			}(),
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchPartial, Rate: 1, PositionMs: 0,
				StartMs: 0, EndMs: 30_000, QueryStartMs: 30_000, QueryEndMs: 60_000},
		},
		{
			name:  "種ごとのずれが秒の境界の両側に分かれる",
			query: secondQuery(scenes[:30]),
			// Each seed votes 400 or -400 ms; only the offset between them puts every entry within 0.5 s of its frame.
			content: ImageSearchContent{VideoID: 1, DurationMs: 30_000, Frames: []ImageSearchFrame{
				{TimeMs: 400, Hash: scenes[0]}, {TimeMs: 8_600, Hash: scenes[9]},
				{TimeMs: 16_400, Hash: scenes[16]}, {TimeMs: 24_600, Hash: scenes[25]},
			}},
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchWhole, Rate: 1, PositionMs: 400,
				StartMs: 0, EndMs: 30_000, QueryStartMs: 0, QueryEndMs: 30_000},
		},
		{
			name:  "同じ一致数なら出せるオフセット",
			query: secondQuery(scenes[:3]),
			// Offset 0 compares three entries and 10 s compares two, both with two matching.
			content: ImageSearchContent{VideoID: 1, DurationMs: 12_000, Frames: []ImageSearchFrame{
				{TimeMs: 0, Hash: scenes[0]}, {TimeMs: 1_000, Hash: scenes[1]}, {TimeMs: 2_000, Hash: scenes[3]},
				{TimeMs: 10_000, Hash: scenes[0]}, {TimeMs: 11_000, Hash: scenes[1]},
			}},
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchPartial, Rate: 2.0 / 3, PositionMs: 0,
				StartMs: 0, EndMs: 2_000, QueryStartMs: 0, QueryEndMs: 2_000},
		},
		{
			name:  "一致数の多いオフセットが一致率に届かない",
			query: secondQuery(scenes[:30]),
			// Offset 0 matches 4 of 10 entries; offset 40 s matches all 3 it compares and is the one listed.
			content: func() ImageSearchContent {
				c := contentAt(1, 50_000, scenes, every(3, 30), 0)
				for i := 4; i < len(c.Frames); i++ {
					c.Frames[i].Hash = scenes[60+i]
				}
				for _, s := range []int{0, 3, 6} {
					c.Frames = append(c.Frames, ImageSearchFrame{TimeMs: 40_000 + int64(s)*1000, Hash: scenes[s]})
				}
				return c
			}(),
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchPartial, Rate: 1, PositionMs: 40_000,
				StartMs: 40_000, EndMs: 50_000, QueryStartMs: 0, QueryEndMs: 10_000},
		},
		{
			name:  "キーフレームが 10 秒ごとの種の間に落ちる長い問い合わせ",
			query: secondQuery(longScenes),
			// Keyframes every 2 s at 0.7 s past the second show odd seconds' scenes, which seeds 10 s apart never are.
			content: func() ImageSearchContent {
				c := ImageSearchContent{VideoID: 1, DurationMs: 900_000}
				for at := int64(700); at < 900_000; at += 2000 {
					c.Frames = append(c.Frames, ImageSearchFrame{TimeMs: at, Hash: longScenes[(at+500)/1000]})
				}
				return c
			}(),
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchWhole, Rate: 1, PositionMs: 700,
				StartMs: 0, EndMs: 900_000, QueryStartMs: 300, QueryEndMs: 900_000},
		},
		{
			name:  "3 分の動画が 4 秒ごとのキーフレームで長い問い合わせの途中にある",
			query: secondQuery(longScenes[:640]),
			// The 18 seeds inside it must still put two on its keyframes, whatever the stretch it starts at.
			content: func() ImageSearchContent {
				c := ImageSearchContent{VideoID: 1, DurationMs: 180_000}
				for at := int64(0); at < 180_000; at += 4000 {
					c.Frames = append(c.Frames, ImageSearchFrame{TimeMs: at, Hash: longScenes[238+at/1000]})
				}
				return c
			}(),
			want: ImageSearchMatch{VideoID: 1, Kind: ImageSearchMatchPartial, Rate: 1, PositionMs: 0,
				StartMs: 0, EndMs: 180_000, QueryStartMs: 238_000, QueryEndMs: 418_000},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchVideoQuery(tc.query, []ImageSearchContent{tc.content})
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("一致 = %+v、want [%+v]", got, tc.want)
			}
		})
	}
}

func TestMatchVideoQueryThresholds(t *testing.T) {
	scenes := randomHashes(4, 30)
	others := randomHashes(5, 30)
	// queryMatching keeps the content's scene for the first `matching` of its ten entries, 3 s apart.
	queryMatching := func(matching int) ImageSearchVideoQuery {
		hashes := make([]uint64, 30)
		for s := range hashes {
			hashes[s] = scenes[s]
			if s >= 3*matching {
				hashes[s] = others[s]
			}
		}
		return secondQuery(hashes)
	}
	for _, tc := range []struct {
		name    string
		query   ImageSearchVideoQuery
		content ImageSearchContent
		listed  bool
	}{
		{"比べたエントリが 3", secondQuery(scenes[:9]), contentAt(1, 9_000, scenes, []int{0, 3, 6}, 0), true},
		{"比べたエントリが 2", secondQuery(scenes[:6]), contentAt(1, 6_000, scenes, []int{0, 3}, 0), false},
		{"一致率 50%", queryMatching(5), contentAt(1, 30_000, scenes, every(3, 30), 0), true},
		{"一致率 40%", queryMatching(4), contentAt(1, 30_000, scenes, every(3, 30), 0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchVideoQuery(tc.query, []ImageSearchContent{tc.content})
			if (len(got) == 1) != tc.listed {
				t.Errorf("一致 = %+v、出るべきか %v", got, tc.listed)
			}
		})
	}
}

func TestRankImageSearchMatchesBreaksTies(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	matches := []ImageSearchMatch{
		{VideoID: 1, Rate: 0.8, StartMs: 0, EndMs: 10_000, AddedAt: newer},
		{VideoID: 2, Rate: 0.8, StartMs: 0, EndMs: 20_000, AddedAt: older},
		{VideoID: 3, Rate: 0.8, StartMs: 5_000, EndMs: 15_000, AddedAt: older},
		{VideoID: 4, Rate: 0.9, StartMs: 0, EndMs: 1_000, AddedAt: older},
	}
	got := RankImageSearchMatches(matches)
	var ids []int64
	for _, m := range got {
		ids = append(ids, m.VideoID)
	}
	want := []int64{4, 2, 1, 3}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("並び %v、want %v", ids, want)
		}
	}

	many := make([]ImageSearchMatch, ImageSearchMaxResults+1)
	if n := len(RankImageSearchMatches(many)); n != ImageSearchMaxResults {
		t.Errorf("%d 件、want %d", n, ImageSearchMaxResults)
	}
}

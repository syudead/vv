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

func TestMatchVideoQuerySections(t *testing.T) {
	scenes := randomHashes(3, 90)
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

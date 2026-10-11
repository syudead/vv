package domain

import (
	"errors"
	"slices"
	"testing"
	"time"
)

var relatedBase = time.Unix(1_757_000_000, 0)

func candidate(id int64, path string, tags ...int64) RelatedCandidate {
	return RelatedCandidate{VideoID: id, Path: path, Title: "x", AddedAt: relatedBase, TagIDs: tags}
}

func rankedIDs(ranked []RankedRelated) []int64 {
	ids := make([]int64, 0, len(ranked))
	for _, item := range ranked {
		ids = append(ids, item.VideoID)
	}
	return ids
}

func scoreOf(self, other RelatedCandidate) RelatedScore {
	return DefaultRelatedScorer.For(self)(other)
}

// 受け入れ条件 1・2: タグがすべて同じ別フォルダの動画は同じディレクトリの動画より上、
// 同じディレクトリの動画はタグが一部だけ共通する別フォルダの動画より上に出る。
func TestRankRelatedWeighsTagsAgainstFolder(t *testing.T) {
	self := candidate(1, "/media/show/a.mp4", 10, 11, 12)
	sameTags := candidate(2, "/media/other/b.mp4", 10, 11, 12)
	sameDir := candidate(3, "/media/show/c.mp4")
	halfTags := candidate(4, "/media/else/d.mp4", 10, 11, 13)
	// 新しく追加された方が同点で先になるので、別フォルダの方を新しくしても順は変わらない。
	halfTags.AddedAt = relatedBase.Add(time.Hour)

	got := rankedIDs(RankRelated(DefaultRelatedScorer, self, []RelatedCandidate{halfTags, sameDir, sameTags}))
	if want := []int64{2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// タグの一致率は 2 本を合わせた種類のうち両方にあるものの割合で、どちらかに無ければ 0。
func TestContentRelatednessTagShare(t *testing.T) {
	self := candidate(1, "/a/x.mp4", 1, 2, 3)
	cases := []struct {
		name  string
		tags  []int64
		share float64
	}{
		{name: "all", tags: []int64{3, 2, 1}, share: 1},
		{name: "two of four", tags: []int64{1, 2, 4}, share: 0.5},
		{name: "none on other", tags: nil, share: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			other := candidate(2, "/b/y.mp4", tc.tags...)
			// 題名「x」は 1 文字なので語にならず、題名の点は付かない。日付は同じ日で満点。
			want := RelatedScore(float64(relatedTagPoints)*tc.share*float64(RelatedScoreUnit)) +
				relatedDatePoints*RelatedScoreUnit
			if got := scoreOf(self, other); got != want {
				t.Fatalf("score = %d, want %d", got, want)
			}
		})
	}
}

// 題名とファイル名は、数字と記号を除いた語の重なりで比べる。かなと漢字は 2 文字ずつ比べる。
func TestTitleWords(t *testing.T) {
	cases := []struct {
		name         string
		a, b         RelatedCandidate
		wantSameness float64
	}{
		{
			name:         "episode numbers and dates are ignored",
			a:            RelatedCandidate{Title: "My Show 2024-01-05 ep01", Path: "/x/my_show_01.mp4"},
			b:            RelatedCandidate{Title: "my show 2025-12-31 ep 02", Path: "/y/MY-SHOW-02.mkv"},
			wantSameness: 1,
		},
		{
			name:         "japanese is compared by pairs of characters",
			a:            RelatedCandidate{Title: "猫の動画 第1話", Path: "/x/1.mp4"},
			b:            RelatedCandidate{Title: "ねこの動画 第2話", Path: "/y/2.mp4"},
			wantSameness: 4.0 / 7.0,
		},
		{
			name:         "nothing in common",
			a:            RelatedCandidate{Title: "alpha", Path: "/x/alpha.mp4"},
			b:            RelatedCandidate{Title: "beta", Path: "/y/beta.mp4"},
			wantSameness: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jaccard(titleWords(tc.a.Title, tc.a.Path), titleWords(tc.b.Title, tc.b.Path))
			if diff := got - tc.wantSameness; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("sameness = %v, want %v (%v / %v)", got, tc.wantSameness,
					titleWords(tc.a.Title, tc.a.Path), titleWords(tc.b.Title, tc.b.Path))
			}
		})
	}
}

func sceneFingerprint(hashes ...uint64) *Fingerprint {
	frames := make([]FrameHash, 0, len(hashes))
	for _, hash := range hashes {
		frames = append(frames, FrameHash{Hash: hash})
	}
	return &Fingerprint{Version: FingerprintVersion, IntervalMs: 1000, Frames: frames}
}

// 指紋は時刻を揃えずに比べ、似た場面が共通する割合に比例する。指紋が無い・比べられる
// コマが足りない・単色ばかりなら 0 点。
func TestContentRelatednessFingerprint(t *testing.T) {
	const a, b, c, d = 0x0000_0000_0000_00ff, 0x0000_0000_ffff_0000, 0x00ff_ff00_0000_0000, 0xff00_0000_00ff_ff00
	base := sceneFingerprint(a, b, c, d)
	flat := sceneFingerprint(a, b, c, d)
	for i := range flat.Frames {
		flat.Frames[i].Flat = true
	}
	cases := []struct {
		name  string
		other *Fingerprint
		share float64
	}{
		// 同じ場面を逆の順で持つ（時刻が揃っていない）。1 ビットの違いは似た場面に入る。
		{name: "same scenes at other times", other: sceneFingerprint(d, c^1, b, a), share: 1},
		{name: "half of the scenes", other: sceneFingerprint(a, b, 0x5555_5555_5555_5555, 0x3333_3333_3333_3333), share: 0.5},
		// 似た場面とみなすのはハミング距離 12 まで。a と 12 ビット違うコマ 3 枚は a と組になり、
		// 13 ビット違うコマはどのコマとも組にならない。
		{name: "12 bits apart", other: sceneFingerprint(a^0xfff, a^0xfff, a^0xfff), share: 4.0 / 7.0},
		{name: "13 bits apart", other: sceneFingerprint(a^0x1fff, a^0x1fff, a^0x1fff), share: 0},
		{name: "missing", other: nil, share: 0},
		{name: "too few frames", other: sceneFingerprint(a, b), share: 0},
		{name: "flat frames only", other: flat, share: 0},
		{name: "older version", other: &Fingerprint{Version: FingerprintVersion + 1, IntervalMs: 1000, Frames: base.Frames}, share: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			self := RelatedCandidate{VideoID: 1, Path: "/x/1.mp4", AddedAt: relatedBase, Fingerprint: base}
			other := RelatedCandidate{VideoID: 2, Path: "/y/2.mp4", AddedAt: relatedBase.Add(400 * 24 * time.Hour), Fingerprint: tc.other}
			got := float64(scoreOf(self, other)) / float64(RelatedScoreUnit)
			want := relatedFingerprintPoints * tc.share
			if diff := got - want; diff > 0.01 || diff < -0.001 {
				t.Fatalf("points = %v, want about %v", got, want)
			}
		})
	}
}

// 日付の近さは同じ日で満点、離れるほど下がり 1 年でほぼ 0 になる。
func TestDateCloseness(t *testing.T) {
	if got := dateCloseness(relatedBase, relatedBase.Add(23*time.Hour)); got != 1 {
		t.Fatalf("same day = %v, want 1", got)
	}
	week := dateCloseness(relatedBase, relatedBase.Add(-8*24*time.Hour))
	month := dateCloseness(relatedBase, relatedBase.Add(31*24*time.Hour))
	year := dateCloseness(relatedBase, relatedBase.Add(365*24*time.Hour))
	if week <= month || month <= year || year >= 0.001 {
		t.Fatalf("closeness week %v, month %v, year %v", week, month, year)
	}
}

// どの手がかりにも当たらない動画は、日付の近い順に後ろに並ぶ。同点なら新しく追加された順、
// それも同じなら id の大きい順。基準の動画と重複は除く。
func TestRankRelatedTieBreaks(t *testing.T) {
	self := RelatedCandidate{VideoID: 1, Path: "/x/self.mp4", Title: "self", AddedAt: relatedBase}
	near := RelatedCandidate{VideoID: 2, Path: "/y/near.mp4", Title: "near", AddedAt: relatedBase.Add(-3 * 24 * time.Hour)}
	far := RelatedCandidate{VideoID: 3, Path: "/z/far.mp4", Title: "far", AddedAt: relatedBase.Add(-300 * 24 * time.Hour)}
	older := RelatedCandidate{VideoID: 9, Path: "/w/older.mp4", Title: "older", AddedAt: relatedBase.Add(-time.Hour)}
	newer := RelatedCandidate{VideoID: 4, Path: "/v/newer.mp4", Title: "newer", AddedAt: relatedBase.Add(time.Hour)}
	sameTimeLow := RelatedCandidate{VideoID: 5, Path: "/u/low.mp4", Title: "low", AddedAt: relatedBase.Add(time.Hour)}

	got := rankedIDs(RankRelated(DefaultRelatedScorer, self,
		[]RelatedCandidate{far, self, near, older, sameTimeLow, newer, near}))
	if want := []int64{5, 4, 9, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// 受け入れ条件 3 と Edge Case: ページを最後まで読むと、すべての動画が重複なく 1 回ずつ出る。
// 読む間に足された・消えた動画があっても、読み込み済みの動画が続きに再び出ない。
func TestPageRelated(t *testing.T) {
	self := RelatedCandidate{VideoID: 100, Path: "/x/self.mp4", AddedAt: relatedBase}
	var candidates []RelatedCandidate
	for i := range int64(25) {
		candidates = append(candidates, RelatedCandidate{
			VideoID: i + 1,
			Path:    "/x/" + string(rune('a'+i)) + ".mp4",
			AddedAt: relatedBase.Add(-time.Duration(i%5) * 24 * time.Hour),
		})
	}
	ranked := RankRelated(DefaultRelatedScorer, self, candidates)

	var seen []int64
	var cursor *RelatedCursor
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("did not reach the end")
		}
		page, next := PageRelated(ranked, cursor, 7)
		seen = append(seen, rankedIDs(page)...)
		if next == nil {
			break
		}
		// 読む間に 1 本消え、1 本足された。
		if pages == 0 {
			ranked = slices.Delete(slices.Clone(ranked), 3, 4)
			extra := RelatedCandidate{VideoID: 50, Path: "/x/new.mp4", AddedAt: relatedBase}
			ranked = RankRelated(DefaultRelatedScorer, self, append(rankedCandidates(ranked, candidates), extra))
		}
		round, err := ParseRelatedCursor(next.String())
		if err != nil || round != *next {
			t.Fatalf("cursor %q does not round-trip: %+v, %v", next.String(), round, err)
		}
		cursor = next
	}
	unique := slices.Clone(seen)
	slices.Sort(unique)
	if len(slices.Compact(unique)) != len(seen) {
		t.Fatalf("a video was shown twice: %v", seen)
	}
	if len(seen) < 24 {
		t.Fatalf("shown %d videos, want every remaining one: %v", len(seen), seen)
	}

	if page, next := PageRelated(nil, nil, 10); len(page) != 0 || next != nil {
		t.Fatalf("empty = %v, %v", page, next)
	}
}

// rankedCandidates は並びに残る動画の候補を返す。
func rankedCandidates(ranked []RankedRelated, candidates []RelatedCandidate) []RelatedCandidate {
	var out []RelatedCandidate
	for _, item := range ranked {
		for _, c := range candidates {
			if c.VideoID == item.VideoID {
				out = append(out, c)
			}
		}
	}
	return out
}

func TestParseRelatedCursorRejectsMalformed(t *testing.T) {
	for _, text := range []string{"", "1.2", "1.2.3.4", "a.2.3", "1.2.0", "1..3"} {
		if _, err := ParseRelatedCursor(text); !errors.Is(err, ErrInvalidRelatedCursor) {
			t.Errorf("ParseRelatedCursor(%q) = %v, want ErrInvalidRelatedCursor", text, err)
		}
	}
	cursor := RelatedCursor{Score: 12_500_000, AddedAt: -5, VideoID: 7}
	if got, err := ParseRelatedCursor(cursor.String()); err != nil || got != cursor {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
}

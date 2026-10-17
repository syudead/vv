package domain

import (
	"runtime"
	"slices"
	"testing"
	"time"
)

// recommendScorer は種と候補の id の組ごとに決めた点を付ける。表に無い組は 0 点である。
type recommendScorer map[[2]int64]RelatedScore

func (s recommendScorer) For(self RelatedCandidate) func(RelatedCandidate) RelatedScore {
	return func(other RelatedCandidate) RelatedScore {
		return s[[2]int64{self.VideoID, other.VideoID}]
	}
}

// recommendCandidates は id ごとに、id 秒に追加した候補を返す。
func recommendCandidates(ids ...int64) []RelatedCandidate {
	candidates := make([]RelatedCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, RelatedCandidate{VideoID: id, AddedAt: time.Unix(id, 0)})
	}
	return candidates
}

// おすすめは種と見終わった動画を除き、どれかの種への近さの最大の高い順に並ぶ。近さの和ではない。
// 同点は関連動画と同じく新しく追加した順で、先頭の limit 本だけを返す。
func TestRankRecommendedOrdersByTheHighestClosenessToAnySeed(t *testing.T) {
	scorer := recommendScorer{
		{1, 3}: 100, // 見終わった
		{1, 2}: 90,  // 種どうし
		{1, 4}: 80,
		{2, 5}: 60,
		{1, 6}: 50, {2, 6}: 50, // 和なら 4 より前
		{2, 7}: 10, {1, 8}: 10, // 同点は新しい 8 が先
	}
	inputs := RecommendInputs{Seeds: []int64{1, 2}, Watched: []int64{3}}
	candidates := recommendCandidates(1, 2, 3, 4, 5, 6, 7, 8, 9)
	if got, want := RankRecommended(scorer, inputs, candidates, 24), []int64{4, 5, 6, 8, 7, 9}; !slices.Equal(got, want) {
		t.Errorf("RankRecommended = %v, want %v", got, want)
	}
	if got, want := RankRecommended(scorer, inputs, candidates, 2), []int64{4, 5}; !slices.Equal(got, want) {
		t.Errorf("RankRecommended(limit 2) = %v, want %v", got, want)
	}
}

// 候補に無い種（もう見せない動画）は点を付けるのに使わない。使える種が無いか limit が 0 なら空である。
func TestRankRecommendedSkipsSeedsThatAreNotCandidates(t *testing.T) {
	scorer := recommendScorer{{1, 4}: 80, {9, 5}: 90}
	candidates := recommendCandidates(1, 4, 5)
	if got, want := RankRecommended(scorer, RecommendInputs{Seeds: []int64{9, 1}}, candidates, 24), []int64{4, 5}; !slices.Equal(got, want) {
		t.Errorf("RankRecommended = %v, want %v", got, want)
	}
	for name, tc := range map[string]struct {
		inputs RecommendInputs
		limit  int
	}{
		"種が無い":      {inputs: RecommendInputs{}, limit: 24},
		"種が候補に無い":   {inputs: RecommendInputs{Seeds: []int64{9}}, limit: 24},
		"limit が 0": {inputs: RecommendInputs{Seeds: []int64{1}}, limit: 0},
	} {
		if got := RankRecommended(scorer, tc.inputs, candidates, tc.limit); got == nil || len(got) != 0 {
			t.Errorf("%s: RankRecommended = %#v, want empty", name, got)
		}
	}
}

// 候補を CPU の数に分けて並行に点を付けても、どの候補も 1 回だけ並び、同じ id の候補は 1 つに
// まとまる。
func TestRankRecommendedScoresEveryCandidateOnceInParallel(t *testing.T) {
	scorer := recommendScorer{}
	ids := []int64{1}
	for id := int64(2); id <= int64(4*runtime.GOMAXPROCS(0)+3); id++ {
		scorer[[2]int64{1, id}] = RelatedScore(id)
		ids = append(ids, id)
	}
	candidates := recommendCandidates(append(ids, 2)...)
	got := RankRecommended(scorer, RecommendInputs{Seeds: []int64{1}}, candidates, len(ids))
	want := slices.Clone(ids[1:])
	slices.Reverse(want)
	if !slices.Equal(got, want) {
		t.Errorf("RankRecommended = %v, want %v", got, want)
	}
}

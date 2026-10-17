package domain

import (
	"runtime"
	"slices"
	"sync"
	"time"
)

// 所有者へのおすすめの棚（docs/design-docs/home.md）。おすすめは、よく再生した動画と新しい
// お気に入り（種）のどれかに中身が近い、見終わっていない動画である。近さは関連動画と同じ
// RelatedScorer で測る（docs/design-docs/related-videos.md）。

const (
	// RecommendSeedPlays は種にする、よく再生した動画の数の上限である。
	RecommendSeedPlays = 10
	// RecommendSeedFavorites は種にする、新しいお気に入りの数の上限である。
	RecommendSeedFavorites = 10
	// RecommendPlayWindow はよく再生した動画を数える期間である。
	RecommendPlayWindow = 90 * 24 * time.Hour
	// RecommendTTL はおすすめの並びを保持する長さである。
	RecommendTTL = 10 * time.Minute
)

// RecommendInputs はおすすめの並びの元である。どの id も所有者に見せる動画の id である。
type RecommendInputs struct {
	// Seeds は種で、よく再生した動画（再生の多い順）と新しいお気に入り（新しい順）を、同じ id は
	// 最初の 1 つだけにしてこの順に並べる。
	Seeds []int64
	// Watched は見終わった動画である。
	Watched []int64
}

// RankRecommended は候補 candidates のうち種と見終わった動画を除いたものを、どれかの種への近さの
// 最大の高い順に並べ、先頭の最大 limit 本の id を返す。同点の並びは関連動画（RankRelated）と同じで
// ある。候補に無い種（もう見せない動画）は使わず、使える種が無ければ空である。2 万本と 20 の種では
// 点を付けるのに数秒かかるので、候補を CPU の数に分けて並行に点を付ける。scorer の For が返す関数は
// 同時に呼ばれる。
func RankRecommended(scorer RelatedScorer, inputs RecommendInputs, candidates []RelatedCandidate, limit int) []int64 {
	excluded := make(map[int64]struct{}, len(inputs.Seeds)+len(inputs.Watched))
	for _, id := range inputs.Watched {
		excluded[id] = struct{}{}
	}
	var scorers []func(RelatedCandidate) RelatedScore
	for _, seed := range inputs.Seeds {
		excluded[seed] = struct{}{}
		if index := slices.IndexFunc(candidates, func(c RelatedCandidate) bool { return c.VideoID == seed }); index >= 0 {
			scorers = append(scorers, scorer.For(candidates[index]))
		}
	}
	if len(scorers) == 0 || limit <= 0 {
		return []int64{}
	}
	var others []RelatedCandidate
	for _, candidate := range candidates {
		if _, skip := excluded[candidate.VideoID]; !skip {
			excluded[candidate.VideoID] = struct{}{}
			others = append(others, candidate)
		}
	}
	ranked := make([]RankedRelated, len(others))
	chunk := max(1, (len(others)+runtime.GOMAXPROCS(0)-1)/runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for start := 0; start < len(others); start += chunk {
		end := min(start+chunk, len(others))
		wg.Go(func() {
			for i := start; i < end; i++ {
				ranked[i] = RankedRelated{Score: highestScore(scorers, others[i]), AddedAt: others[i].AddedAt.Unix(), VideoID: others[i].VideoID}
			}
		})
	}
	wg.Wait()
	slices.SortFunc(ranked, compareRanked)
	ids := make([]int64, 0, min(limit, len(ranked)))
	for _, item := range ranked[:min(limit, len(ranked))] {
		ids = append(ids, item.VideoID)
	}
	return ids
}

// highestScore は候補 candidate のどれかの種への点の最大を返す。scorers は 1 つ以上である。
func highestScore(scorers []func(RelatedCandidate) RelatedScore, candidate RelatedCandidate) RelatedScore {
	best := scorers[0](candidate)
	for _, score := range scorers[1:] {
		best = max(best, score(candidate))
	}
	return best
}

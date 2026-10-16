package domain

import (
	"slices"
	"time"
)

// 所有者へのおすすめの棚（specs/057-home/research.md R-13）。おすすめは、よく再生した動画と新しい
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
// ある。候補に無い種（もう見せない動画）は使わず、使える種が無ければ空である。
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
	ranked := make([]RankedRelated, 0, len(candidates))
	for _, candidate := range candidates {
		if _, skip := excluded[candidate.VideoID]; skip {
			continue
		}
		excluded[candidate.VideoID] = struct{}{}
		best := RelatedScore(0)
		for i, score := range scorers {
			if value := score(candidate); i == 0 || value > best {
				best = value
			}
		}
		ranked = append(ranked, RankedRelated{Score: best, AddedAt: candidate.AddedAt.Unix(), VideoID: candidate.VideoID})
	}
	slices.SortFunc(ranked, compareRanked)
	ids := make([]int64, 0, min(limit, len(ranked)))
	for _, item := range ranked[:min(limit, len(ranked))] {
		ids = append(ids, item.VideoID)
	}
	return ids
}

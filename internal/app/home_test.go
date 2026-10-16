package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// おすすめの棚の並び（specs/057-home/research.md R-13）。棚の項目と要約そのものは保存層と
// internal/httpapi の試験が確かめる。

// fakeShelfStore はホームの保存層の代わりで、おすすめの鍵を落とした棚を空の項目で返し、おすすめの元を
// 返した回数を数える。
type fakeShelfStore struct {
	inputs domain.RecommendInputs
	reads  int
}

func (s *fakeShelfStore) Home(context.Context, domain.Audience) (domain.Home, error) {
	return domain.Home{}, nil
}

func (s *fakeShelfStore) ShelfItems(_ context.Context, _ domain.Audience, keys []domain.HomeShelfKey) (domain.HomeShelfPage, error) {
	page := domain.HomeShelfPage{}
	for _, key := range keys {
		if key.Kind != domain.HomeShelfRecommended {
			page.Shelves = append(page.Shelves, domain.HomeShelfItems{Key: key})
		}
	}
	return page, nil
}

func (s *fakeShelfStore) SetShelves(context.Context, []int64) error { return nil }

func (s *fakeShelfStore) RecommendInputs(context.Context) (domain.RecommendInputs, error) {
	s.reads++
	return s.inputs, nil
}

// fakeRecommendCatalog は候補 ids を手がかりに、どの id も見せる動画として返す。
type fakeRecommendCatalog struct{ ids []int64 }

func (c fakeRecommendCatalog) RelatedCandidates(context.Context, domain.Audience, int64) ([]domain.RelatedCandidate, error) {
	candidates := make([]domain.RelatedCandidate, 0, len(c.ids))
	for _, id := range c.ids {
		candidates = append(candidates, domain.RelatedCandidate{VideoID: id, AddedAt: time.Unix(id, 0)})
	}
	return candidates, nil
}

func (c fakeRecommendCatalog) VideosByIDs(_ context.Context, _ domain.Audience, ids []int64) ([]domain.Video, error) {
	videos := make([]domain.Video, 0, len(ids))
	for _, id := range ids {
		videos = append(videos, domain.Video{ID: id})
	}
	return videos, nil
}

// tableScorer は種と候補の id の組ごとに決めた点を付ける。表に無い組は 0 点である。
type tableScorer map[[2]int64]domain.RelatedScore

func (s tableScorer) For(self domain.RelatedCandidate) func(domain.RelatedCandidate) domain.RelatedScore {
	return func(other domain.RelatedCandidate) domain.RelatedScore { return s[[2]int64{self.VideoID, other.VideoID}] }
}

func recommendedIDsOf(t *testing.T, page domain.HomeShelfPage) []int64 {
	t.Helper()
	for _, shelf := range page.Shelves {
		if shelf.Key.Kind == domain.HomeShelfRecommended {
			ids := make([]int64, 0, len(shelf.Items))
			for _, item := range shelf.Items {
				ids = append(ids, item.Video.ID)
			}
			if shelf.Total != len(ids) {
				t.Errorf("おすすめの total = %d, want %d", shelf.Total, len(ids))
			}
			return ids
		}
	}
	return nil
}

var recommendedKeys = []domain.HomeShelfKey{{Kind: domain.HomeShelfRecent}, {Kind: domain.HomeShelfRecommended}, {Kind: domain.HomeShelfUnshelved}}

// おすすめは種（1、2）と見終わった動画（3）を除き、どれかの種への近さの最大の高い順に並ぶ。近さの和では
// ない: 6 は両方の種に中くらいに近いが、片方の種にだけ近い 4 より後である。棚は求めた鍵の位置に入る。
func TestRecommendedOrdersByTheHighestClosenessToAnySeed(t *testing.T) {
	store := &fakeShelfStore{inputs: domain.RecommendInputs{Seeds: []int64{1, 2}, Watched: []int64{3}}}
	scorer := tableScorer{
		{1, 3}: 100, // 見終わった
		{1, 2}: 90,  // 種どうし
		{1, 4}: 80,
		{2, 5}: 60,
		{1, 6}: 50, {2, 6}: 50,
	}
	home := NewHome(HomeOptions{Store: store, Catalog: fakeRecommendCatalog{ids: []int64{1, 2, 3, 4, 5, 6, 7}}, Scorer: scorer})
	page, err := home.ShelfItems(context.Background(), domain.AudienceOwner, recommendedKeys)
	if err != nil {
		t.Fatal(err)
	}
	// 7 はどの種にも 0 点で、最後に来る。
	if got, want := recommendedIDsOf(t, page), []int64{4, 5, 6, 7}; !slices.Equal(got, want) {
		t.Errorf("おすすめ = %v, want %v", got, want)
	}
	kinds := make([]domain.HomeShelfKind, 0, len(page.Shelves))
	for _, shelf := range page.Shelves {
		kinds = append(kinds, shelf.Key.Kind)
	}
	if want := []domain.HomeShelfKind{domain.HomeShelfRecent, domain.HomeShelfRecommended, domain.HomeShelfUnshelved}; !slices.Equal(kinds, want) {
		t.Errorf("棚 = %v, want %v", kinds, want)
	}
}

// おすすめの並びは domain.RecommendTTL のあいだ保持し、過ぎれば元を読み直す。ゲストと種の無い所有者には
// おすすめの棚を返さない。
func TestRecommendedIsHeldForTheTTLAndOwnerOnly(t *testing.T) {
	store := &fakeShelfStore{inputs: domain.RecommendInputs{Seeds: []int64{1}}}
	clock := time.Unix(1_000_000, 0)
	home := NewHome(HomeOptions{
		Store: store, Catalog: fakeRecommendCatalog{ids: []int64{1, 2}}, Scorer: tableScorer{},
		Now: func() time.Time { return clock },
	})
	ctx := context.Background()
	read := func(audience domain.Audience) []int64 {
		t.Helper()
		page, err := home.ShelfItems(ctx, audience, recommendedKeys)
		if err != nil {
			t.Fatal(err)
		}
		return recommendedIDsOf(t, page)
	}
	if got := read(domain.AudienceGuest); got != nil {
		t.Errorf("ゲストのおすすめ = %v, want none", got)
	}
	if got := read(domain.AudienceOwner); !slices.Equal(got, []int64{2}) {
		t.Fatalf("おすすめ = %v, want [2]", got)
	}
	store.inputs = domain.RecommendInputs{}
	clock = clock.Add(domain.RecommendTTL - time.Second)
	if got := read(domain.AudienceOwner); !slices.Equal(got, []int64{2}) || store.reads != 1 {
		t.Errorf("期限内のおすすめ = %v（元を %d 回読んだ）, want the held [2]", got, store.reads)
	}
	clock = clock.Add(time.Second)
	if got := read(domain.AudienceOwner); got != nil || store.reads != 2 {
		t.Errorf("種の無いおすすめ = %v（元を %d 回読んだ）, want none", got, store.reads)
	}
}

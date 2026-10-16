package app

import (
	"context"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/syudead/vv/internal/domain"
)

// HomeShelfStore はホームの棚の読み出しと並びの書き込みと、おすすめの元の保存先である。
// internal/store の *HomeStore が満たす（specs/057-home/contracts/screen-api.md）。
type HomeShelfStore interface {
	Home(ctx context.Context, audience domain.Audience) (domain.Home, error)
	// ShelfItems はおすすめの棚の鍵を落とし、ほかの棚の最初の項目を keys の順に返す。
	ShelfItems(ctx context.Context, audience domain.Audience, keys []domain.HomeShelfKey) (domain.HomeShelfPage, error)
	SetShelves(ctx context.Context, ids []int64) error
	// RecommendInputs はおすすめの種と見終わった動画を返す。
	RecommendInputs(ctx context.Context) (domain.RecommendInputs, error)
}

// RecommendCatalog はおすすめの近さの手がかりと、並べた動画の本体を読む口である。internal/store の
// *LibraryStore が満たす（関連動画と同じ口。catalog.go の VideoIndex）。
type RecommendCatalog interface {
	RelatedCandidates(ctx context.Context, audience domain.Audience, selfID int64) ([]domain.RelatedCandidate, error)
	VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error)
}

// HomeOptions はホームの読み出しに必要な依存である。
type HomeOptions struct {
	Store   HomeShelfStore
	Catalog RecommendCatalog
	// Scorer は近さの付け方で、nil なら domain.DefaultRelatedScorer である。
	Scorer domain.RelatedScorer
	// Now は nil なら time.Now である。おすすめの保持の期限に使う。
	Now func() time.Time
}

// Home はホームの読み出しで、保存層の読み出しに所有者のおすすめの棚を足す（specs/057-home/research.md
// R-13）。おすすめは種のどれかへの近さの最大で並べ、並びを domain.RecommendTTL のあいだ保持する。
// internal/httpapi の Home を満たす。
type Home struct {
	store   HomeShelfStore
	catalog RecommendCatalog
	scorer  domain.RelatedScorer
	now     func() time.Time

	// mu はおすすめの並びの計算と保持を守る。計算は全動画の手がかりを読むので、同時の読み出しが
	// 重ねて計算しないよう、計算のあいだも持つ。
	mu          sync.Mutex
	recommended []int64
	expiry      time.Time
}

// NewHome はホームの読み出しを組み立てる。
func NewHome(opts HomeOptions) *Home {
	h := &Home{store: opts.Store, catalog: opts.Catalog, scorer: opts.Scorer, now: opts.Now}
	if h.scorer == nil {
		h.scorer = domain.DefaultRelatedScorer
	}
	if h.now == nil {
		h.now = time.Now
	}
	return h
}

// Home は見る人のホームの要約を返す。おすすめの棚は、種があれば保存層が所有者に並べる。
func (h *Home) Home(ctx context.Context, audience domain.Audience) (domain.Home, error) {
	return h.store.Home(ctx, audience)
}

// SetShelves は棚を ids のフォルダにする。
func (h *Home) SetShelves(ctx context.Context, ids []int64) error {
	return h.store.SetShelves(ctx, ids)
}

// ShelfItems は棚 keys の最初の項目を keys の順に返す。おすすめの棚は所有者にだけ、項目があるときだけ
// 返し、ほかの棚は保存層が読む。保持の無いおすすめは計算に 1 秒ほどかかるので、ほかの棚の読み出しと
// 並行に計算する。
func (h *Home) ShelfItems(ctx context.Context, audience domain.Audience, keys []domain.HomeShelfKey) (domain.HomeShelfPage, error) {
	recommendedKey := domain.HomeShelfKey{Kind: domain.HomeShelfRecommended}
	at := slices.Index(keys, recommendedKey)
	if at < 0 || !audience.IsOwner() {
		return h.store.ShelfItems(ctx, audience, keys)
	}
	var group errgroup.Group
	var items []domain.LibraryItem
	group.Go(func() (err error) {
		items, err = h.recommendedItems(ctx)
		return err
	})
	page, err := h.store.ShelfItems(ctx, audience, keys)
	if waitErr := group.Wait(); err == nil {
		err = waitErr
	}
	if err != nil || len(items) == 0 {
		return page, err
	}
	// 保存層の結果はおすすめの鍵を落とした keys の順なので、おすすめの前にある鍵の棚の数の位置に入れる。
	before := 0
	for _, shelf := range page.Shelves {
		if slices.Index(keys, shelf.Key) < at {
			before++
		}
	}
	shelf := domain.HomeShelfItems{Key: recommendedKey, Items: items, Total: len(items)}
	page.Shelves = slices.Insert(page.Shelves, before, shelf)
	return page, nil
}

// recommendedItems はおすすめの並びの動画を項目にする。もう見せない動画は抜ける。
func (h *Home) recommendedItems(ctx context.Context) ([]domain.LibraryItem, error) {
	ids, err := h.recommendedIDs(ctx)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	videos, err := h.catalog.VideosByIDs(ctx, domain.AudienceOwner, ids)
	if err != nil {
		return nil, err
	}
	items := make([]domain.LibraryItem, 0, len(videos))
	for i := range videos {
		items = append(items, domain.LibraryItem{Video: &videos[i]})
	}
	return items, nil
}

// recommendedIDs はおすすめの並びを返す。保持が期限内ならそれを、過ぎていれば計算し直して保持する。
func (h *Home) recommendedIDs(ctx context.Context) ([]int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.recommended != nil && h.now().Before(h.expiry) {
		return h.recommended, nil
	}
	inputs, err := h.store.RecommendInputs(ctx)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	if len(inputs.Seeds) > 0 {
		candidates, err := h.catalog.RelatedCandidates(ctx, domain.AudienceOwner, 0)
		if err != nil {
			return nil, err
		}
		ids = domain.RankRecommended(h.scorer, inputs, candidates, domain.HomeShelfItemLimit)
	}
	h.recommended, h.expiry = ids, h.now().Add(domain.RecommendTTL)
	return ids, nil
}

package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// CatalogIngestStore は動画の応答から取り込み状態を問い合わせ、更新する保存先である。
type CatalogIngestStore interface {
	// RequeueMissingPreview は、作り終えた記録があるのにファイルが無いプレビューの
	// 状態を pending へ戻し、作り直しを積む。両方を1つの取引で行う。積んだら
	// true を返す。すでに戻っていれば false。
	RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error)
	// RequeueMissingSeekThumbnails は、作り終えた記録があるのに置き場が無い
	// シーク用サムネイルの状態を pending へ戻し、作り直しを積む。両方を1つの
	// 取引で行う。積んだら true を返す。すでに戻っていれば false。
	RequeueMissingSeekThumbnails(ctx context.Context, id int64, contentKey string) (bool, error)
	// RetryProbe は読み取りに失敗した動画を読み取り直す状態へ戻し、ジョブを積む。
	RetryProbe(ctx context.Context, id int64) error
}

// CatalogIndexStore は関連動画を組み立てるためのライブラリ索引である。どの
// 読み出しも見る人（domain.Audience）を取り、ゲストには公開の動画だけを返す
type CatalogIndexStore interface {
	// RelatedCandidates は関連動画の近さを測る手がかりを、見る人に見せる動画すべてと基準の
	// 動画 selfID について返す。
	RelatedCandidates(ctx context.Context, audience domain.Audience, selfID int64) ([]domain.RelatedCandidate, error)
	VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error)
	// VideoGroup は動画 id の文脈のグループ（contextID は動画を開いたグループ、無ければ 0。
	// docs/design-docs/catalog-organization.md）を、見る人に見せてよい動画の再生順で返す。詳細は
	// window の範囲だけを読む。動画がどのグループにも無ければ false。
	VideoGroup(ctx context.Context, audience domain.Audience, id int64, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error)
}

// ArtifactFiles は生成物が今あるかを答える。internal/artifacts の *Store が
// これを満たす。ホバープレビューは manifest と大きさが一致するときだけ、ある
// とみなす。
type ArtifactFiles interface {
	PreviewAvailable(contentKey string) bool
	SeekThumbnailsAvailable(contentKey string) bool
}

// CatalogOptions は動画の応答の組み立てに必要な依存である。
type CatalogOptions struct {
	Index  CatalogIndexStore
	Ingest CatalogIngestStore
	Files  ArtifactFiles
	// Scorer は関連動画の近さの付け方。nil なら domain.DefaultRelatedScorer を使う。
	Scorer domain.RelatedScorer
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Catalog は動画を応答に載せるときの判断（消えたプレビューの作り直しの予約、
// シーク用プレビューの状態の導出）と、関連動画の組み立てを受け持つ。
type Catalog struct {
	index  CatalogIndexStore
	ingest CatalogIngestStore
	files  ArtifactFiles
	scorer domain.RelatedScorer
	logger *slog.Logger
}

// NewCatalog は動画の応答の組み立てを返す。
func NewCatalog(opts CatalogOptions) *Catalog {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	scorer := opts.Scorer
	if scorer == nil {
		scorer = domain.DefaultRelatedScorer
	}
	return &Catalog{index: opts.Index, ingest: opts.Ingest, files: opts.Files, scorer: scorer, logger: logger}
}

// PresentVideos は動画たちを応答に載せる形にする。順序は保つ。
//
// プレビューを作り終えた記録があるのにファイルが無ければ、作り直しを積み、
// 準備中として返す。ファイルの有無は、URL を出すかどうかを決めるためにもともと
// 確かめている。見つけたときだけ書くので、ファイルがある通常の場合に書き込みは
// 増えない。積むのは状態が done の間の1回だけで（同じ動画を並行して見ても、
// 保存層の取引で1回になる）、作り直しが上限まで失敗すれば failed になり、
// それ以上は積まない。
func (c *Catalog) PresentVideos(ctx context.Context, videos []domain.Video) []domain.VideoView {
	views := make([]domain.VideoView, 0, len(videos))
	for _, video := range videos {
		views = append(views, c.present(ctx, video))
	}
	return views
}

func (c *Catalog) present(ctx context.Context, video domain.Video) domain.VideoView {
	if video.PreviewState != domain.PreviewStateDone {
		return domain.VideoView{Video: video}
	}
	if c.files.PreviewAvailable(video.ContentKey) {
		return domain.VideoView{Video: video, PreviewAvailable: true}
	}
	if video.ContentKey == "" {
		return domain.VideoView{Video: video}
	}
	requeued, err := c.ingest.RequeueMissingPreview(ctx, video.ID, video.ContentKey)
	switch {
	case err != nil:
		// 作り直しを積めなくても、応答は URL を省いて返せる。次に見つけたときに積む。
		c.logger.Warn("could not queue regeneration of a missing preview",
			slog.Int64("videoId", video.ID), slog.Any("error", err))
	case requeued:
		video.PreviewState = domain.PreviewStatePending
	}
	return domain.VideoView{Video: video}
}

// SeekThumbnailState はシーク用サムネイルの状態を導く。
//
// 置き場があれば done。無ければ、保存した状態が done のときは作り直しを積んで
// pending とし（積めなくても pending。次に見つけたときに積む）、それ以外は
// 保存した状態をそのまま返す。積むのは状態が done の間の1回だけで、同じ動画を
// 並行して見ても保存層の取引で1回になる（プレビューの RequeueMissingPreview と
// 同じ形）。
func (c *Catalog) SeekThumbnailState(ctx context.Context, video domain.Video) (domain.SeekThumbnailState, error) {
	if c.files.SeekThumbnailsAvailable(video.ContentKey) {
		return domain.SeekThumbnailDone, nil
	}
	if video.SeekThumbnailState != domain.SeekThumbnailDone {
		return video.SeekThumbnailState, nil
	}
	if _, err := c.ingest.RequeueMissingSeekThumbnails(ctx, video.ID, video.ContentKey); err != nil {
		c.logger.Warn("could not queue regeneration of missing seek thumbnails",
			slog.Int64("videoId", video.ID), slog.Any("error", err))
	}
	return domain.SeekThumbnailPending, nil
}

// VideoGroup は動画の文脈（opened は動画を開いたグループとスマートフォルダ。domain.VideoContext、
// docs/design-docs/catalog-organization.md）の、見る人に見せてよい動画の再生順を返す
// （GET /api/videos/{id} の group と GET /api/videos/{id}/group-members）。詳細は window の
// 範囲だけを読む。動画がどのグループにも無ければ false。
func (c *Catalog) VideoGroup(ctx context.Context, audience domain.Audience, video domain.Video, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error) {
	return c.index.VideoGroup(ctx, audience, video.ID, opened, window)
}

// RelatedVideos は関連動画の 1 ページを返す（issue 931）。
//
// 並べ方は internal/domain の RankRelated と、近さの付け方 scorer（既定は
// domain.DefaultRelatedScorer）が決める。ここは、見る人に見せる動画すべての手がかりを読み、
// 並べ、page の範囲の動画の本体を引くだけである。どれも見る人（audience）として読むので、
// ゲストには公開の動画だけが並ぶ。基準の動画が手がかりに無ければ（読む間に消えた）、
// 関連動画は空にする。
//
// 動画がグループにあれば、文脈のグループ（page.GroupID、docs/design-docs/catalog-organization.md）の
// 再生順の動画は並べる前に除く。最初のページだけがグループの窓を載せ、次と前の動画を
// その再生順にする。グループに属さない動画の次の動画は、関連動画の並びの
// 先頭である。前の動画は返さない（画面が見てきた順から決める）。
func (c *Catalog) RelatedVideos(ctx context.Context, audience domain.Audience, video domain.Video, page domain.RelatedPage) (domain.RelatedVideos, error) {
	first := page.After == nil
	window := domain.GroupWindow{}
	if first {
		window = domain.GroupWindow{Limit: domain.GroupMemberWindow, Around: true}
	}
	group, grouped, err := c.index.VideoGroup(ctx, audience, video.ID, page.Context(), window)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	members := map[int64]struct{}{}
	if grouped {
		for _, member := range group.MemberIDs {
			members[member] = struct{}{}
		}
	}

	candidates, err := c.index.RelatedCandidates(ctx, audience, video.ID)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	var ranked []domain.RankedRelated
	selfIndex := slices.IndexFunc(candidates, func(candidate domain.RelatedCandidate) bool {
		return candidate.VideoID == video.ID
	})
	if selfIndex >= 0 {
		self := candidates[selfIndex]
		others := slices.DeleteFunc(slices.Clone(candidates), func(candidate domain.RelatedCandidate) bool {
			_, member := members[candidate.VideoID]
			return member
		})
		ranked = domain.RankRelated(c.scorer, self, others)
	}

	shown, next := domain.PageRelated(ranked, page.After, page.Limit)
	ids := make([]int64, 0, len(shown))
	for _, item := range shown {
		ids = append(ids, item.VideoID)
	}
	items, err := c.index.VideosByIDs(ctx, audience, ids)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	related := domain.RelatedVideos{Items: items, NextCursor: next}
	if !first {
		return related, nil
	}
	if grouped {
		related.PrevID, related.NextID = group.Neighbors(video.ID)
		related.Group = &group
	} else if len(ranked) > 0 {
		related.NextID = ranked[0].VideoID
	}
	return related, nil
}

// RetryProbe は読み取りに失敗した動画を読み取り直す。状態を戻すこととジョブを
// 積むことは保存層が1つの取引で行う。
//
// 失敗していない動画には domain.ErrProbeNotFailed、無い動画には
// domain.ErrNotFound を返す。
func (c *Catalog) RetryProbe(ctx context.Context, video domain.Video) error {
	return c.ingest.RetryProbe(ctx, video.ID)
}

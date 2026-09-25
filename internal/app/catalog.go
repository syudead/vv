package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// CatalogIngestStore は動画の応答から取り込み状態を問い合わせ、更新する保存先である。
type CatalogIngestStore interface {
	// RequeueMissingPreview は、作り終えた記録があるのにファイルが無いプレビューの
	// 状態を pending へ戻し、作り直しを積む。両方を1つの取引で行う。積んだら
	// true を返す。すでに戻っていれば false。
	RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error)
	// ThumbnailJobActive はサムネイルのジョブが queued・running かを返す。
	ThumbnailJobActive(ctx context.Context, videoID int64) (bool, error)
	// RetryProbe は読み取りに失敗した動画を読み取り直す状態へ戻し、ジョブを積む。
	RetryProbe(ctx context.Context, id int64, seekThumbnailMissing bool) error
}

// CatalogIndexStore は関連動画を組み立てるためのライブラリ索引である。どの
// 読み出しも見る人（domain.Audience）を取り、ゲストには公開の動画だけを返す
// （specs/016-single-account-auth/data-model.md §3）。
type CatalogIndexStore interface {
	DirectVideoPaths(ctx context.Context, audience domain.Audience, dir string) ([]domain.RelatedSibling, error)
	VideosAddedNear(ctx context.Context, audience domain.Audience, id int64, addedAt time.Time, limit int) ([]domain.RelatedNeighbor, error)
	VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error)
	// VideoGroup は動画 id が属するグループを、見る人に見せてよいメンバーだけで返す。
	// 見る人に見せるグループが無ければ false。
	VideoGroup(ctx context.Context, audience domain.Audience, id int64) (domain.VideoGroup, bool, error)
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
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Catalog は動画を応答に載せるときの判断（消えたプレビューの作り直しの予約、
// シーク用プレビューの状態の導出）と、関連動画の組み立てを受け持つ。
type Catalog struct {
	index  CatalogIndexStore
	ingest CatalogIngestStore
	files  ArtifactFiles
	logger *slog.Logger
}

// NewCatalog は動画の応答の組み立てを返す。
func NewCatalog(opts CatalogOptions) *Catalog {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Catalog{index: opts.Index, ingest: opts.Ingest, files: opts.Files, logger: logger}
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
		c.logger.Warn("消えたプレビューの作り直しを積めませんでした",
			slog.Int64("videoId", video.ID), slog.Any("error", err))
	case requeued:
		video.PreviewState = domain.PreviewStatePending
	}
	return domain.VideoView{Video: video}
}

// SeekThumbnailState はシーク用プレビューの状態を導く。DB 上に状態は無い。
//
// 置き場があれば done。無ければ、thumbnail_state が pending か、サムネイルの
// ジョブが queued・running のときだけ pending で、それ以外は failed とする。
// 失敗したジョブの行は保持期間を過ぎると消えるので、行が無いことを pending と
// 読まない。そう読むと、画面の作成中の1行と取り直しが止まらなくなる。
func (c *Catalog) SeekThumbnailState(ctx context.Context, video domain.Video) (domain.SeekThumbnailState, error) {
	if c.files.SeekThumbnailsAvailable(video.ContentKey) {
		return domain.SeekThumbnailDone, nil
	}
	if video.ThumbnailState == domain.ThumbnailStatePending {
		return domain.SeekThumbnailPending, nil
	}
	active, err := c.ingest.ThumbnailJobActive(ctx, video.ID)
	if err != nil {
		return "", err
	}
	if active {
		return domain.SeekThumbnailPending, nil
	}
	return domain.SeekThumbnailFailed, nil
}

// VideoGroup は動画が属するグループを、見る人に見せてよいメンバーだけで返す
// （GET /api/videos/{id} の group）。見る人に見せるグループが無ければ false。
func (c *Catalog) VideoGroup(ctx context.Context, audience domain.Audience, video domain.Video) (domain.VideoGroup, bool, error) {
	return c.index.VideoGroup(ctx, audience, video.ID)
}

// RelatedVideos は関連動画を返す順に並べる。
//
// 並べ方は internal/domain の OrderRelated が決める。ここは、代表の所在の
// ディレクトリ直下の動画と、追加日時の近い動画を読み、選ばれた動画の本体を
// 引くだけである。どれも見る人（audience）として読むので、ゲストには公開の動画
// だけが並ぶ。
//
// 動画がグループのメンバーなら（specs/017-folder-groups/contracts/folder-groups-api.md §3）、
// グループを載せ、前後をグループの中の並びにする。関連動画は、同じグループのメンバーを
// OrderRelated の入力から先に除いてから並べるので、上限はその後に掛かり、大きなグループ
// でも関連動画が残る。追加日時の近い動画は、除く本数を見込んで多めに読む。
func (c *Catalog) RelatedVideos(ctx context.Context, audience domain.Audience, video domain.Video) (domain.RelatedVideos, error) {
	group, grouped, err := c.index.VideoGroup(ctx, audience, video.ID)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	members := map[int64]struct{}{}
	if grouped {
		for _, member := range group.Members {
			members[member.ID] = struct{}{}
		}
	}
	isMember := func(id int64) bool {
		_, ok := members[id]
		return ok
	}

	siblings, err := c.index.DirectVideoPaths(ctx, audience, filepath.Dir(video.Path))
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	neighbors, err := c.index.VideosAddedNear(ctx, audience, video.ID, video.AddedAt, domain.MaxRelatedVideos+len(members))
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	siblings = slices.DeleteFunc(slices.Clone(siblings), func(s domain.RelatedSibling) bool { return isMember(s.VideoID) })
	neighbors = slices.DeleteFunc(slices.Clone(neighbors), func(n domain.RelatedNeighbor) bool { return isMember(n.VideoID) })

	order := domain.OrderRelated(domain.RelatedSelf{VideoID: video.ID, Path: video.Path, AddedAt: video.AddedAt},
		siblings, neighbors)
	items, err := c.index.VideosByIDs(ctx, audience, order.IDs)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	if !grouped {
		return domain.RelatedVideos{Items: items, NextID: order.NextID, PrevID: order.PrevID}, nil
	}
	prev, next := group.Neighbors(video.ID)
	return domain.RelatedVideos{Items: items, NextID: next, PrevID: prev, Group: &group}, nil
}

// RetryProbe は読み取りに失敗した動画を読み取り直す。状態を戻すこととジョブを
// 積むことは保存層が1つの取引で行う。シーク用プレビューの置き場の有無は
// ファイルの事実なので、ここで確かめて渡す。
//
// 失敗していない動画には domain.ErrProbeNotFailed、無い動画には
// domain.ErrNotFound を返す。
func (c *Catalog) RetryProbe(ctx context.Context, video domain.Video) error {
	return c.ingest.RetryProbe(ctx, video.ID, !c.files.SeekThumbnailsAvailable(video.ContentKey))
}

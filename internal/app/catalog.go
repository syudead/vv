package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// CatalogStore は動画の応答を組み立てるときの問い合わせ先である。
// internal/store の *DB がこれを満たす。
type CatalogStore interface {
	// RequeueMissingPreview は、作り終えた記録があるのにファイルが無いプレビューの
	// 状態を pending へ戻し、作り直しを積む。両方を1つの取引で行う。積んだら
	// true を返す。すでに戻っていれば false。
	RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error)
	// ThumbnailJobActive はサムネイルのジョブが queued・running かを返す。
	ThumbnailJobActive(ctx context.Context, videoID int64) (bool, error)
	// RetryProbe は読み取りに失敗した動画を読み取り直す状態へ戻し、ジョブを積む。
	RetryProbe(ctx context.Context, id int64, seekThumbnailMissing bool) error
	DirectVideoPaths(ctx context.Context, dir string) ([]domain.RelatedSibling, error)
	VideosAddedNear(ctx context.Context, id int64, addedAt time.Time, limit int) ([]domain.RelatedNeighbor, error)
	VideosByIDs(ctx context.Context, ids []int64) ([]domain.Video, error)
}

// ArtifactFiles は生成物のファイルが今あるかを答える。internal/media の
// *Assets がこれを満たす。
type ArtifactFiles interface {
	PreviewAvailable(contentKey string) bool
	SeekThumbnailsAvailable(contentKey string) bool
}

// CatalogOptions は動画の応答の組み立てに必要な依存である。
type CatalogOptions struct {
	Store CatalogStore
	Files ArtifactFiles
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Catalog は動画を応答に載せるときの判断（消えたプレビューの作り直しの予約、
// シーク用プレビューの状態の導出）と、関連動画の組み立てを受け持つ。
type Catalog struct {
	store  CatalogStore
	files  ArtifactFiles
	logger *slog.Logger
}

// NewCatalog は動画の応答の組み立てを返す。
func NewCatalog(opts CatalogOptions) *Catalog {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Catalog{store: opts.Store, files: opts.Files, logger: logger}
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
	requeued, err := c.store.RequeueMissingPreview(ctx, video.ID, video.ContentKey)
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
	active, err := c.store.ThumbnailJobActive(ctx, video.ID)
	if err != nil {
		return "", err
	}
	if active {
		return domain.SeekThumbnailPending, nil
	}
	return domain.SeekThumbnailFailed, nil
}

// RelatedVideos は関連動画を返す順に並べる。
//
// 並べ方は internal/domain の OrderRelated が決める。ここは、代表の所在の
// ディレクトリ直下の動画と、追加日時の近い動画を読み、選ばれた動画の本体を
// 引くだけである。
func (c *Catalog) RelatedVideos(ctx context.Context, video domain.Video) (domain.RelatedVideos, error) {
	siblings, err := c.store.DirectVideoPaths(ctx, filepath.Dir(video.Path))
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	neighbors, err := c.store.VideosAddedNear(ctx, video.ID, video.AddedAt, domain.MaxRelatedVideos)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	order := domain.OrderRelated(domain.RelatedSelf{VideoID: video.ID, Path: video.Path, AddedAt: video.AddedAt},
		siblings, neighbors)
	items, err := c.store.VideosByIDs(ctx, order.IDs)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	return domain.RelatedVideos{Items: items, NextID: order.NextID}, nil
}

// RetryProbe は読み取りに失敗した動画を読み取り直す。状態を戻すこととジョブを
// 積むことは保存層が1つの取引で行う。シーク用プレビューの置き場の有無は
// ファイルの事実なので、ここで確かめて渡す。
//
// 失敗していない動画には domain.ErrProbeNotFailed、無い動画には
// domain.ErrNotFound を返す。
func (c *Catalog) RetryProbe(ctx context.Context, video domain.Video) error {
	return c.store.RetryProbe(ctx, video.ID, !c.files.SeekThumbnailsAvailable(video.ContentKey))
}

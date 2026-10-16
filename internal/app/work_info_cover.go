package app

import (
	"bytes"
	"context"
	"image"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインのカバー画像（docs/design-docs/work-info-plugins.md のカバー画像の節）。ランタイムが取った本文を
// 見出しで確かめて取得の結果と一緒に持ち、反映で選ばれれば手元の画像のアップロードと同じ確かめと適用で
// 動画の代表サムネイルにする。

// WorkInfoCoverApplier はカバー画像を動画の代表サムネイルにする適用である。*Ingest が満たす。
type WorkInfoCoverApplier interface {
	// UploadThumbnailImage は画像 image を動画 videoID の代表サムネイルにし、反映後の動画を返す。contentKey は
	// 反映を始めたときの動画の内容である。
	UploadThumbnailImage(ctx context.Context, videoID int64, contentKey string, image []byte) (domain.Video, error)
}

// fetchedCover は取った本文 data を、上限以下の PNG・JPEG・静止画の WebP で、見出しの画素数がアップロードの
// 上限以下ならカバー画像にする。外れれば nil で、カバー画像は無いものとする。最後まで読むのは反映のときの
// アップロードと同じ確かめである。
func fetchedCover(data []byte) *domain.WorkInfoCover {
	if len(data) == 0 || len(data) > domain.WorkInfoCoverMaxBytes || animatedWebP(data) {
		return nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > domain.ThumbnailUploadMaxPixels {
		return nil
	}
	types := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}
	contentType, ok := types[format]
	if !ok {
		return nil
	}
	return &domain.WorkInfoCover{Data: data, ContentType: contentType, Width: config.Width, Height: config.Height}
}

// WorkInfoFetchCover は動画 videoID の取得の結果 fetchID のカバー画像を返す。結果が無いか期限切れか、別の動画の
// ものか、カバー画像を持たなければ domain.ErrNotFound を返す。
func (p *WorkInfoPlugins) WorkInfoFetchCover(videoID int64, fetchID string) (domain.WorkInfoCover, error) {
	return p.fetches.cover(fetchID, videoID, p.now())
}

// applyCover は取得の結果 fetch のカバー画像を動画の代表サムネイルにし、反映後の動画を返す。作品情報の反映が
// 確定したあとに呼ぶので、できなかったとき（動画の内容が変わった、画像を読めないか合わせられない、書けない）
// は誤りにせず、反映前の動画 video と飛ばした値を返す。結果は使い切りで、誤りを返すと画面は作品情報を
// 反映できたかを知れない。
func (p *WorkInfoPlugins) applyCover(
	ctx context.Context, fetch *workInfoFetch, video domain.Video,
) (domain.Video, []domain.SkippedWorkInfoValue) {
	skipped := []domain.SkippedWorkInfoValue{{
		Field: domain.WorkInfoFieldCover, Value: fetch.details.CoverURL, Reason: domain.SkippedWorkInfoInvalid,
	}}
	if p.covers == nil {
		return video, skipped
	}
	current, err := p.videos.GetVideo(ctx, domain.AudienceOwner, fetch.videoID)
	if err != nil {
		return video, skipped
	}
	saved, err := p.covers.UploadThumbnailImage(ctx, fetch.videoID, current.ContentKey, fetch.cover.Data)
	if err != nil {
		return video, skipped
	}
	// サムネイルの適用が返す動画は作品情報を読まないので、確定した作品情報を写す。
	saved.WorkInfo = video.WorkInfo
	return saved, nil
}

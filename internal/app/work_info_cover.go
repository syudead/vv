package app

import (
	"bytes"
	"context"
	"image"

	"github.com/syudead/vv/internal/domain"
)

// Keep header-validated covers with fetch results; selected covers use the same full validation and application as uploads. See docs/design-docs/work-info-plugins.md.

// WorkInfoCoverApplier installs a cover thumbnail; implemented by Ingest.
type WorkInfoCoverApplier interface {
	// UploadThumbnailImage returns the updated video; contentKey is the content observed when application began.
	UploadThumbnailImage(ctx context.Context, videoID int64, contentKey string, image []byte) (domain.Video, error)
}

// Accept bounded PNG, JPEG or static WebP covers by header, otherwise return nil. Full decoding happens only when applying through upload validation.
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

// WorkInfoFetchCover returns ErrNotFound for missing/expired results, wrong videos or absent covers.
func (p *WorkInfoPlugins) WorkInfoFetchCover(videoID int64, fetchID string) (domain.WorkInfoCover, error) {
	return p.fetches.cover(fetchID, videoID, p.now())
}

// Apply covers after work-info commits; failures return the prior video and a skipped value instead of an error. Results are single-use, so an error would hide whether metadata was already applied.
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
	// Thumbnail application omits work information; copy the committed value.
	saved.WorkInfo = video.WorkInfo
	return saved, nil
}

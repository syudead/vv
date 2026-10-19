package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// PosterFrames extracts the player's poster from the video, its long side at most
// domain.ThumbnailFrameMaxSide.
type PosterFrames interface {
	// Poster picks the frame the automatic thumbnail picks.
	Poster(ctx context.Context, path string, durationMs int64, output string) error
	PosterAt(ctx context.Context, path string, positionMs int64, output string) error
}

// Poster extracts the current thumbnail's frame at poster size under the thumbnail generation lock,
// so a thumbnail published meanwhile cannot leave a poster of the frame it replaced. An applied
// image returns ErrThumbnailFrameUnavailable, because its stored source is the poster.
func (i *Ingest) Poster(ctx context.Context, videoID int64, locationPath, path string) error {
	if i.posters == nil {
		return domain.ErrThumbnailFrameUnavailable
	}
	video, err := i.store.GetVideo(ctx, videoID)
	if err != nil {
		return err
	}
	key := video.ContentKey
	if key == "" {
		return domain.ErrNotFound
	}
	return i.artifacts.generateContext(ctx, key, artifactThumbnail, func() (bool, error) {
		if i.files.PosterAvailable(key) {
			return false, nil
		}
		if err := i.checkThumbnailSource(ctx, videoID, key, locationPath); err != nil {
			return false, err
		}
		current, err := i.store.GetVideo(ctx, videoID)
		if err != nil {
			return false, err
		}
		if current.ContentKey != key {
			return false, domain.ErrContentChanged
		}
		image, err := i.store.ThumbnailImage(ctx, key)
		if err != nil {
			return false, err
		}
		if image != nil || current.ThumbnailState != domain.ThumbnailStateDone {
			return false, domain.ErrThumbnailFrameUnavailable
		}
		err = i.files.PublishPoster(key, func(output string) error {
			if current.ThumbnailPositionMs != nil {
				return i.posters.PosterAt(ctx, path, *current.ThumbnailPositionMs, output)
			}
			return i.posters.Poster(ctx, path, durationOf(current), output)
		})
		return err == nil, err
	})
}
